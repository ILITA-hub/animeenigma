"""Graceful-degradation Phase 3 — sidecar shedding gates."""
import asyncio
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, DegradedShed
from app.recipes.base import RecipeError


def run(coro):
    return asyncio.run(coro)


def _engine(level: int) -> CamoufoxEngine:
    cfg = Config(pool_size=2, warming_enabled=True, governor_url="")
    eng = CamoufoxEngine(cfg)
    eng._sample_ram = lambda: 0  # RAM never the limiter in these tests
    eng._degradation_level = level
    return eng


class TestDegradedShedException(unittest.TestCase):
    def test_is_recipe_error_with_kind(self):
        exc = DegradedShed("critical")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "degraded")


class TestWarmingGate(unittest.TestCase):
    def test_level0_allows_warming(self):
        self.assertTrue(_engine(0)._warming_allowed())

    def test_level1_stops_warming(self):
        self.assertFalse(_engine(1)._warming_allowed())

    def test_level2_stops_warming(self):
        self.assertFalse(_engine(2)._warming_allowed())


class TestNewWorkGate(unittest.TestCase):
    def test_level0_and_1_admit(self):
        _engine(0)._shed_new_work()
        _engine(1)._shed_new_work()  # Elevated only stops warming, not work

    def test_level2_refuses_resolve(self):
        eng = _engine(2)
        with self.assertRaises(DegradedShed):
            run(eng.resolve("gogoanime", {}))

    def test_level2_refuses_browser_fetch(self):
        eng = _engine(2)
        with self.assertRaises(DegradedShed):
            run(eng.browser_fetch("gogoanime", "https://example.com/x"))

    def test_unknown_provider_still_wins_over_shed(self):
        # The unknown-provider check precedes the gate in resolve(): a config
        # error should not be masked as "degraded".
        eng = _engine(2)
        with self.assertRaises(RecipeError) as ctx:
            run(eng.resolve("nope", {}))
        self.assertNotIsInstance(ctx.exception, DegradedShed)


class TestFailOpenDefault(unittest.TestCase):
    def test_engine_boots_at_level_zero(self):
        eng = _engine(0)
        self.assertEqual(eng._degradation_level, 0)
        eng._shed_new_work()  # no raise


if __name__ == "__main__":
    unittest.main()
