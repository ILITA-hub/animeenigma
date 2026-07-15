"""Engine retry/self-heal tests — a browser crash must recycle the browser and
retry on the SAME exit (not burn the only proxy and give up after one attempt).

These stub out the actual Camoufox launch so no browser/runtime is needed.
"""

import asyncio
import tempfile
import unittest

from app.config import Config
from app.engine import CamoufoxEngine
from app.recipes.base import ChallengeError, Recipe, RecipeError


class _FakePage:
    url = "https://megaplay.buzz/stream/s-2/122211/sub"

    async def close(self):
        pass


class _FakeContext:
    async def new_page(self):
        return _FakePage()


class _CrashThenOkRecipe(Recipe):
    """Raises a driver-style crash ``fail_times`` times, then resolves."""

    name = "fake"
    allowed_hosts = {"x"}

    def __init__(self, fail_times: int, exc: Exception):
        self.fail_times = fail_times
        self.exc = exc
        self.calls = 0

    async def resolve(self, rc):
        self.calls += 1
        if self.calls <= self.fail_times:
            raise self.exc
        return {"master_url": "https://s2.cinewave2.site/a/b/master.m3u8",
                "referer": "https://megaplay.buzz/", "subtitles": []}


def _engine(recipe) -> CamoufoxEngine:
    profile_tmp = tempfile.TemporaryDirectory()
    cfg = Config(
        pool_size=1,
        max_proxy_retries=2,
        warming_enabled=False,
        profile_dir=profile_tmp.name,
    )
    eng = CamoufoxEngine(cfg)
    eng._test_profile_tmp = profile_tmp
    eng._recipes = {"fake": recipe}

    async def _fake_ensure(profile, proxy_id):
        profile.proxy_id = proxy_id
        profile.context = _FakeContext()  # → profile.launched becomes True
        profile.user_agent = "UA"
        return profile.context

    async def _fake_teardown(profile, *, reason):
        eng.profiles.reset_handles(profile)

    eng._ensure_browser = _fake_ensure
    eng._teardown = _fake_teardown
    return eng


def run(coro):
    return asyncio.run(coro)


class TestEngineSelfHeal(unittest.TestCase):
    def test_crash_retries_same_exit_then_succeeds(self):
        # The only exit is "direct"; two driver crashes must NOT exhaust it.
        recipe = _CrashThenOkRecipe(
            2, RuntimeError("Connection closed while reading from the driver")
        )
        eng = _engine(recipe)
        self.addCleanup(eng._test_profile_tmp.cleanup)
        payload = run(eng.resolve("fake", {}))
        self.assertEqual(payload["master_url"], "https://s2.cinewave2.site/a/b/master.m3u8")
        self.assertEqual(recipe.calls, 3)  # crash, crash, success — same exit

    def test_persistent_crash_raises_recipe_error_not_challenge(self):
        # All attempts crash → 5xx to the Go side, but as a RecipeError (not a
        # bogus ChallengeError, since no challenge ever occurred).
        recipe = _CrashThenOkRecipe(99, RuntimeError("driver dead"))
        eng = _engine(recipe)
        self.addCleanup(eng._test_profile_tmp.cleanup)
        with self.assertRaises(RecipeError):
            run(eng.resolve("fake", {}))
        self.assertEqual(recipe.calls, 3)  # max_proxy_retries + 1 attempts

    def test_real_challenge_still_rotates_and_exhausts(self):
        # A genuine ChallengeError still marks the exit tried; with one exit it
        # exhausts after the first challenge.
        recipe = _CrashThenOkRecipe(99, ChallengeError("blocked", host="cdn", kind="player"))
        eng = _engine(recipe)
        self.addCleanup(eng._test_profile_tmp.cleanup)
        with self.assertRaises(ChallengeError):
            run(eng.resolve("fake", {}))
        self.assertEqual(recipe.calls, 1)  # exit excluded after 1st challenge


if __name__ == "__main__":
    unittest.main()
