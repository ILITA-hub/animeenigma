---
phase: 03-dynamic-cards-migration
plan: 02
subsystem: catalog-service
workstream: hero-spotlight
milestone: v1.0
tags: [hero-spotlight, catalog-service, middleware, http-client, adaptive-slice, phase-3]
requirements:
  - HSB-BE-23
  - HSB-BE-30
  - HSB-BE-01
dependency_graph:
  requires:
    - services/player/internal/transport/optional_auth.go    # verbatim source
    - services/player/internal/handler/recs.go               # /api/users/recs envelope contract
    - services/player/internal/handler/list_internal.go      # /internal/users/{id}/list envelope
    - libs/authz                                              # JWTConfig + ContextWithClaims
    - libs/httputil                                           # BearerToken
    - libs/logger                                             # *logger.Logger
  provides:
    - OptionalAuthMiddleware (services/catalog/internal/transport)
    - ContextWithJWT / JWTFromContext (services/catalog/internal/service/spotlight/cards)
    - AdaptiveSlice[T] (services/catalog/internal/service/spotlight)
    - PlayerClient.FetchUserRecs / FetchListByStatuses (services/catalog/internal/service/spotlight/client)
    - UserRec, InternalListItem (wire types)
  affects:
    - Plan 03 (PersonalPick / TelegramNews / NowWatching / NotTimeYet / ContinueWatchingNew resolvers)
    - Plan 04 (spotlight handler wires OptionalAuthMiddleware + ContextWithJWT; latest_news retrofitted to AdaptiveSlice)
tech_stack:
  added:
    - go.uber.org/zap/zaptest/observer (test-only, transitive — already in go.sum via zap)
  patterns:
    - Verbatim middleware port across services (frozen behavioral contract via shared tests)
    - Generic typed helper for layout rule (avoids 4 duplicate switch blocks)
    - HTTP-client constructor with sensible-defaults + injectable transport (mirror of web_client.go)
    - Typed-key context propagation for secret values (no env-var, no struct widening)
    - Structured-log secret-redaction (T-03-05: never log JWT value)
key_files:
  created:
    - services/catalog/internal/transport/optional_auth.go (46 LOC)
    - services/catalog/internal/transport/optional_auth_test.go (110 LOC, 3 tests — ports of player tests)
    - services/catalog/internal/service/spotlight/cards/jwt_context.go (43 LOC)
    - services/catalog/internal/service/spotlight/cards/jwt_context_test.go (63 LOC, 4 tests)
    - services/catalog/internal/service/spotlight/adaptive_slice.go (51 LOC)
    - services/catalog/internal/service/spotlight/adaptive_slice_test.go (117 LOC, 9 tests)
    - services/catalog/internal/service/spotlight/client/player_client.go (218 LOC)
    - services/catalog/internal/service/spotlight/client/player_client_test.go (276 LOC, 11 tests)
  modified: []
decisions:
  - "OptionalAuthMiddleware is a verbatim port of player's — `diff <(grep -A12 OptionalAuthMiddleware ...catalog/...) <(...player/...)` returns 0 lines of body diff. Doc comment is the only intentional divergence (points at the catalog use-case, /home/spotlight wrap site)."
  - "JWT propagation via cards.ContextWithJWT/JWTFromContext typed-key helper, NOT via Resolver-interface widening. Empty-string JWT collapses to ok=false to prevent `Authorization: Bearer ` (no value) on the wire."
  - "AdaptiveSlice as a single generic helper rather than 4 copies inline. N=2 random pick injects *rand.Rand for deterministic resolver tests; nil rng on the N==2 branch panics (correctness requirement, not silent fallback)."
  - "PlayerClient.FetchUserRecs decodes the player's httputil.OK envelope (`{success, data: RecsEnvelope}`) and extracts only `data.recs`. FetchListByStatuses decodes the bare `{items: [...]}` shape that player's /internal/* route returns."
  - "Default timeout 700ms (tighter than the aggregator's 800ms per-card budget) so transport-level failures surface before the resolver's parent context deadline trips."
  - "T-03-05 (info disclosure): structured-log fields used by PlayerClient include `url`, `status`, `user_id`, `error` — NEVER `jwt`. TestPlayerClient_FetchUserRecs_NeverLogsJWT asserts this with a zaptest observer that scans the full structured payload."
metrics:
  duration_minutes: 14
  completed: 2026-05-21
  tasks_completed: 3
  total_files: 8
  total_loc: 924
  tests_added: 27
  uxd: 0
  cdi: 0.04
  cdi_effort_fib: 5
  mvq: Sprite 92%/90%
---

# Phase 03 Plan 02: Optional Auth + AdaptiveSlice + PlayerClient — Summary

**One-liner:** Three shared primitives (OptionalAuthMiddleware, AdaptiveSlice[T], PlayerClient + JWT context helpers) that Plan 03's 5 resolvers — and Plan 04's wiring — consume. Each primitive landed as a self-contained TDD-disciplined unit so Plan 03 can focus purely on resolver business logic.

## What shipped

### Task 1 — OptionalAuthMiddleware + JWT context helpers (commit `b70129e`)

- `services/catalog/internal/transport/optional_auth.go` — verbatim port of `services/player/internal/transport/optional_auth.go`. Behavior contract FROZEN by 3 ported tests:
  1. Missing Authorization header → next handler called WITHOUT claims, no 401.
  2. Valid Bearer JWT → next handler called WITH claims attached via `authz.ContextWithClaims`.
  3. Malformed Bearer JWT → next handler called WITHOUT claims, no 401.
- `services/catalog/internal/service/spotlight/cards/jwt_context.go` — `ContextWithJWT(ctx, jwt) → ctx` and `JWTFromContext(ctx) → (jwt, bool)`. Typed-key `struct{}` is unexported so only the cards package can read it. Empty-string JWT collapses to ok=false (prevents `Authorization: Bearer ` on the wire).

### Task 2 — Generic AdaptiveSlice[T] (commit `f492ee6`)

- `services/catalog/internal/service/spotlight/adaptive_slice.go` — implements HSB-BE-30:
  - `len==0 → nil` (resolver treats as eligibility=false)
  - `len==1 → items` (passthrough)
  - `len==2 → []T{items[rng.Intn(2)]}` (random pick — rng MUST be non-nil; nil rng panics with `"rng is required"` substring per test)
  - `len>=3 → items[:3]` (positional top-K, no shuffle)
- 9 table-driven tests cover all 4 branches + nil-input + nil-rng panic + generic-string instantiation + N==2 reach-both-indices probe across seeds.

### Task 3 — PlayerClient (commit `b5a486c`)

- `services/catalog/internal/service/spotlight/client/player_client.go` — thin HTTP fan-out:
  - `FetchUserRecs(ctx, jwt) ([]UserRec, error)` → `GET /api/users/recs`. JWT forwarded in `Authorization: Bearer` header when non-empty (anon callers pass `""` so the header is omitted entirely). Decodes the `httputil.OK` envelope and returns `data.recs`.
  - `FetchListByStatuses(ctx, userID, statuses) ([]InternalListItem, error)` → `GET /internal/users/{userID}/list?status=watching,planned`. NO JWT (gateway does not proxy `/internal/*`). userID `url.PathEscape`'d. Empty/nil statuses short-circuit with no HTTP call.
- Defaults: `baseURL=http://player:8083`, `timeout=700ms` (tighter than the aggregator's 800ms per-card budget — Pitfall 8 from 01-RESEARCH.md).
- T-03-05 mitigation: 11 tests pass including the `NeverLogsJWT` assertion that scans the full zaptest-observer payload for the literal token string `supersecretjwttoken-abc123` — finds it nowhere.

## Verification

| Gate | Command | Result |
|---|---|---|
| OptionalAuth contract | `go test ./internal/transport -run TestOptionalAuth -count=1 -race` | 3/3 PASS |
| JWT context | `go test ./internal/service/spotlight/cards -count=1 -race` | 4/4 PASS |
| AdaptiveSlice | `go test ./internal/service/spotlight -run TestAdaptiveSlice -count=1 -race` | 9/9 PASS |
| PlayerClient | `go test ./internal/service/spotlight/client -run TestPlayerClient -count=1 -race` | 11/11 PASS |
| Build clean | `go build ./...` | exit 0 |
| Vet clean | `go vet ./...` | exit 0 |
| Verbatim-port assertion | `diff <(grep -A12 OptionalAuthMiddleware catalog/...) <(grep -A12 OptionalAuthMiddleware player/...)` | IDENTICAL (body match — only file/package header differs) |
| No JWT in logs | `grep -nv '^\s*//' player_client.go \| grep 'log\.W' \| grep '"jwt"'` | 0 matches |

All 27 new tests pass; existing tests (WebClient × 7, aggregator, etc.) continue to pass — no regressions.

## Deviations from Plan

**None — plan executed exactly as written.**

Per-task deviations to note:
- Plan said "if defaults are private — keeps the production path unchanged" for Test 10; I exposed `BaseURL()` (already the precedent in `web_client.go`) and asserted `c.http.Timeout` against the unexported field by reading it directly within the same package — no production-path leakage. Plan-acceptable per the "exported getters or a separate test-only constructor" disjunction.
- Plan's `<interfaces>` block named the methods `FetchUserRecs` and `FetchListByStatuses` (in the artifact lines) but the prompt mentioned `GetUserList`/`GetRecommendations`. I followed the plan's authoritative `<interfaces>` block (`Fetch*` naming) because that's what the artifact contract pins. Plan 03 is single-author and will consume the `Fetch*` names directly.

## Threat Surface Audit

No new threat surfaces introduced beyond those in the plan's `<threat_model>`. T-03-05 (JWT-in-logs info disclosure) is actively mitigated by the `NeverLogsJWT` test.

## Plan 03 / Plan 04 hand-off

- **Plan 03** consumes:
  - `cards.JWTFromContext(ctx)` inside PersonalPickResolver's login branch.
  - `client.NewPlayerClient(...)` + `FetchUserRecs(ctx, jwt)` for PersonalPick login path.
  - `client.NewPlayerClient(...)` + `FetchListByStatuses(ctx, userID, []string{"planned","postponed"})` for NotTimeYet.
  - `client.NewPlayerClient(...)` + `FetchListByStatuses(ctx, userID, []string{"watching"})` for ContinueWatchingNew.
  - `spotlight.AdaptiveSlice[T](items, rng)` for each multi-item resolver.
- **Plan 04** consumes:
  - `transport.OptionalAuthMiddleware(jwtCfg)` wrapping the `/home/spotlight` route.
  - `cards.ContextWithJWT(ctx, jwt)` inside the spotlight handler BEFORE calling `aggregator.Resolve`.
  - `spotlight.AdaptiveSlice[T]` retrofitted into Phase 1's `latest_news` resolver.

## Self-Check: PASSED

- [x] `services/catalog/internal/transport/optional_auth.go` exists (FOUND)
- [x] `services/catalog/internal/transport/optional_auth_test.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/cards/jwt_context.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/cards/jwt_context_test.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/adaptive_slice.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/adaptive_slice_test.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/client/player_client.go` exists (FOUND)
- [x] `services/catalog/internal/service/spotlight/client/player_client_test.go` exists (FOUND)
- [x] Commit `b70129e` in `git log` (FOUND)
- [x] Commit `f492ee6` in `git log` (FOUND)
- [x] Commit `b5a486c` in `git log` (FOUND)
- [x] `go build ./...` exits 0
- [x] `go vet ./...` exits 0
- [x] All 27 new tests pass under `-race`

**Metrics (per CONVENTIONS.md):**
- **UXΔ = 0 (Ambiguous)** — pure infrastructure; no user-visible change.
- **CDI = 0.04 × 5** — 4 new file pairs (source + test), one verbatim port, one generic helper, one HTTP client mirror of existing pattern. Effort Fib=5 (focused TDD work, no schema or build changes).
- **MVQ = Sprite 92%/80%** — small focused primitives. Slop risk was inventing a fancier middleware than player's; defused by verbatim-port discipline (diff returns IDENTICAL body).
