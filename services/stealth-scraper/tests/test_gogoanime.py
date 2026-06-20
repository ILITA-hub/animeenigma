"""Unit tests for the gogoanime recipe — pure helpers + async chain via a fake
Playwright page (no browser/camoufox runtime needed)."""

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
        "sources": {
            "file": "https://cdn.mewstream.buzz/anime/aa/bb/master.m3u8"
        },
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
        self.assertEqual(out["master_url"], "https://cdn.mewstream.buzz/anime/aa/bb/master.m3u8")
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
        self.assertTrue(looks_like_challenge(200, "verifying you are human turnstile"))
        self.assertFalse(looks_like_challenge(200, "#EXTM3U\n#EXT-X-VERSION:3"))
        self.assertFalse(looks_like_challenge(404, "not found"))


# --------------------------------------------------------------------------- #
# Fake Playwright page for the async chain
# --------------------------------------------------------------------------- #
class FakeResp:
    def __init__(self, status):
        self.status = status


class FakePage:
    def __init__(
        self,
        *,
        embed_url="https://megaplay.buzz/stream/s-2/13461/sub",
        data_id="13461",
        getsources_status=200,
        getsources_body=REAL_GETSOURCES,
        cdn_status=200,
        cdn_head="#EXTM3U",
        title_value="Megaplay",
        category_href="/category/one-piece",
    ):
        self.category_href = category_href
        self.embed_url = embed_url
        self.data_id = data_id
        self.getsources_status = getsources_status
        self.getsources_body = getsources_body
        self.cdn_status = cdn_status
        self.cdn_head = cdn_head
        self.title_value = title_value
        self.visited = []

    async def goto(self, url, wait_until=None, timeout=None):
        self.visited.append(url)
        return FakeResp(200)

    async def title(self):
        return self.title_value

    async def evaluate(self, js, *args):
        if "/category/" in js:
            return self.category_href
        if "data-video" in js:
            return self.embed_url
        if "hosts.some" in js or "querySelectorAll('iframe')" in js:
            return None
        if "data-id" in js:
            return self.data_id
        if "X-Requested-With" in js:
            return {"status": self.getsources_status, "body": self.getsources_body}
        if "head" in js or "slice(0, 256)" in js:
            return {"status": self.cdn_status, "head": self.cdn_head}
        return None


def run(coro):
    return asyncio.run(coro)


def ctx(page, **params):
    base = {"episode_url": "https://gogoanimes.fi/one-piece-episode-1", "category": "sub"}
    base.update(params)
    return RecipeContext(
        page=page,
        context=None,
        params=base,
        cfg=Config(),
        log=None,
        proxy_id="direct",
    )


class TestGogoanimeChain(unittest.TestCase):
    def test_happy_path(self):
        page = FakePage()
        session = run(GogoanimeRecipe().resolve(ctx(page)))
        self.assertEqual(
            session["master_url"], "https://cdn.mewstream.buzz/anime/aa/bb/master.m3u8"
        )
        self.assertEqual(session["referer"], "https://megaplay.buzz/")
        self.assertEqual(session["cdn_probe_status"], 200)
        self.assertEqual(len(session["subtitles"]), 1)

    def test_navigation_host_allowlist(self):
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(ctx(page, episode_url="https://evil.com/ep")))

    def test_challenge_on_navigation(self):
        page = FakePage(title_value="Just a moment...")
        with self.assertRaises(ChallengeError):
            run(GogoanimeRecipe().resolve(ctx(page)))

    def test_challenge_on_cdn_probe(self):
        page = FakePage(cdn_status=403, cdn_head="Attention Required! | Cloudflare")
        with self.assertRaises(ChallengeError):
            run(GogoanimeRecipe().resolve(ctx(page)))

    def test_getsources_non_200_is_recipe_error(self):
        page = FakePage(getsources_status=404, getsources_body="nope")
        with self.assertRaises(RecipeError):
            run(GogoanimeRecipe().resolve(ctx(page)))

    def test_search_path_resolves_with_base_url(self):
        page = FakePage()
        session = run(
            GogoanimeRecipe().resolve(
                ctx(
                    page,
                    episode_url=None,
                    base_url="https://gogoanimes.fi",
                    keyword="One Piece",
                    episode=1100,
                )
            )
        )
        self.assertTrue(session["master_url"].endswith("master.m3u8"))
        # It actually navigated the built episode URL.
        self.assertIn("https://gogoanimes.fi/one-piece-episode-1100", page.visited)

    def test_base_url_required_when_no_episode_url(self):
        page = FakePage()
        with self.assertRaises(RecipeError):
            run(
                GogoanimeRecipe().resolve(
                    ctx(page, episode_url=None, keyword="One Piece", episode=1)
                )
            )


if __name__ == "__main__":
    unittest.main()
