# Scraper P4 — Catalog assembled+ranked capabilities endpoint

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A catalog endpoint `GET /api/anime/{animeId}/capabilities` that returns an assembled, **ranked** per-provider capability report, so a future player can render a decluttered menu ("best provider first; show the rest behind hacker mode") and know who offers sub/dub. Render-only for the player.

**Architecture:** Catalog owns everything needed: the `scraper_providers` table (traits + ranking weight, from P1), `/scraper/health` (liveness/playability), and `/scraper/servers` (per-title categories, made honest in P2). P4a assembles the **EN (ourenglish) family** from the DB traits + one `/scraper/health` call, ranked — cheap, no per-title fan-out. P4b adds the **Kodik / AniLib / Hanime** families via their existing catalog service methods.

**Tech Stack:** Go, chi, GORM (`db.DB`), `libs/cache`, `libs/httputil` ({success,data} envelope), `net/http/httptest` + sqlite for tests. Handwritten fakes (no testify/mock for new service tests — project convention).

**Convention:** every commit includes the standard co-author trailer (Claude Opus 4.6 / 0neymik0 / NANDIorg). Path-scope every `git add`; no `git add -A`; no push (controller lands commits).

**Phasing:** **P4a** (this plan, Tasks 1–4) ships the ranked EN family. **P4b** (Tasks 5–7, outlined) adds RU/Hanime adapters — implement after P4a lands and is reviewed.

---

## Grounding (verified by exploration)

- Routes registered in `services/catalog/internal/transport/router.go:155-158` (`r.Get("/{animeId}/scraper/episodes", catalogHandler.GetScraperEpisodes)` …). `CatalogHandler` embeds `*ScraperEndpointsHandler` (`handler/catalog.go:18-42`), wired via `WireScraperEndpoints` (`handler/scraper.go:45-51`). Service interface `scraperServiceAPI` (`handler/scraper.go:24-31`).
- UUID→(malID,title,altTitles) via `resolveAnime` (`service/scraper.go:68-103`); anime fields `Name/NameEN/NameJP/ShikimoriID/MALID` (`domain/anime.go:12-38`).
- Scraper client `GetHealth(ctx) (int,[]byte,error)` (`parser/scraper/client.go:128-131`) returns the raw `/scraper/health` body `{success,data:{providers:{<name>:{up,enabled,...}}, playable:{<name>:bool}}}`.
- `domain.ScraperProvider` (table `scraper_providers`) queryable via `db.DB.WithContext(ctx).Find(&rows)` (mirror `handler/internal_scraper_providers.go:30-41`). Fields: `Name, Enabled, Group, Reason, Description, SupportsSub/Dub/Raw, SubDelivery, QualityCeiling, PreferenceWeight`.
- Cache: `cache.Get`/`cache.Set` with `errors.Is(err, cache.ErrNotFound)` (`service/spotlight/cards/latest_news.go:54-89`).

---

## File Structure (P4a)

**Create:**
- `services/catalog/internal/domain/capability.go` — unified report types.
- `services/catalog/internal/domain/capability_test.go` — JSON round-trip.
- `services/catalog/internal/service/capability/rank.go` — pure ranking + variant-from-traits.
- `services/catalog/internal/service/capability/rank_test.go`.
- `services/catalog/internal/service/capability/service.go` — assemble EN family (DB + health), cache.
- `services/catalog/internal/service/capability/service_test.go` — assembly with fakes.
- `services/catalog/internal/handler/capabilities.go` — HTTP handler.
- `services/catalog/internal/handler/capabilities_test.go`.

**Modify:**
- `services/catalog/internal/transport/router.go` — register the route.
- `services/catalog/cmd/catalog-api/main.go` — construct + wire the capability service/handler.

---

## Task 1: Unified capability domain types

**Files:** Create `services/catalog/internal/domain/capability.go` + `_test.go`.

- [ ] **Step 1: Write the failing round-trip test** — `capability_test.go`:

```go
package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCapabilityReport_RoundTrip(t *testing.T) {
	pb := true
	in := domain.CapabilityReport{
		AnimeID: "uuid-1",
		Families: []domain.SourceFamily{{
			Family: "ourenglish",
			Providers: []domain.ProviderCap{{
				Provider: "allanime", DisplayName: "AllAnime", Enabled: true,
				Health: "up", Playable: &pb, Rank: 130,
				Variants: []domain.Variant{{
					Category: "dub", SubDelivery: "none", Qualities: []string{"1080p"},
					QualitySource: "trait", Source: "trait",
				}},
			}},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out domain.CapabilityReport
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Families[0].Providers[0].Variants[0].Category != "dub" || out.Families[0].Providers[0].Rank != 130 {
		t.Errorf("round-trip mismatch: %+v", out)
	}
	if out.Families[0].Providers[0].Playable == nil || !*out.Families[0].Providers[0].Playable {
		t.Errorf("playable not preserved")
	}
}
```

- [ ] **Step 2: Run, confirm fail** — `cd /data/animeenigma/services/catalog && go test ./internal/domain/ -run TestCapabilityReport_RoundTrip -v` → FAIL (undefined types).

- [ ] **Step 3: Create `capability.go`:**

```go
package domain

// CapabilityReport is the assembled, ranked per-provider capability view for an
// anime (spec 2026-06-15-scraper-capability-api). The future player renders it:
// best provider first per family, the rest available behind a "hacker mode".
type CapabilityReport struct {
	AnimeID  string         `json:"anime_id"`
	Families []SourceFamily `json:"families"`
}

// SourceFamily groups providers of one source kind; Providers is ranked best-first.
type SourceFamily struct {
	Family    string        `json:"family"` // "ourenglish" | "kodik" | "animelib" | "hanime"
	Providers []ProviderCap `json:"providers"`
}

// ProviderCap is one provider's capability + liveness + rank within a family.
type ProviderCap struct {
	Provider    string    `json:"provider"`
	DisplayName string    `json:"display_name"`
	Enabled     bool      `json:"enabled"`
	Health      string    `json:"health"`             // "up" | "down" | "unknown"
	Playable    *bool     `json:"playable,omitempty"` // real-bytes oracle, if known
	Rank        float64   `json:"rank"`
	Variants    []Variant `json:"variants"`
}

// Variant is a watchable unit: a category (+ optional translation team for RU),
// its subtitle delivery, and quality info. Source records provenance.
type Variant struct {
	Category      string   `json:"category"`       // "sub" | "dub" | "raw"
	Team          *Team    `json:"team,omitempty"` // RU only; nil for EN (reserved — backlog)
	SubDelivery   string   `json:"sub_delivery"`   // "soft" | "hard" | "none"
	Qualities     []string `json:"qualities,omitempty"`
	QualitySource string   `json:"quality_source"` // "hls_master" | "discrete" | "unknown" | "trait"
	Source        string   `json:"source"`         // "trait" | "discovered"
}

// Team is a translation/dub group (real for Kodik/AniLib; nil for EN providers).
type Team struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}
```

- [ ] **Step 4: Run, confirm pass.** `cd /data/animeenigma/services/catalog && go test ./internal/domain/ -run TestCapabilityReport_RoundTrip -v` → PASS. `gofmt -w` the new files.

- [ ] **Step 5: Commit** `domain/capability.go` + `_test.go`:
```bash
git add services/catalog/internal/domain/capability.go services/catalog/internal/domain/capability_test.go
git commit -m "feat(catalog): unified capability report domain types

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Pure ranking + variants-from-traits

**Files:** Create `services/catalog/internal/service/capability/rank.go` + `rank_test.go`.

- [ ] **Step 1: Failing test** — `rank_test.go`:

```go
package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestRankEN_OrdersByWeightHealthPlayable(t *testing.T) {
	up, down := "up", "down"
	yes, no := true, false
	// higher weight ranks higher, all else equal
	a := rankEN(domain.ScraperProvider{PreferenceWeight: 90, QualityCeiling: "1080p", SubDelivery: "hard"}, up, &yes)
	b := rankEN(domain.ScraperProvider{PreferenceWeight: 40, QualityCeiling: "720p", SubDelivery: "hard"}, up, &yes)
	if a <= b {
		t.Errorf("weight 90 (%v) should outrank weight 40 (%v)", a, b)
	}
	// a down provider sinks below an up one even with higher weight
	downHi := rankEN(domain.ScraperProvider{PreferenceWeight: 90, QualityCeiling: "1080p"}, down, &no)
	upLo := rankEN(domain.ScraperProvider{PreferenceWeight: 40, QualityCeiling: "720p"}, up, &yes)
	if downHi >= upLo {
		t.Errorf("down provider (%v) must rank below up provider (%v)", downHi, upLo)
	}
}

func TestVariantsFromTraits(t *testing.T) {
	row := domain.ScraperProvider{SupportsSub: true, SupportsDub: true, SubDelivery: "hard", QualityCeiling: "1080p"}
	vs := variantsFromTraits(row)
	var sub, dub *domain.Variant
	for i := range vs {
		switch vs[i].Category {
		case "sub":
			sub = &vs[i]
		case "dub":
			dub = &vs[i]
		}
	}
	if sub == nil || dub == nil {
		t.Fatalf("want sub+dub variants, got %+v", vs)
	}
	if sub.SubDelivery != "hard" || sub.Source != "trait" || len(sub.Qualities) != 1 || sub.Qualities[0] != "1080p" {
		t.Errorf("sub variant wrong: %+v", sub)
	}
	if dub.SubDelivery != "none" { // dub is audio — no subs
		t.Errorf("dub sub_delivery = %q, want none", dub.SubDelivery)
	}
}
```

- [ ] **Step 2: Run, confirm fail.**

- [ ] **Step 3: Create `rank.go`:**

```go
// Package capability assembles the ranked per-provider capability report.
package capability

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// rankEN scores an EN provider for ordering (higher = better). Pure.
//   weight (preference) + health(down: -100) + playable(true:+25/false:-25/nil:0)
//   + quality_ceiling(2160:+20 / 1080:+15 / 720:+8) + sub_delivery(soft:+10 / hard:-5)
func rankEN(row domain.ScraperProvider, health string, playable *bool) float64 {
	score := float64(row.PreferenceWeight)
	if health == "down" {
		score -= 100
	}
	if playable != nil {
		if *playable {
			score += 25
		} else {
			score -= 25
		}
	}
	score += qualityCeilingScore(row.QualityCeiling)
	switch row.SubDelivery {
	case "soft":
		score += 10
	case "hard":
		score -= 5
	}
	return score
}

func qualityCeilingScore(q string) float64 {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case "2160p":
		return 20
	case "1080p":
		return 15
	case "720p":
		return 8
	default:
		return 0
	}
}

// variantsFromTraits derives the watchable variants a provider claims (no live
// per-title confirmation — Source="trait"). Dub is audio, so its sub_delivery is
// "none"; sub/raw carry the provider's subtitle delivery.
func variantsFromTraits(row domain.ScraperProvider) []domain.Variant {
	var out []domain.Variant
	q := []string{}
	if row.QualityCeiling != "" {
		q = []string{row.QualityCeiling}
	}
	mk := func(cat, delivery string) domain.Variant {
		return domain.Variant{
			Category: cat, SubDelivery: delivery, Qualities: q,
			QualitySource: "trait", Source: "trait",
		}
	}
	if row.SupportsSub {
		out = append(out, mk("sub", row.SubDelivery))
	}
	if row.SupportsDub {
		out = append(out, mk("dub", "none"))
	}
	if row.SupportsRaw {
		out = append(out, mk("raw", row.SubDelivery))
	}
	return out
}

// displayName title-cases a provider id for UI (e.g. "allanime" → "AllAnime"
// where known, else a simple Title-case). Keep a small known-name map.
func displayName(provider string) string {
	known := map[string]string{
		"allanime": "AllAnime", "gogoanime": "GogoAnime", "animepahe": "AnimePahe",
		"animefever": "AnimeFever", "miruro": "Miruro", "nineanime": "9anime",
		"animekai": "AnimeKai", "18anime": "18anime",
	}
	if d, ok := known[provider]; ok {
		return d
	}
	if provider == "" {
		return provider
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}
```

- [ ] **Step 4: Run, confirm pass.** `gofmt -w`.

- [ ] **Step 5: Commit** the `capability/` rank files:
```bash
git add services/catalog/internal/service/capability/rank.go services/catalog/internal/service/capability/rank_test.go
git commit -m "feat(catalog): capability EN ranking + variants-from-traits (pure)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Capability service (assemble EN family, cache)

**Files:** Create `services/catalog/internal/service/capability/service.go` + `service_test.go`.

The service reads enabled EN providers from `scraper_providers`, fetches `/scraper/health`, builds one `ourenglish` family ranked by `rankEN`, and caches the report.

- [ ] **Step 1: Failing test** — `service_test.go` uses a sqlite DB seeded with provider rows + a fake health source + a fake cache:

```go
package capability_test

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeHealth struct {
	up       map[string]bool
	playable map[string]bool
}

func (f fakeHealth) ProviderHealth(_ context.Context) (map[string]capability.HealthInfo, error) {
	out := map[string]capability.HealthInfo{}
	for n, u := range f.up {
		hi := capability.HealthInfo{Up: u}
		if pb, ok := f.playable[n]; ok {
			v := pb
			hi.Playable = &v
		}
		out[n] = hi
	}
	return out, nil
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBuildENFamily_RanksAndFiltersDisabled(t *testing.T) {
	db := newDB(t)
	db.Create(&domain.ScraperProvider{Name: "allanime", Enabled: true, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 90})
	db.Create(&domain.ScraperProvider{Name: "nineanime", Enabled: true, Group: "en", SupportsSub: true, SubDelivery: "hard", QualityCeiling: "720p", PreferenceWeight: 40})
	db.Create(&domain.ScraperProvider{Name: "animepahe", Enabled: false, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", PreferenceWeight: 30})
	db.Create(&domain.ScraperProvider{Name: "18anime", Enabled: true, Group: "adult", SupportsRaw: true, PreferenceWeight: 0})

	svc := capability.NewService(db, fakeHealth{
		up:       map[string]bool{"allanime": true, "nineanime": true},
		playable: map[string]bool{"allanime": true},
	}, nil, nil) // nil cache + logger ok for this unit (service guards nil)

	fam, err := svc.BuildENFamily(context.Background())
	if err != nil {
		t.Fatalf("BuildENFamily: %v", err)
	}
	if fam.Family != "ourenglish" {
		t.Errorf("family = %q", fam.Family)
	}
	// disabled animepahe + adult 18anime excluded; 2 EN providers remain.
	if len(fam.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d (%+v)", len(fam.Providers), fam.Providers)
	}
	// allanime (w90, playable) ranks first.
	if fam.Providers[0].Provider != "allanime" {
		t.Errorf("rank order wrong: %+v", fam.Providers)
	}
	if fam.Providers[0].Health != "up" || fam.Providers[0].Playable == nil || !*fam.Providers[0].Playable {
		t.Errorf("allanime health/playable wrong: %+v", fam.Providers[0])
	}
	// allanime advertises sub+dub variants from traits.
	var sawDub bool
	for _, v := range fam.Providers[0].Variants {
		if v.Category == "dub" {
			sawDub = true
		}
	}
	if !sawDub {
		t.Errorf("allanime should advertise a dub variant: %+v", fam.Providers[0].Variants)
	}
}
```

- [ ] **Step 2: Run, confirm fail.**

- [ ] **Step 3: Create `service.go`:**

```go
package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

const reportTTL = 10 * time.Minute

// HealthInfo is one provider's liveness as seen by /scraper/health.
type HealthInfo struct {
	Up       bool
	Playable *bool
}

// HealthSource yields per-provider health (impl wraps the scraper client).
type HealthSource interface {
	ProviderHealth(ctx context.Context) (map[string]HealthInfo, error)
}

// Service assembles capability reports. EN family in P4a; RU/Hanime in P4b.
type Service struct {
	db     *gorm.DB
	health HealthSource
	cache  cache.Cache // may be nil (skips caching)
	log    *logger.Logger
}

func NewService(db *gorm.DB, health HealthSource, c cache.Cache, log *logger.Logger) *Service {
	return &Service{db: db, health: health, cache: c, log: log}
}

// Report assembles the full (P4a: EN-only) capability report for an anime,
// cache-first. animeID is echoed; the EN family is anime-independent in P4a
// (trait+health based), so the cache key is global-per-day-ish via reportTTL.
func (s *Service) Report(ctx context.Context, animeID string) (domain.CapabilityReport, error) {
	key := "capabilities:en"
	if s.cache != nil {
		var cached domain.CapabilityReport
		if err := s.cache.Get(ctx, key, &cached); err == nil {
			cached.AnimeID = animeID
			return cached, nil
		} else if !errors.Is(err, cache.ErrNotFound) && s.log != nil {
			s.log.Warnw("capability cache get failed", "error", err)
		}
	}
	fam, err := s.BuildENFamily(ctx)
	if err != nil {
		return domain.CapabilityReport{}, err
	}
	report := domain.CapabilityReport{AnimeID: animeID, Families: []domain.SourceFamily{fam}}
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, report, reportTTL); err != nil && s.log != nil {
			s.log.Warnw("capability cache set failed", "error", err)
		}
	}
	return report, nil
}

// BuildENFamily reads enabled EN providers from scraper_providers, joins live
// health, ranks, and returns the "ourenglish" family. Health failure degrades
// to "unknown" per provider (never fails the whole family).
func (s *Service) BuildENFamily(ctx context.Context) (domain.SourceFamily, error) {
	var rows []domain.ScraperProvider
	if err := s.db.WithContext(ctx).
		Where("enabled = ? AND \"group\" = ?", true, "en").
		Order("name asc").Find(&rows).Error; err != nil {
		return domain.SourceFamily{}, fmt.Errorf("load scraper providers: %w", err)
	}

	health := map[string]HealthInfo{}
	if s.health != nil {
		if h, err := s.health.ProviderHealth(ctx); err != nil {
			if s.log != nil {
				s.log.Warnw("scraper health unavailable; providers report unknown", "error", err)
			}
		} else {
			health = h
		}
	}

	caps := make([]domain.ProviderCap, 0, len(rows))
	for _, row := range rows {
		hstatus := "unknown"
		var playable *bool
		if hi, ok := health[row.Name]; ok {
			if hi.Up {
				hstatus = "up"
			} else {
				hstatus = "down"
			}
			playable = hi.Playable
		}
		caps = append(caps, domain.ProviderCap{
			Provider:    row.Name,
			DisplayName: displayName(row.Name),
			Enabled:     row.Enabled,
			Health:      hstatus,
			Playable:    playable,
			Rank:        rankEN(row, hstatus, playable),
			Variants:    variantsFromTraits(row),
		})
	}
	sort.SliceStable(caps, func(i, j int) bool {
		if caps[i].Rank != caps[j].Rank {
			return caps[i].Rank > caps[j].Rank
		}
		return caps[i].Provider < caps[j].Provider // deterministic tiebreak
	})
	return domain.SourceFamily{Family: "ourenglish", Providers: caps}, nil
}
```

- [ ] **Step 4: Run, confirm pass.** `cd /data/animeenigma/services/catalog && go test ./internal/service/capability/ -v`. `gofmt -w`.

- [ ] **Step 5: Commit** the service files.
```bash
git add services/catalog/internal/service/capability/service.go services/catalog/internal/service/capability/service_test.go
git commit -m "feat(catalog): capability service assembles+ranks EN family

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: HealthSource adapter + handler + route + wiring

**Files:** Create `services/catalog/internal/handler/capabilities.go` + `_test.go`; modify `router.go` + `main.go`; add a `HealthSource` impl wrapping the scraper client.

- [ ] **Step 1: HealthSource impl.** In `service/capability/service.go`'s package add `health_scraper.go` that decodes `/scraper/health`. The scraper client's `GetHealth` returns `(int, []byte, error)`. Add:

Create `services/catalog/internal/service/capability/health_scraper.go`:
```go
package capability

import (
	"context"
	"encoding/json"
	"fmt"
)

// scraperHealthClient is the subset of the catalog scraper client this needs.
type scraperHealthClient interface {
	GetHealth(ctx context.Context) (int, []byte, error)
}

// ScraperHealth adapts the scraper /scraper/health body to HealthSource.
type ScraperHealth struct{ Client scraperHealthClient }

func NewScraperHealth(c scraperHealthClient) ScraperHealth { return ScraperHealth{Client: c} }

func (s ScraperHealth) ProviderHealth(ctx context.Context) (map[string]HealthInfo, error) {
	status, body, err := s.Client.GetHealth(ctx)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("scraper health status %d", status)
	}
	// {success,data:{providers:{<name>:{up}}, playable:{<name>:bool}}}
	var env struct {
		Data struct {
			Providers map[string]struct {
				Up bool `json:"up"`
			} `json:"providers"`
			Playable map[string]bool `json:"playable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode scraper health: %w", err)
	}
	out := make(map[string]HealthInfo, len(env.Data.Providers))
	for name, p := range env.Data.Providers {
		hi := HealthInfo{Up: p.Up}
		if pb, ok := env.Data.Playable[name]; ok {
			v := pb
			hi.Playable = &v
		}
		out[name] = hi
	}
	return out, nil
}
```
> Verify the real `/scraper/health` JSON has `data.providers.<name>.up` (bool) and `data.playable.<name>` (bool) — read `services/scraper/internal/handler/scraper.go` GetHealth, or `curl` it, and adjust field names if they differ (e.g. if "up" is nested). Add a unit test `health_scraper_test.go` feeding a sample body and asserting the parse.

- [ ] **Step 2: Failing handler test** — `capabilities_test.go`:
```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

type fakeCapSvc struct{ rep domain.CapabilityReport; err error }

func (f fakeCapSvc) Report(_ context.Context, animeID string) (domain.CapabilityReport, error) {
	f.rep.AnimeID = animeID
	return f.rep, f.err
}

func TestCapabilitiesHandler_OK(t *testing.T) {
	rep := domain.CapabilityReport{Families: []domain.SourceFamily{{Family: "ourenglish", Providers: []domain.ProviderCap{{Provider: "allanime"}}}}}
	h := handler.NewCapabilitiesHandler(fakeCapSvc{rep: rep}, nil)

	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Data domain.CapabilityReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AnimeID != "abc" || len(body.Data.Families) != 1 {
		t.Errorf("bad body: %+v", body.Data)
	}
}
```

- [ ] **Step 3: Create `capabilities.go`:**
```go
package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
)

// capabilityService is the handler's view of the capability assembler.
type capabilityService interface {
	Report(ctx context.Context, animeID string) (domain.CapabilityReport, error)
}

type CapabilitiesHandler struct {
	svc capabilityService
	log *logger.Logger
}

func NewCapabilitiesHandler(svc capabilityService, log *logger.Logger) *CapabilitiesHandler {
	return &CapabilitiesHandler{svc: svc, log: log}
}

// Get handles GET /api/anime/{animeId}/capabilities.
func (h *CapabilitiesHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	report, err := h.svc.Report(r.Context(), animeID)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("capabilities assemble failed", "anime_id", animeID, "error", err)
		}
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, report)
}
```
> Match `httputil.Error`'s real signature (it took an error in the internal-handler example: `httputil.Error(w, errors.Internal(...))`). If `err` from the service is already a domain `*AppError`, pass it; otherwise wrap with `errors.Internal`. Read a sibling handler's error path and mirror it.

- [ ] **Step 4: Run handler test, confirm pass.**

- [ ] **Step 5: Register route + wire main.go.**
- In `router.go`, next to the scraper routes (~line 158), add:
```go
	r.Get("/{animeId}/capabilities", capabilitiesHandler.Get)
```
Add `capabilitiesHandler *handler.CapabilitiesHandler` to `NewRouter`'s params (mirror how other handlers are threaded) and pass it from `main.go`.
- In `main.go`, after the scraper client + cache + db are available, construct:
```go
	capSvc := capability.NewService(db.DB, capability.NewScraperHealth(scraperClient), redisCache, log)
	capabilitiesHandler := handler.NewCapabilitiesHandler(capabilityReportAdapter{capSvc}, log)
```
> `capSvc.Report` matches the handler's `capabilityService` interface directly — pass `capSvc` if its method set matches (it does: `Report(ctx, animeID)`); drop the adapter. Confirm `scraperClient` is the `*scraper.Client` already constructed in main.go for the scraper endpoints (reuse it; it has `GetHealth`). If the scraper client isn't in scope at that point, construct/locate it (grep `scraper.NewClient` / `parser/scraper` in main.go).

- [ ] **Step 6: Build + test + vet.**
`cd /data/animeenigma/services/catalog && gofmt -w ./internal/... && go build ./... && go test ./internal/domain/ ./internal/service/capability/ ./internal/handler/ ./internal/transport/ && go vet ./internal/service/capability/ ./internal/handler/`
Expected: clean, all pass.

- [ ] **Step 7: Commit** handler + health adapter + router + main.
```bash
git add services/catalog/internal/handler/capabilities.go services/catalog/internal/handler/capabilities_test.go services/catalog/internal/service/capability/health_scraper.go services/catalog/internal/service/capability/health_scraper_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): GET /api/anime/{id}/capabilities (EN family, ranked)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Integration verification (after P4a)

- [ ] Redeploy catalog: `make redeploy-catalog && make health`.
- [ ] `curl -s http://localhost:8000/api/anime/<a-real-anime-uuid>/capabilities | jq '.data.families[0].providers[] | {provider,health,rank,playable,variants:(.variants|map(.category))}'` — confirm the `ourenglish` family lists enabled EN providers, ranked (allanime/gogoanime high), with sub/dub variants per traits and live health/playable. Disabled (animepahe/animekai) and adult (18anime) are absent.

---

## P4b (outline — implement after P4a lands & reviewed)

Add the **kodik**, **animelib**, **hanime** families by adapting their existing catalog service methods into `domain.SourceFamily`:

- **kodik** — `CatalogService.GetKodikTranslations(ctx, animeID)` → per translation: `Variant{Category: voice→dub/subtitles→sub, Team:{ID,Name:Title}, SubDelivery:"none"(iframe), QualitySource:"unknown"}`. One `ProviderCap{Provider:"kodik"}` whose Variants are the translations. (Kodik exposes real teams; no quality.)
- **animelib** — `CatalogService.GetAnimeLibTranslations(ctx, animeID, episodeID)` → per translation: `Team{ID,Name}`, category from `TranslationType` (1=sub,2=dub), `SubDelivery: len(Subtitles)>0 ? "soft" : "hard"`, qualities from `Video.Quality[]` (P4b needs an episode; default ep 1). (Full team+soft/hard+quality.)
- **hanime** — `CatalogService.GetHanimeEpisodes`/`GetHanimeStream` → `Variant{Category:"raw", SubDelivery:"none", Qualities:[from Stream.Height]}`, no team.

Each adapter is best-effort (a family that errors is omitted, never fails the report) and runs concurrently. These call upstreams (cached 1h/24h/30min) so the report assembly fans out — gate behind the report cache (reportTTL) and consider a per-family timeout. The `Report` method gains `Families = [ourenglish, kodik, animelib, hanime].filter(non-empty)`. Ranking within RU/Hanime families: by team/quality presence (define when implementing). Tests: one per adapter with a fake service method; assembly with a subset failing.

---

## Self-Review (P4a, completed during authoring)

- **Spec coverage (P4a):** assembled report types ✔ (T1), ranking ✔ (T2), EN family from DB traits + live health, cached, disabled/adult excluded ✔ (T3), endpoint + route + health adapter ✔ (T4). Per-title "discovered" variants and RU/Hanime families explicitly deferred (P4b / future enrich) — documented.
- **Type consistency:** `domain.CapabilityReport/SourceFamily/ProviderCap/Variant/Team` (T1) used identically in `capability` service (T2/T3) and handler (T4). `HealthInfo`/`HealthSource` defined in T3, implemented in T4 (`ScraperHealth`). `Service.Report(ctx,animeID)` matches the handler's `capabilityService` interface.
- **Placeholder scan:** none; every code step complete. The two spots needing live-shape confirmation (the `/scraper/health` JSON field names; `httputil.Error` signature; the scraper client variable in main.go) are flagged with explicit "verify and mirror" instructions — the known integration-boundary unknowns.
- **Cost:** P4a is DB query + one `/scraper/health` call (no per-title fan-out), cached `reportTTL`. EN family is anime-independent in P4a (trait+health), so the cache key is global (`capabilities:en`) with the per-request `AnimeID` stamped on read — flush on trait/weight changes.
- **Risk:** the global EN cache key means a provider enable/disable or weight edit shows after `reportTTL` (10m) or a flush. Acceptable; documented.
