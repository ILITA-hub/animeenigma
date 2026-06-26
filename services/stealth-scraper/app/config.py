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

    # -- Cloudflare managed/Turnstile challenge solving (opt-in per recipe) -- #
    # A recipe with solve_challenge=True (e.g. animepahe) asks the warm-fetch
    # path to SOLVE a Cloudflare "Just a moment…" managed challenge — click the
    # interactive Turnstile checkbox + poll for cf_clearance — instead of
    # rotating the exit IP on the first interstitial. Camoufox (headful in Xvfb,
    # humanized) passes the fingerprint check; the click + token round-trip is
    # the only piece a curl-class client cannot do. Recipes without the flag are
    # UNAFFECTED (the challenge still rotates the exit, as before).
    # solve budget: clearance lands ~10s in practice. The ENTIRE solve (click +
    # clearance poll + the post-clearance reload-confirm) is bounded by this, so
    # nav (nav_timeout_ms) + solve + first fetch (fetch_timeout_ms) stays under
    # the 90s Go sidecar-client timeout. Keep it well below 90 − nav − fetch.
    challenge_solve_timeout_ms: int = 30_000
    # Max interactive Turnstile checkbox clicks per solve attempt.
    challenge_click_max: int = 3

    # How long a resolved stream session (cookies + master url) is advertised as
    # fresh. cf_clearance typically lives ~30 min; keep this comfortably under it.
    session_ttl_seconds: int = 600
    # Hard timeout for a single in-page /hls fetch (playlist/segment). Unlike
    # resolve(), this path was unbounded — a hung CDN fetch pinned a browser slot
    # until the TTL. On timeout the session is torn down and the slot reclaimed.
    fetch_timeout_ms: int = 20_000
    # Max in-page fetch body size (bytes). The body is base64'd into memory, so an
    # oversized response is a memory-exhaustion DoS (~3x: blob + b64 + decode).
    max_body_bytes: int = 64 * 1024 * 1024
    # A resolved-but-never-fetched session (player resolved then abandoned) is
    # reaped after this short grace instead of holding a profile for the full TTL.
    unactivated_grace_seconds: int = 45
    # Background reaper cadence: frees expired/abandoned sessions and retires
    # over-used profiles without waiting for the next request.
    reaper_interval_seconds: float = 30.0

    # -- RAM-budgeted capacity (Phase 2) ------------------------------------ #
    # The pool is governed by the COMBINED Camoufox/Firefox RSS, not a fixed
    # instance count. soft = stop warming + evict idle sessions (back-pressure);
    # hard = refuse a new browser launch (503 kind=capacity) + evict the LRU
    # not-in-use session to reclaim.
    # STEALTH_POOL_SIZE survives only as a high fail-safe ceiling used when the
    # /proc RSS read fails.
    # Host-fit budget: this deployment's box is RAM-tight (already swapping), so
    # the combined Camoufox RSS budget lives INSIDE the existing 3500m container
    # (mem_limit unchanged) — admission refuses at 3 GiB before the kernel OOMs.
    ram_soft_bytes: int = 2 * 1024 * 1024 * 1024   # 2 GiB
    ram_hard_bytes: int = 3 * 1024 * 1024 * 1024   # 3 GiB
    ram_sample_seconds: float = 5.0
    # Max concurrent HELD sessions per user_key (fairness axis, Phase 2).
    user_quota: int = 2

    # -- self-heal (Phase 1) ------------------------------------------------ #
    # After this many consecutive in-page-fetch crashes ("Target closed" /
    # "context was destroyed") on the SAME warm session, the profile is torn
    # down and the caller fails over instead of nav-retrying a poisoned page.
    poison_max: int = 2
    # /readyz returns 503 only after the pool has been saturated (free==0) for
    # at least this long — a transient burst does not flip readiness.
    readyz_saturation_seconds: float = 15.0
    # Crashed-slot resurrection backoff: base * 2**consecutive_fail, capped.
    # 1 -> 2 -> 4 -> 8 -> 16 -> 30 (cap).
    resurrect_backoff_base_seconds: float = 1.0
    resurrect_backoff_cap_seconds: float = 30.0
    # After this many consecutive failed resurrect attempts a crashed slot is
    # retired (its user_data_dir wiped) instead of revived again.
    resurrect_max_fails: int = 3

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
            challenge_solve_timeout_ms=_int(g("STEALTH_CHALLENGE_SOLVE_TIMEOUT_MS"), 30_000),
            challenge_click_max=_int(g("STEALTH_CHALLENGE_CLICK_MAX"), 3),
            session_ttl_seconds=_int(g("STEALTH_SESSION_TTL_SECONDS"), 600),
            fetch_timeout_ms=_int(g("STEALTH_FETCH_TIMEOUT_MS"), 20_000),
            max_body_bytes=_int(g("STEALTH_MAX_BODY_BYTES"), 64 * 1024 * 1024),
            unactivated_grace_seconds=_int(g("STEALTH_UNACTIVATED_GRACE_SECONDS"), 45),
            reaper_interval_seconds=float(_int(g("STEALTH_REAPER_INTERVAL_SECONDS"), 30)),
            ram_soft_bytes=_int(g("STEALTH_RAM_SOFT_BYTES"), 2 * 1024 * 1024 * 1024),
            ram_hard_bytes=_int(g("STEALTH_RAM_HARD_BYTES"), 3 * 1024 * 1024 * 1024),
            ram_sample_seconds=float(_int(g("STEALTH_RAM_SAMPLE_SECONDS"), 5)),
            user_quota=_int(g("STEALTH_USER_QUOTA"), 2),
            poison_max=_int(g("STEALTH_POISON_MAX"), 2),
            readyz_saturation_seconds=float(
                _int(g("STEALTH_READYZ_SATURATION_SECONDS"), 15)
            ),
            resurrect_backoff_base_seconds=float(
                _int(g("STEALTH_RESURRECT_BACKOFF_BASE_SECONDS"), 1)
            ),
            resurrect_backoff_cap_seconds=float(
                _int(g("STEALTH_RESURRECT_BACKOFF_CAP_SECONDS"), 30)
            ),
            resurrect_max_fails=_int(g("STEALTH_RESURRECT_MAX_FAILS"), 3),
        )
