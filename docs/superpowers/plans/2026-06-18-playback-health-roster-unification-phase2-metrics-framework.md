# Playback-Health Roster Unification — Phase 2: Status-Driven Metrics Framework

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `provider_info` / `provider_enabled` cover the FULL 13-provider roster from one shared helper that both catalog and scraper call — replacing catalog's drifted hardcoded 5-player slice with a DB-roster-driven emit — so the Grafana provider count and management table finally reflect every stream source.

**Architecture:** Add a single shared `metrics.EmitProviderRoster([]RosterEntry)` helper in `libs/metrics`. Catalog and scraper PARTITION the roster (no overlap, no duplicate series): catalog emits the rows it owns (`scraper_operated = false`: ae, kodik, animelib, hanime, raw) loaded from the `stream_providers` table; scraper continues to emit the rows it owns (`scraper_operated = true`: the EN chain + 18anime) — but now through the same helper. The drifted hardcoded slice in catalog's `PlayerHealthChecker` constructor is deleted; the live Kodik probe stays.

**Tech Stack:** Go, GORM (Postgres prod / SQLite tests), Prometheus client_golang (+ `prometheus/testutil` for assertions).

## Global Constraints

- **Effort/impact units:** UXΔ / CDI / MVQ only — never days/hours/sprints (`.planning/CONVENTIONS.md`).
- **Go conventions:** snake_case files, PascalCase exported types, `libs/errors` for domain errors, structured `libs/logger`.
- **Shared dirty tree:** path-scoped commits (`git commit <pathspec>`), never `git add -A`, never `--amend`, `git show --stat HEAD` after each commit. Implementers do NOT `git push` — the controller lands commits onto `origin/main`. Execute from an isolated worktree off `origin/main`.
- **Single-emitter partition (the whole point):** a given provider name's `provider_info`/`provider_enabled` is emitted by EXACTLY ONE service. Catalog owns `scraper_operated = false`; scraper owns `scraper_operated = true`. They must never both emit the same provider name (that would create conflicting duplicate series across Prometheus targets).
- **Roster reflection vs live metrics:** `provider_info` (always 1, carries `status` label) and `provider_enabled` (1 iff `status == "enabled"`) are ROSTER REFLECTION — emitted for ALL owned rows including `disabled`, so the management view stays complete. This is distinct from LIVE metrics (`provider_health_up`, probes, parser latency, telemetry) which are gated by registration and excluded for disabled providers — those are Phase 3, NOT this plan.
- **Boot-time emission** (matches current behavior for both services): roster reflection is emitted once at service boot. Operator status edits reflect on the next service restart — same as today; do NOT add a periodic re-emitter (status is a metric label, so re-emitting after a status change would leave a stale old-status series).
- **Canonical provider names come from the DB roster** (`stream_providers`): `raw` (NOT `allanime-raw`), and animelib/hanime status is whatever the DB says (Phase 1 seeded them `enabled`), NOT the stale hardcoded `disabled`.

**Spec:** `docs/superpowers/specs/2026-06-17-playback-health-provider-roster-unification-design.md` (§2 status-driven framework). **Phase 1 (shipped):** table renamed `stream_providers`, `scraper_operated` flag, 13-row roster.

---

## Phase 2 File Structure

| File | Responsibility | Change |
|---|---|---|
| `libs/metrics/roster.go` | Shared roster-reflection emit helper | **Create** |
| `libs/metrics/roster_test.go` | Helper tests | **Create** |
| `services/catalog/internal/service/scraperprovider/roster_metrics.go` | Load catalog-owned rows + emit | **Create** |
| `services/catalog/internal/service/scraperprovider/roster_metrics_test.go` | DB→metrics test | **Create** |
| `services/catalog/internal/service/health_checker.go` | Live Kodik probe | Delete the hardcoded info/enabled emission (lines 47–74); keep the probe |
| `services/catalog/cmd/catalog-api/main.go` | Boot wiring | Call `EmitCatalogSideRoster` after the Phase 1 backfill |
| `services/scraper/cmd/scraper-api/main.go` | Scraper boot emit | Route the existing emit loop through the shared helper |

**Out of scope (later phases):** adding probes/parser-latency/telemetry for catalog-side providers (Phase 3); rewiring the dashboard `$provider` template var from `provider_health_up` to `provider_info` + the Postgres datasource (Phase 4). Phase 2 only makes the metrics cover all 13.

---

### Task 1: Shared `EmitProviderRoster` helper in libs/metrics

**Files:**
- Create: `libs/metrics/roster.go`
- Create: `libs/metrics/roster_test.go`

**Interfaces:**
- Produces: `metrics.RosterEntry{ Name, Status, Reason, Description string }`; `metrics.EmitProviderRoster(entries []RosterEntry)`.

- [ ] **Step 1: Write the failing test**

Create `libs/metrics/roster_test.go`:

```go
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestEmitProviderRoster_SetsInfoAndEnabled(t *testing.T) {
	EmitProviderRoster([]RosterEntry{
		{Name: "t1_enabled", Status: "enabled", Reason: "r-en", Description: "d-en"},
		{Name: "t1_degraded", Status: "degraded", Reason: "r-deg", Description: "d-deg"},
		{Name: "t1_disabled", Status: "disabled", Reason: "r-dis", Description: "d-dis"},
	})

	// provider_enabled: 1 ONLY for status=="enabled".
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_enabled")); got != 1 {
		t.Errorf("provider_enabled{t1_enabled} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_degraded")); got != 0 {
		t.Errorf("provider_enabled{t1_degraded} = %v, want 0 (degraded is not enabled)", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_disabled")); got != 0 {
		t.Errorf("provider_enabled{t1_disabled} = %v, want 0", got)
	}

	// provider_info: always 1, emitted for ALL rows incl. disabled (roster reflection).
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("t1_disabled", "disabled", "r-dis", "d-dis")); got != 1 {
		t.Errorf("provider_info{t1_disabled,...} = %v, want 1 (disabled rows still reflected)", got)
	}
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("t1_enabled", "enabled", "r-en", "d-en")); got != 1 {
		t.Errorf("provider_info{t1_enabled,...} = %v, want 1", got)
	}
}

func TestEmitProviderRoster_EmptyIsNoop(t *testing.T) {
	EmitProviderRoster(nil) // must not panic
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd libs/metrics && go test ./... -run TestEmitProviderRoster -v`
Expected: FAIL — `undefined: RosterEntry` / `undefined: EmitProviderRoster`.

- [ ] **Step 3: Implement the helper**

Create `libs/metrics/roster.go`:

```go
package metrics

// RosterEntry is one provider row for roster-reflection metric emission. It is the
// minimal projection of a stream_providers row that the management metrics need.
type RosterEntry struct {
	Name        string
	Status      string // "enabled" | "degraded" | "disabled"
	Reason      string
	Description string
}

// EmitProviderRoster reflects a set of provider rows into the management metrics:
//   - provider_info{provider,status,reason,description} = 1 for EVERY entry
//     (roster reflection — disabled rows stay visible in the Grafana table).
//   - provider_enabled{provider} = 1 iff status == "enabled", else 0.
//
// Single-emitter contract: the caller passes ONLY the rows IT owns. Catalog owns
// scraper_operated=false rows; the scraper owns scraper_operated=true rows. The two
// sets partition the roster with no name overlap, so there are no duplicate series
// across Prometheus targets. Call at service boot.
func EmitProviderRoster(entries []RosterEntry) {
	for _, e := range entries {
		enabled := 0.0
		if e.Status == "enabled" {
			enabled = 1.0
		}
		ProviderEnabled.WithLabelValues(e.Name).Set(enabled)
		ProviderInfo.WithLabelValues(e.Name, e.Status, e.Reason, e.Description).Set(1)
	}
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `cd libs/metrics && go test ./... -run TestEmitProviderRoster -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git commit libs/metrics/roster.go libs/metrics/roster_test.go \
  -m "feat(metrics): shared EmitProviderRoster roster-reflection helper

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: Catalog emits its owned roster rows from the DB

**Files:**
- Create: `services/catalog/internal/service/scraperprovider/roster_metrics.go`
- Create: `services/catalog/internal/service/scraperprovider/roster_metrics_test.go`
- Modify: `services/catalog/internal/service/health_checker.go` (delete lines 47–74 emission block)
- Modify: `services/catalog/cmd/catalog-api/main.go` (wire the new call)

**Interfaces:**
- Consumes: `metrics.RosterEntry`, `metrics.EmitProviderRoster` (Task 1); `domain.ScraperProvider` (Phase 1).
- Produces: `scraperprovider.EmitCatalogSideRoster(db *gorm.DB) error`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/scraperprovider/roster_metrics_test.go`:

```go
package scraperprovider_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEmitCatalogSideRoster_OnlyEmitsOwnedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// One catalog-owned row (scraper_operated=false) and one scraper-owned row
	// (scraper_operated=true). Catalog must emit ONLY the former.
	db.Create(&domain.ScraperProvider{Name: "t2_ae", Status: domain.StatusEnabled, Group: "firstparty", Reason: "first-party", Description: "self-hosted", ScraperOperated: false})
	db.Create(&domain.ScraperProvider{Name: "t2_gogo", Status: domain.StatusEnabled, Group: "en", ScraperOperated: true})

	if err := scraperprovider.EmitCatalogSideRoster(db); err != nil {
		t.Fatalf("emit: %v", err)
	}

	// Owned row reflected.
	if got := testutil.ToFloat64(metrics.ProviderEnabled.WithLabelValues("t2_ae")); got != 1 {
		t.Errorf("provider_enabled{t2_ae} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.ProviderInfo.WithLabelValues("t2_ae", "enabled", "first-party", "self-hosted")); got != 1 {
		t.Errorf("provider_info{t2_ae,...} = %v, want 1", got)
	}
	// Scraper-owned row must NOT be emitted by catalog (partition contract).
	// A never-Set() gauge reads 0, so this proves catalog skipped t2_gogo.
	if got := testutil.ToFloat64(metrics.ProviderEnabled.WithLabelValues("t2_gogo")); got != 0 {
		t.Errorf("provider_enabled{t2_gogo} = %v, want 0 — catalog must not emit scraper-owned rows", got)
	}
}
```

> Note: `t2_gogo` returning 0 proves catalog did not Set() it (a never-set gauge reads 0). Use the unique `t2_` prefix so no other test contaminates these series.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestEmitCatalogSideRoster -v`
Expected: FAIL — `undefined: scraperprovider.EmitCatalogSideRoster`.

- [ ] **Step 3: Implement `EmitCatalogSideRoster`**

Create `services/catalog/internal/service/scraperprovider/roster_metrics.go`:

```go
package scraperprovider

import (
	"fmt"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// EmitCatalogSideRoster reflects the catalog-OWNED provider rows
// (scraper_operated = false: ae, kodik, animelib, hanime, raw) into the
// provider_info / provider_enabled management metrics via the shared
// metrics.EmitProviderRoster helper. The scraper emits the rows IT owns
// (scraper_operated = true); the two sets partition the roster with no overlap,
// so there are no duplicate series across Prometheus targets. Call once at catalog
// boot, AFTER the Phase 1 rename/seed/backfill so all rows + the scraper_operated
// flag are present. Idempotent (pure Set()).
func EmitCatalogSideRoster(db *gorm.DB) error {
	var rows []domain.ScraperProvider
	if err := db.Where("scraper_operated = ?", false).Order("name asc").Find(&rows).Error; err != nil {
		return fmt.Errorf("load catalog-side roster: %w", err)
	}
	entries := make([]metrics.RosterEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, metrics.RosterEntry{
			Name:        r.Name,
			Status:      string(r.Status),
			Reason:      r.Reason,
			Description: r.Description,
		})
	}
	metrics.EmitProviderRoster(entries)
	return nil
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestEmitCatalogSideRoster -v`
Expected: PASS.

- [ ] **Step 5: Delete the drifted hardcoded emission from health_checker.go**

In `services/catalog/internal/service/health_checker.go`, the `NewPlayerHealthChecker` constructor currently emits `provider_info`/`provider_enabled` for kodik + 4 hardcoded players (lines 47–74). This is now owned by `EmitCatalogSideRoster` (roster-driven, correct names/status). Delete the emission block so the constructor body becomes:

```go
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	interval time.Duration,
	log *logger.Logger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	// provider_info / provider_enabled for kodik + the other catalog-side players is
	// emitted from the DB roster by scraperprovider.EmitCatalogSideRoster at boot
	// (single-emitter partition). This checker only runs the LIVE Kodik liveness
	// probe (provider_health_up{kodik,liveness} + probe_last_tick).
	return &PlayerHealthChecker{
		kodikClient: kodikClient,
		interval:    interval,
		log:         log,
		prevStatus:  make(map[string]bool),
	}
}
```

Leave everything else in the file (`Start`, `checkAll`, `checkProvider`, `checkKodik`, the `providerKodik`/`kodikStage` consts) UNCHANGED — the live probe still emits `provider_health_up{kodik,liveness}`. The `metrics` import stays (still used by `checkProvider`).

- [ ] **Step 6: Wire `EmitCatalogSideRoster` into catalog boot**

In `services/catalog/cmd/catalog-api/main.go`, immediately AFTER the Phase 1 `scraperprovider.BackfillScraperOperated(db.DB)` block, add:

```go
	// Reflect the catalog-owned provider rows (scraper_operated=false) into the
	// provider_info/provider_enabled management metrics. Runs after the roster is
	// fully migrated/seeded/backfilled so names + flags are authoritative. The
	// scraper emits its own (scraper_operated=true) rows — the two partition the
	// roster with no duplicate series.
	if err := scraperprovider.EmitCatalogSideRoster(db.DB); err != nil {
		log.Errorw("emit catalog-side provider roster metrics failed (continuing)", "error", err)
	}
```

(`scraperprovider` is already imported in main.go from Phase 1.)

- [ ] **Step 7: Build + run the catalog suite (incl. any health_checker test)**

Run: `cd services/catalog && go build ./... && go test ./internal/service/scraperprovider/... ./internal/service/ -count=1`
Expected: PASS. If a pre-existing `health_checker_test.go` asserted the deleted `provider_info`/`provider_enabled` emission, update it to assert via `EmitCatalogSideRoster` instead (those rows now come from the DB roster) — do NOT re-add the hardcoded emission.

- [ ] **Step 8: Commit**

```bash
git commit services/catalog/internal/service/scraperprovider/roster_metrics.go \
  services/catalog/internal/service/scraperprovider/roster_metrics_test.go \
  services/catalog/internal/service/health_checker.go \
  services/catalog/cmd/catalog-api/main.go \
  -m "feat(catalog): emit owned provider roster (scraper_operated=false) from DB

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: Route the scraper's emit through the shared helper

**Files:**
- Modify: `services/scraper/cmd/scraper-api/main.go` (the `provider_info`/`provider_enabled` emit loop, ~lines 641–657)

**Interfaces:**
- Consumes: `metrics.RosterEntry`, `metrics.EmitProviderRoster` (Task 1); `cfg.Providers.Rows([]string) []config.ProviderRow` (existing).

- [ ] **Step 1: Replace the inline emit loop with the shared helper**

In `services/scraper/cmd/scraper-api/main.go`, the current block (ISS-023) iterates `cfg.Providers.Rows(...)` and Sets `ProviderEnabled`/`ProviderInfo` inline. Replace the inline loop body with a build-then-emit through the shared helper (preserving the exact same candidate set — its EN candidates + adult candidates):

```go
	// ISS-023 / roster unification: reflect the scraper-OWNED provider rows
	// (scraper_operated=true: the EN chain + 18anime) into provider_info/
	// provider_enabled via the shared metrics.EmitProviderRoster helper. Disabled
	// providers are not Register()-ed but ARE reflected here so they stay visible in
	// the Grafana management table. Catalog emits the scraper_operated=false rows.
	scraperRows := cfg.Providers.Rows(append(append([]string{}, candidateProviders...), adultCandidates...))
	rosterEntries := make([]metrics.RosterEntry, 0, len(scraperRows))
	for _, row := range scraperRows {
		rosterEntries = append(rosterEntries, metrics.RosterEntry{
			Name:        row.Name,
			Status:      string(row.Status),
			Reason:      row.Reason,
			Description: row.Description,
		})
	}
	metrics.EmitProviderRoster(rosterEntries)
```

This is a behavior-preserving refactor: the same providers are emitted with the same `status`/`reason`/`description`, and `provider_enabled` is still 1 iff `status=="enabled"` (the helper computes it; previously `row.Enabled` did, which is also `status==StatusEnabled`). `metrics` is already imported in scraper main.go.

- [ ] **Step 2: Build the scraper**

Run: `cd services/scraper && go build ./...`
Expected: clean build. (No new unit test — this is a behavior-preserving refactor onto a helper that is unit-tested in Task 1; verification is the build here + the deploy-time metric check in Task 4. If `candidateProviders`/`adultCandidates` are now referenced only here, confirm they are still also used by the registration loop above — they are — so no "declared and not used" error.)

- [ ] **Step 3: Run the scraper config + build sanity**

Run: `cd services/scraper && go test ./internal/config/ -count=1 && go vet ./cmd/scraper-api/`
Expected: PASS / clean.

- [ ] **Step 4: Commit**

```bash
git commit services/scraper/cmd/scraper-api/main.go \
  -m "refactor(scraper): emit provider roster via shared metrics.EmitProviderRoster

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 4: Deploy + verify the roster now covers all 13

**Files:** none (deploy/verify only).

- [ ] **Step 1: Redeploy catalog then scraper (catalog first — it now owns the catalog-side rows)**

Build from a fresh clean `origin/main` worktree (copy `docker/.env`; compose project stays `docker`), not the shared dirty tree.

Run: `make redeploy-catalog && make redeploy-scraper`

- [ ] **Step 2: Verify catalog emits its 5 owned rows (and the stale `allanime-raw` series is gone)**

```bash
curl -s http://localhost:8081/metrics | grep '^provider_info{' | sort
```
Expected: rows for `ae`, `kodik`, `animelib`, `hanime`, `raw` (NOT `allanime-raw`), each with the DB `status` (animelib/hanime now `enabled`, matching the DB — not the old `disabled`). The catalog process restart clears the old `allanime-raw` series.

- [ ] **Step 3: Verify scraper emits its 8 owned rows**

```bash
curl -s http://localhost:8088/metrics | grep '^provider_info{' | sort
```
Expected: `gogoanime, allanime, miruro, animefever, nineanime, animepahe, 18anime` (+ `animekai` only if `SCRAPER_ANIMEKAI_ENABLED`), with correct tri-state status (animepahe/animekai `disabled`, animefever `degraded`).

- [ ] **Step 4: Verify NO provider name is emitted by both targets (partition holds)**

```bash
{ curl -s http://localhost:8081/metrics | grep -oP '^provider_info\{provider="\K[^"]+';
  curl -s http://localhost:8088/metrics | grep -oP '^provider_info\{provider="\K[^"]+'; } | sort | uniq -d
```
Expected: EMPTY output (no provider appears on both targets → no duplicate series).

- [ ] **Step 5: Verify the unified count covers the full roster**

```bash
{ curl -s http://localhost:8081/metrics | grep -oP '^provider_info\{provider="\K[^"]+';
  curl -s http://localhost:8088/metrics | grep -oP '^provider_info\{provider="\K[^"]+'; } | sort -u | wc -l
```
Expected: `13` (or `12` if `SCRAPER_ANIMEKAI_ENABLED=false` and animekai is not in the scraper candidate set — note which). This is the metric-side fix for the dashboard "Providers (Enabled/Total)" panel; the panel rewiring itself is Phase 4.

- [ ] **Step 6: Health check**

Run: `make health`
Expected: all services healthy; EN playback unaffected (no live-path change).

---

## Self-Review (Phase 2)

**Spec coverage (§2 framework slice):**
- Shared status-driven emit helper → Task 1. ✓
- Catalog + scraper iterate the roster instead of a hardcoded list → Task 2 (catalog DB-driven, deletes hardcoded slice) + Task 3 (scraper routes through helper; it was already DB-driven). ✓
- Fold ae into provider_info → Task 2 (ae is a `scraper_operated=false` DB row → emitted by `EmitCatalogSideRoster`). ✓
- Disabled rows stay reflected (roster reflection, not live) → encoded in the helper (emits info for all, enabled=0 for non-enabled) + Global Constraints. ✓
- No duplicate series across targets → partition contract (catalog: false / scraper: true) + Task 4 Step 4 verification. ✓
- ae health probe, parser-latency parity, telemetry tagging, dashboard rewire → explicitly deferred to Phase 3/4 (Out-of-scope notes). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every test step has assertion + run command + expected output. ✓

**Type consistency:** `RosterEntry{Name,Status,Reason,Description}` and `EmitProviderRoster([]RosterEntry)` used identically in Tasks 1, 2, 3. `EmitCatalogSideRoster(db *gorm.DB) error` defined in Task 2, wired in Task 2 Step 6. `cfg.Providers.Rows(...) []ProviderRow` with `.Name/.Status/.Reason/.Description` matches the Phase-1-confirmed `config.ProviderRow` shape. ✓

**One judgment call flagged for review:** Task 1's `EmitProviderRoster` emits `provider_info` for disabled rows too (roster reflection), rather than skipping them. This matches current behavior and keeps the management table complete in Prometheus before Phase 4's Postgres datasource lands — chosen over the spec's stricter "disabled → no metrics" (which there refers to LIVE metrics). Called out in Global Constraints; confirm during spec/plan review.
