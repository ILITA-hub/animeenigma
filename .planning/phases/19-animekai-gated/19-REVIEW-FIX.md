---
phase: 19-animekai-gated
fixed_at: 2026-05-12T00:00:00Z
review_path: .planning/phases/19-animekai-gated/19-REVIEW.md
iteration: 1
findings_in_scope: 6
fixed: 6
skipped: 0
status: all_fixed
---

# Phase 19: Code Review Fix Report

**Fixed at:** 2026-05-12
**Source review:** `.planning/phases/19-animekai-gated/19-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 6 (1 critical + 5 warnings; INFO items skipped per scope)
- Fixed: 6
- Skipped: 0

## Fixed Issues

### CR-01: Boot-time metric seed contradicts escape-hatch "no green panel" invariant

**Files modified:** `services/scraper/cmd/scraper-api/main.go`, `services/scraper/cmd/scraper-api/main_test.go`
**Commit:** `840181b`
**Applied fix:** Introduced a `bootHealthSeedValue(name string) float64` helper in `cmd/scraper-api/main.go` that returns `0` for `"animekai"` (escape-hatch invariant — Grafana must never show a green panel during the ~15 min before the first probe tick lands) and `1` for every other provider (optimistic default preserved). The seed loop now calls this helper per provider instead of hard-coding `Set(1)`. Added a new `main_test.go` with `TestBootHealthSeedValue_AnimeKaiSeedsZero` (regression test) and `TestBootHealthSeedValue_RealProvidersSeedOne` (covers `animepahe`, `gogoanime`, and any future name). A name-based decision is an intentional pragmatic shortcut; the longer-term refactor (expose `BootHealthSeed()` as an optional method on `domain.Provider`) is called out in the helper doc comment but intentionally out of Phase 19 scope (would touch every provider).

### WR-01: `New()` accepts `Deps.MalSync == nil` silently — v3.1 fill-in lands a nil-pointer footgun

**Files modified:** `services/scraper/internal/providers/animekai/client.go`, `services/scraper/internal/providers/animekai/client_test.go`, `services/scraper/cmd/scraper-api/main.go`
**Commit:** `1c66249`
**Applied fix:** Tightened `animekai.New()` to reject `Deps.MalSync == nil` with an explicit error that names the field and points at the new `NewNoopMalSync()` helper. Added an exported `NewNoopMalSync()` (sentinel `noopMalSync{}` whose `Lookup` returns `("", false, nil)`) so `main.go` can satisfy the strict validation without touching real-malsync wiring. Updated `main.go` to pass `animekai.NewNoopMalSync()` in place of literal `nil`. Renamed `TestProvider_New_RequiresHTTPAndEmbeds` → `TestProvider_New_RequiresAllDeps` to assert the strict MalSync branch and the actionable error message ("must mention MalSync"). The v3.1 fill-in PR is now a one-line change in `main.go`: replace `NewNoopMalSync()` with `NewMalSyncClient(redisCache)`.

### WR-02: Sidecar `/animekai-token` does not drain the POST request body

**Files modified:** `docker/megacloud-extractor/server.js`
**Commit:** `d0ffd46`
**Applied fix:** Attached `req.on("data", () => {})` + `req.on("end", () => { ... })` around the 501 response so the request body is fully drained before the response closes. Wire-level behavior is otherwise unchanged: same HTTP 501 status, same JSON error payload. The fix prevents ECONNRESET on keep-alive callers (the Go scraper's shared `http.Client`) which would otherwise mask the intended `domain.ErrProviderDown` mapping. Verified with `node -c server.js`.

### WR-03: `getEnvBool` silently swallows unparseable values — operator confusion latent

**Files modified:** `services/scraper/internal/config/config.go`, `services/scraper/internal/config/config_test.go`
**Commit:** `0da36be`
**Applied fix:** Added an `import "log"` and emitted a WARN-level message via `log.Printf` when `getEnvBool` encounters an unparseable env value. The message names both the env-var key and the rejected value verbatim so an operator who set `SCRAPER_ANIMEKAI_ENABLED=yes-please` can grep the log and immediately see their typo. Preserved the lenient fallback-to-default convention (existing `TestLoad_AnimeKaiEnabledInvalid` continues to pass) so no caller behavior shifts. Added `TestGetEnvBool_LogsOnUnparseable` which captures stderr via `log.SetOutput(&buf)` and asserts the captured log contains "WARN", the probe key name, and the rejected literal value.

### WR-04: `client.go:36-41` `stageNames` diverges from `health.AllStages`

**Files modified:** `services/scraper/internal/providers/animekai/client.go`, `services/scraper/internal/providers/animekai/client_test.go`
**Commit:** `ab29688`
**Applied fix:** Replaced the local 4-stage `stageNames` slice with `var stageNames = health.AllStages` (5 stages, including `stream_segment`). For the escape-hatch invariant to hold end-to-end, every stage the Prometheus metric surface knows about must also appear in the provider's in-memory snapshot — otherwise the two surfaces disagree on which stages exist for `animekai`. Updated `TestProvider_HealthCheck_AllStagesDownAtBoot` to iterate `health.AllStages` directly. Added a new `TestProvider_StageNames_MatchesHealthAllStages` regression guard that breaks the build if a future stage is added to `health.AllStages` without also being reflected here. Gogoanime keeps its own 4-stage local copy (not an escape-hatch provider, so out of scope).

### WR-05: Wiring invariant gives a misleading error message on misuse

**Files modified:** `services/scraper/cmd/scraper-api/main.go`
**Commit:** `07e2e30`
**Applied fix:** Captured `orchestrator.RegisteredProviders()` once into a `registered` slice, then collected the names into a `[]string` and passed them as the `registered` field on `log.Fatalw`. An on-call hitting this fatal in production now sees WHICH providers actually landed in the orchestrator without having to read source. Matches the context-richness of the other `Fatalw` call sites in `main.go` (e.g. line 116 "failed to construct AnimePahe provider", "error", err).

## Skipped Issues

None — all 6 in-scope findings were fixed.

The 4 INFO findings (IN-01 SSRF surface, IN-02 unused DTOs, IN-03 fragile test-count comment, IN-04 outdated comment) were intentionally out of scope per the orchestrator prompt and remain as-is for a future cleanup pass.

---

## Verification

- `go build ./...` — passes in `services/scraper/`
- `go test ./...` — passes in `services/scraper/` (all packages; phases 16+18 regression suites included)
- `go test -run TestBootHealthSeedValue ./cmd/scraper-api/` — passes (CR-01 regression)
- `go test -run TestProvider_New_RequiresAllDeps ./internal/providers/animekai/` — passes (WR-01)
- `node -c docker/megacloud-extractor/server.js` — passes (WR-02 syntax)
- `go test -run TestGetEnvBool_LogsOnUnparseable ./internal/config/` — passes (WR-03)
- `go test -run TestProvider_StageNames_MatchesHealthAllStages ./internal/providers/animekai/` — passes (WR-04)

Runtime smoke (`make redeploy-scraper && curl /scraper/health`) is deferred to the verifier phase per the agent contract (no full deploy in-flight from the fixer agent).

---

_Fixed: 2026-05-12_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
