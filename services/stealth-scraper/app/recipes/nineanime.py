"""nineanime recipe — identical megaplay player interception to gogoanime; only
the navigation host allowlist differs (9anime.me.uk family). The Go scraper
always supplies the megaplay `embed_url` it discovered (via /fetch), so the
inherited resolve() takes its embed_url branch and the search path is never run."""
from __future__ import annotations

from .gogoanime import GogoanimeRecipe

# Hosts this recipe may navigate / fetch (SSRF guard). Discovery hosts
# (9anime.me.uk, my.1anime.site) + the megaplay player wrappers. The rotating
# CDN segment hosts (mewstream/flarestorm) are reached via /hls, gated by
# host_allowed_for_session (dynamic), NOT this static list.
NINEANIME_ALLOWED_HOSTS = {
    "9anime.me.uk",
    "my.1anime.site",
    "1anime.site",
    "megaplay.buzz",
    "vidwish.live",
}


class NineAnimeRecipe(GogoanimeRecipe):
    name = "nineanime"
    allowed_hosts = NINEANIME_ALLOWED_HOSTS
