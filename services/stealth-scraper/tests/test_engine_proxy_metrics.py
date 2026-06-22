"""proxy_fetch observability metrics (audit finding L613).

The resource-leak core (fetch timeout + body-size cap) is already fixed; these
tests pin the *observability* that was missing: a duration Histogram and a
result-labelled Counter around the in-page fetch, so an operator can see
ok/too_large/timeout/host_denied rates on the restream path.

Reuses the FakeSession/FakePage shape from test_engine_proxy.py."""

import base64
import time
import unittest

from app import metrics
from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.recipes.base import RecipeError


def run(coro):
    import asyncio
    return asyncio.run(coro)


# Same fake-DNS map the proxy SSRF tests use, so host_allowed_for_session passes
# without touching real DNS.
_DNS = {
    "cdn.mewstream.buzz": ["104.20.0.1"],
    "x9.flarestorm.buzz": ["104.20.0.2"],
}


async def _fake_resolve(host):
    if host in _DNS:
        return _DNS[host]
    raise OSError(f"no fake DNS entry for {host}")


MASTER = "https://cdn.mewstream.buzz/a/b/master.m3u8"
SEG = "https://x9.flarestorm.buzz/a/b/seg0.ts"
SEG_BYTES = bytes([0x47, 0x40, 0x00, 0x10]) + b"\x00" * 512


class _ProxyPage:
    """evaluate() mimics the in-page fetch JS contract:
    "status|content-type|final-url|base64(body)". A body of the literal
    "__TOO_LARGE__" sentinel (4th field, un-base64ed) triggers the over-cap
    RecipeError path in _in_page_fetch."""

    url = "https://megaplay.buzz/stream/s-2/122211/sub"

    def __init__(self, responses):
        self._responses = responses

    async def evaluate(self, js, url):
        st, ct, raw = self._responses[url]
        if raw == metrics_too_large_sentinel():
            # un-base64ed sentinel: _in_page_fetch sees b64 == "__TOO_LARGE__"
            return f"{st}|{ct}|{url}|{raw}"
        return f"{st}|{ct}|{url}|{base64.b64encode(raw).decode()}"

    async def close(self):
        pass


def metrics_too_large_sentinel():
    # Engine's private sentinel; mirror it here so the fake can emit it.
    from app.engine import _TOO_LARGE
    return _TOO_LARGE


def _engine_with_session(responses):
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
    eng._resolve_host = _fake_resolve
    prof = eng.profiles.lease()
    page = _ProxyPage(responses)
    sess = Session(
        id="sid1", profile=prof, proxy_id="direct", referer="https://megaplay.buzz/",
        user_agent="UA", cdn_host="cdn.mewstream.buzz", master_url=MASTER,
        expires_at=time.time() + 600, page=page, player_url=page.url,
    )
    eng._sessions["sid1"] = sess
    return eng, sess


def _hist_count(hist, **labels):
    """Read a labelled Histogram's _count sample via collect()."""
    total = 0.0
    for fam in hist.collect():
        for s in fam.samples:
            if s.name.endswith("_count") and all(
                s.labels.get(k) == v for k, v in labels.items()
            ):
                total += s.value
    return total


def _counter_val(counter, **labels):
    try:
        return counter.labels(**labels)._value.get()
    except Exception:  # noqa: BLE001
        return 0.0


class TestProxyFetchMetrics(unittest.TestCase):
    def test_duration_observed_on_success(self):
        eng, _ = _engine_with_session({SEG: (200, "image/jpeg", SEG_BYTES)})
        before = _hist_count(metrics.STEALTH_PROXY_FETCH_DURATION, result="ok")
        run(eng.proxy_fetch("sid1", SEG))
        after = _hist_count(metrics.STEALTH_PROXY_FETCH_DURATION, result="ok")
        self.assertEqual(after - before, 1.0)

    def test_total_ok_incremented_on_success(self):
        eng, _ = _engine_with_session({SEG: (200, "image/jpeg", SEG_BYTES)})
        before = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="ok")
        run(eng.proxy_fetch("sid1", SEG))
        after = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="ok")
        self.assertEqual(after - before, 1.0)

    def test_too_large_counted(self):
        eng, _ = _engine_with_session(
            {SEG: (200, "image/jpeg", metrics_too_large_sentinel())}
        )
        before = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="too_large")
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", SEG))
        after = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="too_large")
        self.assertEqual(after - before, 1.0)

    def test_host_denied_counted(self):
        # A private/loopback target is refused before the fetch — host_denied.
        eng, _ = _engine_with_session({SEG: (200, "image/jpeg", SEG_BYTES)})
        before = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="host_denied")
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", "http://127.0.0.1:6379/x"))
        after = _counter_val(metrics.STEALTH_PROXY_FETCH_TOTAL, result="host_denied")
        self.assertEqual(after - before, 1.0)


if __name__ == "__main__":
    unittest.main()
