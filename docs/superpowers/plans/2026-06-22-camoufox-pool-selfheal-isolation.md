# Camoufox Pool Self-Heal + Isolation + Provider Auto-Resurrection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the stealth-scraper Camoufox sidecar self-healing and fault-isolated so a flaky browser provider (nineanime) can no longer crash or starve a healthy one (gogoanime), the pool revives crashed slots without a container restart, capacity is governed by a RAM budget, per-user consumption is bounded, and a recovered provider is automatically resurrected (degraded → enabled) — closing maintenance escalation AUTO-527.

**Architecture:** Five components across four services delivered in five independently deployable phases. Phase 1 (sidecar self-heal — poison-fence the warm session, mark/resurrect crashed slots in the reaper, split `/healthz`↔`/readyz`, carry a machine-readable `kind` in 503s) alone closes AUTO-527's collateral damage. Phase 2 swaps the fixed instance pool for a RAM budget (soft 4 GB / hard 6 GB) and adds a per-user quota threaded as `user_key` from catalog → scraper → sidecar. Phases 3–5 add the Go-scraper circuit breaker + orchestrator runtime re-gate, a catalog status-write endpoint, and the analytics probe writeback that drives durable auto-resurrection.

**Tech Stack:** Python 3 (FastAPI, Camoufox/Playwright, `prometheus_client`, dependency-free `/proc` RSS sampling) for the sidecar; Go (chi router, GORM, `libs/errors`, `libs/metrics`, `libs/logger`) for catalog/scraper/analytics. Tests: `python3 -m pytest` (unittest.TestCase classes) for the sidecar; `go test ./...` (table-driven, handwritten fakes) for Go.

**Design spec:** `docs/superpowers/specs/2026-06-22-camoufox-pool-selfheal-isolation-design.md`

## Global Constraints

- **Worktree:** all work happens in the worktree `/data/ae-camoufox` on branch `feat/camoufox-pool-selfheal` (off `origin/main`). Never edit the base tree `/data/animeenigma`.
- **Commit trailers:** EVERY commit ends with the three co-author trailers (already inlined in each task's commit step):
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Line numbers are indicative, not literal.** Cited `file:line` ranges are the pre-Phase-1 baseline; earlier phases shift them. Locate every edit site by SYMBOL (function/struct/route name), not by line number.
- **Tests never hit live external APIs.** Mock the sidecar / catalog / any HTTP. Match each package's existing test style — read a sibling `_test.go` (Go) or existing `tests/test_*.py` (Python) BEFORE writing a new test. No testify/mock unless the package already uses it.
- **Sidecar error sentinel (CORRECTION):** the "provider down" sentinel for failover is **`services/scraper/internal/domain.ErrProviderDown`** (+ `domain.WrapProviderDown`), NOT `libs/errors` — `libs/errors` has no `ErrProviderDown`. All Phase-3 wedged-error wrapping uses the `domain` sentinel.
- **Sidecar 503 `kind` values** (same JSON body shape as the existing `app/main.py` error bodies — `{success:false, error, kind}`): `provider_wedged`, `pool_exhausted` (Phase 1); `capacity`, `user_quota` (Phase 2). The legacy `exhausted` string is renamed to `pool_exhausted` in Phase 1 (no consumer asserts the old value; Go does not read `kind` until Phase 3).
- **In `app/main.py`, the `ProviderWedged` / `PoolExhausted` / `CapacityExceeded` / `UserQuotaExceeded` except-arms MUST precede the generic `except RecipeError` arm** in both `/resolve` and `/fetch` (they all subclass `RecipeError`; a wrong order collapses the `kind`).
- **Catalog status-write:** `POST /internal/scraper/providers/{name}/status`, body `{status, reason}`, only `enabled ↔ degraded` writable, refuses setting/changing `disabled` → 409, unknown → 404, audited, **Docker-network-only** (never gateway-proxied).
- **Probe autogate** is gated by `PROBE_AUTOGATE_ENABLED` (default `true`); auto `enabled ↔ degraded` only; `disabled` stays human-only.
- **Thresholds (verbatim):** `POISON_MAX=2`; resurrect backoff `1→2→4→8→16→30s` (cap), retire-after-3-failed-resurrects; `/readyz` 503 after `free==0` sustained ≥ `15s`; `STEALTH_RAM_SOFT_BYTES=4294967296`, `STEALTH_RAM_HARD_BYTES=6442450944`, `STEALTH_RAM_SAMPLE_SECONDS=5`; `STEALTH_USER_QUOTA=2`; compose `mem_limit 3500m→7g` (confirm host RAM headroom first).
- **Three "read the real file first" action items** the drafters could not fully verify — the implementer MUST open these before writing the listed tests:
  1. **Phase 2 Go plumbing tests:** match the REAL fake structs/helpers in `services/catalog/internal/service/scraper_test.go` and `services/catalog/internal/handler/scraper_test.go` (fake field names were assumed).
  2. **Phase 5 `fetchProviderStatuses`:** verify the exact JSON envelope key (`data.providers` vs `providers`) returned by `GET /internal/scraper/providers` before parsing it.
  3. **Phase 5 wiring:** verify the real `handler.NewProbeHandler` / `probe.Engine` constructor signature before inserting the autogating-reporter decorator.
- **Deploy note:** `libs/metrics` is a shared module; Phases 3 and 5 add new metric vars (additive — no break). Per-phase deploy rebuilds only the target service (`stealth-scraper` → `scraper` → `catalog` → `analytics`).
- **No time-effort units** anywhere. Feature scoring: **UXΔ = +3 (Better) · CDI = 0.08 * 34 · MVQ = Phoenix 88%/85%** 🔥.

---

## Phase 1: Sidecar self-heal

**Goal:** Make the Camoufox pool self-healing and fault-isolated so a wedged provider (nineanime) can no longer crash or starve a healthy one (gogoanime): poison-fence the warm session, mark crashed slots, resurrect them in the 30 s reaper without a container restart, split `/healthz` (process liveness, always 200) from `/readyz` (saturation observability, 503 on sustained `free==0`), expand `health()` to a `{global,providers,users}` breakdown, and carry a machine-readable `kind` in 503 bodies. Phase 1 alone closes AUTO-527's collateral damage.

All Python work lives in `services/stealth-scraper/`. Tests are `unittest.TestCase` classes run via `python3 -m pytest tests/` from the service root (verified: `python3 -m pytest tests/test_engine_lifecycle.py -q` → `7 passed`). Match the existing fixture style exactly: a top-level `run(coro)` wrapping `asyncio.run`, fake `_Page` objects with `async def evaluate(self, js, url)` and `async def close(self)`, and direct construction of `CamoufoxEngine(Config(pool_size=..., warming_enabled=False))`. No testify/httpx/TestClient — call the route handlers (`m.resolve(...)`, `m.fetch(...)`) directly and `json.loads(resp.body)`.

---

### Task P1.1 — Self-heal config knobs

**Files:**
- `services/stealth-scraper/app/config.py:32-99` (add 5 fields to the `Config` dataclass body, after `reaper_interval_seconds`)
- `services/stealth-scraper/app/config.py:101-130` (add 5 parsed assignments in `from_env`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestSelfHealConfig` class)

**Interfaces:**
- Produces: `Config.poison_max: int = 2`, `Config.readyz_saturation_seconds: float = 15.0`, `Config.resurrect_backoff_base_seconds: float = 1.0`, `Config.resurrect_backoff_cap_seconds: float = 30.0`, `Config.resurrect_max_fails: int = 3`
- Consumes: existing `_int` / `from_env` env-parsing pattern (config.py:19-23,101-130)

**Steps:**

1. Write the failing test. Append to `services/stealth-scraper/tests/test_engine_lifecycle.py`:

```python
class TestSelfHealConfig(unittest.TestCase):
    def test_defaults(self):
        cfg = Config()
        self.assertEqual(cfg.poison_max, 2)
        self.assertEqual(cfg.readyz_saturation_seconds, 15.0)
        self.assertEqual(cfg.resurrect_backoff_base_seconds, 1.0)
        self.assertEqual(cfg.resurrect_backoff_cap_seconds, 30.0)
        self.assertEqual(cfg.resurrect_max_fails, 3)

    def test_env_overrides(self):
        cfg = Config.from_env({
            "STEALTH_POISON_MAX": "4",
            "STEALTH_READYZ_SATURATION_SECONDS": "30",
            "STEALTH_RESURRECT_BACKOFF_BASE_SECONDS": "2",
            "STEALTH_RESURRECT_BACKOFF_CAP_SECONDS": "60",
            "STEALTH_RESURRECT_MAX_FAILS": "5",
        })
        self.assertEqual(cfg.poison_max, 4)
        self.assertEqual(cfg.readyz_saturation_seconds, 30.0)
        self.assertEqual(cfg.resurrect_backoff_base_seconds, 2.0)
        self.assertEqual(cfg.resurrect_backoff_cap_seconds, 60.0)
        self.assertEqual(cfg.resurrect_max_fails, 5)
```

2. Run (expected **FAIL** — fields don't exist yet):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestSelfHealConfig -q
# AttributeError: 'Config' object has no attribute 'poison_max'
```

3. Implement. In `app/config.py`, insert after the `reaper_interval_seconds: float = 30.0` field (config.py:99), inside the `Config` dataclass:

```python
    # -- self-heal (Phase 1) ------------------------------------------------ #
    # After this many consecutive in-page-fetch crashes ("Target closed" /
    # "context was destroyed") on the SAME warm session, the profile is torn
    # down and the caller fails over instead of nav-retrying a poisoned page.
    poison_max: int = 2
    # /readyz returns 503 only after the pool has been saturated (free==0) for
    # at least this long — a transient burst does not flip readiness.
    readyz_saturation_seconds: float = 15.0
    # Crashed-slot resurrection backoff: base * 2**consecutive_fail, capped.
    # 1 -> 2 -> 4 -> 8 -> 16 -> 30 (cap).
    resurrect_backoff_base_seconds: float = 1.0
    resurrect_backoff_cap_seconds: float = 30.0
    # After this many consecutive failed resurrect attempts a crashed slot is
    # retired (its user_data_dir wiped) instead of revived again.
    resurrect_max_fails: int = 3
```

In `from_env` (config.py:105-130), add these to the `return cls(...)` kwargs after `reaper_interval_seconds=...`:

```python
            poison_max=_int(g("STEALTH_POISON_MAX"), 2),
            readyz_saturation_seconds=float(
                _int(g("STEALTH_READYZ_SATURATION_SECONDS"), 15)
            ),
            resurrect_backoff_base_seconds=float(
                _int(g("STEALTH_RESURRECT_BACKOFF_BASE_SECONDS"), 1)
            ),
            resurrect_backoff_cap_seconds=float(
                _int(g("STEALTH_RESURRECT_BACKOFF_CAP_SECONDS"), 30)
            ),
            resurrect_max_fails=_int(g("STEALTH_RESURRECT_MAX_FAILS"), 3),
```

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestSelfHealConfig -q
# 2 passed
```

5. Commit:
```
git add services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): self-heal config knobs (poison/readyz/resurrect)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
(`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>` expands to the three standard co-author trailers.)

---

### Task P1.2 — Self-heal metrics

**Files:**
- `services/stealth-scraper/app/metrics.py:57-61` (append 3 new metric defs after `BROWSER_RELAUNCH_TOTAL`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestSelfHealMetrics` class)

**Interfaces:**
- Produces: `metrics.POOL_FREE` (Gauge), `metrics.POOL_CRASHED` (Gauge), `metrics.SLOT_RESURRECT_TOTAL` (Counter, label `result`)
- Consumes: `prometheus_client.Counter/Gauge` already imported (metrics.py:9)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestSelfHealMetrics(unittest.TestCase):
    def test_metrics_exist(self):
        from app import metrics
        # Gauges accept .set(); the resurrect counter is labelled by result.
        metrics.POOL_FREE.set(3)
        metrics.POOL_CRASHED.set(1)
        metrics.SLOT_RESURRECT_TOTAL.labels(result="ok").inc()
        metrics.SLOT_RESURRECT_TOTAL.labels(result="fail").inc()
        names = {m.name for m in metrics.SLOT_RESURRECT_TOTAL.collect()}
        self.assertIn("stealth_slot_resurrect", names)
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestSelfHealMetrics -q
# AttributeError: module 'app.metrics' has no attribute 'POOL_FREE'
```

3. Implement. Append to `app/metrics.py` after `BROWSER_RELAUNCH_TOTAL` (metrics.py:61):

```python
POOL_FREE = Gauge(
    "stealth_pool_free",
    "Browser profiles currently free (status=healthy and not leased).",
)

POOL_CRASHED = Gauge(
    "stealth_pool_crashed",
    "Browser profiles currently marked crashed (awaiting resurrection).",
)

SLOT_RESURRECT_TOTAL = Counter(
    "stealth_slot_resurrect_total",
    "Crashed-slot resurrection attempts in the reaper, by result.",
    ["result"],  # ok|fail|retired
)
```

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestSelfHealMetrics -q
# 1 passed
```

5. Commit:
```
git add services/stealth-scraper/app/metrics.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): self-heal metrics (pool_free/pool_crashed/slot_resurrect)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.3 — Profile health state (status / consecutive_fail / last_crash) + ProfileManager helpers

**Files:**
- `services/stealth-scraper/app/profiles.py:20-34` (extend `Profile` dataclass)
- `services/stealth-scraper/app/profiles.py:37-82` (add `mark_crashed` / `mark_healthy` / `crashed_idle` / `status_counts` to `ProfileManager`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestProfileHealth` class)

**Interfaces:**
- Produces: `Profile.status: str = "healthy"` (one of `healthy|crashed|warming`), `Profile.consecutive_fail: int = 0`, `Profile.last_crash: float = 0.0`, `Profile.next_resurrect_at: float = 0.0`; `ProfileManager.mark_crashed(profile, *, error="")`, `ProfileManager.mark_healthy(profile)`, `ProfileManager.crashed_idle() -> list[Profile]`, `ProfileManager.status_counts() -> dict[str,int]`
- Consumes: existing `Profile` / `ProfileManager` (profiles.py)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestProfileHealth(unittest.TestCase):
    def test_mark_crashed_and_healthy(self):
        from app.profiles import ProfileManager
        pm = ProfileManager("/tmp/ss-health-test", size=2)
        p = pm.all()[0]
        self.assertEqual(p.status, "healthy")
        pm.mark_crashed(p, error="Target closed")
        self.assertEqual(p.status, "crashed")
        self.assertEqual(p.consecutive_fail, 1)
        self.assertGreater(p.last_crash, 0.0)
        # crashed_idle lists crashed, not-leased slots only.
        self.assertIn(p, pm.crashed_idle())
        p.leased = True
        self.assertNotIn(p, pm.crashed_idle())
        p.leased = False
        pm.mark_crashed(p, error="again")
        self.assertEqual(p.consecutive_fail, 2)
        pm.mark_healthy(p)
        self.assertEqual(p.status, "healthy")
        self.assertEqual(p.consecutive_fail, 0)
        self.assertNotIn(p, pm.crashed_idle())

    def test_status_counts(self):
        from app.profiles import ProfileManager
        pm = ProfileManager("/tmp/ss-counts-test", size=3)
        pm.mark_crashed(pm.all()[0])
        counts = pm.status_counts()
        self.assertEqual(counts["crashed"], 1)
        self.assertEqual(counts["healthy"], 2)
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestProfileHealth -q
# AttributeError: 'Profile' object has no attribute 'status'
```

3. Implement. In `app/profiles.py`, extend the `Profile` dataclass (after `user_agent: str = ""`, profiles.py:30):

```python
    user_agent: str = ""
    # -- self-heal health (Phase 1) ----------------------------------------- #
    # "healthy": usable; "crashed": browser died, awaiting reaper resurrect;
    # "warming": resurrect in progress (transient, set by the reaper).
    status: str = "healthy"
    consecutive_fail: int = 0   # failed resurrect attempts in a row
    last_crash: float = 0.0     # time.time() of the most recent crash mark
    next_resurrect_at: float = 0.0  # earliest time the reaper may retry this slot
```

Add `import time` at the top of `profiles.py` (after `import os`, profiles.py:15):

```python
import os
import time
```

Add the four methods to `ProfileManager` (after `reset_uses`, profiles.py:81-82):

```python
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
```

Also add a `last_error: str = ""` field to `Profile` so `mark_crashed(error=...)` has somewhere to store it (insert directly after `next_resurrect_at`):

```python
    next_resurrect_at: float = 0.0  # earliest time the reaper may retry this slot
    last_error: str = ""            # most recent crash error string (for health())
```

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestProfileHealth -q
# 2 passed
```

5. Commit:
```
git add services/stealth-scraper/app/profiles.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): profile health state + ProfileManager crash helpers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.4 — `Session` crash fields + `ProviderWedged` exception + poison-fenced `_in_page_fetch` (no nav-retry; POISON_MAX teardown)

**Files:**
- `services/stealth-scraper/app/engine.py:79-101` (add `crash_count` / `last_error` / `provider` to `Session`)
- `services/stealth-scraper/app/engine.py:602-636` (rewrite `_in_page_fetch` poison handling)
- `services/stealth-scraper/app/engine.py:711-729` (add `ProviderWedged` next to `PoolExhausted` / `FetchTimeout`)
- `services/stealth-scraper/tests/test_engine_fetch.py` (new `TestPoisonFence` class)

**Interfaces:**
- Produces: `Session.crash_count: int = 0`, `Session.last_error: str = ""`, `Session.provider: str = ""`; `engine.ProviderWedged(RecipeError)` with `.provider` attr
- Consumes: `Config.poison_max` (Task P1.1), `aclose_session` (engine.py:647), `_teardown` (engine.py:233), `FetchTimeout` / `SessionGone` (engine.py:711-729), `metrics.BROWSER_RELAUNCH_TOTAL` (metrics.py:57)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_fetch.py`:

```python
class _PoisonPage:
    """evaluate() raises 'Target closed' to simulate a poisoned warm page.
    A liveness probe `()=>1` evaluation (no url arg) returns 1 so we can tell
    the poison-fence apart from the liveness probe."""
    url = "https://9anime.me.uk/"

    def __init__(self):
        self.calls = 0
        self.gotos = 0

    async def evaluate(self, js, *args):
        # Liveness probe: `()=>1` takes no url arg.
        if not args:
            return 1
        self.calls += 1
        raise RuntimeError("Target closed")

    async def goto(self, *a, **k):
        self.gotos += 1

    async def close(self):
        pass


def _engine_poison_session(poison_max=2):
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False, poison_max=poison_max))
    prof = eng.profiles.lease()
    page = _PoisonPage()
    key = "fetch::nineanime::https://9anime.me.uk"
    sess = Session(
        id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
        user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
        expires_at=time.time() + 600, page=page, player_url=page.url, provider="nineanime",
    )
    eng._sessions[key] = sess
    return eng, sess, page


class TestPoisonFence(unittest.TestCase):
    def test_no_nav_retry_on_target_closed(self):
        from app.engine import ProviderWedged
        eng, sess, page = _engine_poison_session(poison_max=2)
        # First crash: increments crash_count, does NOT nav-retry, raises.
        with self.assertRaises(Exception):
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))
        self.assertEqual(sess.crash_count, 1)
        self.assertEqual(page.gotos, 0, "must NOT re-navigate the poisoned page")
        self.assertIn(sess.id, eng._sessions, "below poison_max: session retained")

    def test_poison_max_tears_down_and_wedges(self):
        from app.engine import ProviderWedged
        eng, sess, page = _engine_poison_session(poison_max=2)
        with self.assertRaises(Exception):
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))   # crash_count=1
        with self.assertRaises(ProviderWedged) as ctx:
            run(eng._in_page_fetch(sess, "https://9anime.me.uk/x"))   # crash_count=2 -> wedge
        self.assertEqual(ctx.exception.provider, "nineanime")
        self.assertNotIn(sess.id, eng._sessions, "wedged session must be closed")
        self.assertEqual(sess.profile.status, "crashed", "slot marked crashed for the reaper")
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_fetch.py::TestPoisonFence -q
# ImportError: cannot import name 'ProviderWedged' from 'app.engine'
```

3. Implement.

(a) Extend the `Session` dataclass (after `in_use: int = 0`, engine.py:101):

```python
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
```

(b) Add the `provider` kwarg where warm sessions are built. In `_warm_fetch_session` (engine.py:586-591) add `provider=provider,` to the `Session(...)` constructor:

```python
            session = Session(
                id=key, profile=profile, proxy_id=proxy.id, referer=origin,
                user_agent=profile.user_agent, cdn_host=host_of(origin),
                master_url=origin, expires_at=time.time() + self.cfg.session_ttl_seconds,
                page=page, player_url=origin, provider=provider,
            )
```

(c) Add the `ProviderWedged` exception class, right after `PoolExhausted` (engine.py:717-720):

```python
class ProviderWedged(RecipeError):
    """A warm session for a provider has poisoned itself (>= cfg.poison_max
    in-page-fetch crashes). The profile is torn down + marked crashed for the
    reaper; the caller fails over. Distinct from a plain RecipeError so the API
    maps it to 503 {kind:"provider_wedged"} and the Go breaker can attribute the
    wedge to a provider (it carries .provider)."""

    def __init__(self, message: str, provider: str = ""):
        super().__init__(message)
        self.provider = provider
```

(d) Rewrite `_in_page_fetch` (engine.py:602-636) — replace the nav-retry branch with poison-fence:

```python
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
```

> Note for assembler: `_teardown` (Task P1.5) is updated to mark the slot crashed when `reason == "crash"`; until then this raises `ProviderWedged` but the slot won't yet be `status="crashed"`. P1.4 and P1.5 land in order; P1.4's second test asserts `status == "crashed"`, so P1.5's `_teardown` change is a hard dependency of that assertion. If landing P1.4 alone, gate that one assertion behind P1.5. (Recommended: land P1.4 + P1.5 as one reviewable unit.)

4. Run (expected **PASS** once P1.5's `_teardown` is in; if running P1.4 in isolation the `status == "crashed"` assert needs P1.5):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_fetch.py::TestPoisonFence -q
```

5. Commit (combined with P1.5 — see P1.5 step 5).

---

### Task P1.5 — `_teardown(reason='crash')` marks slot crashed + grace-wait on relaunch

**Files:**
- `services/stealth-scraper/app/engine.py:233-242` (rewrite `_teardown`)
- `services/stealth-scraper/app/engine.py:104-126` (add a grace-sleep after `handle.close()`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestTeardownMarksCrashed` class)

**Interfaces:**
- Produces: `_teardown(profile, *, reason)` marks the profile `crashed` (via `profiles.mark_crashed`) iff `reason == "crash"`, leaves it `healthy`/untouched otherwise
- Consumes: `ProfileManager.mark_crashed` (Task P1.3), `metrics.BROWSER_RELAUNCH_TOTAL` (metrics.py:57)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestTeardownMarksCrashed(unittest.TestCase):
    def _eng(self):
        return CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

    def test_crash_reason_marks_slot_crashed(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        run(eng._teardown(p, reason="crash"))
        self.assertEqual(p.status, "crashed")
        self.assertEqual(p.consecutive_fail, 1)

    def test_non_crash_reason_does_not_mark_crashed(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        run(eng._teardown(p, reason="rotate"))
        self.assertEqual(p.status, "healthy")
        run(eng._teardown(p, reason="recycle"))
        self.assertEqual(p.status, "healthy")
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestTeardownMarksCrashed -q
# AssertionError: 'healthy' != 'crashed'
```

3. Implement.

(a) Rewrite `_teardown` (engine.py:233-242):

```python
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
```

(b) Add the grace-wait after `handle.close()` in `_CamoufoxHandle.close` (engine.py:120-126) so a relaunch does not inherit a half-dead WebSocket. Replace the body:

```python
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
```

(`asyncio` is already imported at engine.py:19.)

4. Run (expected **PASS** — and now P1.4's `TestPoisonFence::test_poison_max_tears_down_and_wedges` also passes):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestTeardownMarksCrashed tests/test_engine_fetch.py::TestPoisonFence -q
# 5 passed
```

5. Commit (P1.4 + P1.5 together):
```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_fetch.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): poison-fence warm session + crash-teardown marks slot for reaper

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.6 — Poison-fence the warm-session REUSE path (liveness probe before reuse)

**Files:**
- `services/stealth-scraper/app/engine.py:549-555` (rewrite the reuse check in `_warm_fetch_session`)
- `services/stealth-scraper/tests/test_engine_fetch.py` (new `TestWarmReuseLiveness` class)

**Interfaces:**
- Produces: `_warm_fetch_session` evicts + recreates a warm session whose page fails a cheap `await page.evaluate('()=>1')` liveness probe, instead of inheriting a poisoned page
- Consumes: `aclose_session` (engine.py:647), `_acquire_profile` (engine.py:377)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_fetch.py`:

```python
class _DeadProbePage:
    """Liveness probe `()=>1` raises (page is dead); used to assert the warm-
    reuse path evicts the session rather than handing back a poisoned page."""
    url = "https://9anime.me.uk/"

    def __init__(self):
        self.probed = False

    async def evaluate(self, js, *args):
        if not args:  # liveness probe
            self.probed = True
            raise RuntimeError("Target closed")
        return 1

    async def close(self):
        pass


class TestWarmReuseLiveness(unittest.TestCase):
    def test_dead_warm_session_is_evicted_not_reused(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        prof = eng.profiles.lease()
        page = _DeadProbePage()
        key = "fetch::nineanime::https://9anime.me.uk"
        sess = Session(
            id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
            user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
            expires_at=time.time() + 600, page=page, player_url=page.url, provider="nineanime",
        )
        eng._sessions[key] = sess
        # The dead session must NOT be returned by the reuse fast-path. Since the
        # pool has no free profile after the dead one is released, recreation
        # will raise PoolExhausted/RecipeError — but the dead session is gone.
        from app.engine import PoolExhausted
        with self.assertRaises((PoolExhausted, RecipeError)):
            run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk"))
        self.assertTrue(page.probed, "reuse path must run the liveness probe")
        self.assertNotIn(key, eng._sessions, "dead warm session must be evicted")
```

2. Run (expected **FAIL** — current code returns the dead `existing` without probing):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_fetch.py::TestWarmReuseLiveness -q
# AssertionError: False is not true (probe never ran)  /  dead session returned
```

3. Implement. Rewrite the reuse fast-path in `_warm_fetch_session` (engine.py:552-555):

```python
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
```

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_fetch.py::TestWarmReuseLiveness tests/test_engine_fetch.py -q
# (all fetch tests pass; existing reuse test still green: live probe returns 1)
```

> Assembler note: the existing `test_session_reused_per_origin` (test_engine_fetch.py:67) uses `_FetchPage.evaluate(self, js, url)` — a **2-arg** signature. The liveness probe calls `evaluate("()=>1")` with **no** url arg, which would raise `TypeError` on `_FetchPage` and (wrongly) evict the session. **Fix the existing fake** as part of this task: change `_FetchPage.evaluate` and `_PoisonPage`-style fakes to `async def evaluate(self, js, *args)` and return `1` when `not args`. Update `_FetchPage` (test_engine_fetch.py:27-29):

```python
    async def evaluate(self, js, *args):
        if not args:      # liveness probe `()=>1`
            return 1
        self.calls += 1
        return f"{self._status}|{self._ctype}|{args[0]}|{base64.b64encode(self._body).decode()}"
```

5. Commit:
```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_fetch.py
git commit -m "feat(stealth): liveness-probe warm session before reuse (no poisoned-page handback)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.7 — Reaper resurrection of crashed slots (exp backoff + retire-after-3)

**Files:**
- `services/stealth-scraper/app/engine.py:694-708` (extend `_reap` to call `_resurrect_crashed_slot`)
- `services/stealth-scraper/app/engine.py` (new `_resurrect_crashed_slot` + `_resurrect_backoff` helpers, after `_reap`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestReaperResurrection` class)

**Interfaces:**
- Produces: `_resurrect_crashed_slot(profile) -> None` (cold-relaunches a crashed, not-in-use slot past its backoff; success → `mark_healthy` + `SLOT_RESURRECT_TOTAL{result="ok"}`; failure → re-`mark_crashed` + backoff bump + `{result="fail"}`; after `cfg.resurrect_max_fails` → retire (`_rm_dir` + `reset_uses` + `mark_healthy`) + `{result="retired"}`); `_resurrect_backoff(consecutive_fail) -> float`
- Consumes: `_ensure_browser` (engine.py:192), `_teardown` (Task P1.5), `_rm_dir` (engine.py:732), `ProfileManager.mark_healthy`/`mark_crashed`/`crashed_idle` (Task P1.3), `metrics.SLOT_RESURRECT_TOTAL` (Task P1.2), `Config.resurrect_*` (Task P1.1)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestReaperResurrection(unittest.TestCase):
    def _eng(self):
        eng = CamoufoxEngine(Config(
            pool_size=1, warming_enabled=False,
            resurrect_backoff_base_seconds=1, resurrect_backoff_cap_seconds=30,
            resurrect_max_fails=3,
        ))
        return eng

    def test_backoff_curve(self):
        eng = self._eng()
        self.assertEqual(eng._resurrect_backoff(0), 1)
        self.assertEqual(eng._resurrect_backoff(1), 2)
        self.assertEqual(eng._resurrect_backoff(2), 4)
        self.assertEqual(eng._resurrect_backoff(3), 8)
        self.assertEqual(eng._resurrect_backoff(4), 16)
        self.assertEqual(eng._resurrect_backoff(5), 30)   # capped
        self.assertEqual(eng._resurrect_backoff(9), 30)   # still capped

    def test_successful_resurrect_marks_healthy(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        eng.profiles.mark_crashed(p)            # consecutive_fail=1, status=crashed
        p.next_resurrect_at = 0.0               # eligible now

        async def _ok_launch(profile, proxy_id):
            return object()
        eng._ensure_browser = _ok_launch

        run(eng._resurrect_crashed_slot(p))
        self.assertEqual(p.status, "healthy")
        self.assertEqual(p.consecutive_fail, 0)

    def test_resurrect_respects_backoff(self):
        eng = self._eng()
        p = eng.profiles.all()[0]
        eng.profiles.mark_crashed(p)
        p.next_resurrect_at = time.time() + 999  # not eligible yet
        launched = {"n": 0}

        async def _count(profile, proxy_id):
            launched["n"] += 1
            return object()
        eng._ensure_browser = _count

        run(eng._resurrect_crashed_slot(p))
        self.assertEqual(launched["n"], 0, "must not attempt before backoff elapses")
        self.assertEqual(p.status, "crashed")

    def test_retire_after_three_failures(self):
        eng = self._eng()
        p = eng.profiles.all()[0]

        async def _boom(profile, proxy_id):
            raise RuntimeError("relaunch failed")
        eng._ensure_browser = _boom

        # 3 failed resurrects -> retire (status reset to healthy, uses zeroed).
        for _ in range(3):
            eng.profiles.mark_crashed(p)
            p.next_resurrect_at = 0.0
            run(eng._resurrect_crashed_slot(p))
        # After the 3rd failure the slot is retired -> healthy + fail counter 0.
        self.assertEqual(p.status, "healthy")
        self.assertEqual(p.consecutive_fail, 0)
        self.assertEqual(p.uses, 0)
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestReaperResurrection -q
# AttributeError: 'CamoufoxEngine' object has no attribute '_resurrect_backoff'
```

3. Implement. Add the two helpers after `_reap` (engine.py:708), and wire a resurrection sweep into `_reap`.

First, extend `_reap` — append after the profile-retire loop (engine.py:704-708):

```python
        # Resurrect crashed, not-in-use slots (background self-heal — no
        # container restart). Each is gated by its own exponential backoff.
        for p in self.profiles.crashed_idle():
            await self._resurrect_crashed_slot(p)
        self._publish_pool_gauges()
```

Then add the helpers:

```python
    def _resurrect_backoff(self, consecutive_fail: int) -> float:
        """Exponential per-slot backoff: base * 2**(fail-1), capped. fail==0/1 ->
        base; fail==2 -> 2*base; ... clamped to resurrect_backoff_cap_seconds.
        (1 -> 2 -> 4 -> 8 -> 16 -> 30 with base=1, cap=30.)"""
        base = self.cfg.resurrect_backoff_base_seconds
        cap = self.cfg.resurrect_backoff_cap_seconds
        steps = max(0, consecutive_fail - 1)
        return min(cap, base * (2 ** steps))

    async def _resurrect_crashed_slot(self, profile: Profile) -> None:
        """Attempt a cold relaunch of one crashed, not-in-use slot. Skips slots
        still inside their backoff window. On success the slot returns to the
        healthy pool; on the cfg.resurrect_max_fails-th consecutive failure the
        slot is retired (user_data_dir wiped, counters reset) rather than revived
        forever. Never raises — a failed attempt counts toward retirement; the
        reaper loop must not die."""
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
            # Failed: re-mark crashed (bumps consecutive_fail), set the next
            # backoff, and retire if we've now hit the limit.
            await self._teardown(profile, reason="crash")  # mark_crashed -> fail+1
            profile.last_error = str(exc)
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
```

> Assembler note: in `_retire_crashed_slot`, `_teardown(reason="recycle")` does NOT call `mark_crashed`, so the subsequent `mark_healthy` is the terminal state — `status="healthy"`, `consecutive_fail=0`, `uses=0`. The `test_retire_after_three_failures` test exercises exactly this: 3rd failure → `_teardown(reason="crash")` bumps `consecutive_fail` to 3 → equals `resurrect_max_fails` → `_retire_crashed_slot` → healthy/0/0. Verified consistent.

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestReaperResurrection -q
# 4 passed
```

5. Commit:
```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): reaper resurrects crashed slots (exp backoff, retire-after-3)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.8 — Expand `health()` to `{global, providers, users}` + saturation tracking

**Files:**
- `services/stealth-scraper/app/engine.py:173-189` (rewrite `health()`)
- `services/stealth-scraper/app/engine.py:130-147` (add `_saturated_since` state to `__init__`)
- `services/stealth-scraper/app/engine.py` (new `is_ready()` helper)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestHealthShape` class)

**Interfaces:**
- Produces: `health()` returns `{"global": {...}, "providers": {...}, "users": {...}, ...legacy keys}`; `is_ready() -> bool` (False only after `free==0` sustained ≥ `cfg.readyz_saturation_seconds`)
- Consumes: `ProfileManager.status_counts`/`crashed_idle` (Task P1.3), `Session.provider`/`crash_count`/`last_error` (Task P1.4), `Config.readyz_saturation_seconds` (Task P1.1)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestHealthShape(unittest.TestCase):
    def test_health_breakdown(self):
        eng = CamoufoxEngine(Config(pool_size=2, warming_enabled=False))
        h = eng.health()
        self.assertIn("global", h)
        self.assertIn("providers", h)
        self.assertIn("users", h)
        g = h["global"]
        self.assertEqual(g["free"], 2)
        self.assertEqual(g["crashed"], 0)
        self.assertIn("warming", g)
        # legacy keys retained for back-compat consumers.
        self.assertEqual(h["status"], "ok")
        self.assertEqual(h["pool_size"], 2)

    def test_providers_breakdown_counts_warm_sessions(self):
        eng = CamoufoxEngine(Config(pool_size=2, warming_enabled=False))
        prof = eng.profiles.lease()
        page = _Page()
        sess = Session(
            id="fetch::nineanime::https://9anime.me.uk", profile=prof,
            proxy_id="d", referer="r", user_agent="UA", cdn_host="9anime.me.uk",
            master_url="https://9anime.me.uk", expires_at=time.time() + 600,
            page=page, player_url=page.url, provider="nineanime",
        )
        sess.last_error = "Target closed"
        eng._sessions[sess.id] = sess
        h = eng.health()
        self.assertEqual(h["providers"]["nineanime"]["held"], 1)
        self.assertEqual(h["providers"]["nineanime"]["last_error"], "Target closed")

    def test_is_ready_only_after_sustained_saturation(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    readyz_saturation_seconds=15))
        # Saturate: lease the only profile.
        eng.profiles.lease()
        # First observation: saturated NOW but window not elapsed -> still ready.
        self.assertTrue(eng.is_ready())
        # Simulate the saturation window having started 20s ago.
        eng._saturated_since = time.time() - 20
        self.assertFalse(eng.is_ready(), "sustained saturation -> not ready")
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestHealthShape -q
# KeyError: 'global'  /  AttributeError: 'CamoufoxEngine' object has no attribute 'is_ready'
```

3. Implement.

(a) Add saturation state to `__init__` (after `self._reaper_task = None`, engine.py:147):

```python
        self._reaper_task: Any = None
        # Monotonic-wall time the pool became saturated (free==0), or 0.0 when
        # not saturated. /readyz flips to 503 only once this persists past
        # cfg.readyz_saturation_seconds (a transient burst stays ready).
        self._saturated_since: float = 0.0
```

(b) Rewrite `health()` (engine.py:173-189):

```python
    def health(self) -> dict:
        # /healthz consumes this and ALWAYS returns 200 (process liveness) — the
        # body's per-provider/per-user breakdown is the observability surface;
        # /readyz (is_ready) is the saturation signal. The reaper + poison-fence
        # + 503-on-exhaustion self-heal the pool, so Docker must NOT restart-loop
        # the container on transient saturation.
        all_profiles = self.profiles.all()
        counts = self.profiles.status_counts()
        free = sum(1 for p in all_profiles if p.status == "healthy" and not p.leased)

        providers: dict[str, dict] = {}
        users: dict[str, dict] = {}
        for s in self._sessions.values():
            if s.provider:
                pv = providers.setdefault(
                    s.provider, {"held": 0, "crashed": 0, "last_error": ""}
                )
                pv["held"] += 1
                if s.crash_count:
                    pv["crashed"] += 1
                if s.last_error:
                    pv["last_error"] = s.last_error
            uk = getattr(s, "user_key", "") or ""
            if uk:
                users.setdefault(uk, {"held": 0})["held"] += 1

        return {
            # Legacy keys (back-compat for any existing scrape/consumer).
            "status": "degraded" if free == 0 else "ok",
            "pool_size": self.cfg.pool_size,
            "free_profiles": free,
            "live_browsers": len(self._handles),
            "active_sessions": len(self._sessions),
            "proxies": [
                {"id": e.id, "type": e.type, "blocked": e.total_blocked}
                for e in self.pool.all()
            ],
            # New self-heal breakdown (Phase 1).
            "global": {
                "free": free,
                "crashed": counts.get("crashed", 0),
                "warming": counts.get("warming", 0),
                "live_browsers": len(self._handles),
                "active_sessions": len(self._sessions),
            },
            "providers": providers,
            "users": users,
        }

    def is_ready(self) -> bool:
        """Readiness for /readyz. NOT a liveness signal (see D8 — never drives a
        restart; in-flight streams must keep playing). Returns False only when
        the pool has been saturated (no healthy free slot) continuously for at
        least cfg.readyz_saturation_seconds."""
        free = sum(
            1 for p in self.profiles.all() if p.status == "healthy" and not p.leased
        )
        now = time.time()
        if free > 0:
            self._saturated_since = 0.0
            return True
        if self._saturated_since == 0.0:
            self._saturated_since = now
        return (now - self._saturated_since) < self.cfg.readyz_saturation_seconds
```

> Assembler note: `health()` reads `getattr(s, "user_key", "")` — `Session.user_key` does NOT exist in Phase 1 (it arrives in Phase 2). The `getattr` default keeps `users` an empty dict until then, so the shape is forward-compatible with no Phase-1 field. The `TestHealthShape` user assertions only check `"users" in h` (empty dict), never a populated value.

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestHealthShape -q
# 3 passed
```

5. Commit:
```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): health() {global,providers,users} breakdown + is_ready saturation gate

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.9 — `/readyz` route (200 liveness split, 503 on sustained saturation) + `kind` in 503 bodies

**Files:**
- `services/stealth-scraper/app/main.py:81-84` (keep `/healthz` at 200; add `/readyz`)
- `services/stealth-scraper/app/main.py:92-120` (`/resolve`: `provider_wedged` + `pool_exhausted` kinds)
- `services/stealth-scraper/app/main.py:123-147` (`/fetch`: same)
- `services/stealth-scraper/app/main.py:26` (import `ProviderWedged`)
- `services/stealth-scraper/tests/test_engine_lifecycle.py` (new `TestReadyzAndKinds` class)

**Interfaces:**
- Produces: `GET /readyz` → 200 `{"ready": true}` / 503 `{"ready": false, ...health}`; `/resolve` + `/fetch` 503 bodies carry `kind` ∈ `{"pool_exhausted", "provider_wedged"}`; `/healthz` stays 200 always
- Consumes: `engine.is_ready()` / `engine.health()` (Task P1.8), `engine.ProviderWedged` (Task P1.4), `PoolExhausted` (engine.py:717)

**Steps:**

1. Write the failing test. Append to `tests/test_engine_lifecycle.py`:

```python
class TestReadyzAndKinds(unittest.TestCase):
    def _set_engine(self, engine):
        import app.main as m
        m.app.state.engine = engine
        return m

    def test_healthz_stays_200_when_saturated(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        eng.profiles.lease()                      # saturate
        m = self._set_engine(eng)
        out = run(m.healthz())
        # /healthz returns the health() dict directly (FastAPI -> 200).
        self.assertEqual(out["status"], "degraded")  # body says degraded...
        # ...but the route is 200 (no JSONResponse status override).

    def test_readyz_503_on_sustained_saturation(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    readyz_saturation_seconds=15))
        eng.profiles.lease()
        eng._saturated_since = time.time() - 20
        m = self._set_engine(eng)
        resp = run(m.readyz())
        self.assertEqual(resp.status_code, 503)
        self.assertFalse(json.loads(resp.body)["ready"])

    def test_readyz_200_when_free(self):
        import json
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        m = self._set_engine(eng)
        resp = run(m.readyz())
        self.assertEqual(resp.status_code, 200)
        self.assertTrue(json.loads(resp.body)["ready"])

    def test_resolve_pool_exhausted_kind(self):
        import json
        from app.main import ResolveRequest
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

        async def _none(*a, **k):
            return None
        eng._acquire_profile = _none
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "pool_exhausted")

    def test_resolve_provider_wedged_kind(self):
        import json
        from app.main import ResolveRequest
        from app.engine import ProviderWedged
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))

        async def _wedge(*a, **k):
            raise ProviderWedged("poisoned", provider="nineanime")
        eng.resolve = _wedge
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="nineanime")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "provider_wedged")
```

2. Run (expected **FAIL**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestReadyzAndKinds -q
# AttributeError: module 'app.main' has no attribute 'readyz'  /  kind == "exhausted" != "pool_exhausted"
```

3. Implement.

(a) Update the import (main.py:26):

```python
from .engine import CamoufoxEngine, FetchTimeout, PoolExhausted, ProviderWedged, SessionGone
```

(b) Add `/readyz` after `/healthz` (main.py:84):

```python
@app.get("/readyz")
async def readyz() -> JSONResponse:
    """Readiness (observability only — D8: does NOT drive a Docker/k8s restart).
    503 when the pool has been saturated continuously past the configured
    window; 200 otherwise. /healthz stays 200 for process liveness so in-flight
    streams keep playing while the pool self-heals."""
    engine: CamoufoxEngine = app.state.engine
    if engine.is_ready():
        return JSONResponse({"ready": True}, status_code=200)
    body = {"ready": False}
    body.update(engine.health())
    return JSONResponse(body, status_code=503)
```

(c) In `/resolve` (main.py:102-106), change the `PoolExhausted` handler `kind` and add a `ProviderWedged` handler **before** the `RecipeError` handler (since both `PoolExhausted` and `ProviderWedged` subclass `RecipeError`, order matters — they must be caught first):

```python
    except NotFoundError as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "not_found"}, status_code=404
        )
    except ProviderWedged as exc:
        # Warm session poisoned (>= poison_max crashes) — 503 (retryable) so the
        # Go orchestrator fails over; the Go breaker reads kind=provider_wedged.
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "provider_wedged"},
            status_code=503,
        )
    except PoolExhausted as exc:
        # Pool saturated — 503 (retryable) so the Go orchestrator fails over.
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "pool_exhausted"},
            status_code=503,
        )
    except ChallengeError as exc:
```

(d) Apply the identical change to `/fetch` (main.py:137-142) — add `ProviderWedged` before `PoolExhausted`, and rename the `PoolExhausted` kind:

```python
    except NotFoundError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "not_found"}, status_code=404)
    except ProviderWedged as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "provider_wedged"}, status_code=503)
    except PoolExhausted as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "pool_exhausted"}, status_code=503)
    except ChallengeError as exc:
```

> Assembler note: the spec's contract says Phase 1 503 kinds are exactly `provider_wedged` and `pool_exhausted`. This **renames** the pre-existing `kind:"exhausted"` (main.py:105,140) to `kind:"pool_exhausted"`. The Go `sidecar.Client` currently maps any 503 body to `ErrProviderDown` (it does not yet read `kind` until Phase 3), so this rename is non-breaking for the current Go side. Note this in the deploy notes for whoever lands Phase 3.

4. Run (expected **PASS**):
```
cd services/stealth-scraper && python3 -m pytest tests/test_engine_lifecycle.py::TestReadyzAndKinds -q
# 5 passed
```

5. Commit:
```
git add services/stealth-scraper/app/main.py services/stealth-scraper/tests/test_engine_lifecycle.py
git commit -m "feat(stealth): /readyz saturation route + provider_wedged/pool_exhausted kinds

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P1.10 — Full-suite green + Docker healthcheck note + changelog

**Files:**
- `services/stealth-scraper/Dockerfile:34` (verify the existing healthcheck still hits `/healthz` — confirm, do NOT switch it to `/readyz`, per D8)
- `frontend/web/changelog.full.json` (prepend a user-facing entry via `/animeenigma-after-update`)

**Interfaces:**
- Consumes: every prior Phase-1 task

**Steps:**

1. Run the **entire** stealth-scraper suite to confirm no regression in the existing tests (the `_Page`/`_FetchPage` fake-signature change in P1.6 is the main risk):
```
cd services/stealth-scraper && python3 -m pytest tests/ -q
# expected: ALL pass (the 7 original lifecycle tests + 6 fetch tests + new ones)
```
Expected: every test passes. If `test_session_reused_per_origin` or any `_Page`-based proxy test fails with a `TypeError` on `evaluate`, the `*args` fake-signature migration from P1.6 was not applied to that fake — fix the fake (not the engine).

2. Confirm the Docker healthcheck is unchanged and still targets `/healthz` (it must NOT move to `/readyz` — a saturated-but-live container must stay up so in-flight streams keep playing, D8):
```
grep -n "healthz\|readyz" services/stealth-scraper/Dockerfile
# expected: line 34 still curls /healthz, no /readyz reference
```
No code change here — this step is a guard assertion.

3. Run the after-update skill to lint, rebuild + redeploy `stealth-scraper`, health-check, and prepend the changelog entry:
```
/animeenigma-after-update
```
The changelog entry (Russian Trump-mode, prepended to `frontend/web/changelog.full.json`; the served `public/changelog.json` is regenerated by `scripts/changelog-trim.mjs`) should convey, factually: the EN-провайдеры теперь self-heal — один зависший провайдер (9anime) БОЛЬШЕ НЕ роняет здоровый (gogoanime); пул сам воскрешает мёртвые слоты БЕЗ перезапуска контейнера. No time-effort units anywhere.

4. Final verification gate before declaring Phase 1 done:
```
make health    # stealth-scraper healthy (200 on /healthz)
docker exec animeenigma-stealth-scraper python -c "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:3000/readyz').status)"  # 200 when pool free
curl -s http://127.0.0.1:3000/metrics | grep -E "stealth_pool_free|stealth_pool_crashed|stealth_slot_resurrect"  # new metrics present
```

5. Commit is handled by `/animeenigma-after-update` (it commits + pushes with co-authors). If running manually:
```
git add frontend/web/changelog.full.json frontend/web/public/changelog.json
git commit -m "docs(changelog): stealth-scraper self-heal (poison-fence + reaper resurrection)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase 2: RAM-budgeted capacity + per-user quota

**Goal:** Replace the fixed `STEALTH_POOL_SIZE` cap with a RAM-budgeted admission controller (soft 4 GB stop-warming/evict-idle, hard 6 GB refuse-launch/evict-LRU) and bound per-user consumption to ≤ 2 concurrent held sessions, threading an opaque `user_key` end-to-end from the catalog (authed user id, salted-IP fallback) through the scraper to the sidecar request body. Builds on Phase 1's self-heal pool; emits machine-readable `kind:"capacity"` / `kind:"user_quota"` 503 bodies in the existing error-body shape.

> Convention: `Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>` at the end of each commit step expands to the three standard co-author trailers.

---

### Task P2.1 — Config: RAM budget + user-quota knobs

**Files:**
- `services/stealth-scraper/app/config.py:32-130` (add fields + `from_env` parsing)
- `services/stealth-scraper/tests/test_config_capacity.py` (NEW)

**Interfaces:**
- Produces: `Config.ram_soft_bytes`, `Config.ram_hard_bytes`, `Config.ram_sample_seconds`, `Config.user_quota`.
- Consumes: existing `Config._int` / `from_env` helpers.

1. Write the failing test `services/stealth-scraper/tests/test_config_capacity.py`:

```python
"""Config parsing for the RAM budget + per-user quota knobs (Phase 2)."""
import unittest

from app.config import Config


class TestCapacityConfig(unittest.TestCase):
    def test_defaults(self):
        c = Config()
        self.assertEqual(c.ram_soft_bytes, 4_294_967_296)
        self.assertEqual(c.ram_hard_bytes, 6_442_450_944)
        self.assertEqual(c.ram_sample_seconds, 5.0)
        self.assertEqual(c.user_quota, 2)

    def test_from_env_overrides(self):
        c = Config.from_env({
            "STEALTH_RAM_SOFT_BYTES": "1000",
            "STEALTH_RAM_HARD_BYTES": "2000",
            "STEALTH_RAM_SAMPLE_SECONDS": "3",
            "STEALTH_USER_QUOTA": "5",
        })
        self.assertEqual(c.ram_soft_bytes, 1000)
        self.assertEqual(c.ram_hard_bytes, 2000)
        self.assertEqual(c.ram_sample_seconds, 3.0)
        self.assertEqual(c.user_quota, 5)

    def test_from_env_bad_values_fall_back_to_defaults(self):
        c = Config.from_env({"STEALTH_RAM_HARD_BYTES": "notint", "STEALTH_USER_QUOTA": ""})
        self.assertEqual(c.ram_hard_bytes, 6_442_450_944)
        self.assertEqual(c.user_quota, 2)


if __name__ == "__main__":
    unittest.main()
```

2. Run it — expected **FAIL** (`AttributeError: 'Config' object has no attribute 'ram_soft_bytes'`):

```
cd services/stealth-scraper && python -m pytest tests/test_config_capacity.py -q
```

3. Implement. In `app/config.py`, add four fields to the `Config` dataclass, immediately after the `reaper_interval_seconds` field (around line 99):

```python
    # RAM-budgeted capacity (Phase 2). The pool is governed by the COMBINED
    # Camoufox/Firefox RSS, not a fixed instance count. soft = stop warming +
    # evict idle sessions (back-pressure); hard = refuse a new browser launch
    # (503 kind=capacity) + evict the LRU not-in-use session to reclaim.
    # STEALTH_POOL_SIZE survives only as a high fail-safe ceiling used when the
    # /proc RSS read fails.
    ram_soft_bytes: int = 4 * 1024 * 1024 * 1024   # 4 GiB
    ram_hard_bytes: int = 6 * 1024 * 1024 * 1024   # 6 GiB
    ram_sample_seconds: float = 5.0
    # Max concurrent HELD sessions per user_key (fairness axis, Phase 2).
    user_quota: int = 2
```

In `from_env` (around line 129, after the `reaper_interval_seconds=` line), add:

```python
            ram_soft_bytes=_int(g("STEALTH_RAM_SOFT_BYTES"), 4 * 1024 * 1024 * 1024),
            ram_hard_bytes=_int(g("STEALTH_RAM_HARD_BYTES"), 6 * 1024 * 1024 * 1024),
            ram_sample_seconds=float(_int(g("STEALTH_RAM_SAMPLE_SECONDS"), 5)),
            user_quota=_int(g("STEALTH_USER_QUOTA"), 2),
```

4. Run it — expected **PASS**:

```
cd services/stealth-scraper && python -m pytest tests/test_config_capacity.py -q
```

5. Commit:

```
git add services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_config_capacity.py
git commit -m "feat(stealth): config knobs for RAM budget + per-user quota"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.2 — RAM sampler (dependency-free `/proc` RSS over the process tree)

**Files:**
- `services/stealth-scraper/app/ramsampler.py` (NEW)
- `services/stealth-scraper/tests/test_ramsampler.py` (NEW)

**Interfaces:**
- Produces: `tree_rss_bytes(root_pid, *, read_statm=..., read_stat=..., page_size=...) -> int`, `process_tree_rss(*, ...) -> int`.
- Consumes: nothing (pure stdlib `os`).

`camoufox` launches Firefox as child processes of THIS python process, so the budget is the RSS of `os.getpid()` plus every descendant. `/proc/<pid>/statm` field 2 (index 1) is `resident` in pages; multiply by `os.sysconf("SC_PAGE_SIZE")`. Children are discovered by scanning `/proc/<pid>/stat` field 4 (ppid). All read fns are injectable so the test never touches a real `/proc`. **No new pip package** (verified `requirements.txt`: camoufox, fastapi, uvicorn, prometheus-client, pydantic — none needed).

1. Write the failing test `services/stealth-scraper/tests/test_ramsampler.py`:

```python
"""RAM sampler: combined RSS of the Camoufox/Firefox process tree via /proc.
Dependency-free — all /proc reads are injected so the test never hits the real
filesystem."""
import unittest

from app.ramsampler import process_tree_rss, tree_rss_bytes


# Fake /proc: a tree root=100 -> {200, 300}; 300 -> {400}. RSS in PAGES.
_PPID = {100: 1, 200: 100, 300: 100, 400: 300, 999: 1}
_RSS_PAGES = {100: 10, 200: 20, 300: 30, 400: 40, 999: 5}


def _read_stat(pid):
    # mimic /proc/<pid>/stat: "pid (comm) state ppid ..." — comm may contain
    # spaces/parens, so the real parser must split on the LAST ')'.
    if pid not in _PPID:
        raise FileNotFoundError(pid)
    return f"{pid} (fire fox) S {_PPID[pid]} 1 1 0 -1"


def _read_statm(pid):
    if pid not in _RSS_PAGES:
        raise FileNotFoundError(pid)
    return f"1000 {_RSS_PAGES[pid]} 5 1 0 200 0"


def _all_pids():
    return list(_PPID.keys())


class TestRamSampler(unittest.TestCase):
    def test_tree_rss_sums_root_and_all_descendants(self):
        got = tree_rss_bytes(
            100, read_stat=_read_stat, read_statm=_read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        # 100+200+300+400 = (10+20+30+40) pages * 4096; 999 is unrelated → excluded
        self.assertEqual(got, (10 + 20 + 30 + 40) * 4096)

    def test_dead_pid_is_skipped_not_fatal(self):
        # 400 vanished mid-scan; the sum still covers the survivors.
        def read_statm(pid):
            if pid == 400:
                raise FileNotFoundError(pid)
            return _read_statm(pid)
        got = tree_rss_bytes(
            100, read_stat=_read_stat, read_statm=read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        self.assertEqual(got, (10 + 20 + 30) * 4096)

    def test_comm_with_spaces_and_parens_parses_ppid(self):
        # "(Web Content)" style comm must not break the ppid split.
        def read_stat(pid):
            return f"{pid} (Web Content (x)) S {_PPID[pid]} 1"
        got = tree_rss_bytes(
            100, read_stat=read_stat, read_statm=_read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        self.assertEqual(got, (10 + 20 + 30 + 40) * 4096)

    def test_process_tree_rss_defaults_to_real_getpid(self):
        # Smoke: real /proc read of THIS process returns a positive number.
        self.assertGreater(process_tree_rss(), 0)


if __name__ == "__main__":
    unittest.main()
```

2. Run — expected **FAIL** (`ModuleNotFoundError: No module named 'app.ramsampler'`):

```
cd services/stealth-scraper && python -m pytest tests/test_ramsampler.py -q
```

3. Implement `services/stealth-scraper/app/ramsampler.py`:

```python
"""Dependency-free combined-RSS sampler for the Camoufox/Firefox process tree.

camoufox spawns Firefox (and its content/GPU processes) as CHILDREN of this
python process, so the RAM budget is the RSS of os.getpid() + every descendant.
We read /proc directly (no psutil): /proc/<pid>/statm field 2 is `resident` in
pages; /proc/<pid>/stat field 4 (after the comm field) is the ppid used to walk
the tree. All readers are injectable so the unit tests never touch real /proc.
"""

from __future__ import annotations

import os


def _default_read_stat(pid: int) -> str:
    with open(f"/proc/{pid}/stat", "r") as f:
        return f.read()


def _default_read_statm(pid: int) -> str:
    with open(f"/proc/{pid}/statm", "r") as f:
        return f.read()


def _default_all_pids() -> list[int]:
    out: list[int] = []
    for name in os.listdir("/proc"):
        if name.isdigit():
            out.append(int(name))
    return out


def _ppid_of(stat_line: str) -> int | None:
    """Parse ppid from a /proc/<pid>/stat line. The comm field (field 2) is
    wrapped in parens and may itself contain spaces/parens, so split on the LAST
    ')': everything after it is space-delimited, and ppid is the 2nd such token
    (state, ppid, ...)."""
    rparen = stat_line.rfind(")")
    if rparen == -1:
        return None
    rest = stat_line[rparen + 1:].split()
    if len(rest) < 2:
        return None
    try:
        return int(rest[1])
    except ValueError:
        return None


def _rss_pages(statm_line: str) -> int:
    parts = statm_line.split()
    if len(parts) < 2:
        return 0
    try:
        return int(parts[1])
    except ValueError:
        return 0


def tree_rss_bytes(
    root_pid: int,
    *,
    read_stat=_default_read_stat,
    read_statm=_default_read_statm,
    all_pids=_default_all_pids,
    page_size: int | None = None,
) -> int:
    """Combined RSS (bytes) of root_pid and all of its descendants.

    Builds the pid→ppid map once from a single /proc scan, then sums statm RSS
    for every pid reachable from root_pid. Dead pids (raced away mid-scan) are
    skipped, never fatal — the sampler must not crash the sidecar."""
    if page_size is None:
        page_size = os.sysconf("SC_PAGE_SIZE")

    children: dict[int, list[int]] = {}
    for pid in all_pids():
        try:
            ppid = _ppid_of(read_stat(pid))
        except (OSError, ValueError):
            continue
        if ppid is None:
            continue
        children.setdefault(ppid, []).append(pid)

    total = 0
    stack = [root_pid]
    seen: set[int] = set()
    while stack:
        pid = stack.pop()
        if pid in seen:
            continue
        seen.add(pid)
        try:
            total += _rss_pages(read_statm(pid)) * page_size
        except (OSError, ValueError):
            pass  # process exited between scan and read — skip, don't crash
        stack.extend(children.get(pid, ()))
    return total


def process_tree_rss(
    *,
    root_pid: int | None = None,
    read_stat=_default_read_stat,
    read_statm=_default_read_statm,
    all_pids=_default_all_pids,
    page_size: int | None = None,
) -> int:
    """Combined RSS of THIS process tree (os.getpid() by default)."""
    if root_pid is None:
        root_pid = os.getpid()
    return tree_rss_bytes(
        root_pid,
        read_stat=read_stat,
        read_statm=read_statm,
        all_pids=all_pids,
        page_size=page_size,
    )
```

4. Run — expected **PASS**:

```
cd services/stealth-scraper && python -m pytest tests/test_ramsampler.py -q
```

5. Commit:

```
git add services/stealth-scraper/app/ramsampler.py services/stealth-scraper/tests/test_ramsampler.py
git commit -m "feat(stealth): dependency-free /proc RSS sampler over the Camoufox process tree"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.3 — Capacity metrics + new typed exceptions

**Files:**
- `services/stealth-scraper/app/metrics.py:52-61` (append gauges/counters)
- `services/stealth-scraper/app/engine.py:711-729` (append `CapacityExceeded` + `UserQuotaExceeded` near `PoolExhausted`)
- `services/stealth-scraper/tests/test_engine_capacity.py` (NEW — exception-shape only in this task)

**Interfaces:**
- Produces: `metrics.RAM_BYTES`, `metrics.ADMISSION_TOTAL{action}`, `metrics.USER_QUOTA_REJECTED_TOTAL`; engine exceptions `CapacityExceeded(kind="capacity")`, `UserQuotaExceeded(kind="user_quota")`.
- Consumes: existing `prometheus_client` import in `metrics.py`.

`CapacityExceeded` and `UserQuotaExceeded` both subclass `RecipeError` (like `PoolExhausted`) so the existing failover classifier keeps treating them as retryable; each carries a `kind` attribute the `main.py` handler maps to the 503 body.

1. Write the failing test `services/stealth-scraper/tests/test_engine_capacity.py` (first assertion block only — the admission/quota tests are added in P2.4/P2.5):

```python
"""RAM admission + per-user quota (Phase 2)."""
import unittest

from app.engine import CapacityExceeded, UserQuotaExceeded, PoolExhausted
from app.recipes.base import RecipeError


class TestCapacityExceptions(unittest.TestCase):
    def test_capacity_is_recipe_error_with_kind(self):
        exc = CapacityExceeded("hard RAM limit")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "capacity")

    def test_user_quota_is_recipe_error_with_kind(self):
        exc = UserQuotaExceeded("u over quota")
        self.assertIsInstance(exc, RecipeError)
        self.assertEqual(exc.kind, "user_quota")

    def test_pool_exhausted_still_distinct(self):
        self.assertNotIsInstance(PoolExhausted("x"), CapacityExceeded)


if __name__ == "__main__":
    unittest.main()
```

2. Run — expected **FAIL** (`ImportError: cannot import name 'CapacityExceeded'`):

```
cd services/stealth-scraper && python -m pytest tests/test_engine_capacity.py -q
```

3. Implement. In `app/metrics.py`, append after `BROWSER_RELAUNCH_TOTAL` (line 61):

```python
RAM_BYTES = Gauge(
    "stealth_ram_bytes",
    "Combined RSS of the Camoufox/Firefox process tree, last sample.",
)

ADMISSION_TOTAL = Counter(
    "stealth_admission_total",
    "Admission-controller actions by type.",
    ["action"],  # soft_evict|hard_refuse|hard_evict
)

USER_QUOTA_REJECTED_TOTAL = Counter(
    "stealth_user_quota_rejected_total",
    "Resolves/fetches rejected because the user_key held >= quota sessions.",
)
```

In `app/engine.py`, append after the `PoolExhausted` class (line 720):

```python
class CapacityExceeded(RecipeError):
    """A new browser launch was refused because the combined Camoufox RSS is at
    or above the hard RAM budget. 503 (retryable) so the Go orchestrator fails
    over; the LRU not-in-use session is evicted to reclaim headroom."""

    kind = "capacity"


class UserQuotaExceeded(RecipeError):
    """The requesting user_key already holds >= STEALTH_USER_QUOTA sessions.
    503 (retryable); bounds a single user's pool footprint so one viewer cannot
    starve the shared pool."""

    kind = "user_quota"
```

4. Run — expected **PASS**:

```
cd services/stealth-scraper && python -m pytest tests/test_engine_capacity.py -q
```

5. Commit:

```
git add services/stealth-scraper/app/metrics.py services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_capacity.py
git commit -m "feat(stealth): capacity/user-quota metrics + typed kind exceptions"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.4 — Admission controller: soft stop-warming/evict-idle, hard refuse-launch/evict-LRU

**Files:**
- `services/stealth-scraper/app/engine.py:130-148` (engine init: RAM sampler hook + `_ram_bytes`)
- `services/stealth-scraper/app/engine.py:153-189` (`start`/`health`: sampler task + `ram_bytes` in health)
- `services/stealth-scraper/app/engine.py:192-231` (`_ensure_browser`: admission gate)
- `services/stealth-scraper/tests/test_engine_capacity.py` (extend)

**Interfaces:**
- Consumes: `ramsampler.process_tree_rss`, `Config.ram_soft_bytes/ram_hard_bytes/ram_sample_seconds`, `metrics.RAM_BYTES/ADMISSION_TOTAL`, Phase-1 self-heal pool.
- Produces: `engine._admit_launch()` (raises `CapacityExceeded` at hard), `engine._evict_one_lru()`, `engine._sample_ram()`, `engine._warming_allowed()`; `health()["ram_bytes"]`.

The gate runs at the single launch chokepoint `_ensure_browser` (every cold/rotate browser open passes through it). Sampling is decoupled into a background task (`ram_sample_seconds` cadence) that caches `self._ram_bytes`; the gate reads the cache (cheap, non-blocking on the request path) but also force-samples once on a hard-looking read so a burst between ticks can't blow past the hard limit. **Fail-safe:** if `process_tree_rss` raises, treat RAM as 0 (admit) and fall back to the `pool_size` ceiling that `ProfileManager` already enforces.

1. Extend `tests/test_engine_capacity.py` — add an admission test class (uses a fake sampler + a stub `_ensure_browser` chokepoint via the real gate):

```python
import asyncio
import time

from app.config import Config
from app.engine import CamoufoxEngine, Session


def run(coro):
    return asyncio.run(coro)


def _engine(soft, hard, *, ram):
    cfg = Config(pool_size=4, warming_enabled=False,
                 ram_soft_bytes=soft, ram_hard_bytes=hard)
    eng = CamoufoxEngine(cfg)
    eng._sample_ram = lambda: ram          # pin the sampled RSS
    return eng


def _mk_session(eng, sid, *, user_key=None, expires_in=600, in_use=0):
    prof = eng.profiles.lease()
    s = Session(
        id=sid, profile=prof, proxy_id="direct", referer="r", user_agent="UA",
        cdn_host="h", master_url="m", expires_at=time.time() + expires_in,
        page=None, player_url="p",
    )
    s.user_key = user_key
    s.in_use = in_use
    eng._sessions[sid] = s
    return s


class TestAdmission(unittest.TestCase):
    def test_below_soft_admits_and_allows_warming(self):
        eng = _engine(1000, 2000, ram=500)
        eng._admit_launch()                # must not raise
        self.assertTrue(eng._warming_allowed())

    def test_soft_stops_warming_and_evicts_idle(self):
        eng = _engine(1000, 2000, ram=1500)   # soft <= ram < hard
        # one idle (not-in-use, expired) + one active (in_use) session
        _mk_session(eng, "idle", expires_in=-1, in_use=0)
        _mk_session(eng, "busy", expires_in=600, in_use=1)
        self.assertFalse(eng._warming_allowed())
        eng._admit_launch()                # soft must NOT refuse a launch
        # idle session reclaimed; busy one survives
        self.assertNotIn("idle", eng._sessions)
        self.assertIn("busy", eng._sessions)

    def test_hard_refuses_launch_and_evicts_lru(self):
        eng = _engine(1000, 2000, ram=2500)   # ram >= hard
        old = _mk_session(eng, "old", in_use=0)
        old.expires_at = time.time() + 600
        new = _mk_session(eng, "new", in_use=0)
        new.expires_at = time.time() + 900
        with self.assertRaises(CapacityExceeded):
            eng._admit_launch()
        # LRU (smallest expires_at = "old") evicted to reclaim headroom
        self.assertNotIn("old", eng._sessions)

    def test_hard_never_evicts_in_use_session(self):
        eng = _engine(1000, 2000, ram=2500)
        busy = _mk_session(eng, "busy", in_use=1)
        with self.assertRaises(CapacityExceeded):
            eng._admit_launch()
        self.assertIn("busy", eng._sessions)   # in-flight fetch protected

    def test_ram_read_failure_is_fail_safe_admit(self):
        eng = _engine(1000, 2000, ram=0)
        def boom():
            raise OSError("proc gone")
        eng._sample_ram = boom
        eng._admit_launch()                # fail-safe: admit, don't crash
```

2. Run — expected **FAIL** (`AttributeError: '...' object has no attribute '_admit_launch'`):

```
cd services/stealth-scraper && python -m pytest tests/test_engine_capacity.py -q
```

3. Implement. In `engine.py` add the import near the top-of-module imports (after `from .profiles import ...`, line 65):

```python
from .ramsampler import process_tree_rss
```

In `CamoufoxEngine.__init__` (after `self._reaper_task = None`, line 147) add:

```python
        # RAM-budgeted admission (Phase 2). _ram_bytes is refreshed by a
        # background sampler; the admission gate reads the cache on the request
        # path (cheap) and force-resamples on a near-hard read.
        self._ram_bytes: int = 0
        self._ram_task: Any = None
```

In `start()` (after the reaper task line 157) add:

```python
        self._ram_task = asyncio.create_task(self._ram_sampler_loop())
```

In `stop()` (after cancelling the reaper, line 162) add:

```python
        if self._ram_task is not None:
            self._ram_task.cancel()
            self._ram_task = None
```

In `health()` (line 178, after `free = ...`) add `ram_bytes` to the returned dict — insert into the dict literal:

```python
            "ram_bytes": self._ram_bytes,
```

Add the sampler + admission methods (place them right after `_acquire_profile`, ~line 383):

```python
    def _sample_ram(self) -> int:
        """Combined Camoufox/Firefox RSS (bytes). Fail-safe: on any /proc read
        error return 0 so the gate admits (the pool_size ceiling still bounds)."""
        try:
            return process_tree_rss()
        except Exception:  # noqa: BLE001
            return 0

    async def _ram_sampler_loop(self) -> None:
        while True:
            try:
                await asyncio.sleep(self.cfg.ram_sample_seconds)
                self._ram_bytes = self._sample_ram()
                metrics.RAM_BYTES.set(self._ram_bytes)
            except asyncio.CancelledError:
                break
            except Exception:  # noqa: BLE001
                if self._log:
                    self._log.exception("ram sampler tick failed")

    def _warming_allowed(self) -> bool:
        """False once combined RSS reaches the soft budget — new profiles are
        not warmed under back-pressure (existing leases untouched)."""
        return self._ram_bytes < self.cfg.ram_soft_bytes

    def _evict_one_lru(self) -> bool:
        """Evict the least-recently-used NOT-in-use session (smallest
        expires_at) to reclaim a browser slot. Returns True if one was freed.
        Never touches an in-use session (a concurrent /hls fetch is awaiting it)."""
        candidates = [
            (s.expires_at, sid, s)
            for sid, s in self._sessions.items()
            if s.in_use <= 0
        ]
        if not candidates:
            return False
        candidates.sort(key=lambda t: t[0])
        _, sid, session = candidates[0]
        self._sessions.pop(sid, None)
        self._spawn(_safe_close_page(session.page))
        self.profiles.release(session.profile, ok=True)
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        return True

    def _admit_launch(self) -> None:
        """Admission gate at the single browser-launch chokepoint.

          - ram < soft            → admit.
          - soft <= ram < hard    → admit, but proactively evict idle/expired
                                     not-in-use sessions (back-pressure).
          - ram >= hard           → refuse (CapacityExceeded); evict the LRU
                                     not-in-use session to reclaim, then raise.
        Fail-safe: _sample_ram() returns 0 on a /proc error → always admits."""
        ram = self._ram_bytes
        if ram >= self.cfg.ram_hard_bytes:
            # Force a fresh read in case the burst outran the sampler cadence.
            ram = self._sample_ram()
            self._ram_bytes = ram
        if ram >= self.cfg.ram_hard_bytes:
            metrics.ADMISSION_TOTAL.labels(action="hard_evict" if self._evict_one_lru() else "hard_refuse").inc()
            raise CapacityExceeded(
                f"combined RSS {ram} >= hard budget {self.cfg.ram_hard_bytes}"
            )
        if ram >= self.cfg.ram_soft_bytes:
            self._evict_expired()  # drop idle/expired not-in-use sessions
            metrics.ADMISSION_TOTAL.labels(action="soft_evict").inc()
```

Wire the gate into `_ensure_browser` — at the very top of the method (line 193, before the early-return), insert:

```python
        # RAM admission: refuse a launch over the hard budget (reclaiming LRU
        # first). A warm reuse (profile already launched on the same proxy)
        # skips the gate — it consumes no NEW memory.
        if not (profile.launched and profile.proxy_id == proxy_id):
            self._admit_launch()
```

4. Run — expected **PASS**:

```
cd services/stealth-scraper && python -m pytest tests/test_engine_capacity.py -q
```

5. Commit:

```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_capacity.py
git commit -m "feat(stealth): RAM-budgeted admission controller (soft evict-idle / hard refuse+evict-LRU)"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.5 — Per-user quota + `user_key` on the engine + `/resolve`,`/fetch` 503 bodies

**Files:**
- `services/stealth-scraper/app/engine.py:79-101` (`Session.user_key` field)
- `services/stealth-scraper/app/engine.py:269-413` (`resolve(...)` + `_open_session` carry `user_key`; quota check)
- `services/stealth-scraper/app/engine.py:514-594` (`browser_fetch` + `_warm_fetch_session` carry `user_key`; quota check)
- `services/stealth-scraper/app/main.py:33-60` (`user_key` on request models)
- `services/stealth-scraper/app/main.py:92-147` (map `CapacityExceeded`/`UserQuotaExceeded` → 503 + `kind`)
- `services/stealth-scraper/tests/test_engine_capacity.py` (extend)

**Interfaces:**
- Consumes: `Config.user_quota`, `UserQuotaExceeded`, `CapacityExceeded`, P2.4 admission.
- Produces: `Session.user_key`; `engine.resolve(provider, params, user_key=None)`, `engine.browser_fetch(provider, url, user_key=None)`; `engine._held_for_user(key)`; 503 bodies `{success:false,error,kind:"capacity"|"user_quota"}` matching the existing `main.py` shape (`{"success": False, "error": str(exc), "kind": ...}`).

`user_key` counts only HELD sessions (those that pin a profile) attributed to that key. `_held_for_user` scans `self._sessions`. A `None`/empty `user_key` is unbounded (anonymous callers are bounded upstream via the catalog's salted-IP fallback — see P2.7/P2.8 — so an empty key reaching the sidecar means "accounting opted out" and stays unlimited, never globally shared by construction since the catalog always supplies one).

1. Extend `tests/test_engine_capacity.py` with a quota class:

```python
class TestUserQuota(unittest.TestCase):
    def test_third_session_for_same_user_rejected(self):
        eng = _engine(10**12, 10**12, ram=0)   # RAM never the limiter here
        eng.cfg.user_quota = 2
        _mk_session(eng, "s1", user_key="alice")
        _mk_session(eng, "s2", user_key="alice")
        with self.assertRaises(UserQuotaExceeded):
            eng._enforce_user_quota("alice")

    def test_other_user_unaffected(self):
        eng = _engine(10**12, 10**12, ram=0)
        eng.cfg.user_quota = 2
        _mk_session(eng, "s1", user_key="alice")
        _mk_session(eng, "s2", user_key="alice")
        eng._enforce_user_quota("bob")          # bob has 0 → ok

    def test_empty_user_key_is_unbounded(self):
        eng = _engine(10**12, 10**12, ram=0)
        eng.cfg.user_quota = 1
        _mk_session(eng, "s1", user_key=None)
        _mk_session(eng, "s2", user_key=None)
        eng._enforce_user_quota(None)           # no key → never rejected
        eng._enforce_user_quota("")


class TestErrorBodies(unittest.TestCase):
    def _set_engine(self, engine):
        import app.main as m
        m.app.state.engine = engine
        return m

    def test_resolve_capacity_is_503_kind_capacity(self):
        import json
        from app.main import ResolveRequest
        from app.engine import CapacityExceeded

        eng = _engine(1, 1, ram=0)

        async def _boom(*a, **k):
            raise CapacityExceeded("hard")
        eng.resolve = _boom
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime", embed_url="https://x/y")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "capacity")

    def test_resolve_user_quota_is_503_kind_user_quota(self):
        import json
        from app.main import ResolveRequest
        from app.engine import UserQuotaExceeded

        eng = _engine(1, 1, ram=0)

        async def _boom(*a, **k):
            raise UserQuotaExceeded("over")
        eng.resolve = _boom
        m = self._set_engine(eng)
        resp = run(m.resolve(ResolveRequest(provider="gogoanime", embed_url="https://x/y")))
        self.assertEqual(resp.status_code, 503)
        self.assertEqual(json.loads(resp.body)["kind"], "user_quota")

    def test_request_models_accept_user_key(self):
        from app.main import ResolveRequest, FetchRequest
        r = ResolveRequest(provider="gogoanime", embed_url="https://x/y", user_key="alice")
        f = FetchRequest(provider="nineanime", url="https://9anime.me.uk/x", user_key="bob")
        self.assertEqual(r.user_key, "alice")
        self.assertEqual(f.user_key, "bob")
```

2. Run — expected **FAIL** (`AttributeError: ... '_enforce_user_quota'` / `ResolveRequest` rejects `user_key`):

```
cd services/stealth-scraper && python -m pytest tests/test_engine_capacity.py -q
```

3. Implement.

In `engine.py`, add to the `Session` dataclass (after `in_use: int = 0`, line 101):

```python
    # Quota accounting key (opaque user id or salted IP hash; never logged in
    # clear). None ⇒ unbounded (the caller opted out of per-user accounting).
    user_key: str | None = None
```

Add the quota helper next to `_admit_launch` (after `_evict_one_lru`):

```python
    def _held_for_user(self, user_key: str) -> int:
        return sum(1 for s in self._sessions.values() if s.user_key == user_key)

    def _enforce_user_quota(self, user_key: str | None) -> None:
        """Reject a NEW held session when user_key already holds >= quota. A
        falsy user_key is unbounded (the catalog always supplies one — a salted
        IP hash for anon — so a missing key here is an explicit opt-out)."""
        if not user_key:
            return
        if self._held_for_user(user_key) >= self.cfg.user_quota:
            metrics.USER_QUOTA_REJECTED_TOTAL.inc()
            raise UserQuotaExceeded(
                f"user {user_key[:8]}… holds >= quota ({self.cfg.user_quota})"
            )
```

Thread `user_key` into `resolve`. Change the signature (line 269) and add the quota check before the acquire loop, then stamp the opened session:

```python
    async def resolve(self, provider: str, params: dict, user_key: str | None = None) -> dict:
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")

        self._evict_expired()
        self._enforce_user_quota(user_key)
        started = time.monotonic()
```

In `_open_session` (line 385), add a `user_key` parameter and stamp it on the `Session`:

```python
    async def _open_session(
        self, partial: dict, context: Any, proxy_id: str, profile: Profile, page: Any,
        user_key: str | None = None,
    ) -> Session:
```
and inside the `Session(...)` constructor add `user_key=user_key,` (alongside `player_url=player_url,`). Update the call site in `resolve` (line 314) to `await self._open_session(partial, context, proxy.id, profile, page, user_key)`.

Thread `user_key` into `browser_fetch` (line 514). Change the signature and enforce before warming:

```python
    async def browser_fetch(self, provider: str, url: str, user_key: str | None = None) -> dict:
```
After `self._evict_expired()` (line 527) and before `session = await self._warm_fetch_session(...)`, the warm session is keyed by `(provider, origin)` and shared, so attribute it to the FIRST user_key that creates it. Pass `user_key` into `_warm_fetch_session` and only enforce when a NEW session is created. Change `_warm_fetch_session(self, provider, origin)` → `_warm_fetch_session(self, provider, origin, user_key=None)`; before acquiring a profile (line 557) add `self._enforce_user_quota(user_key)`, and in the `Session(...)` constructor (line 586) add `user_key=user_key,`. Update the call site (line 529) to `await self._warm_fetch_session(provider, origin, user_key)`.

In `main.py`, add `user_key` to both request models. In `ResolveRequest` (after `proxy_type`, line 49):

```python
    # Opaque quota key (authed user id, or a salted client-IP hash for anon),
    # set by the catalog→scraper hop. Used only for per-user concurrency
    # accounting; never persisted or logged in clear.
    user_key: str | None = Field(default=None, max_length=128)
```
(`params()` excludes `provider` only, so `user_key` flows in `params` — but pass it explicitly instead; see below.)

Update `ResolveRequest.params()` to also exclude `user_key` so it isn't double-fed into the recipe params:

```python
    def params(self) -> dict[str, Any]:
        return self.model_dump(exclude={"provider", "user_key"}, exclude_none=True)
```

In `FetchRequest` (after `method`, line 59):

```python
    user_key: str | None = Field(default=None, max_length=128)
```

In the `resolve` route (line 96), pass `user_key`:

```python
        session = await engine.resolve(req.provider, req.params(), user_key=req.user_key)
```

In the `fetch` route (line 130), pass `user_key`:

```python
        out = await engine.browser_fetch(req.provider, req.url, user_key=req.user_key)
```

Add the new `except` arms to BOTH routes, placed BEFORE the existing `except PoolExhausted` (since `CapacityExceeded`/`UserQuotaExceeded` subclass `RecipeError` but are more specific — order matters; place them before `RecipeError`). In `resolve` (before `except PoolExhausted`, line 102):

```python
    except CapacityExceeded as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "capacity"}, status_code=503
        )
    except UserQuotaExceeded as exc:
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "user_quota"}, status_code=503
        )
```
and the identical two arms in `fetch` before its `except PoolExhausted` (line 139). Update the engine import (line 26):

```python
from .engine import (
    CamoufoxEngine,
    CapacityExceeded,
    FetchTimeout,
    PoolExhausted,
    SessionGone,
    UserQuotaExceeded,
)
```

4. Run — expected **PASS**:

```
cd services/stealth-scraper && python -m pytest tests/ -q
```

5. Commit:

```
git add services/stealth-scraper/app/engine.py services/stealth-scraper/app/main.py services/stealth-scraper/tests/test_engine_capacity.py
git commit -m "feat(stealth): per-user session quota + capacity/user_quota 503 kinds; thread user_key"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.6 — Go: `userkey` ctx package + scraper router middleware + sidecar forwards `user_key`

**Files:**
- `services/scraper/internal/userkey/userkey.go` (NEW)
- `services/scraper/internal/userkey/userkey_test.go` (NEW)
- `services/scraper/internal/transport/router.go:80-141` (mount middleware on the `/scraper` group)
- `services/scraper/internal/sidecar/client.go:52-59,105-114,169-179` (`resolveRequest.UserKey`; read ctx; set body)
- `services/scraper/internal/sidecar/client_test.go` (assert forwarded)

**Interfaces:**
- Produces: `userkey.WithUserKey(ctx, key) context.Context`, `userkey.FromContext(ctx) string`, `userkey.Middleware(next) http.Handler` (reads `X-AE-User`), `userkey.HeaderName = "X-AE-User"`.
- Consumes: incoming `X-AE-User` request header (set by catalog, P2.8); `sidecar.Client.ResolveEmbed` reads `userkey.FromContext`.

The sidecar's `ResolveEmbed(ctx, ...)` is called from deep inside the `gogoBrowserResolve`/`nineBrowserResolve` closures (`cmd/scraper-api/main.go:309,448`) which only receive `ctx`, so `user_key` rides the context, seeded by a scraper-router middleware from the `X-AE-User` header.

1. Write the failing test `services/scraper/internal/userkey/userkey_test.go`:

```go
package userkey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	ctx := WithUserKey(context.Background(), "alice")
	if got := FromContext(ctx); got != "alice" {
		t.Errorf("FromContext = %q; want alice", got)
	}
}

func TestFromContextEmptyWhenUnset(t *testing.T) {
	if got := FromContext(context.Background()); got != "" {
		t.Errorf("FromContext on bare ctx = %q; want empty", got)
	}
}

func TestMiddlewareSeedsHeaderIntoContext(t *testing.T) {
	var seen string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/scraper/stream", nil)
	req.Header.Set(HeaderName, "u-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "u-123" {
		t.Errorf("middleware seeded %q; want u-123", seen)
	}
}

func TestMiddlewareNoHeaderLeavesEmpty(t *testing.T) {
	var seen = "sentinel"
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if seen != "" {
		t.Errorf("no-header middleware seeded %q; want empty", seen)
	}
}
```

2. Run — expected **FAIL** (`no required module provides package .../userkey`):

```
cd services/scraper && go test ./internal/userkey/ 2>&1 | head
```

3. Implement `services/scraper/internal/userkey/userkey.go`:

```go
// Package userkey carries an opaque per-user quota key (an authenticated user
// id, or a salted client-IP hash for anonymous callers) from the inbound
// catalog→scraper request (the X-AE-User header) onto the request context, so
// the deep sidecar.Client.ResolveEmbed call can stamp it onto the stealth-
// scraper request body for per-user session-quota accounting. The key is opaque
// and is never logged in clear or persisted.
package userkey

import (
	"context"
	"net/http"
)

// HeaderName is the request header the catalog sets and the scraper reads.
const HeaderName = "X-AE-User"

type ctxKey struct{}

// WithUserKey returns ctx carrying key (no-op stored value when empty).
func WithUserKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxKey{}, key)
}

// FromContext returns the user key, or "" when unset.
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// Middleware seeds the X-AE-User header value into the request context. A
// missing/empty header leaves the context unchanged (FromContext → "").
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if k := r.Header.Get(HeaderName); k != "" {
			r = r.WithContext(WithUserKey(r.Context(), k))
		}
		next.ServeHTTP(w, r)
	})
}
```

4. Run — expected **PASS**:

```
cd services/scraper && go test ./internal/userkey/
```

5. Mount the middleware on the `/scraper` route group. In `services/scraper/internal/transport/router.go`, add the import (with the other internal imports near the top):

```go
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"
```
and inside `r.Route("/scraper", func(r chi.Router) {` (line 126), as the FIRST line of the group:

```go
		r.Use(userkey.Middleware)
```

6. Forward `user_key` from the sidecar client. In `services/scraper/internal/sidecar/client.go`:

Add the import:

```go
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"
```
Add the field to `resolveRequest` (line 58, after `BaseURL`):

```go
	UserKey  string `json:"user_key,omitempty"`
```
In `ResolveEmbed` (line 105), read the key from ctx and set it on the request:

```go
func (c *Client) ResolveEmbed(
	ctx context.Context, provider, embedURL string, category domain.Category, baseURL string,
) (*domain.Stream, error) {
	return c.resolve(ctx, resolveRequest{
		Provider: provider,
		EmbedURL: embedURL,
		Category: string(category),
		BaseURL:  baseURL,
		UserKey:  userkey.FromContext(ctx),
	})
}
```

7. Extend `services/scraper/internal/sidecar/client_test.go` — add a test asserting the body carries `user_key` from ctx:

```go
func TestResolveEmbed_ForwardsUserKeyFromContext(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	ctx := userkey.WithUserKey(context.Background(), "alice")
	if _, err := c.ResolveEmbed(ctx, "gogoanime", "https://x/y", domain.CategorySub, "https://b"); err != nil {
		t.Fatalf("ResolveEmbed: %v", err)
	}
	if gotReq["user_key"] != "alice" {
		t.Errorf("user_key = %v; want alice", gotReq["user_key"])
	}
}

func TestResolveEmbed_OmitsUserKeyWhenAbsent(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	if _, err := c.ResolveEmbed(context.Background(), "gogoanime", "https://x/y", domain.CategorySub, "https://b"); err != nil {
		t.Fatalf("ResolveEmbed: %v", err)
	}
	if _, present := gotReq["user_key"]; present {
		t.Errorf("user_key present on anon request: %v", gotReq)
	}
}
```
Add `"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"` to the test imports.

8. Run — expected **PASS**:

```
cd services/scraper && go test ./internal/userkey/ ./internal/sidecar/ ./internal/transport/
```

9. Commit:

```
git add services/scraper/internal/userkey/ services/scraper/internal/transport/router.go services/scraper/internal/sidecar/client.go services/scraper/internal/sidecar/client_test.go
git commit -m "feat(scraper): thread user_key from X-AE-User header to sidecar resolve body"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.7 — Catalog scraper HTTP client: send the `X-AE-User` header on the stream call

**Files:**
- `services/catalog/internal/parser/scraper/client.go:117-138` (`GetStream` gains `userKey`; set header)
- `services/catalog/internal/parser/scraper/client.go:185-223` (`doGET` gains an optional header)
- `services/catalog/internal/parser/scraper/client_test.go` (assert header sent)

**Interfaces:**
- Produces: `(*scraper.Client).GetStream(ctx, malID, title, altTitles, episodeID, serverID, category, prefer string, exclusive bool, userKey string)` — sets `X-AE-User: <userKey>` when non-empty.
- Consumes: nothing new (stdlib http).

Only `GetStream` needs the header (the sidecar resolve happens on the stream call). The episodes/servers calls are unchanged. `doGET` grows a small variadic-header escape hatch so the change is local.

1. Extend `services/catalog/internal/parser/scraper/client_test.go` — read an existing test first for the harness, then add:

```go
func TestGetStream_SendsUserKeyHeader(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-AE-User")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second)
	_, _, err := c.GetStream(context.Background(), 1, "t", nil, "ep", "srv", "sub", "gogoanime", false, "alice")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if gotUser != "alice" {
		t.Errorf("X-AE-User = %q; want alice", gotUser)
	}
}

func TestGetStream_OmitsUserKeyHeaderWhenEmpty(t *testing.T) {
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Ae-User"] // canonicalized
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second)
	if _, _, err := c.GetStream(context.Background(), 1, "t", nil, "ep", "srv", "sub", "gogoanime", false, ""); err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if present {
		t.Error("X-AE-User header present on anon stream call")
	}
}
```
Ensure `net/http`, `net/http/httptest`, `context`, `time` are imported in the test file.

2. Run — expected **FAIL** (too many args to `GetStream` / signature mismatch):

```
cd services/catalog && go test ./internal/parser/scraper/ 2>&1 | head
```

3. Implement. In `services/catalog/internal/parser/scraper/client.go`, change `GetStream` (line 119) to take `userKey` and route it through `doGET` as a header:

```go
func (c *Client) GetStream(ctx context.Context, malID int, title string, altTitles []string, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	q := url.Values{}
	q.Set("mal_id", strconv.Itoa(malID))
	if title != "" {
		q.Set("title", title)
	}
	setAltTitles(q, altTitles)
	q.Set("episode", episodeID)
	q.Set("server", serverID)
	if category != "" {
		q.Set("category", category)
	}
	if prefer != "" {
		q.Set("prefer", prefer)
	}
	if exclusive {
		q.Set("exclusive", "true")
	}
	var hdr http.Header
	if userKey != "" {
		hdr = http.Header{"X-AE-User": []string{userKey}}
	}
	return c.doGET(ctx, "/scraper/stream", q, hdr)
}
```

Update `doGET` (line 185) to accept an optional header set, and update the three OTHER callers (`GetEpisodes`, `GetServers`, `GetHealth`, and the two `/anime18/*` calls) to pass `nil`:

```go
func (c *Client) doGET(ctx context.Context, path string, q url.Values, hdr ...http.Header) (int, []byte, error) {
	full := c.baseURL + path
	if q != nil && len(q) > 0 {
		full += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build scraper request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if len(hdr) > 0 && hdr[0] != nil {
		for k, vs := range hdr[0] {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	// ... unchanged body from here (resp, err := c.httpClient.Do(req) ...) ...
```
(The remaining lines of `doGET` are unchanged; the `hdr ...http.Header` variadic keeps the four `nil`-passing callers source-compatible without editing them — `doGET(ctx, path, q)` still compiles.)

4. Run — expected **PASS**:

```
cd services/catalog && go test ./internal/parser/scraper/
```

5. Commit:

```
git add services/catalog/internal/parser/scraper/client.go services/catalog/internal/parser/scraper/client_test.go
git commit -m "feat(catalog): send X-AE-User on the scraper stream call"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.8 — Catalog service+handler: derive `user_key` (authed id → salted IP fallback) and thread it

**Files:**
- `services/catalog/internal/service/scraper.go:40-45` (`scraperForwarder.GetStream` gains `userKey`)
- `services/catalog/internal/service/scraper.go:166-198` (`GetScraperStream` threads `userKey`)
- `services/catalog/internal/handler/scraper.go:26-31,96-130` (extract user/IP → build key → pass)
- `services/catalog/internal/transport/router.go:175` (wrap stream route with `OptionalAuthMiddleware`)
- `services/catalog/internal/handler/scraper_test.go` + `services/catalog/internal/service/scraper_test.go` (extend stub signatures + assert key)

**Interfaces:**
- Produces: `GetScraperStream(ctx, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string)` on both `CatalogService` and `scraperOps`; handler helper `scraperUserKey(r) string` (authed `authz.UserIDFromContext` → else `"ip:" + HashIP(clientIP, salt, now)`).
- Consumes: `libs/authz.UserIDFromContext`, `service.HashIP`-style salted hash, `OptionalAuthMiddleware`.

The scraper stream route is currently public (no auth middleware), so claims are only present if we wrap it; `OptionalAuthMiddleware` never 401s, so anonymous playback keeps working and falls through to the salted-IP key.

1. Extend the Go tests first.

In `services/catalog/internal/service/scraper_test.go`, the existing fake `scraperForwarder` stub must gain the new `userKey` param on `GetStream`; add an assertion that `GetScraperStream` forwards a passed key. (Read the sibling stub in that file for the exact fake-struct style — it uses hand-written fakes, no testify/mock.) Add:

```go
func TestGetScraperStream_ForwardsUserKey(t *testing.T) {
	var gotKey string
	fwd := &fakeScraperForwarder{
		getStream: func(ctx context.Context, malID int, title string, alts []string, ep, srv, cat, prefer string, excl bool, userKey string) (int, []byte, error) {
			gotKey = userKey
			return 200, []byte(`{"success":true}`), nil
		},
	}
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: validUUID, ShikimoriID: "123"}}
	ops := &scraperOps{animeRepo: repo, scraperClient: fwd}
	if _, _, err := ops.GetScraperStream(context.Background(), validUUID, "ep", "srv", "sub", "gogoanime", false, "alice"); err != nil {
		t.Fatalf("GetScraperStream: %v", err)
	}
	if gotKey != "alice" {
		t.Errorf("forwarded user_key = %q; want alice", gotKey)
	}
}
```
Update the existing `fakeScraperForwarder.GetStream` method signature in that file to match the new interface (add `userKey string`).

In `services/catalog/internal/handler/scraper_test.go`, the stub `scraperServiceAPI.GetScraperStream` gains `userKey`; add a test that a request with a JWT-derived user / with an IP produces a non-empty key. (Match the handler-test harness already in that file.)

2. Run — expected **FAIL** (interface/signature mismatch):

```
cd services/catalog && go test ./internal/service/ ./internal/handler/ 2>&1 | head
```

3. Implement.

`services/catalog/internal/service/scraper.go` — update the `scraperForwarder` interface (line 43):

```go
	GetStream(ctx context.Context, malID int, title string, altTitles []string, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error)
```
Update `scraperOps.GetScraperStream` (line 167):

```go
func (o *scraperOps) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	malID, title, altTitles, err := o.resolveAnime(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetStream(ctx, malID, title, altTitles, episodeID, serverID, category, prefer, exclusive, userKey)
}
```
Update the public `CatalogService.GetScraperStream` (line 196):

```go
func (s *CatalogService) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error) {
	return s.scraperOps().GetScraperStream(ctx, animeID, episodeID, serverID, category, prefer, exclusive, userKey)
}
```

`services/catalog/internal/handler/scraper.go` — update the interface (line 29):

```go
	GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error)
```
Add imports (`crypto/sha256`, `encoding/hex`, `net`, `os`, `strings`, `time`, and `github.com/ILITA-hub/animeenigma/libs/authz`). Add the key-derivation helper at the bottom of the file:

```go
// scraperUserKey derives the opaque per-user quota key forwarded to the
// stealth-scraper sidecar (via the scraper service's X-AE-User header):
//   - authenticated → "u:" + the JWT user id (OptionalAuthMiddleware populated
//     claims when a Bearer token was sent);
//   - anonymous → "ip:" + sha256(clientIP | salt | UTC-day) so anon traffic is
//     still bounded per source, never globally shared, and the raw IP is never
//     forwarded or logged. Salt = CATALOG_IP_SALT (empty salt still hashes).
// Returns "" only when there is neither an authed user nor a usable client IP.
func scraperUserKey(r *http.Request) string {
	if uid := authz.UserIDFromContext(r.Context()); uid != "" {
		return "u:" + uid
	}
	ip := clientIP(r)
	if ip == "" {
		return ""
	}
	day := time.Now().UTC().Format("2006-01-02")
	sum := sha256.Sum256([]byte(ip + "|" + os.Getenv("CATALOG_IP_SALT") + "|" + day))
	return "ip:" + hex.EncodeToString(sum[:])
}

// clientIP extracts the best-effort client IP. The catalog router runs
// chi middleware.RealIP, which rewrites r.RemoteAddr from X-Forwarded-For /
// X-Real-IP, so RemoteAddr is the trusted source here.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
```
Update `GetScraperStream` (line 121) to derive + pass the key:

```go
	userKey := scraperUserKey(r)
	status, body, err := h.scraperSvc.GetScraperStream(r.Context(), animeID, episodeID, serverID, category, prefer, exclusive, userKey)
```

`services/catalog/internal/transport/router.go` — wrap the stream route (line 175) with `OptionalAuthMiddleware`:

```go
			r.With(OptionalAuthMiddleware(cfg.JWT)).Get("/{animeId}/scraper/stream", catalogHandler.GetScraperStream)
```
(The other three scraper routes stay as plain `r.Get`.)

4. Run — expected **PASS**:

```
cd services/catalog && go test ./internal/service/ ./internal/handler/ ./internal/transport/ ./internal/parser/scraper/
```

5. Commit:

```
git add services/catalog/internal/service/scraper.go services/catalog/internal/handler/scraper.go services/catalog/internal/transport/router.go services/catalog/internal/service/scraper_test.go services/catalog/internal/handler/scraper_test.go
git commit -m "feat(catalog): derive scraper user_key (authed id / salted-IP) + optional-auth on stream route"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Task P2.9 — Compose `mem_limit` bump + env wiring; `.env.example` docs

**Files:**
- `docker/docker-compose.yml:188,191-200` (stealth-scraper `mem_limit` + env block)
- `docker/.env.example` (NEW stealth section)

**Interfaces:**
- Consumes: `Config.from_env` keys (P2.1) + `CATALOG_IP_SALT` (P2.8).
- Produces: nothing code-facing — deploy config only.

> **Planning gate (carry to deploy):** confirm host RAM headroom before raising `mem_limit` to `7g` (6 GB hard budget + Xvfb/python overhead). The hard budget is governed in-process; the compose limit is the kernel backstop.

1. In `docker/docker-compose.yml`, change the stealth-scraper `mem_limit` (line 188):

```yaml
    mem_limit: 7g               # 6 GiB hard RAM budget (in-process admission) + Xvfb/python overhead
```
and add the new env refs to the `environment:` block (after `STEALTH_POOL_SIZE`, line 192):

```yaml
      # RAM-budgeted capacity (Phase 2). STEALTH_POOL_SIZE survives only as a
      # high fail-safe ceiling used when the /proc RSS read fails; the live cap
      # is the combined Camoufox RSS soft/hard budget below.
      STEALTH_RAM_SOFT_BYTES: ${STEALTH_RAM_SOFT_BYTES:-4294967296}
      STEALTH_RAM_HARD_BYTES: ${STEALTH_RAM_HARD_BYTES:-6442450944}
      STEALTH_RAM_SAMPLE_SECONDS: ${STEALTH_RAM_SAMPLE_SECONDS:-5}
      STEALTH_USER_QUOTA: ${STEALTH_USER_QUOTA:-2}
```
Also add the IP-salt to the `catalog` service env block (so the anon fallback key rotates; find the catalog `environment:` block and add):

```yaml
      CATALOG_IP_SALT: ${CATALOG_IP_SALT:-}
```

2. In `docker/.env.example`, append a documented block:

```
# --- stealth-scraper RAM budget + per-user quota (Phase 2) ---
# The Camoufox pool is governed by combined process-tree RSS, not a fixed count.
# Soft = stop warming + evict idle; hard = refuse a new browser launch
# (503 kind=capacity) + evict the LRU idle session. STEALTH_POOL_SIZE is only a
# fail-safe ceiling used if the /proc RSS read fails.
STEALTH_RAM_SOFT_BYTES=4294967296   # 4 GiB
STEALTH_RAM_HARD_BYTES=6442450944   # 6 GiB
STEALTH_RAM_SAMPLE_SECONDS=5
# Max concurrent stealth sessions per user (authed id or salted client-IP).
STEALTH_USER_QUOTA=2
# Salt for the anonymous-fallback per-IP quota key (catalog → scraper user_key).
# Rotated daily with the UTC date; empty still hashes. Set to a random secret.
CATALOG_IP_SALT=
```

3. Verify the compose file still parses (no code test here — config only):

```
docker compose -f docker/docker-compose.yml config >/dev/null && echo OK
```
Expected: `OK`.

4. Commit:

```
git add docker/docker-compose.yml docker/.env.example
git commit -m "chore(stealth): mem_limit 3500m->7g + RAM budget/user-quota env + IP salt"
```
`Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`

---

### Phase 2 verification

```
cd services/stealth-scraper && python -m pytest tests/ -q
cd services/scraper  && go build ./... && go vet ./... && go test ./internal/userkey/ ./internal/sidecar/ ./internal/transport/
cd services/catalog  && go build ./... && go vet ./... && go test ./internal/service/ ./internal/handler/ ./internal/transport/ ./internal/parser/scraper/
docker compose -f docker/docker-compose.yml config >/dev/null && echo COMPOSE_OK
```
All green ⇒ Phase 2 done: admission controller bounds the pool by RAM (soft 4 GB / hard 6 GB), per-user quota caps concurrent sessions at 2, and `user_key` threads catalog→scraper→sidecar with a salted-IP fallback for anon. `kind:"capacity"` / `kind:"user_quota"` 503 bodies are in place for Phase 3's circuit breaker to classify.

---

## Phase 3: Go scraper — kind surfacing + circuit breaker + orchestrator runtime re-gate

**Phase goal:** Surface the sidecar's machine-readable `kind` as a typed `ProviderWedgedError` (still failover-retryable), wire the existing `InMemoryHealthCache` as a per-provider circuit breaker that trips a wedged provider out of failover in real time and half-opens to recover, and make the orchestrator re-gate its `degraded` failover map on every catalog poll so a probe-driven status change takes effect without a scraper restart.

> **VERIFIED SYMBOL NOTE (read before implementing):** the contract says `ProviderWedgedError` "wraps `errors.ErrProviderDown`". There is **no** `ErrProviderDown` in `libs/errors` — the real sentinel is **`domain.ErrProviderDown`** (`services/scraper/internal/domain/errors.go:23`), wrapped via `domain.WrapProviderDown`. All Phase-3 code uses `domain.ErrProviderDown`. The failover classifier (`orchestrator.go:181`) already matches `errors.Is(err, domain.ErrProviderDown)`.

> **EXISTING-KIND NOTE:** the sidecar already returns `kind` in its 503/502 bodies and `sidecar.Client` already decodes `out.Kind` (`client.go:85,98`). Today every kind collapses into a bare `domain.WrapProviderDown(...)`. Phase 3 only adds a typed wrapper for the **wedged** kinds; non-wedged kinds (`challenge`, `error`, `internal`) stay exactly as they are. Phase 1 renames the sidecar's pool-exhausted kind from `exhausted` to `pool_exhausted`; Phase 3 treats BOTH as wedged so the breaker keeps working whether Phase 1 is deployed ahead of it or not.

---

### Task P3.1 — Typed `ProviderWedgedError{Kind}` in the sidecar client

**Files:**
- `services/scraper/internal/sidecar/client.go` (add type + `wedgedKinds` set + `classifyDown` helper; call it in `resolve()` ~204-209 and `Fetch()` ~150-154)
- `services/scraper/internal/sidecar/client_test.go` (add table-driven tests)

**Interfaces:**
- Consumes: `domain.ErrProviderDown`, `domain.WrapProviderDown` (`internal/domain/errors.go`); existing `resolveResponse.Kind` / `fetchResponse.Kind` decode.
- Produces: `sidecar.ProviderWedgedError` (exported struct), `sidecar.IsWedged(err error) (kind string, ok bool)` helper.

**Steps:**

1. Write the failing test. Append to `services/scraper/internal/sidecar/client_test.go`:

```go
// --- Phase 3: typed ProviderWedgedError on wedged-kind 503 -------------------

// TestResolveEmbed_WedgedKind_SurfacesTypedError verifies that a sidecar 503
// carrying a wedged `kind` (provider_wedged | pool_exhausted | the legacy
// `exhausted` alias | capacity | user_quota) surfaces a *ProviderWedgedError
// that (a) still satisfies errors.Is(err, domain.ErrProviderDown) so failover
// classification is unchanged, and (b) exposes Kind via sidecar.IsWedged so the
// circuit breaker can count it.
func TestResolveEmbed_WedgedKind_SurfacesTypedError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind string
	}{
		{"provider_wedged_503", http.StatusServiceUnavailable, `{"success":false,"error":"target closed","kind":"provider_wedged"}`, "provider_wedged"},
		{"pool_exhausted_503", http.StatusServiceUnavailable, `{"success":false,"error":"pool spin","kind":"pool_exhausted"}`, "pool_exhausted"},
		{"legacy_exhausted_503", http.StatusServiceUnavailable, `{"success":false,"error":"pool spin","kind":"exhausted"}`, "pool_exhausted"},
		{"capacity_503", http.StatusServiceUnavailable, `{"success":false,"error":"ram hard","kind":"capacity"}`, "capacity"},
		{"user_quota_503", http.StatusServiceUnavailable, `{"success":false,"error":"3rd session","kind":"user_quota"}`, "user_quota"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := New(srv.URL, 5*time.Second)
			_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
			if !errors.Is(err, domain.ErrProviderDown) {
				t.Fatalf("err = %v; want still errors.Is ErrProviderDown (failover unchanged)", err)
			}
			kind, ok := IsWedged(err)
			if !ok {
				t.Fatalf("IsWedged(%v) = (_, false); want a wedged kind", err)
			}
			if kind != tc.wantKind {
				t.Errorf("kind = %q; want %q", kind, tc.wantKind)
			}
			var pwe *ProviderWedgedError
			if !errors.As(err, &pwe) {
				t.Errorf("errors.As(*ProviderWedgedError) = false; want true")
			}
		})
	}
}

// TestResolveEmbed_NonWedgedKind_NotTyped verifies that a NON-wedged failure
// (502 challenge, plain error) stays a bare ErrProviderDown wrap — IsWedged is
// false so the breaker never counts a transient upstream challenge.
func TestResolveEmbed_NonWedgedKind_NotTyped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"challenge_502", http.StatusBadGateway, `{"success":false,"kind":"challenge"}`},
		{"error_502", http.StatusBadGateway, `{"success":false,"kind":"error"}`},
		{"internal_500", http.StatusInternalServerError, `{"success":false,"kind":"internal"}`},
		{"empty_kind_503", http.StatusServiceUnavailable, `{"success":false,"kind":""}`},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := New(srv.URL, 5*time.Second)
			_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
			if !errors.Is(err, domain.ErrProviderDown) {
				t.Fatalf("err = %v; want ErrProviderDown", err)
			}
			if kind, ok := IsWedged(err); ok {
				t.Errorf("IsWedged = (%q, true); want false for non-wedged kind", kind)
			}
		})
	}
}

// TestFetch_WedgedKind_SurfacesTypedError mirrors the resolve test for the
// /fetch path (nineanime discovery).
func TestFetch_WedgedKind_SurfacesTypedError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false, "kind": "provider_wedged", "error": "target closed",
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
	if kind, ok := IsWedged(err); !ok || kind != "provider_wedged" {
		t.Errorf("IsWedged = (%q, %v); want (provider_wedged, true)", kind, ok)
	}
}
```

2. Run it — expect a COMPILE failure (undefined: `ProviderWedgedError`, `IsWedged`):

```
cd services/scraper && go test ./internal/sidecar/ -run TestResolveEmbed_WedgedKind -count=1
# FAIL: ./internal/sidecar [build failed] — undefined: ProviderWedgedError, IsWedged
```

3. Implement. In `services/scraper/internal/sidecar/client.go`, add `"errors"` to the import group (currently `bytes context encoding/base64 encoding/json fmt io net/http strings time` + the domain pkg — `errors` is NOT present). Add the new type + helpers near the top, after the `maxFetchBody` const (~line 31):

```go
// wedgedKinds is the set of sidecar `kind` values that mean "this provider is
// wedged / over budget" (as opposed to a transient upstream challenge or a
// genuine stream failure). The Go circuit breaker (internal/health.Breaker)
// counts these per provider; the legacy `exhausted` alias is normalized to
// `pool_exhausted` so the breaker works whether or not the Phase-1 sidecar
// rename has shipped yet.
//
// Phase 1 sidecar kinds: provider_wedged, pool_exhausted (was `exhausted`).
// Phase 2 sidecar kinds: capacity, user_quota.
var wedgedKinds = map[string]string{
	"provider_wedged": "provider_wedged",
	"pool_exhausted":  "pool_exhausted",
	"exhausted":       "pool_exhausted", // legacy alias (pre-Phase-1 sidecar)
	"capacity":        "capacity",
	"user_quota":      "user_quota",
}

// ProviderWedgedError wraps domain.ErrProviderDown for the subset of sidecar
// failures that indicate the browser pool is wedged or over budget for this
// provider. It still satisfies errors.Is(err, domain.ErrProviderDown) (so the
// orchestrator's failover classifier treats it as retryable exactly as before),
// but it ALSO carries the machine-readable Kind so the circuit breaker can
// inspect the cause via sidecar.IsWedged / errors.As.
type ProviderWedgedError struct {
	Kind string
	err  error // the underlying domain.WrapProviderDown(...) value
}

func (e *ProviderWedgedError) Error() string {
	return fmt.Sprintf("sidecar provider wedged (kind=%s): %v", e.Kind, e.err)
}

// Unwrap exposes the wrapped domain.ErrProviderDown chain so errors.Is keeps
// matching the sentinel.
func (e *ProviderWedgedError) Unwrap() error { return e.err }

// IsWedged reports whether err is (or wraps) a *ProviderWedgedError and returns
// its normalized Kind. The breaker uses this; non-wedged errors return ("",false).
func IsWedged(err error) (string, bool) {
	var pwe *ProviderWedgedError
	if errors.As(err, &pwe) {
		return pwe.Kind, true
	}
	return "", false
}

// classifyDown builds the error for a sidecar non-OK response. When `kind` is a
// wedged kind it returns a *ProviderWedgedError (wrapping the ErrProviderDown
// value `base`); otherwise it returns `base` unchanged. Centralizes the wedged
// decision so resolve() and Fetch() stay in sync.
func classifyDown(kind string, base error) error {
	if norm, ok := wedgedKinds[kind]; ok {
		return &ProviderWedgedError{Kind: norm, err: base}
	}
	return base
}
```

   Then in `resolve()`, replace the non-OK branch (currently ~line 204-209):

```go
	if resp.StatusCode != http.StatusOK {
		base := domain.WrapProviderDown(
			fmt.Errorf("sidecar status %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: resolve",
		)
		return nil, classifyDown(out.Kind, base)
	}
```

   And in `Fetch()`, replace the non-OK branch (currently ~line 150-154):

```go
	if resp.StatusCode != http.StatusOK {
		base := domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: fetch")
		return 0, nil, classifyDown(out.Kind, base)
	}
```

4. Run — expect PASS:

```
cd services/scraper && go test ./internal/sidecar/ -count=1
# ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/sidecar
```

5. Commit:

```
git add services/scraper/internal/sidecar/client.go services/scraper/internal/sidecar/client_test.go
git commit -m "feat(scraper/sidecar): typed ProviderWedgedError surfaces sidecar wedged kind

503 bodies carrying kind=provider_wedged|pool_exhausted|capacity|user_quota now
surface a *ProviderWedgedError that still wraps domain.ErrProviderDown (failover
classification unchanged) but exposes Kind via sidecar.IsWedged for the circuit
breaker. Non-wedged kinds (challenge/error/internal) stay a bare ErrProviderDown.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P3.2 — Add `ProviderBreakerTripsTotal` metric

**Files:**
- `libs/metrics/provider.go` (add one `promauto.NewCounterVec` to the existing `var (...)` block)
- `libs/metrics/provider_test.go` (assert the metric registers + increments)

**Interfaces:**
- Consumes: nothing new (mirrors the existing `ProviderEnabled` / `ProviderInfo` collectors).
- Produces: `metrics.ProviderBreakerTripsTotal` (`*prometheus.CounterVec`, labels `{provider}`).

**Steps:**

1. Write the failing test. Append to `libs/metrics/provider_test.go`:

```go
// TestProviderBreakerTripsTotal_Increments verifies the breaker-trip counter is
// registered and increments under its provider label.
func TestProviderBreakerTripsTotal_Increments(t *testing.T) {
	before := testutil.ToFloat64(ProviderBreakerTripsTotal.WithLabelValues("nineanime_btt"))
	ProviderBreakerTripsTotal.WithLabelValues("nineanime_btt").Inc()
	after := testutil.ToFloat64(ProviderBreakerTripsTotal.WithLabelValues("nineanime_btt"))
	if d := after - before; d != 1.0 {
		t.Errorf("ProviderBreakerTripsTotal delta = %v; want 1.0", d)
	}
}
```

   (If `provider_test.go` does not already import `testutil`, add `"github.com/prometheus/client_golang/prometheus/testutil"`.)

2. Run — expect COMPILE failure (undefined: `ProviderBreakerTripsTotal`):

```
cd libs/metrics && go test ./... -run TestProviderBreakerTripsTotal -count=1
# FAIL [build failed] — undefined: ProviderBreakerTripsTotal
```

3. Implement. In `libs/metrics/provider.go`, add inside the existing `var (...)` block (after `ProviderInfo`, before the closing `)`):

```go
	// ProviderBreakerTripsTotal counts circuit-breaker trips per provider: each
	// time the scraper's in-memory breaker observes >=3 sidecar wedged-kind
	// errors within 60s and forces the provider's health-cache entry DOWN
	// (Camoufox pool self-heal, Phase 3). Cardinality is bounded by the provider
	// set (~7). A rising rate means a browser provider is wedging the sidecar
	// pool; pairs with stealth_pool_* sidecar metrics.
	ProviderBreakerTripsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "provider_breaker_trips_total",
			Help: "Total circuit-breaker trips per scraper provider (>=3 sidecar wedged-kind errors in 60s forced the provider health-cache DOWN)",
		},
		[]string{"provider"},
	)
```

4. Run — expect PASS:

```
cd libs/metrics && go test ./... -count=1
# ok  github.com/ILITA-hub/animeenigma/libs/metrics
```

5. Commit:

```
git add libs/metrics/provider.go libs/metrics/provider_test.go
git commit -m "feat(metrics): provider_breaker_trips_total counter{provider}

Counts scraper circuit-breaker trips (>=3 sidecar wedged-kind errors/60s forced
the provider health-cache DOWN). Bounded cardinality (~7 providers).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P3.3 — Circuit `Breaker` wiring the `InMemoryHealthCache`

**Files:**
- `services/scraper/internal/health/breaker.go` (NEW)
- `services/scraper/internal/health/breaker_test.go` (NEW)

**Interfaces:**
- Consumes: `*health.InMemoryHealthCache` (its `Update` + `IsHealthy`), `health.StageStreamSegment`, `metrics.ProviderBreakerTripsTotal` (P3.2).
- Produces: `health.Breaker`, `health.NewBreaker(cache *InMemoryHealthCache)`, `health.NewBreakerWithNow(cache, now)`, method `(*Breaker).Record(provider string, wedged bool)`.

**Design contract (from SHARED INTERFACE CONTRACT):** count wedged-kind errors per provider; trip at **>=3 within 60s** -> `cache.Update(provider, Up=false)`; **half-open after 120s** (allow one trial); **clear on one success**. The breaker holds no durable state.

> **Why `Record(provider, wedged bool)` and not `Record(err)`:** the breaker lives in `internal/health` which must NOT import `internal/sidecar` (sidecar imports domain; keeping health import-light avoids a future cycle and matches the existing health pkg, which imports only stdlib + libs). The CALLER (main.go, P3.6) classifies the sidecar error with `sidecar.IsWedged` and calls `Record(name, wedged)`. A non-wedged error or a success both call `Record(name, false)` so the success path can clear a tripped breaker.

**Mechanics:**
- Per provider, keep a slice of wedged-error timestamps within the trailing 60s window and a `trippedAt time.Time`.
- `Record(provider, true)`: append now; prune timestamps older than 60s; if the window count `>= 3`, set `trippedAt = now`, call `cache.Update(provider, downEntry(now))`, increment `metrics.ProviderBreakerTripsTotal`. While tripped-and-within-120s, additional wedged errors do NOT re-trip (idempotent) but DO re-stamp the DOWN entry's `LastUpdated` so the orchestrator skip-gate never goes stale during a sustained storm.
- `Record(provider, false)`: a success (or non-wedged error) — clear the window AND, if tripped, clear the trip and write an UP entry so the provider rejoins immediately.
- "Half-open after 120s": when `now - trippedAt >= 120s`, the breaker half-opens — reset the window and `trippedAt`, write an UP entry (so the next request reaches the provider), count this error toward a fresh window, and let the normal `>=3/60s` logic re-trip if it is still wedged.

> **downEntry/upEntry helper:** reuse the exact shape the orchestrator tests already rely on (`StageStreamSegment` Up=false/true, `LastUpdated=now`) — that is the single key `IsHealthy` reads (`cache.go:126`).

**Steps:**

1. Write the failing test. Create `services/scraper/internal/health/breaker_test.go`:

```go
package health

import (
	"testing"
	"time"
)

// fakeClock is a deterministic time source for breaker tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time         { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestBreaker() (*Breaker, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	cache := NewInMemoryHealthCacheWithNow(clk.now)
	b := NewBreakerWithNow(cache, clk.now)
	return b, clk
}

// TestBreaker_TripsAtThreeWithin60s: two wedged errors do NOT trip; the third
// within 60s forces the provider DOWN in the cache.
func TestBreaker_TripsAtThreeWithin60s(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"

	b.Record(p, true)
	clk.advance(10 * time.Second)
	b.Record(p, true)
	if !b.cache.IsHealthy(p) {
		t.Fatalf("after 2 wedged errors, IsHealthy = false; want true (below threshold)")
	}
	clk.advance(10 * time.Second)
	b.Record(p, true) // 3rd within 60s -> trip
	if b.cache.IsHealthy(p) {
		t.Errorf("after 3 wedged errors in 60s, IsHealthy = true; want false (tripped)")
	}
}

// TestBreaker_WindowSlides: errors spread > 60s apart never reach 3-in-window.
func TestBreaker_WindowSlides(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "gogoanime"
	b.Record(p, true)
	clk.advance(40 * time.Second)
	b.Record(p, true)
	clk.advance(40 * time.Second) // first error now 80s old -> pruned
	b.Record(p, true)             // only 2 within the trailing 60s
	if !b.cache.IsHealthy(p) {
		t.Errorf("spread-out errors tripped the breaker; want still healthy (window slid)")
	}
}

// TestBreaker_ClearsOnSuccess: a success after a trip writes the provider UP.
func TestBreaker_ClearsOnSuccess(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"
	for i := 0; i < 3; i++ {
		b.Record(p, true)
		clk.advance(5 * time.Second)
	}
	if b.cache.IsHealthy(p) {
		t.Fatalf("precondition: breaker should be tripped")
	}
	b.Record(p, false) // success
	if !b.cache.IsHealthy(p) {
		t.Errorf("after success, IsHealthy = false; want true (breaker cleared)")
	}
}

// TestBreaker_HalfOpenAfter120s: 120s after the trip the breaker half-opens
// (provider rejoins for a trial). If still wedged, 3 more within 60s re-trip.
func TestBreaker_HalfOpenAfter120s(t *testing.T) {
	b, clk := newTestBreaker()
	const p = "nineanime"
	for i := 0; i < 3; i++ {
		b.Record(p, true)
		clk.advance(1 * time.Second)
	}
	if b.cache.IsHealthy(p) {
		t.Fatalf("precondition: tripped")
	}
	// Within the 120s closed window, additional wedged errors keep it DOWN.
	clk.advance(30 * time.Second)
	b.Record(p, true)
	if b.cache.IsHealthy(p) {
		t.Errorf("within 120s, IsHealthy = true; want false (still tripped)")
	}
	// Cross the 120s half-open boundary: the next wedged error is the trial that
	// re-opens the window; the provider is briefly healthy again.
	clk.advance(100 * time.Second) // now ~131s past trip
	b.Record(p, true)              // half-open trial -> resets window, writes UP, counts 1
	if !b.cache.IsHealthy(p) {
		t.Errorf("after 120s half-open, IsHealthy = false; want true (trial allowed through)")
	}
	// Two more wedged within 60s re-trip.
	clk.advance(5 * time.Second)
	b.Record(p, true)
	clk.advance(5 * time.Second)
	b.Record(p, true)
	if b.cache.IsHealthy(p) {
		t.Errorf("after re-tripping post-half-open, IsHealthy = true; want false")
	}
}

// TestBreaker_PerProviderIsolation: tripping nineanime does NOT down gogoanime.
func TestBreaker_PerProviderIsolation(t *testing.T) {
	b, clk := newTestBreaker()
	for i := 0; i < 3; i++ {
		b.Record("nineanime", true)
		clk.advance(1 * time.Second)
	}
	if b.cache.IsHealthy("nineanime") {
		t.Fatalf("nineanime should be tripped")
	}
	if !b.cache.IsHealthy("gogoanime") {
		t.Errorf("gogoanime IsHealthy = false; want true (per-provider isolation)")
	}
}
```

2. Run — expect COMPILE failure (undefined: `Breaker`, `NewBreakerWithNow`):

```
cd services/scraper && go test ./internal/health/ -run TestBreaker -count=1
# FAIL [build failed] — undefined: Breaker, NewBreakerWithNow
```

3. Implement. Create `services/scraper/internal/health/breaker.go`:

```go
package health

import (
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// breakerWindow is the trailing window over which wedged errors are counted.
const breakerWindow = 60 * time.Second

// breakerThreshold is the wedged-error count within breakerWindow that trips
// the breaker (forces the provider's health-cache entry DOWN).
const breakerThreshold = 3

// breakerHalfOpen is how long a tripped breaker stays closed before it lets one
// trial request through (half-open). It mirrors the spec's "half-open after
// 120s". Must be > breakerWindow so a single storm cannot immediately re-trip
// through the half-open path.
const breakerHalfOpen = 120 * time.Second

// providerBreaker is the per-provider breaker state.
type providerBreaker struct {
	fails     []time.Time // wedged-error timestamps within the trailing window
	trippedAt time.Time   // zero == not tripped
}

// Breaker is a per-provider circuit breaker that drives the InMemoryHealthCache.
// It counts sidecar "wedged" errors (provider pool wedged / over budget) and,
// at breakerThreshold within breakerWindow, forces the provider's health-cache
// entry DOWN so the orchestrator skips it per-request (orchestrator.go:317,536).
// It half-opens after breakerHalfOpen and clears on a single success.
//
// The breaker holds NO durable state beyond the in-memory cache it drives — it
// is safe across restarts (the durable signal of record is the catalog
// stream_providers.status row, written by the Phase 5 probe).
//
// Locking: a single mutex guards the per-provider map; no I/O under the lock.
type Breaker struct {
	mu    sync.Mutex
	cache *InMemoryHealthCache
	state map[string]*providerBreaker
	now   func() time.Time
}

// NewBreaker wires a breaker to an InMemoryHealthCache using the wall clock.
func NewBreaker(cache *InMemoryHealthCache) *Breaker {
	return NewBreakerWithNow(cache, time.Now)
}

// NewBreakerWithNow is the test constructor (injectable clock).
func NewBreakerWithNow(cache *InMemoryHealthCache, now func() time.Time) *Breaker {
	return &Breaker{
		cache: cache,
		state: make(map[string]*providerBreaker),
		now:   now,
	}
}

// Record feeds one sidecar outcome for `provider` into the breaker. wedged=true
// means the sidecar returned a wedged-kind error (sidecar.IsWedged); wedged=false
// means a success OR a non-wedged failure (challenge/empty) — both are treated
// as evidence the pool is NOT wedged and clear a tripped breaker.
//
// A nil receiver is a no-op so callers can run unconditionally even when the
// breaker is not configured.
func (b *Breaker) Record(provider string, wedged bool) {
	if b == nil || provider == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	pb := b.state[provider]
	if pb == nil {
		pb = &providerBreaker{}
		b.state[provider] = pb
	}
	now := b.now()

	if !wedged {
		// Success / non-wedged: clear the window; if tripped, rejoin immediately.
		pb.fails = pb.fails[:0]
		if !pb.trippedAt.IsZero() {
			pb.trippedAt = time.Time{}
			b.cache.Update(provider, upEntry(now))
		}
		return
	}

	// Tripped + within the closed window: keep it DOWN (re-stamp so the skip
	// never goes stale during a sustained storm) and do NOT re-trip.
	if !pb.trippedAt.IsZero() && now.Sub(pb.trippedAt) < breakerHalfOpen {
		b.cache.Update(provider, downEntry(now))
		return
	}

	// Tripped + past half-open: this wedged error is the trial. Reset to a fresh
	// window (counting this error) and rejoin (UP) so one request gets through;
	// if it is still wedged, the normal threshold logic below re-trips it.
	if !pb.trippedAt.IsZero() {
		pb.trippedAt = time.Time{}
		pb.fails = pb.fails[:0]
		b.cache.Update(provider, upEntry(now))
	}

	// Append + prune to the trailing window.
	pb.fails = append(pb.fails, now)
	cutoff := now.Add(-breakerWindow)
	kept := pb.fails[:0]
	for _, t := range pb.fails {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	pb.fails = kept

	if len(pb.fails) >= breakerThreshold {
		pb.trippedAt = now
		b.cache.Update(provider, downEntry(now))
		metrics.ProviderBreakerTripsTotal.WithLabelValues(provider).Inc()
	}
}

// downEntry / upEntry build the single-stage cache entry IsHealthy reads
// (StageStreamSegment, LastUpdated=now). Mirrors the orchestrator test helpers.
func downEntry(now time.Time) ProviderHealth {
	return ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: false, LastErr: "circuit breaker: provider wedged"}},
		LastUpdated: now,
	}
}

func upEntry(now time.Time) ProviderHealth {
	return ProviderHealth{
		Stages:      map[string]StageStatus{StageStreamSegment: {Up: true, LastOK: now}},
		LastUpdated: now,
	}
}
```

   > **Half-open test note:** in `TestBreaker_HalfOpenAfter120s`, after the trial `Record(p,true)` resets the window to 1 and writes UP, two more wedged errors push the window back to 3 and re-trip. The assertions above match this exactly.

4. Run — expect PASS:

```
cd services/scraper && go test ./internal/health/ -count=1
# ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/health
```

5. Commit:

```
git add services/scraper/internal/health/breaker.go services/scraper/internal/health/breaker_test.go
git commit -m "feat(scraper/health): per-provider circuit breaker driving the health cache

Counts sidecar wedged-kind errors per provider; >=3/60s forces the provider's
InMemoryHealthCache stream_segment entry DOWN so the orchestrator skips it
per-request (orchestrator.go:317,536). Half-opens after 120s (one trial), clears
on a success. Injectable clock; no durable state. Increments
provider_breaker_trips_total on each trip.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P3.4 — Orchestrator runtime re-gate: `ApplyStatuses`

**Files:**
- `services/scraper/internal/service/orchestrator.go` (add `ApplyStatuses`; cite the existing skip-gate)
- `services/scraper/internal/service/orchestrator_regate_test.go` (NEW)

**Interfaces:**
- Consumes: the orchestrator's own `providers` slice + `degraded` map (already present).
- Produces: `(*Orchestrator).ApplyStatuses(statuses map[string]string)` where each value is `"enabled"` or `"degraded"`.

**Confirm the skip-gate (cite, do not change):** the spec's "orchestrator already skips `!IsHealthy`" is true and lives in TWO places:
- `runFailoverNamed` — `orchestrator.go:317`: `if cache != nil && !cache.IsHealthy(p.Name()) { ... continue }`.
- `GetStreamGated` — `orchestrator.go:536`: `if o.cache != nil && !o.cache.IsHealthy(p.Name()) { ... }`.
So once P3.3's breaker forces a provider's cache entry DOWN, BOTH the gated and non-gated failover paths skip it per-request. No change needed there — Task P3.4 is ONLY the catalog-status-driven `degraded`-map re-gate.

**Scope boundary (important):** at boot, `registerByStatus` (main.go:242) does NOT register `disabled` providers into the orchestrator at all. Therefore runtime re-gate can only move providers that ARE registered (enabled <-> degraded). A provider that flips to `disabled` in the DB at runtime stays registered until the next restart — but the breaker + probe path (cache DOWN) already covers "stop sending it traffic", and a human flipping to `disabled` is the explicit human-only terminal action that is allowed to wait for a restart. `ApplyStatuses` therefore accepts only `enabled`/`degraded`; any other value (including `disabled`) leaves membership unchanged. This matches D5: "Automatic enabled <-> degraded; disabled stays human-only."

**Steps:**

1. Write the failing test. Create `services/scraper/internal/service/orchestrator_regate_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// regateProvider is a minimal registered provider for re-gate order assertions.
func regateProvider(name string) *fakeProvider {
	return &fakeProvider{
		nameVal: name,
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: name}}, nil
		},
	}
}

// TestOrchestrator_ApplyStatuses_EnabledToDegraded: a provider enabled at boot
// is dropped from the auto-failover order after ApplyStatuses marks it degraded
// (but stays reachable via an explicit prefer).
func TestOrchestrator_ApplyStatuses_EnabledToDegraded(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	b := regateProvider("nineanime")
	o.Register(a)
	o.Register(b)

	// Both in the auto order initially.
	if got := o.OrderedProviderNames("", false); len(got) != 2 {
		t.Fatalf("initial auto order = %v; want both", got)
	}

	o.ApplyStatuses(map[string]string{"gogoanime": "enabled", "nineanime": "degraded"})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 1 || auto[0] != "gogoanime" {
		t.Errorf("auto order = %v; want [gogoanime] (nineanime degraded out)", auto)
	}
	// Still reachable via explicit prefer.
	pref := o.OrderedProviderNames("nineanime", false)
	if len(pref) != 2 || pref[0] != "nineanime" {
		t.Errorf("prefer order = %v; want [nineanime, gogoanime]", pref)
	}
}

// TestOrchestrator_ApplyStatuses_DegradedToEnabled: a provider degraded at boot
// rejoins the auto order after ApplyStatuses marks it enabled.
func TestOrchestrator_ApplyStatuses_DegradedToEnabled(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	d := regateProvider("nineanime")
	o.Register(a)
	o.RegisterDegraded(d) // boot-degraded

	if got := o.OrderedProviderNames("", false); len(got) != 1 || got[0] != "gogoanime" {
		t.Fatalf("initial auto order = %v; want [gogoanime] only", got)
	}

	o.ApplyStatuses(map[string]string{"gogoanime": "enabled", "nineanime": "enabled"})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 2 {
		t.Errorf("auto order = %v; want both after re-enable", auto)
	}
}

// TestOrchestrator_ApplyStatuses_UnknownAndDisabledIgnored: names not registered
// in the orchestrator, and a "disabled"/garbage status value, are no-ops (no
// panic, no spurious add). Registered providers absent from the map keep their
// current degraded state.
func TestOrchestrator_ApplyStatuses_UnknownAndDisabledIgnored(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	d := regateProvider("nineanime")
	o.Register(a)
	o.RegisterDegraded(d)

	o.ApplyStatuses(map[string]string{
		"gogoanime":   "enabled",
		"nineanime":   "disabled", // not enabled/degraded -> ignored, stays degraded
		"allanime":    "enabled",  // not registered -> ignored, must NOT be added
		"bogus_value": "weird",
	})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 1 || auto[0] != "gogoanime" {
		t.Errorf("auto order = %v; want [gogoanime] (nineanime stays degraded, allanime never added)", auto)
	}
	// allanime must not have been added to the provider set.
	for _, n := range o.OrderedProviderNames("allanime", false) {
		if n == "allanime" {
			t.Errorf("allanime appeared in provider set; ApplyStatuses must not add unregistered providers")
		}
	}
}
```

2. Run — expect COMPILE failure (undefined: `ApplyStatuses`):

```
cd services/scraper && go test ./internal/service/ -run TestOrchestrator_ApplyStatuses -count=1
# FAIL [build failed] — o.ApplyStatuses undefined
```

3. Implement. In `services/scraper/internal/service/orchestrator.go`, add after `RegisterDegraded` (~line 94):

```go
// ApplyStatuses re-gates the auto-failover membership of ALREADY-REGISTERED
// providers at runtime, so a catalog status change (from the Phase 5 probe or a
// manual catalog status-write) takes effect WITHOUT a scraper restart. Each
// map value MUST be "enabled" or "degraded":
//
//   - "enabled"  -> remove from the degraded set (rejoins auto-failover)
//   - "degraded" -> add to the degraded set (excluded from auto-failover, still
//     reachable via an explicit `prefer`)
//
// Providers NOT registered in this orchestrator are ignored (a provider the DB
// marks `disabled` was never registered at boot — ApplyStatuses never ADDS a
// provider, only flips the degraded flag of one already present). Any status
// value other than "enabled"/"degraded" (including "disabled") is ignored for
// that provider, leaving its current membership unchanged. This honours the
// "disabled is human-only / restart-gated" invariant (D5).
//
// Registered providers absent from `statuses` keep their current state.
func (o *Orchestrator) ApplyStatuses(statuses map[string]string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.degraded == nil {
		o.degraded = make(map[string]bool)
	}
	// Build the set of currently-registered names so we never add an unknown.
	registered := make(map[string]bool, len(o.providers))
	for _, p := range o.providers {
		registered[p.Name()] = true
	}
	for name, status := range statuses {
		if !registered[name] {
			continue // never add an unregistered (e.g. boot-disabled) provider
		}
		switch status {
		case "enabled":
			delete(o.degraded, name)
		case "degraded":
			o.degraded[name] = true
		default:
			// "disabled" or garbage: leave membership unchanged (human/restart-gated).
		}
	}
}
```

4. Run — expect PASS:

```
cd services/scraper && go test ./internal/service/ -count=1
# ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/service
```

5. Commit:

```
git add services/scraper/internal/service/orchestrator.go services/scraper/internal/service/orchestrator_regate_test.go
git commit -m "feat(scraper/service): Orchestrator.ApplyStatuses runtime enabled<->degraded re-gate

Moves already-registered providers in/out of the degraded auto-failover map at
runtime so a catalog status change takes effect without a scraper restart. Never
adds an unregistered provider; disabled/garbage values leave membership unchanged
(disabled stays restart/human-gated, D5). The breaker's cache-DOWN skip-gate
(orchestrator.go:317,536) already handles per-request wedged-provider skipping.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P3.5 — Refresher callback so each poll triggers the re-gate

**Files:**
- `services/scraper/internal/config/providers_refresh.go` (add `onRefresh func()` param)
- `services/scraper/internal/config/providers_refresh_test.go` (extend)

**Interfaces:**
- Consumes: existing `ProvidersConfig.Replace`, `LoadProvidersRemote`.
- Produces: new signature `StartProvidersRefresher(ctx, target, catalogURL, interval, log, onRefresh func())`. The callback fires AFTER a successful `Replace`, on the refresher goroutine, every poll. Keeps `config` free of any `service` import (the callback is supplied by main.go).

> **Caller-update note:** `StartProvidersRefresher` has exactly one production caller (`cmd/scraper-api/main.go:543`) and one test file. Both are updated in this plan (main.go in P3.6). The callback is `nil`-safe.

**Steps:**

1. Write the failing test. Extend `services/scraper/internal/config/providers_refresh_test.go` — add the import block + two tests (merge with the existing `import "sync"`/`testing` block rather than duplicating):

```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestStartProvidersRefresher_FiresCallbackEachPoll verifies the onRefresh
// callback runs after each successful refresh (so the orchestrator re-gate
// reflects the freshly-loaded catalog status without a restart).
func TestStartProvidersRefresher_FiresCallbackEachPoll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"providers":[
			{"name":"gogoanime","status":"enabled","scraper_operated":true}
		]}}`))
	}))
	defer srv.Close()

	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "gogoanime", Status: StatusEnabled}})
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartProvidersRefresher(ctx, &pc, srv.URL, 20*time.Millisecond, nil, func() {
		atomic.AddInt32(&calls, 1)
	})

	// Wait for at least 2 polls.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) < 2 {
		select {
		case <-deadline:
			t.Fatalf("callback fired %d times; want >= 2", atomic.LoadInt32(&calls))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestStartProvidersRefresher_CallbackNotFiredOnFailure verifies a failed
// refresh (catalog 500) does NOT fire the callback (last-good config kept).
func TestStartProvidersRefresher_CallbackNotFiredOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "gogoanime", Status: StatusEnabled}})
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartProvidersRefresher(ctx, &pc, srv.URL, 20*time.Millisecond, nil, func() {
		atomic.AddInt32(&calls, 1)
	})
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("callback fired %d times on failing refresh; want 0", got)
	}
}
```

2. Run — expect COMPILE failure (not enough arguments to `StartProvidersRefresher`):

```
cd services/scraper && go test ./internal/config/ -run TestStartProvidersRefresher -count=1
# FAIL [build failed] — not enough arguments in call to StartProvidersRefresher
```

3. Implement. Replace the body of `StartProvidersRefresher` in `services/scraper/internal/config/providers_refresh.go`:

```go
// StartProvidersRefresher periodically re-fetches provider config from catalog
// and atomically swaps it into target via Replace. Runs until ctx is canceled.
// A failed refresh keeps the last-good config (logged at WARN). No-op if
// catalogURL is empty or interval <= 0.
//
// onRefresh (nil-safe) is invoked AFTER each successful Replace, on the
// refresher goroutine. The scraper wires it to Orchestrator.ApplyStatuses so a
// catalog status change re-gates the failover roster without a restart. It is
// called every poll (idempotent) — ApplyStatuses is a cheap map walk.
func StartProvidersRefresher(ctx context.Context, target *ProvidersConfig, catalogURL string, interval time.Duration, log Logger, onRefresh func()) {
	if catalogURL == "" || interval <= 0 || target == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pc, err := LoadProvidersRemote(ctx, catalogURL, nil, 5*time.Second)
				if err != nil {
					if log != nil {
						log.Warnw("provider config refresh failed; keeping last-good", "error", err)
					}
					continue
				}
				entries := make([]ProviderMeta, 0)
				for _, m := range pc.load() {
					entries = append(entries, m)
				}
				target.Replace(entries)
				if log != nil {
					log.Infow("provider config refreshed", "disabled", target.DisabledNames())
				}
				if onRefresh != nil {
					onRefresh()
				}
			}
		}
	}()
}
```

4. Run — expect PASS:

```
cd services/scraper && go test ./internal/config/ -count=1
# ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/config
```

5. Commit:

```
git add services/scraper/internal/config/providers_refresh.go services/scraper/internal/config/providers_refresh_test.go
git commit -m "feat(scraper/config): refresher onRefresh callback for runtime re-gate

StartProvidersRefresher now takes a nil-safe onRefresh func() fired after each
successful catalog refresh. main.go wires it to Orchestrator.ApplyStatuses so a
catalog status change re-gates the failover roster without a restart. config
stays free of any service import (callback supplied by main).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task P3.6 — Wire the breaker + re-gate callback in `main.go`

**Files:**
- `services/scraper/cmd/scraper-api/main.go` (construct breaker; wrap the four sidecar-call closures with `Record`; hoist `candidateProviders`; build the enabled<->degraded status map; pass the callback to `StartProvidersRefresher`)

**Interfaces:**
- Consumes: `health.NewBreaker` (P3.3), `sidecar.IsWedged` (P3.1), `Orchestrator.ApplyStatuses` (P3.4), `StartProvidersRefresher(...,onRefresh)` (P3.5), existing `cfg.Providers` (`Status`, `DegradedNames`, `DisabledNames`).
- Produces: live wiring (no new exported symbols).

> This is integration glue. There is no cheap unit test for `main()`; the behaviour is covered by P3.1–P3.5 unit tests + the spec's E2E (black-hole `SCRAPER_NINEANIME` host). The verification step is a build + `go vet` + the full package test suite.

**Steps:**

1. Construct the breaker right after the health cache is built. In `main.go`, immediately after `cache := health.NewInMemoryHealthCache()` (~line 213), add:

```go
	// Phase 3 (Camoufox self-heal) — per-provider circuit breaker that drives the
	// same health cache the orchestrator skip-gates on. The sidecar-call closures
	// below feed it; >=3 wedged-kind sidecar errors/60s force the provider DOWN so
	// the orchestrator skips it per-request (protecting the healthy provider in
	// real time, not in 15 min).
	breaker := health.NewBreaker(cache)
```

2. Wrap the gogoanime browser-resolve closure (~line 309):

```go
	gogoBrowserResolve := func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
		st, err := stealthClient.ResolveEmbed(ctx, "gogoanime", embedURL, category, cfg.Providers.BaseURLOf("gogoanime"))
		_, wedged := sidecar.IsWedged(err)
		breaker.Record("gogoanime", wedged)
		return st, err
	}
```

   > `IsWedged(nil)` returns `("", false)`, so a success records `wedged=false` and clears any tripped breaker. A non-wedged error also records `false` (correct: a challenge is not pool-wedge evidence).

3. Wrap the nineanime browser-resolve + browser-fetch closures (~line 448-453):

```go
	nineBrowserResolve := func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
		st, err := stealthClient.ResolveEmbed(ctx, "nineanime", embedURL, category, cfg.Providers.BaseURLOf("nineanime"))
		_, wedged := sidecar.IsWedged(err)
		breaker.Record("nineanime", wedged)
		return st, err
	}
	nineBrowserFetch := func(ctx context.Context, provider, url string) (int, []byte, error) {
		status, body, err := stealthClient.Fetch(ctx, provider, url)
		_, wedged := sidecar.IsWedged(err)
		breaker.Record(provider, wedged)
		return status, body, err
	}
```

4. Hoist `candidateProviders` + build the re-gate callback. The `candidateProviders` slice is currently declared inside the wiring-invariant block (~line 565), AFTER the `StartProvidersRefresher` call (~line 543). Move its declaration UP to just before the `StartProvidersRefresher` call site (keep the later wiring-invariant block referencing the same slice). Then replace the `StartProvidersRefresher(...)` call with:

```go
	// Phase 19/28 candidate set — also drives the Phase-3 runtime re-gate below.
	// (Moved up from the wiring-invariant block so the regate closure can range
	// over it; the invariant block still reads this same slice.)
	candidateProviders := []string{"gogoanime", "animepahe", "allanime", "animefever", "miruro", "nineanime"}
	if cfg.AnimeKai.Enabled {
		candidateProviders = append(candidateProviders, "animekai")
	}

	// Phase 3 — runtime re-gate: each catalog refresh moves providers in/out of
	// the orchestrator's degraded failover map without a restart. Only EN-group
	// candidate providers that are REGISTERED (enabled or degraded at boot) are
	// re-gated; disabled providers were never registered and stay restart-gated
	// (D5). The adult orchestrator is intentionally NOT re-gated (single fixed
	// 18+ provider).
	regate := func() {
		statuses := make(map[string]string, len(candidateProviders))
		for _, name := range candidateProviders {
			switch cfg.Providers.Status(name) {
			case config.StatusEnabled:
				statuses[name] = "enabled"
			case config.StatusDegraded:
				statuses[name] = "degraded"
				// StatusDisabled: omitted — never re-gated at runtime (D5).
			}
		}
		orchestrator.ApplyStatuses(statuses)
		log.Infow("orchestrator re-gated from catalog status",
			"degraded", cfg.Providers.DegradedNames(),
			"disabled", cfg.Providers.DisabledNames())
	}

	// Hot-reload provider config from catalog (enable/disable without restart) and
	// re-gate the failover roster after each successful refresh.
	config.StartProvidersRefresher(context.Background(), &cfg.Providers, cfg.CatalogURL, cfg.ProvidersRefresh, log, regate)
```

   > **Invariant-block note:** after hoisting, DELETE the now-duplicate `candidateProviders := ...` (+ the `if cfg.AnimeKai.Enabled { append }`) that currently lives ~line 565 inside the wiring-invariant block — the block keeps using the hoisted slice. This is a pure move, not a logic change. Verify with `go vet` (it will flag a redeclaration if a copy is left behind).

5. Verify the whole service builds + vets + tests:

```
cd services/scraper && go build ./... && go vet ./... && go test ./... -count=1
# expect: ok across sidecar, health, service, config; no vet warnings
cd /data/ae-camoufox/libs/metrics && go test ./... -count=1
# expect: ok
```

6. Commit:

```
git add services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): wire circuit breaker + runtime re-gate into scraper-api

Feeds every gogoanime/nineanime sidecar resolve+fetch outcome into the per-
provider breaker (sidecar.IsWedged -> breaker.Record); >=3 wedged-kind errors/60s
force the provider DOWN in the health cache so the orchestrator skips it
per-request. Passes a re-gate callback to StartProvidersRefresher so each catalog
refresh moves EN providers in/out of the degraded failover map (enabled<->degraded)
without a restart; disabled stays restart-gated (D5).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Phase 3 verification (run after all tasks)

```
cd services/scraper && go build ./... && go vet ./... && go test ./... -race -count=1
cd /data/ae-camoufox/libs/metrics && go test ./... -count=1
```

All green = Phase 3 complete. Deploy: `make redeploy-scraper`. The sidecar (Phases 1-2) can deploy independently; Phase 3 degrades gracefully if the sidecar still emits the legacy `exhausted` kind (normalized to `pool_exhausted`) or has not yet added `capacity`/`user_quota` (those kinds simply never arrive until Phase 2 ships).

**Scoring (`.planning/CONVENTIONS.md`):** UXΔ = +2 (Better) — a wedged browser provider stops poisoning the healthy one within ~3 strikes instead of 15 min, and recovered providers rejoin failover on the next catalog poll with no restart. CDI = 0.05 * 21 — single-service spread (scraper + one shared libs/metrics counter), one genuinely new pattern (in-process breaker driving the health cache + orchestrator runtime re-gate superseding the boot-frozen status). MVQ = Phoenix 86%/82% 🔥 — the wedged provider dies out of failover and self-resurrects via half-open + catalog re-gate; resists slop via table-driven tests with an injected clock + a fail-open posture (nil breaker / stale cache both keep dispatching).

---

## Phase 4: Catalog status-write endpoint

Add `POST /internal/scraper/providers/{name}/status` to the catalog service — a Docker-network-only endpoint that lets the analytics probe (Phase 5) and the Go circuit breaker (Phase 3) flip a provider's lifecycle between `enabled` and `degraded` without restarting the service or touching SQL manually. `disabled` is a human-only terminal state; the endpoint refuses to set it and refuses to change any provider that is already `disabled`.

---

### Task P4.1 — Failing test: `UpdateStatus` handler behaviours

**Files:**
- `/data/ae-camoufox/services/catalog/internal/handler/internal_scraper_providers_test.go` (append after the existing `TestInternalScraperProviders_List` test)

**Interfaces — Consumes:**
- `domain.ScraperProvider{Name, Status}`, `domain.StatusEnabled`, `domain.StatusDegraded`, `domain.StatusDisabled`
- `gorm.DB` (SQLite in-memory)
- `handler.NewInternalScraperProvidersHandler(db, log) *InternalScraperProvidersHandler`
- `httputil.OK`, `httputil.Error` (via response shape `{success, data}` / `{success, error}`)

**Interfaces — Produces:**
- Test cases that drive the exact method signature `(h *InternalScraperProvidersHandler) UpdateStatus(w, r)` and the route shape `POST /internal/scraper/providers/{name}/status`

**Steps:**

1. **Write the tests** (they will fail until Task P4.2 adds the implementation):

```go
// Append to: services/catalog/internal/handler/internal_scraper_providers_test.go
// package handler_test  (already declared in the existing file)

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
    "github.com/go-chi/chi/v5"
    "go.uber.org/zap"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

// openStatusTestDB returns an in-memory SQLite DB pre-seeded with a small
// roster for UpdateStatus tests. The DB is NOT shared with other test funcs.
func openStatusTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatal(err)
    }
    if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
        t.Fatal(err)
    }
    rows := []domain.ScraperProvider{
        {Name: "gogoanime",  Status: domain.StatusEnabled,  Group: "en", SubDelivery: "hard"},
        {Name: "animefever", Status: domain.StatusDegraded, Group: "en", SubDelivery: "hard"},
        {Name: "animepahe",  Status: domain.StatusDisabled, Group: "en", SubDelivery: "hard"},
    }
    for i := range rows {
        if err := db.Create(&rows[i]).Error; err != nil {
            t.Fatal(err)
        }
    }
    return db
}

// fireStatusRequest posts to POST /internal/scraper/providers/{name}/status
// through a minimal chi router that mirrors the production registration.
func fireStatusRequest(t *testing.T, h *handler.InternalScraperProvidersHandler, name, body string) *httptest.ResponseRecorder {
    t.Helper()
    r := chi.NewRouter()
    r.Post("/internal/scraper/providers/{name}/status", h.UpdateStatus)

    req := httptest.NewRequest(http.MethodPost,
        "/internal/scraper/providers/"+name+"/status",
        bytes.NewBufferString(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)
    return rec
}

func nopLog() *logger.Logger {
    return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// TestUpdateStatus_HappyPath_EnabledToDegraded — enabled → degraded returns 200
// and persists the new status.
func TestUpdateStatus_HappyPath_EnabledToDegraded(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    rec := fireStatusRequest(t, h, "gogoanime",
        `{"status":"degraded","reason":"probe scored 0%"}`)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
    }

    // Verify DB was updated.
    var got domain.ScraperProvider
    if err := db.First(&got, "name = ?", "gogoanime").Error; err != nil {
        t.Fatal(err)
    }
    if got.Status != domain.StatusDegraded {
        t.Errorf("DB status = %q, want degraded", got.Status)
    }
    if got.Reason != "probe scored 0%" {
        t.Errorf("DB reason = %q, want 'probe scored 0%%'", got.Reason)
    }
}

// TestUpdateStatus_HappyPath_DegradedToEnabled — degraded → enabled returns 200.
func TestUpdateStatus_HappyPath_DegradedToEnabled(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    rec := fireStatusRequest(t, h, "animefever",
        `{"status":"enabled","reason":"2 consecutive UP probe runs"}`)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
    }

    var got domain.ScraperProvider
    if err := db.First(&got, "name = ?", "animefever").Error; err != nil {
        t.Fatal(err)
    }
    if got.Status != domain.StatusEnabled {
        t.Errorf("DB status = %q, want enabled", got.Status)
    }
}

// TestUpdateStatus_RefusesSetDisabled — any attempt to set status="disabled"
// returns 409 regardless of the provider's current state.
func TestUpdateStatus_RefusesSetDisabled(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    rec := fireStatusRequest(t, h, "gogoanime",
        `{"status":"disabled","reason":"attacker"}`)

    if rec.Code != http.StatusConflict {
        t.Fatalf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
    }
    // DB must be untouched.
    var got domain.ScraperProvider
    db.First(&got, "name = ?", "gogoanime")
    if got.Status != domain.StatusEnabled {
        t.Errorf("DB status changed to %q, want enabled (unchanged)", got.Status)
    }
}

// TestUpdateStatus_RefusesChangeAlreadyDisabled — a provider already "disabled"
// must never be touched by this endpoint, even if target status is valid.
func TestUpdateStatus_RefusesChangeAlreadyDisabled(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    for _, target := range []string{"enabled", "degraded"} {
        t.Run("target="+target, func(t *testing.T) {
            rec := fireStatusRequest(t, h, "animepahe",
                `{"status":"`+target+`","reason":"trying to change disabled"}`)

            if rec.Code != http.StatusConflict {
                t.Fatalf("status = %d, want 409 for target=%s (body=%s)",
                    rec.Code, target, rec.Body.String())
            }
            var got domain.ScraperProvider
            db.First(&got, "name = ?", "animepahe")
            if got.Status != domain.StatusDisabled {
                t.Errorf("DB status changed to %q, want disabled (unchanged)", got.Status)
            }
        })
    }
}

// TestUpdateStatus_UnknownProvider_404 — provider not in stream_providers → 404.
func TestUpdateStatus_UnknownProvider_404(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    rec := fireStatusRequest(t, h, "phantom",
        `{"status":"degraded","reason":"does not exist"}`)

    if rec.Code != http.StatusNotFound {
        t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
    }
}

// TestUpdateStatus_BadBody_400 — malformed JSON / missing required fields.
func TestUpdateStatus_BadBody_400(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    cases := []struct {
        name string
        body string
    }{
        {"not-json", `{bad}`},
        {"missing-status", `{"reason":"no status field"}`},
        {"unknown-status-value", `{"status":"quarantined","reason":"not a real status"}`},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            rec := fireStatusRequest(t, h, "gogoanime", tc.body)
            if rec.Code != http.StatusBadRequest {
                t.Fatalf("%s: status = %d, want 400 (body=%s)",
                    tc.name, rec.Code, rec.Body.String())
            }
        })
    }
}

// TestUpdateStatus_Idempotent_SameStatus — writing the current status again
// returns 200 and leaves the row unchanged (idempotent UPDATE).
func TestUpdateStatus_Idempotent_SameStatus(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    // gogoanime is already enabled; write enabled → still 200.
    rec := fireStatusRequest(t, h, "gogoanime",
        `{"status":"enabled","reason":"no-op idempotent"}`)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
    }
    var got domain.ScraperProvider
    db.First(&got, "name = ?", "gogoanime")
    if got.Status != domain.StatusEnabled {
        t.Errorf("DB status = %q, want enabled", got.Status)
    }
}

// TestUpdateStatus_ResponseBodyShape — 200 response wraps the updated provider
// in the standard {success:true, data:{provider:{...}}} envelope.
func TestUpdateStatus_ResponseBodyShape(t *testing.T) {
    db := openStatusTestDB(t)
    h := handler.NewInternalScraperProvidersHandler(db, nopLog())

    rec := fireStatusRequest(t, h, "gogoanime",
        `{"status":"degraded","reason":"shape test"}`)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
    }
    var env struct {
        Success bool `json:"success"`
        Data    struct {
            Provider domain.ScraperProvider `json:"provider"`
        } `json:"data"`
    }
    if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
        t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
    }
    if !env.Success {
        t.Errorf("success = false, want true")
    }
    if env.Data.Provider.Name != "gogoanime" {
        t.Errorf("provider.name = %q, want gogoanime", env.Data.Provider.Name)
    }
    if env.Data.Provider.Status != domain.StatusDegraded {
        t.Errorf("provider.status = %q, want degraded", env.Data.Provider.Status)
    }
}
```

2. **Run the tests** — they must FAIL because `UpdateStatus` does not exist yet:

```
cd /data/ae-camoufox/services/catalog && \
  go test ./internal/handler/ -run TestUpdateStatus -count=1 2>&1 | head -30
```

Expected output (compile error):
```
./internal/handler/internal_scraper_providers_test.go:XX: h.UpdateStatus undefined
FAIL github.com/ILITA-hub/animeenigma/services/catalog/internal/handler [build failed]
```

---

### Task P4.2 — Implement `UpdateStatus` on `InternalScraperProvidersHandler`

**Files:**
- `/data/ae-camoufox/services/catalog/internal/handler/internal_scraper_providers.go` (append after `List`)

**Interfaces — Consumes:**
- `domain.ScraperProvider`, `domain.StatusEnabled`, `domain.StatusDegraded`, `domain.StatusDisabled`
- `gorm.DB.WithContext().First()`, `.Updates()`  
- `httputil.Bind`, `httputil.OK`, `httputil.Error`, `httputil.BadRequest`, `httputil.NotFound`
- `errors.New(errors.CodeConflict, ...)`, `errors.NotFound(...)`, `errors.InvalidInput(...)`
- `libs/logger.Logger.Infow`
- `chi.URLParam`

**Interfaces — Produces:**
- `(h *InternalScraperProvidersHandler) UpdateStatus(w http.ResponseWriter, r *http.Request)` — callable by the router and the test suite

**Steps:**

1. **Add the implementation** (full file shown; existing `List` method and type retained verbatim — only the new block is appended):

```go
// services/catalog/internal/handler/internal_scraper_providers.go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// InternalScraperProvidersHandler serves the scraper provider config + capability
// traits to the scraper service over the Docker network.
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as /internal/cache/invalidate/raw/ and
// /internal/anime/{shikimoriId}/episodes (see internal_cache.go for the precedent).
// The gateway does NOT proxy /internal/*, so the route is reachable only from
// within the Docker network (spec 2026-06-15-scraper-capability-api).
type InternalScraperProvidersHandler struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewInternalScraperProvidersHandler constructs the handler.
func NewInternalScraperProvidersHandler(db *gorm.DB, log *logger.Logger) *InternalScraperProvidersHandler {
	return &InternalScraperProvidersHandler{db: db, log: log}
}

// List handles GET /internal/scraper/providers.
// Returns all domain.ScraperProvider rows ordered by name as
// {"providers":[...]} inside the standard {success,data:{...}} envelope.
func (h *InternalScraperProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Order("name asc").Find(&rows).Error; err != nil {
		h.log.Errorw("failed to load scraper providers", "error", err)
		httputil.Error(w, errors.Internal("failed to load scraper providers"))
		return
	}
	httputil.OK(w, map[string]any{"providers": rows})
}

// updateStatusRequest is the JSON body for UpdateStatus.
type updateStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// validWritableStatuses are the ONLY values the automated path may write.
// "disabled" is human-only (D5 / spec §Component 4).
var validWritableStatuses = map[domain.ProviderStatus]bool{
	domain.StatusEnabled:  true,
	domain.StatusDegraded: true,
}

// UpdateStatus handles POST /internal/scraper/providers/{name}/status.
//
// Constraints (spec §Component 4):
//   - Docker-network-only: the gateway never proxies /internal/*.
//   - Only enabled ↔ degraded are writable; "disabled" is rejected with 409.
//   - Changing an already-"disabled" provider is rejected with 409.
//   - Unknown provider → 404.
//   - Success → 200 with the updated provider row in the response envelope.
//   - Structured audit log: caller, provider, old→new status, reason.
func (h *InternalScraperProvidersHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		httputil.BadRequest(w, "name is required")
		return
	}

	var req updateStatusRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	// Validate target status: must be a non-empty, recognised writable value.
	target := domain.ProviderStatus(req.Status)
	if req.Status == "" || !validWritableStatuses[target] {
		httputil.Error(w, errors.InvalidInput(
			"status must be one of: enabled, degraded (disabled is human-only)"))
		return
	}

	// Load the current row.
	var row domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).
		First(&row, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.Error(w, errors.NotFound("provider "+name))
			return
		}
		h.log.Errorw("status-write: load provider failed",
			"provider", name, "error", err)
		httputil.Error(w, errors.Internal("failed to load provider"))
		return
	}

	// Gate: never touch a disabled provider (human-only terminal state).
	if row.Status == domain.StatusDisabled {
		httputil.Error(w, errors.New(errors.CodeConflict,
			"provider "+name+" is disabled; only a human may change it"))
		return
	}

	oldStatus := row.Status

	// Idempotent: GORM UPDATE is unconditional, but we keep the audit log
	// honest by noting when old == new.
	if err := h.db.WithContext(r.Context()).
		Model(&domain.ScraperProvider{}).
		Where("name = ?", name).
		Updates(map[string]any{
			"status": target,
			"reason": req.Reason,
		}).Error; err != nil {
		h.log.Errorw("status-write: update failed",
			"provider", name, "error", err)
		httputil.Error(w, errors.Internal("failed to update provider status"))
		return
	}

	// Structured audit log (caller = Docker-network peer IP; this endpoint
	// is never gateway-proxied so RemoteAddr is the real service/container).
	h.log.Infow("provider status-write",
		"caller", r.RemoteAddr,
		"provider", name,
		"old_status", string(oldStatus),
		"new_status", string(target),
		"reason", req.Reason,
	)

	// Re-read the updated row so the response reflects the persisted state.
	var updated domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).
		First(&updated, "name = ?", name).Error; err != nil {
		h.log.Errorw("status-write: re-read after update failed",
			"provider", name, "error", err)
		httputil.Error(w, errors.Internal("update applied but re-read failed"))
		return
	}

	httputil.OK(w, map[string]any{"provider": updated})
}
```

2. **Verify the implementation compiles**:

```
cd /data/ae-camoufox/services/catalog && go build ./... 2>&1
```

Expected: no output (clean build).

3. **Run the failing tests** — they must now PASS:

```
cd /data/ae-camoufox/services/catalog && \
  go test ./internal/handler/ -run TestUpdateStatus -v -count=1 2>&1
```

Expected output (all green):
```
=== RUN   TestUpdateStatus_HappyPath_EnabledToDegraded
--- PASS: TestUpdateStatus_HappyPath_EnabledToDegraded (0.00s)
=== RUN   TestUpdateStatus_HappyPath_DegradedToEnabled
--- PASS: TestUpdateStatus_HappyPath_DegradedToEnabled (0.00s)
=== RUN   TestUpdateStatus_RefusesSetDisabled
--- PASS: TestUpdateStatus_RefusesSetDisabled (0.00s)
=== RUN   TestUpdateStatus_RefusesChangeAlreadyDisabled
--- PASS: TestUpdateStatus_RefusesChangeAlreadyDisabled (0.00s)
=== RUN   TestUpdateStatus_UnknownProvider_404
--- PASS: TestUpdateStatus_UnknownProvider_404 (0.00s)
=== RUN   TestUpdateStatus_BadBody_400
--- PASS: TestUpdateStatus_BadBody_400 (0.00s)
=== RUN   TestUpdateStatus_Idempotent_SameStatus
--- PASS: TestUpdateStatus_Idempotent_SameStatus (0.00s)
=== RUN   TestUpdateStatus_ResponseBodyShape
--- PASS: TestUpdateStatus_ResponseBodyShape (0.00s)
PASS
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/handler
```

---

### Task P4.3 — Register the route in the internal block of the catalog router

**Files:**
- `/data/ae-camoufox/services/catalog/internal/transport/router.go` `:90-94` (the existing `if internalScraperProvidersHandler != nil { r.Get(…) }` block)

**Interfaces — Consumes:**
- `handler.InternalScraperProvidersHandler.UpdateStatus`
- `chi.Router.Post`

**Interfaces — Produces:**
- `POST /internal/scraper/providers/{name}/status` registered on the root-level chi router (outside `/api`, no middleware, Docker-network-only, same security model as all other `/internal/*` routes)

**Steps:**

1. **Edit the router** — extend the existing `internalScraperProvidersHandler` block to also register the new POST route:

Change (`:92-94` in the existing file):
```go
	// Scraper provider config + capability traits (spec 2026-06-15).
	// Same gateway-non-routing security model as the internal endpoints above.
	if internalScraperProvidersHandler != nil {
		r.Get("/internal/scraper/providers", internalScraperProvidersHandler.List)
	}
```

To:
```go
	// Scraper provider config + capability traits (spec 2026-06-15).
	// GET  — read-only roster for the scraper microservice.
	// POST {name}/status — automated enabled↔degraded lifecycle write
	//   (spec 2026-06-22 §Component 4). Docker-network-only: the gateway
	//   never proxies /internal/* paths (verified in transport_test.go).
	//   disabled is a human-only terminal state; the handler refuses it.
	if internalScraperProvidersHandler != nil {
		r.Get("/internal/scraper/providers", internalScraperProvidersHandler.List)
		r.Post("/internal/scraper/providers/{name}/status", internalScraperProvidersHandler.UpdateStatus)
	}
```

2. **Verify the router still builds**:

```
cd /data/ae-camoufox/services/catalog && go build ./... 2>&1
```

Expected: clean.

---

### Task P4.4 — Transport-level test: route is registered AND not on gateway router

**Files:**
- `/data/ae-camoufox/services/catalog/internal/transport/router.go` (read-only; already edited in P4.3)
- `/data/ae-camoufox/services/catalog/internal/transport/scraper_routes_test.go` — the existing file uses `package transport`; append a new test function

**Interfaces — Consumes:**
- `handler.InternalScraperProvidersHandler`, `handler.NewInternalScraperProvidersHandler`
- `chi.NewRouter`, `chi.Router.Get`, `chi.Router.Post`
- `gorm.DB` (SQLite in-memory, `gorm.Open(sqlite.Open(":memory:"), ...)`)
- `domain.ScraperProvider`, `domain.StatusEnabled`

**Interfaces — Produces:**
- `TestInternalRouter_StatusWriteRoute_RegisteredNotOnGateway` — proves the route resolves (no 404 from the catalog's own router) AND that the gateway's `/api/*`-scoped `ProxyToCatalog` registration does not cover any `/internal/*` path

**Steps:**

1. **Append to** `/data/ae-camoufox/services/catalog/internal/transport/scraper_routes_test.go` (inside `package transport`):

```go
// Additional imports needed by the new test (add to the existing import block):
//   "bytes"
//   "gorm.io/driver/sqlite"
//   "gorm.io/gorm"
//   "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
//   "github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"

// TestInternalRouter_StatusWriteRoute_RegisteredNotOnGateway verifies two
// invariants in one test to keep the fixture cost low:
//
//  1. The catalog's own /internal/scraper/providers/{name}/status route
//     resolves to a non-404 (i.e. the route IS registered).
//  2. The gateway's /api/*-scoped handlers do NOT cover /internal/* paths
//     (the gateway has no r.HandleFunc("/internal/…") registration).
//
// For (1) we build a minimal chi router that mirrors the production block in
// NewRouter and fire a POST with a valid body. The DB has one enabled row so
// the handler reaches the UPDATE path and returns 200.
// For (2) we inspect the gateway router source — the gateway registers ONLY
// /api/*, /admin/*, /og/*, /ws, /health, /api/status paths (verified by
// reading router.go). We confirm this structurally by checking that a
// /internal/* request against the gateway-shaped router returns 404.
func TestInternalRouter_StatusWriteRoute_RegisteredNotOnGateway(t *testing.T) {
	// --- Part 1: catalog internal router registers the route ----------------
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name: "gogoanime", Status: domain.StatusEnabled,
		Group: "en", SubDelivery: "hard",
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalScraperProvidersHandler(db, logger.Default())

	// Mirror the production registration block (NewRouter lines 90-94 after
	// Task P4.3 edit): GET roster + POST {name}/status on the root router.
	internalRouter := chi.NewRouter()
	internalRouter.Get("/internal/scraper/providers", h.List)
	internalRouter.Post("/internal/scraper/providers/{name}/status", h.UpdateStatus)

	body := bytes.NewBufferString(`{"status":"degraded","reason":"test"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/internal/scraper/providers/gogoanime/status", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	internalRouter.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("Part 1: POST /internal/scraper/providers/{name}/status returned 404 — route not registered")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Part 1: status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// --- Part 2: gateway-shaped /api router does NOT cover /internal/* ------
	// The gateway only registers paths under /api, /admin, /og, /ws, /health,
	// /api/status. A minimal gateway-shaped router exercises this structurally.
	gatewayRouter := chi.NewRouter()
	gatewayRouter.Route("/api", func(r chi.Router) {
		r.HandleFunc("/anime", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r.HandleFunc("/anime/*", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	probeReq := httptest.NewRequest(http.MethodPost,
		"/internal/scraper/providers/gogoanime/status",
		bytes.NewBufferString(`{"status":"degraded","reason":"test"}`))
	probeRec := httptest.NewRecorder()
	gatewayRouter.ServeHTTP(probeRec, probeReq)

	// A request to /internal/* must NOT match any gateway route → 404.
	if probeRec.Code != http.StatusNotFound {
		t.Errorf("Part 2: gateway router matched /internal/* — status = %d, want 404; "+
			"this means /internal/scraper/providers/{name}/status is gateway-exposed, "+
			"violating the Docker-network-only constraint", probeRec.Code)
	}
}
```

2. **Run the transport test**:

```
cd /data/ae-camoufox/services/catalog && \
  go test ./internal/transport/ -run TestInternalRouter_StatusWriteRoute -v -count=1 2>&1
```

Expected:
```
=== RUN   TestInternalRouter_StatusWriteRoute_RegisteredNotOnGateway
--- PASS: TestInternalRouter_StatusWriteRoute_RegisteredNotOnGateway (0.00s)
PASS
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/transport
```

---

### Task P4.5 — Full test suite green + build check

**Files:** none new

**Steps:**

1. **Run the full handler and transport test suites** (skip the Redis-dependent cache tests that need a live Redis by targeting the packages we changed):

```
cd /data/ae-camoufox/services/catalog && \
  go test \
    ./internal/handler/ \
    ./internal/transport/ \
    -count=1 -race 2>&1 | tail -20
```

Expected (all packages pass, no races):
```
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/handler    0.XXs
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/transport  0.XXs
```

2. **Run vet**:

```
cd /data/ae-camoufox/services/catalog && go vet ./... 2>&1
```

Expected: no output.

3. **Commit** (from a clean worktree off `origin/main`, per the golden rule):

```
git add \
  services/catalog/internal/handler/internal_scraper_providers.go \
  services/catalog/internal/handler/internal_scraper_providers_test.go \
  services/catalog/internal/transport/router.go \
  services/catalog/internal/transport/scraper_routes_test.go

git commit -m "$(cat <<'EOF'
feat(catalog): POST /internal/scraper/providers/{name}/status endpoint

Adds a Docker-network-only status-write route (Phase 4 of the Camoufox
pool self-heal spec) so the analytics probe and Go circuit breaker can
flip a provider between enabled↔degraded without a restart or SQL edit.
disabled remains a human-only terminal state — the handler returns 409
on any attempt to set it or change an already-disabled provider.
Structured audit log (caller/provider/old→new/reason) on every write.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Phase 5: Analytics Probe Writeback / Auto-Resurrection

After each probe run the analytics service reads the per-provider Rollup verdicts and, when `PROBE_AUTOGATE_ENABLED=true`, writes status transitions back to the catalog (enabled→degraded on DOWN; degraded→enabled on two consecutive UP) without ever touching `disabled` providers. The catalog endpoint (`POST /internal/scraper/providers/{name}/status`) was implemented in Phase 4 — this phase adds the analytics-side client, the evaluator, and the metric.

---

### Task P5.1 — Add `ProbeAutogateTransitions` metric to `libs/metrics/probe.go`

**Files:** `/data/ae-camoufox/libs/metrics/probe.go`

**Interfaces:**
- Produces: `metrics.ProbeAutogateTransitions` — `CounterVec{from, to, provider}`

**Steps:**

1. Read current file (already done above). Add the new counter after `ProbeProviderStatus`.

   *Run (expect build success / new symbol visible):*
   ```bash
   cd /data/ae-camoufox && go build ./libs/metrics/...
   ```
   Before the edit the symbol does not exist; any import attempt would fail compilation.

2. **Implement** — edit `libs/metrics/probe.go`, appending one `var` block entry:

   ```go
   // ProbeAutogateTransitions counts probe-driven enabled↔degraded transitions.
   // Labels: from (enabled|degraded), to (enabled|degraded), provider.
   // Never carries "disabled" — the autogate never touches disabled providers.
   ProbeAutogateTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
       Name: "probe_autogate_transitions_total",
       Help: "Probe-driven provider status transitions (enabled↔degraded) per provider.",
   }, []string{"from", "to", "provider"})
   ```

   Full updated file:

   ```go
   package metrics

   import "github.com/prometheus/client_golang/prometheus/promauto"
   import "github.com/prometheus/client_golang/prometheus"

   var (
       ProbeProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
           Name: "probe_provider_up",
           Help: "Per-provider playability verdict: 1 up, 0.5 degraded, 0 down.",
       }, []string{"provider"})

       ProbeRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
           Name: "probe_runs_total",
           Help: "Playability probe results per (provider, slot, server, result, reason).",
       }, []string{"provider", "slot", "server", "result", "reason"})

       ProbeLastRun = promauto.NewGauge(prometheus.GaugeOpts{
           Name: "probe_last_run_timestamp",
           Help: "Unix timestamp of the last completed probe run.",
       })

       // ProbeProviderStatus is an info-style gauge (value always 1) carrying the
       // per-provider rollup verdict as labels, so the playback dashboard table can
       // render Provider | Status | Reason directly. Reset() each run (the probe
       // reports the COMPLETE provider set each run) to avoid stale label series.
       ProbeProviderStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
           Name: "probe_provider_status",
           Help: "Per-provider playability rollup as labels (value always 1).",
       }, []string{"provider", "status", "reason"})

       // ProbeAutogateTransitions counts probe-driven enabled↔degraded transitions.
       // Labels: from (enabled|degraded), to (enabled|degraded), provider.
       // Never carries "disabled" — the autogate never touches disabled providers.
       ProbeAutogateTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
           Name: "probe_autogate_transitions_total",
           Help: "Probe-driven provider status transitions (enabled↔degraded) per provider.",
       }, []string{"from", "to", "provider"})
   )
   ```

3. *Run (expect PASS):*
   ```bash
   cd /data/ae-camoufox && go build ./libs/metrics/...
   ```

4. Commit:
   ```
   feat(metrics): add probe_autogate_transitions_total counter

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

### Task P5.2 — Catalog status-write client: `catalog_status_writer.go`

**Files:** `/data/ae-camoufox/services/analytics/internal/probe/catalog_status_writer.go` (new), `/data/ae-camoufox/services/analytics/internal/probe/catalog_status_writer_test.go` (new)

**Interfaces:**
- Consumes: `POST /internal/scraper/providers/{name}/status` (Phase 4 endpoint)
- Produces: `StatusWriter` interface used by `Autogate` (Task P5.3)

**Steps:**

1. Write the failing test first.

   *Run (expect compilation failure because `StatusWriter` does not exist yet):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestHTTPStatusWriter 2>&1 | head -5
   ```

2. **Write test file** `catalog_status_writer_test.go`:

   ```go
   package probe

   import (
       "context"
       "encoding/json"
       "net/http"
       "net/http/httptest"
       "testing"
   )

   func TestHTTPStatusWriter_HappyPath(t *testing.T) {
       var gotBody struct {
           Status string `json:"status"`
           Reason string `json:"reason"`
       }
       srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if r.Method != http.MethodPost {
               t.Errorf("method = %s, want POST", r.Method)
           }
           if r.URL.Path != "/internal/scraper/providers/gogoanime/status" {
               t.Errorf("path = %s", r.URL.Path)
           }
           _ = json.NewDecoder(r.Body).Decode(&gotBody)
           w.WriteHeader(http.StatusOK)
       }))
       defer srv.Close()

       w := NewHTTPStatusWriter(srv.URL, nil)
       if err := w.WriteStatus(context.Background(), "gogoanime", "degraded", "probe scored DOWN"); err != nil {
           t.Fatal(err)
       }
       if gotBody.Status != "degraded" {
           t.Errorf("body.status = %q, want degraded", gotBody.Status)
       }
       if gotBody.Reason != "probe scored DOWN" {
           t.Errorf("body.reason = %q", gotBody.Reason)
       }
   }

   func TestHTTPStatusWriter_409_ReturnsError(t *testing.T) {
       srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
           w.WriteHeader(http.StatusConflict)
       }))
       defer srv.Close()

       w := NewHTTPStatusWriter(srv.URL, nil)
       err := w.WriteStatus(context.Background(), "miruro", "disabled", "attempt to set disabled")
       if err == nil {
           t.Fatal("expected error on 409, got nil")
       }
   }

   func TestHTTPStatusWriter_404_ReturnsError(t *testing.T) {
       srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
           w.WriteHeader(http.StatusNotFound)
       }))
       defer srv.Close()

       w := NewHTTPStatusWriter(srv.URL, nil)
       err := w.WriteStatus(context.Background(), "unknown", "degraded", "probe")
       if err == nil {
           t.Fatal("expected error on 404, got nil")
       }
   }

   func TestHTTPStatusWriter_NetworkError_ReturnsError(t *testing.T) {
       // Point at a port that is not listening.
       w := NewHTTPStatusWriter("http://127.0.0.1:1", nil)
       err := w.WriteStatus(context.Background(), "gogoanime", "degraded", "probe")
       if err == nil {
           t.Fatal("expected network error, got nil")
       }
   }
   ```

   *Run (expect FAIL — undefined: NewHTTPStatusWriter):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestHTTPStatusWriter 2>&1 | head -10
   ```

3. **Implement** `catalog_status_writer.go`:

   ```go
   package probe

   import (
       "bytes"
       "context"
       "encoding/json"
       "fmt"
       "net/http"
       "strings"
       "time"
   )

   // StatusWriter writes a provider status transition to the catalog's
   // /internal/scraper/providers/{name}/status endpoint (Phase 4).
   // Implementations must be safe for concurrent use.
   type StatusWriter interface {
       // WriteStatus POSTs {status, reason} for the named provider.
       // Returns a non-nil error when the catalog refuses (409 forbidden
       // transition or 404 unknown provider) or is unreachable.
       WriteStatus(ctx context.Context, provider, status, reason string) error
   }

   // HTTPStatusWriter is the production StatusWriter calling the catalog over
   // the Docker network.
   type HTTPStatusWriter struct {
       base string
       hc   *http.Client
   }

   // NewHTTPStatusWriter constructs an HTTPStatusWriter. hc may be nil (a 10 s
   // timeout client is used). catalogBaseURL should be "http://catalog:8081".
   func NewHTTPStatusWriter(catalogBaseURL string, hc *http.Client) *HTTPStatusWriter {
       if hc == nil {
           hc = &http.Client{Timeout: 10 * time.Second}
       }
       return &HTTPStatusWriter{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
   }

   func (w *HTTPStatusWriter) WriteStatus(ctx context.Context, provider, status, reason string) error {
       body, err := json.Marshal(struct {
           Status string `json:"status"`
           Reason string `json:"reason"`
       }{Status: status, Reason: reason})
       if err != nil {
           return fmt.Errorf("catalog status-write marshal: %w", err)
       }
       url := w.base + "/internal/scraper/providers/" + provider + "/status"
       req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
       if err != nil {
           return fmt.Errorf("catalog status-write request build: %w", err)
       }
       req.Header.Set("Content-Type", "application/json")
       resp, err := w.hc.Do(req)
       if err != nil {
           return fmt.Errorf("catalog status-write %s: %w", provider, err)
       }
       defer resp.Body.Close()
       if resp.StatusCode != http.StatusOK {
           return fmt.Errorf("catalog status-write %s: HTTP %d", provider, resp.StatusCode)
       }
       return nil
   }

   // compile-time interface guard.
   var _ StatusWriter = (*HTTPStatusWriter)(nil)
   ```

   *Run (expect PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestHTTPStatusWriter -v 2>&1
   ```

4. Commit:
   ```
   feat(analytics/probe): catalog status-write client for provider enabled↔degraded

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

### Task P5.3 — Autogate evaluator: `autogate.go`

**Files:** `/data/ae-camoufox/services/analytics/internal/probe/autogate.go` (new), `/data/ae-camoufox/services/analytics/internal/probe/autogate_test.go` (new)

**Interfaces:**
- Consumes: `StatusWriter` (Task P5.2), `[]ProviderVerdict` from `Rollup`, `metrics.ProbeAutogateTransitions` (Task P5.1)
- Produces: `Autogate` struct with `Evaluate(ctx, verdicts []ProviderVerdict, currentStatuses map[string]string) error`

**Steps:**

1. Write the failing test first.

   *Run (expect compilation failure — Autogate undefined):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestAutogate 2>&1 | head -5
   ```

2. **Write test file** `autogate_test.go`:

   ```go
   package probe

   import (
       "context"
       "testing"
   )

   // fakeStatusWriter records WriteStatus calls; optionally returns an error.
   type fakeStatusWriter struct {
       calls []statusCall
       err   error
   }

   type statusCall struct {
       Provider string
       Status   string
       Reason   string
   }

   func (f *fakeStatusWriter) WriteStatus(_ context.Context, provider, status, reason string) error {
       f.calls = append(f.calls, statusCall{provider, status, reason})
       return f.err
   }

   // makeVerdicts builds a []ProviderVerdict from a map[name]Status.
   func makeVerdicts(m map[string]Status) []ProviderVerdict {
       out := make([]ProviderVerdict, 0, len(m))
       for name, st := range m {
           out = append(out, ProviderVerdict{Provider: name, Status: st})
       }
       return out
   }

   // currentStatuses provides the "current DB status" for each provider.
   func cs(m map[string]string) map[string]string { return m }

   func TestAutogate_DegradeOnDown(t *testing.T) {
       sw := &fakeStatusWriter{}
       ag := NewAutogate(sw, true)

       // enabled + scored DOWN → should write enabled→degraded on the first run.
       verdicts := makeVerdicts(map[string]Status{"gogoanime": StatusDown})
       if err := ag.Evaluate(context.Background(), verdicts, cs(map[string]string{"gogoanime": "enabled"})); err != nil {
           t.Fatal(err)
       }
       if len(sw.calls) != 1 {
           t.Fatalf("want 1 write call, got %d", len(sw.calls))
       }
       if sw.calls[0].Provider != "gogoanime" || sw.calls[0].Status != "degraded" {
           t.Fatalf("unexpected call: %+v", sw.calls[0])
       }
   }

   func TestAutogate_ReenableOnlyAfter2ConsecutiveUP(t *testing.T) {
       sw := &fakeStatusWriter{}
       ag := NewAutogate(sw, true)

       // First run: degraded + UP → consecutive_up becomes 1; no write yet.
       if err := ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"nineanime": StatusUp}),
           cs(map[string]string{"nineanime": "degraded"}),
       ); err != nil {
           t.Fatal(err)
       }
       if len(sw.calls) != 0 {
           t.Fatalf("after 1 UP: expected 0 writes, got %d", len(sw.calls))
       }

       // Second run: degraded + UP again → consecutive_up becomes 2 → write degraded→enabled.
       if err := ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"nineanime": StatusUp}),
           cs(map[string]string{"nineanime": "degraded"}),
       ); err != nil {
           t.Fatal(err)
       }
       if len(sw.calls) != 1 {
           t.Fatalf("after 2 UPs: expected 1 write, got %d; calls=%+v", len(sw.calls), sw.calls)
       }
       if sw.calls[0].Status != "enabled" {
           t.Fatalf("want enabled, got %s", sw.calls[0].Status)
       }
   }

   func TestAutogate_CounterResetOnNonUP(t *testing.T) {
       sw := &fakeStatusWriter{}
       ag := NewAutogate(sw, true)

       // Run 1: degraded + UP → count=1.
       _ = ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"miruro": StatusUp}),
           cs(map[string]string{"miruro": "degraded"}),
       )
       // Run 2: degraded + DOWN → count resets to 0 AND degrade write (already degraded → no write).
       _ = ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"miruro": StatusDown}),
           cs(map[string]string{"miruro": "degraded"}),
       )
       // Run 3: degraded + UP → count=1 again, no write.
       _ = ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"miruro": StatusUp}),
           cs(map[string]string{"miruro": "degraded"}),
       )
       // No write should have happened (only 1 consecutive UP, not 2).
       if len(sw.calls) != 0 {
           t.Fatalf("want 0 writes, got %d: %+v", len(sw.calls), sw.calls)
       }
   }

   func TestAutogate_NeverTouchesDisabled(t *testing.T) {
       sw := &fakeStatusWriter{}
       ag := NewAutogate(sw, true)

       // disabled + DOWN → no write.
       _ = ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"animepahe": StatusDown}),
           cs(map[string]string{"animepahe": "disabled"}),
       )
       // disabled + UP (2 runs) → still no write.
       for i := 0; i < 2; i++ {
           _ = ag.Evaluate(context.Background(),
               makeVerdicts(map[string]Status{"animepahe": StatusUp}),
               cs(map[string]string{"animepahe": "disabled"}),
           )
       }
       if len(sw.calls) != 0 {
           t.Fatalf("autogate must never touch disabled providers; got calls: %+v", sw.calls)
       }
   }

   func TestAutogate_FlagOff_NoOp(t *testing.T) {
       sw := &fakeStatusWriter{}
       ag := NewAutogate(sw, false) // disabled

       _ = ag.Evaluate(context.Background(),
           makeVerdicts(map[string]Status{"gogoanime": StatusDown}),
           cs(map[string]string{"gogoanime": "enabled"}),
       )
       if len(sw.calls) != 0 {
           t.Fatalf("flag-off: expected 0 writes, got %d", len(sw.calls))
       }
   }

   func TestAutogate_WriteError_DoesNotAbortOtherProviders(t *testing.T) {
       // The first write fails; the evaluator must still process the second provider.
       callCount := 0
       sw := &countingWriter{failOn: "gogoanime"}
       ag := NewAutogate(sw, true)

       verdicts := makeVerdicts(map[string]Status{
           "gogoanime": StatusDown,
           "miruro":    StatusDown,
       })
       statuses := cs(map[string]string{"gogoanime": "enabled", "miruro": "enabled"})
       if err := ag.Evaluate(context.Background(), verdicts, statuses); err != nil {
           t.Fatal(err)
       }
       _ = callCount
       // Both providers should have been attempted despite the first error.
       if sw.total < 2 {
           t.Fatalf("expected ≥2 write attempts, got %d", sw.total)
       }
   }

   type countingWriter struct {
       failOn string
       total  int
   }

   func (c *countingWriter) WriteStatus(_ context.Context, provider, _, _ string) error {
       c.total++
       if provider == c.failOn {
           return fmt.Errorf("injected error for %s", provider)
       }
       return nil
   }
   ```

   *Run (expect compilation failure — Autogate/NewAutogate/fmt undefined):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestAutogate 2>&1 | head -10
   ```

3. **Implement** `autogate.go`:

   ```go
   package probe

   import (
       "context"
       "fmt"

       "github.com/ILITA-hub/animeenigma/libs/logger"
       "github.com/ILITA-hub/animeenigma/libs/metrics"
   )

   // consecutiveUpThreshold is the number of consecutive UP runs required to
   // re-enable a degraded provider (anti-thrash guard; spec Phase 5).
   const consecutiveUpThreshold = 2

   // Autogate evaluates per-provider Rollup verdicts after a probe run and
   // writes status transitions to the catalog (enabled→degraded on DOWN;
   // degraded→enabled after consecutiveUpThreshold consecutive UP runs).
   //
   // Rules (from spec):
   //   - provider currently "enabled" + scored DOWN (0 %) → write enabled→degraded.
   //   - provider currently "degraded" + scored UP (>50 %) for N=2 consecutive runs
   //     → write degraded→enabled.
   //   - provider "disabled" → never touched.
   //   - A write error is logged and counted but never surfaces to the caller —
   //     probe-run failure must not propagate from a catalog write error.
   //
   // Autogate is NOT safe for concurrent use. The caller (Engine.RunOnce via
   // AutogatingEngine) is serial.
   type Autogate struct {
       sw      StatusWriter
       enabled bool
       log     *logger.Logger
       // consecutiveUP tracks how many consecutive UP runs each provider has
       // accumulated while in "degraded" state. Reset to 0 on any non-UP run or
       // when a transition fires. No DB backing — resets on restart, which is
       // acceptable: the anti-thrash gate merely delays re-enable by one probe
       // cycle after a restart.
       consecutiveUP map[string]int
   }

   // NewAutogate constructs an Autogate. When enabled is false, Evaluate is a
   // no-op (PROBE_AUTOGATE_ENABLED=false dark-ship).
   func NewAutogate(sw StatusWriter, enabled bool) *Autogate {
       return &Autogate{sw: sw, enabled: enabled, consecutiveUP: map[string]int{}}
   }

   // WithLogger attaches a logger; returns the receiver for chaining.
   func (a *Autogate) WithLogger(log *logger.Logger) *Autogate {
       a.log = log
       return a
   }

   // Evaluate processes one probe run's provider verdicts.
   //
   // currentStatuses maps provider name → its current DB status string
   // ("enabled"|"degraded"|"disabled"). Providers absent from the map are treated
   // as unknown and skipped (non-scraper-operated entries: ae, kodik-noads).
   //
   // Returns nil even when individual writes fail (fail-open; spec §Error handling).
   func (a *Autogate) Evaluate(ctx context.Context, verdicts []ProviderVerdict, currentStatuses map[string]string) error {
       if !a.enabled {
           return nil
       }
       for _, pv := range verdicts {
           current, ok := currentStatuses[pv.Provider]
           if !ok {
               // Provider not in the status map — not scraper-operated; skip.
               continue
           }
           if current == "disabled" {
               // Human-only terminal state; never touch.
               a.consecutiveUP[pv.Provider] = 0
               continue
           }

           switch {
           case current == "enabled" && pv.Status == StatusDown:
               // Degrade-on-DOWN: write enabled → degraded.
               if err := a.sw.WriteStatus(ctx, pv.Provider, "degraded", "probe scored DOWN (0% slots playable)"); err != nil {
                   a.logf("autogate write failed", pv.Provider, "enabled", "degraded", err)
               } else {
                   metrics.ProbeAutogateTransitions.WithLabelValues("enabled", "degraded", pv.Provider).Inc()
               }
               a.consecutiveUP[pv.Provider] = 0

           case current == "degraded" && pv.Status == StatusUp:
               // Re-enable-after-N-UP: accumulate consecutive UP count.
               a.consecutiveUP[pv.Provider]++
               if a.consecutiveUP[pv.Provider] >= consecutiveUpThreshold {
                   if err := a.sw.WriteStatus(ctx, pv.Provider, "enabled", fmt.Sprintf("probe scored UP for %d consecutive runs", consecutiveUpThreshold)); err != nil {
                       a.logf("autogate write failed", pv.Provider, "degraded", "enabled", err)
                   } else {
                       metrics.ProbeAutogateTransitions.WithLabelValues("degraded", "enabled", pv.Provider).Inc()
                       // Reset counter after a successful transition.
                       a.consecutiveUP[pv.Provider] = 0
                   }
               }

           default:
               // No transition: reset the consecutive-UP counter (e.g. degraded+DOWN,
               // degraded+Degraded, enabled+UP, enabled+Degraded).
               a.consecutiveUP[pv.Provider] = 0
           }
       }
       return nil
   }

   func (a *Autogate) logf(msg, provider, from, to string, err error) {
       if a.log != nil {
           a.log.Warnw(msg, "provider", provider, "from", from, "to", to, "error", err)
       }
   }
   ```

   *Run (expect PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/ -run TestAutogate -v 2>&1
   ```

4. Commit:
   ```
   feat(analytics/probe): Autogate evaluator — degrade-on-DOWN, re-enable-after-2-UP

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

### Task P5.4 — Config: `PROBE_AUTOGATE_ENABLED` env

**Files:** `/data/ae-camoufox/services/analytics/internal/config/config.go`

**Interfaces:**
- Produces: `Config.ProbeAutogateEnabled bool`

**Steps:**

1. *Run (expect missing field):*
   ```bash
   grep "ProbeAutogateEnabled" /data/ae-camoufox/services/analytics/internal/config/config.go; echo "exit: $?"
   ```

2. **Implement** — add the field to `Config` and load it in `Load()`:

   In the `Config` struct (after `ProbeProviders string`), add:
   ```go
   // ProbeAutogateEnabled gates the probe-driven enabled↔degraded writeback.
   // Set PROBE_AUTOGATE_ENABLED=false to dark-ship. Default on.
   ProbeAutogateEnabled bool
   ```

   In the `Load()` return literal (after `ProbeProviders: getEnv(...)`), add:
   ```go
   ProbeAutogateEnabled: getEnvBool("PROBE_AUTOGATE_ENABLED", true),
   ```

   Add the helper below `getEnvDuration`:
   ```go
   func getEnvBool(k string, d bool) bool {
       v := os.Getenv(k)
       if v == "" {
           return d
       }
       return v == "1" || v == "true" || v == "yes"
   }
   ```

3. *Run (expect build PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go build ./internal/config/...
   ```

4. Commit (can be batched with Task P5.5's main.go commit):
   ```
   feat(analytics/config): PROBE_AUTOGATE_ENABLED env (default true)

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

### Task P5.5 — Wire Autogate into the engine via `AutogatingEngine` in `main.go`

**Files:** `/data/ae-camoufox/services/analytics/cmd/analytics-api/main.go`

**Interfaces:**
- Consumes: `probe.NewAutogate`, `probe.NewHTTPStatusWriter`, `config.ProbeAutogateEnabled`, `probe.Engine.RunOnce`
- Produces: `AutogatingEngine` (local wrapper) exposing the same `RunOnce(ctx) error` signature consumed by `ProbeHandler`

The `handler.ProbeHandler` takes an interface; verify:

```bash
grep -n "ProbeRunner\|RunOnce\|interface" /data/ae-camoufox/services/analytics/internal/handler/probe.go 2>/dev/null | head -10
```

The handler already uses a `RunOnce(ctx) error` interface (the `*probe.Engine` satisfies it). The cleanest approach is a local `autogatingEngine` adapter struct defined inline in `main.go`'s engine-wiring block — no new file needed, matching the existing `countingSink`/`repoEraser` local-adapter pattern in `adapters.go`.

**Steps:**

1. **No test required for the wiring itself** — the adapter is two lines of delegation; behavior is fully covered by Task P5.3. Build test suffices.

2. **Implement** — inside the `if chConn != nil {` block in `main.go`, after `engine := probe.NewEngine(...)`, insert:

   ```go
   // autogatingEngine wraps the probe engine and, on each run completion,
   // evaluates Rollup verdicts for status transitions (enabled↔degraded)
   // via the catalog's internal status-write endpoint.
   type autogatingEngine struct {
       inner   *probe.Engine
       autogate *probe.Autogate
       sw       probe.StatusWriter
       catalogBase string
       log      *logger.Logger
   }

   func (ae *autogatingEngine) RunOnce(ctx context.Context) error {
       if err := ae.inner.RunOnce(ctx); err != nil {
           return err
       }
       // Fetch current provider statuses from catalog so Autogate can
       // compare against the live DB state (avoids stale in-memory assumptions).
       statuses, fetchErr := ae.fetchStatuses(ctx)
       if fetchErr != nil {
           // Fail-open: log the error but do not fail the probe run.
           if ae.log != nil {
               ae.log.Warnw("autogate: failed to fetch provider statuses from catalog", "error", fetchErr)
           }
           return nil
       }
       // RunResult is captured by PromReporter via the Reporter chain; the engine
       // does not expose it directly. Instead we re-derive the per-provider
       // verdicts from the latest ProviderVerdicts in the PromReporter — which we
       // cannot access from here. The idiomatic solution is to carry the verdicts
       // through a capturing Reporter decorator. See implementation note below.
       return nil
   }
   ```

   > **Implementation note:** the `Engine.RunOnce` consumes the `Reporter` directly and does not return `RunResult`. The right hook-point is a **Reporter decorator** that captures the verdicts and invokes `Autogate.Evaluate` after the base reporter completes. This avoids mutating the engine signature and is the minimal-blast-radius pattern, matching how the existing `PromReporter` is already a strategy.

   **Revised approach** — implement a `CapturingReporter` that wraps the `PromReporter` and calls `Autogate.Evaluate`:

   ```go
   // In main.go, inside the `if chConn != nil {` block, replace:
   //   probe.NewPromReporter(chStore)
   // with:
   //   newAutogatingReporter(probe.NewPromReporter(chStore), ag, cfg, log)
   //
   // newAutogatingReporter is defined immediately below.
   ```

   Full wiring code to insert **after** `ag := probe.NewAutogate(...)` and **before** the `probeHandler =` line:

   ```go
   ag := probe.NewAutogate(
       probe.NewHTTPStatusWriter(cfg.CatalogURL, nil),
       cfg.ProbeAutogateEnabled,
   ).WithLogger(log)

   engine := probe.NewEngine(
       targets,
       validator,
       newAutogatingReporter(probe.NewPromReporter(chStore), ag, cfg.CatalogURL, log),
       pool, rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
       func() int64 { return time.Now().Unix() },
       log,
   )
   probeHandler = handler.NewProbeHandler(engine)
   ```

   Define `newAutogatingReporter` as a local type in `main.go` (following the `countingSink` / `repoEraser` adapter pattern already in `adapters.go`; but since this needs multiple symbols it's cleaner as a named local type):

   ```go
   // autogatingReporter wraps a base Reporter and, after each run, fetches the
   // current catalog provider statuses and passes the Rollup verdicts to Autogate.
   // Defined at package main to stay co-located with the wiring.
   type autogatingReporter struct {
       base        probe.Reporter
       ag          *probe.Autogate
       catalogBase string
       hc          *http.Client
       log         *logger.Logger
   }

   func newAutogatingReporter(base probe.Reporter, ag *probe.Autogate, catalogBase string, log *logger.Logger) *autogatingReporter {
       return &autogatingReporter{base: base, ag: ag, catalogBase: catalogBase, hc: &http.Client{Timeout: 10 * time.Second}, log: log}
   }

   func (r *autogatingReporter) Report(ctx context.Context, run probe.RunResult) error {
       // Always run the base reporter first (Prometheus + ClickHouse).
       if err := r.base.Report(ctx, run); err != nil {
           return err
       }
       // Fetch current scraper-provider statuses from the catalog.
       statuses, err := r.fetchProviderStatuses(ctx)
       if err != nil {
           if r.log != nil {
               r.log.Warnw("autogate: could not fetch provider statuses", "error", err)
           }
           // Fail-open: autogate is best-effort; never block the report.
           return nil
       }
       // Evaluate verdicts. Errors are already logged + swallowed inside Evaluate.
       _ = r.ag.Evaluate(ctx, run.ProviderVerdicts, statuses)
       return nil
   }

   // fetchProviderStatuses GETs /internal/scraper/providers and returns a
   // map[name]status for every provider in the catalog roster.
   func (r *autogatingReporter) fetchProviderStatuses(ctx context.Context) (map[string]string, error) {
       req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.catalogBase+"/internal/scraper/providers", nil)
       if err != nil {
           return nil, err
       }
       resp, err := r.hc.Do(req)
       if err != nil {
           return nil, err
       }
       defer resp.Body.Close()
       if resp.StatusCode != http.StatusOK {
           return nil, fmt.Errorf("GET /internal/scraper/providers -> %d", resp.StatusCode)
       }
       var env struct {
           Data struct {
               Providers []struct {
                   Name   string `json:"name"`
                   Status string `json:"status"`
               } `json:"providers"`
           } `json:"data"`
       }
       if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
           return nil, err
       }
       m := make(map[string]string, len(env.Data.Providers))
       for _, p := range env.Data.Providers {
           m[p.Name] = p.Status
       }
       return m, nil
   }
   ```

   Add `"encoding/json"`, `"fmt"`, and `"net/http"` to the `main.go` import block if not already present (they already appear via other uses in the file).

3. *Run (expect build PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go build ./... 2>&1
   ```

4. *Run all probe tests (expect PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./internal/probe/... -count=1 -race 2>&1
   ```

5. Commit:
   ```
   feat(analytics): wire Autogate into probe engine via autogatingReporter

   After each probe run the autogatingReporter fetches current catalog provider
   statuses and calls Autogate.Evaluate so enabled→degraded (DOWN) and
   degraded→enabled (2-consecutive-UP) transitions are written back automatically.
   Gated by PROBE_AUTOGATE_ENABLED (default true). Fail-open: a catalog fetch
   or write error is logged and never fails the probe run itself.

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

### Task P5.6 — Integration test for `autogatingReporter` with a fake catalog server

**Files:** `/data/ae-camoufox/services/analytics/internal/probe/autogate_test.go` — extend with an additional test for the reporter-level integration; OR add a new test in `services/analytics/cmd/analytics-api/main_integration_test.go` if the wiring is in `package main`.

Because `autogatingReporter` is defined in `package main` (cmd/analytics-api), its test must live in the same package. The pattern for testing `package main` helpers is `package main` test files in the cmd directory.

**Steps:**

1. **Write test file** `/data/ae-camoufox/services/analytics/cmd/analytics-api/autogate_reporter_test.go`:

   ```go
   package main

   import (
       "context"
       "encoding/json"
       "net/http"
       "net/http/httptest"
       "testing"

       "github.com/ILITA-hub/animeenigma/services/analytics/internal/probe"
   )

   // fakeBaseReporter records the RunResult passed to it.
   type fakeBaseReporter struct{ got probe.RunResult }

   func (f *fakeBaseReporter) Report(_ context.Context, run probe.RunResult) error {
       f.got = run
       return nil
   }

   // fakeStatusWriterMain mirrors fakeStatusWriter in probe package but lives here.
   type fakeStatusWriterMain struct{ calls []string }

   func (f *fakeStatusWriterMain) WriteStatus(_ context.Context, provider, status, _ string) error {
       f.calls = append(f.calls, provider+":"+status)
       return nil
   }

   func TestAutogatingReporter_FullPath(t *testing.T) {
       // Catalog stub returns gogoanime=enabled, miruro=degraded.
       catalogSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if r.URL.Path != "/internal/scraper/providers" {
               w.WriteHeader(http.StatusNotFound)
               return
           }
           _ = json.NewEncoder(w).Encode(map[string]any{
               "data": map[string]any{
                   "providers": []map[string]any{
                       {"name": "gogoanime", "status": "enabled"},
                       {"name": "miruro", "status": "degraded"},
                   },
               },
           })
       }))
       defer catalogSrv.Close()

       sw := &fakeStatusWriterMain{}
       ag := probe.NewAutogate(sw, true)
       base := &fakeBaseReporter{}

       r := newAutogatingReporter(base, ag, catalogSrv.URL, nil)

       // Run 1: gogoanime DOWN (enabled) → should degrade.
       //        miruro UP (degraded)  → consecutive_up = 1, no write.
       run1 := probe.RunResult{
           ProviderVerdicts: []probe.ProviderVerdict{
               {Provider: "gogoanime", Status: probe.StatusDown},
               {Provider: "miruro", Status: probe.StatusUp},
           },
       }
       if err := r.Report(context.Background(), run1); err != nil {
           t.Fatal(err)
       }
       if len(sw.calls) != 1 || sw.calls[0] != "gogoanime:degraded" {
           t.Fatalf("after run1: calls = %v, want [gogoanime:degraded]", sw.calls)
       }

       // Run 2: miruro UP again (degraded) → consecutive_up = 2 → re-enable.
       run2 := probe.RunResult{
           ProviderVerdicts: []probe.ProviderVerdict{
               {Provider: "miruro", Status: probe.StatusUp},
           },
       }
       // Reset catalog stub to return miruro=degraded (still degraded after run1).
       if err := r.Report(context.Background(), run2); err != nil {
           t.Fatal(err)
       }
       if len(sw.calls) != 2 || sw.calls[1] != "miruro:enabled" {
           t.Fatalf("after run2: calls = %v, want [gogoanime:degraded miruro:enabled]", sw.calls)
       }
   }

   func TestAutogatingReporter_CatalogDown_FailOpen(t *testing.T) {
       // Catalog not reachable — reporter must not error.
       sw := &fakeStatusWriterMain{}
       ag := probe.NewAutogate(sw, true)
       base := &fakeBaseReporter{}

       r := newAutogatingReporter(base, ag, "http://127.0.0.1:1", nil)
       run := probe.RunResult{
           ProviderVerdicts: []probe.ProviderVerdict{{Provider: "gogoanime", Status: probe.StatusDown}},
       }
       if err := r.Report(context.Background(), run); err != nil {
           t.Fatalf("catalog-down must be fail-open, got error: %v", err)
       }
       if len(sw.calls) != 0 {
           t.Fatalf("catalog-down: expected 0 writes, got %v", sw.calls)
       }
   }
   ```

   *Run (expect PASS):*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./cmd/analytics-api/ -run TestAutogating -v 2>&1
   ```

2. *Run full analytics test suite:*
   ```bash
   cd /data/ae-camoufox/services/analytics && go test ./... -count=1 -race 2>&1
   ```

3. Commit:
   ```
   test(analytics): integration test for autogatingReporter — fail-open + full path

   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
   ```

---

## Execution order & deploy cadence

Implement and ship phase-by-phase; each phase is independently deployable and testable:

1. **Phase 1 — Sidecar self-heal.** `make redeploy-stealth-scraper`. Closes AUTO-527's collateral damage on its own.
2. **Phase 2 — RAM budget + per-user quota.** Redeploy `stealth-scraper` (+ thin `catalog`/`scraper` `user_key` plumbing); bump compose `mem_limit` after confirming host RAM.
3. **Phase 3 — Go scraper kind + breaker + runtime re-gate.** `make redeploy-scraper`.
4. **Phase 4 — Catalog status-write endpoint.** `make redeploy-catalog`.
5. **Phase 5 — Analytics probe writeback.** `make redeploy-analytics` (flag-gated).

After each phase: run that service's full test suite, deploy from the clean `feat/camoufox-pool-selfheal` worktree, and verify health. The whole branch merges to `main` via `superpowers:finishing-a-development-branch` once all phases are green, with `/animeenigma-after-update` for the changelog + final deploy.
