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

    async def evaluate(self, js, url):
        self.calls += 1
        return f"{self._status}|{self._ctype}|{url}|{base64.b64encode(self._body).decode()}"

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


if __name__ == "__main__":
    unittest.main()
