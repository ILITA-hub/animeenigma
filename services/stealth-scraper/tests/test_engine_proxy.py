"""proxy_fetch tests — the browser-restream path that makes Cloudflare-
fingerprint-gated CDNs work: fetch playlist + segments through the session
page's IN-PAGE fetch, rewrite playlists, pass segments through, and allow the
rotating segment CDN host (which differs from the master host)."""

import base64
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session, host_allowed_for_session
from app.recipes.base import RecipeError


def run(coro):
    import asyncio
    return asyncio.run(coro)


class _ProxyPage:
    """Fake page whose evaluate() mimics the in-page fetch JS contract:
    returns "status|content-type|base64(body)"."""

    url = "https://megaplay.buzz/stream/s-2/122211/sub"

    def __init__(self, responses):
        self._responses = responses  # url -> (status, content_type, body bytes)

    async def evaluate(self, js, url):
        st, ct, body = self._responses[url]
        return f"{st}|{ct}|{base64.b64encode(body).decode()}"

    async def close(self):
        pass


MASTER = "https://cdn.mewstream.buzz/a/b/master.m3u8"
VARIANT = "https://cdn.mewstream.buzz/a/b/index-v1.m3u8"
SEG = "https://x9.flarestorm.buzz/a/b/seg0.ts"  # different host from master!
SEG_BYTES = bytes([0x47, 0x40, 0x00, 0x10]) + b"\x00" * 512  # MPEG-TS sync byte


def _engine_with_session():
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
    prof = eng.profiles.lease()
    page = _ProxyPage({
        MASTER: (200, "application/vnd.apple.mpegurl",
                 f"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n{VARIANT}\n".encode()),
        VARIANT: (200, "application/vnd.apple.mpegurl",
                  f"#EXTM3U\n#EXTINF:6,\n{SEG}\n#EXT-X-ENDLIST\n".encode()),
        SEG: (200, "image/jpeg", SEG_BYTES),  # CDN mislabels TS as jpeg
    })
    sess = Session(
        id="sid1", profile=prof, proxy_id="direct", referer="https://megaplay.buzz/",
        user_agent="UA", cdn_host="cdn.mewstream.buzz", master_url=MASTER,
        expires_at=time.time() + 600, page=page, player_url=page.url,
    )
    eng._sessions["sid1"] = sess
    return eng, sess


class TestProxyFetch(unittest.TestCase):
    def test_playlist_rewritten_to_route_back(self):
        eng, _ = _engine_with_session()
        out = run(eng.proxy_fetch("sid1", MASTER))
        self.assertEqual(out["content_type"], "application/vnd.apple.mpegurl")
        body = out["body"].decode()
        # variant URI rewritten to route back through this /hls proxy
        self.assertIn("/hls?sid=sid1", body)
        self.assertNotIn(VARIANT, body)

    def test_segment_passthrough_bytes(self):
        eng, _ = _engine_with_session()
        out = run(eng.proxy_fetch("sid1", SEG))
        self.assertEqual(out["body"], SEG_BYTES)
        self.assertEqual(out["body"][0], 0x47)  # valid TS sync byte survived b64 roundtrip

    def test_rotating_segment_cdn_host_allowed(self):
        _, sess = _engine_with_session()
        # segment host (flarestorm) != master host (mewstream) — must be allowed
        self.assertTrue(host_allowed_for_session(SEG, sess))
        self.assertTrue(host_allowed_for_session(MASTER, sess))

    def test_ssrf_private_host_blocked(self):
        eng, sess = _engine_with_session()
        self.assertFalse(host_allowed_for_session("http://localhost:6379/x", sess))
        self.assertFalse(host_allowed_for_session("https://10.0.0.1/x", sess))
        self.assertFalse(host_allowed_for_session("https://stealth-scraper:3000/x", sess))
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", "http://127.0.0.1:6379/x"))

    def test_unknown_session_raises(self):
        eng, _ = _engine_with_session()
        from app.engine import SessionGone
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch("nope", MASTER))


if __name__ == "__main__":
    unittest.main()
