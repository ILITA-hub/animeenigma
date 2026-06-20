"""Recipe interface + shared pure helpers (no browser/third-party imports).

Importable in unit tests without camoufox/playwright installed: the async
``Recipe.resolve`` receives a duck-typed page/context, so this module never
imports Playwright types at runtime.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any
from urllib.parse import urlsplit


class RecipeError(Exception):
    """Hard, non-retryable failure (bad DOM, malformed payload, etc.)."""


class NotFoundError(RecipeError):
    """The requested title/episode could not be located on the provider."""


class ChallengeError(Exception):
    """A bot/anti-DDoS challenge (Cloudflare 403, Turnstile, JS interstitial)
    defeated this attempt. The engine catches this and rotates to a fresh exit
    IP / profile before retrying."""

    def __init__(self, message: str, host: str | None = None, kind: str = "unknown"):
        super().__init__(message)
        self.host = host
        self.kind = kind


# Substrings that mark a Cloudflare / DDoS-Guard / Turnstile challenge response.
# Matched case-insensitively against the response body or title.
_CHALLENGE_MARKERS = (
    "attention required",          # Cloudflare IP/firewall block
    "just a moment",               # Cloudflare managed/JS challenge
    "checking your browser",       # legacy CF / DDoS-Guard
    "cf-chl",                      # Cloudflare challenge platform tokens
    "challenge-platform",
    "cf_chl_opt",
    "ddos-guard",
    "ddosguard",
    "verifying you are human",
    "turnstile",
    "enable javascript and cookies to continue",
)


def looks_like_challenge(status: int | None, body: str | None) -> bool:
    """Heuristic: does this (status, body) pair look like a bot challenge?

    A 403/429/503 with an HTML body carrying a known challenge marker is treated
    as a challenge. A 2xx with a marker (managed-challenge interstitial served
    200) also counts.
    """
    text = (body or "").lower()
    has_marker = any(m in text for m in _CHALLENGE_MARKERS)
    if status in (401, 403, 429, 503) and (has_marker or "<html" in text):
        return True
    return has_marker


def host_allowed(host: str | None, allowed: set[str]) -> bool:
    """Exact-or-subdomain host allowlist (SSRF guard for recipe navigation)."""
    if not host:
        return False
    h = host.lower()
    return any(h == a or h.endswith("." + a) for a in allowed)


def host_of(url: str | None) -> str | None:
    if not url:
        return None
    try:
        return (urlsplit(url).hostname or "").lower() or None
    except ValueError:
        return None


@dataclass
class RecipeContext:
    """Everything a recipe needs for one resolve, supplied by the engine."""

    page: Any                  # Playwright Page (duck-typed)
    context: Any               # Playwright BrowserContext (duck-typed)
    params: dict[str, Any]
    cfg: Any
    log: Any
    proxy_id: str
    user_agent: str = ""

    def child(self, **overrides: Any) -> "RecipeContext":
        data = {**self.__dict__, **overrides}
        return RecipeContext(**data)


class Recipe:
    """Base class for provider recipes."""

    name: str = "base"
    # Hosts a recipe is permitted to navigate to. Enforced by the engine before
    # every page.goto so a compromised upstream cannot pivot the browser (SSRF).
    allowed_hosts: set[str] = field(default_factory=set)  # type: ignore[assignment]

    async def resolve(self, rc: RecipeContext) -> dict[str, Any]:
        """Drive the page chain and return a partial stream session:

            {
              "master_url": str,
              "referer": str,
              "subtitles": [{"url","label","default"}],
              "intro": {"start","end"} | None,
              "outro": {"start","end"} | None,
              "cdn_probe_status": int | None,   # status of the in-browser CDN fetch
            }

        The engine enriches this with cookies (harvested for the master_url
        host), user_agent, proxy_id, and expires_at. Raise ChallengeError to
        request an exit-IP rotation; RecipeError / NotFoundError to fail hard.
        """
        raise NotImplementedError
