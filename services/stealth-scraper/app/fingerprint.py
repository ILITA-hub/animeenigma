"""Browser fingerprint / launch-option construction for Camoufox.

Camoufox injects a coherent fingerprint (BrowserForge) at LAUNCH — it is a
patched Firefox, so the fingerprint is per-browser-instance, not per-context.
We therefore pin one fingerprint per persistent *profile* (humans don't change
their canvas hash between page loads) and rotate by leasing a different profile.

``build_launch_options`` is pure (returns a dict) so it is unit-testable without
the camoufox runtime.
"""

from __future__ import annotations

import hashlib
from typing import Any

# A small map from ISO country to a sensible default locale, used when a proxy
# carries a geo hint but camoufox geoip is disabled. With geoip on, camoufox
# derives locale/timezone from the exit IP and these are ignored.
_GEO_LOCALE = {
    "US": "en-US",
    "GB": "en-GB",
    "DE": "de-DE",
    "FR": "fr-FR",
    "RU": "ru-RU",
    "JP": "ja-JP",
}


def _headless_opt(value):
    """Map the config headless string to camoufox's headless arg:
    "virtual" → 'virtual' (Xvfb headful), "false" → False, anything else → True."""
    v = str(value).strip().lower()
    if v == "virtual":
        return "virtual"
    if v in ("false", "0", "no", "off"):
        return False
    return True


def _seed(profile_id: str) -> int:
    """Deterministic per-profile seed so a profile's OS/fingerprint is stable
    across relaunches (sticky identity)."""
    h = hashlib.sha256(profile_id.encode("utf-8")).hexdigest()
    return int(h[:8], 16)


def pick_os(profile_id: str, os_rotate: list[str]) -> str:
    if not os_rotate:
        return "windows"
    return os_rotate[_seed(profile_id) % len(os_rotate)]


def build_launch_options(
    *,
    profile_id: str,
    user_data_dir: str,
    proxy: dict | None,
    geo: str | None,
    cfg: Any,
) -> dict:
    """Assemble the kwargs passed to ``AsyncCamoufox(...)``.

    ``proxy`` is the Playwright proxy dict (or None for direct). When camoufox
    ``geoip`` is on it aligns locale/timezone/WebRTC to the proxy exit; we still
    pass a ``locale`` fallback derived from the geo hint for the geoip-off path.
    """
    opts: dict[str, Any] = {
        "headless": _headless_opt(cfg.headless),
        "humanize": bool(cfg.humanize),
        "os": pick_os(profile_id, cfg.os_rotate),
        "persistent_context": True,
        "user_data_dir": user_data_dir,
        # block_images cuts bandwidth + a fingerprinting surface; media/font
        # blocking is applied via context routing in the engine.
        "block_images": "image" in (cfg.block_resources or []),
        "i_know_what_im_doing": True,
    }
    # Camoufox bundles uBlock Origin and loads it by default. Measured
    # 2026-06-22 on both browser providers (gogoanime megaplay + nineanime
    # 9anime): uBO blocks ZERO third-party requests on either surface yet adds
    # ~1.7-2.5s to a cold session. It is net-negative here, so exclude it.
    # (Lazy import keeps this module camoufox-free at import time.)
    from camoufox.addons import DefaultAddons

    opts["exclude_addons"] = [DefaultAddons.UBO]
    if proxy:
        opts["proxy"] = proxy
        opts["geoip"] = bool(cfg.geoip)
    if geo and not cfg.geoip:
        loc = _GEO_LOCALE.get(geo.upper())
        if loc:
            opts["locale"] = loc
    return opts


def proxy_to_playwright(url: str | None) -> dict | None:
    """Convert a ``scheme://user:pass@host:port`` proxy URL into the Playwright
    proxy dict camoufox expects, or None for direct."""
    if not url:
        return None
    from urllib.parse import urlsplit

    s = urlsplit(url)
    if not s.hostname:
        raise ValueError(f"invalid proxy url: {url!r}")
    server = f"{s.scheme}://{s.hostname}"
    if s.port:
        server += f":{s.port}"
    out: dict[str, Any] = {"server": server}
    if s.username:
        out["username"] = s.username
    if s.password:
        out["password"] = s.password
    return out
