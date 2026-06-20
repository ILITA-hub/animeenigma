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
import os
import time
import uuid
from dataclasses import dataclass
from typing import Any

from . import metrics
from .config import Config
from .fingerprint import build_launch_options, proxy_to_playwright
from .profiles import Profile, ProfileManager
from .recipes import ChallengeError, NotFoundError, Recipe, RecipeContext, RecipeError
from .recipes.base import host_of
from .recipes.gogoanime import GogoanimeRecipe
from .streamproxy import looks_like_m3u8, make_wrap, rewrite_playlist
from .tunnels import ProxyPool, build_pool_from_config

_ROUTE_BLOCK = {"font", "media"}


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
        async def _route(route: Any) -> None:
            try:
                if route.request.resource_type in _ROUTE_BLOCK:
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

            try:
                context = await self._ensure_browser(profile, proxy.id)
                page = await context.new_page()
                try:
                    rc = RecipeContext(
                        page=page, context=context, params=params, cfg=self.cfg,
                        log=self._log, proxy_id=proxy.id, user_agent=profile.user_agent,
                    )
                    partial = await asyncio.wait_for(
                        recipe.resolve(rc),
                        timeout=self.cfg.resolve_timeout_ms / 1000.0,
                    )
                finally:
                    try:
                        await page.close()
                    except Exception:  # noqa: BLE001
                        pass

                session = await self._open_session(partial, context, proxy.id, profile)
                self.pool.mark_ok(proxy.id)
                # NB: profile is RETAINED by the session (NOT released) so the
                # stream proxy can reuse its clearance-bearing context.
                metrics.RESOLVE_TOTAL.labels(provider=provider, result="ok").inc()
                metrics.RESOLVE_DURATION.labels(provider=provider).observe(
                    time.monotonic() - started
                )
                return self._session_payload(session, partial)

            except ChallengeError as exc:
                last_err = exc
                metrics.CHALLENGE_TOTAL.labels(host=exc.host or "?", kind=exc.kind).inc()
                metrics.PROXY_BLOCK_TOTAL.labels(proxy_id=proxy.id).inc()
                self.pool.mark_blocked(proxy.id)
                tried.add(proxy.id)
                await self._teardown(profile, reason="rotate")
                self.profiles.release(profile, ok=False)
                continue

            except (NotFoundError, RecipeError) as exc:
                last_err = exc
                self.profiles.release(profile, ok=False)
                result = "not_found" if isinstance(exc, NotFoundError) else "error"
                metrics.RESOLVE_TOTAL.labels(provider=provider, result=result).inc()
                raise

            except asyncio.TimeoutError as exc:
                last_err = exc
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                tried.add(proxy.id)
                continue

            except Exception as exc:  # noqa: BLE001
                last_err = exc
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                tried.add(proxy.id)
                continue

        metrics.RESOLVE_TOTAL.labels(provider=provider, result="challenge").inc()
        raise ChallengeError(
            f"exhausted {len(tried)} exit(s); last: {last_err}", kind="exhausted"
        )

    async def _acquire_profile(self, retries: int = 50) -> Profile | None:
        for _ in range(retries):
            p = self.profiles.lease()
            if p is not None:
                return p
            await asyncio.sleep(0.1)
        return None

    async def _open_session(
        self, partial: dict, context: Any, proxy_id: str, profile: Profile
    ) -> Session:
        master = partial.get("master_url")
        cookies: list[dict] = []
        try:
            for c in (await context.cookies(master) if master else []):
                cookies.append(
                    {
                        "name": c.get("name"), "value": c.get("value"),
                        "domain": c.get("domain"), "path": c.get("path", "/"),
                    }
                )
        except Exception:  # noqa: BLE001
            cookies = []
        partial["cookies"] = cookies

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

    # -- stream proxy (mandatory) ------------------------------------------ #
    async def proxy_fetch(self, sid: str, url: str) -> dict:
        """Fetch ``url`` through the session's clearance-bearing browser context
        (same exit IP + cookies + TLS). Rewrites playlists so child URIs route
        back through this proxy. Returns {status, content_type, body(bytes)}."""
        self._evict_expired()
        session = self._sessions.get(sid)
        if session is None:
            raise SessionGone(sid)
        if not host_allowed_for_session(url, session):
            raise RecipeError(f"proxy host not allowed for session: {host_of(url)}")

        ctx = session.profile.context
        if ctx is None:
            raise SessionGone(sid)
        resp = await ctx.request.get(
            url, headers={"Referer": session.referer} if session.referer else {}
        )
        status = resp.status
        ctype = (resp.headers or {}).get("content-type", "")
        body = await resp.body()

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

    def close_session(self, sid: str) -> bool:
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
                self.profiles.release(session.profile, ok=True)


class SessionGone(Exception):
    def __init__(self, sid: str):
        super().__init__(f"unknown or expired session: {sid}")
        self.sid = sid


def host_allowed_for_session(url: str, session: Session) -> bool:
    """A session may only proxy URLs on the recipe's allowed hosts (SSRF guard).
    The gogoanime recipe's allowlist already covers mewstream/lostproject."""
    from .recipes.gogoanime import GOGOANIME_ALLOWED_HOSTS
    from .recipes.base import host_allowed

    return host_allowed(host_of(url), GOGOANIME_ALLOWED_HOSTS)


def _q(url: str) -> str:
    from urllib.parse import quote

    return quote(url, safe="")
