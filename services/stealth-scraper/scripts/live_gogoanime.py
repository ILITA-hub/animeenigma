"""Live end-to-end gogoanime probe (server-only, needs Camoufox binary).

Runs the real GogoanimeRecipe through a properly-configured Camoufox (virtual
display, persistent profile, humanize) to get a FRESH master.m3u8, then tries
the actual streaming fetch path (APIRequestContext, not browser fetch → no CORS)
the /hls proxy uses. Usage: python scripts/live_gogoanime.py "One Piece" 1
"""

import asyncio
import sys

from camoufox.async_api import AsyncCamoufox

from app.config import Config
from app.recipes.base import RecipeContext
from app.recipes.gogoanime import GogoanimeRecipe


async def main(keyword: str, episode: int) -> None:
    cfg = Config()
    async with AsyncCamoufox(
        headless="virtual", humanize=True, os="windows", geoip=True,
        persistent_context=True, user_data_dir="/tmp/ae-prof-live",
        i_know_what_im_doing=True,
    ) as ctx:
        page = await ctx.new_page()
        rc = RecipeContext(
            page=page, context=ctx,
            params={"keyword": keyword, "episode": episode, "category": "sub",
                    "base_url": "https://gogoanimes.fi"},
            cfg=cfg, log=None, proxy_id="direct",
        )
        try:
            session = await GogoanimeRecipe().resolve(rc)
        except Exception as e:  # noqa: BLE001
            print("RECIPE FAILED:", type(e).__name__, str(e)[:200])
            print("visited:", getattr(page, "_visited", "n/a"))
            return
        print("MASTER:", session["master_url"])
        print("referer:", session["referer"], "| cdn_probe_status:", session.get("cdn_probe_status"))
        print("subtitles:", len(session.get("subtitles", [])))

        # The REAL streaming path: APIRequestContext (no browser CORS).
        r = await ctx.request.get(session["master_url"], headers={"Referer": session["referer"]})
        body = await r.text()
        print("APIRequest master ->", r.status, "| ct:", (r.headers or {}).get("content-type"))
        print("body head:", repr(body[:80]))
        cookies = await ctx.cookies(session["master_url"])
        print("cdn cookies:", [c["name"] for c in cookies])


if __name__ == "__main__":
    kw = sys.argv[1] if len(sys.argv) > 1 else "One Piece"
    ep = int(sys.argv[2]) if len(sys.argv) > 2 else 1
    asyncio.run(main(kw, ep))
