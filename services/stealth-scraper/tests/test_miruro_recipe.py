"""miruro recipe: registered, challenge-solving, host-locked.

miruro is discovery-only (Go builds/decodes the secure-pipe envelope); the recipe
only carries the solve_challenge flag + the SSRF allowlist, so these are the
invariants that matter."""
import unittest

from app.config import Config
from app.engine import CamoufoxEngine
from app.recipes.base import host_allowed
from app.recipes.miruro import MiruroRecipe


class TestMiruroRecipe(unittest.TestCase):
    def test_registered_in_engine(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        self.assertIn("miruro", eng._recipes)
        self.assertIsInstance(eng._recipes["miruro"], MiruroRecipe)

    def test_solves_challenge(self):
        # www.miruro.tv is Cloudflare-gated — the recipe MUST opt into the solver
        # (else browser_fetch rotates the exit instead of clicking the Turnstile).
        self.assertTrue(MiruroRecipe().solve_challenge)

    def test_host_allowlist_locked(self):
        r = MiruroRecipe()
        self.assertTrue(host_allowed("www.miruro.tv", r.allowed_hosts))
        self.assertTrue(host_allowed("miruro.tv", r.allowed_hosts))
        # SSRF guard: unrelated / internal hosts are refused.
        self.assertFalse(host_allowed("pro.ultracloud.cc", r.allowed_hosts))
        self.assertFalse(host_allowed("169.254.169.254", r.allowed_hosts))
        self.assertFalse(host_allowed("evil.example.com", r.allowed_hosts))


if __name__ == "__main__":
    unittest.main()
