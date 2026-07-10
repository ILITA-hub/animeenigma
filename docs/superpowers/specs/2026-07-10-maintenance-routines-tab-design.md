# Maintenance Routines Tab — Design Spec

**Date:** 2026-07-10
**Branch:** `feat/maintenance-routines-tab`
**Status:** Design approved (owner), pre-plan
**Owner ask:** Add a tab to `https://animeenigma.org/admin/policy` to control maintenance
routines — starting with the **maintenance bot** and the **daily provider-recovery
operator** — plus a roster of other routines worth exposing.

---

## 1. Problem & context

`/admin/policy` (`frontend/web/src/views/admin/AdminPolicy.vue`) today has two tabs:

- **Features** — served by `policy-service` (:8098): runtime RBAC feature flags.
- **Providers** — a facade over `catalog`: flip scraper providers auto↔disabled.

The owner wants a **third tab** to see and pause the platform's background
**maintenance routines**. The two named routines — and most of the roster below —
run on the **host** as `systemd` units / cron jobs, **not** as Docker services:

| Routine | Where it runs |
|---------|---------------|
| Maintenance bot (`animeenigma-maintenance`) | host systemd daemon (host binary `/data/animeenigma/bin/maintenance`) |
| Daily provider-recovery operator | host systemd timer `animeenigma-provider-recovery.timer` (04:17 UTC) |
| Git autosync | host cron `animeenigma-git-sync.cron` (every 10 min ff-sync) |
| Disk / build-cache prune | host cron (daily / weekly) |
| Scheduler crons (subtitle-probe, shikimori-sync, playability-canary) | Docker `scheduler` (:8085), env-cron |
| Provider self-heal loop (24h demote/promote, 6h probe) | Docker `catalog` (:8081) |

**Core constraint:** a browser → gateway → Docker-network request **cannot
`systemctl` the host**. The design is therefore a **pull-config model**, not a
push/exec model.

## 2. Chosen approach — pull-config (owner-selected)

Admin *intent* (enabled + knobs) lives in a DB table the web page writes. Each
routine **reads that intent at the top of every run and self-skips when disabled**.
Status is written back by each routine after it runs. No inbound host access; no
new host daemon; no `systemctl` from the network.

This mirrors the pattern RBAC-and-roulette already established ("policy owns
admin-intent, services read it") and the `/internal/*` loopback convention that
`provider-recovery.sh` already uses to curl `localhost:8081/internal/...`.

**Rejected alternatives:**
- *Run-now / dial-home host agent* — more powerful (actual `systemctl`, on-demand
  triggers, live logs) but a much larger security + ops surface (a persistent host
  daemon with `systemctl` rights + command queue + auth). Over-scoped for v1.
- *Store in catalog* — catalog is also host-reachable, but this splits the
  admin-policy backend across two services and stretches catalog's scope.

## 3. Store & owner: `policy-service` (:8098)

policy-service is the service already behind `/admin/policy` **and** it is published
to the host (`docker-compose.yml`: `127.0.0.1:8098:8098`), so **every consumer —
host script or Docker service — reads it the same way**.

### 3.1 New table `maintenance_routines`

```go
type MaintenanceRoutine struct {
    ID          string         `gorm:"primaryKey"`          // stable slug, see §4
    Enabled     bool           `gorm:"not null;default:true"` // GORM omits false on default → set explicitly in seed
    Settings    string         `gorm:"type:text"`           // JSON blob of knob values (see §4)
    LastRunAt   *time.Time     `json:"last_run_at"`
    LastOK      *bool          `json:"last_ok"`
    LastSummary string         `json:"last_summary"`        // short human line, e.g. "adopted okru · exit 0"
    NextRunAt   *time.Time     `json:"next_run_at"`         // optional; for timers we can compute/echo
    UpdatedAt   time.Time
}
```

> **GORM bool gotcha (carried from RBAC-and-roulette):** `default:true` on a bool
> makes GORM **omit `false`** on insert. Seed rows must set `Enabled` explicitly and
> `SetRoutine` must write the column unconditionally (use `Select("enabled", ...)`
> or a full-struct `Save`).

### 3.2 Seed (day-one parity)

On boot, seed all routines with `Enabled=true` and `Settings` = **today's real
defaults** (current env/hardcoded values). First deploy = byte-identical behavior.
Idempotent: seed only inserts missing rows (never overwrites live admin edits) —
mirror `domain.SeedFlags`' first-boot-only insert.

### 3.3 Endpoints (all on policy-service)

| Purpose | Method + path | Auth | Consumer |
|---------|---------------|------|----------|
| List (with status) | `GET /api/admin/maintenance/routines` | admin (gateway) | web page |
| Update | `PUT /api/admin/maintenance/routines/{id}` `{enabled?, settings?}` | admin (gateway) | web page |
| Gate read | `GET /internal/maintenance/routines/{id}` → `{enabled, settings}` | none (loopback + not gateway-proxied) | host scripts (`localhost:8098`), Docker services (`http://policy:8098`) |
| Status write-back | `POST /internal/maintenance/routines/{id}/status` `{ok, summary, next_run_at?}` | none (internal) | each routine after it runs |

- `/api/admin/*` reaches policy-service via the existing gateway admin proxy (same
  route family as `/api/admin/scraper-providers`). Add the proxy path if not already
  covered by a catch-all; verify against `services/gateway` routing.
- `/internal/*` is **not** gateway-proxied (Docker-network + host-loopback only),
  matching catalog's `/internal/*`. No token — protected by not being exposed and by
  loopback binding.
- Unknown `{id}` on gate read ⇒ **404**; the caller treats any non-200 as
  fail-open (`enabled=true`, see §6.1), so a stale/renamed slug never silently
  pauses a routine.

## 4. Routine roster & knob descriptors

Knob values are stored as a JSON blob in `Settings`; the FE renders controls from a
typed **descriptor registry** (`frontend/web/src/config/maintenanceRoutines.ts`,
mirroring `config/policyFeatures.ts`). Descriptor control types: `switch` | `select`
| `number` | `chips`.

| ID (slug) | Type | Knobs (JSON keys) | Status line | Enforcement change |
|-----------|------|-------------------|-------------|--------------------|
| `maintenance_bot` | host daemon | `auto_apply_max_risk` (select: none/low/medium) — **new bot behavior**; `suppressed_alerts` (chips — moves `SUPPRESSED_ALERTS` env → config) | last fix + AUTO-id | daemon reads gate in its process loop; `enabled=false` = stays up but passive; `auto_apply_max_risk` caps `decideAutoApply` |
| `provider_recovery` | host timer 04:17 | `model` (select: sonnet/opus — moves existing env) | adopted provider · exit code | 1 gate-check (early-exit) + 1 status-ping in `animeenigma-provider-recovery.sh` |
| `git_autosync` | host cron */10 | — | last sync · **in-sync / DIVERGED** | gate-check + status-ping in `animeenigma-git-autosync.sh` |
| `disk_prune` | host cron daily | `high_water_pct` (number) | disk % · bytes freed | gate-check + ping in the prune script |
| `build_cache_prune` | host cron weekly | — | last prune · bytes freed | gate-check + ping |
| `subtitle_probe` | Docker `scheduler` | — (pause only) | up / degraded / down counts | scheduler reads gate before firing the job |
| `shikimori_sync` | Docker `scheduler` | — | last sync · N updated | scheduler gate |
| `playability_canary` | Docker `scheduler` | — | last run · pass/total | scheduler gate |
| `provider_self_heal` | Docker `catalog` | `demote_after` (select), `probe_every` (select) | last state change | catalog reads gate/knobs; reuses existing self-heal state |

> Routines whose status-ping isn't wired yet render status `—` (unknown). **The
> enable-gate is the MVP; status is purely additive**, so P3 wiring can land one
> routine at a time.

## 5. Frontend — the Maintenance tab

Added to the existing `Tabs` in `AdminPolicy.vue`; **reuses the Providers-tab card
shape** (this is pattern-reuse, not a novel surface).

- **Composable** `useAdminMaintenance` (mirrors `useAdminProviders`): `list()`,
  `setRoutine(id, {enabled?, settings?})`. `adminApi` gains
  `maintenanceRoutines()` / `setMaintenanceRoutine()`.
- **Per-routine card** (`Card`/`CardHeader`/`Badge`/`Switch`/`Select` primitives):
  - **Enable Switch** — instant apply, optimistic + revert-on-failure + toast
    (identical to `onToggleProvider`). **Disable gated behind `useConfirm`
    (destructive)** — pausing auto-recovery/prune has real consequences.
  - **Knobs** — render from the descriptor registry; **collected and committed on a
    per-card `Save` button** (owner-selected: instant toggle, explicit Save for
    knobs). Save = optimistic with revert + toast; dirty-tracking like the Features
    tab's `isDirty`.
  - **Status badge** — `ok` (success) / `failed` (destructive) / `—` (default,
    unknown) / **`stale`** (warning) when `last_run_at` is older than the routine's
    expected cadence. This makes the tab a live health board, not just switches.
  - **Grouping** — a divider between **Host routines** and **In-cluster routines**,
    since pause semantics differ (timer-fires-but-skips vs live-gate). Keeps the
    mental model honest.
- **DS compliance:** native `<select>` is banned (DS Rule 5) — use `Select`
  primitive (this surface is outside `components/player/`, no fullscreen exemption).
  No off-palette colors; bind status variants to semantic tokens.
- **i18n:** `admin.policy.maintenance.*` sub-namespace in **en / ru / ja**
  (parity-gated) — routine display names, knob labels, status strings, confirm copy.

## 6. Safety invariants

1. **Fail-open.** If a routine can't reach policy-service (outage, timeout), it
   **runs as normal**. A policy blip must never silently pause disk-prune or
   provider-recovery. The toggle is a best-effort *pause*, never a hard interlock.
   Gate read helper: unreachable/non-200/parse-fail ⇒ treat as `enabled=true`.
2. **Seed parity.** First boot = today's behavior exactly (all enabled, real
   default knobs).
3. **Confirm-gate** on every disable (destructive `useConfirm`).
4. **Pause ≠ disable.** Host timers still fire; the script early-exits. We never
   `systemctl disable` from the app. (The systemd units stay installed & owner-owned.)
5. **Host install is an owner step.** Edited host scripts land committed in
   `infra/host/` (source of truth); **installing to `/usr/local/bin` is owner-run**
   — the auto-mode classifier blocks the interactive AI from self-wiring host units
   (documented in the provider-recovery memory). P3 script diffs are repo changes;
   the owner deploys them.

## 7. Rollout (phased, each independently shippable)

1. **P1 — policy-service backend.** `maintenance_routines` table + idempotent seed +
   admin CRUD + internal gate/status endpoints + gateway proxy route (if needed).
   *Nothing reads the gate yet → zero behavior change.*
2. **P2 — FE tab.** Maintenance tab, `useAdminMaintenance`, descriptor registry,
   i18n (en/ru/ja), specs. Reads/writes the store. *No enforcement yet → toggles
   persist but are cosmetic until P3.*
3. **P3 — enforcement wiring (additive, one routine at a time).** Host `.sh`
   gate-checks + status-pings (`provider-recovery`, `git-autosync`, prune); the
   maintenance-bot daemon gate + `auto_apply_max_risk`; scheduler per-job gate;
   catalog self-heal gate. Each diff is small and isolated; an unwired routine simply
   ignores its toggle (fail-open).

## 8. Testing

- **policy-service (Go):** seed parity, CRUD, gate JSON shape, status stamping,
  GORM-false-bool guard. Handwritten fakes, **no testify/mock** (house style).
- **FE:** `AdminPolicy` maintenance-tab `.spec.ts` (≥5 Vitest assertions: renders
  rows, toggle calls composable + reverts on error, Save gathers knobs, status
  badge mapping, confirm-gate on disable). i18n parity spec for the new namespace.
  `frontend-verify` gate (DS-lint + i18n + real `bun run build`).
- **Host scripts:** each wired script keeps a `--check` dry-run that exercises the
  gate-read + status-ping paths without doing real work.
- **E2E:** extend `frontend/web/src/router/admin-policy-route.spec.ts` / admin e2e
  for the third tab.

## 9. Effort scoring (project convention — no time units)

- **UXΔ = +2 (Better)** — opaque host cron/systemd routines become a visible,
  pausable health board for the owner; no end-user surface.
- **CDI = 0.04 × 21** — Spread across policy-service + gateway + FE + 4 host scripts
  + 2 Docker services (wide); low per-site Shift (additive, seed-parity, fail-open);
  Effort_Fib 21.
- **MVQ = Griffin 88% / 82%** — methodical control-plane work fronting many small
  watchful routines; disciplined; guarded against slop by seed-parity + fail-open.

## 10. Open items / deferred

- **Run-now / schedule editing / live logs** — explicitly deferred (would need the
  dial-home host agent). Revisit only if the pause+status board proves insufficient.
- **Provider self-heal overlap** — its cadence knobs live here, but its per-provider
  state stays on the Providers tab; avoid duplicating provider listing.
- **`SUPPRESSED_ALERTS` / recovery `model` env → config migration** — during P3,
  the env stays as the fail-open default; config overrides it when reachable.
- Consider a `design-prototyping` pass before writing the Vue (per CLAUDE.md heavy-FE
  guidance), though the tab closely mirrors the existing Providers card — likely
  skippable; owner's call at plan time.
