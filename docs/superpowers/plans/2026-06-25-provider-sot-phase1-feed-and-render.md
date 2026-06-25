# Provider Single Source of Truth — Phase 1: Authoritative Feed + FE Dumb-Render — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the backend `/capabilities` feed the single authority for the player's Source list — every provider's existence, state, selectability, order, group, and audio support comes from the `stream_providers` DB — and make the frontend a dumb renderer of it, deleting `providerRegistry.ts`, `CURATED_TIER`, and the registry-driven `useProviderHealth`.

**Architecture:** Extend the existing per-anime, gateway-public `GET /api/anime/{id}/capabilities` so each `ProviderCap` carries computed `state`/`selectable`/`hacker_only`/`order`/`group`/`audios`/`reason`, derived server-side from the DB row (`policy=disabled` rows are never emitted; `status=degraded` → hacker-only; the first-party `ae` provider's `state` reflects a backend library-presence check). The frontend's `ProviderRow` is redefined to carry those feed fields directly; `ProviderChip`/`SourcePanel` color by `state` (DS semantic tokens, not per-provider hue) and order by the feed's `order`.

**Tech Stack:** Go 1.x + GORM (catalog service), Vue 3 + TypeScript + Vitest + Tailwind v4 (frontend/web), `bun` for FE tooling.

## Global Constraints

- **Effort metrics, never time** — any plan/CHANGELOG scoring uses UXΔ / CDI / MVQ per `.planning/CONVENTIONS.md`. No "days/hours/sprints".
- **Spec is authority** — `docs/superpowers/specs/2026-06-25-provider-single-source-of-truth-design.md`. Phase 1 = the feed + FE render only. No-content reactive cache (Phase 2), notifications purge (Phase 3), and the resurrection dashboard (Phase 4) are OUT of this plan.
- **DS-lint is build-enforced** — no off-palette Tailwind color classes; bind to semantic tokens; only `font-medium`/`font-semibold`. State colors use semantic tokens: `active`→`text-success`/`--brand-cyan`, `recovering`→`text-lime-400` (already used), `degraded`→`text-warning`, `no_content`→`text-muted-foreground`. Run `bash frontend/web/scripts/design-system-lint.sh` clean.
- **i18n 3-locale gate** — any new UI string added to `en.json` + `ru.json` + `ja.json` or the build fails.
- **FE pre-flight** — run `/frontend-verify` (DS-lint + i18n parity + real `bun run build`) before the FE is considered done.
- **Go tests** — handwritten fakes, no testify/mock. Run `cd services/catalog && go test ./... -count=1`.
- **No new scraper CDNs in the proxy allowlist** — not relevant to this phase but do not touch `videoutils` allowlists.
- **Commit co-authors (every commit):**
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

## File Structure

**Backend (catalog):**
- Create `services/catalog/internal/service/capability/providerview.go` — pure `deriveProviderView(row, hasContent)` + `audiosFromTraits(row)`.
- Create `services/catalog/internal/service/capability/providerview_test.go` — table tests.
- Modify `services/catalog/internal/domain/capability.go` — add fields to `ProviderCap`.
- Modify `services/catalog/internal/service/capability/service.go` (`BuildENFamily`) — populate new fields.
- Modify `services/catalog/internal/service/capability/families_ru.go` (kodik/animelib/hanime) — populate new fields from the DB row.
- Create `services/catalog/internal/service/capability/families_firstparty.go` — `ae` (library-presence) + `raw` + `18anime` families, DB-row-driven.
- Modify `services/catalog/internal/service/capability/service.go` (`buildFamilies`) — wire the new families.

**Frontend:**
- Modify `frontend/web/src/types/capabilities.ts` — add feed fields to `ProviderCap`.
- Modify `frontend/web/src/types/aePlayer.ts` — redefine `ProviderRow` + `ChipState`; retire `ProviderDef`/`ScraperProviderHealth` usage.
- Create `frontend/web/src/composables/aePlayer/useProviderFeed.ts` — builds `ProviderRow[]` from the capability report.
- Delete `frontend/web/src/components/player/aePlayer/providerRegistry.ts`.
- Delete `frontend/web/src/composables/aePlayer/useProviderHealth.ts`.
- Modify `frontend/web/src/components/player/aePlayer/ProviderChip.vue` — render feed fields; color by `state`.
- Modify `frontend/web/src/components/player/aePlayer/SourcePanel.vue` — new `ProviderRow` shape; `STATE_RANK` updated.
- Modify `frontend/web/src/components/player/aePlayer/AePlayer.vue` — build rows from the feed; drop registry imports; `smartDefault` consumes the feed.
- Modify `frontend/web/src/composables/aePlayer/smartDefault.ts` — pick by feed `order`/`state`, drop `CURATED_TIER`.
- Modify locale files + specs as noted per task.

---

## Backend

### Task B1: Add the pure provider-view derivation

**Files:**
- Create: `services/catalog/internal/service/capability/providerview.go`
- Test: `services/catalog/internal/service/capability/providerview_test.go`

**Interfaces:**
- Consumes: `domain.ScraperProvider` (fields `Status`, `Health`, `PreferenceWeight`, `Group`, `SupportsSub/Dub/Raw`, `Reason`; methods `IsDegraded()`, `IsEnabled()`).
- Produces:
  - `func deriveProviderView(row domain.ScraperProvider, hasContent bool) (state string, selectable, hackerOnly bool)`
  - `func audiosFromTraits(row domain.ScraperProvider) []string`
  - `func wireGroup(g string) string` (maps DB `firstparty`→`firstparty`, passthrough; normalizes empty→`en`).

- [ ] **Step 1: Write the failing test**

```go
package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestDeriveProviderView(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.ProviderStatus
		health     domain.ProviderHealth
		hasContent bool
		wantState  string
		wantSel    bool
		wantHacker bool
	}{
		{"enabled up with content", domain.StatusEnabled, domain.HealthUp, true, "active", true, false},
		{"enabled down with content stays active", domain.StatusEnabled, domain.HealthDown, true, "active", true, false},
		{"enabled recovering", domain.StatusEnabled, domain.HealthRecovering, true, "recovering", true, false},
		{"enabled up no content", domain.StatusEnabled, domain.HealthUp, false, "no_content", false, false},
		{"degraded is hacker-only", domain.StatusDegraded, domain.HealthDown, true, "degraded", true, true},
		{"degraded ignores content", domain.StatusDegraded, domain.HealthUp, false, "degraded", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := domain.ScraperProvider{Status: c.status, Health: c.health}
			gotState, gotSel, gotHacker := deriveProviderView(row, c.hasContent)
			if gotState != c.wantState || gotSel != c.wantSel || gotHacker != c.wantHacker {
				t.Fatalf("got (%q,%v,%v) want (%q,%v,%v)",
					gotState, gotSel, gotHacker, c.wantState, c.wantSel, c.wantHacker)
			}
		})
	}
}

func TestAudiosFromTraits(t *testing.T) {
	row := domain.ScraperProvider{SupportsSub: true, SupportsDub: true}
	got := audiosFromTraits(row)
	if len(got) != 2 || got[0] != "sub" || got[1] != "dub" {
		t.Fatalf("got %v want [sub dub]", got)
	}
	if a := audiosFromTraits(domain.ScraperProvider{SupportsRaw: true}); len(a) != 1 || a[0] != "raw" {
		t.Fatalf("raw-only got %v want [raw]", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'TestDeriveProviderView|TestAudiosFromTraits' -count=1`
Expected: FAIL — `undefined: deriveProviderView` / `undefined: audiosFromTraits`.

- [ ] **Step 3: Write the implementation**

```go
package capability

import "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"

// deriveProviderView computes the FE-facing presentation of a provider from its
// DB row plus whether this title has content on it. policy=disabled rows are
// filtered out BEFORE this is called (they are never emitted). Phase 1:
// hasContent is true for every family member that survived family-level presence
// gating, except first-party `ae` whose hasContent is a live library lookup.
// Phase 2 feeds EN providers' hasContent from the reactive no_content cache.
//
//	degraded            → hacker-only (selectable only when hacker mode is on)
//	enabled + !content  → no_content (tinted, not selectable)
//	enabled + recovering→ recovering (selectable)
//	enabled + (up|down) → active     (selectable; status keeps it in the chain)
func deriveProviderView(row domain.ScraperProvider, hasContent bool) (state string, selectable, hackerOnly bool) {
	if row.IsDegraded() {
		return "degraded", true, true
	}
	if !hasContent {
		return "no_content", false, false
	}
	if row.Health == domain.HealthRecovering {
		return "recovering", true, false
	}
	return "active", true, false
}

// audiosFromTraits lists the audio kinds a provider serves, sub before dub
// before raw (stable for the FE filter).
func audiosFromTraits(row domain.ScraperProvider) []string {
	out := make([]string, 0, 3)
	if row.SupportsSub {
		out = append(out, "sub")
	}
	if row.SupportsDub {
		out = append(out, "dub")
	}
	if row.SupportsRaw {
		out = append(out, "raw")
	}
	return out
}

// wireGroup normalizes the DB group for the wire. Empty defaults to "en".
func wireGroup(g string) string {
	if g == "" {
		return "en"
	}
	return g
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'TestDeriveProviderView|TestAudiosFromTraits' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/capability/providerview.go services/catalog/internal/service/capability/providerview_test.go
git commit -m "feat(catalog): pure provider-view derivation for capability feed

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B2: Add feed fields to the `ProviderCap` domain type

**Files:**
- Modify: `services/catalog/internal/domain/capability.go:18-30`

**Interfaces:**
- Produces: `domain.ProviderCap` gains `State string`, `Selectable bool`, `HackerOnly bool`, `Order int`, `Group string`, `Audios []string`, `Reason string`. Existing fields are kept (transitional) so current consumers compile.

- [ ] **Step 1: Write the failing test** (round-trip in the domain package)

Create `services/catalog/internal/domain/capability_provider_test.go`:

```go
package domain

import (
	"encoding/json"
	"testing"
)

func TestProviderCapFeedFieldsRoundTrip(t *testing.T) {
	in := ProviderCap{
		Provider: "gogoanime", DisplayName: "GogoAnime",
		State: "active", Selectable: true, HackerOnly: false,
		Order: 85, Group: "en", Audios: []string{"sub", "dub"}, Reason: "",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ProviderCap
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.State != "active" || !out.Selectable || out.Order != 85 ||
		out.Group != "en" || len(out.Audios) != 2 {
		t.Fatalf("round-trip lost feed fields: %+v", out)
	}
	// JSON keys are snake_case for the FE.
	if got := string(b); !contains(got, `"state":"active"`) || !contains(got, `"hacker_only":false`) {
		t.Fatalf("unexpected json: %s", got)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}()) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/domain/ -run TestProviderCapFeedFieldsRoundTrip -count=1`
Expected: FAIL — `unknown field 'State' in struct literal`.

- [ ] **Step 3: Add the fields**

In `services/catalog/internal/domain/capability.go`, extend `ProviderCap` (keep existing fields above `Variants`):

```go
type ProviderCap struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
	Degraded    bool   `json:"degraded"`
	Health      string `json:"health"`
	Playable    *bool  `json:"playable,omitempty"`
	Rank        float64 `json:"rank"`

	// Phase-1 single-source-of-truth feed fields. Computed server-side from the
	// DB row via deriveProviderView; the player renders these verbatim.
	State      string   `json:"state"`        // active | recovering | degraded | no_content
	Selectable bool     `json:"selectable"`
	HackerOnly bool     `json:"hacker_only"`  // true only for degraded
	Order      int      `json:"order"`        // preference_weight; FE sorts desc
	Group      string   `json:"group"`        // en | ru | adult | jp | firstparty
	Audios     []string `json:"audios"`       // ["sub","dub","raw"] from supports_*
	Reason     string   `json:"reason,omitempty"`

	Variants []Variant `json:"variants"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/domain/ -run TestProviderCapFeedFieldsRoundTrip -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/domain/capability.go services/catalog/internal/domain/capability_provider_test.go
git commit -m "feat(catalog): ProviderCap carries state/selectable/order/group/audios feed fields

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B3: Populate the feed fields in the EN family

**Files:**
- Modify: `services/catalog/internal/service/capability/service.go:138-160` (the cap-build loop in `BuildENFamily`)
- Test: `services/catalog/internal/service/capability/service_test.go`

**Interfaces:**
- Consumes: `deriveProviderView`, `audiosFromTraits`, `wireGroup` (Task B1).
- Produces: every EN `ProviderCap` in the report carries the feed fields. EN providers are `hasContent=true` in Phase 1 (per-title emptiness is Phase 2's reactive cache).

- [ ] **Step 1: Write the failing test**

Add to `services/catalog/internal/service/capability/service_test.go` (uses the existing in-memory SQLite test DB helper in that file — mirror its setup):

```go
func TestBuildENFamilyPopulatesFeedFields(t *testing.T) {
	db := newTestDB(t) // existing helper in service_test.go
	must(t, db.Create(&domain.ScraperProvider{
		Name: "gogoanime", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "en", PreferenceWeight: 85, SupportsSub: true, SupportsDub: true,
		Reason: "live",
	}).Error)
	must(t, db.Create(&domain.ScraperProvider{
		Name: "animefever", Status: domain.StatusDegraded, Health: domain.HealthDown,
		Group: "en", PreferenceWeight: 60, SupportsSub: true, Reason: "ad-substitution",
	}).Error)

	svc := NewService(db, nil, nil, nil, nil)
	fam, err := svc.BuildENFamily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]domain.ProviderCap{}
	for _, p := range fam.Providers {
		byName[p.Provider] = p
	}
	gg := byName["gogoanime"]
	if gg.State != "active" || !gg.Selectable || gg.HackerOnly || gg.Order != 85 ||
		gg.Group != "en" || len(gg.Audios) != 2 {
		t.Fatalf("gogoanime feed fields wrong: %+v", gg)
	}
	af := byName["animefever"]
	if af.State != "degraded" || !af.Selectable || !af.HackerOnly || af.Reason != "ad-substitution" {
		t.Fatalf("animefever feed fields wrong: %+v", af)
	}
}
```

(If `newTestDB`/`must` helpers are named differently in `service_test.go`, reuse the existing ones — do not add duplicates.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestBuildENFamilyPopulatesFeedFields -count=1`
Expected: FAIL — feed fields are zero-valued (`State == ""`).

- [ ] **Step 3: Populate in the cap-build loop**

In `BuildENFamily` (`service.go`), replace the `caps = append(...)` block (currently lines ~150-159) with:

```go
		state, selectable, hackerOnly := deriveProviderView(row, true) // EN: hasContent=true in Phase 1
		caps = append(caps, domain.ProviderCap{
			Provider:    row.Name,
			DisplayName: displayName(row.Name),
			Enabled:     row.IsEnabled(),
			Degraded:    row.IsDegraded(),
			Health:      hstatus,
			Playable:    playable,
			Rank:        rankEN(row, hstatus, playable),
			State:       state,
			Selectable:  selectable,
			HackerOnly:  hackerOnly,
			Order:       row.PreferenceWeight,
			Group:       wireGroup(row.Group),
			Audios:      audiosFromTraits(row),
			Reason:      row.Reason,
			Variants:    variantsFromTraits(row),
		})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/capability/ -count=1`
Expected: PASS (the new test and all existing capability tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/capability/service.go services/catalog/internal/service/capability/service_test.go
git commit -m "feat(catalog): EN family emits state/selectable/order/group/audios

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B4: DB-row authority for the RU / Hanime families

**Files:**
- Modify: `services/catalog/internal/service/capability/families_ru.go` (kodik/animelib/hanime builders)
- Modify: `services/catalog/internal/service/capability/service.go` (`kodikFamily`/`animelibFamily`/`hanimeFamily` need DB-row access — add a small loader)
- Test: `services/catalog/internal/service/capability/families_ru_test.go`

**Interfaces:**
- Consumes: `deriveProviderView`, `audiosFromTraits`, `wireGroup`; a new helper `func (s *Service) providerRow(ctx, name string) (domain.ScraperProvider, bool)` loading one row by name.
- Produces: kodik/animelib/hanime caps carry the feed fields from their DB rows. A family whose DB row is `policy=disabled` (e.g. `animelib` today) is **omitted entirely**. The DB row names are `kodik-noads` (the served Kodik provider), `animelib`, `hanime` — map family→row name.

- [ ] **Step 1: Write the failing test**

Add to `families_ru_test.go`:

```go
func TestKodikFamilyOmittedWhenRowDisabled(t *testing.T) {
	db := newTestDB(t)
	must(t, db.Create(&domain.ScraperProvider{Name: "kodik-noads", Status: domain.StatusDisabled, Group: "ru"}).Error)
	s := &Service{db: db, catalog: fakeCatalog{kodik: []domain.KodikTranslation{{ID: 1, Title: "Team", Type: "voice"}}}}
	if _, ok := s.kodikFamily(context.Background(), "uuid"); ok {
		t.Fatal("kodik family must be omitted when its DB row is disabled")
	}
}

func TestKodikFamilyCarriesFeedFields(t *testing.T) {
	db := newTestDB(t)
	must(t, db.Create(&domain.ScraperProvider{
		Name: "kodik-noads", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "ru", PreferenceWeight: 50, SupportsSub: true, SupportsDub: true,
	}).Error)
	s := &Service{db: db, catalog: fakeCatalog{kodik: []domain.KodikTranslation{{ID: 1, Title: "Team", Type: "voice"}}}}
	fam, ok := s.kodikFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("expected kodik family")
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "ru" || p.Order != 50 {
		t.Fatalf("kodik feed fields wrong: %+v", p)
	}
}
```

(`fakeCatalog` already exists in `families_ru_test.go`. `db`/`newTestDB`/`must` — reuse the EN test helpers; if `families_ru_test.go` builds `Service{}` without a db, add `db` to those constructions.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'TestKodikFamily' -count=1`
Expected: FAIL — family still built without a DB-row check; feed fields empty.

- [ ] **Step 3: Add the row loader + wire it into the three builders**

Add to `families_ru.go`:

```go
// providerRow loads one stream_providers row by name. ok=false when absent.
func (s *Service) providerRow(ctx context.Context, name string) (domain.ScraperProvider, bool) {
	if s.db == nil {
		return domain.ScraperProvider{}, false
	}
	var row domain.ScraperProvider
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error; err != nil {
		return domain.ScraperProvider{}, false
	}
	return row, true
}

// applyFeedFields fills the feed presentation on a built provider cap from its
// DB row. Returns ok=false when the row is disabled (caller omits the family).
func applyFeedFields(cap *domain.ProviderCap, row domain.ScraperProvider) bool {
	if !row.IsRegistered() { // disabled → omit
		return false
	}
	state, selectable, hackerOnly := deriveProviderView(row, true)
	cap.State, cap.Selectable, cap.HackerOnly = state, selectable, hackerOnly
	cap.Order = row.PreferenceWeight
	cap.Group = wireGroup(row.Group)
	cap.Audios = audiosFromTraits(row)
	cap.Reason = row.Reason
	return true
}
```

In `kodikFamily`, after building `variants` and before the `return`, replace the `return domain.SourceFamily{...}, true` with:

```go
	row, ok := s.providerRow(ctx, "kodik-noads")
	if !ok {
		return domain.SourceFamily{}, false
	}
	cap := domain.ProviderCap{Provider: "kodik", DisplayName: "Kodik", Enabled: true, Health: "unknown", Variants: variants}
	if !applyFeedFields(&cap, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "kodik", Providers: []domain.ProviderCap{cap}}, true
```

Apply the same pattern in `animelibFamily` (row name `"animelib"`, `Provider: "animelib"`, `DisplayName: "AniLib"`) and `hanimeFamily` (row name `"hanime"`, `Provider: "hanime"`, `DisplayName: "Hanime"`, single raw variant).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/capability/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/capability/families_ru.go services/catalog/internal/service/capability/families_ru_test.go
git commit -m "feat(catalog): RU/Hanime families gated by DB row + feed fields

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B5: First-party `ae` (library-presence), `raw`, and `18anime` families

**Files:**
- Create: `services/catalog/internal/service/capability/families_firstparty.go`
- Modify: `services/catalog/internal/service/capability/service.go` (`buildFamilies` wiring + `CatalogSource` gains a library-presence method, OR a new `LibrarySource` interface)
- Test: `services/catalog/internal/service/capability/families_firstparty_test.go`

**Interfaces:**
- Consumes: `s.providerRow`, `applyFeedFields`, `deriveProviderView`. New interface:
  ```go
  type LibrarySource interface {
      HasLibraryTitle(ctx context.Context, animeID string) (bool, error)
  }
  ```
  Add `library LibrarySource` to `Service` (nil-safe; nil ⇒ `ae` treated as `no_content`). Wire a real impl backed by the library/streaming index in `cmd/catalog-api/main.go` (separate small task — see Step 6 note).
- Produces: `aeFamily`/`rawFamily`/`adultScraperFamily` builders; each returns `(domain.SourceFamily, bool)`. `ae` is `no_content` (tinted, not selectable) when the title is not in the library; `active` when present.

- [ ] **Step 1: Write the failing test**

```go
package capability

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

type fakeLibrary struct{ has bool; err error }

func (f fakeLibrary) HasLibraryTitle(context.Context, string) (bool, error) { return f.has, f.err }

func TestAeFamilyPresent(t *testing.T) {
	db := newTestDB(t)
	must(t, db.Create(&domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp,
		Group: "firstparty", PreferenceWeight: 100, SupportsSub: true, SupportsRaw: true,
	}).Error)
	s := &Service{db: db, library: fakeLibrary{has: true}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family expected")
	}
	p := fam.Providers[0]
	if p.State != "active" || !p.Selectable || p.Group != "firstparty" || p.Order != 100 {
		t.Fatalf("ae present feed wrong: %+v", p)
	}
}

func TestAeFamilyAbsentIsNoContent(t *testing.T) {
	db := newTestDB(t)
	must(t, db.Create(&domain.ScraperProvider{
		Name: "ae", Status: domain.StatusEnabled, Health: domain.HealthUp, Group: "firstparty",
	}).Error)
	s := &Service{db: db, library: fakeLibrary{has: false}}
	fam, ok := s.aeFamily(context.Background(), "uuid")
	if !ok {
		t.Fatal("ae family still emitted (tinted), not omitted")
	}
	if p := fam.Providers[0]; p.State != "no_content" || p.Selectable {
		t.Fatalf("ae absent must be no_content/not-selectable: %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestAeFamily -count=1`
Expected: FAIL — `s.library` undefined / `s.aeFamily` undefined.

- [ ] **Step 3: Implement**

Add `library LibrarySource` to the `Service` struct in `service.go` and to `NewService` (append param `library LibrarySource`; update all `NewService(...)` call sites to pass `nil` for now except `cmd/catalog-api/main.go` — see Step 6). Then create `families_firstparty.go`:

```go
package capability

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// LibrarySource reports whether AnimeEnigma has a title self-hosted.
type LibrarySource interface {
	HasLibraryTitle(ctx context.Context, animeID string) (bool, error)
}

// aeFamily builds the first-party "ae" family. The provider is always emitted
// (so the user sees it), but is `no_content` (tinted, not selectable) until the
// title is encoded into the library. Library lookup failures fall back to
// no_content rather than dropping the family.
func (s *Service) aeFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, "ae")
	if !ok || !row.IsRegistered() {
		return domain.SourceFamily{}, false
	}
	has := false
	if s.library != nil {
		if h, err := s.library.HasLibraryTitle(ctx, animeID); err == nil {
			has = h
		} else if s.log != nil {
			s.log.Warnw("ae library presence lookup failed; tinting", "anime_id", animeID, "error", err)
		}
	}
	cap := domain.ProviderCap{
		Provider: "ae", DisplayName: "AnimeEnigma", Enabled: true, Health: "up",
		Variants: variantsFromTraits(row),
	}
	state, selectable, hackerOnly := deriveProviderView(row, has)
	cap.State, cap.Selectable, cap.HackerOnly = state, selectable, hackerOnly
	cap.Order = row.PreferenceWeight
	cap.Group = wireGroup(row.Group)
	cap.Audios = audiosFromTraits(row)
	cap.Reason = row.Reason
	return domain.SourceFamily{Family: "ae", Providers: []domain.ProviderCap{cap}}, true
}
```

Wire `aeFamily` into `buildFamilies` (`service.go`) alongside kodik/animelib/hanime (add to the `slot` set and the best-effort append loop). Stable order: place `ae` first (before `ourenglish`) so first-party leads.

> **Scope note for `raw` + `18anime`:** these mirror `aeFamily`'s shape (DB-row-driven via `applyFeedFields`, presence from their existing per-title resolvers). Add `rawFamily` and `adultScraperFamily` as sibling builders in `families_firstparty.go` with the same pattern and their own table tests. They are independent steps; keep each its own commit. (`raw` presence = AllAnime raw resolver; `18anime` presence = the adult scraper orchestrator — reuse the existing resolver methods on `CatalogSource`/the scraper client; if a method is missing, the family returns `ok=false` until wired, which is acceptable for Phase 1 — the FE simply won't show it, same as today.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/capability/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/capability/families_firstparty.go services/catalog/internal/service/capability/families_firstparty_test.go services/catalog/internal/service/capability/service.go
git commit -m "feat(catalog): ae family with backend library-presence; raw/18anime DB-row families

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

- [ ] **Step 6: Wire the real `LibrarySource` + update `NewService` call sites**

In `services/catalog/cmd/catalog-api/main.go`, implement `HasLibraryTitle` against the existing library/streaming availability path (the same check `smartDefault.ts` used to make client-side — find the existing "is this anime in the library" lookup the streaming/library service exposes, e.g. an internal HTTP call or a DB index) and pass it to `NewService`. Update every other `NewService(...)` call site (tests included) to pass `nil` for `library`. Run `cd services/catalog && go build ./... && go test ./... -count=1`. Commit with the same co-author block.

---

## Frontend

### Task F1: Extend the FE `ProviderCap` feed type

**Files:**
- Modify: `frontend/web/src/types/capabilities.ts`
- Test: `frontend/web/src/types/__tests__/capabilities.spec.ts` (create if absent)

**Interfaces:**
- Produces: the FE `ProviderCap` interface gains `state: 'active'|'recovering'|'degraded'|'no_content'`, `selectable: boolean`, `hacker_only: boolean`, `order: number`, `group: 'en'|'ru'|'adult'|'jp'|'firstparty'`, `audios: ('sub'|'dub'|'raw')[]`, `reason?: string`. (Match the backend JSON keys exactly — snake_case.)

- [ ] **Step 1: Read the current type** — open `frontend/web/src/types/capabilities.ts`, locate the `ProviderCap` interface.

- [ ] **Step 2: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import type { ProviderCap } from '@/types/capabilities'

describe('ProviderCap feed fields', () => {
  it('accepts the Phase-1 feed shape', () => {
    const p: ProviderCap = {
      provider: 'gogoanime', display_name: 'GogoAnime',
      state: 'active', selectable: true, hacker_only: false,
      order: 85, group: 'en', audios: ['sub', 'dub'],
      variants: [],
    } as ProviderCap
    expect(p.state).toBe('active')
    expect(p.selectable).toBe(true)
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/types/__tests__/capabilities.spec.ts`
Expected: FAIL — TS error: `state` not assignable / unknown property.

- [ ] **Step 4: Add the fields** to `ProviderCap` in `frontend/web/src/types/capabilities.ts`:

```ts
  state: 'active' | 'recovering' | 'degraded' | 'no_content'
  selectable: boolean
  hacker_only: boolean
  order: number
  group: 'en' | 'ru' | 'adult' | 'jp' | 'firstparty'
  audios: ('sub' | 'dub' | 'raw')[]
  reason?: string
```

- [ ] **Step 5: Run test + typecheck**

Run: `cd frontend/web && bunx vitest run src/types/__tests__/capabilities.spec.ts && bunx tsc --noEmit`
Expected: PASS + no type errors.

- [ ] **Step 6: Commit** (co-author block).

---

### Task F2: Redefine `ProviderRow` and build it from the feed (`useProviderFeed`)

**Files:**
- Modify: `frontend/web/src/types/aePlayer.ts:30-53` (`ChipState`, `ProviderRow`; remove `ProviderDef`/`ScraperProviderHealth`)
- Create: `frontend/web/src/composables/aePlayer/useProviderFeed.ts`
- Create: `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts`

**Interfaces:**
- Produces:
  - `ChipState = 'active' | 'recovering' | 'degraded' | 'no_content'`
  - `ProviderRow = { id: string; label: string; group: ProviderGroup; state: ChipState; selectable: boolean; hackerOnly: boolean; order: number; audios: AudioKind[]; reason?: string }`
  - `function rowsFromReport(report: CapabilityReport, filter: RowFilter): ProviderRow[]` — flattens families→providers into rows, applies the audio/lang/content relevance filter, sorts by `order` desc.
- Consumes: `ProviderCap`/`CapabilityReport` from `@/types/capabilities`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { rowsFromReport } from '@/composables/aePlayer/useProviderFeed'
import type { CapabilityReport } from '@/types/capabilities'

const report: CapabilityReport = {
  anime_id: 'x',
  families: [
    { family: 'ourenglish', providers: [
      { provider: 'gogoanime', display_name: 'GogoAnime', state: 'active', selectable: true,
        hacker_only: false, order: 85, group: 'en', audios: ['sub', 'dub'], variants: [] },
      { provider: 'animefever', display_name: 'AnimeFever', state: 'degraded', selectable: true,
        hacker_only: true, order: 60, group: 'en', audios: ['sub'], variants: [], reason: 'ads' },
    ] },
  ],
} as unknown as CapabilityReport

describe('rowsFromReport', () => {
  it('flattens, sorts by order desc, carries state', () => {
    const rows = rowsFromReport(report, { audio: 'sub', lang: 'en', content: 'common' })
    expect(rows.map(r => r.id)).toEqual(['gogoanime', 'animefever'])
    expect(rows[0].state).toBe('active')
    expect(rows[1].hackerOnly).toBe(true)
  })
  it('disabled providers never appear (backend omits them)', () => {
    // animepahe is policy=disabled → absent from the report → absent from rows
    expect(rowsFromReport(report, { audio: 'sub', lang: 'en', content: 'common' })
      .find(r => r.id === 'animepahe')).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Redefine the types + implement**

In `frontend/web/src/types/aePlayer.ts`, replace `ChipState`, `ProviderRow`, and delete `ProviderDef` + `ScraperProviderHealth`:

```ts
export type ChipState = 'active' | 'recovering' | 'degraded' | 'no_content'

/** A provider as rendered in the Source panel — fields come straight from the
 *  backend capability feed (single source of truth). No FE-side registry. */
export interface ProviderRow {
  id: string
  label: string
  group: ProviderGroup
  state: ChipState
  selectable: boolean
  hackerOnly: boolean
  order: number
  audios: AudioKind[]
  reason?: string
}
```

Update `ProviderGroup` to match the wire: `export type ProviderGroup = 'en' | 'ru' | 'adult' | 'jp' | 'firstparty'`.

Create `useProviderFeed.ts`:

```ts
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow, AudioKind, TrackLang, ContentKind, ProviderGroup } from '@/types/aePlayer'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

// Backend groups → which (audio,lang,content) they serve. Group is the wire
// group from the DB; relevance still filters by the active toggle so the menu
// shows only providers that can serve the current combo.
const GROUP_LANGS: Record<ProviderGroup, TrackLang[]> = {
  en: ['en'], ru: ['ru'], adult: ['en', 'ru'], jp: ['ja'], firstparty: ['en', 'ru', 'ja'],
}
const GROUP_CONTENT: Record<ProviderGroup, ContentKind[]> = {
  en: ['common'], ru: ['common'], adult: ['hentai'], jp: ['common'], firstparty: ['common'],
}

function relevant(cap: ProviderCap, f: RowFilter): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  return cap.audios.includes(f.audio) && GROUP_LANGS[g].includes(f.lang) && GROUP_CONTENT[g].includes(f.content)
}

function toRow(cap: ProviderCap): ProviderRow {
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios: cap.audios.filter((a): a is AudioKind => a === 'sub' || a === 'dub'),
    reason: cap.reason,
  }
}

/** Flatten the capability report into rendered rows, relevance-filtered and
 *  sorted by backend `order` (desc). The backend already omitted disabled
 *  providers, so anything here is a real, backend-sanctioned source. */
export function rowsFromReport(report: CapabilityReport, filter: RowFilter): ProviderRow[] {
  const rows: ProviderRow[] = []
  for (const fam of report.families) {
    for (const cap of fam.providers) {
      if (relevant(cap, filter)) rows.push(toRow(cap))
    }
  }
  return rows.sort((a, b) => b.order - a.order)
}
```

- [ ] **Step 4: Run test + typecheck**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts && bunx tsc --noEmit`
Expected: vitest PASS. `tsc` will now report errors in `ProviderChip.vue`/`SourcePanel.vue`/`AePlayer.vue`/`useProviderHealth.ts` — those are fixed in F3–F5; this step's gate is the spec PASS + no NEW errors in `useProviderFeed.ts`/`aePlayer.ts`.

- [ ] **Step 5: Commit** (co-author block).

---

### Task F3: Render `ProviderChip` from the feed; color by state

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/ProviderChip.vue`
- Modify/Create: `frontend/web/src/components/player/aePlayer/ProviderChip.spec.ts`

**Interfaces:**
- Consumes: the new `ProviderRow` (F2). `selectable` now comes from the row (`row.selectable && (!row.hackerOnly || hackerMode)`), not recomputed from registry state.
- Produces: a chip that reads `row.label`/`row.state`/`row.reason`; the identity-hue dot is replaced by a state-colored dot.

- [ ] **Step 1: Write the failing test** — assert: disabled providers can't reach the chip (N/A — omitted upstream); a `no_content` row renders tinted + not selectable; a `degraded` row is selectable only in hacker mode; the state dot uses the state token class.

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow } from '@/types/aePlayer'

const base: ProviderRow = { id: 'gogoanime', label: 'GogoAnime', group: 'en', state: 'active',
  selectable: true, hackerOnly: false, order: 85, audios: ['sub'] }

const stub = { global: { mocks: { $t: (k: string) => k } } }

describe('ProviderChip', () => {
  it('no_content is tinted and not selectable', () => {
    const w = mount(ProviderChip, { props: { row: { ...base, state: 'no_content', selectable: false, reason: 'No episodes' } }, ...stub })
    expect(w.find('button').attributes('disabled')).toBeDefined()
    expect(w.find('[data-test=cap-nocontent]').exists()).toBe(true)
  })
  it('degraded selectable only in hacker mode', async () => {
    const row = { ...base, state: 'degraded' as const, hackerOnly: true }
    const off = mount(ProviderChip, { props: { row, hackerMode: false }, ...stub })
    expect(off.find('button').attributes('disabled')).toBeDefined()
    const on = mount(ProviderChip, { props: { row, hackerMode: true }, ...stub })
    expect(on.find('button').attributes('disabled')).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/ProviderChip.spec.ts`
Expected: FAIL (template still references `row.def.*`).

- [ ] **Step 3: Rewrite the chip** — key changes (apply to `ProviderChip.vue`):
  - Props: `row: ProviderRow`, keep `selected?`, `cap?: ProviderCap`, `best?`, `hackerMode?`.
  - `selectable` computed: `props.row.selectable && (!props.row.hackerOnly || props.hackerMode === true)`.
  - Replace `row.def.name` → `row.label`; remove `row.def.blurb` block (blurb retired; `reason` shown via `:title` already).
  - Replace the identity-hue dot (`:style="{ background: row.def.hue, ... }"`) with a state-colored dot using a class map:

```vue
      <span
        class="flex-shrink-0 w-[9px] h-[9px] rounded-full"
        :class="dotClass"
        aria-hidden="true"
      />
```
```ts
const DOT: Record<ProviderRow['state'], string> = {
  active: 'bg-[var(--brand-cyan)]',
  recovering: 'bg-lime-400',
  degraded: 'bg-warning',
  no_content: 'bg-[var(--muted-foreground)]',
}
const dotClass = computed(() => DOT[props.row.state])
```
  - State badges: keep `recovering`/`degraded`; remove `wip`/`down`; add `no_content`:

```vue
      <span
        v-else-if="row.state === 'no_content'"
        data-test="cap-nocontent"
        class="ml-auto flex-shrink-0 text-[10px] font-semibold uppercase tracking-wide text-[var(--muted-foreground)] font-mono"
      >{{ $t('player.sources.noContent') }}</span>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/ProviderChip.spec.ts`
Expected: PASS.

- [ ] **Step 5: Add i18n key** — add `player.sources.noContent` to `en.json` ("No episodes"), `ru.json` ("Нет серий"), `ja.json` ("エピソードなし"). (`recovering`/`degraded` keys already exist.)

- [ ] **Step 6: Commit** (co-author block).

---

### Task F4: Update `SourcePanel` to the new `ProviderRow` + state rank

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SourcePanel.vue`
- Modify: its spec (find `SourcePanel.spec.ts`/`__tests__`).

**Interfaces:**
- Consumes: `ProviderRow[]` (new shape) via the `rows` prop. `r.def.id` → `r.id` throughout; `STATE_RANK` keyed on the new `ChipState`.

- [ ] **Step 1: Write/adjust the failing test** — assert sorting: `active` before `recovering` before `degraded` before `no_content`; `activeCount` counts `state==='active'`.

```ts
// in SourcePanel spec — rows now carry id/state/order directly
const rows = [
  { id: 'a', label: 'A', group: 'en', state: 'no_content', selectable: false, hackerOnly: false, order: 10, audios: ['sub'] },
  { id: 'b', label: 'B', group: 'en', state: 'active', selectable: true, hackerOnly: false, order: 20, audios: ['sub'] },
]
// expect visible order to put 'b' (active) before 'a' (no_content)
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/` (the SourcePanel spec)
Expected: FAIL (references `r.def.id`).

- [ ] **Step 3: Apply the changes**
  - `STATE_RANK`:
```ts
const STATE_RANK: Record<ChipState, number> = {
  active: 0, recovering: 1, degraded: 2, no_content: 3,
}
```
  - Template: `:key="r.id"`, `:row="r"`, `:cap="capMap.get(r.id)"`, `:best="... r.id === topRow?.id"`, `:selected="r.id === provider"`, `@select="emit('select-provider', r.id)"`.
  - `pos(id)`, `topRow`, `collapsedRows`, `activeRows` filters: `r.def.id` → `r.id`.
  - `sortedRows` may drop `rankedIds` tiebreak in favor of `order` (rows already arrive `order`-sorted from `rowsFromReport`); keep the `STATE_RANK` primary sort, use `b.order - a.order` (or input order) as the tiebreak. Remove the `rankedIds` prop if no longer used (and its provider in `AePlayer.vue`).

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/`
Expected: PASS.

- [ ] **Step 5: Commit** (co-author block).

---

### Task F5: Switch `AePlayer.vue` + `smartDefault` to the feed; delete the registry

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/composables/aePlayer/smartDefault.ts` (+ its spec)
- Delete: `frontend/web/src/components/player/aePlayer/providerRegistry.ts`
- Delete: `frontend/web/src/composables/aePlayer/useProviderHealth.ts` (+ its spec)

**Interfaces:**
- Consumes: `rowsFromReport` (F2), the capability report `AePlayer` already fetches for `capMap`/ranking.
- Produces: `AePlayer` builds `rows` from the report; `smartDefault(rows)` picks the highest-`order` row whose `state === 'active'`.

- [ ] **Step 1: Write the failing test** for `smartDefault`:

```ts
import { describe, it, expect } from 'vitest'
import { pickSmartDefault } from '@/composables/aePlayer/smartDefault'
import type { ProviderRow } from '@/types/aePlayer'

const rows: ProviderRow[] = [
  { id: 'ae', label: 'AnimeEnigma', group: 'firstparty', state: 'no_content', selectable: false, hackerOnly: false, order: 100, audios: ['sub'] },
  { id: 'gogoanime', label: 'GogoAnime', group: 'en', state: 'active', selectable: true, hackerOnly: false, order: 85, audios: ['sub'] },
]
describe('pickSmartDefault', () => {
  it('skips no_content ae, picks highest-order active', () => {
    expect(pickSmartDefault(rows)?.id).toBe('gogoanime')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/smartDefault.spec.ts`
Expected: FAIL (signature mismatch — `pickSmartDefault` not exported / takes registry).

- [ ] **Step 3: Rewrite `smartDefault.ts`**

```ts
import type { ProviderRow } from '@/types/aePlayer'

/** Pick the default provider: the highest-`order` row that is actually
 *  selectable and `active`. Rows arrive pre-sorted by order, but sort defensively.
 *  Returns null when nothing is active (caller shows the empty/error state). */
export function pickSmartDefault(rows: ProviderRow[]): ProviderRow | null {
  return [...rows]
    .filter(r => r.state === 'active' && r.selectable)
    .sort((a, b) => b.order - a.order)[0] ?? null
}
```

Update `AePlayer.vue`:
  - Remove imports of `PROVIDER_REGISTRY`, `providerById`, `CURATED_TIER`, `useProviderHealth`.
  - Build `rows` via `rowsFromReport(report.value, filter.value)` (the report is already fetched for `capMap`); make it a `computed`/watched value off the existing capability fetch.
  - Replace the smart-default call with `pickSmartDefault(rows.value)`.
  - Drop the `rankedIds` prop passed to `SourcePanel` if F4 removed it.

- [ ] **Step 4: Delete the registry + old health composable**

```bash
git rm frontend/web/src/components/player/aePlayer/providerRegistry.ts
git rm frontend/web/src/composables/aePlayer/useProviderHealth.ts
# delete their specs too if present:
git rm frontend/web/src/composables/aePlayer/useProviderHealth.spec.ts 2>/dev/null || true
```

- [ ] **Step 5: Run the full FE check**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/ src/composables/aePlayer/ && bunx tsc --noEmit`
Expected: PASS + **zero** type errors (no dangling registry references). Fix any remaining `providerRegistry`/`useProviderHealth`/`row.def` references the compiler surfaces.

- [ ] **Step 6: Commit** (co-author block).

---

### Task F6: DS-lint allowlist cleanup + frontend-verify

**Files:**
- Modify: `frontend/web/scripts/design-system-allowlist.txt` / `design-system-spacing-allowlist.txt` (remove now-dead `providerRegistry.ts` hue entries; remove any `ProviderChip.vue` hue-dot entries no longer used).

**Interfaces:** none.

- [ ] **Step 1: Remove dead allowlist lines** — grep the allowlists for `providerRegistry.ts` and the removed identity-hue hex usages; delete those lines (a stale allowlist line is itself a lint error per `project_ds_allowlist_pathcheck_i18n_placeholder`).

Run: `grep -n 'providerRegistry' frontend/web/scripts/design-system-allowlist.txt frontend/web/scripts/design-system-spacing-allowlist.txt`

- [ ] **Step 2: Run DS-lint**

Run: `bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS: 0`.

- [ ] **Step 3: Run `/frontend-verify`** (DS-lint + i18n en/ru/ja parity + real `bun run build`).
Expected: all gates green.

- [ ] **Step 4: Commit** (co-author block).

---

### Task F7: Backend integration check + end-to-end smoke

**Files:** none (verification task).

- [ ] **Step 1: Rebuild + redeploy catalog**

Run: `make redeploy-catalog && make health`
Expected: catalog healthy.

- [ ] **Step 2: Verify the feed omits disabled + carries state**

Run (use the Witch-Hat anime UUID from the design session, or any populated title):
```bash
curl -s "http://localhost:8000/api/anime/<uuid>/capabilities" | python3 -c '
import sys,json
d=json.load(sys.stdin)["data"]
for fam in d["families"]:
    for p in fam["providers"]:
        print(fam["family"], p["provider"], p.get("state"), p.get("selectable"), p.get("hacker_only"), p.get("order"), p.get("group"))'
```
Expected: NO `animepahe`/`animelib`/`animekai` rows (policy=disabled, omitted); `gogoanime` `state=active selectable=true`; degraded providers `hacker_only=true`; `ae` present with `state` reflecting library presence.

- [ ] **Step 3: Confirm the original bug is closed** — open the player for an anime whose `animepahe` is disabled; the Source list must NOT show AnimePahe as selectable (it should be absent entirely). gogoanime shows its real DB state.

- [ ] **Step 4: Commit** any notes; this task has no code.

---

## Self-Review

**Spec coverage (Phase 1 scope only):**
- "One authority: extend `/capabilities`" → B1–B5 (feed fields + per-family wiring). ✓
- "`policy=disabled` omitted entirely" → B3/B4/B5 (EN already filters `status<>disabled`; RU/adult/firstparty omit via `IsRegistered()`); F2 test asserts absence. ✓
- "degraded → hacker-only" → `deriveProviderView` (B1) + ProviderChip selectable gate (F3). ✓
- "cosmetics status-driven, no per-provider hue" → F3 state-dot map; registry hue deleted (F5). ✓
- "library presence backend-side for `ae`" → B5 `LibrarySource`. ✓
- "FE deletes providerRegistry/CURATED_TIER/staticDisabled, renders feed" → F2/F4/F5. ✓
- OUT of Phase 1 (correctly deferred): reactive `no_content` cache (Phase 2 — `deriveProviderView` already accepts `hasContent` so Phase 2 just feeds it), notifications purge (Phase 3), resurrection dashboard (Phase 4). ✓

**Placeholder scan:** `raw`/`18anime` family builders in B5 Step 3 are described as "mirror `aeFamily`'s pattern" with explicit row names + the shared `applyFeedFields` helper shown in B4 — the real code is the helper; each family is a one-call application. The `LibrarySource` real impl (B5 Step 6) points at the existing library lookup `smartDefault.ts` used; the executor must locate that exact path. These are the two spots requiring the executor to read existing code — flagged explicitly, not hidden.

**Type consistency:** `deriveProviderView(row, hasContent) (state, selectable, hackerOnly)` is used identically in B3/B4/B5. FE `ProviderRow` fields (`id/label/group/state/selectable/hackerOnly/order/audios/reason`) match across F2/F3/F4/F5. Wire JSON keys are snake_case (`hacker_only`, `display_name`) on the backend (B2) and consumed as such in the FE `ProviderCap` (F1) then mapped to camelCase `ProviderRow` in `toRow` (F2).

## Effort & Impact

- **UXΔ = +3 (Better)** — the player and ops dashboard finally agree; disabled providers vanish, degraded is honestly hacker-gated, state is truthful.
- **CDI = 0.05 × 21** — spread (catalog capability pkg + 4 FE components) × shift (FE loses its provider brain; `/capabilities` becomes load-bearing) × Effort_Fib 21. Not pre-multiplied.
- **MVQ = Griffin 88% / 82%** — disciplined consolidation onto an existing endpoint; slop-resistance from the pure `deriveProviderView` + table tests + the explicit "disabled never reaches the FE" assertion.

---

## FE Re-Decomposition (R1–R4) — supersedes F5–F7

> **Why:** F1–F4's leaf pieces shipped (committed WIP `01e31c94`: `useProviderFeed`, types,
> `ProviderChip`, `SourcePanel`, `smartDefault` rewritten to sync 1-arg, `useCapabilities`,
> `deepLinkProvider`, i18n). The holistic F5 then **timed out** — the AePlayer.vue rewire +
> deletions are wider than F5 captured: AePlayer.vue still calls the OLD **async 3-arg**
> `pickSmartDefault(rows, orderedIds, opts)` at 3 sites, and four FE-brain modules
> (`providerRegistry.ts`, `useProviderHealth.ts`, `useProviderAvailability.ts`,
> `rankedProviderIds.ts`) + their specs + 3 AePlayer integration specs all break under the new
> flat `ProviderRow`. R1–R4 replace F5/F6/F7 with bounded, independently-reviewable tasks.

**Decisions locked (the backend now owns these — delete the FE equivalents):**
- `ae` library presence → backend `state:'no_content'` (B5). Delete the FE `aeApi` probe
  (`isProviderAvailable`/`aeAvailable`/`aeAvailableCache`) and the `displayRows` `ae` special-case.
- Provider ordering → backend `order`. Delete `rankedProviderIds.ts` + `orderedProviderIds` +
  `CURATED_TIER`; `rowsFromReport` and `pickSmartDefault` both sort by `order` desc.
- Degraded/recovering gating → backend `selectable`/`hacker_only`. Delete `isCapPlayable` + the
  `isPlayable`/`needsCheck`/`isAvailable` smart-default options.
- Hacker-mode per-title availability overlay (`useProviderAvailability` + `overlayAvailability` +
  `scraperProviderIds` + `markCdnUnreachable`/`checkExists`) → **deleted for Phase 1** (it's pure
  FE-brain; the backend reactive `no_content` cache is Phase 2). **Failover is unaffected** — it
  gates on the `triedSources`/`triedWithCurrent` Sets, NOT on availability; `markCdnUnreachable`
  only fed the display overlay.
- Per-provider brand hue → `activeProviderHue` returns the single token `'var(--brand-cyan)'`
  (cosmetics are state-driven, not per-provider).

### Task R1: Rewire `AePlayer.vue` to the feed; delete the four FE-brain modules

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (~2805 lines; edits localized)
- Delete: `frontend/web/src/components/player/aePlayer/providerRegistry.ts`
- Delete: `frontend/web/src/composables/aePlayer/useProviderHealth.ts` + `useProviderHealth.spec.ts`
- Delete: `frontend/web/src/composables/aePlayer/useProviderAvailability.ts` + `useProviderAvailability.spec.ts`
- Delete: `frontend/web/src/composables/aePlayer/rankedProviderIds.ts` + `rankedProviderIds.spec.ts`

**Interfaces:**
- Consumes: `rowsFromReport(report, filter)` from `@/composables/aePlayer/useProviderFeed`;
  `pickSmartDefault(rows): ProviderRow | null` from `@/composables/aePlayer/smartDefault`;
  `useCapabilities(animeIdRef)` → `{ report, capMap }`. New flat `ProviderRow` (`r.id`/`r.label`/
  `r.group`/`r.state`/`r.selectable`/`r.hackerOnly`/`r.order`/`r.audios`/`r.reason` — **no `r.def`**).
- Produces: AePlayer.vue with zero references to the four deleted modules and zero `r.def` usages.

**Symbol-fate table (every current AePlayer.vue reference — all line numbers approximate):**

| Current (≈line) | Action |
|---|---|
| `import { useProviderHealth }` (353) | **remove** |
| `import { useProviderAvailability, overlayAvailability }` (354) | **remove** |
| `import { providerById, CURATED_TIER, PROVIDER_REGISTRY } from './providerRegistry'` (358) | **remove** |
| `import { pickSmartDefault } from '…/smartDefault'` (359) | keep |
| `import { useCapabilities } from '…/useCapabilities'` (361) | keep |
| `import { rankedProviderIds } from '…/rankedProviderIds'` (362) | **remove** |
| `import { aeApi } from '@/api/client'` (365) | **remove** (only used by the deleted `ae` probe — confirm no other use first; if used elsewhere, keep) |
| `scraperProviderIds = new Set(PROVIDER_REGISTRY…)` (445) | **remove** |
| `const { rows, recompute: recomputeRows, start } = useProviderHealth(filter)` (588) | **replace** (see below) |
| `useCapabilities` destructure `{ capMap, rankedIds: capRankedIds }` (595) | change to `{ report, capMap }` |
| `orderedProviderIds` computed (596–598) | **remove** |
| `availability = useProviderAvailability(...)` (599) | **remove** |
| `aeAvailable`/`aeAvailableCache`/`isProviderAvailable` (610–625) | **remove** |
| `displayRows` computed (632–650) | **remove** — pass `rows` straight to SourcePanel |
| animeId watcher body: `aeAvailableCache=null`/`aeAvailable.value=null`/`availability.reset()`/`void isProviderAvailable('ae')` (679–685) | **remove those 4 lines** (keep the rest of the watcher) |
| `pickSmartDefault(...)` site A — failover (734–738) | **rewrite** (see below) |
| `availability.markCdnUnreachable(provider)` + its `if (scraperProviderIds.has(provider))` (743–745) | **remove** |
| `pickSmartDefault(...)` site B — auto-select watcher (916–944) | **rewrite** (see below) |
| `AE_NEEDS_CHECK` (809) + `isCapPlayable` (816) | **remove** |
| `activeProviderDef`/`activeProviderName`/`activeProviderHue` (948–958) | **rewrite** (see below) |
| `pickSmartDefault(...)` site C — facet repick (1471–1495) | **rewrite** (see below) |
| `recomputeRows()` (1471) | **remove** (rows is a computed — auto-reactive) |
| `availability.checkExists(id)` + its guard (1504–1506) | **remove** |
| `start()` + `void isProviderAvailable('ae')` in onMounted (≈2491) | **remove both lines** |
| template `:rows="displayRows"` (217) | → `:rows="rows"` |
| template `:ranked-ids="orderedProviderIds"` (226) | **remove** (SourcePanel no longer accepts it) |

**New code (exact):**

`rows` (replaces line 588) — derive purely from the report + existing `filter` computed:
```ts
const animeIdRef = computed(() => props.animeId)
const { report, capMap } = useCapabilities(animeIdRef)
const rows = computed<ProviderRow[]>(() => rowsFromReport(report.value, filter.value))
```

Smart-default **site A** (failover — replaces 734–738):
```ts
    const triedProviders = new Set([...triedWithCurrent].map((k) => k.split(':')[0]))
    toProvider = pickSmartDefault(rows.value.filter((r) => !triedProviders.has(r.id)))?.id ?? null
```

Smart-default **site B** (auto-select watcher — replaces 915–944):
```ts
watch(
  [rows, preferenceSettled],
  () => {
    if (roomHasCombo.value) return
    if (state.combo.value.provider) return
    if (!preferenceSettled.value) return // let saved prefs (audio/lang) settle first
    const pick = pickSmartDefault(rows.value)
    if (pick && !state.combo.value.provider) {
      providerAutoSelected.value = true
      state.setProvider(pick.id, '')
      recordDecision('smart default — best available source')
    }
  },
  { immediate: true },
)
```

Smart-default **site C** (facet repick — replaces 1484–1495, and drop `recomputeRows()` at 1471,
change `r.def.id` → `r.id` at 1473):
```ts
  const pick = pickSmartDefault(rows.value)
  if (!pick) {
    sourceError.value = 'No source for this language / audio'
    return
  }
  providerAutoSelected.value = true
  state.setProvider(pick.id, '') // provider watcher re-lists episodes + refreshes teams
  recordDecision('re-picked best source for the new audio / language')
```

Active-provider display (replaces 948–958) — name from the feed, hue is a single token:
```ts
const activeProviderName = computed(() => {
  const id = state.combo.value.provider
  return capMap.value.get(id)?.display_name ?? rows.value.find((r) => r.id === id)?.label ?? id ?? ''
})
const activeProviderHue = computed(() => 'var(--brand-cyan)')
```

**Steps:**
- [ ] **Step 1** — Read AePlayer.vue's provider regions (≈340–380, 440–450, 580–700, 720–770, 800–960, 1465–1510, 2485–2495). Confirm `aeApi` has no use outside the deleted `ae` probe (`grep -n 'aeApi' AePlayer.vue`); if it does, keep its import.
- [ ] **Step 2** — Apply every row of the symbol-fate table + the four "New code" blocks. Work top-down through the file.
- [ ] **Step 3** — `git rm` the seven dead files (4 modules + 3 specs):
```bash
cd frontend/web
git rm src/components/player/aePlayer/providerRegistry.ts \
       src/composables/aePlayer/useProviderHealth.ts \
       src/composables/aePlayer/useProviderHealth.spec.ts \
       src/composables/aePlayer/useProviderAvailability.ts \
       src/composables/aePlayer/useProviderAvailability.spec.ts \
       src/composables/aePlayer/rankedProviderIds.ts \
       src/composables/aePlayer/rankedProviderIds.spec.ts
```
- [ ] **Step 4** — Verify the source migration. The ONLY remaining type errors must be in the three
  AePlayer integration specs (handed to R2):
```bash
cd frontend/web && bunx vue-tsc --noEmit 2>&1 | grep -vE "AePlayer\.(urlsync|room|subtitles)\.spec" | grep -E "error TS" || echo "SOURCE CLEAN"
```
  Expected: `SOURCE CLEAN` (no error lines outside those 3 specs). Also confirm no dangling refs:
```bash
grep -rnE "providerRegistry|useProviderHealth|useProviderAvailability|rankedProviderIds|\.def\b|CURATED_TIER|PROVIDER_REGISTRY" src --include=*.vue --include=*.ts | grep -v "\.spec\."
```
  Expected: empty (every non-spec reference gone).
- [ ] **Step 5** — Run the player logic specs that do NOT import the deleted modules, to confirm no regression:
```bash
cd frontend/web && bunx vitest run src/composables/aePlayer/ src/components/player/aePlayer/ProviderChip.spec.ts src/components/player/aePlayer/SourcePanel.spec.ts 2>&1 | tail -20
```
  Expected: green (the deleted modules' specs are gone; the 3 AePlayer integration specs are R2).
- [ ] **Step 6** — Commit (co-author block): `refactor(aePlayer): render Source list from /capabilities feed; delete FE provider registry/health/availability/ranking`.

### Task R2: Migrate the three AePlayer integration specs; full green gate

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.urlsync.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.room.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`

**Interfaces:**
- These specs currently `vi.mock('@/composables/aePlayer/useProviderHealth', …)` (and may import
  `providerRegistry`) to inject provider rows into a mounted AePlayer. The composable no longer
  exists. Re-point them at the feed: mock `@/composables/aePlayer/useCapabilities` (or
  `@/api/client`'s `capabilitiesApi.get`) to return a `CapabilityReport`, so `rowsFromReport`
  produces the rows the test needs. The flat `ProviderRow`/`ProviderCap` shapes (see R1/F1/F2) are
  the contract.

**Steps:**
- [ ] **Step 1** — For each of the 3 specs, read its current `useProviderHealth`/`providerRegistry`
  mock and the assertions it drives. Identify which provider rows each test depends on.
- [ ] **Step 2** — Replace the `useProviderHealth` mock with a `useCapabilities` mock returning a
  `CapabilityReport` whose `families[].providers[]` carry the needed `state`/`selectable`/
  `hacker_only`/`order`/`group`/`audios` so `rowsFromReport` yields equivalent rows. Drop any
  `providerRegistry` import. Keep every behavioral assertion (url-sync, room-combo suppression,
  subtitle wiring) intact — only the provider-injection mechanism changes.
- [ ] **Step 3** — Full FE green gate:
```bash
cd frontend/web && bunx vue-tsc --noEmit && bunx vitest run src/components/player/aePlayer/ src/composables/aePlayer/ src/types/
```
  Expected: zero type errors; all specs pass.
- [ ] **Step 4** — Commit (co-author block): `test(aePlayer): drive integration specs off the capability feed`.

### Task R3: DS-lint allowlist cleanup + `bun run build` + i18n parity (was F6)

**Files:**
- Modify: `frontend/web/scripts/design-system-allowlist.txt` (remove the now-dead AePlayer
  `#00d4ff` line — line ≈90 `activeProviderHue` — since `activeProviderHue` now returns
  `var(--brand-cyan)` with no hex literal).
- Possibly Modify: `frontend/web/scripts/design-system-spacing-allowlist.txt` (only if R1/F3 removed
  the `ProviderChip.vue` `gap-[5px]`/`py-[9px]` classes; otherwise leave).

**Steps:**
- [ ] **Step 1** — `grep -n '#00d4ff' src/components/player/aePlayer/AePlayer.vue` → expect empty
  (R1 removed it). Remove the matching allowlist line(s) for AePlayer.vue `#00d4ff`. A stale
  allowlist line is itself a lint error (`project_ds_allowlist_pathcheck_i18n_placeholder`).
- [ ] **Step 2** — `bash scripts/design-system-lint.sh` → expect `ERRORS: 0`.
- [ ] **Step 3** — Run `/frontend-verify` (DS-lint + i18n en/ru/ja parity + real `bun run build`).
  The three `player.sources.noContent` keys must exist in en/ru/ja (added in the WIP). Expected: all green.
- [ ] **Step 4** — Commit (co-author block): `chore(ds): drop dead AePlayer hue allowlist line; verify provider-SoT build`.

### Task R4: Deploy catalog + end-to-end smoke (was F7)

**Files:** none (verification + deploy).

- [ ] **Step 1** — `make redeploy-catalog && make health` → catalog healthy.
- [ ] **Step 2** — Curl `/capabilities` for a populated title; confirm disabled providers
  (animepahe/animelib/animekai) are ABSENT, `gogoanime` is `state=active selectable=true`, degraded
  providers carry `hacker_only=true`, and `ae` carries a `state` reflecting library presence.
- [ ] **Step 3** — Open the player for an anime whose `animepahe` is policy=disabled: the Source
  list must NOT show AnimePahe (absent, not merely greyed); gogoanime shows its real DB state. This
  closes the original disagreement.
- [ ] **Step 4** — `/animeenigma-after-update` (changelog + final redeploy of any remaining services + push).
