"""Session aging / profile warming.

Drives a fresh profile through a realistic browsing pass BEFORE it ever touches
a target, so anti-bot heuristics that weight cookie age / GA presence / referrer
chain see a human-looking, aged profile rather than a cold bot. Best-effort:
warming failures never fail a resolve.
"""

from __future__ import annotations

import asyncio
from typing import Any


async def warm_profile(page: Any, sites: list[str], log: Any, *, nav_timeout_ms: int) -> None:
    """Visit each warming site with a short dwell + scroll. Swallows errors per
    site (a dead warming URL must not break the pool)."""
    for url in sites:
        try:
            await page.goto(url, wait_until="domcontentloaded", timeout=nav_timeout_ms)
            # Light human texture: scroll + dwell.
            await page.evaluate(
                "() => window.scrollBy(0, Math.floor(Math.random() * 800) + 200)"
            )
            await asyncio.sleep(0.8)
        except Exception as exc:  # noqa: BLE001 - warming is best-effort
            if log is not None:
                log.warning("warming visit failed", extra={"url": url, "err": str(exc)})
            continue
