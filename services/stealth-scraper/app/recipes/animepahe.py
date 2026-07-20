"""animepahe recipe — DISCOVERY-ONLY (no player interception).

animepahe.pw sits behind a Cloudflare **managed (interactive Turnstile)
challenge**: a curl-class / plain-Go client gets a 403 "Just a moment…" page,
but Camoufox (headful in Xvfb, humanized) clears it in ~10s once the Turnstile
checkbox is clicked and cf_clearance is issued — verified live on this server's
own (datacenter) IP, no residential proxy needed.

So this recipe opts into the engine's challenge solver (``solve_challenge=True``):
the warm-fetch nav clicks the checkbox + polls for clearance instead of rotating
the exit on the first interstitial. All three discovery legs then ride the warm
session's in-page fetch (same TLS fingerprint + clearance cookie):

  - search:  /api?m=search&q=<query>                 → JSON (anime ``session``)
  - release: /api?m=release&id=<session>&page=<n>    → JSON (episode ``session``)
  - play:    /play/<anime_session>/<episode_session> → HTML (kwik.cx embeds)

There is NO ``resolve()`` here: the kwik.cx stream leg is reachable from plain
Go and is unpacked by the Go KwikExtractor (services/scraper/internal/embeds),
so only the animepahe.pw discovery legs need the browser. The Go provider drives
the whole chain (owner-locked Approach 2 — Go parses, the sidecar only fetches).
"""
from __future__ import annotations

from .base import Recipe

# Host this recipe may fetch (SSRF guard for /fetch). Only the canonical
# animepahe.pw origin — the kwik.cx stream host is fetched in Go, never here.
ANIMEPAHE_ALLOWED_HOSTS = {"animepahe.pw"}


class AnimePaheRecipe(Recipe):
    name = "animepahe"
    allowed_hosts = ANIMEPAHE_ALLOWED_HOSTS
    # animepahe.pw is Cloudflare-managed-challenge gated — solve it in the warm
    # nav rather than rotating the exit (see module docstring + engine
    # _solve_cf_challenge).
    solve_challenge = True
    # 2026-07-20: CF escalated animepahe.pw to the same SILENT __cf_chl_rt_tk
    # managed challenge as miruro — unpassable from our datacenter IP (the
    # interactive-Turnstile click the docstring describes is gone). Pin the warm
    # solve to the Cloudflare WARP exit; fail-open to direct if warp is unset.
    preferred_proxy_type = "warp"
