"""Concurrency: _warm_fetch_session's get-or-create on self._sessions[key] must
serialize concurrent callers for the SAME (provider, origin) key. Without a
lock, two concurrent warm-fetch calls for the same key both miss the cache,
both lease a DISTINCT profile, and the loser's profile is silently orphaned
when the winner's session overwrites the shared dict slot afterward — draining
the shared Camoufox pool one slot at a time with no session, no crash flag,
and no admission-gate counter to explain it (the "N of 4 profiles leased,
unaccounted for" signature recurring in production, e.g. animepahe 2026-07-10).
"""
import asyncio
import unittest

from app.config import Config
from app.engine import CamoufoxEngine


def run(coro):
    return asyncio.run(coro)


class _Resp:
    def __init__(self, status):
        self.status = status


class _OKPage:
    url = "https://9anime.me.uk/"

    async def goto(self, *a, **k):
        return _Resp(200)

    async def title(self):
        return "9anime - watch anime online"

    async def close(self):
        pass


class _OKCtx:
    def __init__(self, page):
        self._page = page

    async def new_page(self):
        return self._page


class TestWarmFetchSessionConcurrency(unittest.TestCase):
    def test_concurrent_calls_for_same_key_do_not_leak_a_profile(self):
        async def _slow_ensure(profile, proxy_id):
            # Force a real event-loop yield so two concurrent callers actually
            # interleave here (mirrors the real cold-browser-launch await) —
            # both must pass the "no existing session yet" check before either
            # finishes and stores its session.
            await asyncio.sleep(0.05)
            return _OKCtx(_OKPage())

        eng = CamoufoxEngine(Config(pool_size=4, warming_enabled=False))
        eng._ensure_browser = _slow_ensure

        async def _both():
            await asyncio.gather(
                eng._warm_fetch_session("nineanime", "https://9anime.me.uk"),
                eng._warm_fetch_session("nineanime", "https://9anime.me.uk"),
            )

        run(_both())

        leased = [p for p in eng.profiles.all() if p.leased]
        self.assertEqual(
            len(leased), 1,
            "two concurrent warm-fetches for the SAME key must not strand a "
            "second leased profile — the loser must reuse the winner's "
            "session (or its own lease must be released), not get silently "
            "orphaned when the dict slot is overwritten",
        )
        self.assertEqual(len(eng._sessions), 1)


if __name__ == "__main__":
    unittest.main()
