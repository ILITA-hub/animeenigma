"""miruro recipe — DISCOVERY-ONLY (secure-pipe API fetch through a solved session).

www.miruro.tv serves a Cloudflare **interactive Turnstile** challenge on its
homepage/SPA paths and a **hard WAF managed-rule block** ("Sorry, you have been
blocked") on ``/api/secure/pipe`` for any client without a solved challenge +
``cf_clearance``. A curl / plain-Go client gets 403 on BOTH. Camoufox clears the
homepage Turnstile (~9s, one checkbox click, our own datacenter IP, no
residential proxy); an in-page fetch to ``/api/secure/pipe`` then rides the same
origin + cf_clearance + TLS fingerprint and is treated as the SPA → 200 +
``x-obfuscated`` body (verified live 2026-07-02).

So this recipe opts into the engine's challenge solver (``solve_challenge=True``):
the warm-fetch nav solves the homepage Turnstile, and each secure-pipe GET rides
the warm session's in-page fetch.

There is NO ``resolve()`` here. The Go miruro provider builds every secure-pipe
``e=`` request descriptor and decodes every ``x-obfuscated`` response (owner-locked
Approach 2 — Go parses, the sidecar only fetches). The ``x-obfuscated`` RESPONSE
HEADER — which the Go decoder needs to pick the transport codec (gzip vs
xor+gzip) — is surfaced back through the /fetch header allowlist
(engine ``_FETCH_HEADER_ALLOWLIST``).
"""
from __future__ import annotations

from .base import Recipe

# Hosts this recipe may fetch (SSRF guard for /fetch). The canonical SPA origin
# plus the bare apex; the env2.js VITE_PROXY_A/B hosts (pro/pru.ultracloud.cc) are
# NOT reachable here — the live SPA talks to www.miruro.tv directly.
MIRURO_ALLOWED_HOSTS = {"www.miruro.tv", "miruro.tv"}


class MiruroRecipe(Recipe):
    name = "miruro"
    allowed_hosts = MIRURO_ALLOWED_HOSTS
    # www.miruro.tv is Cloudflare-managed-challenge gated on the homepage and hard-
    # WAF-blocked on /api/* for un-cleared clients — solve the Turnstile in the warm
    # nav rather than rotating the exit (see module docstring + engine
    # _solve_cf_challenge).
    solve_challenge = True
    # CF's silent __cf_chl_rt_tk managed challenge on www.miruro.tv is unpassable
    # from our datacenter IP (2026-07-20) — pin the warm solve to the Cloudflare
    # WARP exit, which clears it cleanly. Fail-open to direct if warp is unset.
    preferred_proxy_type = "warp"
    # The secure-pipe response marks its transport codec (gzip vs xor+gzip) in the
    # x-obfuscated RESPONSE header; the Go decoder needs it, so surface it back.
    response_header_allowlist = ("x-obfuscated",)
