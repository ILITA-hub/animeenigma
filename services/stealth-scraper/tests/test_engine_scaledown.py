"""Graduated scale-down (spec 2026-07-21): user/service classification, victim
selection, and the per-tick drain-trigger threshold. Migration itself (the
happy path + failure path) is exercised in the migration test class below with
stubbed browser internals — no real Camoufox."""
import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app import metrics, scaling


def _engine(pool_size=6, **kw):
    cfg = Config(pool_size=pool_size, warming_enabled=False, **kw)
    eng = CamoufoxEngine(cfg)
    eng._sample_ram = lambda: 0  # RAM never the limiter here
    return eng


def _mk_session(eng, sid, *, user_key=None, in_use=0, age_s=0, expires_in=600):
    prof = eng.profiles.lease()
    s = Session(
        id=sid, profile=prof, proxy_id="direct", referer="r", user_agent="UA",
        cdn_host="h", master_url="m", expires_at=time.time() + expires_in,
        page=None, player_url="p",
    )
    s.user_key = user_key
    s.in_use = in_use
    s.last_persist = time.time() - age_s
    eng._sessions[sid] = s
    return s


class ClassificationTest(unittest.TestCase):
    def test_user_session_requires_key_and_live_stream(self):
        eng = _engine()
        cases = [
            ("u1", 1, 999, True),   # in-flight fetch
            ("u1", 0, 60, True),    # recent segment activity
            ("u1", 0, 600, False),  # idle user session = service-class
            (None, 5, 10, False),   # probe/warm: no key, never user
        ]
        for user_key, in_use, age_s, expected in cases:
            s = _mk_session(eng, f"s-{user_key}-{in_use}-{age_s}",
                             user_key=user_key, in_use=in_use, age_s=age_s)
            self.assertEqual(
                eng._session_is_user(s), expected,
                f"user_key={user_key!r} in_use={in_use} age_s={age_s}",
            )


class VictimOrderTest(unittest.TestCase):
    def test_service_class_lru_first_then_user(self):
        eng = _engine()
        # user-live session expires soonest (best LRU position by expires_at)
        # but must NOT be picked while a service-class session exists.
        _mk_session(eng, "user-live", user_key="alice", in_use=1, expires_in=10)
        _mk_session(eng, "service", user_key=None, in_use=0, expires_in=9999)
        picked = eng._pick_victim()
        self.assertIsNotNone(picked)
        sid, _s = picked
        self.assertEqual(sid, "service")

    def test_falls_back_to_user_when_no_service_class(self):
        eng = _engine()
        _mk_session(eng, "user-a", user_key="alice", in_use=1, expires_in=10)
        _mk_session(eng, "user-b", user_key="bob", in_use=1, expires_in=999)
        picked = eng._pick_victim()
        self.assertIsNotNone(picked)
        sid, _s = picked
        self.assertEqual(sid, "user-a")  # LRU (smallest expires_at) among users

    def test_draining_sessions_excluded(self):
        eng = _engine()
        svc = _mk_session(eng, "service", user_key=None, in_use=0, expires_in=10)
        svc.draining = True
        _mk_session(eng, "other", user_key=None, in_use=0, expires_in=9999)
        picked = eng._pick_victim()
        self.assertIsNotNone(picked)
        sid, _s = picked
        self.assertEqual(sid, "other")

    def test_no_candidates_returns_none(self):
        eng = _engine()
        self.assertIsNone(eng._pick_victim())


class DrainTriggerTest(unittest.TestCase):
    def _run(self, coro):
        import asyncio
        return asyncio.run(coro)

    def test_no_action_at_or_under_threshold(self):
        eng = _engine()
        eng._degradation_score = 0.60  # curve default 0.60:2 -> target=2... use explicit curve
        eng.cfg.pool_curve = "0.60:4"
        eng._pool_curve = scaling.parse_curve(eng.cfg.pool_curve)
        # target=4 -> threshold ceil(4/2)+1 = 3; current=3 -> no victim picked.
        for i in range(3):
            _mk_session(eng, f"s{i}", user_key=None, in_use=0)
        self.assertEqual(eng._pool_target(), 4)
        self._run(eng._scale_down_step())
        self.assertEqual(len(eng._sessions), 3)
        for s in eng._sessions.values():
            self.assertFalse(s.draining)

    def test_over_threshold_picks_victim_and_sets_draining(self):
        eng = _engine()
        eng.cfg.pool_curve = "0.60:2"
        eng._pool_curve = scaling.parse_curve(eng.cfg.pool_curve)
        # target=2 -> threshold ceil(2/2)+1 = 2; current=3 (all service) -> one victim drains.
        for i in range(3):
            _mk_session(eng, f"s{i}", user_key=None, in_use=0)
        self.assertEqual(eng._pool_target(), 2)
        self._run(eng._scale_down_step())
        # Service-class victims are force-killed synchronously by the stub, so
        # the pool should have shrunk by exactly one.
        self.assertEqual(len(eng._sessions), 2)


class MigrationFailureTest(unittest.TestCase):
    """Migration-failure leaves the victim in self._sessions with
    draining == False (spec: user-stream sanctity — a victim that cannot be
    migrated survives untouched, above target, until a later tick)."""

    def _run(self, coro):
        import asyncio
        return asyncio.run(coro)

    def test_migration_failure_survives_with_draining_cleared(self):
        eng = _engine()
        eng.cfg.pool_curve = "0.60:1"
        eng._pool_curve = scaling.parse_curve(eng.cfg.pool_curve)
        # target=1 -> threshold ceil(1/2)+1 = 2. Two decoys are already
        # draining (excluded from _pick_victim, but still counted in
        # len(_sessions)) so "victim" is deterministically the only pickable
        # candidate — current=3 > threshold=2 triggers the step.
        victim = _mk_session(eng, "victim", user_key="alice", in_use=1, expires_in=10)
        victim.player_url = "https://player.example/watch/1"
        for i in range(2):
            decoy = _mk_session(eng, f"decoy{i}", user_key=None, in_use=0, expires_in=9999)
            decoy.draining = True
        self.assertEqual(eng._pool_target(), 1)
        self.assertEqual(len(eng._sessions), 3)

        calls = []

        async def _boom(sid):
            calls.append(sid)
            return False

        eng._migrate_session = _boom
        self._run(eng._scale_down_step())
        self.assertEqual(calls, ["victim"])
        self.assertIn("victim", eng._sessions)
        self.assertFalse(eng._sessions["victim"].draining)

    def test_migrate_session_failure_via_no_survivors(self):
        eng = _engine()
        victim = _mk_session(eng, "victim", user_key="alice", in_use=1, expires_in=10)
        victim.player_url = "https://player.example/watch/1"
        ok = self._run(eng._migrate_session("victim"))
        self.assertFalse(ok)
        self.assertIn("victim", eng._sessions)


class _FakeGotoPage:
    """Mirrors tests/test_engine_lifecycle.py's _Page + test_engine_rehydrate.py's
    _GotoPage: records the navigated URL and answers the in-page fetch JS with a
    canned 200 (master-playlist verify passes)."""

    url = "https://player.example/watch/1"

    def __init__(self):
        self.goto_url = None
        self.closed = False

    async def goto(self, url, **kw):
        self.goto_url = url

    async def evaluate(self, js, url):
        return f"200|application/vnd.apple.mpegurl|{url}||"

    async def close(self):
        self.closed = True


class _FakeSurvivorContext:
    def __init__(self, page):
        self._page = page

    async def new_page(self):
        return self._page


def _counter_val(counter, **labels):
    try:
        return counter.labels(**labels)._value.get()
    except Exception:  # noqa: BLE001
        return 0.0


class MigrationSuccessTest(unittest.TestCase):
    """Happy-path migration: the victim's live stream re-resolves on a
    survivor's browser context under the SAME sid, verifies a 200 on the
    master playlist, then swaps self._sessions[sid] with owns_profile=False."""

    def _run(self, coro):
        import asyncio
        return asyncio.run(coro)

    def _engine(self):
        cfg = Config(pool_size=4, warming_enabled=False,
                     profile_dir=tempfile.mkdtemp())
        eng = CamoufoxEngine(cfg)
        eng._sample_ram = lambda: 0
        return eng

    def test_migrate_swaps_session_without_double_leasing_profile(self):
        eng = self._engine()
        victim = _mk_session(eng, "victim", user_key="alice", in_use=0, age_s=5,
                              expires_in=10)
        victim.player_url = "https://player.example/watch/1"
        victim.master_url = "https://cdn.example/master.m3u8"
        survivor = _mk_session(eng, "survivor", user_key="bob", in_use=0, age_s=5,
                                expires_in=9999)
        self.assertTrue(survivor.owns_profile)

        page = _FakeGotoPage()

        async def _ensure(profile, proxy_id):
            self.assertIs(profile, survivor.profile)
            return _FakeSurvivorContext(page)

        eng._ensure_browser = _ensure

        before_ok = _counter_val(metrics.STREAM_MIGRATIONS, result="ok")
        ok = self._run(eng._migrate_session("victim"))
        self.assertTrue(ok)
        self.assertEqual(page.goto_url, victim.player_url)

        moved = eng._sessions["victim"]
        self.assertIsNot(moved, victim)          # session object was swapped
        self.assertFalse(moved.owns_profile)     # rides the survivor's lease
        self.assertIs(moved.profile, survivor.profile)
        # The survivor's own session entry is untouched — its lease is not
        # doubled and its own sid keeps owning the profile.
        self.assertIs(eng._sessions["survivor"].profile, survivor.profile)
        self.assertTrue(eng._sessions["survivor"].owns_profile)
        self.assertEqual(
            _counter_val(metrics.STREAM_MIGRATIONS, result="ok"), before_ok + 1
        )
        # Neither profile is left double-leased/mis-released by the swap
        # itself (the old victim's page-retire/lease-release is deferred to
        # _retire_after_drain, not asserted here).
        self.assertTrue(survivor.profile.leased)


if __name__ == "__main__":
    unittest.main()
