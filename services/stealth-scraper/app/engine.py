"""CamoufoxEngine — warm browser pool + recipe execution + challenge rotation +
mandatory in-session stream proxying.

Ties together ProfileManager (aged persistent identities), ProxyPool (sticky exit
IPs, rotate-on-block), Camoufox launch options, and the recipe chain.

Stream proxying is MANDATORY (see streamproxy.py): on a successful resolve the
profile/browser is RETAINED as a Session, and the playlist + every segment are
fetched back through that same session's Playwright APIRequestContext — same exit
IP, same cookie jar (clearance), same TLS fingerprint. A bare master_url alone is
useless because the CDN is clearance-walled.

Camoufox is imported lazily so the pure-logic modules + unit tests need no
runtime/binary.
"""

from __future__ import annotations

import asyncio
import base64
import json
import os
import time
import uuid
from dataclasses import dataclass, field
from typing import Any
from urllib.parse import unquote

def _in_page_fetch_js(max_bytes: int, header_allowlist: tuple[str, ...] = ()) -> str:
    """Build the in-page fetch JS, returning "status|content-type|final-url|
    headers|base64body" (or "…|__TOO_LARGE__" in the body slot when the body
    exceeds ``max_bytes``). ``headers`` is encodeURIComponent(JSON) of the
    ``header_allowlist`` response headers that are present — URL-encoding is
    pipe-safe (``|`` → ``%7C``) so it can never collide with the field delimiter.
    The allowlist is per-recipe (Recipe.response_header_allowlist), empty for
    paths that don't need headers (e.g. the /hls proxy), so nothing leaks by
    default.

    Encodes the body via FileReader/blob for base64 (manual Uint8Array + btoa trips
    Camoufox's xray wrapper → "Permission denied to access property
    constructor"). Takes a single STRING arg (max_bytes + the allowlist are baked
    in as trusted values; object args also trip the wrapper). The size cap is
    baked in — checked via Content-Length first, then the realized blob size — so
    an oversized upstream body can't be base64'd into memory (a DoS amplified ~3x:
    blob + b64 + decode)."""
    allow_js = "[" + ",".join(f'"{h}"' for h in header_allowlist) + "]"
    return (
        "async (url) => {\n"
        "  const r = await fetch(url);\n"
        "  const ct = r.headers.get('content-type') || '';\n"
        f"  const allow = {allow_js};\n"
        "  const hs = {};\n"
        "  for (const k of allow) { const v = r.headers.get(k); if (v !== null) hs[k] = v; }\n"
        "  const he = encodeURIComponent(JSON.stringify(hs));\n"
        "  const head = r.status + '|' + ct + '|' + (r.url || '') + '|' + he + '|';\n"
        f"  const cap = {int(max_bytes)};\n"
        "  const clen = parseInt(r.headers.get('content-length') || '0', 10);\n"
        "  if (clen > cap) return head + '__TOO_LARGE__';\n"
        "  const blob = await r.blob();\n"
        "  if (blob.size > cap) return head + '__TOO_LARGE__';\n"
        "  const b64 = await new Promise((resolve, reject) => {\n"
        "    const fr = new FileReader();\n"
        "    fr.onloadend = () => resolve((fr.result + '').split(',')[1] || '');\n"
        "    fr.onerror = () => reject(new Error('filereader'));\n"
        "    fr.readAsDataURL(blob);\n"
        "  });\n"
        "  return head + b64;\n"
        "}"
    )


# Sentinel the in-page JS returns (in the body slot) when the upstream body
# exceeds the configured cap.
_TOO_LARGE = "__TOO_LARGE__"

from . import metrics
from .config import Config
from .fingerprint import build_launch_options, proxy_to_playwright
from .profiles import Profile, ProfileManager
from .ramsampler import process_tree_rss
from .recipes import ChallengeError, NotFoundError, Recipe, RecipeContext, RecipeError
from .recipes.animepahe import AnimePaheRecipe
from .recipes.base import host_allowed, host_of, looks_like_challenge
from .recipes.gogoanime import GogoanimeRecipe
from .recipes.miruro import MiruroRecipe
from .recipes.nineanime import NineAnimeRecipe
from .sessionstore import (
    SessionStore,
    camoufox_build,
    read_warm_marker,
    write_warm_marker,
)
from .streamproxy import looks_like_m3u8, make_wrap, rewrite_playlist
from .tunnels import ProxyPool, build_pool_from_config

# Resource types eligible for routing aborts. The ACTUAL set aborted is the
# intersection with cfg.block_resources (default empty → no routing installed),
# because aborting media/xhr can prevent the player JS from firing its .m3u8.
_ROUTE_BLOCKABLE = {"image", "font", "media", "stylesheet"}


@dataclass
class Session:
    id: str
    profile: Profile
    proxy_id: str
    referer: str
    user_agent: str
    cdn_host: str | None
    master_url: str
    expires_at: float
    # The live Playwright page kept open on the megaplay player origin. Cloudflare
    # gates these CDNs on the TLS/HTTP2 fingerprint of the connection, so the
    # playlist + segments can ONLY be fetched through the real Firefox network
    # stack (in-page fetch) — NOT the driver's HTTP client / Go / curl. This page
    # is that fetch context; it is the whole reason the session is retained.
    page: Any = None
    player_url: str = ""
    # Per-session cache of host -> SSRF-guard verdict so active playback does
    # not re-resolve DNS for every segment (see host_allowed_for_session).
    host_acl_cache: dict = field(default_factory=dict)
    # In-flight /hls fetch refcount: eviction must not close a page that a
    # concurrent proxy_fetch is awaiting a fetch on (use-after-close guard).
    in_use: int = 0
    # -- self-heal (Phase 1) ------------------------------------------------ #
    # Consecutive in-page-fetch crashes ("Target closed"/"context was destroyed")
    # on THIS warm session. At cfg.poison_max the profile is torn down and the
    # caller fails over, instead of nav-retrying a poisoned page (the AUTO-527
    # warm-session poison loop).
    crash_count: int = 0
    last_error: str = ""
    # Provider that owns this session (set for warm fetch:: sessions); used to
    # tag the wedge error so the Go breaker can attribute it.
    provider: str = ""
    # Quota accounting key (opaque user id or salted IP hash; never logged in
    # clear). None ⇒ unbounded (the caller opted out of per-user accounting).
    user_key: str | None = None
    # Wall-clock of the last persisted-record refresh (Layer B). Throttles
    # store writes to ~1/min instead of one per proxied segment.
    last_persist: float = 0.0


class _CamoufoxHandle:
    """Holds an open Camoufox persistent context (so cookie harvest works) via
    the async-CM protocol, kept warm outside an ``async with`` block."""

    def __init__(self, opts: dict) -> None:
        self._opts = opts
        self._cm: Any = None
        self.context: Any = None  # Playwright BrowserContext (persistent)

    async def open(self) -> Any:
        from camoufox.async_api import AsyncCamoufox  # lazy

        self._cm = AsyncCamoufox(**self._opts)
        self.context = await self._cm.__aenter__()
        return self.context

    async def close(self) -> None:
        if self._cm is not None:
            try:
                await self._cm.__aexit__(None, None, None)
            finally:
                self._cm = None
                self.context = None
                # Short grace so the OS reaps the browser process / closes the
                # CDP WebSocket before a relaunch reuses the same user_data_dir;
                # without it the new context can inherit a half-dead socket.
                try:
                    await asyncio.sleep(0.25)
                except Exception:  # noqa: BLE001
                    pass


class CamoufoxEngine:
    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.pool: ProxyPool = build_pool_from_config(cfg)
        self.profiles = ProfileManager(cfg.profile_dir, cfg.pool_size)
        self.store = SessionStore(os.path.join(cfg.profile_dir, "sessions"))
        self._handles: dict[str, _CamoufoxHandle] = {}     # profile id -> handle
        self._sessions: dict[str, Session] = {}            # session id -> Session
        # Per-(provider,origin) warm-fetch lock (SCRAPER-HEAL-04): serializes
        # concurrent _warm_fetch_session() callers for the SAME key so a second
        # caller reuses the first's session instead of racing it — see
        # _fetch_lock() for why an unlocked get-or-create leaks a profile.
        self._fetch_locks: dict[str, asyncio.Lock] = {}
        # Per-sid lazy-rehydrate lock (Layer B): serializes concurrent
        # proxy_fetch callers racing to rebuild the SAME dead sid so only one
        # relaunches the profile/page. Entries are intentionally never popped
        # (see _rehydrate) — the dict only grows by the rare sid that actually
        # gets rehydrated, which is bounded and small.
        self._rehydrate_locks: dict[str, asyncio.Lock] = {}
        self._recipes: dict[str, Recipe] = {
            "gogoanime": GogoanimeRecipe(),
            "nineanime": NineAnimeRecipe(),
            "animepahe": AnimePaheRecipe(),
            "miruro": MiruroRecipe(),
        }
        self._log: Any = None
        # Async DNS resolver (host -> list[ip str]); injectable in tests so the
        # SSRF guard is exercised without real network resolution.
        self._resolve_host = _default_resolve
        # Retained references for fire-and-forget page-close tasks (so they are
        # not GC'd before they run) + the background reaper handle.
        self._bg_tasks: set = set()
        self._reaper_task: Any = None
        # Monotonic-wall time the pool became saturated (free==0), or 0.0 when
        # not saturated. /readyz flips to 503 only once this persists past
        # cfg.readyz_saturation_seconds (a transient burst stays ready).
        self._saturated_since: float = 0.0
        # RAM-budgeted admission (Phase 2). _ram_bytes is refreshed by a
        # background sampler; the admission gate reads the cache on the request
        # path (cheap) and force-resamples on a near-hard read.
        self._ram_bytes: int = 0
        self._ram_task: Any = None
        # Governor-published degradation level (graceful-degradation Phase 3),
        # refreshed by _degradation_loop. 0 when the poller is disabled, the
        # governor is unreachable, or the level key is absent — FAIL-OPEN.
        self._degradation_level: int = 0
        self._degradation_task: Any = None
        self._degradation_warned: bool = False

    def set_logger(self, log: Any) -> None:
        self._log = log

    # -- lifecycle ---------------------------------------------------------- #
    async def start(self) -> None:
        os.makedirs(self.cfg.profile_dir, exist_ok=True)
        metrics.BROWSER_POOL_SIZE.set(0)
        metrics.ACTIVE_SESSIONS.set(0)
        self._reaper_task = asyncio.create_task(self._reaper_loop())
        self._ram_task = asyncio.create_task(self._ram_sampler_loop())
        if self.cfg.governor_url:
            self._degradation_task = asyncio.create_task(self._degradation_loop())

    async def stop(self) -> None:
        if self._degradation_task is not None:
            self._degradation_task.cancel()
            self._degradation_task = None
        if self._reaper_task is not None:
            self._reaper_task.cancel()
            self._reaper_task = None
        if self._ram_task is not None:
            self._ram_task.cancel()
            self._ram_task = None
        self._sessions.clear()
        for pid, handle in list(self._handles.items()):
            try:
                await handle.close()
            except Exception:  # noqa: BLE001
                pass
            self._handles.pop(pid, None)
        metrics.BROWSER_POOL_SIZE.set(0)
        metrics.ACTIVE_SESSIONS.set(0)

    def health(self) -> dict:
        # /healthz consumes this and ALWAYS returns 200 (process liveness) — the
        # body's per-provider/per-user breakdown is the observability surface;
        # /readyz (is_ready) is the saturation signal. The reaper + poison-fence
        # + 503-on-exhaustion self-heal the pool, so Docker must NOT restart-loop
        # the container on transient saturation.
        all_profiles = self.profiles.all()
        counts = self.profiles.status_counts()
        free = sum(1 for p in all_profiles if p.status == "healthy" and not p.leased)

        providers: dict[str, dict] = {}
        users: dict[str, dict] = {}
        for s in self._sessions.values():
            if s.provider:
                pv = providers.setdefault(
                    s.provider, {"held": 0, "crashed": 0, "last_error": ""}
                )
                pv["held"] += 1
                if s.crash_count:
                    pv["crashed"] += 1
                if s.last_error:
                    pv["last_error"] = s.last_error
            uk = getattr(s, "user_key", "") or ""
            if uk:
                users.setdefault(uk, {"held": 0})["held"] += 1

        return {
            # Legacy keys (back-compat for any existing scrape/consumer).
            "status": "degraded" if free == 0 else "ok",
            "pool_size": self.cfg.pool_size,
            "free_profiles": free,
            "live_browsers": len(self._handles),
            "active_sessions": len(self._sessions),
            # RAM-budgeted capacity (Phase 2): last sampled combined RSS.
            "ram_bytes": self._ram_bytes,
            "proxies": [
                {"id": e.id, "type": e.type, "blocked": e.total_blocked}
                for e in self.pool.all()
            ],
            # New self-heal breakdown (Phase 1).
            "global": {
                "free": free,
                "crashed": counts.get("crashed", 0),
                "warming": counts.get("warming", 0),
                "live_browsers": len(self._handles),
                "active_sessions": len(self._sessions),
            },
            "providers": providers,
            "users": users,
        }

    def is_ready(self) -> bool:
        """Readiness for /readyz. NOT a liveness signal (see D8 — never drives a
        restart; in-flight streams must keep playing). Returns False only when
        the pool has been saturated (no healthy free slot) continuously for at
        least cfg.readyz_saturation_seconds."""
        free = sum(
            1 for p in self.profiles.all() if p.status == "healthy" and not p.leased
        )
        now = time.time()
        if free > 0:
            self._saturated_since = 0.0
            return True
        if self._saturated_since == 0.0:
            self._saturated_since = now
        return (now - self._saturated_since) < self.cfg.readyz_saturation_seconds

    # -- launch / teardown -------------------------------------------------- #
    async def _ensure_browser(self, profile: Profile, proxy_id: str) -> Any:
        if profile.launched and profile.proxy_id == proxy_id:
            return profile.context
        # RAM admission: refuse a launch over the hard budget (reclaiming LRU
        # first). A warm reuse (profile already launched on the same proxy)
        # returns above and skips the gate — it consumes no NEW memory.
        self._admit_launch()
        if profile.launched:
            await self._teardown(profile, reason="rotate")

        proxy_entry = self.pool.get(proxy_id)
        proxy_dict = proxy_to_playwright(proxy_entry.url) if proxy_entry else None
        opts = build_launch_options(
            profile_id=profile.id,
            user_data_dir=profile.user_data_dir,
            proxy=proxy_dict,
            geo=proxy_entry.geo if proxy_entry else None,
            cfg=self.cfg,
        )
        handle = _CamoufoxHandle(opts)
        context = await handle.open()
        await self._install_routing(context)

        self._handles[profile.id] = handle
        profile.browser = handle
        profile.context = context
        profile.proxy_id = proxy_id
        try:
            page = await context.new_page()
            profile.user_agent = await page.evaluate("() => navigator.userAgent")
            marker = read_warm_marker(profile.user_data_dir)
            marker_fresh = (
                marker is not None
                and marker.get("camoufox_build") == camoufox_build()
                and time.time() - marker.get("warmed_at", 0)
                < self.cfg.warm_marker_ttl_seconds
            )
            if self.cfg.warming_enabled and self._warming_allowed() and not marker_fresh:
                from .warming import warm_profile

                await warm_profile(
                    page, self.cfg.warming_sites, self._log,
                    nav_timeout_ms=self.cfg.nav_timeout_ms,
                )
                write_warm_marker(profile.user_data_dir)
            await page.close()
        except Exception:  # noqa: BLE001
            profile.user_agent = profile.user_agent or ""

        metrics.BROWSER_POOL_SIZE.set(len(self._handles))
        metrics.BROWSER_RELAUNCH_TOTAL.labels(reason="cold").inc()
        return context

    async def _teardown(self, profile: Profile, *, reason: str) -> None:
        handle = self._handles.pop(profile.id, None)
        if handle is not None:
            try:
                await handle.close()
            except Exception:  # noqa: BLE001
                pass
        self.profiles.reset_handles(profile)
        # A "crash" teardown leaves the slot marked crashed so the reaper
        # resurrects it (a relaunch can't be done inline — the caller is failing
        # over). Other reasons (rotate/recycle/cold) keep the slot healthy so a
        # fresh lease re-launches it normally.
        if reason == "crash":
            self.profiles.mark_crashed(profile, error=profile.last_error)
        metrics.BROWSER_POOL_SIZE.set(len(self._handles))
        metrics.BROWSER_RELAUNCH_TOTAL.labels(reason=reason).inc()
        metrics.POOL_CRASHED.set(self.profiles.status_counts().get("crashed", 0))

    async def _install_routing(self, context: Any) -> None:
        # Only abort the configured resource types; default (empty) installs no
        # routing at all — aborting media/xhr can stop the player firing its .m3u8.
        block = {r for r in (self.cfg.block_resources or []) if r in _ROUTE_BLOCKABLE}
        if not block:
            return

        async def _route(route: Any) -> None:
            try:
                if route.request.resource_type in block:
                    await route.abort()
                else:
                    await route.continue_()
            except Exception:  # noqa: BLE001
                try:
                    await route.continue_()
                except Exception:  # noqa: BLE001
                    pass

        try:
            await context.route("**/*", _route)
        except Exception:  # noqa: BLE001
            pass

    # -- resolve ------------------------------------------------------------ #
    async def resolve(self, provider: str, params: dict, user_key: str | None = None) -> dict:
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")
        self._shed_new_work()

        self._evict_expired()
        self._enforce_user_quota(user_key)
        started = time.monotonic()
        tried: set[str] = set()
        last_err: Exception | None = None

        for _ in range(self.cfg.max_proxy_retries + 1):
            profile = await self._acquire_profile()
            if profile is None:
                metrics.POOL_EXHAUSTED_TOTAL.inc()
                metrics.RESOLVE_TOTAL.labels(provider=provider, result="exhausted").inc()
                raise PoolExhausted("no free browser profile (pool/sessions exhausted)")

            proxy = self.pool.select(
                preferred_type=params.get("proxy_type"),
                sticky_key=profile.id,
                exclude=tried,
            )
            if proxy is None:
                self.profiles.release(profile, ok=False)
                break
            metrics.PROXY_SELECT_TOTAL.labels(proxy_id=proxy.id, type=proxy.type).inc()

            page = None
            try:
                context = await self._ensure_browser(profile, proxy.id)
                page = await context.new_page()
                rc = RecipeContext(
                    page=page, context=context, params=params, cfg=self.cfg,
                    log=self._log, proxy_id=proxy.id, user_agent=profile.user_agent,
                )
                partial = await asyncio.wait_for(
                    recipe.resolve(rc),
                    timeout=self.cfg.resolve_timeout_ms / 1000.0,
                )

                # SUCCESS: retain the page + profile as a Session. The CDN is
                # Cloudflare-fingerprint-gated, so the playlist + segments can
                # only be fetched through THIS browser page (in-page fetch). The
                # page stays open (NOT closed) and the profile stays leased until
                # the session expires / is closed.
                session = await self._open_session(partial, context, proxy.id, profile, page, user_key)
                self.pool.mark_ok(proxy.id)
                metrics.RESOLVE_TOTAL.labels(provider=provider, result="ok").inc()
                metrics.RESOLVE_DURATION.labels(provider=provider).observe(
                    time.monotonic() - started
                )
                return self._session_payload(session, partial)

            except ChallengeError as exc:
                last_err = exc
                await _safe_close_page(page)
                metrics.CHALLENGE_TOTAL.labels(host=exc.host or "?", kind=exc.kind).inc()
                metrics.PROXY_BLOCK_TOTAL.labels(proxy_id=proxy.id).inc()
                self.pool.mark_blocked(proxy.id)
                tried.add(proxy.id)
                await self._teardown(profile, reason="rotate")
                self.profiles.release(profile, ok=False)
                continue

            except (NotFoundError, RecipeError) as exc:
                last_err = exc
                await _safe_close_page(page)
                self.profiles.release(profile, ok=False)
                result = "not_found" if isinstance(exc, NotFoundError) else "error"
                metrics.RESOLVE_TOTAL.labels(provider=provider, result=result).inc()
                raise

            except asyncio.TimeoutError as exc:
                # A timeout/crash is a BROWSER fault, not a proxy fault — tear the
                # (possibly wedged) browser down and retry with a FRESH cold one
                # on the SAME exit. Do NOT add the proxy to ``tried``: with a
                # single exit, excluding it here makes pool.select() return None
                # on the next pass → the loop gives up after one attempt instead
                # of self-healing the browser.
                last_err = exc
                await _safe_close_page(page)
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                continue

            except asyncio.CancelledError:
                # HTTP client disconnected while we were awaiting inside the
                # browser. CancelledError is a BaseException, not an Exception,
                # so the generic handler below cannot catch it — without this
                # clause the profile stays permanently leased (pool exhausted).
                await _safe_close_page(page)
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                raise

            except Exception as exc:  # noqa: BLE001
                # Driver/context death ("Connection closed while reading from the
                # driver", "unable to perform operation on <WriteUnixTransport>")
                # — same as above: recycle the browser, keep the exit, retry.
                last_err = exc
                await _safe_close_page(page)
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                continue

        # Distinguish "every exit was challenged" (rotate-worthy) from "the
        # browser kept crashing on the only exit" — both 5xx to the Go side, but
        # the metric + message should not lie about a challenge that never came.
        if isinstance(last_err, ChallengeError):
            metrics.RESOLVE_TOTAL.labels(provider=provider, result="challenge").inc()
            raise ChallengeError(
                f"exhausted {len(tried)} exit(s); last: {last_err}", kind="exhausted"
            )
        metrics.RESOLVE_TOTAL.labels(provider=provider, result="error").inc()
        raise RecipeError(
            f"resolve failed after {self.cfg.max_proxy_retries + 1} browser attempt(s): {last_err}"
        )

    async def _acquire_profile(self, retries: int = 50) -> Profile | None:
        for _ in range(retries):
            p = self.profiles.lease()
            if p is not None:
                return p
            await asyncio.sleep(0.1)
        return None

    # -- RAM-budgeted admission (Phase 2) ----------------------------------- #
    def _sample_ram(self) -> int:
        """Combined Camoufox/Firefox RSS (bytes). Fail-safe: on any /proc read
        error return 0 so the gate admits (the pool_size ceiling still bounds)."""
        try:
            return process_tree_rss()
        except Exception:  # noqa: BLE001
            return 0

    def _read_ram(self) -> int:
        """Force a fresh fail-safe RSS read and refresh the cache. ``_sample_ram``
        is the protected reader (returns 0 on /proc error), but a test/override
        may itself raise — treat ANY failure as 0 (admit) so the gate never
        crashes the request path. Both the warming check and the admission gate
        read RAM through here so a burst between sampler ticks can't slip past
        the budget."""
        try:
            ram = self._sample_ram()
        except Exception:  # noqa: BLE001
            ram = 0
        self._ram_bytes = ram
        return ram

    async def _ram_sampler_loop(self) -> None:
        while True:
            try:
                await asyncio.sleep(self.cfg.ram_sample_seconds)
                self._ram_bytes = self._sample_ram()
                metrics.RAM_BYTES.set(self._ram_bytes)
            except asyncio.CancelledError:
                break
            except Exception:  # noqa: BLE001
                if self._log:
                    self._log.exception("ram sampler tick failed")

    async def _degradation_loop(self) -> None:
        """Poll the governor's status endpoint (graceful-degradation Phase 3)
        and cache the published level. Fail-open: any error, timeout, or shape
        mismatch reads as level 0 — the sidecar must never shed because the
        signal is missing. Runs in a thread executor (stdlib urllib; the
        sidecar deliberately has no async HTTP client dependency)."""
        import json as _json
        import urllib.request as _rq

        url = self.cfg.governor_url.rstrip("/") + "/api/degradation/status"

        def _fetch() -> int:
            with _rq.urlopen(url, timeout=3) as resp:  # noqa: S310 (fixed internal URL)
                body = _json.loads(resp.read(65536))
            level = int(body.get("data", {}).get("level", 0))
            return level if 0 <= level <= 2 else 0

        loop = asyncio.get_running_loop()
        while True:
            try:
                await asyncio.sleep(self.cfg.degradation_poll_seconds)
                level = await loop.run_in_executor(None, _fetch)
                if level != self._degradation_level and self._log:
                    self._log.info("degradation level %d -> %d", self._degradation_level, level)
                self._degradation_level = level
                self._degradation_warned = False
            except asyncio.CancelledError:
                break
            except Exception:  # noqa: BLE001 — fail-open on ANY poll failure
                if not self._degradation_warned and self._log:
                    self._log.warning("governor poll failed; degradation level fails open to 0")
                self._degradation_warned = True
                self._degradation_level = 0
            metrics.DEGRADATION_LEVEL_SEEN.set(self._degradation_level)
            metrics.DEGRADATION_SHED.labels(subsystem="camoufox").set(
                0 if self._degradation_level < 1 else self._degradation_level
            )

    def _shed_new_work(self) -> None:
        """Raise DegradedShed when the host is at Critical pressure — gates the
        two NEW-work entry points (resolve, browser_fetch). Session reuse and
        /hls proxy fetches for existing sessions are never gated (they serve
        in-flight playback, the very thing shedding protects)."""
        if self._degradation_level >= 2:
            raise DegradedShed("new work refused: host degradation level 2 (critical)")

    def _warming_allowed(self) -> bool:
        """False once combined RSS reaches the soft budget — new profiles are
        not warmed under back-pressure (existing leases untouched) — or while
        the platform degradation level is Elevated+ (Phase 3 shedding)."""
        if self._degradation_level >= 1:
            return False
        return self._read_ram() < self.cfg.ram_soft_bytes

    def _evict_one_lru(self) -> bool:
        """Evict the least-recently-used NOT-in-use session (smallest
        expires_at) to reclaim a browser slot. Returns True if one was freed.
        Never touches an in-use session (a concurrent /hls fetch is awaiting it)."""
        candidates = [
            (s.expires_at, sid, s)
            for sid, s in self._sessions.items()
            if s.in_use <= 0
        ]
        if not candidates:
            return False
        candidates.sort(key=lambda t: t[0])
        _, sid, session = candidates[0]
        self._sessions.pop(sid, None)
        self.store.delete(sid)
        self._spawn(_safe_close_page(session.page))
        self.profiles.release(session.profile, ok=True)
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        return True

    def _held_for_user(self, user_key: str) -> int:
        return sum(1 for s in self._sessions.values() if s.user_key == user_key)

    def _enforce_user_quota(self, user_key: str | None) -> None:
        """Reject a NEW held session when user_key already holds >= quota. A
        falsy user_key is unbounded (the catalog always supplies one — a salted
        IP hash for anon — so a missing key here is an explicit opt-out)."""
        if not user_key:
            return
        if self._held_for_user(user_key) >= self.cfg.user_quota:
            metrics.USER_QUOTA_REJECTED_TOTAL.inc()
            raise UserQuotaExceeded(
                f"user {user_key[:8]}… holds >= quota ({self.cfg.user_quota})"
            )

    def _admit_launch(self) -> None:
        """Admission gate at the single browser-launch chokepoint.

          - ram < soft            → admit.
          - soft <= ram < hard    → admit, but proactively evict idle/expired
                                     not-in-use sessions (back-pressure).
          - ram >= hard           → refuse (CapacityExceeded); evict the LRU
                                     not-in-use session to reclaim, then raise.
        Fail-safe: _read_ram() returns 0 on a /proc (or sampler) error → always
        admits, and the pool_size ceiling still bounds the launch."""
        ram = self._read_ram()
        if ram >= self.cfg.ram_hard_bytes:
            metrics.ADMISSION_TOTAL.labels(
                action="hard_evict" if self._evict_one_lru() else "hard_refuse"
            ).inc()
            raise CapacityExceeded(
                f"combined RSS {ram} >= hard budget {self.cfg.ram_hard_bytes}"
            )
        if ram >= self.cfg.ram_soft_bytes:
            self._evict_expired()  # drop idle/expired not-in-use sessions
            metrics.ADMISSION_TOTAL.labels(action="soft_evict").inc()

    def _session_record(self, session: Session) -> dict:
        return {
            "sid": session.id,
            "master_url": session.master_url,
            "player_url": session.player_url,
            "referer": session.referer,
            "profile_id": session.profile.id,
            "proxy_id": session.proxy_id,
            "user_key": session.user_key,
            "cdn_host": session.cdn_host,
            "expires_at": session.expires_at,
            "camoufox_build": camoufox_build(),
            "created_at": time.time(),
        }

    async def _open_session(
        self, partial: dict, context: Any, proxy_id: str, profile: Profile, page: Any,
        user_key: str | None = None,
    ) -> Session:
        master = partial.get("master_url")
        player_url = ""
        try:
            player_url = page.url
        except Exception:  # noqa: BLE001
            player_url = ""

        sid = uuid.uuid4().hex
        session = Session(
            id=sid,
            profile=profile,
            proxy_id=proxy_id,
            referer=partial.get("referer", ""),
            user_agent=profile.user_agent,
            cdn_host=host_of(master),
            master_url=master,
            # Start on the SHORT unactivated grace: a resolved-but-never-fetched
            # session (player resolved then abandoned) frees its profile fast.
            # The first proxy_fetch slides it to the full session_ttl_seconds.
            expires_at=time.time() + self.cfg.unactivated_grace_seconds,
            page=page,
            player_url=player_url,
            user_key=user_key,
        )
        self._sessions[sid] = session
        # Deliberately leave last_persist at its dataclass default (0.0): this
        # initial record carries the SHORT unactivated_grace expires_at, and
        # proxy_fetch's refresh-persist is throttled to >60s since last_persist.
        # If we stamped last_persist here, the record would sit un-refreshed
        # (still on the short grace) for up to a minute of active playback
        # before the throttle would allow the first real persist — a window
        # where the record can expire while the session is actually alive.
        # Leaving it at 0.0 makes the FIRST proxy_fetch persist immediately,
        # sliding the record to the full session_ttl_seconds right away.
        self.store.save(self._session_record(session))
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        return session

    def _session_payload(self, session: Session, partial: dict) -> dict:
        return {
            "session_id": session.id,
            "master_url": session.master_url,
            # Path the caller proxies the stream through (host-prefixed by caller).
            "playlist_proxy_path": f"/hls?sid={session.id}&url={_q(session.master_url)}",
            "referer": session.referer,
            # Subtitle files live on the SAME Cloudflare-fingerprint-gated CDN
            # family as the playlist/segments (verified 2026-07-17: a direct
            # fetch of the raw subtitle URL 403s exactly like the raw master
            # URL does) — proxy_path routes them through this session's /hls
            # browser fetch too, mirroring playlist_proxy_path above.
            "subtitles": [
                {**s, "proxy_path": f"/hls?sid={session.id}&url={_q(s['url'])}"}
                for s in partial.get("subtitles", [])
                if s.get("url")
            ],
            "intro": partial.get("intro"),
            "outro": partial.get("outro"),
            "cdn_probe_status": partial.get("cdn_probe_status"),
            "cookies": partial.get("cookies", []),
            "user_agent": session.user_agent,
            "proxy_id": session.proxy_id,
            "cdn_host": session.cdn_host,
            "resolved_via": "camoufox",
            # Advertise the full watch TTL (the session starts on the short
            # unactivated grace, but the first /hls fetch extends it to this).
            "expires_at": int(time.time() + self.cfg.session_ttl_seconds),
        }

    def _direct_payload(self, partial: dict, proxy_id: str) -> dict:
        """Return the resolved REAL master URL + referer (no session). The
        downstream streaming HLS proxy fetches it directly with the Referer."""
        master = partial.get("master_url")
        return {
            "master_url": master,
            "referer": partial.get("referer", ""),
            "subtitles": partial.get("subtitles", []),
            "intro": partial.get("intro"),
            "outro": partial.get("outro"),
            "proxy_id": proxy_id,
            "cdn_host": host_of(master),
            "cdn_probe_status": partial.get("cdn_probe_status"),
            "resolved_via": "camoufox",
        }

    async def _rehydrate(self, sid: str) -> Session | None:
        """Rebuild a session that died with a previous process (Layer B of the
        playback self-healing design): same sid, recorded profile preferred,
        warming skipped (the on-disk profile is already warm), master playlist
        re-fetched once as the go/no-go check. Refuses across Camoufox builds.

        Lock lifecycle: the per-sid lock in ``self._rehydrate_locks`` is
        deliberately never removed. Popping it inside the ``finally`` while
        still under ``async with lock:`` would let a late-arriving concurrent
        caller's ``_rehydrate_lock()`` create a brand-new, unlocked
        ``asyncio.Lock`` for the same sid and start a second rehydrate before
        the first one's ``async
        with`` actually releases the original lock — two coroutines rebuilding
        the same sid at once. Leaving the entry in place means every future
        call for this sid contends on the SAME lock object, which is correct;
        the dict only grows by the (rare) distinct sids that ever needed
        rehydrating, which is small and bounded over a process lifetime."""
        rec = self.store.load(sid)
        if rec is None:
            metrics.REHYDRATE_TOTAL.labels(result="no_record").inc()
            return None
        if rec.get("camoufox_build") != camoufox_build():
            metrics.REHYDRATE_TOTAL.labels(result="build_mismatch").inc()
            self.store.delete(sid)
            return None
        if rec.get("expires_at", 0) <= time.time():
            metrics.REHYDRATE_TOTAL.labels(result="expired").inc()
            self.store.delete(sid)
            return None

        async with self._rehydrate_lock(sid):
            existing = self._sessions.get(sid)
            if existing is not None:  # lost the race — another fetch rebuilt it
                return existing
            # CapacityExceeded is a TRANSIENT RAM condition, not a dead sid — it
            # must not fall through to the generic `except Exception` below,
            # which would delete the still-good persisted record and (once
            # profile exists) release a lease that was never taken. Catching it
            # here, BEFORE the lease, means: no profile to release, the record
            # survives so a later, less-pressured fetch can still rehydrate, and
            # the caller sees plain SessionGone (proxy_fetch -> /hls -> 410) that
            # the FE's retry net + Go liveness gate already handle — instead of
            # a dead-end RecipeError -> 400.
            try:
                self._admit_launch()
            except CapacityExceeded:
                metrics.REHYDRATE_TOTAL.labels(result="error").inc()
                return None
            profile = self.profiles.lease(preferred=rec.get("profile_id"))
            if profile is None:
                metrics.REHYDRATE_TOTAL.labels(result="no_profile").inc()
                return None
            page = None
            try:
                context = await self._ensure_browser(profile, rec["proxy_id"])
                page = await context.new_page()
                await page.goto(
                    rec["player_url"],
                    wait_until="domcontentloaded",
                    timeout=self.cfg.nav_timeout_ms,
                )
                session = Session(
                    id=sid,
                    profile=profile,
                    proxy_id=rec["proxy_id"],
                    referer=rec.get("referer", ""),
                    user_agent=profile.user_agent,
                    cdn_host=rec.get("cdn_host"),
                    master_url=rec["master_url"],
                    expires_at=time.time() + self.cfg.session_ttl_seconds,
                    page=page,
                    player_url=rec.get("player_url", ""),
                    user_key=rec.get("user_key"),
                )
                status, _ctype, _final, _hdrs, _body = await self._in_page_fetch(
                    session, rec["master_url"]
                )
                if status != 200:
                    raise RecipeError(f"rehydrate verify: master fetch {status}")
                self._sessions[sid] = session
                session.last_persist = time.time()
                self.store.save(self._session_record(session))
                metrics.ACTIVE_SESSIONS.set(len(self._sessions))
                metrics.REHYDRATE_TOTAL.labels(result="ok").inc()
                if self._log:
                    self._log.info("session rehydrated", extra={"sid": sid[:8]})
                return session
            except Exception as exc:  # noqa: BLE001 — any failure ⇒ clean 410
                metrics.REHYDRATE_TOTAL.labels(
                    result="verify_failed" if isinstance(exc, RecipeError) else "error"
                ).inc()
                if page is not None:
                    await _safe_close_page(page)
                self.profiles.release(profile, ok=False)
                self.store.delete(sid)
                return None

    # -- stream proxy (mandatory: Cloudflare-fingerprint-gated CDNs) -------- #
    async def proxy_fetch(self, sid: str, url: str) -> dict:
        """Fetch ``url`` through the session page's IN-PAGE ``fetch()`` — i.e. the
        real Firefox network stack (same TLS/HTTP2 fingerprint the player uses).

        This is the whole point of the sidecar restream: the megaplay CDNs
        (mewstream.buzz / flarestorm.buzz / …) sit behind Cloudflare bot-management
        that gates on the connection's TLS fingerprint, so curl / Go net/http /
        Playwright's APIRequestContext all get a 403 "Attention Required" page —
        ONLY an in-page browser fetch passes (verified 2026-06-20). Playlists are
        rewritten so child URIs route back through this proxy. Returns
        {status, content_type, body(bytes)}."""
        self._evict_expired()
        session = self._sessions.get(sid)
        if session is None:
            session = await self._rehydrate(sid)
        if session is None:
            raise SessionGone(sid)
        if not await host_allowed_for_session(url, session, self._resolve_host):
            metrics.STEALTH_PROXY_FETCH_TOTAL.labels(result="host_denied").inc()
            raise RecipeError(f"proxy host not allowed for session: {host_of(url)}")

        # Slide the TTL BEFORE the (possibly long) fetch and mark the session
        # in-use, so a concurrent eviction can't close the page mid-fetch
        # (use-after-close) and an actively-fetched session can't lapse its
        # deadline while the network round-trip is in flight.
        session.expires_at = time.time() + self.cfg.session_ttl_seconds
        session.in_use += 1
        started = time.monotonic()
        try:
            status, ctype, final_url, _hdrs, body = await self._in_page_fetch(session, url)
        except FetchTimeout:
            # A hung fetch would pin this browser slot forever — reclaim it.
            self._observe_proxy_fetch("timeout", started)
            await self.aclose_session(sid)
            raise
        except FetchTooLarge:
            # Over-cap body (resource-leak guard tripped) — count it distinctly.
            self._observe_proxy_fetch("too_large", started)
            raise
        finally:
            session.in_use = max(0, session.in_use - 1)
        self._observe_proxy_fetch("ok", started, body_bytes=len(body))

        # The in-page fetch follows redirects; a trusted CDN could 30x toward an
        # internal target. Re-validate the post-redirect URL before returning its
        # body (closes the redirect bypass of the SSRF guard).
        if (
            final_url
            and final_url != url
            and not await host_allowed_for_session(final_url, session, self._resolve_host)
        ):
            raise RecipeError(f"proxy redirect target not allowed: {host_of(final_url)}")

        # Bump the sliding window again on completion.
        session.expires_at = time.time() + self.cfg.session_ttl_seconds

        # Refresh the persisted record so a redeploy mid-watch can rehydrate
        # with an accurate deadline — throttled, segments arrive every ~4s.
        if time.time() - session.last_persist > 60:
            session.last_persist = time.time()
            self.store.save(self._session_record(session))

        text_head = body[:64].decode("utf-8", "ignore")
        if looks_like_m3u8(text_head, ctype):
            wrapped = rewrite_playlist(
                body.decode("utf-8", "ignore"),
                url,
                make_wrap(sid, lambda s, u: f"/hls?sid={s}&url={_q(u)}"),
            )
            return {
                "status": status,
                "content_type": "application/vnd.apple.mpegurl",
                "body": wrapped.encode("utf-8"),
            }
        return {"status": status, "content_type": ctype or "application/octet-stream", "body": body}

    @staticmethod
    def _observe_proxy_fetch(
        result: str, started: float, body_bytes: int | None = None
    ) -> None:
        """Record restream in-page-fetch observability (audit L613): duration +
        result counter, plus body size on success. Pure side-effect; never
        raises into the request path."""
        metrics.STEALTH_PROXY_FETCH_DURATION.labels(result=result).observe(
            time.monotonic() - started
        )
        metrics.STEALTH_PROXY_FETCH_TOTAL.labels(result=result).inc()
        if body_bytes is not None:
            metrics.STEALTH_PROXY_FETCH_BYTES.observe(body_bytes)

    # -- discovery fetch (challenge-gated sites: warm session + in-page fetch) -- #
    async def browser_fetch(self, provider: str, url: str, user_key: str | None = None) -> dict:
        """GET ``url`` through a warm, challenge-solved session keyed by
        (provider, origin), returning the RAW body (no playlist rewrite). For
        providers whose whole site is challenge-gated (e.g. 9anime DDoS-Guard):
        the in-page fetch clears the challenge a curl/Go client cannot. SSRF is
        gated by the recipe's static ``allowed_hosts``."""
        self._shed_new_work()
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")
        h = host_of(url)
        if not host_allowed(h, recipe.allowed_hosts):
            raise RecipeError(f"fetch host not allowed for {provider}: {h}")

        self._evict_expired()
        origin = _origin_of(url)
        session = await self._warm_fetch_session(provider, origin, user_key)
        session.in_use += 1
        try:
            status, ctype, final, headers, body = await self._in_page_fetch(
                session, url, getattr(recipe, "response_header_allowlist", ())
            )
        except FetchTimeout:
            await self.aclose_session(session.id)
            raise
        finally:
            session.in_use = max(0, session.in_use - 1)

        # The in-page fetch follows redirects; a compromised / open-redirecting
        # upstream could 30x toward an internal host. Re-validate the POST-redirect
        # host against the recipe's static allowlist (mirrors proxy_fetch's
        # redirect guard) so the discovery path can't be used as an SSRF pivot.
        if final and host_of(final) != h and not host_allowed(host_of(final), recipe.allowed_hosts):
            await self.aclose_session(session.id)
            raise RecipeError(
                f"fetch redirect target not allowed for {provider}: {host_of(final)}"
            )

        # A challenge can re-appear mid-session (cookie expiry / new edge): drop
        # the poisoned session and surface ChallengeError so Go fails over.
        if looks_like_challenge(status, body[:4096].decode("utf-8", "ignore")):
            await self.aclose_session(session.id)
            metrics.CHALLENGE_TOTAL.labels(host=h or "?", kind="fetch").inc()
            raise ChallengeError(f"challenge on fetch {h}", host=h, kind="fetch")

        session.expires_at = time.time() + self.cfg.session_ttl_seconds
        return {"status": status, "content_type": ctype, "headers": headers, "body": body}

    async def _click_turnstile(self, page: Any) -> bool:
        """Click the interactive Cloudflare Turnstile checkbox if its challenge
        iframe is present. Mouse events are coordinate-based, so a click at the
        widget's checkbox (near its left edge) passes through the nested
        challenge iframes. Humanized approach (move-then-click) mirrors the live-
        verified spike. Best-effort: returns True iff a click was dispatched."""
        try:
            frames = list(page.frames)
        except Exception:  # noqa: BLE001
            return False
        for fr in frames:
            u = (getattr(fr, "url", "") or "")
            if "challenges.cloudflare.com" not in u and "turnstile" not in u.lower():
                continue
            try:
                el = await fr.frame_element()
                box = await el.bounding_box()
                if not box:
                    continue
                # checkbox sits near the left of the widget (~30px in), vertically centered
                cx = box["x"] + min(33.0, box["width"] / 2.0)
                cy = box["y"] + box["height"] / 2.0
                await page.mouse.move(cx - 60, cy - 25)
                await asyncio.sleep(0.25)
                await page.mouse.move(cx, cy)
                await asyncio.sleep(0.2)
                await page.mouse.click(cx, cy)
                return True
            except Exception:  # noqa: BLE001
                continue
        return False

    async def _log_challenge_dom_snapshot(self, page: Any) -> None:
        """One-time diagnostic capture for a stuck challenge (2026-07-20):
        the 2026-07-18 incident narrowed a 12h/1200+-attempt outage to
        "_click_turnstile finds no matching frame on every poll" but had no
        way to see what the DOM actually looked like — no screenshot/HTML-
        dump capability existed, only a bare Prometheus counter. This logs
        every frame URL plus whether "turnstile" appears anywhere in
        page.content(), so the NEXT time Cloudflare changes the widget's
        embed shape it's diagnosable straight from `docker compose logs`
        instead of requiring a live human debug session. No network calls —
        frames/content are already in memory. Caller gates this to once per
        solve attempt; safe to no-op on any failure (best-effort only)."""
        if not self._log:
            return
        try:
            frame_urls = [(getattr(fr, "url", "") or "")[:200] for fr in page.frames]
        except Exception:  # noqa: BLE001
            frame_urls = []
        has_turnstile_markup = False
        try:
            content = await page.content()
            has_turnstile_markup = "turnstile" in content.lower()
        except Exception:  # noqa: BLE001
            pass
        self._log.warning(
            "challenge dom snapshot frames=%r turnstile_markup_present=%s",
            frame_urls, has_turnstile_markup,
        )

    async def _solve_cf_challenge(self, page: Any, context: Any, origin: str) -> bool:
        """Clear a Cloudflare managed/Turnstile challenge on the already-navigated
        ``page`` AND confirm it yields real content. Returns True only once
        cf_clearance is present AND the page is off the interstitial; False if
        cfg.challenge_solve_timeout_ms elapses (the caller then rotates the exit).

        A bare cf_clearance cookie is NOT sufficient: Cloudflare can issue an
        interim cookie while the interstitial is still looping, and an in-page
        fetch made against that half-solved state 403s (observed live). So once
        clearance is present but the page still looks like a challenge, we RELOAD
        the origin (clearance now rides in the cookie jar) and require the
        reloaded page to be non-challenge before declaring success.

        Camoufox passes the fingerprint check; the only piece a curl/Go client
        cannot do is the interactive checkbox click + the clearance round-trip —
        which is exactly this. Verified live against animepahe.pw."""
        # A persistent/aged warm profile can carry a STALE or interim cf_clearance
        # from a prior attempt. That cookie is poison: it makes has_clearance read
        # True (so the solver would skip the Turnstile click) yet still 403s on the
        # actual fetch. Wipe the cookie jar and re-navigate so every solve starts
        # from a clean, freshly-issued challenge. (The warm profile is leased
        # exclusively for this session, so clearing it is safe.)
        try:
            if context is not None:
                await context.clear_cookies()
                await page.goto(
                    origin, wait_until="domcontentloaded", timeout=self.cfg.nav_timeout_ms
                )
        except Exception:  # noqa: BLE001
            pass

        deadline = time.monotonic() + self.cfg.challenge_solve_timeout_ms / 1000.0
        clicks = 0
        clearance_since = 0.0
        last_reload = 0.0
        dom_snapshot_logged = False
        while time.monotonic() < deadline:
            try:
                title = await page.title()
            except Exception:  # noqa: BLE001
                title = ""
            try:
                cookies = await context.cookies() if context is not None else []
            except Exception:  # noqa: BLE001
                cookies = []
            has_clearance = any(c.get("name") == "cf_clearance" for c in cookies)
            # Solved iff clearance is set AND the page reached real content.
            if has_clearance and title and not looks_like_challenge(None, title):
                return True
            now = time.monotonic()
            if has_clearance and clearance_since == 0.0:
                clearance_since = now
            # Click the interactive Turnstile whenever the page is still a
            # challenge — do NOT gate on cf_clearance (a stale cookie must never
            # suppress the click). _click_turnstile is a no-op when no iframe.
            challenged = (not title) or looks_like_challenge(None, title)
            clicked = False
            if challenged and clicks < self.cfg.challenge_click_max:
                clicked = await self._click_turnstile(page)
                if clicked:
                    clicks += 1
                elif not dom_snapshot_logged:
                    await self._log_challenge_dom_snapshot(page)
                    dom_snapshot_logged = True
            # Clearance present but page stuck on the interstitial with NOTHING to
            # click (interim cookie / no auto-reload): reload to apply the cookie.
            # Only after it's been stuck >6s (let CF's own auto-reload settle
            # first) and throttled, so we never race CF's reload.
            if (
                challenged
                and not clicked
                and has_clearance
                and now - clearance_since > 6.0
                and now - last_reload > 6.0
            ):
                try:
                    await page.goto(
                        origin, wait_until="domcontentloaded", timeout=self.cfg.nav_timeout_ms
                    )
                except Exception:  # noqa: BLE001
                    pass
                last_reload = now
            await asyncio.sleep(1.2)
        if self._log:
            try:
                final_title = await page.title()
            except Exception:  # noqa: BLE001
                final_title = ""
            # The values MUST be interpolated into the message itself (not just
            # passed via `extra=`) — main.py's logging.basicConfig format string
            # is "%(asctime)s %(levelname)s %(message)s", which silently drops
            # any `extra` keys the formatter doesn't reference. A prior version
            # of this fix relied on `extra=` alone and the diagnostic fields
            # never actually reached stdout/docker logs.
            self._log.warning(
                "challenge solve timed out host=%s clicks=%d clearance_obtained=%s "
                "final_title=%r",
                host_of(origin), clicks, clearance_since != 0.0, final_title[:80],
            )
        return False

    def _fetch_lock(self, key: str) -> asyncio.Lock:
        """Lock guarding one (provider,origin) warm-session's get-or-create.

        Without this, two concurrent _warm_fetch_session() calls for the SAME
        key both see no existing session (nothing here awaits before the
        lease), both lease a DISTINCT profile, and the loser's session/profile
        is silently orphaned when the winner's `self._sessions[key] = session`
        overwrites the shared dict slot — no exception, no crash flag, no
        admission-gate counter. That's the "N of pool_size profiles leased,
        unaccounted for" signature that recurred repeatedly in production
        despite three earlier fixes, because those all targeted exception
        paths and this is a success-path race (animepahe 2026-07-10).
        Dict lookup/creation here has no await, so it can't itself race.
        """
        lock = self._fetch_locks.get(key)
        if lock is None:
            lock = asyncio.Lock()
            self._fetch_locks[key] = lock
        return lock

    def _rehydrate_lock(self, sid: str) -> asyncio.Lock:
        """Lock guarding one sid's lazy rehydrate — mirrors ``_fetch_lock()``
        (get-then-create, no await between the check and the store, so it
        can't itself race) rather than ``dict.setdefault(sid, asyncio.Lock())``,
        which always constructs a throwaway ``asyncio.Lock()`` on every call
        (the default-value argument is evaluated eagerly) even when the key
        already exists. See ``_rehydrate`` for why the entry is never popped."""
        lock = self._rehydrate_locks.get(sid)
        if lock is None:
            lock = asyncio.Lock()
            self._rehydrate_locks[sid] = lock
        return lock

    async def _warm_fetch_session(
        self, provider: str, origin: str, user_key: str | None = None
    ) -> Session:
        """Get-or-create a session warmed on ``origin`` (navigates it once to
        solve the site challenge; later fetches reuse its cookies). The warm
        session is keyed by (provider, origin) and SHARED — so it is attributed
        to the FIRST user_key that creates it, and the quota is enforced only
        when a NEW session must be opened (an existing warm reuse is free)."""
        key = f"fetch::{provider}::{origin}"
        async with self._fetch_lock(key):
            existing = self._sessions.get(key)
            if existing is not None and existing.page is not None:
                # Poison-fence the REUSE path: a cheap liveness probe. A poisoned
                # page (Target closed) would otherwise be handed back and re-navved
                # on the next fetch (the AUTO-527 loop). On failure: evict + fall
                # through to recreate a fresh warm session.
                try:
                    await existing.page.evaluate("()=>1")
                    return existing
                except Exception:  # noqa: BLE001
                    # Liveness probe failed: the page (and therefore the browser slot)
                    # is dead. Mirror the poison-fence path EXACTLY (see _in_page_fetch):
                    # aclose_session() only closes the PAGE, so the browser handle/
                    # context survive — if we stop there, profile.launched stays True
                    # and _ensure_browser's launched-guard would hand the reaper back
                    # the DEAD context (mark_healthy, no relaunch). We must ALSO tear
                    # the handle down via _teardown(reason='crash') so launched==False
                    # and _handles[pid] is popped, forcing a real cold relaunch. Order
                    # matches the poison-fence: evict the session, THEN teardown.
                    # _teardown(reason='crash') calls mark_crashed itself, so set the
                    # real reason on the Profile first (don't double-mark).
                    dead = existing.profile
                    dead.last_error = existing.last_error or "liveness-probe: page dead"
                    await self.aclose_session(key)
                    await self._teardown(dead, reason="crash")

            # A NEW warm session is about to be opened — enforce the per-user quota
            # (the reuse path above returns before here, so a shared hit is free).
            self._enforce_user_quota(user_key)

            profile = await self._acquire_profile()
            if profile is None:
                metrics.POOL_EXHAUSTED_TOTAL.inc()
                raise PoolExhausted("no free browser profile (pool/sessions exhausted)")
            recipe = self._recipes.get(provider)
            # solve_challenge providers whose CF gate is unpassable from our
            # datacenter IP (miruro/animepahe) pin to the `warp` exit; the pool
            # fail-opens to direct when warp isn't configured. sticky_key keeps
            # the stream + subsequent fetches for this profile on the same exit.
            proxy = self.pool.select(
                sticky_key=profile.id,
                preferred_type=getattr(recipe, "preferred_proxy_type", None),
            )
            if proxy is None:
                self.profiles.release(profile, ok=False)
                raise RecipeError("no proxy available for fetch warm")

            try:
                # solve_challenge providers (Cloudflare Turnstile, e.g. animepahe) need
                # a CLEAN profile: an aged/pooled profile accumulates CF
                # challenge-platform state (cookies, but also localStorage/IndexedDB on
                # challenges.cloudflare.com that clear_cookies cannot reach
                # cross-origin), and a failed prior attempt POISONS re-solving — the
                # click stops yielding cf_clearance (proven: a fresh profile solves, a
                # copy of the poisoned pool profile does not). Wipe + cold-launch the
                # leased profile so every solve starts from genuinely clean state. The
                # deterministic fingerprint is derived from profile.id (not the
                # on-disk dir), so identity is preserved.
                #
                # This runs INSIDE the try (not before it): it used to sit ahead of
                # this block, so a CancelledError (HTTP client disconnect) from the
                # recycle teardown leaked the just-acquired profile forever — none of
                # the except clauses below could see it. Every solve_challenge
                # provider (currently only animepahe) hit this on EVERY warm fetch,
                # silently draining the shared browser pool.
                if recipe is not None and getattr(recipe, "solve_challenge", False):
                    await self._teardown(profile, reason="recycle")
                    _rm_dir(profile.user_data_dir)

                context = await self._ensure_browser(profile, proxy.id)
                page = await context.new_page()
                resp = await page.goto(
                    origin, wait_until="domcontentloaded", timeout=self.cfg.nav_timeout_ms
                )
                status = resp.status if resp else 0
                try:
                    title = await page.title()
                except Exception:  # noqa: BLE001
                    title = ""
                if looks_like_challenge(status, title):
                    # A recipe may opt into SOLVING a Cloudflare managed/Turnstile
                    # challenge (click the checkbox + poll for cf_clearance) rather
                    # than rotating the exit on the first interstitial. Recipes
                    # without the flag fall straight through to the rotate path —
                    # behavior is unchanged for them.
                    recipe = self._recipes.get(provider)
                    solved = False
                    if recipe is not None and getattr(recipe, "solve_challenge", False):
                        try:
                            solved = await self._solve_cf_challenge(page, context, origin)
                        except Exception:  # noqa: BLE001 — any solve fault ⇒ rotate
                            solved = False
                    if solved:
                        metrics.CHALLENGE_TOTAL.labels(
                            host=host_of(origin) or "?", kind="warm_solved"
                        ).inc()
                    else:
                        await _safe_close_page(page)
                        self.pool.mark_blocked(proxy.id)
                        self.profiles.release(profile, ok=False)
                        metrics.CHALLENGE_TOTAL.labels(host=host_of(origin) or "?", kind="warm").inc()
                        metrics.PROXY_BLOCK_TOTAL.labels(proxy_id=proxy.id).inc()
                        raise ChallengeError(
                            f"challenge warming {origin}", host=host_of(origin), kind="warm"
                        )
                session = Session(
                    id=key, profile=profile, proxy_id=proxy.id, referer=origin,
                    user_agent=profile.user_agent, cdn_host=host_of(origin),
                    master_url=origin, expires_at=time.time() + self.cfg.session_ttl_seconds,
                    page=page, player_url=origin, provider=provider, user_key=user_key,
                )
                self._sessions[key] = session
                metrics.ACTIVE_SESSIONS.set(len(self._sessions))
                return session
            except ChallengeError:
                raise
            except (CapacityExceeded, UserQuotaExceeded, PoolExhausted, ProviderWedged):
                # Typed back-pressure/quota signals (e.g. _ensure_browser → _admit_launch
                # raising CapacityExceeded mid-launch) MUST keep their concrete class so
                # the /fetch handler emits the right `kind` (capacity / user_quota /
                # pool_exhausted / provider_wedged) instead of a flattened error. The
                # browser HANDLE never opened on these paths, so there is no handle to
                # tear down — but the profile LEASE taken at `_acquire_profile()` above
                # already happened and is a separate thing. Without releasing it here,
                # every hard-RAM refusal permanently strands one shared Camoufox pool
                # slot (silent, uncounted by any crash/exception log — this exact class
                # of profile stays "leased" forever with no session to ever free it).
                self.profiles.release(profile, ok=False)
                raise
            except asyncio.CancelledError:
                # HTTP client disconnected while we were inside the browser.
                # CancelledError is BaseException, not Exception — without this
                # handler the profile leaks permanently leased.
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                raise
            except Exception as exc:  # noqa: BLE001
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                raise RecipeError(f"fetch warm failed for {origin}: {exc}") from exc

    async def _in_page_fetch(
        self, session: Session, url: str, header_allowlist: tuple[str, ...] = ()
    ) -> tuple[int, str, str, dict[str, str], bytes]:
        """Run ``fetch(url)`` inside the session's live page and marshal the
        response back as (status, content_type, final_url, headers, bytes).
        ``headers`` carries the ``header_allowlist`` response headers (per-recipe;
        empty default ⇒ none, e.g. the /hls proxy path). Body is encoded via
        FileReader/base64 (NOT typed-array + btoa, which trips Camoufox's xray
        wrapper).

        Poison-fence (Phase 1): a "Target closed" / "context was destroyed" /
        "navigation" error means the page is dead. We DO NOT re-navigate and
        retry the same page (that is the AUTO-527 poison loop — every retry
        burns a pool slot). Instead we count the crash on the session; below
        cfg.poison_max we surface the error so the caller fails over once; AT
        poison_max we tear the profile down (mark it crashed for the reaper) and
        raise ProviderWedged so the Go breaker can trip the provider."""
        page = session.page
        if page is None:
            raise SessionGone(session.id)
        try:
            raw = await self._evaluate_fetch(page, url, header_allowlist)
        except asyncio.TimeoutError as exc:
            raise FetchTimeout(session.id) from exc
        except Exception as exc:  # noqa: BLE001
            msg = str(exc)
            if (
                "context was destroyed" in msg
                or "Target closed" in msg
                or "navigation" in msg
            ):
                session.crash_count += 1
                session.last_error = msg
                if session.crash_count >= self.cfg.poison_max:
                    profile = session.profile
                    # _teardown(reason='crash') marks the slot crashed with
                    # error=profile.last_error, but the real crash message lives
                    # on the SESSION — thread it onto the Profile so health()'s
                    # crashed-slot line carries the actual reason, not a blank.
                    profile.last_error = session.last_error or msg
                    await self.aclose_session(session.id)
                    await self._teardown(profile, reason="crash")
                    raise ProviderWedged(
                        f"warm session poisoned ({session.crash_count} strikes): {msg}",
                        provider=session.provider,
                    ) from exc
                # Below the limit: surface the crash so the caller fails over
                # ONCE; the session stays for one more strike (no nav-retry).
                raise
            raise
        status_s, ctype, final_url, hdrs_enc, b64 = raw.split("|", 4)
        if b64 == _TOO_LARGE:
            raise FetchTooLarge(
                f"upstream body exceeds cap ({self.cfg.max_body_bytes} bytes): {host_of(url)}"
            )
        headers: dict[str, str] = {}
        if hdrs_enc:
            try:
                parsed = json.loads(unquote(hdrs_enc))
                if isinstance(parsed, dict):
                    headers = {str(k): str(v) for k, v in parsed.items()}
            except (ValueError, TypeError):
                headers = {}  # malformed header blob is non-fatal — body still returns
        body = base64.b64decode(b64) if b64 else b""
        return int(status_s), ctype, final_url, headers, body

    async def _evaluate_fetch(
        self, page: Any, url: str, header_allowlist: tuple[str, ...] = ()
    ) -> str:
        """Run the in-page fetch JS with a hard timeout. page.evaluate has no
        built-in timeout, so an unbounded ``await fetch(url)`` against a stalled
        CDN would otherwise pin this browser slot forever (pool exhaustion)."""
        return await asyncio.wait_for(
            page.evaluate(_in_page_fetch_js(self.cfg.max_body_bytes, header_allowlist), url),
            timeout=self.cfg.fetch_timeout_ms / 1000.0,
        )

    async def aclose_session(self, sid: str) -> bool:
        session = self._sessions.pop(sid, None)
        if session is None:
            return False
        self.store.delete(sid)
        await _safe_close_page(session.page)
        self.profiles.release(session.profile, ok=True)
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        return True

    def _evict_expired(self) -> None:
        """Drop expired, NOT-in-use sessions (cheap request-path sweep). The page
        close is scheduled via a retained task (get_running_loop, not the
        deprecated get_event_loop); the background reaper also closes pages, so a
        missed schedule here can't strand one."""
        now = time.time()
        for sid, session in list(self._sessions.items()):
            if session.expires_at <= now and session.in_use <= 0:
                self._sessions.pop(sid, None)
                self.store.delete(sid)
                self._spawn(_safe_close_page(session.page))
                self.profiles.release(session.profile, ok=True)
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))

    def _spawn(self, coro) -> None:
        """Fire-and-forget a coroutine, retaining the task so it isn't GC'd
        before it runs. No running loop ⇒ drop (the reaper handles cleanup)."""
        try:
            loop = asyncio.get_running_loop()
        except RuntimeError:
            return
        task = loop.create_task(coro)
        self._bg_tasks.add(task)
        task.add_done_callback(self._bg_tasks.discard)

    async def _reaper_loop(self) -> None:
        """Background sweeper: reclaim expired/abandoned sessions and retire
        over-used profiles WITHOUT waiting for the next request, so an idle gap
        after a burst can't leave the pool wedged."""
        while True:
            try:
                await asyncio.sleep(self.cfg.reaper_interval_seconds)
                await self._reap()
            except asyncio.CancelledError:
                break
            except Exception:  # noqa: BLE001
                if self._log:
                    self._log.exception("reaper tick failed")

    async def _reap(self) -> None:
        now = time.time()
        for sid, session in list(self._sessions.items()):
            if session.expires_at <= now and session.in_use <= 0:
                self._sessions.pop(sid, None)
                await _safe_close_page(session.page)
                self.profiles.release(session.profile, ok=True)
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        # Drop persisted records of sessions that died with a previous process
        # (crash/redeploy without a clean aclose_session/_evict_* pass) so
        # they don't accumulate forever on the volume.
        self.store.sweep(time.time())
        # Retire over-used, unleased profiles: tear the browser down and clear
        # the on-disk user_data_dir so cookie/cache cruft can't grow unbounded.
        for p in self.profiles.all():
            if not p.leased and self.profiles.needs_retire(p):
                await self._teardown(p, reason="recycle")
                _rm_dir(p.user_data_dir)
                self.profiles.reset_uses(p)
        # Resurrect crashed, not-in-use slots (background self-heal — no
        # container restart). Each is gated by its own exponential backoff.
        for p in self.profiles.crashed_idle():
            await self._resurrect_crashed_slot(p)
        self._publish_pool_gauges()

    def _resurrect_backoff(self, consecutive_fail: int) -> float:
        """Exponential per-slot backoff: base * 2**fail, capped. fail==0 -> base;
        fail==1 -> 2*base; fail==2 -> 4*base; ... clamped to
        resurrect_backoff_cap_seconds. (1 -> 2 -> 4 -> 8 -> 16 -> 30 with
        base=1, cap=30 — the canonical 1→2→4→8→16→30s curve.)"""
        base = self.cfg.resurrect_backoff_base_seconds
        cap = self.cfg.resurrect_backoff_cap_seconds
        steps = max(0, consecutive_fail)
        return min(cap, base * (2 ** steps))

    async def _resurrect_crashed_slot(self, profile: Profile) -> None:
        """Attempt a cold relaunch of one crashed, not-in-use slot. Skips slots
        still inside their backoff window. On success the slot returns to the
        healthy pool; on the cfg.resurrect_max_fails-th consecutive failed
        relaunch the slot is retired (user_data_dir wiped, counters reset)
        rather than revived forever. Never raises — a failed relaunch is counted
        toward retirement and the reaper loop must not die."""
        if profile.leased or profile.status != "crashed":
            return
        now = time.time()
        if profile.next_resurrect_at and now < profile.next_resurrect_at:
            return
        # Retire BEFORE attempting if we've already exhausted the budget.
        if profile.consecutive_fail >= self.cfg.resurrect_max_fails:
            await self._retire_crashed_slot(profile)
            return

        profile.status = "warming"
        proxy = self.pool.select(sticky_key=profile.id)
        proxy_id = proxy.id if proxy is not None else (profile.proxy_id or "")
        try:
            await self._ensure_browser(profile, proxy_id)
            self.profiles.mark_healthy(profile)
            metrics.SLOT_RESURRECT_TOTAL.labels(result="ok").inc()
            if self._log:
                self._log.info("resurrected crashed slot %s", profile.id)
        except Exception as exc:  # noqa: BLE001
            # Failed: re-mark crashed (bumps consecutive_fail toward the
            # retire-after-N limit — this is the per-failed-relaunch counter),
            # arm the next exponential backoff, and retire if we've hit the cap.
            profile.last_error = str(exc)
            await self._teardown(profile, reason="crash")  # mark_crashed -> fail+1
            profile.next_resurrect_at = time.time() + self._resurrect_backoff(
                profile.consecutive_fail
            )
            metrics.SLOT_RESURRECT_TOTAL.labels(result="fail").inc()
            if self._log:
                self._log.warning("resurrect failed for %s: %s", profile.id, exc)
            if profile.consecutive_fail >= self.cfg.resurrect_max_fails:
                await self._retire_crashed_slot(profile)

    async def _retire_crashed_slot(self, profile: Profile) -> None:
        """Give up on a slot: wipe its on-disk profile, zero its counters, and
        return it to the healthy pool as a fresh identity (a future lease cold-
        launches it)."""
        await self._teardown(profile, reason="recycle")
        _rm_dir(profile.user_data_dir)
        self.profiles.reset_uses(profile)
        self.profiles.mark_healthy(profile)
        metrics.SLOT_RESURRECT_TOTAL.labels(result="retired").inc()
        if self._log:
            self._log.warning("retired crashed slot %s after %d failed resurrects",
                              profile.id, self.cfg.resurrect_max_fails)

    def _publish_pool_gauges(self) -> None:
        counts = self.profiles.status_counts()
        free = sum(1 for p in self.profiles.all()
                   if p.status == "healthy" and not p.leased)
        metrics.POOL_FREE.set(free)
        metrics.POOL_CRASHED.set(counts.get("crashed", 0))

    def session_state(self, sid: str) -> str:
        """Liveness for the scraper's cached-stream gate: ``alive`` (registered
        live session), ``rehydratable`` (valid persisted record — a fetch will
        lazily rebuild it), ``gone`` (nothing usable; the caller must
        re-resolve). Cheap: dict lookup + at most one small JSON read."""
        session = self._sessions.get(sid)
        if session is not None and session.expires_at > time.time():
            return "alive"
        rec = self.store.load(sid)
        if (
            rec is not None
            and rec.get("camoufox_build") == camoufox_build()
            and rec.get("expires_at", 0) > time.time()
        ):
            return "rehydratable"
        return "gone"


class SessionGone(Exception):
    def __init__(self, sid: str):
        super().__init__(f"unknown or expired session: {sid}")
        self.sid = sid


class PoolExhausted(RecipeError):
    """Every browser profile is leased (pool saturated). Distinct from a recipe
    failure so the API can map it to 503 (retryable ⇒ the Go orchestrator fails
    over) and a dedicated metric can alert on saturation."""


class CapacityExceeded(RecipeError):
    """A new browser launch was refused because the combined Camoufox RSS is at
    or above the hard RAM budget. 503 (retryable) so the Go orchestrator fails
    over; the LRU not-in-use session is evicted to reclaim headroom."""

    kind = "capacity"


class UserQuotaExceeded(RecipeError):
    """The requesting user_key already holds >= STEALTH_USER_QUOTA sessions.
    503 (retryable); bounds a single user's pool footprint so one viewer cannot
    starve the shared pool."""

    kind = "user_quota"


class ProviderWedged(RecipeError):
    """A warm session for a provider has poisoned itself (>= cfg.poison_max
    in-page-fetch crashes). The profile is torn down + marked crashed for the
    reaper; the caller fails over. Distinct from a plain RecipeError so the API
    maps it to 503 {kind:"provider_wedged"} and the Go breaker can attribute the
    wedge to a provider (it carries .provider)."""

    def __init__(self, message: str, provider: str = ""):
        super().__init__(message)
        self.provider = provider


class DegradedShed(RecipeError):
    """A NEW resolve/fetch was refused because the host is at Critical
    degradation pressure (governor level 2, graceful-degradation Phase 3).
    503 (retryable) with kind="degraded"; the Go breaker parks the provider and
    half-open-retries once pressure clears. In-flight sessions are untouched —
    active playback is exactly what the shedding protects."""

    kind = "degraded"


class FetchTimeout(Exception):
    """The in-page /hls fetch exceeded cfg.fetch_timeout_ms. The caller tears the
    session down (reclaiming the wedged browser slot); mapped to 504."""

    def __init__(self, sid: str):
        super().__init__(f"in-page fetch timeout: {sid}")
        self.sid = sid


class FetchTooLarge(RecipeError):
    """The upstream body exceeded cfg.max_body_bytes (the over-cap path). A
    RecipeError subclass so every existing ``except RecipeError`` handler treats
    it identically — only the proxy_fetch metrics path tells it apart (so the
    ``too_large`` result is countable separately from other recipe failures)."""


def _rm_dir(path: str) -> None:
    import shutil

    try:
        shutil.rmtree(path, ignore_errors=True)
    except Exception:  # noqa: BLE001
        pass


async def host_allowed_for_session(url: str, session: Session, resolve=None) -> bool:
    """SSRF guard for the /hls proxy. The child URLs come from playlists the
    controlled browser fetched, so the host is dynamic/rotating (mewstream.buzz
    master → flarestorm.buzz segments) and unpredictable — we allow any PUBLIC
    https host but reject anything that resolves to a private / loopback /
    link-local / reserved address, so a poisoned upstream playlist cannot point
    the in-page fetch at an internal service.

    Hardened (2026-06-21): https on EVERY path; DNS resolution drives the
    private-address rejection (closing the octal/hex/decimal-IP, DNS-rebind, and
    IPv4-mapped-IPv6 bypasses the old textual check let through). ``resolve`` is
    an injectable async resolver (host -> list[ip str]) for tests; the verdict is
    cached per session so active playback doesn't re-resolve each segment."""
    from urllib.parse import urlsplit

    if urlsplit(url).scheme != "https":
        return False
    h = host_of(url)  # lowercased hostname, no port, IPv6 brackets stripped
    if not h:
        return False
    if h in ("localhost", "stealth-scraper") or h.endswith(".local"):
        return False
    # Must be a routable target shape: a dotted hostname or an IP literal — a
    # bare single-label name (e.g. a Docker service: redis, catalog) is blocked.
    if "." not in h and _parse_ip(h) is None:
        return False

    cache = session.host_acl_cache
    if h in cache:
        return cache[h]
    ok = await _resolves_public_only(h, resolve or _default_resolve)
    cache[h] = ok
    return ok


async def _resolves_public_only(host: str, resolve) -> bool:
    """True iff host is (or every resolved address of host is) a public IP."""
    ip = _parse_ip(host)
    if ip is not None:
        return _is_public_ip(ip)
    try:
        addrs = await resolve(host)
    except Exception:  # noqa: BLE001 — resolution failure ⇒ deny
        return False
    parsed = [p for p in (_parse_ip(a) for a in addrs) if p is not None]
    if not parsed:
        return False
    return all(_is_public_ip(p) for p in parsed)


async def _default_resolve(host: str) -> list[str]:
    import socket

    loop = asyncio.get_running_loop()
    infos = await loop.getaddrinfo(host, None, type=socket.SOCK_STREAM)
    return [info[4][0] for info in infos]


def _parse_ip(s: str):
    import ipaddress

    try:
        return ipaddress.ip_address(s)
    except ValueError:
        return None


def _is_public_ip(ip) -> bool:
    # Unwrap IPv4-mapped IPv6 (::ffff:127.0.0.1) so the v4 classification applies.
    mapped = getattr(ip, "ipv4_mapped", None)
    if mapped is not None:
        ip = mapped
    return not (
        ip.is_private
        or ip.is_loopback
        or ip.is_link_local
        or ip.is_reserved
        or ip.is_multicast
        or ip.is_unspecified
    )


async def _safe_close_page(page: Any) -> None:
    if page is None:
        return
    try:
        await page.close()
    except Exception:  # noqa: BLE001
        pass


def _q(url: str) -> str:
    from urllib.parse import quote

    return quote(url, safe="")


def _origin_of(url: str) -> str:
    from urllib.parse import urlsplit

    p = urlsplit(url)
    return f"{p.scheme}://{p.netloc}"
