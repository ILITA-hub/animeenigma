"""Graceful-degradation Phase 3 — sidecar shedding gates."""
import asyncio
import contextlib
import json
import unittest
from unittest import mock

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


def _fake_response(payload: dict):
    data = json.dumps(payload).encode()

    class _FakeResp:
        def __enter__(self):
            return self

        def __exit__(self, *exc):
            return False

        def read(self, _n=-1):
            return data

    return _FakeResp()


def _run_one_poll_tick(eng: CamoufoxEngine) -> None:
    """Start _degradation_loop, let it run (at least) one iteration, then
    cancel it — degradation_poll_seconds is pinned to 0.0 by callers so the
    first (and only observed) iteration fires almost immediately."""

    async def _drive():
        task = asyncio.ensure_future(eng._degradation_loop())
        await asyncio.sleep(0.05)
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task

    asyncio.run(_drive())


class TestDegradedShedException(unittest.TestCase):
    def test_is_recipe_error_with_kind(self):
        exc = DegradedShed("critical")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "degraded")


class TestWarmingGate(unittest.TestCase):
    def test_level0_allows_warming(self):
        self.assertTrue(_engine(0)._warming_allowed())

    def test_level_alone_no_longer_gates_warming(self):
        # spec 2026-07-21: the binary level>=1 stop was replaced by the
        # graduated pool_target() curve — level by itself (score still at its
        # fail-open 0.0 default, no sessions leased) no longer blocks warming.
        # See TestPoolTargetGate below for the curve-driven replacement.
        self.assertTrue(_engine(1)._warming_allowed())
        self.assertTrue(_engine(2)._warming_allowed())


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


class TestPollWiresScore(unittest.TestCase):
    """Graduated degradation (spec 2026-07-21): the poller now reads
    data.score alongside data.level and feeds it into _pool_target()."""

    def _engine(self) -> CamoufoxEngine:
        cfg = Config(
            pool_size=6,
            warming_enabled=True,
            governor_url="http://fake-governor",
            degradation_poll_seconds=0.0,
            pool_curve="0.40:6,0.60:2,0.80:1",
        )
        return CamoufoxEngine(cfg)

    def test_score_sets_degradation_score_and_pool_target(self):
        eng = self._engine()
        payload = {"data": {"level": 1, "score": 0.55}}
        with mock.patch("urllib.request.urlopen", return_value=_fake_response(payload)):
            _run_one_poll_tick(eng)
        self.assertEqual(eng._degradation_score, 0.55)
        # curve 6->2 over 0.40-0.60: floor(6 - 4*0.75) == 3
        self.assertEqual(eng._pool_target(), 3)

    def test_poll_error_zeroes_score(self):
        eng = self._engine()
        eng._degradation_score = 0.9  # nonzero baseline to prove it gets reset
        with mock.patch("urllib.request.urlopen", side_effect=OSError("boom")):
            _run_one_poll_tick(eng)
        self.assertEqual(eng._degradation_score, 0.0)


class TestPoolTargetGate(unittest.TestCase):
    """_warming_allowed() now stops at the graduated pool_target(), not a
    binary level check (see TestWarmingGate.test_level_alone_no_longer_gates_warming)."""

    def _engine(self, score: float) -> CamoufoxEngine:
        cfg = Config(
            pool_size=6, warming_enabled=True, governor_url="",
            pool_curve="0.40:6,0.60:2,0.80:1",
        )
        eng = CamoufoxEngine(cfg)
        eng._sample_ram = lambda: 0  # RAM never the limiter in these tests
        eng._degradation_score = score
        return eng

    def test_warming_blocked_at_pool_target(self):
        eng = self._engine(0.55)
        target = eng._pool_target()
        self.assertEqual(target, 3)
        for i in range(target):
            eng._sessions[str(i)] = object()  # dummy occupants; only len() matters
        self.assertFalse(eng._warming_allowed())

    def test_warming_allowed_below_pool_target(self):
        eng = self._engine(0.55)
        target = eng._pool_target()
        for i in range(target - 1):
            eng._sessions[str(i)] = object()
        self.assertTrue(eng._warming_allowed())


if __name__ == "__main__":
    unittest.main()
