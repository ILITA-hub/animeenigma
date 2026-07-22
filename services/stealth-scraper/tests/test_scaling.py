import json
import unittest
from pathlib import Path

from app import scaling

# Shared cross-language parity fixture (repo-root test/fixtures). Resolved
# relative to this file so it works from any working directory; absent in a
# stripped per-service container (tests run against the full checkout in CI).
_PARITY_FIXTURE = (
    Path(__file__).resolve().parents[3] / "test" / "fixtures" / "degradation_curve_parity.json"
)


class PoolTargetTest(unittest.TestCase):
    CURVE = scaling.parse_curve("0.40:6,0.60:2,0.80:1")

    def test_bands(self):
        cases = [
            (0.0, 6), (0.40, 6), (0.41, 5), (0.50, 4),
            (0.60, 2), (0.70, 1), (0.80, 1), (1.0, 1),
        ]
        for score, want in cases:
            self.assertEqual(
                scaling.pool_target_for(score, self.CURVE, 6), want, f"score={score}"
            )

    def test_floor_is_one_even_for_zero_cap_curve(self):
        curve = scaling.parse_curve("0.40:6,0.80:0")
        self.assertEqual(scaling.pool_target_for(1.0, curve, 6), 1)

    def test_max_pool_clamps(self):
        self.assertEqual(scaling.pool_target_for(0.0, self.CURVE, 3), 3)

    def test_garbage_curve_falls_back(self):
        for bad in ("", "junk", "0.6:2,0.4:6", "0.4:-1"):
            curve = scaling.parse_curve(bad)
            self.assertEqual(scaling.pool_target_for(0.0, curve, 6), 6)
            self.assertEqual(scaling.pool_target_for(1.0, curve, 6), 1)

    def test_curve_parity_fixture(self):
        # Asserts the Python curve algorithm against the SAME shared fixture the
        # Go side asserts (services/content-verify/internal/service/
        # curve_parity_test.go::TestCurve_ParityFixture) — so a divergent edit to
        # either piecewise-linear-floor(+1e-9 epsilon) implementation is caught in
        # both suites. Cases sit within [1, max_pool] where Go and Python agree
        # (the service-local clamps differ and are covered by the tests above).
        if not _PARITY_FIXTURE.is_file():
            self.skipTest(f"shared parity fixture not present at {_PARITY_FIXTURE}")
        fx = json.loads(_PARITY_FIXTURE.read_text())
        curve = [(p[0], p[1]) for p in fx["curve"]]
        max_pool = fx["max_pool"]
        self.assertTrue(fx["cases"])
        for case in fx["cases"]:
            score, want = case["score"], case["cap"]
            self.assertEqual(
                scaling.pool_target_for(score, curve, max_pool), want, f"score={score}"
            )
