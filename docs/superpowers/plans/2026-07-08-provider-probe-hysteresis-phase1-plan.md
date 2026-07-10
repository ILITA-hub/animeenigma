# Phase 1 Implementation Plan — Provider Health Hysteresis (UP→Degraded→Down)

Spec: `docs/superpowers/specs/2026-07-08-provider-probe-hysteresis-and-paced-scheduler-design.md` (§3).
Worktree: `/data/animeenigma/.claude/worktrees/probe-hysteresis` (branch `feat/provider-probe-hysteresis-pacing`).
Execution: subagent-driven, TDD per task. All paths below are relative to the worktree root.

Phase 2 (paced scheduler) is a separate follow-up plan — NOT in scope here.

---

## Task 1 — domain: `HealthDegraded` + derivation truth tables

**File:** `services/catalog/internal/domain/scraper_provider.go` (+ `scraper_provider_test.go`)

1. Add `HealthDegraded ProviderHealth = "degraded"` to the health consts (between Up and Recovering; comment: one failed probe, pending confirmation — transient, still failover-trusted).
2. `DerivedState()` — new derivation (branch order mirrored into Grafana in Task 5):
   - `policy == disabled` **or** `policy == manual` → `StateDisabled`
   - `auto + up` → `StateUP`
   - `health == recovering` → `StateRecovering`
   - `auto + degraded` → `StateDegraded`
   - default (`auto + down`) → `StateDown`
   Update the doc comments on the `State*` consts: `StateDegraded` = "transient: one failed probe, pending confirmation (still in auto-failover)"; `StateDisabled` = "admin lock: policy manual or disabled".
3. `StateCode()` — unchanged mapping (UP=4, Recovering=3, Degraded=2, Down=1, Disabled=0).
4. `WireStatus()` — `auto`: `up|degraded → enabled`, `recovering|down → degraded`; `manual → degraded` (unchanged — keeps hacker-selectability); `disabled → disabled`.
5. `Eligible()` — `policy == auto && (health == up || health == degraded)`.
6. `ProbeCadence()` — `degraded` → `c.Up` (re-probe next cycle; Phase-1 interim, Phase 2 replaces). Other branches unchanged.
7. `ProbeSample()` — `degraded` → `(c.FullSample, false)` (same as up: honest full sample, no abort).
8. **Tests (write first):** table-driven truth tables for `DerivedState`, `StateCode`, `WireStatus`, `Eligible` covering every `(policy, health)` combo incl. the new `degraded` health and `manual+up`/`manual+down` → Disabled band; `ProbeCadence`/`ProbeSample` rows for `degraded`.

## Task 2 — engine: hysteresis + delete `ApplyPolicy`

**File:** `services/catalog/internal/service/providerpolicy/engine.go` (+ `engine_test.go`)

1. `ApplyHealth` new transition table (`HealthSince` reset only on real change — unchanged mechanism):

   | current | pass | fail |
   |---|---|---|
   | up | up | **degraded** |
   | degraded | **up** | **down** |
   | recovering | up if `now-HealthSince ≥ promoteAfter` else recovering | down |
   | down | recovering | down |
   | unseeded | recovering | down |

2. **Delete `ApplyPolicy`** (both 24h auto→manual demotion and manual→auto promotion). Policy is admin-only now.
3. `ApplyVerdict(p, pass, now, promoteAfter)` — drop the `demoteAfter` param; body = `ApplyHealth` + stamp `LastProbedAt`.
4. **Tests (write first):** full matrix incl. the two headline behaviors — a lone fail from `up` lands `degraded` (NOT `down`); `degraded`+fail → `down`; `degraded`+pass → `up`; policy never mutated by any verdict sequence (auto stays auto through fail-fail-fail; manual stays manual through sustained passes).

## Task 3 — config: drop `DemoteAfter`

**File:** `services/catalog/internal/config/config.go`

Remove `DemoteAfter` field from `ProviderPolicyConfig`, its `PROVIDER_DEMOTE_AFTER` load line, and its doc-comment line. Keep `PromoteAfter` + `Cadence`. Grep for any other `DemoteAfter` reader.

## Task 4 — handler: updated `ApplyVerdict` call

**File:** `services/catalog/internal/handler/internal_provider_policy.go` (+ `internal_provider_policy_test.go`)

1. Call becomes `providerpolicy.ApplyVerdict(&p, req.Pass, now, h.cfg.PromoteAfter)`.
2. The persisted `updates` map still writes `policy`/`policy_since` (harmless no-ops now; keep — the TOCTOU disabled-guard depends on the same Updates call shape).
3. Handler doc-comment: drop the demote mention.
4. **Tests:** fix the cfg fixture (`DemoteAfter` field gone); add a handler-level case: provider `auto/up` + `pass=false` → response/DB show `health=degraded`, `policy=auto`.

## Task 5 — observability text + Grafana mirror

**Files:** `libs/metrics/provider.go`, `docker/grafana/dashboards/playback-health.json`

1. `provider_state` Help: `"... (4=UP, 3=Recovering, 2=Degraded(one failed probe, pending confirm), 1=Down, 0=Disabled(admin lock: manual/disabled)) ..."`.
2. Roster SQL `CASE` (panel 102, line ~676) — mirror the new `DerivedState` exactly:
   `CASE WHEN policy IN ('disabled','manual') THEN 'Disabled' WHEN policy='auto' AND health='up' THEN 'UP' WHEN health='recovering' THEN 'Recovering' WHEN policy='auto' AND health='degraded' THEN 'Degraded' ELSE 'Down' END`.
3. Panel descriptions (roster ~355, state-history ~801): update the 5-state legend wording — Degraded = "one failed probe, pending confirmation (still in auto-failover)"; Disabled = "admin lock (manual or disabled)".
4. Validate: `python3 -m json.tool < playback-health.json > /dev/null`.
5. NOTE: `libs/metrics` is a go.work lib — text-only change here, no API change, no Dockerfile impact.

## Task 6 — sweep for stale references

Grep the whole tree for `ApplyPolicy`, `DemoteAfter`, `PROVIDER_DEMOTE_AFTER`, `manual-only` (comment sweeps in `roster_metrics.go`, `admin_scraper_providers.go`, `migrate.go` comments, compose env). Update comments/expectations; `docker/docker-compose.yml` has no `PROVIDER_DEMOTE_AFTER` (verify). Fix `migrate_test.go`/`roster_metrics_test.go` band expectations if they assert `manual → Degraded` (WireStatus expectations unchanged).

## Task 7 — verify

- `cd services/catalog && go build ./... && go vet ./... && go test ./... -count=1 -race`
- `cd libs/metrics && go build ./...`
- JSON validity check (Task 5.4).
- Confirm no `ApplyPolicy`/`DemoteAfter` references remain (`grep -rn` clean).

## Rollout (after review, via /animeenigma-after-update)

- Redeploy **catalog**; grafana dashboard is bind-mounted from base tree — after push+base-sync, provisioner auto-reloads (~10s).
- One-shot ops (post-deploy): reset Miruro if still down from the false-negative:
  `UPDATE stream_providers SET health='up', health_since=now(), reason='hand-reset after false-negative; hysteresis deployed' WHERE name='miruro' AND health <> 'up';`
- Expect existing `manual/down` tombstones to move Degraded→Disabled band on the dashboard (cosmetic, intended).
