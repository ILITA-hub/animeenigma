"""NineAnimeRecipe is a thin megaplay subclass: it reuses gogoanime's player
interception and only narrows the navigation host allowlist to 9anime's family."""
import unittest

from app.engine import CamoufoxEngine
from app.config import Config
from app.recipes.gogoanime import GogoanimeRecipe
from app.recipes.nineanime import NineAnimeRecipe


class TestNineAnimeRecipe(unittest.TestCase):
    def test_name_and_inheritance(self):
        r = NineAnimeRecipe()
        self.assertEqual(r.name, "nineanime")
        self.assertIsInstance(r, GogoanimeRecipe)  # reuses resolve()/megaplay interception

    def test_allowed_hosts_cover_discovery_and_player(self):
        r = NineAnimeRecipe()
        for h in ("9anime.me.uk", "my.1anime.site", "1anime.site",
                  "megaplay.buzz", "vidwish.live"):
            self.assertIn(h, r.allowed_hosts, h)

    def test_registered_in_engine(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        self.assertIn("nineanime", eng._recipes)
        self.assertIsInstance(eng._recipes["nineanime"], NineAnimeRecipe)


if __name__ == "__main__":
    unittest.main()
