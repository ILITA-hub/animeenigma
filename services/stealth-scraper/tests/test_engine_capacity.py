"""RAM admission + per-user quota (Phase 2)."""
import unittest

from app.engine import CapacityExceeded, UserQuotaExceeded, PoolExhausted
from app.recipes.base import RecipeError


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


if __name__ == "__main__":
    unittest.main()
