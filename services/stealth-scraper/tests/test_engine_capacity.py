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


if __name__ == "__main__":
    unittest.main()
