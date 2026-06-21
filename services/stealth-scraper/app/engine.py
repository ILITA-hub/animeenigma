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

# Fetch a URL inside the page and return "status|content-type|base64body".
# Uses FileReader/blob for base64 (manual Uint8Array + btoa trips Camoufox's
# xray security wrapper → "Permission denied to access property constructor").
# Takes a single STRING arg (object args also trip the wrapper).
_IN_PAGE_FETCH_JS = """async (url) => {
  const r = await fetch(url);
  const ct = r.headers.get('content-type') || '';
  const blob = await r.blob();
  const b64 = await new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onloadend = () => resolve((fr.result + '').split(',')[1] || '');
    fr.onerror = () => reject(new Error('filereader'));
    fr.readAsDataURL(blob);
  });
  return r.status + '|' + ct + '|' + (r.url || '') + '|' + b64;
}"""

from . import metrics
from .config import Config
from .fingerprint import build_launch_options, proxy_to_playwright
from .profiles import Profile, ProfileManager
from .recipes import ChallengeError, NotFoundError, Recipe, RecipeContext, RecipeError
from .recipes.base import host_of
from .recipes.gogoanime import GogoanimeRecipe
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


class CamoufoxEngine:
    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.pool: ProxyPool = build_pool_from_config(cfg)
        self.profiles = ProfileManager(cfg.profile_dir, cfg.pool_size)
        self._handles: dict[str, _CamoufoxHandle] = {}     # profile id -> handle
        self._sessions: dict[str, Session] = {}            # session id -> Session
        self._recipes: dict[str, Recipe] = {"gogoanime": GogoanimeRecipe()}
        self._log: Any = None
        # Async DNS resolver (host -> list[ip str]); injectable in tests so the
        # SSRF guard is exercised without real network resolution.
        self._resolve_host = _default_resolve

    def set_logger(self, log: Any) -> None:
        self._log = log

    # -- lifecycle ---------------------------------------------------------- #
    async def start(self) -> None:
        os.makedirs(self.cfg.profile_dir, exist_ok=True)
        metrics.BROWSER_POOL_SIZE.set(0)

    async def stop(self) -> None:
        self._sessions.clear()
        for pid, handle in list(self._handles.items()):
            try:
                await handle.close()
            except Exception:  # noqa: BLE001
                pass
            self._handles.pop(pid, None)
        metrics.BROWSER_POOL_SIZE.set(0)

    def health(self) -> dict:
        return {
            "status": "ok",
            "pool_size": self.cfg.pool_size,
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
        metrics.BROWSER_POOL_SIZE.set(len(self._handles))
        metrics.BROWSER_RELAUNCH_TOTAL.labels(reason=reason).inc()

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
                raise RecipeError("no free browser profile (pool/sessions exhausted)")

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
            expires_at=time.time() + self.cfg.session_ttl_seconds,
            page=page,
            player_url=player_url,
        )
        self._sessions[sid] = session
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
            "expires_at": int(session.expires_at),
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

        status, ctype, final_url, body = await self._in_page_fetch(session, url)

        # The in-page fetch follows redirects; a trusted CDN could 30x toward an
        # internal target. Re-validate the post-redirect URL before returning its
        # body (closes the redirect bypass of the SSRF guard).
        if (
            final_url
            and final_url != url
            and not await host_allowed_for_session(final_url, session, self._resolve_host)
        ):
            raise RecipeError(f"proxy redirect target not allowed: {host_of(final_url)}")

        # Bump TTL on activity (sliding window) so an actively-watched stream
        # isn't evicted mid-playback.
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

    async def _in_page_fetch(self, session: Session, url: str) -> tuple[int, str, str, bytes]:
        """Run ``fetch(url)`` inside the session's live page and marshal the
        response back as bytes. Encodes via FileReader/base64 (NOT typed-array +
        btoa, which trips Camoufox's xray wrapper). Re-navigates the page to the
        player origin and retries once if the execution context was destroyed
        (the player page occasionally self-navigates during a long watch)."""
        page = session.page
        if page is None:
            raise SessionGone(session.id)
        try:
            raw = await page.evaluate(_IN_PAGE_FETCH_JS, url)
        except Exception as exc:  # noqa: BLE001
            msg = str(exc)
            if "context was destroyed" in msg or "Target closed" in msg or "navigation" in msg:
                if not session.player_url:
                    raise SessionGone(session.id) from exc
                await page.goto(
                    session.player_url, referer=session.referer or None,
                    wait_until="domcontentloaded", timeout=self.cfg.nav_timeout_ms,
                )
                raw = await page.evaluate(_IN_PAGE_FETCH_JS, url)
            else:
                raise
        status_s, ctype, final_url, b64 = raw.split("|", 3)
        body = base64.b64decode(b64) if b64 else b""
        return int(status_s), ctype, final_url, body

    async def aclose_session(self, sid: str) -> bool:
        session = self._sessions.pop(sid, None)
        if session is None:
            return False
        await _safe_close_page(session.page)
        self.profiles.release(session.profile, ok=True)
        return True

    def close_session(self, sid: str) -> bool:
        """Sync drop (page closed lazily on next evict). Prefer aclose_session."""
        session = self._sessions.pop(sid, None)
        if session is None:
            return False
        self.profiles.release(session.profile, ok=True)
        return True

    def _evict_expired(self) -> None:
        now = time.time()
        for sid, session in list(self._sessions.items()):
            if session.expires_at <= now:
                self._sessions.pop(sid, None)
                # Fire-and-forget page close (sync context can't await).
                page = session.page
                if page is not None:
                    try:
                        asyncio.get_event_loop().create_task(_safe_close_page(page))
                    except Exception:  # noqa: BLE001
                        pass
                self.profiles.release(session.profile, ok=True)


class SessionGone(Exception):
    def __init__(self, sid: str):
        super().__init__(f"unknown or expired session: {sid}")
        self.sid = sid


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
