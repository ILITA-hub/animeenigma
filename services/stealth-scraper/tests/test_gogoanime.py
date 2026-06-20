"""Unit tests for the gogoanime recipe — pure helpers + the interception-based
async chain via a fake Playwright page (no browser/camoufox runtime needed)."""

import asyncio
import json
import unittest

from app.config import Config
from app.recipes.base import ChallengeError, RecipeContext, RecipeError, looks_like_challenge
from app.recipes.gogoanime import (
    GogoanimeRecipe,
    build_episode_url,
    build_search_url,
    parse_getsources,
    search_keywords,
)

REAL_GETSOURCES = json.dumps(
    {
        "sources": {"file": "https://s2.cinewave2.site/anime/aa/bb/master.m3u8"},
        "tracks": [
            {
                "file": "https://1oe.lostproject.club/anime/aa/bb/subtitles/eng-2.vtt",
                "label": "English",
                "kind": "captions",
                "default": True,
            }
        ],
        "intro": {"start": 0, "end": 130},
        "outro": {"start": 0, "end": 0},
    }
)


class TestPureHelpers(unittest.TestCase):
    def test_search_keywords(self):
        self.assertEqual(search_keywords("Frieren: Beyond Journey's End"), ["Frieren"])
        self.assertEqual(search_keywords("Re:Zero kara Hajimeru"), ["Re"])
        self.assertEqual(search_keywords("One Piece"), ["One Piece", "One"])
        self.assertEqual(search_keywords("  "), [])

    def test_url_builders(self):
        self.assertEqual(
            build_search_url("https://gogoanimes.fi/", "One Piece"),
            "https://gogoanimes.fi/search.html?keyword=One%20Piece",
        )
        self.assertEqual(
            build_episode_url("https://gogoanimes.fi", "one-piece", 1100),
            "https://gogoanimes.fi/one-piece-episode-1100",
        )
        self.assertEqual(
            build_episode_url("https://gogoanimes.fi", "naruto", 1, dub=True),
            "https://gogoanimes.fi/naruto-dub-episode-1",
        )

    def test_parse_getsources_object_shape(self):
        out = parse_getsources(json.loads(REAL_GETSOURCES))
        self.assertEqual(out["master_url"], "https://s2.cinewave2.site/anime/aa/bb/master.m3u8")
        self.assertEqual(len(out["subtitles"]), 1)
        self.assertTrue(out["subtitles"][0]["default"])
        self.assertEqual(out["intro"], {"start": 0, "end": 130})

    def test_parse_getsources_tolerates_list(self):
        out = parse_getsources({"sources": [{"file": "https://x/y.m3u8"}]})
        self.assertEqual(out["master_url"], "https://x/y.m3u8")

    def test_parse_getsources_rejects_relative(self):
        with self.assertRaises(RecipeError):
            parse_getsources({"sources": {"file": "/relative.m3u8"}})

    def test_challenge_detection(self):
        self.assertTrue(looks_like_challenge(403, "<html><title>Just a moment...</title>"))
        self.assertTrue(looks_like_challenge(403, "Attention Required! | Cloudflare"))
        self.assertFalse(looks_like_challenge(200, "#EXTM3U\n#EXT-X-VERSION:3"))
        self.assertFalse(looks_like_challenge(404, "not found"))


# --------------------------------------------------------------------------- #
# Fakes for the interception-based async chain
# --------------------------------------------------------------------------- #
class FakeResp:
    def __init__(self, url, status=200):
        self.url = url
        self.status = status


class FakeAPIResp:
    def __init__(self, status, body):
        self.status = status
        self._body = body

    async def text(self):
        return self._body


class FakeRequest:
    def __init__(self, getsources_body):
        self._body = getsources_body

    async def get(self, url, headers=None):
        # Only the getSources enrichment fetch uses APIRequestContext now (the
        # master liveness probe runs as an IN-PAGE fetch via page.evaluate).
        return FakeAPIResp(200, self._body)


class FakeContext:
    def __init__(self, getsources_body):
        self.request = FakeRequest(getsources_body)


class FakePage:
    def __init__(
        self,
        *,
        embed_url="https://megaplay.buzz/stream/s-2/141568/sub",
        master_url="https://s2.cinewave2.site/anime/aa/bb/master.m3u8",
        getsources_url="https://megaplay.buzz/stream/getSources?id=161323",
        fire_getsources=True,
        fire_master=True,
        title_value="Megaplay",
        category_href="/category/one-piece",
        nested_url="https://megaplay.buzz/stream/s-2/141568/sub",
    ):
        self.embed_url = embed_url
        self.master_url = master_url
        self.getsources_url = getsources_url
        self.fire_getsources = fire_getsources
        self.fire_master = fire_master
        self.title_value = title_value
        self.category_href = category_href
        self.nested_url = nested_url
        self.visited = []
        self._handlers = []
        # In-page master probe result (set by ctx()): "<status>|<first16>".
        self._probe_status = 200
        self._probe_head = "#EXTM3U\n#EXT-X-VER"

    def on(self, event, cb):
        if event == "response":
            self._handlers.append(cb)

    async def goto(self, url, referer=None, wait_until=None, timeout=None):
        self.visited.append(url)
        # The player goto happens after on('response') is registered → simulate
        # the player JS firing its getSources + master.m3u8 requests.
        if self._handlers:
            for cb in self._handlers:
                if self.fire_getsources:
                    cb(FakeResp(self.getsources_url))
                if self.fire_master:
                    cb(FakeResp(self.master_url))
        return FakeResp(url, status=200)

    async def title(self):
        return self.title_value

    async def evaluate(self, js, *args):
        # The in-page master liveness probe (returns "status|first16chars").
        if "fetch(url)" in js:
            return f"{self._probe_status}|{self._probe_head}"
        if "/category/" in js:
            return self.category_href
        if "data-video" in js:
            return self.embed_url
        if "querySelectorAll('iframe')" in js or "hosts.some" in js:
            return self.nested_url
        return None


def run(coro):
    return asyncio.run(coro)


def ctx(page, *, master_status=200, master_body="#EXTM3U\n#EXT-X-VERSION:3\n", **params):
    # The master liveness probe is an in-page fetch → drive it via the page.
    page._probe_status = master_status
    page._probe_head = master_body[:16]
    base = {"episode_url": "https://gogoanimes.fi/one-piece-episode-1", "category": "sub"}
    base.update(params)
    cfg = Config(capture_attempts=3, capture_delay=0.0)
    return RecipeContext(
        page=page,
        context=FakeContext(REAL_GETSOURCES),
        params=base,
        cfg=cfg,
        log=None,
        proxy_id="direct",
    )


class TestGogoanimeChain(unittest.TestCase):
    def test_happy_path_intercepts_master(self):
        page = FakePage()
        session = run(GogoanimeRecipe().resolve(ctx(page)))
        self.assertEqual(session["master_url"], "https://s2.cinewave2.site/anime/aa/bb/master.m3u8")
        self.assertEqual(session["referer"], "https://megaplay.buzz/")
        self.assertEqual(len(session["subtitles"]), 1)  # enriched from getSources
        self.assertEqual(session["intro"], {"start": 0, "end": 130})
        self.assertEqual(session["cdn_probe_status"], 200)  # probed live

    def test_dead_cdn_403_raises_so_orchestrator_fails_over(self):
        # megaplay pinned this stream to a WAF-blocked CDN (cdn.mewstream.buzz):
        # the master returns a 403 Cloudflare page. We must NOT hand that back as
        # success — raise so the Go orchestrator fails over to the next provider.
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(
                ctx(page, master_status=403, master_body="<!DOCTYPE html><title>Attention Required! | Cloudflare</title>")
            ))

    def test_expired_master_404_rejected(self):
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(
                ctx(page, master_status=404, master_body="<html>404 not found</html>")
            ))

    def test_200_but_not_a_playlist_rejected(self):
        # 200 with an HTML body (soft-block / interstitial) is not a live stream.
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(
                ctx(page, master_status=200, master_body="<html><body>blocked</body></html>")
            ))

    def test_navigation_host_allowlist(self):
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(ctx(page, episode_url="https://evil.com/ep")))

    def test_challenge_on_player(self):
        page = FakePage(title_value="Just a moment...")
        with self.assertRaises(ChallengeError):
            run(GogoanimeRecipe().resolve(ctx(page)))

    def test_getsources_only_no_playback_succeeds(self):
        # Cold profiles block autoplay so the .m3u8 is never fetched — the master
        # must still resolve from the getSources response alone.
        page = FakePage(fire_master=False)
        session = run(GogoanimeRecipe().resolve(ctx(page)))
        self.assertEqual(session["master_url"], "https://s2.cinewave2.site/anime/aa/bb/master.m3u8")

    def test_no_getsources_no_m3u8_is_recipe_error(self):
        page = FakePage(fire_getsources=False, fire_master=False)
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(ctx(page)))

    def test_search_path_resolves_with_base_url(self):
        page = FakePage()
        session = run(
            GogoanimeRecipe().resolve(
                ctx(page, episode_url=None, base_url="https://gogoanimes.fi",
                    keyword="One Piece", episode=1100)
            )
        )
        self.assertTrue(session["master_url"].endswith("master.m3u8"))
        self.assertIn("https://gogoanimes.fi/one-piece-episode-1100", page.visited)

    def test_base_url_required_when_no_episode_url(self):
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(ctx(page, episode_url=None, keyword="X", episode=1)))

    def test_embed_url_skips_episode_discovery(self):
        # The Go scraper passes a known megaplay embed; recipe goes straight to
        # the player + interception, no search/category navigation.
        page = FakePage()
        session = run(
            GogoanimeRecipe().resolve(
                ctx(page, episode_url=None,
                    embed_url="https://megaplay.buzz/stream/s-2/141568/sub")
            )
        )
        self.assertEqual(session["master_url"], "https://s2.cinewave2.site/anime/aa/bb/master.m3u8")
        self.assertFalse(any("/search.html" in u for u in page.visited))
        self.assertFalse(any("/category/" in u for u in page.visited))


if __name__ == "__main__":
    unittest.main()
