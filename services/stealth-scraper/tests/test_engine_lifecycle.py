"""Lifecycle & resilience tests for the engine: pool-exhaustion 503, eviction
in-use guard (no use-after-close), TTL slide-before-fetch, in-page fetch timeout
reclaiming the wedged slot, body-size cap, unactivated-session grace, and the
profile-retirement bookkeeping."""

import asyncio
import base64
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, FetchTimeout, PoolExhausted, Session
from app.profiles import ProfileManager
from app.recipes.base import RecipeError


def run(coro):
    return asyncio.run(coro)


async def _resolve_public(host):
    return ["104.20.0.1"]  # any host resolves to a public IP for these tests


class _Page:
    url = "https://megaplay.buzz/stream/x"

    def __init__(self, *, result=None, sleep=0.0):
        self._result = result
        self._sleep = sleep
        self.closed = False

    async def evaluate(self, js, url):
        if self._sleep:
            await asyncio.sleep(self._sleep)
        if self._result is not None:
            return self._result
        body = bytes([0x47, 0x40, 0x00, 0x10]) + b"seg"
        return f"200|video/mp2t|{url}|{base64.b64encode(body).decode()}"

    async def close(self):
        self.closed = True


def _engine_session(page, *, ttl=600, fetch_timeout_ms=20_000, grace=45):
    cfg = Config(
        pool_size=1, warming_enabled=False, session_ttl_seconds=ttl,
        fetch_timeout_ms=fetch_timeout_ms, unactivated_grace_seconds=grace,
    )
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    prof = eng.profiles.lease()
    sess = Session(
        id="sid1", profile=prof, proxy_id="direct", referer="https://megaplay.buzz/",
        user_agent="UA", cdn_host="cdn.mewstream.buzz",
        master_url="https://cdn.mewstream.buzz/m.m3u8",
        expires_at=time.time() + ttl, page=page, player_url=page.url,
    )
    eng._sessions["sid1"] = sess
    return eng, sess


class TestPoolExhaustion(unittest.TestCase):
    def test_resolve_raises_pool_exhausted(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

        async def _none(*a, **k):
            return None

        eng._acquire_profile = _none  # simulate every profile leased
        with self.assertRaises(PoolExhausted):
            run(eng.resolve("gogoanime", {}))


class TestEvictionInUseGuard(unittest.TestCase):
    def test_in_use_session_not_evicted(self):
        page = _Page()
        eng, sess = _engine_session(page)
        sess.expires_at = time.time() - 1  # expired

        async def _evict():
            eng._evict_expired()

        sess.in_use = 1
        run(_evict())
        self.assertIn("sid1", eng._sessions, "in-use session must survive eviction")

        sess.in_use = 0
        run(_evict())
        self.assertNotIn("sid1", eng._sessions, "idle expired session must be evicted")
        self.assertFalse(sess.profile.leased, "evicted session's profile must be released")


class TestProxyFetchLifecycle(unittest.TestCase):
    def test_ttl_slid_before_fetch(self):
        page = _Page()
        eng, sess = _engine_session(page, ttl=600)
        sess.expires_at = time.time() + 2  # about to expire
        run(eng.proxy_fetch("sid1", "https://cdn.mewstream.buzz/m.m3u8"))
        self.assertGreater(sess.expires_at, time.time() + 300, "TTL must be slid on activity")
        self.assertEqual(sess.in_use, 0, "in_use refcount must return to 0")

    def test_fetch_timeout_reclaims_slot(self):
        page = _Page(sleep=0.2)  # evaluate hangs past the timeout
        eng, sess = _engine_session(page, fetch_timeout_ms=20)
        with self.assertRaises(FetchTimeout):
            run(eng.proxy_fetch("sid1", "https://cdn.mewstream.buzz/m.m3u8"))
        self.assertNotIn("sid1", eng._sessions, "timed-out session must be torn down")
        self.assertFalse(sess.profile.leased, "wedged browser slot must be reclaimed")

    def test_body_cap_rejected(self):
        page = _Page(result="200|video/mp2t|https://cdn.mewstream.buzz/x|__TOO_LARGE__")
        eng, _ = _engine_session(page)
        with self.assertRaises(RecipeError):
            run(eng.proxy_fetch("sid1", "https://cdn.mewstream.buzz/m.m3u8"))


class TestUnactivatedGrace(unittest.TestCase):
    def test_open_session_starts_on_short_grace(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False, unactivated_grace_seconds=45))
        prof = eng.profiles.lease()
        page = _Page()
        sess = run(eng._open_session(
            {"master_url": "https://cdn.mewstream.buzz/m.m3u8", "referer": "r"},
            None, "direct", prof, page,
        ))
        # Resolved-but-unfetched session expires on the short grace, not the TTL.
        self.assertLessEqual(sess.expires_at, time.time() + 60)


class TestProfileRetirement(unittest.TestCase):
    def test_uses_survive_teardown_and_retire(self):
        pm = ProfileManager("/tmp/ss-profiles-test", size=1, max_uses=2)
        p = pm.lease()
        pm.release(p, ok=True)            # uses = 1
        pm.reset_handles(p)               # MUST NOT zero uses (the old bug)
        self.assertEqual(p.uses, 1)
        self.assertFalse(pm.needs_retire(p))
        p2 = pm.lease()
        self.assertIs(p2, p)
        pm.release(p, ok=True)            # uses = 2
        self.assertTrue(pm.needs_retire(p), "profile must retire at max_uses")
        pm.reset_uses(p)
        self.assertEqual(p.uses, 0)
        self.assertFalse(pm.needs_retire(p))


class TestSelfHealConfig(unittest.TestCase):
    def test_defaults(self):
        cfg = Config()
        self.assertEqual(cfg.poison_max, 2)
        self.assertEqual(cfg.readyz_saturation_seconds, 15.0)
        self.assertEqual(cfg.resurrect_backoff_base_seconds, 1.0)
        self.assertEqual(cfg.resurrect_backoff_cap_seconds, 30.0)
        self.assertEqual(cfg.resurrect_max_fails, 3)

    def test_env_overrides(self):
        cfg = Config.from_env({
            "STEALTH_POISON_MAX": "4",
            "STEALTH_READYZ_SATURATION_SECONDS": "30",
            "STEALTH_RESURRECT_BACKOFF_BASE_SECONDS": "2",
            "STEALTH_RESURRECT_BACKOFF_CAP_SECONDS": "60",
            "STEALTH_RESURRECT_MAX_FAILS": "5",
        })
        self.assertEqual(cfg.poison_max, 4)
        self.assertEqual(cfg.readyz_saturation_seconds, 30.0)
        self.assertEqual(cfg.resurrect_backoff_base_seconds, 2.0)
        self.assertEqual(cfg.resurrect_backoff_cap_seconds, 60.0)
        self.assertEqual(cfg.resurrect_max_fails, 5)


class TestSelfHealMetrics(unittest.TestCase):
    def test_metrics_exist(self):
        from app import metrics
        # Gauges accept .set(); the resurrect counter is labelled by result.
        metrics.POOL_FREE.set(3)
        metrics.POOL_CRASHED.set(1)
        metrics.SLOT_RESURRECT_TOTAL.labels(result="ok").inc()
        metrics.SLOT_RESURRECT_TOTAL.labels(result="fail").inc()
        names = {m.name for m in metrics.SLOT_RESURRECT_TOTAL.collect()}
        self.assertIn("stealth_slot_resurrect", names)


if __name__ == "__main__":
    unittest.main()
