"""Config parsing for the RAM budget + per-user quota knobs (Phase 2)."""
import unittest

from app.config import Config


class TestCapacityConfig(unittest.TestCase):
    def test_defaults(self):
        c = Config()
        self.assertEqual(c.ram_soft_bytes, 4_294_967_296)
        self.assertEqual(c.ram_hard_bytes, 6_442_450_944)
        self.assertEqual(c.ram_sample_seconds, 5.0)
        self.assertEqual(c.user_quota, 2)

    def test_from_env_overrides(self):
        c = Config.from_env({
            "STEALTH_RAM_SOFT_BYTES": "1000",
            "STEALTH_RAM_HARD_BYTES": "2000",
            "STEALTH_RAM_SAMPLE_SECONDS": "3",
            "STEALTH_USER_QUOTA": "5",
        })
        self.assertEqual(c.ram_soft_bytes, 1000)
        self.assertEqual(c.ram_hard_bytes, 2000)
        self.assertEqual(c.ram_sample_seconds, 3.0)
        self.assertEqual(c.user_quota, 5)

    def test_from_env_bad_values_fall_back_to_defaults(self):
        c = Config.from_env({"STEALTH_RAM_HARD_BYTES": "notint", "STEALTH_USER_QUOTA": ""})
        self.assertEqual(c.ram_hard_bytes, 6_442_450_944)
        self.assertEqual(c.user_quota, 2)


if __name__ == "__main__":
    unittest.main()
