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
import os
import time
import uuid
from dataclasses import dataclass, field
from typing import Any

def _in_page_fetch_js(max_bytes: int) -> str:
    """Build the in-page fetch JS, returning "status|content-type|final-url|
    base64body" (or "…|__TOO_LARGE__" when the body exceeds ``max_bytes``).

    Encodes via FileReader/blob for base64 (manual Uint8Array + btoa trips
    Camoufox's xray wrapper → "Permission denied to access property
    constructor"). Takes a single STRING arg (object args also trip the wrapper).
    The size cap is baked in (max_bytes is a trusted int) — checked via
    Content-Length first, then the realized blob size — so an oversized upstream
    body can't be base64'd into memory (a DoS amplified ~3x: blob + b64 + decode)."""
    return (
        "async (url) => {\n"
        "  const r = await fetch(url);\n"
        "  const ct = r.headers.get('content-type') || '';\n"
        "  const head = r.status + '|' + ct + '|' + (r.url || '') + '|';\n"
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
from .recipes import ChallengeError, NotFoundError, Recipe, RecipeContext, RecipeError
from .recipes.base import host_allowed, host_of, looks_like_challenge
from .recipes.gogoanime import GogoanimeRecipe
from .recipes.nineanime import NineAnimeRecipe
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
        self._handles: dict[str, _CamoufoxHandle] = {}     # profile id -> handle
        self._sessions: dict[str, Session] = {}            # session id -> Session
        self._recipes: dict[str, Recipe] = {
            "gogoanime": GogoanimeRecipe(),
            "nineanime": NineAnimeRecipe(),
        }
        self._log: Any = None
        # Async DNS resolver (host -> list[ip str]); injectable in tests so the
        # SSRF guard is exercised without real network resolution.
        self._resolve_host = _default_resolve
        # Retained references for fire-and-forget page-close tasks (so they are
        # not GC'd before they run) + the background reaper handle.
        self._bg_tasks: set = set()
        self._reaper_task: Any = None

    def set_logger(self, log: Any) -> None:
        self._log = log

    # -- lifecycle ---------------------------------------------------------- #
    async def start(self) -> None:
        os.makedirs(self.cfg.profile_dir, exist_ok=True)
        metrics.BROWSER_POOL_SIZE.set(0)
        metrics.ACTIVE_SESSIONS.set(0)
        self._reaper_task = asyncio.create_task(self._reaper_loop())

    async def stop(self) -> None:
        if self._reaper_task is not None:
            self._reaper_task.cancel()
            self._reaper_task = None
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
        # degraded = every profile is leased (pool saturated). Surfaced for
        # monitoring; /healthz still returns 200 so Docker does NOT restart-loop
        # the container on transient saturation (the fetch timeout + reaper +
        # 503-on-exhaustion self-heal the pool instead).
        free = sum(1 for p in self.profiles.all() if not p.leased)
        return {
            "status": "degraded" if free == 0 else "ok",
            "pool_size": self.cfg.pool_size,
            "free_profiles": free,
            "live_browsers": len(self._handles),
            "active_sessions": len(self._sessions),
            "proxies": [
                {"id": e.id, "type": e.type, "blocked": e.total_blocked}
                for e in self.pool.all()
            ],
        }

    # -- launch / teardown -------------------------------------------------- #
    async def _ensure_browser(self, profile: Profile, proxy_id: str) -> Any:
        if profile.launched and profile.proxy_id == proxy_id:
            return profile.context
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
            if self.cfg.warming_enabled:
                from .warming import warm_profile

                await warm_profile(
                    page, self.cfg.warming_sites, self._log,
                    nav_timeout_ms=self.cfg.nav_timeout_ms,
                )
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
    async def resolve(self, provider: str, params: dict) -> dict:
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")

        self._evict_expired()
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
                session = await self._open_session(partial, context, proxy.id, profile, page)
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

    async def _open_session(
        self, partial: dict, context: Any, proxy_id: str, profile: Profile, page: Any
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
        )
        self._sessions[sid] = session
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        return session

    def _session_payload(self, session: Session, partial: dict) -> dict:
        return {
            "session_id": session.id,
            "master_url": session.master_url,
            # Path the caller proxies the stream through (host-prefixed by caller).
            "playlist_proxy_path": f"/hls?sid={session.id}&url={_q(session.master_url)}",
            "referer": session.referer,
            "subtitles": partial.get("subtitles", []),
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
            raise SessionGone(sid)
        if not await host_allowed_for_session(url, session, self._resolve_host):
            raise RecipeError(f"proxy host not allowed for session: {host_of(url)}")

        # Slide the TTL BEFORE the (possibly long) fetch and mark the session
        # in-use, so a concurrent eviction can't close the page mid-fetch
        # (use-after-close) and an actively-fetched session can't lapse its
        # deadline while the network round-trip is in flight.
        session.expires_at = time.time() + self.cfg.session_ttl_seconds
        session.in_use += 1
        try:
            status, ctype, final_url, body = await self._in_page_fetch(session, url)
        except FetchTimeout:
            # A hung fetch would pin this browser slot forever — reclaim it.
            await self.aclose_session(sid)
            raise
        finally:
            session.in_use = max(0, session.in_use - 1)

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

    # -- discovery fetch (challenge-gated sites: warm session + in-page fetch) -- #
    async def browser_fetch(self, provider: str, url: str) -> dict:
        """GET ``url`` through a warm, challenge-solved session keyed by
        (provider, origin), returning the RAW body (no playlist rewrite). For
        providers whose whole site is challenge-gated (e.g. 9anime DDoS-Guard):
        the in-page fetch clears the challenge a curl/Go client cannot. SSRF is
        gated by the recipe's static ``allowed_hosts``."""
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")
        h = host_of(url)
        if not host_allowed(h, recipe.allowed_hosts):
            raise RecipeError(f"fetch host not allowed for {provider}: {h}")

        self._evict_expired()
        origin = _origin_of(url)
        session = await self._warm_fetch_session(provider, origin)
        session.in_use += 1
        try:
            status, ctype, _final, body = await self._in_page_fetch(session, url)
        except FetchTimeout:
            await self.aclose_session(session.id)
            raise
        finally:
            session.in_use = max(0, session.in_use - 1)

        # A challenge can re-appear mid-session (cookie expiry / new edge): drop
        # the poisoned session and surface ChallengeError so Go fails over.
        if looks_like_challenge(status, body[:4096].decode("utf-8", "ignore")):
            await self.aclose_session(session.id)
            metrics.CHALLENGE_TOTAL.labels(host=h or "?", kind="fetch").inc()
            raise ChallengeError(f"challenge on fetch {h}", host=h, kind="fetch")

        session.expires_at = time.time() + self.cfg.session_ttl_seconds
        return {"status": status, "content_type": ctype, "body": body}

    async def _warm_fetch_session(self, provider: str, origin: str) -> Session:
        """Get-or-create a session warmed on ``origin`` (navigates it once to
        solve the site challenge; later fetches reuse its cookies)."""
        key = f"fetch::{provider}::{origin}"
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
                await self.aclose_session(key)

        profile = await self._acquire_profile()
        if profile is None:
            metrics.POOL_EXHAUSTED_TOTAL.inc()
            raise PoolExhausted("no free browser profile (pool/sessions exhausted)")
        proxy = self.pool.select(sticky_key=profile.id)
        if proxy is None:
            self.profiles.release(profile, ok=False)
            raise RecipeError("no proxy available for fetch warm")

        try:
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
                page=page, player_url=origin, provider=provider,
            )
            self._sessions[key] = session
            metrics.ACTIVE_SESSIONS.set(len(self._sessions))
            return session
        except ChallengeError:
            raise
        except Exception as exc:  # noqa: BLE001
            await self._teardown(profile, reason="crash")
            self.profiles.release(profile, ok=False)
            raise RecipeError(f"fetch warm failed for {origin}: {exc}") from exc

    async def _in_page_fetch(self, session: Session, url: str) -> tuple[int, str, str, bytes]:
        """Run ``fetch(url)`` inside the session's live page and marshal the
        response back as bytes. Encodes via FileReader/base64 (NOT typed-array +
        btoa, which trips Camoufox's xray wrapper).

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
            raw = await self._evaluate_fetch(page, url)
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
        status_s, ctype, final_url, b64 = raw.split("|", 3)
        if b64 == _TOO_LARGE:
            raise RecipeError(
                f"upstream body exceeds cap ({self.cfg.max_body_bytes} bytes): {host_of(url)}"
            )
        body = base64.b64decode(b64) if b64 else b""
        return int(status_s), ctype, final_url, body

    async def _evaluate_fetch(self, page: Any, url: str) -> str:
        """Run the in-page fetch JS with a hard timeout. page.evaluate has no
        built-in timeout, so an unbounded ``await fetch(url)`` against a stalled
        CDN would otherwise pin this browser slot forever (pool exhaustion)."""
        return await asyncio.wait_for(
            page.evaluate(_in_page_fetch_js(self.cfg.max_body_bytes), url),
            timeout=self.cfg.fetch_timeout_ms / 1000.0,
        )

    async def aclose_session(self, sid: str) -> bool:
        session = self._sessions.pop(sid, None)
        if session is None:
            return False
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


class SessionGone(Exception):
    def __init__(self, sid: str):
        super().__init__(f"unknown or expired session: {sid}")
        self.sid = sid


class PoolExhausted(RecipeError):
    """Every browser profile is leased (pool saturated). Distinct from a recipe
    failure so the API can map it to 503 (retryable ⇒ the Go orchestrator fails
    over) and a dedicated metric can alert on saturation."""


class ProviderWedged(RecipeError):
    """A warm session for a provider has poisoned itself (>= cfg.poison_max
    in-page-fetch crashes). The profile is torn down + marked crashed for the
    reaper; the caller fails over. Distinct from a plain RecipeError so the API
    maps it to 503 {kind:"provider_wedged"} and the Go breaker can attribute the
    wedge to a provider (it carries .provider)."""

    def __init__(self, message: str, provider: str = ""):
        super().__init__(message)
        self.provider = provider


class FetchTimeout(Exception):
    """The in-page /hls fetch exceeded cfg.fetch_timeout_ms. The caller tears the
    session down (reclaiming the wedged browser slot); mapped to 504."""

    def __init__(self, sid: str):
        super().__init__(f"in-page fetch timeout: {sid}")
        self.sid = sid


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
