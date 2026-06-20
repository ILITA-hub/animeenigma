"""CamoufoxEngine — warm browser pool + recipe execution + challenge rotation.

Ties together: ProfileManager (aged persistent identities), ProxyPool (sticky
exit IPs, rotate-on-block), Camoufox launch options, and the recipe chain.

Camoufox is imported lazily so the pure-logic modules + unit tests don't need
the runtime/binary. End-to-end resolution requires the camoufox Firefox binary
(``python -m camoufox fetch``) + network and is verified on deploy.
"""

from __future__ import annotations

import asyncio
import os
import time
from typing import Any

from . import metrics
from .config import Config
from .fingerprint import build_launch_options, proxy_to_playwright
from .profiles import Profile, ProfileManager
from .recipes import ChallengeError, NotFoundError, Recipe, RecipeContext, RecipeError
from .recipes.base import host_of
from .recipes.gogoanime import GogoanimeRecipe
from .tunnels import ProxyPool, build_pool_from_config
from .warming import warm_profile

# Resource types blocked at the context level to cut bandwidth + fingerprint
# surface (images are handled by camoufox's block_images launch option).
_ROUTE_BLOCK = {"font", "media"}


class _CamoufoxHandle:
    """Holds an open Camoufox persistent context via the async-CM protocol so we
    can keep the browser warm outside an ``async with`` block."""

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
        self._handles: dict[str, _CamoufoxHandle] = {}  # profile id -> handle
        self._recipes: dict[str, Recipe] = {"gogoanime": GogoanimeRecipe()}
        self._lock = asyncio.Lock()  # serializes launch/teardown bookkeeping
        self._log: Any = None

    def set_logger(self, log: Any) -> None:
        self._log = log

    # -- lifecycle ---------------------------------------------------------- #
    async def start(self) -> None:
        os.makedirs(self.cfg.profile_dir, exist_ok=True)
        metrics.BROWSER_POOL_SIZE.set(0)

    async def stop(self) -> None:
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
            "proxies": [
                {"id": e.id, "type": e.type, "blocked": e.total_blocked}
                for e in self.pool.all()
            ],
        }

    # -- launch / teardown of a profile's browser --------------------------- #
    async def _ensure_browser(self, profile: Profile, proxy_id: str) -> Any:
        """Ensure ``profile`` has a live Camoufox bound to ``proxy_id``. A
        clearance is exit-IP-bound, so a proxy change forces a relaunch."""
        if profile.launched and profile.proxy_id == proxy_id:
            return profile.context

        # Tear down a stale browser (different proxy) before relaunch.
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
                await warm_profile(
                    page,
                    self.cfg.warming_sites,
                    self._log,
                    nav_timeout_ms=self.cfg.nav_timeout_ms,
                )
            await page.close()
        except Exception:  # noqa: BLE001 - UA/warming best-effort
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
        except Exception:  # noqa: BLE001 - routing is an optimization, not required
            pass

    # -- resolve ------------------------------------------------------------ #
    async def resolve(self, provider: str, params: dict) -> dict:
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")

        started = time.monotonic()
        tried: set[str] = set()
        last_err: Exception | None = None

        for attempt in range(self.cfg.max_proxy_retries + 1):
            profile = await self._acquire_profile()
            if profile is None:
                raise RecipeError("no free browser profile (pool exhausted)")

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
                        page=page,
                        context=context,
                        params=params,
                        cfg=self.cfg,
                        log=self._log,
                        proxy_id=proxy.id,
                        user_agent=profile.user_agent,
                    )
                    session = await asyncio.wait_for(
                        recipe.resolve(rc),
                        timeout=self.cfg.resolve_timeout_ms / 1000.0,
                    )
                    session = await self._finalize(session, context, proxy.id, profile)
                finally:
                    try:
                        await page.close()
                    except Exception:  # noqa: BLE001
                        pass

                self.pool.mark_ok(proxy.id)
                self.profiles.release(profile, ok=True)
                if self.profiles.needs_retire(profile):
                    await self._teardown(profile, reason="recycle")
                metrics.RESOLVE_TOTAL.labels(provider=provider, result="ok").inc()
                metrics.RESOLVE_DURATION.labels(provider=provider).observe(
                    time.monotonic() - started
                )
                return session

            except ChallengeError as exc:
                last_err = exc
                metrics.CHALLENGE_TOTAL.labels(
                    host=exc.host or "?", kind=exc.kind
                ).inc()
                metrics.PROXY_BLOCK_TOTAL.labels(proxy_id=proxy.id).inc()
                self.pool.mark_blocked(proxy.id)
                tried.add(proxy.id)
                # Clearance is dead on this IP; drop the browser so the retry
                # relaunches on a fresh exit.
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

            except Exception as exc:  # noqa: BLE001 - browser crash etc.
                last_err = exc
                await self._teardown(profile, reason="crash")
                self.profiles.release(profile, ok=False)
                tried.add(proxy.id)
                continue

        metrics.RESOLVE_TOTAL.labels(provider=provider, result="challenge").inc()
        raise ChallengeError(
            f"exhausted {len(tried)} exit(s); last: {last_err}",
            kind="exhausted",
        )

    async def _acquire_profile(self, retries: int = 50) -> Profile | None:
        for _ in range(retries):
            p = self.profiles.lease()
            if p is not None:
                return p
            await asyncio.sleep(0.1)
        return None

    async def _finalize(
        self, session: dict, context: Any, proxy_id: str, profile: Profile
    ) -> dict:
        """Harvest the clearance cookies for the CDN host + attach the binding
        the streaming HLS proxy needs to fetch the playlist/segments itself."""
        master = session.get("master_url")
        cookies: list[dict] = []
        try:
            raw = await context.cookies(master) if master else []
            for c in raw:
                cookies.append(
                    {
                        "name": c.get("name"),
                        "value": c.get("value"),
                        "domain": c.get("domain"),
                        "path": c.get("path", "/"),
                    }
                )
        except Exception:  # noqa: BLE001 - cookie harvest best-effort
            cookies = []

        session["cookies"] = cookies
        session["user_agent"] = profile.user_agent
        session["proxy_id"] = proxy_id
        session["cdn_host"] = host_of(master)
        session["resolved_via"] = "camoufox"
        session["expires_at"] = int(time.time()) + self.cfg.session_ttl_seconds
        return session
