"""RAM admission + per-user quota (Phase 2)."""
import asyncio
import time
import unittest

from app.config import Config
from app.engine import (
    CamoufoxEngine,
    CapacityExceeded,
    PoolExhausted,
    Session,
    UserQuotaExceeded,
)
from app.recipes.base import RecipeError


def run(coro):
    return asyncio.run(coro)


def _engine(soft, hard, *, ram):
    cfg = Config(pool_size=4, warming_enabled=False,
                 ram_soft_bytes=soft, ram_hard_bytes=hard)
    eng = CamoufoxEngine(cfg)
    eng._sample_ram = lambda: ram          # pin the sampled RSS
    return eng


def _mk_session(eng, sid, *, user_key=None, expires_in=600, in_use=0):
    prof = eng.profiles.lease()
    s = Session(
        id=sid, profile=prof, proxy_id="direct", referer="r", user_agent="UA",
        cdn_host="h", master_url="m", expires_at=time.time() + expires_in,
        page=None, player_url="p",
    )
    s.user_key = user_key
    s.in_use = in_use
    eng._sessions[sid] = s
    return s


class TestCapacityExceptions(unittest.TestCase):
    def test_capacity_is_recipe_error_with_kind(self):
        exc = CapacityExceeded("hard RAM limit")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "capacity")

    def test_user_quota_is_recipe_error_with_kind(self):
        exc = UserQuotaExceeded("u over quota")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "user_quota")

    def test_pool_exhausted_still_distinct(self):
        self.assertNotIsInstance(PoolExhausted("x"), CapacityExceeded)


class TestAdmission(unittest.TestCase):
    def test_below_soft_admits_and_allows_warming(self):
        eng = _engine(1000, 2000, ram=500)
        eng._admit_launch()                # must not raise
        self.assertTrue(eng._warming_allowed())

    def test_soft_stops_warming_and_evicts_idle(self):
        eng = _engine(1000, 2000, ram=1500)   # soft <= ram < hard
        # one idle (not-in-use, expired) + one active (in_use) session
        _mk_session(eng, "idle", expires_in=-1, in_use=0)
        _mk_session(eng, "busy", expires_in=600, in_use=1)
        self.assertFalse(eng._warming_allowed())

        async def _admit():
            eng._admit_launch()            # soft must NOT refuse a launch

        run(_admit())                      # eviction _spawn() needs a live loop
        # idle session reclaimed; busy one survives
        self.assertNotIn("idle", eng._sessions)
        self.assertIn("busy", eng._sessions)

    def test_hard_refuses_launch_and_evicts_lru(self):
        eng = _engine(1000, 2000, ram=2500)   # ram >= hard
        old = _mk_session(eng, "old", in_use=0)
        old.expires_at = time.time() + 600
        new = _mk_session(eng, "new", in_use=0)
        new.expires_at = time.time() + 900

        async def _admit():
            eng._admit_launch()

        with self.assertRaises(CapacityExceeded):
            run(_admit())                  # eviction _spawn() needs a live loop
        # LRU (smallest expires_at = "old") evicted to reclaim headroom
        self.assertNotIn("old", eng._sessions)

    def test_hard_never_evicts_in_use_session(self):
        eng = _engine(1000, 2000, ram=2500)
        busy = _mk_session(eng, "busy", in_use=1)
        with self.assertRaises(CapacityExceeded):
            eng._admit_launch()
        self.assertIn("busy", eng._sessions)   # in-flight fetch protected

    def test_ram_read_failure_is_fail_safe_admit(self):
        eng = _engine(1000, 2000, ram=0)
        def boom():
            raise OSError("proc gone")
        eng._sample_ram = boom
        eng._admit_launch()                # fail-safe: admit, don't crash


class TestUserQuota(unittest.TestCase):
    def test_third_session_for_same_user_rejected(self):
        eng = _engine(10**12, 10**12, ram=0)   # RAM never the limiter here
        eng.cfg.user_quota = 2
        _mk_session(eng, "s1", user_key="alice")
        _mk_session(eng, "s2", user_key="alice")
        with self.assertRaises(UserQuotaExceeded):
            eng._enforce_user_quota("alice")

    def test_other_user_unaffected(self):
        eng = _engine(10**12, 10**12, ram=0)
        eng.cfg.user_quota = 2
        _mk_session(eng, "s1", user_key="alice")
        _mk_session(eng, "s2", user_key="alice")
        eng._enforce_user_quota("bob")          # bob has 0 → ok

    def test_empty_user_key_is_unbounded(self):
        eng = _engine(10**12, 10**12, ram=0)
        eng.cfg.user_quota = 1
        _mk_session(eng, "s1", user_key=None)
        _mk_session(eng, "s2", user_key=None)
        eng._enforce_user_quota(None)           # no key → never rejected
        eng._enforce_user_quota("")


class _FakePage:
    async def evaluate(self, js, *args):
        return "FakeUA/1.0"

    async def close(self):
        pass


class _FakeContext:
    async def new_page(self):
        return _FakePage()


class _FakeHandle:
    def __init__(self, opts):
        self._opts = opts

    async def open(self):
        return _FakeContext()


class TestWarmingTrigger(unittest.TestCase):
    """The ONLY warming trigger (_ensure_browser) must consult _warming_allowed()
    so the soft-budget back-pressure actually stops warming — not just expose a
    dead helper. Patches the launch internals so no real Camoufox is spawned."""

    def _engine_under(self, *, ram, soft=1000, hard=10**12):
        eng = CamoufoxEngine(Config(
            pool_size=2, warming_enabled=True,
            ram_soft_bytes=soft, ram_hard_bytes=hard,
        ))
        eng._sample_ram = lambda: ram
        return eng

    def _run_ensure_with_recorder(self, eng):
        """Drive the real _ensure_browser with fake launch internals; return True
        iff warm_profile was invoked by the warming trigger."""
        import app.engine as e
        import app.warming as w

        called = {"warmed": False}

        async def _fake_warm(page, sites, log, *, nav_timeout_ms):
            called["warmed"] = True

        orig_handle, orig_build = e._CamoufoxHandle, e.build_launch_options
        orig_warm = w.warm_profile
        e._CamoufoxHandle = _FakeHandle
        e.build_launch_options = lambda **k: {}
        w.warm_profile = _fake_warm
        try:
            profile = eng.profiles.lease()
            run(eng._ensure_browser(profile, "direct"))
        finally:
            e._CamoufoxHandle, e.build_launch_options = orig_handle, orig_build
            w.warm_profile = orig_warm
        return called["warmed"]

    def test_warming_fires_below_soft(self):
        eng = self._engine_under(ram=500)            # ram < soft ⇒ allowed
        self.assertTrue(eng._warming_allowed())
        self.assertTrue(self._run_ensure_with_recorder(eng),
                        "warm_profile must run when below the soft budget")

    def test_warming_suppressed_under_soft_pressure(self):
        eng = self._engine_under(ram=1500)           # soft <= ram < hard
        self.assertFalse(eng._warming_allowed())
        # The trigger must CONSULT _warming_allowed() — proves the gate is live,
        # not that the helper returns False in isolation. Pre-fix (ungated) this
        # would warm regardless and the assertion would fail.
        self.assertFalse(self._run_ensure_with_recorder(eng),
                         "warm_profile must NOT run once at/over the soft budget")


class TestErrorBodies(unittest.TestCase):
    def _set_engine(self, engine):
        import app.main as m
        m.app.state.engine = engine
        return m

    def test_resolve_capacity_is_503_kind_capacity(self):
        import json
        from app.main import ResolveRequest
        from app.engine import CapacityExceeded

        eng = _engine(1, 1, ram=0)

        async def _boom(*a, **k):
            raise CapacityExceeded("hard")
        eng.resolve = _boom
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime", embed_url="https://x/y")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "capacity")

    def test_resolve_user_quota_is_503_kind_user_quota(self):
        import json
        from app.main import ResolveRequest
        from app.engine import UserQuotaExceeded

        eng = _engine(1, 1, ram=0)

        async def _boom(*a, **k):
            raise UserQuotaExceeded("over")
        eng.resolve = _boom
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime", embed_url="https://x/y")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "user_quota")

    def test_request_models_accept_user_key(self):
        from app.main import ResolveRequest, FetchRequest
        r = ResolveRequest(provider="gogoanime", embed_url="https://x/y", user_key="alice")
        f = FetchRequest(provider="nineanime", url="https://9anime.me.uk/x", user_key="bob")
        self.assertEqual(r.user_key, "alice")
        self.assertEqual(f.user_key, "bob")


if __name__ == "__main__":
    unittest.main()
