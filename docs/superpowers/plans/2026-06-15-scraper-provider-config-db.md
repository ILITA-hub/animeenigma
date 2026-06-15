# Scraper Provider-Config → DB (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the scraper provider config (currently `docker/scraper-providers.yaml`) into catalog's Postgres as the runtime source of truth — with capability trait columns added — so the scraper fetches it over an internal API at boot + on a refresh interval, falling back to the bundled YAML when catalog is unreachable.

**Architecture:** The existing `docker/scraper-providers.yaml` is *populated* with capability traits (it stays the human-editable seed + offline fallback). Catalog gains a `scraper_providers` table (GORM AutoMigrate), a boot-time idempotent seeder that reads the mounted YAML, and a Docker-network-only `GET /internal/scraper/providers` endpoint. The scraper learns to load its `ProvidersConfig` from that endpoint (YAML fallback) and refresh it periodically; `ProvidersConfig` is made concurrency-safe so the refresher can swap it under live reads.

**Tech Stack:** Go, chi router, GORM (`libs/database`), `gopkg.in/yaml.v3`, `net/http` + `net/http/httptest`, sqlite (`gorm.io/driver/sqlite`) for schema/seed unit tests (existing pattern in `services/catalog/internal/domain/anime_attributes_test.go`).

**Convention:** every `git commit` in this plan MUST include the standard co-author trailer:
```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
```
Commit only the path(s) each task touches (`git add <paths>`), never `git add -A` — this is a shared working tree with concurrent committers.

**This phase is scoped to P1 of `docs/superpowers/specs/2026-06-15-scraper-capability-api-design.md`.** P2 (per-provider category/quality parsing), P3 (scraper `/capabilities` join endpoint), and P4 (catalog assembled+ranked `/api/anime/{uuid}/capabilities`) are separate plans written after P1 lands.

---

## File Structure

**Modify:**
- `docker/scraper-providers.yaml` — populate each entry with trait fields (Task 1).
- `services/scraper/internal/config/providers.go` — trait fields on `providerEntry` + `ProviderMeta`; concurrency-safe `ProvidersConfig`; remote loader (Tasks 2, 7, 8).
- `services/scraper/internal/config/config.go` — `CATALOG_URL` + `SCRAPER_PROVIDERS_REFRESH` env (Task 6 wiring used by Tasks 7–8).
- `services/scraper/cmd/scraper-api/main.go` — prefer remote config, start refresher (Tasks 7, 8).
- `services/catalog/cmd/catalog-api/main.go` — AutoMigrate the new model, run seeder, wire internal handler (Tasks 3, 4, 5).
- `services/catalog/internal/transport/router.go` — register `/internal/scraper/providers` (Task 5).
- `docker/docker-compose.yml` — mount YAML into catalog; scraper env (Task 6).

**Create:**
- `services/catalog/internal/domain/scraper_provider.go` — GORM model (Task 3).
- `services/catalog/internal/domain/scraper_provider_test.go` — schema test (Task 3).
- `services/catalog/internal/service/scraperprovider/seed.go` — YAML→DB seeder (Task 4).
- `services/catalog/internal/service/scraperprovider/seed_test.go` — seeder test (Task 4).
- `services/catalog/internal/handler/internal_scraper_providers.go` — internal handler (Task 5).
- `services/catalog/internal/handler/internal_scraper_providers_test.go` — handler test (Task 5).
- `services/scraper/internal/config/providers_remote.go` — HTTP loader (Task 7).
- `services/scraper/internal/config/providers_remote_test.go` — loader test (Task 7).
- `services/scraper/internal/config/providers_refresh.go` — refresher goroutine (Task 8).
- `services/scraper/internal/config/providers_refresh_test.go` — refresher test (Task 8).

---

## Task 1: Populate `scraper-providers.yaml` with trait fields

**Files:**
- Modify: `docker/scraper-providers.yaml` (the `providers:` list only — keep the header comment block intact)

- [ ] **Step 1: Replace the `providers:` list** with a normalized block-style list that adds the trait fields. Leave every comment line ABOVE `providers:` unchanged. Replace from the line `providers:` to the end of file with:

```yaml
providers:
  - name: allanime
    enabled: true
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 90

  - name: gogoanime
    enabled: true
    reason: "Revived via gogoanimes.fi mirror + megaplay"
    description: >
      anitaku.to migrated to anineko.to ("We Have Moved"). Repointed
      SCRAPER_GOGOANIME_BASE_URL to gogoanimes.fi (classic gogo HTML:
      anime_muti_link + /search.html), whose newplayer.php embed nests the
      megaplay.buzz player — now routed through the megaplay extractor
      (gogoanime.me.uk added to its wrapper allowlist). Re-enabled 2026-06-05.
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 85

  - name: miruro
    enabled: true
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 70

  - name: animefever
    enabled: true
    supports_sub: true
    supports_dub: false
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 60

  - name: nineanime
    enabled: true
    supports_sub: true
    supports_dub: false
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 720p
    preference_weight: 40

  - name: animepahe
    enabled: false
    reason: "Cloudflare challenge"
    description: >
      animepahe.pw migrated DDoS-Guard -> Cloudflare managed challenge; the
      stealth-Chromium sidecar can't solve it (0% solve rate). See ISS-023.
      Disabled 2026-06-03.
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 30

  - name: animekai
    enabled: false
    reason: "Stub — ListServers unimplemented (SCRAPER-KAI-03)"
    description: >
      animekai provider is a stub; ListServers returns ErrProviderDown.
      Disabled until implemented so it never wastes a failover slot.
    supports_sub: true
    supports_dub: false
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 0

  - name: 18anime
    enabled: true
    group: adult
    reason: "18+ provider (separate group)"
    description: >
      18anime.me hentai source for the 18+ player. Runs in its own orchestrator
      on /anime18/* — NEVER part of the EN (OurEnglish) failover chain.
    supports_sub: true
    supports_dub: false
    supports_raw: true
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 0
```

> Note: `animekai` is now listed explicitly as `enabled: false` (it was previously unlisted → defaulted enabled, but its `ListServers` is a stub). This removes a guaranteed-failing failover slot.

- [ ] **Step 2: Validate the YAML parses** (sanity, before code changes):

Run: `cd /data/animeenigma && python3 -c "import yaml,sys; d=yaml.safe_load(open('docker/scraper-providers.yaml')); print(len(d['providers']),'providers'); print([p['name'] for p in d['providers']])"`
Expected: `8 providers` and the list `['allanime','gogoanime','miruro','animefever','nineanime','animepahe','animekai','18anime']`

- [ ] **Step 3: Commit**

```bash
git add docker/scraper-providers.yaml
git commit -m "feat(scraper): populate provider YAML with capability traits

Adds supports_sub/dub/raw, sub_delivery, quality_ceiling, preference_weight
per provider; lists animekai explicitly disabled (stub). Seed source for the
DB migration. Spec: docs/superpowers/specs/2026-06-15-scraper-capability-api-design.md

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Parse trait fields in the scraper YAML loader

**Files:**
- Modify: `services/scraper/internal/config/providers.go` (`providerEntry`, `ProviderMeta`, `LoadProviders`)
- Test: `services/scraper/internal/config/providers_traits_test.go` (Create)

- [ ] **Step 1: Write the failing test**

Create `services/scraper/internal/config/providers_traits_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProviders_ParsesTraits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	yaml := `providers:
  - name: allanime
    enabled: true
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 90
  - name: nineanime
    enabled: true
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	pc, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	all := pc.Meta("allanime")
	if !all.SupportsSub || !all.SupportsDub || all.SupportsRaw {
		t.Errorf("allanime sub/dub/raw = %v/%v/%v, want true/true/false", all.SupportsSub, all.SupportsDub, all.SupportsRaw)
	}
	if all.SubDelivery != "hard" || all.QualityCeiling != "1080p" || all.PreferenceWeight != 90 {
		t.Errorf("allanime traits = %q/%q/%d", all.SubDelivery, all.QualityCeiling, all.PreferenceWeight)
	}
	// Omitted traits default: bools false, sub_delivery "hard".
	nine := pc.Meta("nineanime")
	if nine.SupportsSub || nine.SubDelivery != "hard" {
		t.Errorf("nineanime defaults = sub %v delivery %q, want false/hard", nine.SupportsSub, nine.SubDelivery)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestLoadProviders_ParsesTraits -v`
Expected: FAIL — `all.SupportsSub undefined (type ProviderMeta has no field SupportsSub)`

- [ ] **Step 3: Add trait fields to `providerEntry`** in `providers.go` (inside the `type providerEntry struct`), after the `Group` field:

```go
	SupportsSub      *bool  `yaml:"supports_sub"`
	SupportsDub      *bool  `yaml:"supports_dub"`
	SupportsRaw      *bool  `yaml:"supports_raw"`
	SubDelivery      string `yaml:"sub_delivery"`
	QualityCeiling   string `yaml:"quality_ceiling"`
	PreferenceWeight *int   `yaml:"preference_weight"`
```

- [ ] **Step 4: Add trait fields to `ProviderMeta`** in `providers.go` (inside the `type ProviderMeta struct`), after the `Group` field:

```go
	SupportsSub      bool
	SupportsDub      bool
	SupportsRaw      bool
	SubDelivery      string // "soft" | "hard" | "none" (default "hard")
	QualityCeiling   string
	PreferenceWeight int
```

- [ ] **Step 5: Map the traits in `LoadProviders`** — in the `metas[e.Name] = ProviderMeta{...}` literal at the end of the loop, add a small helper above the loop and the fields. Replace the existing `metas[e.Name] = ProviderMeta{...}` block with:

```go
		deref := func(p *bool) bool { return p != nil && *p }
		subDelivery := e.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		weight := 0
		if e.PreferenceWeight != nil {
			weight = *e.PreferenceWeight
		}
		metas[e.Name] = ProviderMeta{
			Name:             e.Name,
			Enabled:          *e.Enabled,
			Reason:           e.Reason,
			Description:      e.Description,
			Group:            GroupOf(e.Name),
			SupportsSub:      deref(e.SupportsSub),
			SupportsDub:      deref(e.SupportsDub),
			SupportsRaw:      deref(e.SupportsRaw),
			SubDelivery:      subDelivery,
			QualityCeiling:   e.QualityCeiling,
			PreferenceWeight: weight,
		}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestLoadProviders_ParsesTraits -v`
Expected: PASS

- [ ] **Step 7: Run the full config package + vet**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ && go vet ./internal/config/`
Expected: ok (no failures)

- [ ] **Step 8: Commit**

```bash
git add services/scraper/internal/config/providers.go services/scraper/internal/config/providers_traits_test.go
git commit -m "feat(scraper): parse capability traits in provider config loader

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Catalog `ScraperProvider` GORM model + AutoMigrate

**Files:**
- Create: `services/catalog/internal/domain/scraper_provider.go`
- Create: `services/catalog/internal/domain/scraper_provider_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (AutoMigrate list, ~line 78)

- [ ] **Step 1: Write the failing schema test**

Create `services/catalog/internal/domain/scraper_provider_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScraperProviderSchema_AutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if !db.Migrator().HasTable("scraper_providers") {
		t.Fatal("scraper_providers table not created")
	}
	for _, col := range []string{"name", "enabled", "group", "supports_sub", "supports_dub", "supports_raw", "sub_delivery", "quality_ceiling", "preference_weight"} {
		if !db.Migrator().HasColumn(&domain.ScraperProvider{}, col) {
			t.Errorf("missing column %q", col)
		}
	}
	// Round-trip an insert keyed by Name (primary key).
	row := domain.ScraperProvider{Name: "allanime", Enabled: true, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 90}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got domain.ScraperProvider
	if err := db.First(&got, "name = ?", "allanime").Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.PreferenceWeight != 90 || got.SubDelivery != "hard" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/domain/ -run TestScraperProviderSchema_AutoMigrate -v`
Expected: FAIL — `undefined: domain.ScraperProvider`

- [ ] **Step 3: Create the model**

Create `services/catalog/internal/domain/scraper_provider.go`:

```go
package domain

import "time"

// ScraperProvider is the DB-backed source of truth for scraper EN-provider
// management + capability traits (migrated from docker/scraper-providers.yaml,
// spec 2026-06-15-scraper-capability-api). The scraper service fetches these
// rows via GET /internal/scraper/providers at boot + on a refresh interval;
// the YAML remains the seed + offline fallback. Maintained in the DB (no admin
// UI this phase — edited via SQL/migration).
type ScraperProvider struct {
	// Name is the canonical provider id (gogoanime, animepahe, …). Primary key.
	Name string `gorm:"primaryKey;size:32" json:"name"`
	// Enabled controls failover participation.
	Enabled bool `json:"enabled"`
	// Group is intrinsic: "en" (default) or "adult". `group` is a reserved word
	// in some SQL dialects — keep the column name quoted-safe via the tag.
	Group string `gorm:"column:group;size:16;default:en" json:"group"`
	// Reason is a short dashboard label; Description is the full why.
	Reason      string `json:"reason"`
	Description string `json:"description"`
	// Capability traits (curated; refined per-title by live discovery in P2).
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `gorm:"size:8;default:hard" json:"sub_delivery"` // soft|hard|none
	QualityCeiling   string `gorm:"size:8" json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName pins the table name (GORM would pluralize to "scraper_providers"
// anyway, but make it explicit for the internal endpoint contract).
func (ScraperProvider) TableName() string { return "scraper_providers" }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/domain/ -run TestScraperProviderSchema_AutoMigrate -v`
Expected: PASS

- [ ] **Step 5: Register the model in AutoMigrate** — in `services/catalog/cmd/catalog-api/main.go`, inside the `db.AutoMigrate(...)` call (~line 78), add `&domain.ScraperProvider{},` after `&domain.CollectionItem{},`:

```go
		&domain.Collection{},
		&domain.CollectionItem{},
		// Scraper provider config + capability traits (spec 2026-06-15).
		&domain.ScraperProvider{},
```

- [ ] **Step 6: Build the catalog binary** to confirm wiring compiles:

Run: `cd /data/animeenigma/services/catalog && go build ./... && go vet ./internal/domain/`
Expected: builds clean

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/domain/scraper_provider.go services/catalog/internal/domain/scraper_provider_test.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): scraper_providers model + automigrate

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Catalog YAML→DB seeder (idempotent, insert-if-absent)

**Files:**
- Create: `services/catalog/internal/service/scraperprovider/seed.go`
- Create: `services/catalog/internal/service/scraperprovider/seed_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/scraperprovider/seed_test.go`:

```go
package scraperprovider_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func writeYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "providers.yaml")
	y := `providers:
  - name: allanime
    enabled: true
    supports_sub: true
    supports_dub: true
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 90
  - name: 18anime
    enabled: true
    group: adult
    supports_raw: true
    preference_weight: 0
`
	if err := os.WriteFile(p, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSeedFromYAML_InsertsRows(t *testing.T) {
	db := newDB(t)
	if err := scraperprovider.SeedFromYAML(db, writeYAML(t)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 2 {
		t.Fatalf("want 2 rows, got %d", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" {
		t.Errorf("allanime seeded wrong: %+v", all)
	}
}

func TestSeedFromYAML_IdempotentDoesNotOverwrite(t *testing.T) {
	db := newDB(t)
	path := writeYAML(t)
	if err := scraperprovider.SeedFromYAML(db, path); err != nil {
		t.Fatalf("seed1: %v", err)
	}
	// Operator edits a row in the DB.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime").Update("enabled", false)
	// Re-seed; existing row must NOT be clobbered, no duplicate rows.
	if err := scraperprovider.SeedFromYAML(db, path); err != nil {
		t.Fatalf("seed2: %v", err)
	}
	var count int64
	db.Model(&domain.ScraperProvider{}).Count(&count)
	if count != 2 {
		t.Fatalf("re-seed created duplicates: %d rows", count)
	}
	var all domain.ScraperProvider
	db.First(&all, "name = ?", "allanime")
	if all.Enabled {
		t.Error("re-seed overwrote operator edit (enabled flipped back to true)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/service/scraperprovider/ -v`
Expected: FAIL — package/`SeedFromYAML` undefined

- [ ] **Step 3: Create the seeder**

Create `services/catalog/internal/service/scraperprovider/seed.go`:

```go
// Package scraperprovider seeds the scraper_providers table from the bundled
// docker/scraper-providers.yaml. Insert-if-absent only: a row that already
// exists is never overwritten, so operator edits in the DB survive re-seeding.
// (Catalog cannot import services/scraper/internal/* — Go internal rule — so a
// small local YAML shape is defined here rather than reused.)
package scraperprovider

import (
	"fmt"
	"os"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type seedEntry struct {
	Name             string  `yaml:"name"`
	Enabled          *bool   `yaml:"enabled"`
	Group            string  `yaml:"group"`
	Reason           string  `yaml:"reason"`
	Description      string  `yaml:"description"`
	SupportsSub      *bool   `yaml:"supports_sub"`
	SupportsDub      *bool   `yaml:"supports_dub"`
	SupportsRaw      *bool   `yaml:"supports_raw"`
	SubDelivery      string  `yaml:"sub_delivery"`
	QualityCeiling   string  `yaml:"quality_ceiling"`
	PreferenceWeight *int    `yaml:"preference_weight"`
}

type seedFile struct {
	Providers []seedEntry `yaml:"providers"`
}

func deref(p *bool) bool { return p != nil && *p }

// SeedFromYAML reads path and inserts any provider rows not already present.
// Returns nil (no-op) if path is empty so a missing seed file never blocks boot.
func SeedFromYAML(db *gorm.DB, path string) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read scraper providers seed %q: %w", path, err)
	}
	var sf seedFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		return fmt.Errorf("parse scraper providers seed: %w", err)
	}
	for _, e := range sf.Providers {
		if e.Name == "" {
			continue
		}
		var count int64
		if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", e.Name).Count(&count).Error; err != nil {
			return fmt.Errorf("count %q: %w", e.Name, err)
		}
		if count > 0 {
			continue // insert-if-absent: never overwrite an existing row
		}
		group := e.Group
		if group == "" {
			group = "en"
		}
		subDelivery := e.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		weight := 0
		if e.PreferenceWeight != nil {
			weight = *e.PreferenceWeight
		}
		row := domain.ScraperProvider{
			Name:             e.Name,
			Enabled:          enabled,
			Group:            group,
			Reason:           e.Reason,
			Description:      e.Description,
			SupportsSub:      deref(e.SupportsSub),
			SupportsDub:      deref(e.SupportsDub),
			SupportsRaw:      deref(e.SupportsRaw),
			SubDelivery:      subDelivery,
			QualityCeiling:   e.QualityCeiling,
			PreferenceWeight: weight,
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("create %q: %w", e.Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/service/scraperprovider/ -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/scraperprovider/
git commit -m "feat(catalog): idempotent YAML->DB seeder for scraper_providers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Catalog internal endpoint `GET /internal/scraper/providers`

**Files:**
- Create: `services/catalog/internal/handler/internal_scraper_providers.go`
- Create: `services/catalog/internal/handler/internal_scraper_providers_test.go`
- Modify: `services/catalog/internal/transport/router.go` (register route)
- Modify: `services/catalog/cmd/catalog-api/main.go` (construct handler + pass to router)

- [ ] **Step 1: Write the failing handler test**

Create `services/catalog/internal/handler/internal_scraper_providers_test.go`:

```go
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInternalScraperProviders_List(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&domain.ScraperProvider{Name: "nineanime", Enabled: true, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 40})
	db.Create(&domain.ScraperProvider{Name: "allanime", Enabled: true, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", PreferenceWeight: 90})

	h := handler.NewInternalScraperProvidersHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/internal/scraper/providers", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Providers []domain.ScraperProvider `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(body.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d", len(body.Providers))
	}
	// Ordered by name asc → allanime first.
	if body.Providers[0].Name != "allanime" || body.Providers[1].Name != "nineanime" {
		t.Errorf("order = %s,%s want allanime,nineanime", body.Providers[0].Name, body.Providers[1].Name)
	}
}
```

> If the catalog handler package wraps responses in a `{success,data}` envelope via `httputil.OK`, adjust the decode struct to match (`body.Data.Providers`). Check an existing internal handler (`internal_episodes.go`) for the exact response helper before Step 3 and mirror it; the test's decode struct must match what Step 3 writes.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/handler/ -run TestInternalScraperProviders_List -v`
Expected: FAIL — `undefined: handler.NewInternalScraperProvidersHandler`

- [ ] **Step 3: Create the handler**

First check the response helper used by sibling internal handlers:
Run: `grep -nE "httputil\.(OK|JSON|WriteJSON)" services/catalog/internal/handler/internal_episodes.go`

Create `services/catalog/internal/handler/internal_scraper_providers.go` (uses `httputil.OK`; if Step-1 note showed an envelope, the test already matches it):

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// InternalScraperProvidersHandler serves the scraper provider config + traits
// to the scraper service over the Docker network. Mounted OUTSIDE /api with no
// auth — the gateway does not proxy /internal/*.
type InternalScraperProvidersHandler struct {
	db *gorm.DB
}

func NewInternalScraperProvidersHandler(db *gorm.DB) *InternalScraperProvidersHandler {
	return &InternalScraperProvidersHandler{db: db}
}

// List returns all provider rows ordered by name.
func (h *InternalScraperProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Order("name asc").Find(&rows).Error; err != nil {
		httputil.Error(w, http.StatusInternalServerError, "db_error", "failed to load scraper providers")
		return
	}
	httputil.OK(w, map[string]any{"providers": rows})
}
```

> Match `httputil.Error`/`httputil.OK` to the actual signatures in `libs/httputil` (grep them first — `grep -nE "func (OK|Error)\(" libs/httputil/*.go`). Adjust the call and the test's decode shape together so they agree.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/handler/ -run TestInternalScraperProviders_List -v`
Expected: PASS

- [ ] **Step 5: Register the route** — in `services/catalog/internal/transport/router.go`, after the `internalEpisodesValidateHandler` block (~line 82), add a parameter + route. First add the handler to the router constructor's signature (find `func NewRouter(` and add `internalScraperProvidersHandler *handler.InternalScraperProvidersHandler` to the parameter list), then register:

```go
	// Scraper provider config + capability traits (spec 2026-06-15).
	// Same gateway-non-routing security model as the internal endpoints above.
	if internalScraperProvidersHandler != nil {
		r.Get("/internal/scraper/providers", internalScraperProvidersHandler.List)
	}
```

- [ ] **Step 6: Wire it in main.go** — in `services/catalog/cmd/catalog-api/main.go`, after `db.AutoMigrate` succeeds, construct the handler and pass it into the `NewRouter(...)` call:

```go
	internalScraperProvidersHandler := handler.NewInternalScraperProvidersHandler(db.DB)
```

Add `internalScraperProvidersHandler` to the `transport.NewRouter(...)` argument list at the matching position you added in Step 5.

- [ ] **Step 7: Build catalog**

Run: `cd /data/animeenigma/services/catalog && go build ./... && go test ./internal/handler/ ./internal/transport/`
Expected: builds clean, tests pass

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/handler/internal_scraper_providers.go services/catalog/internal/handler/internal_scraper_providers_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): GET /internal/scraper/providers endpoint

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Wire the seeder at boot + compose env/mounts

**Files:**
- Modify: `services/catalog/cmd/catalog-api/main.go` (call seeder after AutoMigrate)
- Modify: `docker/docker-compose.yml` (catalog: mount YAML + seed env; scraper: CATALOG_URL + refresh env)

- [ ] **Step 1: Call the seeder at boot** — in `services/catalog/cmd/catalog-api/main.go`, immediately after the AutoMigrate / SetupJoinTable block succeeds (and after the handler import is present), add:

```go
	// Seed scraper_providers from the bundled YAML (insert-if-absent; the DB is
	// the runtime source of truth, YAML is seed + scraper offline fallback).
	if seedPath := os.Getenv("SCRAPER_PROVIDERS_SEED_FILE"); seedPath != "" {
		if err := scraperprovider.SeedFromYAML(db.DB, seedPath); err != nil {
			log.Errorw("scraper provider seed failed (continuing)", "error", err, "path", seedPath)
		} else {
			log.Infow("scraper provider seed applied", "path", seedPath)
		}
	}
```

Add the import `"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"` to the import block. Ensure `os` is imported (it usually is).

- [ ] **Step 2: Build catalog**

Run: `cd /data/animeenigma/services/catalog && go build ./...`
Expected: builds clean

- [ ] **Step 3: Mount the YAML into catalog + add env** — in `docker/docker-compose.yml`, under the `catalog:` service, add a read-only mount and the seed-path env. Find the `catalog:` service `volumes:` (add one if absent) and `environment:` blocks:

```yaml
    volumes:
      - ./scraper-providers.yaml:/config/scraper-providers.yaml:ro
    environment:
      SCRAPER_PROVIDERS_SEED_FILE: /config/scraper-providers.yaml
```

(Merge into the existing `volumes:`/`environment:` blocks — do not create duplicate keys.)

- [ ] **Step 4: Add scraper env** — under the `scraper:` service `environment:` block in `docker/docker-compose.yml`, add:

```yaml
      CATALOG_URL: http://catalog:8081
      SCRAPER_PROVIDERS_REFRESH: 60s
```

- [ ] **Step 5: Validate compose** parses:

Run: `cd /data/animeenigma && docker compose -f docker/docker-compose.yml config >/dev/null && echo COMPOSE_OK`
Expected: `COMPOSE_OK`

- [ ] **Step 6: Commit**

```bash
git add services/catalog/cmd/catalog-api/main.go docker/docker-compose.yml
git commit -m "feat(catalog): seed scraper_providers at boot; compose mounts+env

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: Scraper remote config loader (catalog → ProvidersConfig, YAML fallback)

**Files:**
- Create: `services/scraper/internal/config/providers_remote.go`
- Create: `services/scraper/internal/config/providers_remote_test.go`
- Modify: `services/scraper/internal/config/config.go` (read `CATALOG_URL`; keep `SCRAPER_PROVIDERS_FILE` as fallback)
- Modify: `services/scraper/cmd/scraper-api/main.go` (prefer remote at boot)

- [ ] **Step 1: Write the failing test**

Create `services/scraper/internal/config/providers_remote_test.go`:

```go
package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoadProvidersRemote_ParsesAndBuilds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/scraper/providers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":[
			{"name":"allanime","enabled":true,"group":"en","supports_sub":true,"supports_dub":true,"sub_delivery":"hard","quality_ceiling":"1080p","preference_weight":90},
			{"name":"animepahe","enabled":false,"group":"en","supports_sub":true,"supports_dub":true,"sub_delivery":"hard","preference_weight":30}
		]}`))
	}))
	defer srv.Close()

	pc, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err != nil {
		t.Fatalf("LoadProvidersRemote: %v", err)
	}
	if pc.Source != "remote" {
		t.Errorf("Source = %q, want remote", pc.Source)
	}
	if !pc.IsEnabled("allanime") || pc.IsEnabled("animepahe") {
		t.Errorf("enabled: allanime=%v animepahe=%v want true/false", pc.IsEnabled("allanime"), pc.IsEnabled("animepahe"))
	}
	all := pc.Meta("allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" {
		t.Errorf("allanime meta wrong: %+v", all)
	}
}

func TestLoadProvidersRemote_RejectsUnknownProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"providers":[{"name":"bogus","enabled":true}]}`))
	}))
	defer srv.Close()
	_, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err == nil {
		t.Fatal("expected error for unknown provider name, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestLoadProvidersRemote -v`
Expected: FAIL — `undefined: LoadProvidersRemote`

- [ ] **Step 3: Create the remote loader**

Create `services/scraper/internal/config/providers_remote.go`:

```go
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// remoteProvider mirrors the JSON shape of catalog's
// GET /internal/scraper/providers response items.
type remoteProvider struct {
	Name             string `json:"name"`
	Enabled          bool   `json:"enabled"`
	Group            string `json:"group"`
	Reason           string `json:"reason"`
	Description      string `json:"description"`
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `json:"sub_delivery"`
	QualityCeiling   string `json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
}

type remoteResponse struct {
	Providers []remoteProvider `json:"providers"`
}

// LoadProvidersRemote fetches provider config from catalog's internal endpoint
// and builds a ProvidersConfig (Source="remote"). Unknown provider names are
// rejected (same fail-fast invariant as LoadProviders) so a bad DB row falls
// back to YAML at the call site rather than silently mis-registering.
func LoadProvidersRemote(ctx context.Context, baseURL string, client *http.Client, timeout time.Duration) (ProvidersConfig, error) {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/internal/scraper/providers"
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("fetch provider config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProvidersConfig{}, fmt.Errorf("provider config status %d", resp.StatusCode)
	}
	var rr remoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return ProvidersConfig{}, fmt.Errorf("decode provider config: %w", err)
	}

	known := make(map[string]bool, len(KnownProviders))
	for _, n := range KnownProviders {
		known[n] = true
	}
	metas := make(map[string]ProviderMeta, len(rr.Providers))
	for _, p := range rr.Providers {
		if p.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("provider config: entry with empty name")
		}
		if !known[p.Name] {
			return ProvidersConfig{}, fmt.Errorf("provider config: unknown provider %q", p.Name)
		}
		subDelivery := p.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		metas[p.Name] = ProviderMeta{
			Name:             p.Name,
			Enabled:          p.Enabled,
			Reason:           p.Reason,
			Description:      p.Description,
			Group:            GroupOf(p.Name), // intrinsic — never trust remote group
			SupportsSub:      p.SupportsSub,
			SupportsDub:      p.SupportsDub,
			SupportsRaw:      p.SupportsRaw,
			SubDelivery:      subDelivery,
			QualityCeiling:   p.QualityCeiling,
			PreferenceWeight: p.PreferenceWeight,
		}
	}
	return ProvidersConfig{metas: metas, Source: "remote"}, nil
}
```

> Note: `Group` is set from the intrinsic `GroupOf(p.Name)`, never the remote value — preserves the "typo can't move 18anime into the EN chain" invariant.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestLoadProvidersRemote -v`
Expected: PASS (both subtests)

- [ ] **Step 5: Add `CATALOG_URL` to scraper config** — in `services/scraper/internal/config/config.go`, add a field to `Config` (near `Providers`):

```go
	// CatalogURL is the base URL of the catalog service, used to fetch provider
	// config from /internal/scraper/providers. Empty disables remote config
	// (YAML/env fallback only).
	CatalogURL string

	// ProvidersRefresh is how often to re-fetch remote provider config. 0 = no
	// periodic refresh (boot-only).
	ProvidersRefresh time.Duration
```

In the config builder (where other env vars are read, near the `SCRAPER_PROVIDERS_FILE` block ~line 247), add:

```go
	cfg.CatalogURL = getEnv("CATALOG_URL", "")
	if d, err := time.ParseDuration(getEnv("SCRAPER_PROVIDERS_REFRESH", "60s")); err == nil {
		cfg.ProvidersRefresh = d
	}
```

- [ ] **Step 6: Prefer remote at boot** — in `services/scraper/cmd/scraper-api/main.go`, after `cfg` is loaded and before `scraperHandler.WithProvidersConfig(&cfg.Providers)` (~line 583), insert:

```go
	// Prefer DB-backed provider config from catalog; fall back to the YAML/env
	// config already in cfg.Providers if catalog is unreachable at boot.
	if cfg.CatalogURL != "" {
		if pc, err := config.LoadProvidersRemote(context.Background(), cfg.CatalogURL, nil, 5*time.Second); err != nil {
			log.Warnw("remote provider config unavailable; using YAML/env fallback", "error", err, "catalog_url", cfg.CatalogURL)
		} else {
			cfg.Providers = pc
			log.Infow("loaded provider config from catalog", "source", pc.Source, "disabled", pc.DisabledNames())
		}
	}
```

Ensure `context` is imported in main.go (it usually is; add it if not).

- [ ] **Step 7: Build scraper**

Run: `cd /data/animeenigma/services/scraper && go build ./... && go test ./internal/config/`
Expected: builds clean, config tests pass

- [ ] **Step 8: Commit**

```bash
git add services/scraper/internal/config/providers_remote.go services/scraper/internal/config/providers_remote_test.go services/scraper/internal/config/config.go services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): load provider config from catalog at boot (YAML fallback)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 8: Concurrency-safe `ProvidersConfig` + periodic refresher

The refresher swaps `cfg.Providers` while request handlers read it, so reads must be atomic. Refactor `ProvidersConfig` to hold its map behind `atomic.Pointer` and add a `Replace` method; the handler API (`WithProvidersConfig(&cfg.Providers)`) stays unchanged.

**Files:**
- Modify: `services/scraper/internal/config/providers.go` (atomic map + `Replace`)
- Create: `services/scraper/internal/config/providers_refresh.go`
- Create: `services/scraper/internal/config/providers_refresh_test.go`
- Modify: `services/scraper/cmd/scraper-api/main.go` (start refresher)

- [ ] **Step 1: Write the failing test**

Create `services/scraper/internal/config/providers_refresh_test.go`:

```go
package config

import (
	"sync"
	"testing"
)

func TestProvidersConfig_ReplaceIsAtomic(t *testing.T) {
	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "allanime", Enabled: true}})
	// Concurrent readers while Replace runs — must not race (run with -race).
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = pc.IsEnabled("allanime")
					_ = pc.Meta("allanime")
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		pc.Replace([]ProviderMeta{{Name: "allanime", Enabled: i%2 == 0}})
	}
	close(stop)
	wg.Wait()
	// Final state: i=99 was odd → last Replace set Enabled=false.
	if pc.IsEnabled("allanime") {
		t.Error("expected allanime disabled after final Replace")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestProvidersConfig_ReplaceIsAtomic -race -v`
Expected: FAIL — `pc.Replace undefined`

- [ ] **Step 3: Refactor `ProvidersConfig` to atomic** — in `services/scraper/internal/config/providers.go`:

Change the struct definition:

```go
type ProvidersConfig struct {
	metas  *atomic.Pointer[map[string]ProviderMeta]
	Source string
}
```

Add `"sync/atomic"` to the imports.

Add a constructor + `Replace`, and update every method that reads `p.metas` to load atomically. Replace the existing method bodies that touch `p.metas` (`IsEnabled`, `Meta`, `Rows`, `toDegradedConfig`, `DisabledNames`) and the two factory functions (`NewProvidersConfigForTest`, and the `return ProvidersConfig{metas: metas, ...}` in `LoadProviders`, `providersFromDegraded`, and `LoadProvidersRemote`) to go through a helper:

```go
// newProvidersConfig wraps a metas map in an atomic pointer.
func newProvidersConfig(metas map[string]ProviderMeta, source string) ProvidersConfig {
	ap := &atomic.Pointer[map[string]ProviderMeta]{}
	ap.Store(&metas)
	return ProvidersConfig{metas: ap, Source: source}
}

// load returns the current metas map (never nil after construction).
func (p ProvidersConfig) load() map[string]ProviderMeta {
	if p.metas == nil {
		return nil
	}
	if m := p.metas.Load(); m != nil {
		return *m
	}
	return nil
}

// Replace atomically swaps the provider metadata (used by the refresher).
func (p ProvidersConfig) Replace(entries []ProviderMeta) {
	if p.metas == nil {
		return
	}
	m := make(map[string]ProviderMeta, len(entries))
	for _, e := range entries {
		m[e.Name] = e
	}
	p.metas.Store(&m)
}
```

Then update the readers to use `p.load()`:

```go
func (p ProvidersConfig) IsEnabled(name string) bool {
	if m, ok := p.load()[name]; ok {
		return m.Enabled
	}
	return true
}

func (p ProvidersConfig) Meta(name string) ProviderMeta { return p.load()[name] }
```

Update `Rows`, `toDegradedConfig`, and `DisabledNames` to range over `p.load()` instead of `p.metas`. Update the four construction sites:
- `NewProvidersConfigForTest`: `return newProvidersConfig(metas, "test")`
- `LoadProviders`: `return newProvidersConfig(metas, "file"), nil`
- `providersFromDegraded`: `return newProvidersConfig(metas, source)`
- `LoadProvidersRemote` (Task 7): `return newProvidersConfig(metas, "remote"), nil`

- [ ] **Step 4: Run the atomic test (with -race)**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -run TestProvidersConfig_ReplaceIsAtomic -race -v`
Expected: PASS, no race report

- [ ] **Step 5: Run the WHOLE config package with -race** (catches any missed `p.metas` reader):

Run: `cd /data/animeenigma/services/scraper && go test ./internal/config/ -race`
Expected: ok — all prior tests (traits, remote, etc.) still pass against the atomic refactor

- [ ] **Step 6: Create the refresher**

Create `services/scraper/internal/config/providers_refresh.go`:

```go
package config

import (
	"context"
	"time"
)

// Logger is the minimal logging surface the refresher needs (satisfied by
// libs/logger's SugaredLogger).
type Logger interface {
	Infow(msg string, kv ...any)
	Warnw(msg string, kv ...any)
}

// StartProvidersRefresher periodically re-fetches provider config from catalog
// and atomically swaps it into target via Replace. Runs until ctx is canceled.
// A failed refresh keeps the last-good config (logged at WARN). No-op if
// catalogURL is empty or interval <= 0.
func StartProvidersRefresher(ctx context.Context, target *ProvidersConfig, catalogURL string, interval time.Duration, log Logger) {
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
			}
		}
	}()
}
```

- [ ] **Step 7: Start the refresher in main.go** — in `services/scraper/cmd/scraper-api/main.go`, after `scraperHandler.WithProvidersConfig(&cfg.Providers)` (~line 583), add:

```go
	// Hot-reload provider config from catalog (enable/disable without restart).
	config.StartProvidersRefresher(context.Background(), &cfg.Providers, cfg.CatalogURL, cfg.ProvidersRefresh, log)
```

- [ ] **Step 8: Build + full scraper test with -race**

Run: `cd /data/animeenigma/services/scraper && go build ./... && go test ./... -race 2>&1 | tail -30`
Expected: builds clean; config + handler packages pass (pre-existing unrelated failures, if any, are out of scope — note them but do not fix here).

- [ ] **Step 9: Commit**

```bash
git add services/scraper/internal/config/providers.go services/scraper/internal/config/providers_refresh.go services/scraper/internal/config/providers_refresh_test.go services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): atomic ProvidersConfig + periodic catalog refresh

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Integration verification (after all tasks)

- [ ] **Deploy both services** (from a clean HEAD worktree per the shared-tree deploy hazard — do NOT `make redeploy` the dirty shared tree):

```bash
make redeploy-catalog && make redeploy-scraper && make health
```

- [ ] **Confirm the table seeded** (8 rows):

```bash
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -c "SELECT name,enabled,\"group\",supports_dub,sub_delivery,preference_weight FROM scraper_providers ORDER BY name;"
```
Expected: 8 rows (allanime, animefever, animekai, animepahe, gogoanime, miruro, nineanime, 18anime) with the seeded traits.

- [ ] **Confirm the internal endpoint** answers on the Docker network (via the scraper container, since /internal/* is not gateway-routed):

```bash
docker compose -f docker/docker-compose.yml exec -T scraper \
  wget -qO- http://catalog:8081/internal/scraper/providers | head -c 400
```
Expected: JSON `{"providers":[...]}` with 8 entries.

- [ ] **Confirm scraper loaded remote config** (boot log):

```bash
docker compose -f docker/docker-compose.yml logs scraper | grep -E "loaded provider config from catalog|using YAML/env fallback" | tail -3
```
Expected: `loaded provider config from catalog source=remote ...`

- [ ] **Confirm failover still works** (smoke a known-good EN title via the existing scraper health/episodes path) — pick a title that worked before this change and verify `/api/anime/{uuid}/scraper/episodes` still returns episodes. Confirms the provider registration path is unchanged by the config-source swap.

- [ ] **Run `/animeenigma-after-update`** to changelog + push (per project workflow).

---

## Self-Review (completed during authoring)

- **Spec coverage (P1 only):** YAML traits (T1) ✔ · DB model+migrate (T3) ✔ · seeder insert-if-absent (T4) ✔ · internal endpoint (T5) ✔ · scraper remote fetch + YAML fallback (T7) ✔ · ~60s refresh (T8) ✔ · compose mounts/env (T6) ✔. P2/P3/P4 explicitly deferred to follow-on plans.
- **Type consistency:** `ProviderMeta` trait fields (T2) match the seeder `seedEntry` columns (T4), the `ScraperProvider` model columns (T3), and `remoteProvider` JSON fields (T7). `ProvidersConfig.metas` becomes `*atomic.Pointer[...]` in T8 — all construction sites (`NewProvidersConfigForTest`, `LoadProviders`, `providersFromDegraded`, `LoadProvidersRemote`) routed through `newProvidersConfig`; all readers through `load()`. `Replace([]ProviderMeta)` signature matches the refresher's call.
- **Placeholder scan:** no TBD/TODO; every code step shows complete code; response-helper/​envelope steps (T5) instruct grep-then-match so the test and handler agree.
- **Known dependency on real code:** T5 assumes `libs/httputil.OK`/`Error` signatures and the router constructor parameter convention — both are verified by the grep steps inside the task before writing code.
