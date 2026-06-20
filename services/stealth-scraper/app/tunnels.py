"""ProxyPool — pluggable IP-tunnel selection for the stealth-scraper.

Pure stdlib; unit-tested without a browser. Holds a typed set of exit tunnels
(direct / warp / datacenter / residential / mobile), supports:

  - sticky-per-session selection (a Cloudflare ``cf_clearance`` cookie is bound
    to its exit IP, so a profile MUST keep the same proxy across the resolve +
    the downstream segment fetches — see plan §3.6),
  - rotate-on-block: a tunnel that returns a challenge is benched for a cooldown
    and the next-best tunnel is chosen,
  - health scoring so chronically-blocked exits sink to the bottom.

The clock is injected (defaults to ``time.monotonic``) for deterministic tests.
"""

from __future__ import annotations

import json
import time
from dataclasses import dataclass
from typing import Callable, Iterable

DIRECT_ID = "direct"

# Selection preference when no explicit type is requested: residential/mobile
# exits are the scarce, challenge-passing ones — prefer cheaper exits first and
# escalate only when they get blocked.
_TYPE_ORDER = {"direct": 0, "warp": 1, "datacenter": 2, "residential": 3, "mobile": 4}


@dataclass
class ProxyEntry:
    id: str
    type: str                 # direct|warp|datacenter|residential|mobile
    url: str | None = None    # None ⇒ direct (no proxy)
    geo: str | None = None    # ISO country hint, drives browser locale/timezone
    consecutive_blocks: int = 0
    total_ok: int = 0
    total_blocked: int = 0
    last_blocked_at: float = 0.0

    @property
    def is_direct(self) -> bool:
        return self.url is None


class ProxyPool:
    def __init__(
        self,
        entries: Iterable[ProxyEntry],
        clock: Callable[[], float] = time.monotonic,
        cooldown: float = 120.0,
    ) -> None:
        self._entries: dict[str, ProxyEntry] = {}
        self._order: list[str] = []
        for e in entries:
            if e.id in self._entries:
                raise ValueError(f"duplicate proxy id: {e.id}")
            self._entries[e.id] = e
            self._order.append(e.id)
        if not self._order:
            raise ValueError("ProxyPool requires at least one entry")
        self._clock = clock
        self._cooldown = cooldown
        self._sticky: dict[str, str] = {}

    def all(self) -> list[ProxyEntry]:
        return [self._entries[i] for i in self._order]

    def get(self, pid: str) -> ProxyEntry | None:
        return self._entries.get(pid)

    def _cooling(self, e: ProxyEntry, now: float) -> bool:
        return bool(e.last_blocked_at) and (now - e.last_blocked_at) < self._cooldown

    def select(
        self,
        preferred_type: str | None = None,
        sticky_key: str | None = None,
        exclude: set[str] | None = None,
    ) -> ProxyEntry | None:
        """Pick the best available tunnel.

        Honors a prior sticky binding for ``sticky_key`` when that tunnel is
        still healthy, so a session keeps its exit IP. ``exclude`` lets the
        engine skip tunnels already tried this resolve (rotation).
        """
        now = self._clock()
        exclude = exclude or set()

        if sticky_key and sticky_key in self._sticky:
            sid = self._sticky[sticky_key]
            e = self._entries.get(sid)
            if e is not None and sid not in exclude and not self._cooling(e, now):
                return e

        candidates = [
            e
            for i in self._order
            if (e := self._entries[i]).id not in exclude and not self._cooling(e, now)
        ]
        if not candidates:
            # Total starvation guard: ignore cooldown rather than fail the
            # request outright (a cooling exit may still beat no playback).
            candidates = [self._entries[i] for i in self._order if i not in exclude]
        if not candidates:
            return None

        def rank(e: ProxyEntry) -> tuple:
            type_match = 0 if (preferred_type and e.type == preferred_type) else 1
            return (
                type_match,
                e.consecutive_blocks,
                _TYPE_ORDER.get(e.type, 99),
                e.total_blocked,
            )

        candidates.sort(key=rank)
        chosen = candidates[0]
        if sticky_key:
            self._sticky[sticky_key] = chosen.id
        return chosen

    def mark_ok(self, pid: str) -> None:
        e = self._entries.get(pid)
        if e is not None:
            e.consecutive_blocks = 0
            e.total_ok += 1

    def mark_blocked(self, pid: str) -> None:
        e = self._entries.get(pid)
        if e is None:
            return
        e.consecutive_blocks += 1
        e.total_blocked += 1
        e.last_blocked_at = self._clock()
        # Drop any sticky binding pointing at the blocked exit so the next
        # select() for that session rotates to a fresh IP.
        for key, val in list(self._sticky.items()):
            if val == pid:
                del self._sticky[key]


def build_pool_from_config(cfg, clock: Callable[[], float] = time.monotonic) -> ProxyPool:
    """Assemble a ProxyPool from Config: always a ``direct`` exit, optional WARP,
    plus any residential/mobile entries from ``STEALTH_PROXIES`` (JSON)."""
    entries: list[ProxyEntry] = [ProxyEntry(id=DIRECT_ID, type="direct", url=None)]
    if getattr(cfg, "warp_proxy_url", ""):
        entries.append(ProxyEntry(id="warp", type="warp", url=cfg.warp_proxy_url))
    raw = getattr(cfg, "proxies_json", "") or ""
    if raw.strip():
        parsed = json.loads(raw)
        if not isinstance(parsed, list):
            raise ValueError("STEALTH_PROXIES must be a JSON array")
        for i, p in enumerate(parsed):
            pid = p.get("id") or f"proxy{i}"
            url = p.get("url")
            if not url:
                raise ValueError(f"STEALTH_PROXIES[{i}] missing 'url'")
            entries.append(
                ProxyEntry(
                    id=pid,
                    type=p.get("type", "residential"),
                    url=url,
                    geo=p.get("geo"),
                )
            )
    return ProxyPool(
        entries,
        clock=clock,
        cooldown=getattr(cfg, "proxy_cooldown_seconds", 120.0),
    )
