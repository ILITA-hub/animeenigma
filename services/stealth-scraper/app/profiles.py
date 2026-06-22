"""Persistent browser-profile pool.

Each profile is a long-lived identity: its own ``user_data_dir`` (cookies,
localStorage, history → session aging), a pinned OS/fingerprint (seeded by the
profile id, see fingerprint.py), and a sticky proxy binding (a Cloudflare
clearance is exit-IP-bound, so a profile must keep one IP). The engine leases a
profile per resolve and returns it; profiles retire after N uses or on a crash.

This module is pure bookkeeping (no browser handle) so it unit-tests cleanly;
the engine attaches the live browser/context to a profile via ``attach``.
"""

from __future__ import annotations

import os
import time
from dataclasses import dataclass, field
from typing import Any


@dataclass
class Profile:
    id: str
    user_data_dir: str
    proxy_id: str | None = None     # sticky exit-IP binding
    uses: int = 0
    leased: bool = False
    # Live handles attached by the engine (None until launched).
    browser: Any = None
    context: Any = None
    user_agent: str = ""
    # -- self-heal health (Phase 1) ----------------------------------------- #
    # "healthy": usable; "crashed": browser died, awaiting reaper resurrect;
    # "warming": resurrect in progress (transient, set by the reaper).
    status: str = "healthy"
    consecutive_fail: int = 0   # failed resurrect attempts in a row
    last_crash: float = 0.0     # time.time() of the most recent crash mark
    next_resurrect_at: float = 0.0  # earliest time the reaper may retry this slot
    last_error: str = ""            # most recent crash error string (for health())

    @property
    def launched(self) -> bool:
        return self.context is not None


class ProfileManager:
    def __init__(self, base_dir: str, size: int, max_uses: int = 50) -> None:
        if size < 1:
            raise ValueError("profile pool size must be >= 1")
        self.base_dir = base_dir
        self.max_uses = max_uses
        self._profiles: list[Profile] = []
        for i in range(size):
            pid = f"p{i}"
            self._profiles.append(
                Profile(id=pid, user_data_dir=os.path.join(base_dir, pid))
            )

    def all(self) -> list[Profile]:
        return list(self._profiles)

    def lease(self) -> Profile | None:
        """Lease a free healthy profile, preferring already-launched ones (warm)
        with the fewest uses. Crashed slots are excluded — the reaper resurrects
        them; they must not be handed to a live request path."""
        free = [p for p in self._profiles if not p.leased and p.status == "healthy"]
        if not free:
            return None
        free.sort(key=lambda p: (not p.launched, p.uses))
        p = free[0]
        p.leased = True
        return p

    def release(self, profile: Profile, *, ok: bool) -> None:
        profile.leased = False
        if ok:
            profile.uses += 1

    def needs_retire(self, profile: Profile) -> bool:
        return profile.uses >= self.max_uses

    def reset_handles(self, profile: Profile) -> None:
        # Clear live handles only. Do NOT zero `uses` here: this runs on every
        # browser teardown (rotate/crash), and zeroing the success counter on
        # each teardown is why needs_retire never fired. Use reset_uses() to
        # actually retire a profile.
        profile.browser = None
        profile.context = None
        profile.user_agent = ""

    def reset_uses(self, profile: Profile) -> None:
        profile.uses = 0

    # -- self-heal bookkeeping (Phase 1) ------------------------------------ #
    def mark_crashed(self, profile: Profile, *, error: str = "") -> None:
        """Flag a slot as crashed (browser dead). Increments the consecutive
        failure counter (drives the retire-after-N rule) and stamps last_crash
        so the reaper can apply an exponential per-slot backoff. The live
        handles are cleared by the engine's _teardown; this only sets state."""
        profile.status = "crashed"
        profile.consecutive_fail += 1
        profile.last_crash = time.time()
        if error:
            profile.last_error = error

    def mark_healthy(self, profile: Profile) -> None:
        """Clear crash state after a successful resurrect / launch."""
        profile.status = "healthy"
        profile.consecutive_fail = 0
        profile.next_resurrect_at = 0.0

    def crashed_idle(self) -> list[Profile]:
        """Crashed slots that are NOT currently leased — the reaper may try to
        resurrect these without racing an in-flight lease."""
        return [p for p in self._profiles if p.status == "crashed" and not p.leased]

    def status_counts(self) -> dict[str, int]:
        counts = {"healthy": 0, "crashed": 0, "warming": 0}
        for p in self._profiles:
            counts[p.status] = counts.get(p.status, 0) + 1
        return counts
