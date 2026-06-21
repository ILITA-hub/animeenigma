"""proxy_fetch tests — the browser-restream path that makes Cloudflare-
fingerprint-gated CDNs work: fetch playlist + segments through the session
page's IN-PAGE fetch, rewrite playlists, pass segments through, and allow the
rotating segment CDN host (which differs from the master host).

Also covers the hardened SSRF guard (host_allowed_for_session): https-only,
DNS-resolve-and-reject-private (octal/hex/decimal-IP, DNS-rebind, IPv4-mapped
IPv6 bypasses), and the post-redirect re-validation."""

import base64
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session, host_allowed_for_session
from app.recipes.base import RecipeError


def run(coro):
    import asyncio
    return asyncio.run(coro)


# Injectable async DNS resolver: maps the test hostnames to fixed addresses so
# the SSRF guard is exercised without touching real DNS. IP literals are parsed
# directly by the guard (never passed here); an unknown host raises (⇒ deny).
_DNS = {
    "cdn.mewstream.buzz": ["104.20.0.1"],
    "x9.flarestorm.buzz": ["104.20.0.2"],
    "good.example.com": ["93.184.216.34"],
    "0177.0.0.1": ["127.0.0.1"],              # octal-encoded loopback
    "127.0.0.1.nip.io": ["127.0.0.1"],        # DNS-rebind to loopback
    "metadata.google.internal": ["169.254.169.254"],  # cloud metadata (link-local)
    "evil.example.com": ["10.0.0.5"],         # public name → private A record
}


async def _fake_resolve(host):
    if host in _DNS:
        return _DNS[host]
    raise OSError(f"no fake DNS entry for {host}")


class _ProxyPage:
    """Fake page whose evaluate() mimics the in-page fetch JS contract:
    returns "status|content-type|final-url|base64(body)". A response may carry a
    4th element to simulate a redirect (final_url != requested url)."""

    url = "https://megaplay.buzz/stream/s-2/122211/sub"

    def __init__(self, responses):
        self._responses = responses  # url -> (status, content_type, body[, final_url])

    async def evaluate(self, js, url):
        entry = self._responses[url]
        st, ct, body = entry[0], entry[1], entry[2]
        final_url = entry[3] if len(entry) > 3 else url
        return f"{st}|{ct}|{final_url}|{base64.b64encode(body).decode()}"

    async def close(self):
        pass


MASTER = "https://cdn.mewstream.buzz/a/b/master.m3u8"
VARIANT = "https://cdn.mewstream.buzz/a/b/index-v1.m3u8"
SEG = "https://x9.flarestorm.buzz/a/b/seg0.ts"  # different host from master!
SEG_BYTES = bytes([0x47, 0x40, 0x00, 0x10]) + b"\x00" * 512  # MPEG-TS sync byte
REDIRECTER = "https://good.example.com/redirect.ts"


def _engine_with_session():
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
    eng._resolve_host = _fake_resolve  # inject test DNS
    prof = eng.profiles.lease()
    page = _ProxyPage({
        MASTER: (200, "application/vnd.apple.mpegurl",
                 f"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n{VARIANT}\n".encode()),
        VARIANT: (200, "application/vnd.apple.mpegurl",
                  f"#EXTM3U\n#EXTINF:6,\n{SEG}\n#EXT-X-ENDLIST\n".encode()),
        SEG: (200, "image/jpeg", SEG_BYTES),  # CDN mislabels TS as jpeg
        # A "trusted" public host that 30x-redirects the in-page fetch to an
        # internal target — the post-redirect re-check must refuse it.
        REDIRECTER: (200, "video/mp2t", SEG_BYTES, "https://10.0.0.9/internal.ts"),
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
        self.assertTrue(run(host_allowed_for_session(SEG, sess, _fake_resolve)))
        self.assertTrue(run(host_allowed_for_session(MASTER, sess, _fake_resolve)))

    def test_redirect_to_internal_refused(self):
        eng, _ = _engine_with_session()
        # The fetch follows a 30x to https://10.0.0.9/... — body must NOT return.
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", REDIRECTER))


class TestSSRFGuard(unittest.TestCase):
    def _blocked(self, url):
        _, sess = _engine_with_session()
        self.assertFalse(
            run(host_allowed_for_session(url, sess, _fake_resolve)), f"{url} must be blocked"
        )

    def _allowed(self, url):
        _, sess = _engine_with_session()
        self.assertTrue(
            run(host_allowed_for_session(url, sess, _fake_resolve)), f"{url} must be allowed"
        )

    def test_trivial_private_and_names_blocked(self):
        for u in (
            "http://localhost:6379/x",
            "https://10.0.0.1/x",
            "https://192.168.1.1/x",
            "https://169.254.169.254/x",       # link-local literal
            "https://stealth-scraper:3000/x",
            "https://redis/x",                 # bare docker service name
            "https://catalog.local/x",
        ):
            self._blocked(u)

    def test_encoded_and_rebind_bypasses_blocked(self):
        for u in (
            "https://0177.0.0.1/x",            # octal loopback
            "https://127.0.0.1.nip.io/x",      # DNS-rebind → loopback
            "https://metadata.google.internal/latest/meta-data",  # cloud metadata
            "https://evil.example.com/x",      # public name → private A record
            "https://[::1]/x",                 # IPv6 loopback literal
            "https://[::ffff:127.0.0.1]/x",    # IPv4-mapped IPv6 loopback
        ):
            self._blocked(u)

    def test_http_scheme_blocked_on_all_branches(self):
        # http:// must be rejected even on a would-be-trusted CDN/player host.
        self._blocked("http://cdn.mewstream.buzz/x")
        self._blocked("http://megaplay.buzz/x")

    def test_public_https_allowed(self):
        self._allowed("https://good.example.com/seg.ts")
        self._allowed(MASTER)
        self._allowed(SEG)

    def test_proxy_fetch_rejects_private_target(self):
        eng, _ = _engine_with_session()
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", "http://127.0.0.1:6379/x"))


class TestSessionGone(unittest.TestCase):
    def test_unknown_session_raises(self):
        eng, _ = _engine_with_session()
        from app.engine import SessionGone
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch("nope", MASTER))


if __name__ == "__main__":
    unittest.main()
