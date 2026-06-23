"""browser_fetch: in-page fetch of an allowlisted discovery URL through a warm,
challenge-solved session keyed per (provider, origin). Returns the raw body.
Used for providers whose whole site is challenge-gated (9anime DDoS-Guard)."""
import base64
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.recipes.base import ChallengeError, RecipeError


def run(coro):
    import asyncio
    return asyncio.run(coro)


class _FetchPage:
    """Fake page: evaluate() mimics the in-page fetch JS contract
    'status|content-type|final-url|base64(body)'. Counts calls for reuse asserts."""
    url = "https://9anime.me.uk/"

    def __init__(self, body: bytes, status: int = 200, ctype: str = "application/json"):
        self._body, self._status, self._ctype = body, status, ctype
        self.calls = 0

    async def evaluate(self, js, *args):
        if not args:      # liveness probe `()=>1`
            return 1
        self.calls += 1
        return f"{self._status}|{self._ctype}|{args[0]}|{base64.b64encode(self._body).decode()}"

    async def close(self):
        pass


def _engine_with_fetch_session(body=b'{"ok":1}', status=200, ctype="application/json",
                               key="fetch::nineanime::https://9anime.me.uk"):
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
    prof = eng.profiles.lease()
    page = _FetchPage(body, status, ctype)
    sess = Session(
        id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
        user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
        expires_at=time.time() + 600, page=page, player_url=page.url,
    )
    eng._sessions[key] = sess
    return eng, sess, page


class TestBrowserFetch(unittest.TestCase):
    def test_unknown_provider_raises(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        with self.assertRaises(RecipeError):
            run(eng.browser_fetch("nope", "https://9anime.me.uk/x"))

    def test_host_not_allowed_raises(self):
        eng, _, _ = _engine_with_fetch_session()
        with self.assertRaises(RecipeError):
            run(eng.browser_fetch("nineanime", "https://evil.example.com/x"))

    def test_returns_raw_body_via_warm_session(self):
        eng, _, page = _engine_with_fetch_session(body=b'{"hello":"world"}')
        out = run(eng.browser_fetch("nineanime", "https://9anime.me.uk/wp-json/wp/v2/search?search=x"))
        self.assertEqual(out["status"], 200)
        self.assertEqual(out["body"], b'{"hello":"world"}')
        self.assertEqual(page.calls, 1)

    def test_session_reused_per_origin(self):
        eng, _, page = _engine_with_fetch_session()
        run(eng.browser_fetch("nineanime", "https://9anime.me.uk/a"))
        run(eng.browser_fetch("nineanime", "https://9anime.me.uk/b"))
        self.assertEqual(page.calls, 2)               # same page reused
        self.assertEqual(len(eng._sessions), 1)        # no second session opened

    def test_challenge_body_raises_and_drops_session(self):
        eng, _, _ = _engine_with_fetch_session(body=b"<html><title>Just a moment...</title>")
        with self.assertRaises(ChallengeError):
            run(eng.browser_fetch("nineanime", "https://9anime.me.uk/x"))
        self.assertEqual(len(eng._sessions), 0)        # poisoned session dropped


class TestFetchRoute(unittest.TestCase):
    """Exercise the /fetch route handler directly (no httpx/TestClient dep)."""

    def _set_engine(self, engine):
        import app.main as m
        m.app.state.engine = engine
        return m

    def test_fetch_route_returns_base64_body(self):
        import json
        from app.main import FetchRequest
        eng, _, _ = _engine_with_fetch_session(body=b'{"hello":"world"}')
        m = self._set_engine(eng)
        resp = run(m.fetch(FetchRequest(
            provider="nineanime",
            url="https://9anime.me.uk/wp-json/wp/v2/search?search=x")))
        self.assertEqual(resp.status_code, 200)
        data = json.loads(resp.body)
        self.assertTrue(data["success"])
        self.assertEqual(data["status"], 200)
        self.assertEqual(base64.b64decode(data["body"]), b'{"hello":"world"}')

    def test_fetch_route_host_denied_is_502(self):
        import json
        from app.main import FetchRequest
        eng, _, _ = _engine_with_fetch_session()
        m = self._set_engine(eng)
        resp = run(m.fetch(FetchRequest(provider="nineanime", url="https://evil.example.com/x")))
        self.assertEqual(resp.status_code, 502)
        self.assertEqual(json.loads(resp.body)["kind"], "error")


class _PoisonPage:
    """evaluate() raises 'Target closed' to simulate a poisoned warm page.
    A liveness probe `()=>1` evaluation (no url arg) returns 1 so we can tell
    the poison-fence apart from the liveness probe."""
    url = "https://9anime.me.uk/"

    def __init__(self):
        self.calls = 0
        self.gotos = 0

    async def evaluate(self, js, *args):
        # Liveness probe: `()=>1` takes no url arg.
        if not args:
            return 1
        self.calls += 1
        raise RuntimeError("Target closed")

    async def goto(self, *a, **k):
        self.gotos += 1

    async def close(self):
        pass


def _engine_poison_session(poison_max=2):
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False, poison_max=poison_max))
    prof = eng.profiles.lease()
    page = _PoisonPage()
    key = "fetch::nineanime::https://9anime.me.uk"
    sess = Session(
        id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
        user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
        expires_at=time.time() + 600, page=page, player_url=page.url, provider="nineanime",
    )
    eng._sessions[key] = sess
    return eng, sess, page


class TestPoisonFence(unittest.TestCase):
    def test_no_nav_retry_on_target_closed(self):
        from app.engine import ProviderWedged
        eng, sess, page = _engine_poison_session(poison_max=2)
        # First crash: increments crash_count, does NOT nav-retry, raises.
        with self.assertRaises(Exception):
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))
        self.assertEqual(sess.crash_count, 1)
        self.assertEqual(page.gotos, 0, "must NOT re-navigate the poisoned page")
        self.assertIn(sess.id, eng._sessions, "below poison_max: session retained")

    def test_poison_max_tears_down_and_wedges(self):
        from app.engine import ProviderWedged
        eng, sess, page = _engine_poison_session(poison_max=2)
        with self.assertRaises(Exception):
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))   # crash_count=1
        with self.assertRaises(ProviderWedged) as ctx:
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))   # crash_count=2 -> wedge
        self.assertEqual(ctx.exception.provider, "nineanime")
        self.assertNotIn(sess.id, eng._sessions, "wedged session must be closed")
        self.assertEqual(sess.profile.status, "crashed", "slot marked crashed for the reaper")


class _DeadProbePage:
    """Liveness probe `()=>1` raises (page is dead); used to assert the warm-
    reuse path evicts the session rather than handing back a poisoned page."""
    url = "https://9anime.me.uk/"

    def __init__(self):
        self.probed = False

    async def evaluate(self, js, *args):
        if not args:  # liveness probe
            self.probed = True
            raise RuntimeError("Target closed")
        return 1

    async def close(self):
        pass


class TestWarmReuseLiveness(unittest.TestCase):
    def test_dead_warm_session_is_evicted_not_reused(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        prof = eng.profiles.lease()
        page = _DeadProbePage()
        key = "fetch::nineanime::https://9anime.me.uk"
        sess = Session(
            id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
            user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
            expires_at=time.time() + 600, page=page, player_url=page.url, provider="nineanime",
        )
        eng._sessions[key] = sess
        # The dead session must NOT be returned by the reuse fast-path. Since the
        # pool has no free profile after the dead one is released, recreation
        # will raise PoolExhausted/RecipeError — but the dead session is gone.
        from app.engine import PoolExhausted
        with self.assertRaises((PoolExhausted, RecipeError)):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk"))
        self.assertTrue(page.probed, "reuse path must run the liveness probe")
        self.assertNotIn(key, eng._sessions, "dead warm session must be evicted")


class _FakeHandle:
    """Stand-in for _CamoufoxHandle: holds a context, records close()."""
    def __init__(self):
        self.context = object()
        self.closed = False

    async def close(self):
        self.closed = True
        self.context = None


class TestWarmReuseLivenessResurrectionSeam(unittest.TestCase):
    """Interlock for the resurrection seam: a warm-session liveness-fail must
    tear the profile's browser handle/context fully down (NOT just close the
    page), so the reaper's _resurrect_crashed_slot -> _ensure_browser actually
    cold-relaunches instead of the launched-guard short-circuiting a dead
    context. The existing resurrection tests STUB _ensure_browser, which bypasses
    that guard — this exercises the real seam."""

    def _engine_with_launched_dead_session(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        prof = eng.profiles.lease()
        # Simulate a fully-launched browser slot: handle in _handles + context on
        # the profile (so profile.launched is True and the _ensure_browser guard
        # would fire on a matching proxy_id).
        handle = _FakeHandle()
        prof.proxy_id = "direct"
        prof.browser = handle
        prof.context = handle.context
        eng._handles[prof.id] = handle
        page = _DeadProbePage()
        key = "fetch::nineanime::https://9anime.me.uk"
        sess = Session(
            id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
            user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
            expires_at=time.time() + 600, page=page, player_url=page.url, provider="nineanime",
        )
        eng._sessions[key] = sess
        return eng, prof, handle, key

    def test_liveness_fail_tears_down_handle_so_reaper_relaunches(self):
        eng, prof, handle, key = self._engine_with_launched_dead_session()

        # (1) Warm-reuse liveness probe fails -> the seam must fully tear down.
        from app.engine import PoolExhausted
        with self.assertRaises((PoolExhausted, RecipeError)):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk"))

        # The dead browser handle/context must be GONE, not just the page.
        self.assertNotIn(key, eng._sessions, "dead warm session must be evicted")
        self.assertNotIn(prof.id, eng._handles,
                         "dead browser handle must be popped from _handles")
        self.assertFalse(prof.launched,
                         "profile.context must be cleared so launched==False")
        self.assertTrue(handle.closed, "the dead browser handle must be closed")
        self.assertEqual(prof.status, "crashed",
                         "slot must be marked crashed for the reaper")

        # (2) The reaper now resurrects: because launched==False, the
        # _ensure_browser guard must NOT short-circuit — a REAL relaunch happens.
        relaunches = {"n": 0}
        real_ensure = eng._ensure_browser

        async def _counting_ensure(profile, proxy_id):
            relaunches["n"] += 1
            # Re-attach a fresh handle/context the way a real cold launch would.
            h = _FakeHandle()
            eng._handles[profile.id] = h
            profile.browser = h
            profile.context = h.context
            profile.proxy_id = proxy_id
            return h.context

        eng._ensure_browser = _counting_ensure
        prof.next_resurrect_at = 0.0  # eligible now
        run(eng._resurrect_crashed_slot(prof))

        self.assertEqual(relaunches["n"], 1,
                         "resurrect must invoke _ensure_browser (no dead-context short-circuit)")
        self.assertEqual(prof.status, "healthy", "resurrected slot returns to the pool")
        self.assertEqual(prof.consecutive_fail, 0)

    def test_old_behavior_would_leak_dead_handle(self):
        """Regression guard: prove the assertion actually depends on teardown.
        If the seam only marked-crashed + closed the page (the OLD bug), the
        handle would persist and launched would stay True — the guard below
        documents exactly that failure mode is what we fixed."""
        eng, prof, handle, key = self._engine_with_launched_dead_session()
        from app.engine import PoolExhausted
        with self.assertRaises((PoolExhausted, RecipeError)):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk"))
        # Post-fix invariants (these are the lines that fail against OLD code).
        self.assertFalse(prof.launched)
        self.assertNotIn(prof.id, eng._handles)


def _held_fetch_session(eng, sid, *, user_key=None, expires_in=600, in_use=0):
    """Pin a held warm session attributed to ``user_key`` (no live page needed —
    only its presence in eng._sessions matters for the quota count)."""
    prof = eng.profiles.lease()
    sess = Session(
        id=sid, profile=prof, proxy_id="direct", referer="r", user_agent="UA",
        cdn_host="h", master_url="m", expires_at=time.time() + expires_in,
        page=None, player_url="p", user_key=user_key,
    )
    sess.in_use = in_use
    eng._sessions[sid] = sess
    return sess


class TestWarmFetchUserQuota(unittest.TestCase):
    """The /fetch (warm-fetch discovery) path must enforce the per-user quota the
    same way resolve() does — a long-lived warm session is the exact poison-prone
    resource the quota bounds."""

    def test_third_warm_fetch_for_same_user_raises_user_quota(self):
        from app.engine import UserQuotaExceeded
        eng = CamoufoxEngine(Config(pool_size=4, warming_enabled=False))
        eng.cfg.user_quota = 2
        eng._sample_ram = lambda: 0  # RAM never the limiter here
        # alice already holds 2 warm sessions on other origins.
        _held_fetch_session(eng, "fetch::nineanime::https://a.example", user_key="alice")
        _held_fetch_session(eng, "fetch::nineanime::https://b.example", user_key="alice")
        # A 3rd, for a FRESH origin (no reuse fast-path), must trip the quota.
        with self.assertRaises(UserQuotaExceeded):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk", user_key="alice"))

    def test_fetch_route_user_quota_is_503_kind_user_quota(self):
        import json
        from app.main import FetchRequest
        eng = CamoufoxEngine(Config(pool_size=4, warming_enabled=False))
        eng.cfg.user_quota = 2
        eng._sample_ram = lambda: 0
        _held_fetch_session(eng, "fetch::nineanime::https://a.example", user_key="alice")
        _held_fetch_session(eng, "fetch::nineanime::https://b.example", user_key="alice")
        import app.main as m
        m.app.state.engine = eng
        resp = run(m.fetch(FetchRequest(
            provider="nineanime", url="https://9anime.me.uk/x", user_key="alice")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "user_quota")


class TestWarmFetchCapacitySurfacing(unittest.TestCase):
    """A CapacityExceeded raised inside the warm path (e.g. _ensure_browser →
    _admit_launch refusing a launch over the hard RAM budget) must keep its
    concrete class so the /fetch handler emits kind=capacity — NOT be flattened
    to a generic RecipeError (kind=error) by the warm-path catch-all."""

    def test_capacity_in_warm_path_surfaces_as_capacity(self):
        from app.engine import CapacityExceeded
        # ram >= hard ⇒ _admit_launch (called inside _ensure_browser) raises
        # CapacityExceeded. No held session ⇒ nothing to evict ⇒ it propagates.
        eng = CamoufoxEngine(Config(pool_size=4, warming_enabled=False,
                                    ram_soft_bytes=1000, ram_hard_bytes=2000))
        eng._sample_ram = lambda: 2500
        with self.assertRaises(CapacityExceeded):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk", user_key="alice"))

    def test_fetch_route_capacity_in_warm_path_is_503_kind_capacity(self):
        import json
        from app.main import FetchRequest
        eng = CamoufoxEngine(Config(pool_size=4, warming_enabled=False,
                                    ram_soft_bytes=1000, ram_hard_bytes=2000))
        eng._sample_ram = lambda: 2500
        import app.main as m
        m.app.state.engine = eng
        resp = run(m.fetch(FetchRequest(
            provider="nineanime", url="https://9anime.me.uk/x", user_key="alice")))
        self.assertEqual(resp.status_code, 503)
        # The whole point: kind=capacity, NOT kind=error (the pre-fix flattening).
        self.assertEqual(json.loads(resp.body)["kind"], "capacity")


if __name__ == "__main__":
    unittest.main()
