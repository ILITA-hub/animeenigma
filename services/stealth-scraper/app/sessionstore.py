"""Persisted stealth-session records (Layer B of the playback self-healing
design, docs/superpowers/specs/2026-07-10-playback-self-healing-design.md).

One tiny JSON per sid under ``{profile_dir}/sessions/`` on the existing
``docker_stealth_profiles`` volume. Only session *metadata* persists — the
live Camoufox page cannot survive a restart; the engine lazily rebuilds it
from a record (same sid) when the Camoufox build is unchanged.
"""

from __future__ import annotations

import json
import os
import re

_SID_RE = re.compile(r"^[a-f0-9]{8,64}$")


def camoufox_build() -> str:
    """Version stamp of the running Camoufox — a rehydrate across builds is
    refused (fingerprint family changed; old profiles/sessions untrusted)."""
    try:
        from importlib.metadata import version

        return version("camoufox")
    except Exception:  # noqa: BLE001 — metadata missing in odd envs
        return "unknown"


class SessionStore:
    def __init__(self, base_dir: str) -> None:
        self.base_dir = base_dir
        os.makedirs(base_dir, exist_ok=True)

    def _path(self, sid: str) -> str | None:
        if not _SID_RE.match(sid or ""):
            return None
        return os.path.join(self.base_dir, f"{sid}.json")

    def save(self, rec: dict) -> None:
        path = self._path(rec["sid"])
        if path is None:
            return
        tmp = f"{path}.tmp"
        with open(tmp, "w") as f:
            json.dump(rec, f)
        os.replace(tmp, path)  # atomic — a crashed write never corrupts

    def load(self, sid: str) -> dict | None:
        path = self._path(sid)
        if path is None or not os.path.exists(path):
            return None
        try:
            with open(path) as f:
                return json.load(f)
        except (json.JSONDecodeError, OSError):
            try:
                os.remove(path)
            except OSError:
                pass
            return None

    def delete(self, sid: str) -> None:
        path = self._path(sid)
        if path is None:
            return
        try:
            os.remove(path)
        except FileNotFoundError:
            pass

    def sweep(self, now: float) -> int:
        """Drop expired records; returns how many were removed. Called from
        the engine's reaper loop so abandoned records don't accumulate."""
        dropped = 0
        try:
            names = os.listdir(self.base_dir)
        except OSError:
            return 0
        for name in names:
            if not name.endswith(".json"):
                continue
            sid = name[:-5]
            rec = self.load(sid)
            if rec is None or rec.get("expires_at", 0) <= now:
                self.delete(sid)
                dropped += 1
        return dropped
