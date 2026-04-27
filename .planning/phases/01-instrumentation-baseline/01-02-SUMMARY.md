---
phase: 01-instrumentation-baseline
plan: 02
subsystem: instrumentation
tags: [metrics, prometheus, handler, player-service, observability]
requirements: [M-01]
threats: [T-01-01, T-01-02, T-01-03]
dependency_graph:
  requires:
    - libs/metrics (existing promauto.NewCounterVec pattern)
    - libs/authz (ClaimsFromContext)
    - libs/httputil (Bind, Error)
    - libs/errors (InvalidInput)
    - libs/logger (Infow via embedded SugaredLogger)
  provides:
    - libs/metrics.ComboOverrideTotal (5-label CounterVec)
    - libs/metrics.ComboResolveTotal (4-label CounterVec)
    - services/player/internal/handler.OverrideHandler
    - services/player/internal/handler.NewOverrideHandler
    - services/player/internal/handler.(*OverrideHandler).RecordOverride
  affects:
    - plan 01-03 (routing + resolver integration consumes both metrics + handler)
tech-stack:
  added: []
  patterns:
    - "promauto.NewCounterVec for Prometheus metric registration"
    - "Optional-auth: JWT claims OR X-Anon-ID header (T-01-01 mitigation)"
    - "Closed-set whitelist for user-controlled Prometheus labels (T-01-02)"
    - "Structured Loki log line via libs/logger Infow (T-01-03 PII-clean)"
key-files:
  created:
    - services/player/internal/handler/override.go
  modified:
    - libs/metrics/watch.go
decisions:
  - "Two new CounterVecs registered package-level via promauto — auto-register, no main.go change"
  - "Handler is pure instrumentation — no service/repo/DB dependencies, no business logic"
  - "Anon flood vector mitigated by requiring at least one identity (claims OR X-Anon-ID)"
metrics:
  duration: ~12 min
  completed_date: 2026-04-27
---

# Phase 1 Plan 02: Backend Metrics + OverrideHandler Summary

**One-liner:** Two Prometheus CounterVecs (`ComboOverrideTotal` 5-label, `ComboResolveTotal` 4-label) added to `libs/metrics/watch.go`, plus a pure-instrumentation `OverrideHandler` in the player service that emits one counter increment + one structured `combo_override` Loki log line per accepted request, with strict T-01-01/T-01-02/T-01-03 mitigations baked into the request path.

## Status

| Item | Result |
|------|--------|
| Tasks completed | 2 / 2 |
| Wave-0 contract satisfaction | Wave-1 plan — Wave-0 tests live in plan 01-01's parallel worktree; orchestrator merges before integration verify |
| Build green | `cd libs/metrics && go build ./...` exits 0 |
| Handler regressions | `cd services/player && go test ./internal/handler -count=1` → `ok` |
| New CounterVecs | 2 (3 existing → 5 total in `watch.go`; 6 total in package) |

## Tasks

### Task 1 — Add ComboOverrideTotal + ComboResolveTotal

- **File:** `libs/metrics/watch.go` (modified, +24 lines)
- **Commit:** `8469ba0`
- **What:** Inserted two new `promauto.NewCounterVec` definitions immediately after `PreferenceFallbackTotal` inside the same `var (...)` block.
  - `ComboOverrideTotal` — labels `[]string{"tier", "dimension", "language", "anon", "player"}` (5-label, 384-series cardinality budget)
  - `ComboResolveTotal` — labels `[]string{"tier", "language", "anon", "player"}` (4-label, denominator for the override-rate PromQL)
- **Acceptance grep counts:**
  - `grep -c "ComboOverrideTotal = promauto.NewCounterVec" libs/metrics/watch.go` → 1
  - `grep -c "ComboResolveTotal = promauto.NewCounterVec" libs/metrics/watch.go` → 1
  - `grep -c "promauto.NewCounterVec" libs/metrics/watch.go` → 5 (3 existing + 2 new)

### Task 2 — Create OverrideHandler with RecordOverride

- **File:** `services/player/internal/handler/override.go` (new, 139 lines)
- **Commit:** `c647913`
- **What:** New handler with:
  - `OverrideRequest` payload struct (9 fields, JSON-tagged per Wave-0 contract: `anime_id, load_session_id, dimension, original_combo, new_combo, ms_since_load, tier, tier_number, player`)
  - `validDimensions` map enforcing closed set `{language, player, team, episode}` (T-01-02)
  - `OverrideHandler` struct holding only `*logger.Logger` — no service/repo deps
  - `NewOverrideHandler(log *logger.Logger) *OverrideHandler` constructor
  - `(*OverrideHandler).RecordOverride(w, r)` — full request path with the four guards (Bind error → 400, missing IDs → 400, invalid dimension → 400, no identity → 400), one Prometheus increment, one structured Loki log line, then 204 No Content.
  - `labelOrUnknown` helper to coerce empty strings to `"unknown"` (Pitfall 3 mitigation).
  - `stringFromMap` helper extracting `language` from `NewCombo` for the metric label.
- **Auth model:** Optional — JWT claims set `userID`/`anon=false`; otherwise `X-Anon-ID` sets `anonID`/`anon=true`. Neither → 400 (T-01-01).
- **PII:** Log line fields enumerated explicitly — `anime_id, load_session_id, dimension, user_id, anon_id, original_combo, new_combo, ms_since_load, tier, tier_number, player`. NO `username`, NO `Authorization`, NO bearer token, NO request URL (T-01-03).

## Threat Mitigations Applied

| Threat | Mitigation Site | Verification Anchor |
|--------|-----------------|---------------------|
| T-01-01 (DoS — headerless flood) | `if userID == "" && anonID == "" { httputil.Error(...); return }` before any metric touch | `grep -n 'X-Anon-ID required for unauthenticated requests' services/player/internal/handler/override.go` |
| T-01-02 (Tampering — cardinality explosion) | `validDimensions` whitelist + `labelOrUnknown` coercion for free-form labels | `grep -n 'validDimensions\[req.Dimension\]' services/player/internal/handler/override.go` |
| T-01-03 (Information Disclosure — PII in logs) | Explicit field enumeration in `Infow`; no `username`, no `Authorization` | `grep -v '^[[:space:]]*//' services/player/internal/handler/override.go \| grep -iE 'username\|authorization\|bearer\\s'` returns empty |

## Verification Run

```
=== libs/metrics build ===
libs/metrics OK
=== player handler tests ===
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/handler	0.022s
=== promauto.NewCounterVec count (expect >=5) ===
6
=== libs/metrics test regressions ===
no FAIL
```

## Wave-0 Test Coverage Note

Plan 01-02 is Wave 1; the Wave-0 `services/player/internal/handler/override_test.go` (8 `TestOverride_*` cases) lives in plan 01-01's parallel worktree. This worktree contains only the implementation matching the Wave-0 contract documented in `01-02-PLAN.md` `<interfaces>`:

- `metrics.ComboOverrideTotal.WithLabelValues(tier, dimension, language, anon, player)` — 5-label match
- `NewOverrideHandler(log *logger.Logger) *OverrideHandler` — exact signature
- `(*OverrideHandler).RecordOverride(w, r)` — exact signature
- `OverrideRequest` JSON tags match the 9 fields in the contract

The orchestrator merge (Wave 0 + Wave 1) brings both files into the same tree; the integration test will run as plan 03 / phase verifier confirms `go test ./internal/handler -run "TestOverride" -count=1` passes 8/8.

## Operator-Facing Verify After Deploy

After `make redeploy-player`, operators can confirm the metrics are registered via:

```bash
curl -s http://localhost:8083/metrics | grep -E "^combo_(override|resolve)_total"
```

(The lines will appear with zero counts until the first request lands; the metric registry is populated at process start because `promauto` registers on package init.)

## Note for Plan 03

Routing + resolver-integration is plan 03's job. This plan delivers:

- The two metric definitions (so plan 03's resolver-edit can reference `metrics.ComboResolveTotal.WithLabelValues(...)` directly, and the gateway/router can wire the override path).
- The handler (`overrideHandler.RecordOverride`) ready to be plugged into the chi router under an `OptionalAuth`-protected `/preferences/*` group.

Plan 03 owns:

- `services/player/internal/transport/optional_auth.go` (new middleware)
- `services/player/internal/transport/router.go` modifications (route group, constructor signature)
- `services/player/internal/service/preference.go` modification (emit `ComboResolveTotal` next to existing `PreferenceResolutionTotal`)
- `services/player/internal/handler/preference.go` modification (drop hard-401, accept anon)
- `services/gateway/internal/transport/router.go` modification (public `/preferences/*` proxy line)
- `cmd/player-api/main.go` constructor wiring update

## Deviations from Plan

**None — plan executed exactly as written.**

The plan's task-2 acceptance criterion `grep -E '"(language|player|team|episode)": true' ... | wc -l` returns 1 instead of 4 because `gofmt` aligns the colons (`"player":   true`) which adds whitespace the regex doesn't allow. Content is correct; the looser check `grep -cE '"(language|player|team|episode)":' ...` returns 4. Treating this as a documentation nit, not a deviation — the closed-set whitelist is in the file and verified by the build + the upcoming Wave-0 tests.

## Self-Check: PASSED

- [x] FOUND: libs/metrics/watch.go (modified)
  - 2 new CounterVec definitions verified by grep
- [x] FOUND: services/player/internal/handler/override.go (created, 139 lines)
- [x] FOUND commit: 8469ba0 (Task 1 — metrics)
- [x] FOUND commit: c647913 (Task 2 — handler)
- [x] BUILD: `cd libs/metrics && go build ./...` exits 0
- [x] BUILD: `cd services/player && go build ./...` exits 0
- [x] TEST: `cd services/player && go test ./internal/handler -count=1` → `ok`
- [x] No accidental file deletions in either commit
- [x] No PII in handler (T-01-03 grep clean)
- [x] No regressions in libs/metrics tests
