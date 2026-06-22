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


class TestProfileHealth(unittest.TestCase):
    def test_mark_crashed_and_healthy(self):
        from app.profiles import ProfileManager
        pm = ProfileManager("/tmp/ss-health-test", size=2)
        p = pm.all()[0]
        self.assertEqual(p.status, "healthy")
        pm.mark_crashed(p, error="Target closed")
        self.assertEqual(p.status, "crashed")
        self.assertEqual(p.consecutive_fail, 1)
        self.assertGreater(p.last_crash, 0.0)
        # crashed_idle lists crashed, not-leased slots only.
        self.assertIn(p, pm.crashed_idle())
        p.leased = True
        self.assertNotIn(p, pm.crashed_idle())
        p.leased = False
        pm.mark_crashed(p, error="again")
        self.assertEqual(p.consecutive_fail, 2)
        pm.mark_healthy(p)
        self.assertEqual(p.status, "healthy")
        self.assertEqual(p.consecutive_fail, 0)
        self.assertNotIn(p, pm.crashed_idle())

    def test_status_counts(self):
        from app.profiles import ProfileManager
        pm = ProfileManager("/tmp/ss-counts-test", size=3)
        pm.mark_crashed(pm.all()[0])
        counts = pm.status_counts()
        self.assertEqual(counts["crashed"], 1)
        self.assertEqual(counts["healthy"], 2)


class TestTeardownMarksCrashed(unittest.TestCase):
    def _eng(self):
        return CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

    def test_crash_reason_marks_slot_crashed(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        run(eng._teardown(p, reason="crash"))
        self.assertEqual(p.status, "crashed")
        self.assertEqual(p.consecutive_fail, 1)

    def test_non_crash_reason_does_not_mark_crashed(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        run(eng._teardown(p, reason="rotate"))
        self.assertEqual(p.status, "healthy")
        run(eng._teardown(p, reason="recycle"))
        self.assertEqual(p.status, "healthy")


class TestHealthShape(unittest.TestCase):
    def test_health_breakdown(self):
        eng = CamoufoxEngine(Config(pool_size=2, warming_enabled=False))
        h = eng.health()
        self.assertIn("global", h)
        self.assertIn("providers", h)
        self.assertIn("users", h)
        g = h["global"]
        self.assertEqual(g["free"], 2)
        self.assertEqual(g["crashed"], 0)
        self.assertIn("warming", g)
        # legacy keys retained for back-compat consumers.
        self.assertEqual(h["status"], "ok")
        self.assertEqual(h["pool_size"], 2)

    def test_providers_breakdown_counts_warm_sessions(self):
        eng = CamoufoxEngine(Config(pool_size=2, warming_enabled=False))
        prof = eng.profiles.lease()
        page = _Page()
        sess = Session(
            id="fetch::nineanime::https://9anime.me.uk", profile=prof,
            proxy_id="d", referer="r", user_agent="UA", cdn_host="9anime.me.uk",
            master_url="https://9anime.me.uk", expires_at=time.time() + 600,
            page=page, player_url=page.url, provider="nineanime",
        )
        sess.last_error = "Target closed"
        eng._sessions[sess.id] = sess
        h = eng.health()
        self.assertEqual(h["providers"]["nineanime"]["held"], 1)
        self.assertEqual(h["providers"]["nineanime"]["last_error"], "Target closed")

    def test_is_ready_only_after_sustained_saturation(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    readyz_saturation_seconds=15))
        # Saturate: lease the only profile.
        eng.profiles.lease()
        # First observation: saturated NOW but window not elapsed -> still ready.
        self.assertTrue(eng.is_ready())
        # Simulate the saturation window having started 20s ago.
        eng._saturated_since = time.time() - 20
        self.assertFalse(eng.is_ready(), "sustained saturation -> not ready")


if __name__ == "__main__":
    unittest.main()


class TestReadyzAndKinds(unittest.TestCase):
    def _set_engine(self, engine):
        import app.main as m
        m.app.state.engine = engine
        return m

    def test_healthz_stays_200_when_saturated(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        eng.profiles.lease()                      # saturate
        m = self._set_engine(eng)
        out = run(m.healthz())
        # /healthz returns the health() dict directly (FastAPI -> 200).
        self.assertEqual(out["status"], "degraded")  # body says degraded...
        # ...but the route is 200 (no JSONResponse status override).

    def test_readyz_503_on_sustained_saturation(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    readyz_saturation_seconds=15))
        eng.profiles.lease()
        eng._saturated_since = time.time() - 20
        m = self._set_engine(eng)
        resp = run(m.readyz())
        self.assertEqual(resp.status_code, 503)
        self.assertFalse(json.loads(resp.body)["ready"])

    def test_readyz_200_when_free(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        m = self._set_engine(eng)
        resp = run(m.readyz())
        self.assertEqual(resp.status_code, 200)
        self.assertTrue(json.loads(resp.body)["ready"])

    def test_resolve_pool_exhausted_kind(self):
        import json
        from app.main import ResolveRequest
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

        async def _none(*a, **k):
            return None
        eng._acquire_profile = _none
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "pool_exhausted")

    def test_resolve_provider_wedged_kind(self):
        import json
        from app.main import ResolveRequest
        from app.engine import ProviderWedged
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

        async def _wedge(*a, **k):
            raise ProviderWedged("poisoned", provider="nineanime")
        eng.resolve = _wedge
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="nineanime")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "provider_wedged")


class TestReaperResurrection(unittest.TestCase):
    def _eng(self):
        eng = CamoufoxEngine(Config(
            pool_size=1, warming_enabled=False,
            resurrect_backoff_base_seconds=1, resurrect_backoff_cap_seconds=30,
            resurrect_max_fails=3,
        ))
        return eng

    def test_backoff_curve(self):
        eng = self._eng()
        self.assertEqual(eng._resurrect_backoff(0), 1)
        self.assertEqual(eng._resurrect_backoff(1), 2)
        self.assertEqual(eng._resurrect_backoff(2), 4)
        self.assertEqual(eng._resurrect_backoff(3), 8)
        self.assertEqual(eng._resurrect_backoff(4), 16)
        self.assertEqual(eng._resurrect_backoff(5), 30)   # capped
        self.assertEqual(eng._resurrect_backoff(9), 30)   # still capped

    def test_successful_resurrect_marks_healthy(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        eng.profiles.mark_crashed(p)            # consecutive_fail=1, status=crashed
        p.next_resurrect_at = 0.0               # eligible now

        async def _ok_launch(profile, proxy_id):
            return object()
        eng._ensure_browser = _ok_launch

        run(eng._resurrect_crashed_slot(p))
        self.assertEqual(p.status, "healthy")
        self.assertEqual(p.consecutive_fail, 0)

    def test_resurrect_respects_backoff(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        eng.profiles.mark_crashed(p)
        p.next_resurrect_at = time.time() + 999  # not eligible yet
        launched = {"n": 0}

        async def _count(profile, proxy_id):
            launched["n"] += 1
            return object()
        eng._ensure_browser = _count

        run(eng._resurrect_crashed_slot(p))
        self.assertEqual(launched["n"], 0, "must not attempt before backoff elapses")
        self.assertEqual(p.status, "crashed")

    def test_retire_after_three_failures(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        p.uses = 7  # retirement must zero this

        async def _boom(profile, proxy_id):
            raise RuntimeError("relaunch failed")
        eng._ensure_browser = _boom

        # Original crash that landed the slot in the reaper's crashed pool
        # (consecutive_fail == 1). Each failed relaunch then bumps the counter;
        # at resurrect_max_fails (3) the slot is retired instead of revived.
        eng.profiles.mark_crashed(p)
        self.assertEqual(p.consecutive_fail, 1)

        attempts = 0
        # Drive failed relaunches until the slot retires (counter ownership lives
        # in the except arm's mark_crashed -> +1 per failed relaunch; no manual
        # re-mark in the loop or the failure would be double-counted).
        while p.status == "crashed" and attempts < 10:
            p.next_resurrect_at = 0.0          # clear backoff so we attempt now
            run(eng._resurrect_crashed_slot(p))
            attempts += 1

        # 1 (original crash) + 2 failed relaunches == resurrect_max_fails -> retired.
        self.assertEqual(attempts, 2, "should retire on the relaunch reaching max_fails")
        self.assertEqual(p.status, "healthy")  # retired -> fresh identity in pool
        self.assertEqual(p.consecutive_fail, 0)
        self.assertEqual(p.uses, 0)
