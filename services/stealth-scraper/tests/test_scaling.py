import unittest

from app import scaling


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
