"""Environment-driven configuration for the stealth-scraper sidecar.

Pure stdlib (dataclass + os.environ) so it imports with zero third-party deps —
unit tests can construct/parse config without installing camoufox/fastapi.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field


def _bool(value: str | None, default: bool) -> bool:
    if value is None or value == "":
        return default
    return value.strip().lower() in ("1", "true", "yes", "on")


def _int(value: str | None, default: int) -> int:
    try:
        return int(value)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return default


def _csv(value: str | None, default: list[str]) -> list[str]:
    if not value:
        return list(default)
    return [v.strip() for v in value.split(",") if v.strip()]


@dataclass
class Config:
    # HTTP server
    host: str = "0.0.0.0"
    port: int = 3000

    # Browser pool.
    # headless: "virtual" (headful in an Xvfb virtual display — least detectable,
    # the default), "true" (true headless), or "false" (real headful display).
    # True headless Firefox is detectable; "virtual" passes where headless fails.
    headless: str = "virtual"
    pool_size: int = 2
    profile_dir: str = "/data/profiles"
    geoip: bool = True          # camoufox: align locale/timezone/geo to the proxy IP
    humanize: bool = True       # camoufox: human-like cursor movement
    os_rotate: list[str] = field(default_factory=lambda: ["windows", "macos", "linux"])
    # Resource types to abort during a resolve (bandwidth/fingerprint trim).
    # DEFAULT EMPTY: aborting media/xhr can stop the player JS from firing its
    # .m3u8 (verified — "no .m3u8 intercepted"). Correctness > bandwidth; opt in
    # per-deploy via STEALTH_BLOCK_RESOURCES only if a provider tolerates it.
    block_resources: list[str] = field(default_factory=list)

    # Tunnels (proxy)
    # STEALTH_PROXIES is a JSON array of {"id","type","url","geo"} entries.
    # Residential/mobile endpoints are SECRETS — supply via docker/.env, never git.
    proxies_json: str = ""
    warp_proxy_url: str = ""    # e.g. socks5://warp-proxy:1080 (off by default)
    proxy_cooldown_seconds: float = 120.0
    max_proxy_retries: int = 2  # rotate to a fresh exit this many times on challenge

    # Session aging / warming
    warming_enabled: bool = False
    warming_sites: list[str] = field(
        default_factory=lambda: [
            "https://www.google.com/",
            "https://en.wikipedia.org/wiki/Anime",
            "https://www.youtube.com/",
        ]
    )

    # Recipe / navigation.
    # NOTE: provider config (base_url, engine, enabled, ...) is NOT held here.
    # It lives in the DB roster table `scraper_providers` (catalog domain) and is
    # supplied per-request by the Go scraper. The sidecar carries NO provider-
    # specific envs/consts — only generic browser/navigation knobs.
    nav_timeout_ms: int = 30_000
    resolve_timeout_ms: int = 60_000
    # How long to wait for the player JS to fire its .m3u8 request (interception):
    # capture_attempts polls of capture_delay seconds each.
    capture_attempts: int = 40
    capture_delay: float = 0.5

    # How long a resolved stream session (cookies + master url) is advertised as
    # fresh. cf_clearance typically lives ~30 min; keep this comfortably under it.
    session_ttl_seconds: int = 600

    @classmethod
    def from_env(cls, env: dict[str, str] | None = None) -> "Config":
        e = env if env is not None else os.environ
        g = e.get
        return cls(
            host=g("HOST", "0.0.0.0"),
            port=_int(g("PORT"), 3000),
            headless=g("STEALTH_HEADLESS", "virtual").strip().lower(),
            pool_size=_int(g("STEALTH_POOL_SIZE"), 2),
            profile_dir=g("STEALTH_PROFILE_DIR", "/data/profiles"),
            geoip=_bool(g("STEALTH_GEOIP"), True),
            humanize=_bool(g("STEALTH_HUMANIZE"), True),
            os_rotate=_csv(g("STEALTH_OS_ROTATE"), ["windows", "macos", "linux"]),
            block_resources=_csv(g("STEALTH_BLOCK_RESOURCES"), []),
            proxies_json=g("STEALTH_PROXIES", ""),
            warp_proxy_url=g("STEALTH_WARP_PROXY_URL", ""),
            proxy_cooldown_seconds=float(_int(g("STEALTH_PROXY_COOLDOWN_SECONDS"), 120)),
            max_proxy_retries=_int(g("STEALTH_MAX_PROXY_RETRIES"), 2),
            warming_enabled=_bool(g("STEALTH_WARMING_ENABLED"), False),
            warming_sites=_csv(g("STEALTH_WARMING_SITES"), Config().warming_sites),
            nav_timeout_ms=_int(g("STEALTH_NAV_TIMEOUT_MS"), 30_000),
            resolve_timeout_ms=_int(g("STEALTH_RESOLVE_TIMEOUT_MS"), 60_000),
            capture_attempts=_int(g("STEALTH_CAPTURE_ATTEMPTS"), 40),
            capture_delay=float(g("STEALTH_CAPTURE_DELAY") or 0.5),
            session_ttl_seconds=_int(g("STEALTH_SESSION_TTL_SECONDS"), 600),
        )
