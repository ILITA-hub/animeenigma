"""WARP-exit pinning for the solve_challenge providers (miruro + animepahe).

Cloudflare's silent __cf_chl_rt_tk managed challenge on www.miruro.tv /
animepahe.pw is unpassable from our datacenter IP but clears cleanly through a
Cloudflare WARP exit (proven live 2026-07-20). The fix pins those two providers
to the `warp` pool exit. These tests lock the wiring:

  1. the recipes declare preferred_proxy_type == 'warp' (base default None so
     every other provider keeps unchanged direct-first selection);
  2. browser_fetch forwards the recipe's preferred_proxy_type to pool.select;
  3. the pool FAIL-OPENS to direct when no warp exit is configured — an unset
     STEALTH_WARP_PROXY_URL degrades to today's behavior, never a hard failure.
"""
import unittest

from app.config import Config
from app.engine import CamoufoxEngine
from app.recipes.animepahe import AnimePaheRecipe
from app.recipes.base import Recipe, RecipeError
from app.recipes.miruro import MiruroRecipe
from app.tunnels import ProxyEntry, ProxyPool


def run(coro):
    import asyncio

    return asyncio.run(coro)


class TestRecipePreferredProxy(unittest.TestCase):
    def test_base_default_is_none(self):
        # Non-challenge providers must keep direct-first selection unchanged.
        self.assertIsNone(Recipe().preferred_proxy_type)

    def test_miruro_prefers_warp(self):
        self.assertEqual(MiruroRecipe().preferred_proxy_type, "warp")

    def test_animepahe_prefers_warp(self):
        self.assertEqual(AnimePaheRecipe().preferred_proxy_type, "warp")


class TestPoolFailOpen(unittest.TestCase):
    def test_warp_chosen_when_present(self):
        pool = ProxyPool(
            [
                ProxyEntry("direct", "direct"),
                ProxyEntry("warp", "warp", "socks5://warp-proxy:1080"),
            ]
        )
        self.assertEqual(pool.select(preferred_type="warp").id, "warp")

    def test_fail_open_to_direct_when_warp_absent(self):
        # STEALTH_WARP_PROXY_URL unset => no warp exit => degrade to direct,
        # never a hard failure (providers stay as broken as today, not worse).
        pool = ProxyPool([ProxyEntry("direct", "direct")])
        chosen = pool.select(preferred_type="warp")
        self.assertIsNotNone(chosen)
        self.assertEqual(chosen.type, "direct")


class TestStickyRespectsPreferredType(unittest.TestCase):
    """A sticky binding must not defeat an explicit preferred_type. A profile
    bound to `direct` (by warming or a non-pinned provider) would otherwise pin a
    warp-preferring provider (miruro/animepahe) to direct forever, so they'd
    never actually reach the WARP exit. The pinned select must re-bind to warp."""

    @staticmethod
    def _pool():
        return ProxyPool(
            [
                ProxyEntry("direct", "direct"),
                ProxyEntry("warp", "warp", "socks5://warp-proxy:1080"),
            ]
        )

    def test_preferred_type_overrides_mismatched_sticky(self):
        p = self._pool()
        self.assertEqual(p.select(sticky_key="prof").id, "direct")  # bound to direct
        # A warp-preferring fetch on the SAME profile must re-pin to warp.
        self.assertEqual(
            p.select(sticky_key="prof", preferred_type="warp").id, "warp"
        )
        # ...and the re-pin sticks (subsequent warp fetches stay on warp).
        self.assertEqual(
            p.select(sticky_key="prof", preferred_type="warp").id, "warp"
        )

    def test_matching_sticky_is_kept(self):
        p = self._pool()
        p.select(sticky_key="prof", preferred_type="warp")  # bind to warp
        self.assertEqual(
            p.select(sticky_key="prof", preferred_type="warp").id, "warp"
        )

    def test_sticky_kept_when_no_preference(self):
        # Non-pinned callers keep exit stability (CDN affinity) unchanged.
        p = self._pool()
        self.assertEqual(p.select(sticky_key="prof").id, "direct")
        self.assertEqual(p.select(sticky_key="prof").id, "direct")


class TestBrowserFetchPinsWarp(unittest.TestCase):
    def test_browser_fetch_forwards_recipe_preferred_type(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        captured = {}

        async def _fake_acquire():
            return eng.profiles.lease()

        def _spy_select(*args, **kwargs):
            captured.update(kwargs)
            return None  # => browser_fetch raises RecipeError right after select

        eng._acquire_profile = _fake_acquire
        eng.pool.select = _spy_select

        with self.assertRaises(RecipeError):
            run(
                eng.browser_fetch(
                    "miruro", "https://www.miruro.tv/api/secure/pipe?e=x"
                )
            )
        self.assertEqual(captured.get("preferred_type"), "warp")


if __name__ == "__main__":
    unittest.main()
