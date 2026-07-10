# Playback Self-Healing (Layer B + safety net) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** stealth-scraper sessions survive container redeploys (same-sid lazy rehydrate, warming skipped when Camoufox is unchanged), and the scraper never serves a cached stream URL whose stealth session is dead.

**Architecture:** Session metadata persists as one tiny JSON per sid on the existing `docker_stealth_profiles` volume. `/hls` with an unknown sid lazily rebuilds the live Camoufox page under the same sid (build-id + expiry + master-fetch verified). A `GET /session/{sid}/alive` endpoint lets the Go scraper gate its Redis stream-cache hits, treating `gone` as a cache miss. Spec: `docs/superpowers/specs/2026-07-10-playback-self-healing-design.md`. **Layer A (solodcdn edge rotation) already shipped** on main as `0a8936b9` — NOT in this plan.

**Tech Stack:** Python 3 / FastAPI / asyncio + unittest (stealth-scraper); Go + libs/cache (scraper).

## Global Constraints

- Work ONLY in the worktree `/data/ae-playback-selfheal` — never edit `/data/animeenigma` (base tree).
- Commit with explicit pathspec (`git commit -- <files>`), never a bare `git commit -a` (shared index hazard).
- Co-authors on EVERY commit:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>`, `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>`, `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`.
- Python tests: stdlib `unittest` classes + `run(coro)` helper style (match `tests/test_engine_lifecycle.py`); run with `python3 -m unittest <module> -v` from `services/stealth-scraper/`.
- Go: shared `libs/errors`/`libs/logger` conventions; tests `go test ./...` from `services/scraper/`.
- New tests use `tempfile.mkdtemp()` for `profile_dir` — never the default `/data/profiles`.
- Never run `gofmt -w` / `make fmt` (smart-quote landmine); rely on `go build` + targeted edits.
- Env vars documented in `docs/environment-variables.md` when added.

---

### Task 1: SessionStore — persisted session records (stealth-scraper)

**Files:**
- Create: `services/stealth-scraper/app/sessionstore.py`
- Test: `services/stealth-scraper/tests/test_sessionstore.py`

**Interfaces:**
- Produces: `camoufox_build() -> str`; `class SessionStore(base_dir)` with `save(rec: dict) -> None`, `load(sid: str) -> dict | None`, `delete(sid: str) -> None`, `sweep(now: float) -> int`. Record keys: `sid, master_url, player_url, referer, profile_id, proxy_id, user_key, cdn_host, expires_at, camoufox_build, created_at`.

- [ ] **Step 1: Write the failing test**

```python
"""SessionStore: atomic per-sid JSON records on the profiles volume."""

import os
import tempfile
import time
import unittest

from app.sessionstore import SessionStore, camoufox_build


def _rec(sid="s1", expires=None):
    return {
        "sid": sid,
        "master_url": "https://cdn.mewstream.buzz/m.m3u8",
        "player_url": "https://megaplay.buzz/stream/x",
        "referer": "https://megaplay.buzz/",
        "profile_id": "p0",
        "proxy_id": "direct",
        "user_key": "u1",
        "cdn_host": "cdn.mewstream.buzz",
        "expires_at": expires if expires is not None else time.time() + 600,
        "camoufox_build": camoufox_build(),
        "created_at": time.time(),
    }


class TestSessionStore(unittest.TestCase):
    def setUp(self):
        self.store = SessionStore(tempfile.mkdtemp())

    def test_save_load_roundtrip(self):
        rec = _rec()
        self.store.save(rec)
        got = self.store.load("s1")
        self.assertEqual(got["master_url"], rec["master_url"])
        self.assertEqual(got["profile_id"], "p0")

    def test_load_missing_returns_none(self):
        self.assertIsNone(self.store.load("nope"))

    def test_delete_is_idempotent(self):
        self.store.save(_rec())
        self.store.delete("s1")
        self.store.delete("s1")  # second delete must not raise
        self.assertIsNone(self.store.load("s1"))

    def test_load_corrupt_file_returns_none_and_removes(self):
        path = os.path.join(self.store.base_dir, "bad.json")
        with open(path, "w") as f:
            f.write("{not json")
        self.assertIsNone(self.store.load("bad"))
        self.assertFalse(os.path.exists(path))

    def test_sweep_drops_expired_only(self):
        self.store.save(_rec("old", expires=time.time() - 5))
        self.store.save(_rec("new"))
        dropped = self.store.sweep(time.time())
        self.assertEqual(dropped, 1)
        self.assertIsNone(self.store.load("old"))
        self.assertIsNotNone(self.store.load("new"))

    def test_camoufox_build_stable_nonempty(self):
        self.assertTrue(camoufox_build())
        self.assertEqual(camoufox_build(), camoufox_build())

    def test_sid_is_sanitized_no_traversal(self):
        self.assertIsNone(self.store.load("../etc/passwd"))
        self.store.delete("../etc/passwd")  # must not raise / escape base_dir


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_sessionstore -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'app.sessionstore'`

- [ ] **Step 3: Write the implementation**

```python
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
```

Note: `test_load_corrupt_file_returns_none_and_removes` uses sid `bad` (4 chars) — `_SID_RE` requires 8+. Write the corrupt file as `badbadbad1.json` and load `badbadbad1` instead when writing the test (fix the test to use a valid-shaped sid; the traversal test keeps its invalid sid on purpose).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_sessionstore -v`
Expected: PASS (7 tests)

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/stealth-scraper/app/sessionstore.py services/stealth-scraper/tests/test_sessionstore.py
git commit -m "feat(stealth-scraper): SessionStore — persisted per-sid session records" -- services/stealth-scraper/app/sessionstore.py services/stealth-scraper/tests/test_sessionstore.py
```
(Append the three standard co-author trailers to this and every commit message.)

---

### Task 2: Persist session lifecycle in the engine

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (Session dataclass ~line 97; `__init__` ~line 170; `_open_session` ~line 671; `proxy_fetch` ~line 740; `aclose_session` ~line 1226; `_evict_expired` ~line 1235; `_evict_one_lru` ~line 615; `_reaper_loop` ~line 1260)
- Test: `services/stealth-scraper/tests/test_engine_persist.py`

**Interfaces:**
- Consumes: `SessionStore`, `camoufox_build()` from Task 1.
- Produces: `CamoufoxEngine.store: SessionStore`; every live session has a persisted record; `Session.last_persist: float` field; engine helper `_session_record(session) -> dict`.

- [ ] **Step 1: Write the failing test**

```python
"""Session lifecycle persistence: records are written on open, refreshed on
TTL slide (throttled), and removed on close/eviction."""

import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public, run


def _engine(tmp=None):
    cfg = Config(
        pool_size=1, warming_enabled=False,
        profile_dir=tmp or tempfile.mkdtemp(),
    )
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


def _attach_session(eng, page, sid="a" * 32, ttl=600):
    prof = eng.profiles.lease()
    sess = Session(
        id=sid, profile=prof, proxy_id="direct",
        referer="https://megaplay.buzz/", user_agent="UA",
        cdn_host="cdn.mewstream.buzz",
        master_url="https://cdn.mewstream.buzz/m.m3u8",
        expires_at=time.time() + ttl, page=page, player_url=page.url,
    )
    eng._sessions[sid] = sess
    eng.store.save(eng._session_record(sess))
    return sess


class TestSessionPersistence(unittest.TestCase):
    def test_open_session_writes_record(self):
        eng = _engine()
        prof = eng.profiles.lease()
        page = _Page()
        sess = run(eng._open_session(
            {"master_url": "https://cdn.mewstream.buzz/m.m3u8",
             "referer": "https://megaplay.buzz/"},
            context=None, proxy_id="direct", profile=prof, page=page,
        ))
        rec = eng.store.load(sess.id)
        self.assertIsNotNone(rec)
        self.assertEqual(rec["camoufox_build"], camoufox_build())
        self.assertEqual(rec["profile_id"], prof.id)
        self.assertEqual(rec["player_url"], page.url)

    def test_proxy_fetch_refreshes_record_throttled(self):
        eng = _engine()
        sess = _attach_session(eng, _Page())
        sess.last_persist = 0.0  # stale — next fetch must persist
        run(eng.proxy_fetch(sess.id, "https://cdn.mewstream.buzz/seg1.ts"))
        rec = eng.store.load(sess.id)
        self.assertGreater(rec["expires_at"], time.time() + 500)
        first_persist = sess.last_persist
        run(eng.proxy_fetch(sess.id, "https://cdn.mewstream.buzz/seg2.ts"))
        self.assertEqual(sess.last_persist, first_persist)  # throttled

    def test_close_and_evictions_delete_record(self):
        eng = _engine()
        sess = _attach_session(eng, _Page())
        run(eng.aclose_session(sess.id))
        self.assertIsNone(eng.store.load(sess.id))

        sess2 = _attach_session(eng, _Page(), sid="b" * 32)
        sess2.expires_at = time.time() - 1
        eng._evict_expired()
        self.assertIsNone(eng.store.load(sess2.id))

        sess3 = _attach_session(eng, _Page(), sid="c" * 32)
        self.assertTrue(eng._evict_one_lru())
        self.assertIsNone(eng.store.load(sess3.id))


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_engine_persist -v`
Expected: FAIL — `AttributeError: 'CamoufoxEngine' object has no attribute 'store'` (and `_session_record` missing).

- [ ] **Step 3: Implement**

In `app/engine.py`:

1. Imports: `from .sessionstore import SessionStore, camoufox_build`.
2. `Session` dataclass — add field (after `user_key`):

```python
    # Wall-clock of the last persisted-record refresh (Layer B). Throttles
    # store writes to ~1/min instead of one per proxied segment.
    last_persist: float = 0.0
```

3. `CamoufoxEngine.__init__` — after `self.profiles = ProfileManager(...)`:

```python
        self.store = SessionStore(os.path.join(cfg.profile_dir, "sessions"))
```

4. New method (near `_open_session`):

```python
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
```

5. `_open_session` — right after `self._sessions[sid] = session`:

```python
        session.last_persist = time.time()
        self.store.save(self._session_record(session))
```

6. `proxy_fetch` — right after `session.expires_at = time.time() + self.cfg.session_ttl_seconds`:

```python
        # Refresh the persisted record so a redeploy mid-watch can rehydrate
        # with an accurate deadline — throttled, segments arrive every ~4s.
        if time.time() - session.last_persist > 60:
            session.last_persist = time.time()
            self.store.save(self._session_record(session))
```

7. `aclose_session` — after `self._sessions.pop(sid, None)` succeeds (inside the non-None branch): `self.store.delete(sid)`.
8. `_evict_expired` — inside the eviction branch, after `self._sessions.pop(sid, None)`: `self.store.delete(sid)`.
9. `_evict_one_lru` — after `self._sessions.pop(sid, None)`: `self.store.delete(sid)`.
10. `_reaper_loop` — wherever the periodic sweep body runs, add `self.store.sweep(time.time())` (records of sessions that died with a previous process must not accumulate).

- [ ] **Step 4: Run the new test AND the full stealth suite**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_engine_persist -v && python3 -m unittest discover tests -v 2>&1 | tail -5`
Expected: new tests PASS; full suite green (existing lifecycle tests construct engines with default `profile_dir` — if any fail on `/data/profiles` permissions, pass `profile_dir=tempfile.mkdtemp()` into their `Config(...)` as part of this task).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_persist.py
git commit -m "feat(stealth-scraper): persist session records through the lifecycle" -- services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_persist.py
```

---

### Task 3: Lazy same-sid rehydrate in `/hls`

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (`proxy_fetch` ~line 752; `ProfileManager.lease` in `app/profiles.py` ~line 62)
- Modify: `services/stealth-scraper/app/metrics.py` (new counter)
- Test: `services/stealth-scraper/tests/test_engine_rehydrate.py`

**Interfaces:**
- Consumes: `SessionStore.load/delete`, `camoufox_build()`, `_session_record` (Task 2).
- Produces: `async CamoufoxEngine._rehydrate(sid) -> Session | None`; `ProfileManager.lease(preferred: str | None = None)`; metric `stealth_rehydrate_total{result}` with results `ok|no_record|expired|build_mismatch|no_profile|verify_failed|error`.

- [ ] **Step 1: Write the failing test**

```python
"""Lazy same-sid rehydrate: an unknown sid with a valid persisted record is
rebuilt (profile leased, page reopened, master verified) under the SAME sid;
invalid records refuse and are deleted."""

import tempfile
import time
import unittest
from unittest import mock

from app.config import Config
from app.engine import CamoufoxEngine, SessionGone
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public, run


def _engine():
    cfg = Config(pool_size=1, warming_enabled=False,
                 profile_dir=tempfile.mkdtemp())
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


def _record(eng, sid="d" * 32, **over):
    rec = {
        "sid": sid,
        "master_url": "https://cdn.mewstream.buzz/m.m3u8",
        "player_url": "https://megaplay.buzz/stream/x",
        "referer": "https://megaplay.buzz/",
        "profile_id": "p0",
        "proxy_id": "direct",
        "user_key": "u1",
        "cdn_host": "cdn.mewstream.buzz",
        "expires_at": time.time() + 300,
        "camoufox_build": camoufox_build(),
        "created_at": time.time(),
    }
    rec.update(over)
    eng.store.save(rec)
    return rec


class _Ctx:
    def __init__(self, page):
        self._page = page

    async def new_page(self):
        return self._page


def _stub_browser(eng, page):
    async def _ensure(profile, proxy_id):
        profile.launched = True
        return _Ctx(page)

    eng._ensure_browser = _ensure


class _GotoPage(_Page):
    def __init__(self, **kw):
        super().__init__(**kw)
        self.goto_url = None

    async def goto(self, url, **kw):
        self.goto_url = url


class TestRehydrate(unittest.TestCase):
    def test_rehydrate_serves_same_sid(self):
        eng = _engine()
        rec = _record(eng)
        page = _GotoPage()
        _stub_browser(eng, page)
        out = run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/seg1.ts"))
        self.assertEqual(out["status"], 200)
        self.assertIn(rec["sid"], eng._sessions)
        self.assertEqual(page.goto_url, rec["player_url"])

    def test_no_record_raises_gone(self):
        eng = _engine()
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch("e" * 32, "https://cdn.mewstream.buzz/x.ts"))

    def test_build_mismatch_refuses_and_deletes(self):
        eng = _engine()
        rec = _record(eng, camoufox_build="0.0.1-other")
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))

    def test_expired_record_refuses_and_deletes(self):
        eng = _engine()
        rec = _record(eng, expires_at=time.time() - 1)
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))

    def test_verify_failure_releases_profile_and_deletes(self):
        eng = _engine()
        rec = _record(eng)
        page = _GotoPage(result="403|text/html|u||" )  # master fetch -> 403
        _stub_browser(eng, page)
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))
        self.assertTrue(all(not p.leased for p in eng.profiles.all()))

    def test_lease_prefers_recorded_profile(self):
        cfg = Config(pool_size=2, warming_enabled=False,
                     profile_dir=tempfile.mkdtemp())
        eng = CamoufoxEngine(cfg)
        got = eng.profiles.lease(preferred="p1")
        self.assertEqual(got.id, "p1")


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_engine_rehydrate -v`
Expected: FAIL — `SessionGone` raised in `test_rehydrate_serves_same_sid` (no rehydrate path), and `TypeError: lease() got an unexpected keyword argument 'preferred'`.

- [ ] **Step 3: Implement**

`app/profiles.py` — extend `lease`:

```python
    def lease(self, preferred: str | None = None) -> Profile | None:
        """Lease a free healthy profile, preferring already-launched ones (warm)
        with the fewest uses. ``preferred`` (a profile id) wins when it is free
        and healthy — the rehydrate path asks for the session's original
        identity so its cookies/clearance still apply."""
        free = [p for p in self._profiles if not p.leased and p.status == "healthy"]
        if not free:
            return None
        if preferred:
            for p in free:
                if p.id == preferred:
                    p.leased = True
                    p.uses += 1
                    return p
        free.sort(key=lambda p: (not p.launched, p.uses))
        p = free[0]
        p.leased = True
        p.uses += 1
        return p
```

(Keep the exact `uses` bookkeeping the current method has — copy it, do not guess; if `lease()` today doesn't increment `uses`, don't add it.)

`app/metrics.py` — add:

```python
REHYDRATE_TOTAL = Counter(
    "stealth_rehydrate_total",
    "Lazy session-rehydrate attempts by result",
    ["result"],
)
```

`app/engine.py`:

1. `__init__`: `self._rehydrate_locks: dict[str, asyncio.Lock] = {}`.
2. `proxy_fetch` — replace the two lines

```python
        session = self._sessions.get(sid)
        if session is None:
            raise SessionGone(sid)
```

with

```python
        session = self._sessions.get(sid)
        if session is None:
            session = await self._rehydrate(sid)
        if session is None:
            raise SessionGone(sid)
```

3. New method:

```python
    async def _rehydrate(self, sid: str) -> Session | None:
        """Rebuild a session that died with a previous process (Layer B of the
        playback self-healing design): same sid, recorded profile preferred,
        warming skipped (the on-disk profile is already warm), master playlist
        re-fetched once as the go/no-go check. Refuses across Camoufox builds."""
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

        lock = self._rehydrate_locks.setdefault(sid, asyncio.Lock())
        async with lock:
            existing = self._sessions.get(sid)
            if existing is not None:  # lost the race — another fetch rebuilt it
                return existing
            self._admit_launch()
            profile = self.profiles.lease(preferred=rec.get("profile_id"))
            if profile is None:
                metrics.REHYDRATE_TOTAL.labels(result="no_profile").inc()
                return None
            page = None
            try:
                context = await self._ensure_browser(profile, rec["proxy_id"])
                page = await context.new_page()
                await page.goto(rec["player_url"], wait_until="domcontentloaded")
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
                self._log.info("session rehydrated", extra={"sid": sid[:8]})
                return session
            except Exception as exc:  # noqa: BLE001 — any failure ⇒ clean 410
                metrics.REHYDRATE_TOTAL.labels(
                    result="verify_failed" if isinstance(exc, RecipeError) else "error"
                ).inc()
                if page is not None:
                    await _safe_close_page(page)
                self.profiles.release(profile, ok=True)
                self.store.delete(sid)
                return None
            finally:
                self._rehydrate_locks.pop(sid, None)
```

Adaptation notes (verify against the real file, do not guess): the logger attribute is whatever `engine.py` already uses (`self._log` appears in warming calls — match it); `RecipeError` is already imported from `.recipes.base`; `_safe_close_page` exists at module level; `_in_page_fetch` signature is `(session, url) -> (status, ctype, final_url, hdrs, body)`. `_Page.goto` doesn't exist on the plain test fake — the test's `_GotoPage` adds it; production Playwright pages have it.

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_engine_rehydrate -v && python3 -m unittest discover tests -v 2>&1 | tail -3`
Expected: PASS, full suite green.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/stealth-scraper/app/engine.py services/stealth-scraper/app/profiles.py services/stealth-scraper/app/metrics.py services/stealth-scraper/tests/test_engine_rehydrate.py
git commit -m "feat(stealth-scraper): lazy same-sid session rehydrate on unknown /hls sid" -- services/stealth-scraper/app/engine.py services/stealth-scraper/app/profiles.py services/stealth-scraper/app/metrics.py services/stealth-scraper/tests/test_engine_rehydrate.py
```

---

### Task 4: Warming skip via persisted warm marker

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (`_ensure_browser` ~line 309)
- Modify: `services/stealth-scraper/app/sessionstore.py` (marker helpers)
- Modify: `services/stealth-scraper/app/config.py` (new knob)
- Modify: `docs/environment-variables.md` (document `STEALTH_WARM_MARKER_TTL_SECONDS`)
- Test: `services/stealth-scraper/tests/test_warm_marker.py`

**Interfaces:**
- Produces: `sessionstore.read_warm_marker(user_data_dir) -> dict | None`, `sessionstore.write_warm_marker(user_data_dir) -> None`; `Config.warm_marker_ttl_seconds: int = 86400` (env `STEALTH_WARM_MARKER_TTL_SECONDS`).

- [ ] **Step 1: Write the failing test**

```python
"""Warm-marker skip: a profile warmed on THIS Camoufox build within the marker
TTL is not re-warmed on relaunch; a build change or stale marker re-warms."""

import json
import os
import tempfile
import time
import unittest

from app.sessionstore import (
    camoufox_build,
    read_warm_marker,
    write_warm_marker,
)


class TestWarmMarker(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()

    def test_roundtrip(self):
        write_warm_marker(self.dir)
        m = read_warm_marker(self.dir)
        self.assertEqual(m["camoufox_build"], camoufox_build())
        self.assertAlmostEqual(m["warmed_at"], time.time(), delta=5)

    def test_missing_returns_none(self):
        self.assertIsNone(read_warm_marker(self.dir))

    def test_corrupt_returns_none(self):
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            f.write("{oops")
        self.assertIsNone(read_warm_marker(self.dir))

    def test_skip_decision_matrix(self):
        # Fresh + same build -> skip; stale or other build -> warm again.
        def should_skip(marker, ttl=86400):
            return (
                marker is not None
                and marker.get("camoufox_build") == camoufox_build()
                and time.time() - marker.get("warmed_at", 0) < ttl
            )

        write_warm_marker(self.dir)
        self.assertTrue(should_skip(read_warm_marker(self.dir)))
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            json.dump({"camoufox_build": "old", "warmed_at": time.time()}, f)
        self.assertFalse(should_skip(read_warm_marker(self.dir)))
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            json.dump({"camoufox_build": camoufox_build(),
                       "warmed_at": time.time() - 90000}, f)
        self.assertFalse(should_skip(read_warm_marker(self.dir)))


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_warm_marker -v`
Expected: FAIL — `ImportError: cannot import name 'read_warm_marker'`

- [ ] **Step 3: Implement**

`app/sessionstore.py` — append:

```python
_WARM_MARKER = "warmed.json"


def read_warm_marker(user_data_dir: str) -> dict | None:
    path = os.path.join(user_data_dir, _WARM_MARKER)
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return None


def write_warm_marker(user_data_dir: str) -> None:
    import time as _time

    path = os.path.join(user_data_dir, _WARM_MARKER)
    tmp = f"{path}.tmp"
    try:
        os.makedirs(user_data_dir, exist_ok=True)
        with open(tmp, "w") as f:
            json.dump({"warmed_at": _time.time(),
                       "camoufox_build": camoufox_build()}, f)
        os.replace(tmp, path)
    except OSError:
        pass  # best-effort — a missed marker only costs one extra warm
```

`app/config.py` — add field `warm_marker_ttl_seconds: int = 86400` to the dataclass and `warm_marker_ttl_seconds=_int(g("STEALTH_WARM_MARKER_TTL_SECONDS"), 86400),` to the env loader (mirror how `session_ttl_seconds` is parsed at line ~190).

`app/engine.py` `_ensure_browser` — replace the warming call site

```python
            if self.cfg.warming_enabled and self._warming_allowed():
                from .warming import warm_profile
                await warm_profile(...)
```

with

```python
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
                )
                write_warm_marker(profile.user_data_dir)
```

(Copy the existing `warm_profile(...)` argument list verbatim from the file — the snippet above abbreviates it. Import `read_warm_marker, write_warm_marker` alongside the existing sessionstore import.)

`docs/environment-variables.md` — add one row to the stealth-scraper section: `STEALTH_WARM_MARKER_TTL_SECONDS` (default `86400`) — how long a persisted per-profile warm marker suppresses re-warming on relaunch; invalidated automatically by a Camoufox version change.

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_warm_marker -v && python3 -m unittest discover tests -v 2>&1 | tail -3`
Expected: PASS, suite green.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/stealth-scraper/app/sessionstore.py services/stealth-scraper/app/engine.py services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_warm_marker.py docs/environment-variables.md
git commit -m "feat(stealth-scraper): skip profile warming when the persisted warm marker is fresh on the same Camoufox build" -- services/stealth-scraper/app/sessionstore.py services/stealth-scraper/app/engine.py services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_warm_marker.py docs/environment-variables.md
```

---

### Task 5: `GET /session/{sid}/alive` endpoint

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (new `session_state`)
- Modify: `services/stealth-scraper/app/main.py` (new route next to the DELETE `/session/{sid}` at line ~241)
- Test: `services/stealth-scraper/tests/test_session_alive.py`

**Interfaces:**
- Produces: `CamoufoxEngine.session_state(sid) -> str` returning `"alive" | "rehydratable" | "gone"`; HTTP `GET /session/{sid}/alive` → `200 {"state": "<that>"}`. The Go scraper (Task 6) treats exactly the string `gone` as cache-poison; anything else serves the cache.

- [ ] **Step 1: Write the failing test**

```python
"""session_state: alive (registered), rehydratable (valid persisted record),
gone (nothing / expired / other build)."""

import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public


def _engine():
    cfg = Config(pool_size=1, warming_enabled=False,
                 profile_dir=tempfile.mkdtemp())
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


class TestSessionState(unittest.TestCase):
    def test_alive_when_registered(self):
        eng = _engine()
        prof = eng.profiles.lease()
        page = _Page()
        eng._sessions["f" * 32] = Session(
            id="f" * 32, profile=prof, proxy_id="direct",
            referer="", user_agent="UA", cdn_host=None,
            master_url="https://cdn.mewstream.buzz/m.m3u8",
            expires_at=time.time() + 60, page=page, player_url=page.url,
        )
        self.assertEqual(eng.session_state("f" * 32), "alive")

    def test_rehydratable_when_record_valid(self):
        eng = _engine()
        eng.store.save({
            "sid": "a" * 32, "master_url": "m", "player_url": "p",
            "referer": "", "profile_id": "p0", "proxy_id": "direct",
            "user_key": None, "cdn_host": None,
            "expires_at": time.time() + 60,
            "camoufox_build": camoufox_build(), "created_at": time.time(),
        })
        self.assertEqual(eng.session_state("a" * 32), "rehydratable")

    def test_gone_when_nothing(self):
        eng = _engine()
        self.assertEqual(eng.session_state("0" * 32), "gone")

    def test_gone_when_record_expired_or_other_build(self):
        eng = _engine()
        base = {
            "sid": "b" * 32, "master_url": "m", "player_url": "p",
            "referer": "", "profile_id": "p0", "proxy_id": "direct",
            "user_key": None, "cdn_host": None, "created_at": time.time(),
        }
        eng.store.save({**base, "expires_at": time.time() - 1,
                        "camoufox_build": camoufox_build()})
        self.assertEqual(eng.session_state("b" * 32), "gone")
        eng.store.save({**base, "expires_at": time.time() + 60,
                        "camoufox_build": "other"})
        self.assertEqual(eng.session_state("b" * 32), "gone")


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_session_alive -v`
Expected: FAIL — `AttributeError: ... no attribute 'session_state'`

- [ ] **Step 3: Implement**

`app/engine.py`:

```python
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
```

`app/main.py` — next to the existing DELETE route:

```python
@app.get("/session/{sid}/alive")
async def session_alive(sid: str) -> JSONResponse:
    """Liveness gate for the Go scraper's Redis stream cache (Layer B safety
    net): 'gone' ⇒ the scraper treats its cached stream URL as a miss."""
    return JSONResponse({"state": engine.session_state(sid)})
```

- [ ] **Step 4: Run tests**

Run: `cd /data/ae-playback-selfheal/services/stealth-scraper && python3 -m unittest tests.test_session_alive -v && python3 -m unittest discover tests -v 2>&1 | tail -3`
Expected: PASS, suite green.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/stealth-scraper/app/engine.py services/stealth-scraper/app/main.py services/stealth-scraper/tests/test_session_alive.py
git commit -m "feat(stealth-scraper): GET /session/{sid}/alive liveness endpoint" -- services/stealth-scraper/app/engine.py services/stealth-scraper/app/main.py services/stealth-scraper/tests/test_session_alive.py
```

---

### Task 6: Scraper dead-sid liveness gate on cached streams (Go)

**Files:**
- Modify: `services/scraper/internal/sidecar/client.go` (new `SessionAlive` + `SIDFromProxyURL`)
- Modify: `services/scraper/internal/providers/gogoanime/client.go` (Deps ~line 180, Provider struct ~line 218, `New()`, `GetStream` cache-hit ~line 888)
- Modify: `services/scraper/cmd/scraper-api/main.go` (wire the func into gogoanime Deps; grep `gogoanime.Deps{`)
- Test: `services/scraper/internal/sidecar/client_test.go` (or new `alive_test.go`), `services/scraper/internal/providers/gogoanime/client_gated_test.go` pattern → new test in `client_browser_test.go`

**Interfaces:**
- Consumes: HTTP `GET /session/{sid}/alive` → `{"state":"alive|rehydratable|gone"}` (Task 5).
- Produces: `func SIDFromProxyURL(rawURL string) (string, bool)` (package `sidecar`); `func (c *Client) SessionAlive(ctx context.Context, sid string) string` (fail-open: any error ⇒ `"alive"`); gogoanime `Deps.SessionAlive func(ctx context.Context, sid string) string` (nil ⇒ gate disabled).

- [ ] **Step 1: Write the failing tests**

`services/scraper/internal/sidecar/alive_test.go`:

```go
package sidecar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSIDFromProxyURL(t *testing.T) {
	sid, ok := SIDFromProxyURL("http://stealth-scraper:3000/hls?sid=abc123def456abc123def456abc12345&url=https%3A%2F%2Fcdn.mewstream.buzz%2Fm.m3u8")
	if !ok || sid != "abc123def456abc123def456abc12345" {
		t.Fatalf("want sid extracted, got %q ok=%v", sid, ok)
	}
	if _, ok := SIDFromProxyURL("https://vault-99.owocdn.top/stream/uwu.m3u8"); ok {
		t.Fatal("non-sidecar URL must not yield a sid")
	}
	if _, ok := SIDFromProxyURL("http://stealth-scraper:3000/hls?url=x"); ok {
		t.Fatal("missing sid must not yield a sid")
	}
}

func TestSessionAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/deadbeef/alive" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"state":"gone"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, 2*time.Second)
	if got := c.SessionAlive(context.Background(), "deadbeef"); got != "gone" {
		t.Fatalf("want gone, got %q", got)
	}
}

func TestSessionAliveFailsOpen(t *testing.T) {
	c := New("http://127.0.0.1:1", 200*time.Millisecond) // nothing listening
	if got := c.SessionAlive(context.Background(), "deadbeef"); got != "alive" {
		t.Fatalf("errors must fail open to alive, got %q", got)
	}
}
```

gogoanime gate test — add to `services/scraper/internal/providers/gogoanime/client_browser_test.go` (reuse that file's existing provider/fake-cache construction helpers; the assertions that matter):

```go
// TestGetStream_CachedDeadSidRefetches: a cache hit whose source URL embeds a
// stealth-scraper sid that the sidecar reports "gone" must be treated as a
// cache MISS — entry deleted, browser resolve re-run. Any other state (or a
// nil SessionAlive) serves the cache untouched.
func TestGetStream_CachedDeadSidRefetches(t *testing.T) {
	// Arrange: fakeCache pre-seeded at the provider's stream cache key with a
	// Stream whose Sources[0].URL is
	//   http://stealth-scraper:3000/hls?sid=<32 hex>&url=...
	// Provider constructed with UseBrowser=true and a BrowserResolve stub that
	// records invocation and returns a fresh Stream.

	// Case 1: SessionAlive returns "gone" -> BrowserResolve called once,
	// cache key deleted before the re-set.
	// Case 2: SessionAlive returns "alive" -> cached stream returned,
	// BrowserResolve NOT called.
	// Case 3: SessionAlive nil -> cached stream returned (gate disabled).
	// Case 4: cached URL has no sid (plain CDN URL) -> cached stream returned,
	// SessionAlive NOT called.
}
```

Write the four cases as real code against the existing helpers in `client_browser_test.go`/`helpers_test.go` (the `fakeCache` there already implements `cache.Cache`; it needs a `Delete` recorder if it lacks one — add `deleted []string` tracking to it in this task).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /data/ae-playback-selfheal/services/scraper && go test ./internal/sidecar/ ./internal/providers/gogoanime/ 2>&1 | tail -5`
Expected: compile FAIL — `undefined: SIDFromProxyURL`, `undefined: (*Client).SessionAlive`, `unknown field SessionAlive`.

- [ ] **Step 3: Implement**

`services/scraper/internal/sidecar/client.go` — append:

```go
// SIDFromProxyURL extracts the stealth-scraper session id from a sidecar
// stream-proxy URL (http://stealth-scraper:3000/hls?sid=...&url=...). Returns
// ok=false for every non-sidecar URL so callers can gate cheaply.
func SIDFromProxyURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path != "/hls" {
		return "", false
	}
	sid := u.Query().Get("sid")
	return sid, sid != ""
}

// SessionAlive reports the sidecar's liveness verdict for sid: "alive",
// "rehydratable" or "gone". FAIL-OPEN: any transport/decode error returns
// "alive" — a sidecar hiccup must not stampede cache re-resolves.
func (c *Client) SessionAlive(ctx context.Context, sid string) string {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/session/"+url.PathEscape(sid)+"/alive", nil)
	if err != nil {
		return "alive"
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "alive"
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "alive"
	}
	var out struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&out); err != nil {
		return "alive"
	}
	switch out.State {
	case "alive", "rehydratable", "gone":
		return out.State
	}
	return "alive"
}
```

(Match the receiver's actual field names — `c.baseURL`/`c.http` per `New(baseURL, timeout)` at line ~105; adjust if the struct calls them differently.)

`services/scraper/internal/providers/gogoanime/client.go`:

1. `Deps` — add:

```go
	// SessionAlive asks the stealth-scraper whether a cached stream URL's
	// embedded session still exists ("gone" ⇒ treat the cache hit as a miss).
	// nil disables the gate (in-process extraction paths never need it).
	SessionAlive func(ctx context.Context, sid string) string
```

2. `Provider` struct — add `sessionAlive func(ctx context.Context, sid string) string`; copy from Deps in `New()` like the other fields.
3. `GetStream` — replace the cache-hit block:

```go
	var cached domain.Stream
	if err := p.cache.Get(ctx, cacheKey, &cached); err == nil {
		if !p.cachedSessionGone(ctx, &cached) {
			p.markStage(health.StageStream, nil)
			return &cached, nil
		}
		// Dead stealth session behind the cached URL (Layer B safety net) —
		// serving it would 410 every segment. Drop and re-resolve.
		_ = p.cache.Delete(ctx, cacheKey)
	}
```

4. New helper (same file, near `GetStream`):

```go
// cachedSessionGone reports whether a cached stream's first source is a
// stealth-scraper proxy URL whose session the sidecar declares gone. False
// whenever the gate is disabled, the URL isn't sidecar-shaped, or the sidecar
// says alive/rehydratable (fail-open lives in SessionAlive itself).
func (p *Provider) cachedSessionGone(ctx context.Context, s *domain.Stream) bool {
	if p.sessionAlive == nil || s == nil || len(s.Sources) == 0 {
		return false
	}
	sid, ok := sidecar.SIDFromProxyURL(s.Sources[0].URL)
	if !ok {
		return false
	}
	return p.sessionAlive(ctx, sid) == "gone"
}
```

Add the `sidecar` import (`github.com/ILITA-hub/animeenigma/services/scraper/internal/sidecar`) — check for an import cycle first (`sidecar` must not import `providers/gogoanime`; it doesn't today). If a cycle exists, put `SIDFromProxyURL` in a leaf package instead and adjust both call sites.

5. `services/scraper/cmd/scraper-api/main.go` — locate `gogoanime.Deps{` (grep) and add `SessionAlive: sidecarClient.SessionAlive,` using whatever variable already holds the `sidecar.Client` used for `BrowserResolve`.

6. Check the sibling browser-engine provider: `grep -n "BrowserResolve\|sidecar" services/scraper/internal/providers/nineanime/client.go`. If nineanime caches sidecar `/hls?sid=` stream URLs the same way (a `cache.Set` on a stream whose sources come from the sidecar), apply the identical Deps field + cache-hit gate + main.go wiring there; if its cached URLs are plain CDN URLs, skip it and note that in the commit message.

- [ ] **Step 4: Run tests + build**

Run: `cd /data/ae-playback-selfheal/services/scraper && go build ./... && go test ./internal/sidecar/ ./internal/providers/gogoanime/ 2>&1 | tail -5`
Expected: build OK, tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-playback-selfheal
git add services/scraper/internal/sidecar/ services/scraper/internal/providers/gogoanime/ services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): dead-sid liveness gate — cached sidecar streams re-resolve when the session is gone" -- services/scraper/internal/sidecar/ services/scraper/internal/providers/gogoanime/ services/scraper/cmd/scraper-api/main.go
```

---

### Task 7: Deploy, live verification, ship

**Files:**
- No code. Uses `make redeploy-stealth-scraper`, `make redeploy-scraper`, `bin/feedback-status` n/a.

- [ ] **Step 1: Land + deploy**

From the worktree: `git pull --rebase origin main && git push origin HEAD:main`, then `make redeploy-stealth-scraper && make redeploy-scraper && make health`.

- [ ] **Step 2: Live rehydrate verification (real anime, house rule)**

1. Resolve a browser-engine stream: `curl -s "http://localhost:8081/api/anime/da49e513-7df9-44b7-9fd2-bb15542948e3/scraper/stream?episode=<epId>&server=<serverId>&category=sub&prefer=gogoanime"` (get episode/server ids from the `/scraper/episodes` + `/scraper/servers` endpoints first). Extract the `sid` from the returned source URL.
2. Fetch one segment through the public proxy — expect 200.
3. `docker restart animeenigma-stealth-scraper` and wait for healthy.
4. Fetch the SAME proxied URL again — expect 200 (rehydrated, NOT 410) and `curl -s localhost:<stealth-port>/metrics | grep stealth_rehydrate_total` shows `result="ok"`.
5. `curl http://stealth-scraper:3000/session/<sid>/alive` (via `docker exec animeenigma-scraper wget -qO- ...`) — expect `alive`.
6. Kill the record (`docker exec animeenigma-stealth-scraper rm /data/profiles/sessions/<sid>.json` + restart) and confirm the scraper gate: first `/scraper/stream` call re-resolves (new sid in the URL) instead of returning the dead cached one.

- [ ] **Step 3: Deploy-hygiene check (spec ops note)**

Diagnose why the prometheus-only commit recreated stealth-scraper on 2026-07-10 03:33 UTC: `docker compose -f docker/docker-compose.yml config --hash stealth-scraper` before/after that commit (or inspect what `make redeploy-*`/after-update ran in that session's changelog entry). Record the answer + fix (e.g. after-update should scope `redeploy-<svc>` to services whose files changed) in `docs/issues/` if it's a real bug.

- [ ] **Step 4: After-update skill**

Run `/animeenigma-after-update` (simplify → lint/build → redeploy → health → Trump-mode changelog → commit/push). The changelog entry covers BOTH layers (edge rotation already shipped as AUTO-562 — mention the resurrection as the completing half).

---

## Self-review (done at write time)

- **Spec coverage:** persist ✔ (T1/T2) · rehydrate ✔ (T3) · warming skip + build rule ✔ (T4) · alive endpoint ✔ (T5) · scraper gate ✔ (T6) · deploy hygiene + live verify ✔ (T7) · Layer A — already on main (`0a8936b9`), excluded by design.
- **Placeholder scan:** Task 6's gogoanime test lists exact cases but delegates helper reuse to the existing file's fixtures — acceptable because the fixtures already exist in-repo; everything else is complete code.
- **Type consistency:** `SessionStore.save(dict)`/`load->dict|None` used identically in T1–T5; `session_state` strings match the Go switch in T6; `lease(preferred=)` defined in T3 before use.
