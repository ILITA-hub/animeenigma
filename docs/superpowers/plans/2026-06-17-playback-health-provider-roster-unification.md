# Playback-Health Provider Roster Unification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `scraper_providers` table to `stream_providers`, make it the single roster for *every* stream source (ae + EN scraper chain + adult + legacy players) with a new `scraper_operated` flag, so the playback-health dashboard and all metric emitters can drive off one authoritative list.

**Architecture:** Catalog Postgres owns the roster (`stream_providers`). A guarded one-time migration renames the table; the seed is extended with the first-party + legacy player rows; an intrinsic `scraper_operated` flag (mirrors the existing intrinsic `group`) marks scraper-microservice providers. The scraper service consumes the roster over `/internal/scraper/providers` and now filters to `scraper_operated=true` **before** its fail-fast unknown-name validation, so the enlarged roster never pollutes EN failover or breaks scraper boot.

**Tech Stack:** Go 1.x, GORM (Postgres prod / SQLite in tests), Prometheus client_golang, ClickHouse (analytics), Grafana provisioning JSON.

## Global Constraints

- **Effort/impact units:** UXΔ / CDI / MVQ only — never days/hours/sprints (`.planning/CONVENTIONS.md`).
- **Go conventions:** snake_case files, PascalCase exported types, `libs/errors` for domain errors, structured `libs/logger`.
- **Shared dirty tree:** commit path-scoped (`git commit <pathspec>`), never `git add -A`, never `--amend`, `git show --stat HEAD` after each commit. Always `git push` after committing (realtime backup). If push is rejected, cherry-pick onto a fresh `origin/main` worktree.
- **Migrations are insert-if-absent / guarded-idempotent.** GORM `AutoMigrate` only ADDs columns; it never drops/renames — table renames need explicit guarded SQL.
- **`group` and `scraper_operated` are INTRINSIC** (derived from provider name, not operator-editable) — backfilling them onto existing rows is correct.
- **The Go type stays `domain.ScraperProvider`** for this plan; only the physical TABLE is renamed (minimizes blast radius in the shared tree). A Go-identifier rename to `StreamProvider` is an optional later cleanup, out of scope here.

**Spec:** `docs/superpowers/specs/2026-06-17-playback-health-provider-roster-unification-design.md`

---

## Phase Roadmap

This plan delivers **Phase 1 (Foundation)** in full. Phases 2–4 are summarized below and will each be authored as their own detailed plan once Phase 1 has landed and deployed (their exact emit-site code depends on the foundation being live and is best grounded against the then-current tree).

- **Phase 1 — Foundation (THIS PLAN):** rename table → `stream_providers`; add `scraper_operated`; seed all providers; scraper consumer filter. *Deliverable: roster holds every provider, scraper boots green consuming only its own rows, no behavior change for users.*
- **Phase 2 — Status-driven instrumentation framework:** extend `libs/metrics/provider.go` into a roster-driven emit helper gated by `status` (enabled→metrics+alerts, degraded→metrics no-alerts, disabled→nothing); catalog + scraper iterate the roster instead of hardcoded lists; fold `ae` into `provider_info`/health/telemetry.
- **Phase 3 — Per-provider metric parity:** add health probing for catalog-side providers (ae/kodik/animelib/hanime/raw); ensure the scraper emits `parser_request_duration` per EN-chain provider; uniform `provider` tagging on resolve telemetry; retire the bespoke `/api/anime/:id/ae` measurement.
- **Phase 4 — Dashboard + Postgres datasource:** provision a read-only Grafana Postgres datasource; rebuild `playback-health.json` so every panel is roster-keyed; unified status table from the DB roster; template vars from the roster; alerts gated to `enabled`.

---

## Phase 1 File Structure

| File | Responsibility | Change |
|---|---|---|
| `services/catalog/internal/domain/scraper_provider.go` | Roster model | Add `ScraperOperated` field; `TableName()` → `"stream_providers"`; doc updates |
| `services/catalog/internal/service/scraperprovider/migrate.go` | Guarded table-rename + intrinsic backfill helpers | **Create** |
| `services/catalog/internal/service/scraperprovider/seed.go` | Bootstrap roster | Add 5 player rows; intrinsic `scraper_operated`; set flag on insert |
| `services/catalog/internal/service/scraperprovider/migrate_test.go` | Migration tests | **Create** |
| `services/catalog/internal/service/scraperprovider/seed_test.go` | Seed tests | Update count 8→13; add scraper_operated assertions |
| `services/catalog/cmd/catalog-api/main.go` | Boot wiring | Call rename before AutoMigrate; backfill after seed; fix raw-SQL literal |
| `services/scraper/internal/config/providers_remote.go` | Roster consumer | Add `ScraperOperated`; skip non-scraper rows before validation |
| `services/scraper/internal/config/providers_remote_test.go` | Consumer tests | Update unknown-name test; add filter test |

---

### Task 1: Add `scraper_operated` field + rename table to `stream_providers`

**Files:**
- Modify: `services/catalog/internal/domain/scraper_provider.go`
- Create: `services/catalog/internal/service/scraperprovider/migrate.go`
- Create: `services/catalog/internal/service/scraperprovider/migrate_test.go`

**Interfaces:**
- Produces: `domain.ScraperProvider.ScraperOperated bool`; `ScraperProvider.TableName() == "stream_providers"`; `scraperprovider.RenameScraperProvidersTable(db *gorm.DB) error`; `scraperprovider.BackfillScraperOperated(db *gorm.DB) error`.

- [ ] **Step 1: Add the field + rename the table on the model**

In `services/catalog/internal/domain/scraper_provider.go`, add the field after `PreferenceWeight` (line ~49) and change `TableName()`:

```go
	PreferenceWeight int       `json:"preference_weight"`
	// ScraperOperated marks providers operated by the scraper microservice (the
	// EN failover chain + the 18+ orchestrator). Intrinsic (derived from name,
	// NOT operator-editable). The scraper consumes only scraper_operated=true
	// rows, so first-party/legacy players (ae, kodik, animelib, hanime, raw) in
	// this table never enter EN failover. Added 2026-06-17 (roster unification).
	ScraperOperated bool      `json:"scraper_operated"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName pins the physical table. Renamed scraper_providers → stream_providers
// 2026-06-17: the table is now the roster for EVERY stream source (ae + EN chain
// + adult + legacy players), not just scraper EN-providers. The Go type keeps its
// ScraperProvider name (table rename only) to limit blast radius.
func (ScraperProvider) TableName() string { return "stream_providers" }
```

- [ ] **Step 2: Write the failing migration test**

Create `services/catalog/internal/service/scraperprovider/migrate_test.go`:

```go
package scraperprovider_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRenameScraperProvidersTable_RenamesLegacy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Simulate a legacy DB: a physical table literally named scraper_providers.
	if err := db.Exec(`CREATE TABLE scraper_providers (name text primary key, status text)`).Error; err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := db.Exec(`INSERT INTO scraper_providers(name,status) VALUES ('gogoanime','enabled')`).Error; err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	if err := scraperprovider.RenameScraperProvidersTable(db); err != nil {
		t.Fatalf("rename: %v", err)
	}

	m := db.Migrator()
	if m.HasTable("scraper_providers") {
		t.Error("old scraper_providers table still exists after rename")
	}
	if !m.HasTable("stream_providers") {
		t.Fatal("stream_providers table missing after rename")
	}
	var name string
	if err := db.Raw(`SELECT name FROM stream_providers WHERE name='gogoanime'`).Scan(&name).Error; err != nil || name != "gogoanime" {
		t.Errorf("row not preserved through rename: name=%q err=%v", name, err)
	}
}

func TestRenameScraperProvidersTable_IdempotentOnFreshDB(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	// Fresh DB: only the new table exists (AutoMigrate would have made it).
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.RenameScraperProvidersTable(db); err != nil {
		t.Fatalf("rename on fresh DB should be a no-op, got: %v", err)
	}
	if !db.Migrator().HasTable("stream_providers") {
		t.Error("stream_providers missing after no-op rename")
	}
}

func TestBackfillScraperOperated_SetsIntrinsicFlag(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// One scraper provider, one first-party — both default scraper_operated=false.
	db.Create(&domain.ScraperProvider{Name: "gogoanime", Status: domain.StatusEnabled})
	db.Create(&domain.ScraperProvider{Name: "ae", Status: domain.StatusEnabled})

	if err := scraperprovider.BackfillScraperOperated(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var gogo, ae domain.ScraperProvider
	db.First(&gogo, "name = ?", "gogoanime")
	db.First(&ae, "name = ?", "ae")
	if !gogo.ScraperOperated {
		t.Error("gogoanime should be scraper_operated=true")
	}
	if ae.ScraperOperated {
		t.Error("ae should be scraper_operated=false")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run 'Rename|Backfill' -v`
Expected: FAIL — `undefined: scraperprovider.RenameScraperProvidersTable` / `BackfillScraperOperated`.

- [ ] **Step 4: Implement the migration helpers**

Create `services/catalog/internal/service/scraperprovider/migrate.go`:

```go
package scraperprovider

import (
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// RenameScraperProvidersTable renames the legacy scraper_providers table to
// stream_providers exactly once. Guarded: only renames when the old table exists
// and the new one does not, so it is a no-op on fresh DBs and on every boot after
// the rename. Must run BEFORE AutoMigrate(&domain.ScraperProvider{}) so the new
// scraper_operated column is added to the renamed (data-carrying) table rather
// than to a fresh empty stream_providers. Works on SQLite (tests) + Postgres.
func RenameScraperProvidersTable(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable("scraper_providers") && !m.HasTable("stream_providers") {
		if err := db.Exec("ALTER TABLE scraper_providers RENAME TO stream_providers").Error; err != nil {
			return fmt.Errorf("rename scraper_providers -> stream_providers: %w", err)
		}
	}
	return nil
}

// BackfillScraperOperated sets the intrinsic scraper_operated flag on every row.
// Idempotent and safe to run every boot: like Group, the flag is intrinsic (NOT
// operator-editable), so re-deriving it from the canonical name set is always
// correct. Bounded row count (~13).
func BackfillScraperOperated(db *gorm.DB) error {
	names := scraperOperatedNameList()
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name IN ?", names).Update("scraper_operated", true).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=true: %w", err)
	}
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name NOT IN ?", names).Update("scraper_operated", false).Error; err != nil {
		return fmt.Errorf("backfill scraper_operated=false: %w", err)
	}
	return nil
}
```

(`scraperOperatedNameList()` is defined in seed.go in Task 2 — Task 1's helper depends on it; if implementing Task 1 first, add a temporary `func scraperOperatedNameList() []string { return []string{"gogoanime","animepahe","allanime","animefever","miruro","nineanime","animekai","18anime"} }` and move it to seed.go in Task 2.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run 'Rename|Backfill' -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git commit services/catalog/internal/domain/scraper_provider.go \
  services/catalog/internal/service/scraperprovider/migrate.go \
  services/catalog/internal/service/scraperprovider/migrate_test.go \
  -m "feat(catalog): rename scraper_providers->stream_providers + scraper_operated flag

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 2: Seed the first-party + legacy player rows

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go`
- Modify: `services/catalog/internal/service/scraperprovider/seed_test.go`

**Interfaces:**
- Consumes: `domain.ScraperProvider.ScraperOperated` (Task 1).
- Produces: `scraperOperatedNameList() []string`; the 13-row default roster; `SeedDefaults` sets `ScraperOperated` intrinsically on insert.

- [ ] **Step 1: Update the seed test for the enlarged roster**

In `services/catalog/internal/service/scraperprovider/seed_test.go`, update `TestSeedDefaults_InsertsRoster` count and add assertions:

```go
func TestSeedDefaults_InsertsRoster(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 13 {
		t.Fatalf("want 13 default rows, got %d", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" || !all.IsEnabled() {
		t.Errorf("allanime seeded wrong: %+v", all)
	}
	if !all.ScraperOperated {
		t.Error("allanime should be scraper_operated=true")
	}
	var ae domain.ScraperProvider
	db.First(&ae, "name = ?", "ae")
	if ae.Group != "firstparty" || !ae.IsEnabled() || ae.ScraperOperated {
		t.Errorf("ae seeded wrong (want firstparty/enabled/not-scraper-operated): %+v", ae)
	}
	var kodik domain.ScraperProvider
	db.First(&kodik, "name = ?", "kodik")
	if kodik.Group != "ru" || kodik.ScraperOperated {
		t.Errorf("kodik seeded wrong (want ru/not-scraper-operated): %+v", kodik)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestSeedDefaults_InsertsRoster -v`
Expected: FAIL — `want 13 default rows, got 8`.

- [ ] **Step 3: Add the new rows + intrinsic flag to seed.go**

In `services/catalog/internal/service/scraperprovider/seed.go`, append the 5 rows to `defaultProviders` (after the `18anime` entry, before the closing `}`):

```go
	{
		Name: "ae", Status: domain.StatusEnabled,
		Reason: "First-party AnimeEnigma source (survivor)",
		Description: "Self-hosted HLS from the private raw-library MinIO bucket. The " +
			"long-term user-facing player; all other players are being retired (2026-06-17).",
		SupportsSub: true, SupportsRaw: true, SubDelivery: "soft",
		QualityCeiling: "1080p", PreferenceWeight: 100,
	},
	{
		Name: "kodik", Status: domain.StatusEnabled,
		Reason:      "RU iframe player (legacy, slated for retirement)",
		Description: "Kodik iframe embed. Retiring in favor of aePlayer (2026-06-17).",
		SupportsDub: true, SubDelivery: "none", PreferenceWeight: 0,
	},
	{
		Name: "animelib", Status: domain.StatusEnabled,
		Reason:      "RU direct-MP4 player (legacy, slated for retirement)",
		Description: "AniLib direct MP4. Retiring in favor of aePlayer (2026-06-17).",
		SupportsDub: true, SubDelivery: "none",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		Name: "hanime", Status: domain.StatusEnabled,
		Reason:      "18+ HLS player (legacy, slated for retirement)",
		Description: "Hanime HLS. Retiring in favor of aePlayer (2026-06-17).",
		SubDelivery: "none", QualityCeiling: "1080p", PreferenceWeight: 0,
	},
	{
		Name: "raw", Status: domain.StatusEnabled,
		Reason:      "JP original-audio player (AllAnime raw, legacy, slated for retirement)",
		Description: "Raw JP player (AllAnime fast4speed.rsvp + HLS). Retiring in favor of aePlayer (2026-06-17).",
		SupportsSub: true, SupportsRaw: true, SubDelivery: "soft",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
```

Extend `intrinsicGroups` so the new providers get correct intrinsic groups:

```go
var intrinsicGroups = map[string]string{
	"18anime":  "adult",
	"hanime":   "adult",
	"ae":       "firstparty",
	"kodik":    "ru",
	"animelib": "ru",
	"raw":      "jp",
}
```

Add the intrinsic scraper-operated set + list helper (used by `BackfillScraperOperated` in Task 1):

```go
// scraperOperatedNames is the intrinsic set of providers operated by the scraper
// microservice (EN failover chain + 18+ orchestrator). Like Group, it is
// intrinsic — derived from the name, never operator-editable.
var scraperOperatedNames = map[string]bool{
	"gogoanime": true, "animepahe": true, "allanime": true, "animefever": true,
	"miruro": true, "nineanime": true, "animekai": true, "18anime": true,
}

func isScraperOperated(name string) bool { return scraperOperatedNames[name] }

// scraperOperatedNameList returns the intrinsic scraper-operated names as a slice
// (for the backfill UPDATE ... WHERE name IN (...)).
func scraperOperatedNameList() []string {
	out := make([]string, 0, len(scraperOperatedNames))
	for n := range scraperOperatedNames {
		out = append(out, n)
	}
	return out
}
```

In `SeedDefaults`, set the flag on insert — modify the row-prep block (after `row.Group = intrinsicGroup(p.Name)`):

```go
		row := p
		// Group + scraper_operated are intrinsic — always derive from the name.
		row.Group = intrinsicGroup(p.Name)
		row.ScraperOperated = isScraperOperated(p.Name)
		if row.SubDelivery == "" {
			row.SubDelivery = "hard"
		}
```

(If you added a temporary `scraperOperatedNameList` in Task 1's migrate.go, delete it now — the canonical one lives here.)

- [ ] **Step 4: Run the seed + migration tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -v`
Expected: PASS (all seed + migration tests; 13 rows; ae/kodik groups + flags correct).

- [ ] **Step 5: Commit**

```bash
git commit services/catalog/internal/service/scraperprovider/seed.go \
  services/catalog/internal/service/scraperprovider/seed_test.go \
  -m "feat(catalog): seed ae + legacy players into stream_providers roster

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 3: Wire the migrations into catalog boot

**Files:**
- Modify: `services/catalog/cmd/catalog-api/main.go`

**Interfaces:**
- Consumes: `scraperprovider.RenameScraperProvidersTable` (Task 1), `scraperprovider.BackfillScraperOperated` (Task 1), `scraperprovider.SeedDefaults` (existing).

- [ ] **Step 1: Rename the table before AutoMigrate**

In `services/catalog/cmd/catalog-api/main.go`, immediately **before** the `db.AutoMigrate(` block (currently line ~80), insert the guarded rename:

```go
	// One-time migration: rename scraper_providers → stream_providers (roster
	// unification 2026-06-17). MUST run before AutoMigrate so the new
	// scraper_operated column lands on the renamed, data-carrying table rather
	// than a fresh empty one. Guarded + idempotent (no-op on fresh DBs).
	if err := scraperprovider.RenameScraperProvidersTable(db.DB); err != nil {
		log.Fatalw("rename scraper_providers -> stream_providers failed", "error", err)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(
```

- [ ] **Step 2: Fix the legacy raw-SQL literal to the new table name**

In the existing `enabled → status` migration block (currently line ~126), change the hardcoded table name in the raw `UPDATE` so it targets the renamed table:

```go
		if err := db.DB.Exec(
			`UPDATE stream_providers SET status = CASE WHEN enabled THEN 'enabled' ELSE 'disabled' END`,
		).Error; err != nil {
```

(The surrounding `Migrator().HasColumn(&domain.ScraperProvider{}, "enabled")` / `DropColumn` calls resolve the table via the struct's `TableName()`, so they already target `stream_providers` after Task 1 — only the raw literal needed editing.)

- [ ] **Step 3: Backfill the intrinsic flag after seeding**

Immediately **after** the existing `scraperprovider.SeedDefaults(db.DB)` block (line ~145), add:

```go
	// Backfill the intrinsic scraper_operated flag on every roster row (idempotent;
	// the flag mirrors Group — intrinsic, not operator-editable). Ensures pre-
	// existing rows (which AutoMigrate created the column on as default-false) and
	// any operator-touched rows reflect the canonical scraper-operated set.
	if err := scraperprovider.BackfillScraperOperated(db.DB); err != nil {
		log.Errorw("backfill scraper_operated failed (continuing)", "error", err)
	}
```

- [ ] **Step 4: Verify catalog builds**

Run: `cd services/catalog && go build ./... && go vet ./internal/service/scraperprovider/ ./cmd/catalog-api/`
Expected: clean build, no vet errors.

- [ ] **Step 5: Run the full catalog scraperprovider + capability suites (no regressions)**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/... ./internal/service/capability/... -count=1`
Expected: PASS (capability `BuildENFamily` unaffected — it filters `status <> 'disabled' AND "group"='en'`, which the new firstparty/ru/jp rows don't match).

- [ ] **Step 6: Commit**

```bash
git commit services/catalog/cmd/catalog-api/main.go \
  -m "feat(catalog): wire stream_providers rename + scraper_operated backfill at boot

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 4: Scraper consumer filters `scraper_operated=true`

**Files:**
- Modify: `services/scraper/internal/config/providers_remote.go`
- Modify: `services/scraper/internal/config/providers_remote_test.go`

**Interfaces:**
- Consumes: the `scraper_operated` field now present in `/internal/scraper/providers` JSON (Tasks 1–3).
- Produces: `LoadProvidersRemote` ignoring non-scraper rows, so EN failover roster is unchanged by the enlarged table.

- [ ] **Step 1: Write the failing filter test + fix the unknown-name test**

In `services/scraper/internal/config/providers_remote_test.go`, add a new test and update the existing `TestLoadProvidersRemote_RejectsUnknownProvider` (its `bogus` row must be marked `scraper_operated:true`, otherwise the new filter would correctly skip it and no error would fire):

```go
func TestLoadProvidersRemote_SkipsNonScraperRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Roster now holds first-party/legacy rows (scraper_operated:false) that
		// are NOT in KnownProviders. They must be silently skipped, not rejected.
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"gogoanime","status":"enabled","group":"en","scraper_operated":true,"supports_sub":true},
			{"name":"ae","status":"enabled","group":"firstparty","scraper_operated":false},
			{"name":"kodik","status":"enabled","group":"ru","scraper_operated":false}
		]}}`))
	}))
	defer srv.Close()

	pc, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err != nil {
		t.Fatalf("LoadProvidersRemote should skip non-scraper rows, got: %v", err)
	}
	if !pc.IsEnabled("gogoanime") {
		t.Error("gogoanime should be present + enabled")
	}
	if _, ok := pc.load()["ae"]; ok {
		t.Error("ae (scraper_operated=false) must not enter the scraper roster")
	}
	if _, ok := pc.load()["kodik"]; ok {
		t.Error("kodik (scraper_operated=false) must not enter the scraper roster")
	}
}
```

Update the existing unknown-name test's payload (around line 44) to keep a *scraper-operated* unknown name so rejection still fires:

```go
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[{"name":"bogus","status":"enabled","scraper_operated":true}]}}`))
```

- [ ] **Step 2: Run the tests to verify the new one fails**

Run: `cd services/scraper && go test ./internal/config/ -run 'SkipsNonScraperRows|RejectsUnknownProvider' -v`
Expected: `SkipsNonScraperRows` FAILs — `ae` is rejected as an unknown provider (error non-nil) because the filter doesn't exist yet.

- [ ] **Step 3: Add the field + filter in providers_remote.go**

In `services/scraper/internal/config/providers_remote.go`, add the field to `remoteProvider` (after `Description`, line ~23):

```go
	Description      string `json:"description"`
	ScraperOperated  bool   `json:"scraper_operated"`
	SupportsSub      bool   `json:"supports_sub"`
```

Then, in the decode loop in `LoadProvidersRemote`, skip non-scraper rows **before** the empty-name and known-name checks (the loop currently starts at line ~87):

```go
	for _, p := range rr.Data.Providers {
		// The stream_providers roster holds EVERY stream source (ae + legacy
		// players + EN chain). The scraper operates ONLY scraper_operated rows;
		// first-party/legacy rows are skipped here so they never enter EN
		// failover and never trip the KnownProviders fail-fast below.
		if !p.ScraperOperated {
			continue
		}
		if p.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("provider config: entry with empty name")
		}
		if !known[p.Name] {
			return ProvidersConfig{}, fmt.Errorf("provider config: unknown provider %q", p.Name)
		}
```

- [ ] **Step 4: Run the config suite to verify it passes**

Run: `cd services/scraper && go test ./internal/config/ -count=1`
Expected: PASS (new filter test passes; existing `ParsesAndBuilds` still passes because those rows have no `scraper_operated` field → wait: see Step 5).

- [ ] **Step 5: Fix any pre-existing tests whose rows now lack `scraper_operated`**

The filter skips rows where `scraper_operated` is absent/false. Existing tests like `TestLoadProvidersRemote_ParsesAndBuilds` (allanime/animepahe rows without the field) would now skip both rows and assert on absent providers. Add `"scraper_operated":true` to every provider row in the existing remote-config test payloads (`providers_remote_test.go`, `providers_refresh_test.go` if it builds remote payloads), then re-run:

Run: `cd services/scraper && go test ./internal/config/ -count=1 -v`
Expected: PASS across the whole config package.

- [ ] **Step 6: Commit**

```bash
git commit services/scraper/internal/config/providers_remote.go \
  services/scraper/internal/config/providers_remote_test.go \
  -m "feat(scraper): consume only scraper_operated rows from stream_providers roster

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
git push
```

---

### Task 5: Deploy + live boot verification

**Files:** none (deploy/verify only).

- [ ] **Step 1: Redeploy catalog then scraper (order matters — catalog owns the migration)**

Per [[feedback_deploy_from_clean_worktree]]: build from a fresh clean `origin/main` worktree (copy `docker/.env`, compose project stays `docker`), not the shared dirty tree.

Run: `make redeploy-catalog && make redeploy-scraper`

- [ ] **Step 2: Verify the table renamed + holds all providers**

```bash
docker compose -p docker exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" \
  -c "SELECT name, status, \"group\", scraper_operated FROM stream_providers ORDER BY scraper_operated DESC, name;"
```
Expected: 13 rows; `scraper_providers` no longer exists; 8 scraper rows `scraper_operated=t`, 5 (ae/kodik/animelib/hanime/raw) `=f`; ae group=firstparty.

- [ ] **Step 3: Verify scraper booted green consuming only its rows**

Run: `make logs-scraper | grep -iE "provider|roster|registered" | tail -30`
Expected: scraper registered exactly its EN chain + 18anime; no "unknown provider" fatal; ae/kodik never registered as EN providers.

- [ ] **Step 4: Verify the internal roster endpoint returns the enlarged set**

```bash
docker compose -p docker exec -T scraper wget -qO- http://catalog:8081/internal/scraper/providers | head -c 800
```
Expected: JSON envelope listing all 13 providers, each with a `scraper_operated` boolean.

- [ ] **Step 5: Confirm no user-facing playback regression**

Run: `make health`
Expected: all services healthy. EN playback path unchanged (failover roster identical to before — the new rows are filtered out scraper-side).

---

## Self-Review (Phase 1)

**Spec coverage (Phase 1 scope only):**
- Rename `scraper_providers` → `stream_providers` → Task 1 + Task 3. ✓
- `scraper_operated bool` column + intrinsic semantics → Task 1 (field/backfill) + Task 2 (seed). ✓
- Roster holds ALL providers (ae + legacy players) → Task 2. ✓
- Scraper consumes only `scraper_operated=true` before `KnownProviders` validation (the critical correctness point) → Task 4. ✓
- Lifecycle via `status`, retire = disabled (not delete) → preserved (model unchanged; new rows seeded `enabled`). ✓
- `BuildENFamily` unaffected by rename → verified in Task 3 Step 5. ✓
- Phases 2–4 (metrics framework, parity, dashboard) → explicitly deferred to their own just-in-time plans (Roadmap section). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every test step shows the assertion and the run command + expected output. ✓

**Type consistency:** `RenameScraperProvidersTable` / `BackfillScraperOperated` / `scraperOperatedNameList` / `isScraperOperated` / `ScraperOperated` field / `TableName()=="stream_providers"` used identically across Tasks 1–4. The one cross-task dependency (`scraperOperatedNameList` defined in seed.go, used by migrate.go) is called out explicitly in Task 1 Step 4 and Task 2 Step 3. ✓
