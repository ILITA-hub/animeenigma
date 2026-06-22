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


if __name__ == "__main__":
    unittest.main()
