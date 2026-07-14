# AUTO-608 Provider Existence Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `stream_providers.name` (the DB PK) the universal provider key: adding/enabling a row makes the provider exist across capabilities, scraper config, analytics telemetry, probes, and notifications automatically; a row with no code becomes an observable "unwired" gauge, never a config failure or silently dropped data.

**Architecture:** Each irreducible code seam (Go constructors, capability family builders, probe resolvers, FE adapters) becomes a registry keyed by DB `name`; DB rows drive membership; unknown rows fall to a generic default (capabilities) or a skip+gauge (scraper/probe). Three separately-deployable phases: P1 backend existence core, P2 player-key namespace bridge, P3 FE.

**Tech Stack:** Go (GORM/Postgres, promauto via libs/metrics), Vue 3 + TS (vitest, bun), Grafana provisioning YAML/JSON.

**Spec:** `docs/superpowers/specs/2026-07-14-auto608-provider-existence-wiring-design.md`

## Deviations from spec (verified against code 2026-07-14 — flag to owner, already justified)

1. **Scraper constructor block is NOT converted to DB-ordered registry iteration.** Registration is *already* DB-membership-driven (`registerByStatus` consults `cfg.Providers.Status(name)` per provider; disabled rows are skipped). Iterating by `preference_weight` would CHANGE the hand-tuned failover order (weights: miruro 70 > nineanime 40 > allanime-okru 35 > animepahe 30, vs code order gogoanime→animepahe→allanime-okru→miruro→nineanime) — and the same column drives FE panel ordering, so it can't be rebalanced freely. Instead: fail-open remote loader + `provider_unwired` gauge + roster-metrics reflection from live config rows.
2. **Probe seam gets no unwired gauge.** Non-scraper rows without probe resolvers (kodik-iframe, hanime, animelib, 18anime) are *intentionally* unprobeable/unprobed — a permanent gauge would be alert noise. Roster-driven membership + boot log line instead.
3. **Catalog capability seam needs no unwired gauge** — the generic `dbRowFamily` default means EVERY row builds something. That IS the "always wired" win.
4. **Grafana "Wired column"** on the Postgres roster table is impossible (different datasource) → separate Prometheus-datasource panel + alert on `provider_unwired`.
5. **`anime_level` column semantic** = "supports translation-less (any-team) new-episode lookup" (matches the hotcombos IN-list: english, ae, kodik, animelib, animejoy legs), NOT the narrower `anime_level_episodes.go` dispatch switch (which stays code — it maps player→resolver func and needs code per player anyway).
6. **Roster-derived hotcombos will drop disabled rows' player_keys** (e.g. animelib, status=disabled, leaves the allowed set). The old literal list ignored status. This is intended — roster truth.

## Global Constraints

- **NO time-effort units** — metrics are UXΔ/CDI/MVQ only (spec header has them).
- Commits: always pathspec (`git commit -m ... -- <paths>`), co-authors exactly:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>` / `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>` / `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`. Push after every commit (`git push origin HEAD:main`, pull-rebase first if rejected).
- Never run `gofmt -w` / `make fmt` (smart-quote landmine); format only the files you touched via targeted `gofmt -l` check.
- Frontend: `bun`/`bunx` only. Any `frontend/web` change must pass `/frontend-verify` before landing (DS-lint runs automatically on edit via hook).
- Go tests: `go test ./...` from the service dir. FE tests: `cd frontend/web && bun run test:unit -- <pattern>` (vitest; check package.json for the exact script name — `bunx vitest run <path>` also works).
- libs/metrics: extend the EXISTING module (no new go.work module, no Dockerfile sweep). Follow the promauto package-var pattern of `libs/metrics/roster.go`.
- Envelope decode: every catalog internal endpoint wraps responses `{"success":true,"data":{...}}` — decode `data.providers`, NOT root (ISS-032 class bug).
- The `"group"` column name must stay double-quoted in raw SQL (reserved word; SQLite tests + Postgres prod).
- Do not touch `intrinsicGroups` / `GroupOf` / `scraperOperatedNames` (security: name-derived by design).
- DB migrations: GORM AutoMigrate adds new columns; prod-row backfills use the RUN-ONCE guarded pattern (`catalog_migration_guards` ledger — copy `BumpKodikNoadsPriority`, `migrate.go:885`).

---

# Phase 1 — Backend existence core

### Task 1: `provider_unwired` gauge in libs/metrics

**Files:**
- Modify: `libs/metrics/roster.go`
- Test: `libs/metrics/roster_test.go`

**Interfaces:**
- Produces: `metrics.ProviderUnwired` — `*prometheus.GaugeVec`, labels `{provider, seam}`. Task 2 (scraper) sets it with `seam="scraper"`.

- [ ] **Step 1: Write the failing test** — append to `libs/metrics/roster_test.go`:

```go
func TestProviderUnwiredGauge(t *testing.T) {
	ProviderUnwired.WithLabelValues("newprov", "scraper").Set(1)
	if v := testutil.ToFloat64(ProviderUnwired.WithLabelValues("newprov", "scraper")); v != 1 {
		t.Fatalf("provider_unwired = %v, want 1", v)
	}
	ProviderUnwired.WithLabelValues("newprov", "scraper").Set(0)
	if v := testutil.ToFloat64(ProviderUnwired.WithLabelValues("newprov", "scraper")); v != 0 {
		t.Fatalf("provider_unwired = %v, want 0", v)
	}
}
```

Add `"github.com/prometheus/client_golang/prometheus/testutil"` to the test imports if absent (check how `roster_test.go` already asserts — reuse its idiom if it differs).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd libs/metrics && go test ./... -run TestProviderUnwiredGauge -v`
Expected: FAIL — `undefined: ProviderUnwired`

- [ ] **Step 3: Implement** — append to `libs/metrics/roster.go` (match the file's existing promauto import/var style; if the vars live in `provider.go`, put it next to `ProviderEnabled` instead):

```go
// ProviderUnwired flags a stream_providers roster row that a service cannot
// serve because its per-provider code registry has no implementation for the
// name (seam: "scraper" = no Go constructor in scraper-api). 1 = row exists,
// is not disabled, but nothing registered. 0/absent = wired or disabled.
// AUTO-608: rows-without-code must be observable, never silent.
var ProviderUnwired = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "provider_unwired",
		Help: "1 when a non-disabled stream_providers row has no code implementation at the labeled seam",
	},
	[]string{"provider", "seam"},
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd libs/metrics && go test ./... -v -run TestProviderUnwiredGauge`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add libs/metrics/roster.go libs/metrics/roster_test.go
git commit -m "feat(metrics): provider_unwired{provider,seam} gauge (AUTO-608)" -- libs/metrics/
git push origin HEAD:main
```

### Task 2: Scraper — fail-open remote loader + unwired reflection

**Files:**
- Modify: `services/scraper/internal/config/providers_remote.go` (delete the unknown-name hard-fail, ~line 103)
- Modify: `services/scraper/internal/config/providers.go` (add `AllNames()`; retarget the `KnownProviders` doc comment)
- Modify: `services/scraper/cmd/scraper-api/main.go` (metrics reflection off live config rows; unwired gauge after registration)
- Test: `services/scraper/internal/config/providers_remote_test.go`

**Interfaces:**
- Consumes: `metrics.ProviderUnwired` from Task 1.
- Produces: `ProvidersConfig.AllNames() []string` — sorted names of every loaded row (all are scraper_operated).

- [ ] **Step 1: Write the failing tests.** In `providers_remote_test.go` there is almost certainly an existing case asserting the `unknown provider` error — find it (`grep -n "unknown" services/scraper/internal/config/providers_remote_test.go`) and INVERT it: an unknown scraper_operated name must now load successfully and be present in the config. Add:

```go
func TestLoadProvidersRemote_UnknownProviderAccepted(t *testing.T) {
	// Serve one known + one brand-new provider; the loader must accept both
	// (AUTO-608 fail-open: one new DB row must never void the whole DB config).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"gogoanime","status":"enabled","scraper_operated":true},
			{"name":"newprov","status":"enabled","scraper_operated":true}
		]}}`))
	}))
	defer srv.Close()
	pc, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), time.Second)
	if err != nil {
		t.Fatalf("unknown provider must not fail the load: %v", err)
	}
	if pc.Status("newprov") != StatusEnabled {
		t.Fatalf("newprov status = %q, want enabled", pc.Status("newprov"))
	}
	names := pc.AllNames()
	want := []string{"gogoanime", "newprov"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("AllNames() = %v, want %v", names, want)
	}
}
```

Adapt the fixture shape to whatever the existing tests in that file use (they may have a helper for the envelope) — but the envelope MUST be the wrapped `{"success":true,"data":{...}}` form. Keep the existing empty-name and duplicate-name failure tests untouched (those stay errors).

- [ ] **Step 2: Run to verify failure**

Run: `cd services/scraper && go test ./internal/config/ -run TestLoadProvidersRemote -v`
Expected: FAIL — `unknown provider "newprov"` error, and `AllNames` undefined.

- [ ] **Step 3: Implement.**

(a) In `providers_remote.go`, delete the membership gate (keep empty-name and dup checks):

```go
		if p.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("provider config: entry with empty name")
		}
		// AUTO-608 fail-open: names outside the compiled constructor set are
		// ACCEPTED (a new DB row must never make the scraper discard the whole
		// DB config and fall back to the offline default). Rows without a Go
		// constructor simply never register; main.go reflects them as
		// provider_unwired{seam="scraper"}.
		if _, dup := metas[p.Name]; dup {
			return ProvidersConfig{}, fmt.Errorf("provider config: duplicate provider %q", p.Name)
		}
```

Also delete the now-unused `known := make(map[string]bool, ...)` block above the loop.

(b) In `providers.go`, add after `Meta`:

```go
// AllNames returns every loaded provider name, sorted. Under the remote
// loader all entries are scraper_operated rows; under the offline fallback
// it equals KnownProviders. AUTO-608: main.go keys roster-reflection metrics
// and the unwired check off this (NOT off compile-time KnownProviders), so a
// new DB row surfaces on the dashboard without a code change.
func (p ProvidersConfig) AllNames() []string {
	m := p.load()
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
```

(c) Update the `KnownProviders` doc comment: it no longer gates the remote loader; it seeds the offline fallback + adult/EN candidate split only.

(d) In `cmd/scraper-api/main.go`:
- The ISS-023 reflection loop (~line 717) becomes `for _, row := range cfg.Providers.Rows(cfg.Providers.AllNames()) {` (keeps codeless tombstones visible AND auto-includes new rows).
- Track constructed names: `registerByStatus` already appends to `candidateProviders`; find the adult registration site (18anime, ~line 590 + the `adultOrch` registration) and mirror the same append into a `constructedProviders []string` slice that BOTH paths append to (initialize `constructedProviders` next to `candidateProviders`; inside `registerByStatus` append to both, and append "18anime" at its adult registration call). Then after the reflection loop add:

```go
	// AUTO-608: reflect DB rows this binary has no constructor for. A row that
	// is disabled (tombstone: animefever, allanime) is expected to be codeless
	// and reads 0; anything else without code is a real wiring gap.
	constructed := make(map[string]bool, len(constructedProviders))
	for _, n := range constructedProviders {
		constructed[n] = true
	}
	for _, name := range cfg.Providers.AllNames() {
		unwired := 0.0
		if !constructed[name] && cfg.Providers.Status(name) != config.StatusDisabled {
			unwired = 1.0
			log.Warnw("provider row UNWIRED — DB roster row has no provider code in this binary",
				"name", name, "status", cfg.Providers.Status(name))
		}
		metrics.ProviderUnwired.WithLabelValues(name, "scraper").Set(unwired)
	}
```

(Verify the `metrics` import alias used at line ~721 for `ProviderEnabled` and reuse it.)

- [ ] **Step 4: Run tests + build**

Run: `cd services/scraper && go test ./... && go build ./...`
Expected: PASS (fix any existing test that pinned the unknown-name error).

- [ ] **Step 5: Commit**

```bash
git add services/scraper/
git commit -m "feat(scraper): fail-open remote provider config + provider_unwired reflection (AUTO-608)" -- services/scraper/
git push origin HEAD:main
```

### Task 3: Catalog — `display_name` / `player_key` / `anime_level` columns + seed + guarded backfill

**Files:**
- Modify: `services/catalog/internal/domain/scraper_provider.go`
- Modify: `services/catalog/internal/service/scraperprovider/seed.go` (every entry gains the three fields)
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go` (new `BackfillProviderIdentityV1`)
- Modify: `services/catalog/cmd/catalog-api/main.go` (call the backfill AFTER `SeedDefaults`)
- Test: `services/catalog/internal/service/scraperprovider/migrate_test.go` (or the file where sibling migrations are tested — `grep -rn "BumpKodikNoadsPriority" --include="*_test.go" services/catalog/`)

**Interfaces:**
- Produces: `domain.ScraperProvider.DisplayName / PlayerKey / AnimeLevel` — consumed by Tasks 4 (capabilities), 7 (hotcombos), 8 (validPlayers), 9 (feed).

**The canonical value table (used by BOTH seed and backfill — keep them identical):**

| name | display_name | player_key | anime_level |
|---|---|---|---|
| gogoanime | GogoAnime | english | true |
| animepahe | AnimePahe | english | true |
| allanime | AllAnime | english | true |
| allanime-okru | AllAnime (OK.ru) | english | true |
| animefever | AnimeFever | english | true |
| miruro | Miruro | english | true |
| nineanime | 9anime | english | true |
| animekai | AnimeKai | english | true |
| 18anime | 18anime | hanime | false |
| ae | AnimeEnigma | ae | true |
| kodik-noads | Kodik | kodik | true |
| kodik-iframe | Kodik (iframe) | kodik | true |
| animelib | AniLib | animelib | true |
| hanime | Hanime | hanime | false |
| animejoy-sibnet | Sibnet | animejoy-sibnet | true |
| animejoy-allvideo | AllVideo | animejoy-allvideo | true |

(`anime_level` = supports translation-less any-team new-episode lookup. EN chain shares `player_key='english'`; 18anime maps to the `hanime` player surface — mirrors FE `providerToLegacyPlayer` exactly. Display names for EN rows copied verbatim from `capability/rank.go:displayName`.)

- [ ] **Step 1: Domain fields.** Add to `ScraperProvider` (after `PreferenceWeight`):

```go
	// DisplayName is the operator-editable pretty label for player/dashboard
	// surfaces (capability DisplayName, Grafana). Empty ⇒ callers fall back to
	// a title-cased Name. Seeded; backfilled once by BackfillProviderIdentityV1.
	DisplayName string `gorm:"size:64" json:"display_name"`
	// PlayerKey maps this row into the legacy watch_history.player namespace
	// ('english', 'kodik', 'ae', 'hanime', …) consumed by watch preferences,
	// notifications hot-combos and episode validation. Multiple rows may share
	// one key (the whole EN chain is 'english'; both kodik rows are 'kodik').
	// Empty ⇒ the provider has no legacy-player identity.
	PlayerKey string `gorm:"size:32" json:"player_key"`
	// AnimeLevel marks providers whose new-episode lookup works without a
	// translation_id (any-team/anime-level: english, ae, kodik, animelib,
	// animejoy legs). Drives the notifications hot-combos eligibility subselect.
	AnimeLevel bool `json:"anime_level"`
```

- [ ] **Step 2: Seed.** Add the three fields to every entry in `seed.go` per the table (e.g. the `ae` entry gains `DisplayName: "AnimeEnigma", PlayerKey: "ae", AnimeLevel: true,`).

- [ ] **Step 3: Write the failing migration test.** Follow the existing guarded-migration test idiom in the package (find via `grep -rn "migrationGuard\|BumpKodikNoads" services/catalog/internal/service/scraperprovider/*_test.go`). Test body:

```go
func TestBackfillProviderIdentityV1(t *testing.T) {
	db := testDB(t) // reuse the package's sqlite test-DB helper
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	// A pre-existing prod row without the new fields.
	if err := db.Create(&domain.ScraperProvider{Name: "gogoanime", Status: domain.StatusEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := BackfillProviderIdentityV1(db); err != nil {
		t.Fatal(err)
	}
	var row domain.ScraperProvider
	db.Where("name = ?", "gogoanime").First(&row)
	if row.DisplayName != "GogoAnime" || row.PlayerKey != "english" || !row.AnimeLevel {
		t.Fatalf("backfill missed: %+v", row)
	}
	// Idempotent + operator-respecting: re-run must not clobber a manual edit.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "gogoanime").Update("display_name", "Custom")
	if err := BackfillProviderIdentityV1(db); err != nil {
		t.Fatal(err)
	}
	db.Where("name = ?", "gogoanime").First(&row)
	if row.DisplayName != "Custom" {
		t.Fatalf("guarded migration re-ran and clobbered operator edit: %q", row.DisplayName)
	}
}
```

- [ ] **Step 4: Run to verify it fails** — `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestBackfillProviderIdentityV1 -v` → FAIL (`undefined: BackfillProviderIdentityV1`).

- [ ] **Step 5: Implement the backfill** in `migrate.go` (copy the `BumpKodikNoadsPriority` shape — guard ledger, no clobber on re-run):

```go
// backfillProviderIdentityGuardKey marks BackfillProviderIdentityV1 as applied.
const backfillProviderIdentityGuardKey = "backfill_provider_identity_v1_2026_07_14"

// BackfillProviderIdentityV1 stamps display_name/player_key/anime_level onto
// pre-existing prod rows exactly once (AUTO-608). The seed is insert-if-absent
// and never updates prod rows; this run-once guarded migration is what carries
// the new identity columns to live DBs. Values mirror the seed table one-for-one.
func BackfillProviderIdentityV1(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", backfillProviderIdentityGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check backfill-provider-identity guard: %w", err)
	}
	if guards > 0 {
		return nil // applied — never clobber later operator re-tunes
	}
	type identity struct {
		display   string
		playerKey string
		animeLvl  bool
	}
	identities := map[string]identity{
		"gogoanime":         {"GogoAnime", "english", true},
		"animepahe":         {"AnimePahe", "english", true},
		"allanime":          {"AllAnime", "english", true},
		"allanime-okru":     {"AllAnime (OK.ru)", "english", true},
		"animefever":        {"AnimeFever", "english", true},
		"miruro":            {"Miruro", "english", true},
		"nineanime":         {"9anime", "english", true},
		"animekai":          {"AnimeKai", "english", true},
		"18anime":           {"18anime", "hanime", false},
		"ae":                {"AnimeEnigma", "ae", true},
		"kodik-noads":       {"Kodik", "kodik", true},
		"kodik-iframe":      {"Kodik (iframe)", "kodik", true},
		"animelib":          {"AniLib", "animelib", true},
		"hanime":            {"Hanime", "hanime", false},
		"animejoy-sibnet":   {"Sibnet", "animejoy-sibnet", true},
		"animejoy-allvideo": {"AllVideo", "animejoy-allvideo", true},
	}
	for name, id := range identities {
		if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", name).
			Updates(map[string]any{
				"display_name": id.display,
				"player_key":   id.playerKey,
				"anime_level":  id.animeLvl,
			}).Error; err != nil {
			return fmt.Errorf("backfill provider identity %q: %w", name, err)
		}
		// RowsAffected 0 is fine — absent rows are created complete by the seed.
	}
	if err := db.Create(&migrationGuard{Key: backfillProviderIdentityGuardKey}).Error; err != nil {
		return fmt.Errorf("write backfill-provider-identity guard: %w", err)
	}
	return nil
}
```

Wire it in `catalog-api/main.go` right after the `SeedDefaults` call (~line 190):

```go
	if err := scraperprovider.BackfillProviderIdentityV1(db.DB); err != nil {
		logg.Fatalw("failed to backfill provider identity columns", "error", err)
	}
```

(Match the surrounding error-handling style; verify the logger variable name.)

- [ ] **Step 6: Run tests** — `cd services/catalog && go test ./internal/service/scraperprovider/ -v -run "Backfill|Seed"` → PASS.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/
git commit -m "feat(catalog): display_name/player_key/anime_level roster columns + seed + guarded backfill (AUTO-608)" -- services/catalog/
git push origin HEAD:main
```

### Task 4: Catalog — roster-driven capability family assembly

**Files:**
- Modify: `services/catalog/internal/service/capability/service.go` (`buildFamilies`, `BuildENFamily` display name, `familyLabel` if needed)
- Modify: `services/catalog/internal/service/capability/families_ru.go` (builders take/read `row.DisplayName`)
- Modify: `services/catalog/internal/service/capability/families_firstparty.go` (`dbRowFamily` generic-row variant)
- Modify: `services/catalog/internal/service/capability/rank.go` (`displayName` becomes the fallback only)
- Test: `services/catalog/internal/service/capability/` existing test files (find with `ls services/catalog/internal/service/capability/*_test.go`)

**Interfaces:**
- Consumes: `row.DisplayName` (Task 3).
- Produces: `/capabilities` now includes a generic family for ANY registered non-EN row with no dedicated builder; kodik-iframe explicitly skipped.

- [ ] **Step 1: Write the failing test** (in the capability package's service test file, using its existing sqlite fixture helper — locate the helper that seeds `stream_providers` rows for `BuildENFamily` tests and reuse it):

```go
func TestBuildFamilies_UnknownRosterRowGetsGenericFamily(t *testing.T) {
	svc, db := newTestService(t) // reuse/adapt the package's existing constructor helper
	// A brand-new non-EN provider row with NO dedicated family builder.
	if err := db.Create(&domain.ScraperProvider{
		Name: "newru", Status: domain.StatusEnabled, Group: "ru",
		DisplayName: "NewRU", SupportsDub: true, PreferenceWeight: 10,
	}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(context.Background(), "some-anime-id")
	if err != nil {
		t.Fatal(err)
	}
	var found *domain.ProviderCap
	for _, fam := range report.Families {
		for i := range fam.Providers {
			if fam.Providers[i].Provider == "newru" {
				found = &fam.Providers[i]
			}
		}
	}
	if found == nil {
		t.Fatal("new roster row must surface in /capabilities via the generic dbRowFamily default")
	}
	if found.DisplayName != "NewRU" {
		t.Fatalf("display_name not read from row: %q", found.DisplayName)
	}
}

func TestBuildFamilies_KodikIframeSkipped(t *testing.T) {
	svc, db := newTestService(t)
	if err := db.Create(&domain.ScraperProvider{
		Name: "kodik-iframe", Status: domain.StatusEnabled, Group: "ru",
	}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(context.Background(), "some-anime-id")
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range report.Families {
		for _, cap := range fam.Providers {
			if cap.Provider == "kodik-iframe" {
				t.Fatal("kodik-iframe is the Classic-Kodik iframe surface, not an aePlayer capability — must be skipped")
			}
		}
	}
}
```

Note: cache must be nil or flushed between Report calls in these tests (Report caches per anime — use distinct anime ids or a nil cache, matching the existing tests' approach).

- [ ] **Step 2: Run to verify failure** — `cd services/catalog && go test ./internal/service/capability/ -run TestBuildFamilies_Unknown -v` → FAIL (row invisible).

- [ ] **Step 3: Implement.** Rewrite `buildFamilies` around a roster query + name-keyed dispatch. Complete replacement for the function body:

```go
// familyBuilder builds one roster row's family. ok=false omits it (best-effort).
type familyBuilder func(ctx context.Context, animeID string, row domain.ScraperProvider) (domain.SourceFamily, bool)

// buildFamilies assembles the EN family (DB-driven, required) plus one family
// per registered non-EN stream_providers row (best-effort). AUTO-608: the
// per-row dispatch is a name-keyed registry with a GENERIC default
// (rowFamily), so a brand-new DB row surfaces in /capabilities without a code
// change. kodik-iframe maps to nil = intentionally no capability (it is the
// Classic-Kodik iframe surface, not an aePlayer source). Builders that need
// the per-title catalog parsers guard on s.catalog == nil themselves.
func (s *Service) buildFamilies(ctx context.Context, animeID string) ([]domain.SourceFamily, error) {
	var raw map[string]providerScore
	if s.playability != nil {
		raw = s.playability.Scores(ctx, animeID)
	}
	ctx = withBlend(ctx, newBlendData(raw))

	// Non-EN registered rows, best-first (weight desc mirrors the FE sort).
	var rows []domain.ScraperProvider
	if err := s.db.WithContext(ctx).
		Where(`status <> ? AND "group" <> ?`, domain.StatusDisabled, "en").
		Order("preference_weight desc, name asc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load non-EN providers: %w", err)
	}

	// AnimeJoy legs share ONE discovery call (title→news_id→playlist); resolve
	// it lazily at most once per report, from whichever leg runs first.
	var (
		ajOnce  sync.Once
		ajTeams []domain.AnimejoyTeam
		ajErr   error
	)
	animejoyLeg := func(display, leg string) familyBuilder {
		return func(ctx context.Context, animeID string, row domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			ajOnce.Do(func() { ajTeams, ajErr = s.catalog.GetAnimejoyTeams(ctx, animeID) })
			if ajErr != nil {
				return domain.SourceFamily{}, false // discovery error → leg absent, not no_content
			}
			return s.animejoyLegFamily(ctx, ajTeams, row.Name, displayOf(row, display), leg)
		}
	}

	builders := map[string]familyBuilder{
		"ae": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			return s.aeFamily(ctx, animeID)
		},
		"kodik-noads": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			return s.kodikFamily(ctx, animeID)
		},
		"kodik-iframe": nil, // Classic-Kodik iframe surface — no aePlayer capability
		"animelib": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			return s.animelibFamily(ctx, animeID)
		},
		"hanime": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			return s.hanimeFamily(ctx, animeID)
		},
		"animejoy-sibnet":   animejoyLeg("Sibnet", "sibnet"),
		"animejoy-allvideo": animejoyLeg("AllVideo", "allvideo"),
		// default (absent key): generic rowFamily — see loop below.
	}

	type slot struct {
		fam domain.SourceFamily
		ok  bool
	}
	slots := make([]slot, len(rows))
	var (
		en    domain.SourceFamily
		enErr error
		wg    sync.WaitGroup
	)
	wg.Add(1)
	go func() { defer wg.Done(); en, enErr = s.BuildENFamily(ctx) }()
	for i, row := range rows {
		b, has := builders[row.Name]
		if has && b == nil {
			continue // explicit skip (kodik-iframe)
		}
		if !has {
			b = func(ctx context.Context, _ string, row domain.ScraperProvider) (domain.SourceFamily, bool) {
				return s.rowFamily(ctx, row) // generic default — ANY new row is wired
			}
		}
		wg.Add(1)
		go func(i int, row domain.ScraperProvider, b familyBuilder) {
			defer wg.Done()
			slots[i].fam, slots[i].ok = b(ctx, animeID, row)
		}(i, row, b)
	}
	wg.Wait()
	if enErr != nil {
		return nil, enErr
	}

	// Assembly: ae leads (first-party first), then EN, then the rest in the
	// roster's weight order (slots is already weight-sorted via the query).
	families := make([]domain.SourceFamily, 0, len(rows)+1)
	var rest []domain.SourceFamily
	for i, sl := range slots {
		if !sl.ok {
			continue
		}
		if rows[i].Name == "ae" {
			families = append(families, sl.fam)
			continue
		}
		rest = append(rest, sl.fam)
	}
	families = append(families, en)
	families = append(families, rest...)
	return regroupFamilies(families), nil
}

// displayOf prefers the row's operator-editable DisplayName, falling back to
// the compiled default label.
func displayOf(row domain.ScraperProvider, fallback string) string {
	if row.DisplayName != "" {
		return row.DisplayName
	}
	return fallback
}
```

And in `families_firstparty.go`, generalize `dbRowFamily` into a row-based variant (keep the old signature as a thin wrapper if other callers exist — `grep -rn "dbRowFamily" services/catalog/`):

```go
// rowFamily builds a single-provider family straight from a stream_providers
// row — the generic default for ANY roster row without a dedicated builder
// (AUTO-608), and the trait-only sources (18anime). hasContent=true (trait-only
// rows always render selectable; per-title truth needs a dedicated builder).
// The wire family name is the row name; group adult collapses to "18+" via
// familyLabel/regroupFamilies as usual.
func (s *Service) rowFamily(ctx context.Context, row domain.ScraperProvider) (domain.SourceFamily, bool) {
	pc := domain.ProviderCap{
		Provider:    row.Name,
		DisplayName: displayOf(row, displayName(row.Name)),
		Variants:    variantsFromTraits(row),
	}
	if !applyFeedFields(ctx, &pc, row, true) {
		return domain.SourceFamily{}, false
	}
	family := row.Name
	if row.Group == "adult" {
		family = "adult" // collapses to the "18+" wire label (see familyLabel)
	}
	return domain.SourceFamily{Family: family, Providers: []domain.ProviderCap{pc}}, true
}
```

Then: delete `dbRowFamily` and repoint its 18anime call — the 18anime row now flows through the generic default in the registry loop (it is non-EN group `adult`, so the roster query picks it up; verify the old `dbRowFamily(ctx, "18anime", "18anime", "adult")` produced `Family: "adult"` vs `"18anime"` — read `familyLabel` and keep the WIRE output identical; adjust the `family` string above to whatever preserves today's `"18+"` wire label). Also check `familyLabel`'s default branch maps arbitrary internal names to `"others"` — the generic family for a new RU row must land in `"others"`.

In `kodikFamily` / `animelibFamily` / `hanimeFamily` / `noContentFamily`: replace the display literals with `displayOf(row, "<old literal>")` where the row is already loaded (`providerRow`). In `BuildENFamily`: `DisplayName: displayOf(row, displayName(row.Name))`.

- [ ] **Step 4: Run the full capability test suite** — `cd services/catalog && go test ./internal/service/capability/ -v` → PASS. Fix tests that pinned the old fixed family ORDER (the wire re-sort is FE-side; internal test expectations may need the weight order).

- [ ] **Step 5: Run whole-service tests + build** — `cd services/catalog && go test ./... && go build ./...` → PASS.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/
git commit -m "feat(catalog): roster-driven capability family assembly with generic default (AUTO-608)" -- services/catalog/
git push origin HEAD:main
```

### Task 5: Analytics — roster client; roster-fed whitelist, playability filter, probe targets

**Files:**
- Create: `services/analytics/internal/roster/roster.go`
- Test: `services/analytics/internal/roster/roster_test.go`
- Modify: `services/analytics/internal/handler/playertelemetry_whitelist.go`
- Modify: `services/analytics/internal/handler/playertelemetry.go` (handler carries the roster lookup)
- Modify: `services/analytics/internal/handler/playability.go` (`inRoster` wiring at :48)
- Modify: `services/analytics/cmd/analytics-api/main.go` (construct client; roster-driven probe targets)
- Modify: `services/analytics/internal/config/config.go` (PROBE_PROVIDERS default → `""`, doc as optional filter)

**Interfaces:**
- Produces: `roster.Client` with `New(catalogURL string, ttl time.Duration) *Client`, `Known(name string) bool`, `Rows(ctx context.Context) []Row` where `Row{Name, Group, Status string; ScraperOperated bool}`.
- Consumes: catalog `GET /internal/scraper/providers` (envelope `{"success":true,"data":{"providers":[...]}}`, returns ALL rows incl. disabled tombstones).

- [ ] **Step 1: Write the failing roster-client tests:**

```go
package roster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const payload = `{"success":true,"data":{"providers":[
	{"name":"gogoanime","group":"en","status":"enabled","scraper_operated":true},
	{"name":"kodik-noads","group":"ru","status":"enabled","scraper_operated":false},
	{"name":"animefever","group":"en","status":"disabled","scraper_operated":true}
]}}`

func TestClient_FetchKnownAndTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Minute)
	if !c.Known("gogoanime") || !c.Known("KODIK-NOADS") { // case-insensitive
		t.Fatal("roster rows must be Known")
	}
	if !c.Known("animefever") {
		t.Fatal("disabled tombstone rows stay Known (legacy events keep recording)")
	}
	if c.Known("nosuch") {
		t.Fatal("unknown name must not be Known")
	}
	c.Known("gogoanime")
	if hits != 1 {
		t.Fatalf("TTL cache must serve repeat lookups from memory, got %d fetches", hits)
	}
	if got := len(c.Rows(context.Background())); got != 3 {
		t.Fatalf("Rows() = %d rows, want 3", got)
	}
}

func TestClient_FallbackSnapshotWhenCatalogDown(t *testing.T) {
	c := New("http://127.0.0.1:1", time.Minute) // unreachable
	// Cold-start fallback: the embedded snapshot must cover the live roster.
	for _, name := range []string{"gogoanime", "kodik-noads", "ae", "hanime", "animejoy-sibnet"} {
		if !c.Known(name) {
			t.Fatalf("embedded fallback snapshot missing %q", name)
		}
	}
}

func TestClient_LastGoodSurvivesOutage(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Millisecond) // force refetch
	if !c.Known("gogoanime") {
		t.Fatal("initial fetch")
	}
	up = false
	time.Sleep(5 * time.Millisecond)
	if !c.Known("gogoanime") {
		t.Fatal("last-good roster must survive a catalog outage")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `cd services/analytics && go test ./internal/roster/ -v` → FAIL (package missing).

- [ ] **Step 3: Implement `roster.go`:**

```go
// Package roster is the analytics-side client for the catalog stream_providers
// roster (the DB single source of truth for provider EXISTENCE — AUTO-608).
// It replaces the compile-time knownProviders map: player-telemetry whitelisting,
// the playability roster filter, and probe-target membership all key off this.
package roster

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Row is the minimal projection of a stream_providers row analytics needs.
type Row struct {
	Name            string `json:"name"`
	Group           string `json:"group"`
	Status          string `json:"status"`
	ScraperOperated bool   `json:"scraper_operated"`
}

// fallbackSnapshot is the embedded cold-start roster, used ONLY until the
// first successful catalog fetch (e.g. analytics boots before catalog).
// Mirrors the seed roster; new providers do NOT need to be added here — they
// arrive via the live fetch. Keep tombstones so legacy events keep recording.
var fallbackSnapshot = []Row{
	{Name: "gogoanime", Group: "en", ScraperOperated: true},
	{Name: "animepahe", Group: "en", ScraperOperated: true},
	{Name: "allanime", Group: "en", ScraperOperated: true},
	{Name: "allanime-okru", Group: "en", ScraperOperated: true},
	{Name: "animefever", Group: "en", ScraperOperated: true},
	{Name: "miruro", Group: "en", ScraperOperated: true},
	{Name: "nineanime", Group: "en", ScraperOperated: true},
	{Name: "animekai", Group: "en", ScraperOperated: true},
	{Name: "18anime", Group: "adult", ScraperOperated: true},
	{Name: "ae", Group: "firstparty"},
	{Name: "kodik-noads", Group: "ru"},
	{Name: "kodik-iframe", Group: "ru"},
	{Name: "animelib", Group: "ru"},
	{Name: "hanime", Group: "adult"},
	{Name: "animejoy-sibnet", Group: "ru"},
	{Name: "animejoy-allvideo", Group: "ru"},
}

// Client fetches + TTL-caches the roster with last-good fallback.
type Client struct {
	url  string
	ttl  time.Duration
	http *http.Client

	mu        sync.Mutex
	rows      []Row
	names     map[string]struct{}
	fetchedAt time.Time
	everGood  bool
}

// New builds a client over CATALOG_URL. ttl 60s matches the scraper's
// remote-config refresh cadence.
func New(catalogURL string, ttl time.Duration) *Client {
	c := &Client{
		url:  strings.TrimRight(catalogURL, "/") + "/internal/scraper/providers",
		ttl:  ttl,
		http: &http.Client{Timeout: 10 * time.Second},
	}
	c.install(fallbackSnapshot, false)
	return c
}

func (c *Client) install(rows []Row, good bool) {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[strings.ToLower(r.Name)] = struct{}{}
	}
	c.rows, c.names = rows, names
	if good {
		c.everGood = true
		c.fetchedAt = time.Now()
	}
}

// refresh fetches when stale; on error the last-good (or fallback) set stays.
func (c *Client) refresh(ctx context.Context) {
	if c.everGood && time.Since(c.fetchedAt) < c.ttl {
		return
	}
	if !c.everGood && time.Since(c.fetchedAt) < time.Second {
		return // don't hammer an unreachable catalog on the cold path
	}
	c.fetchedAt = time.Now() // stamp attempt time even on failure (retry backoff)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	// Envelope: {"success":true,"data":{"providers":[...]}} — decode data.providers
	// (ISS-032: decoding the root silently yields an EMPTY roster).
	var body struct {
		Data struct {
			Providers []Row `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || len(body.Data.Providers) == 0 {
		return // empty/undecodable ⇒ keep last-good
	}
	c.install(body.Data.Providers, true)
}

// Known reports (case-insensitively) whether name is a roster row.
func (c *Client) Known(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(context.Background())
	_, ok := c.names[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Rows returns the last-good roster rows.
func (c *Client) Rows(ctx context.Context) []Row {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh(ctx)
	out := make([]Row, len(c.rows))
	copy(out, c.rows)
	return out
}
```

- [ ] **Step 4: Run roster tests** — `cd services/analytics && go test ./internal/roster/ -v` → PASS. (The refresh-in-Known makes lookups do a blocking fetch at most once per TTL; if an existing handler benchmark objects, move refresh to a background goroutine — keep it simple first.)

- [ ] **Step 5: Rewire the whitelist.** Replace `playertelemetry_whitelist.go` content:

```go
package handler

import "strings"

// providerRoster is the injected roster membership check (roster.Client.Known).
// Set at construction; nil-safe for tests that don't care (falls back to the
// synthetic set only).
type providerRoster interface{ Known(name string) bool }

// syntheticProviders are player-surface ids that are NOT stream_providers rows
// but legitimately appear as combo.provider on player events:
//   - "kodik": the capability/FE id for the kodik-noads row (the alias lives in
//     catalog capability assembly; analytics accepts both spellings).
//   - "offline": the PWA offline-downloads synthetic provider.
var syntheticProviders = map[string]struct{}{"kodik": {}, "offline": {}}

// whitelistProvider returns the canonical (lowercased) provider key when it is
// a roster row or a known synthetic, else "". Player-telemetry provider becomes
// the source-ranking GROUP BY target, so an unwhitelisted value injects
// arbitrary rows into the ranking aggregates (audit medium #2). AUTO-608: the
// roster is fetched live from catalog (DB = source of truth), so a new
// provider's telemetry records without a code change here.
func whitelistProvider(s string, roster providerRoster) string {
	k := strings.ToLower(strings.TrimSpace(s))
	if _, ok := syntheticProviders[k]; ok {
		return k
	}
	if roster != nil && roster.Known(k) {
		return k
	}
	return ""
}
```

Thread the roster through: `NewPlayerTelemetryHandler(sink Sink)` → `NewPlayerTelemetryHandler(sink Sink, roster providerRoster)`; store it on the struct; the call at `playertelemetry.go:102` becomes `whitelistProvider(we.Provider, h.roster)`. Update `handler/playability.go`'s `inRoster` wiring (line ~48) to `func(p string) bool { return whitelistProvider(p, h.roster) != "" }` — check the actual current shape first and preserve it, only swapping the membership source. Update every test that constructs these handlers: pass a stub `rosterStub{names: ...}` implementing `Known` with the OLD static set so existing expectations hold; add one new case asserting a roster-known new provider passes and an unknown one is dropped.

- [ ] **Step 6: Roster-driven probe targets.** In `cmd/analytics-api/main.go` construct the client near the other clients: `rosterClient := roster.New(cfg.CatalogURL, time.Minute)` and pass it to both handler constructors. Replace the `strings.Split(cfg.ProbeProviders, ...)` target loop with:

```go
			// AUTO-608: probe-target membership comes from the DB roster (the
			// catalog probe-plan already gates WHEN each row is probed; this
			// gates WHAT this binary can probe). PROBE_PROVIDERS is now an
			// OPTIONAL comma-separated filter (unset ⇒ every wirable row).
			var filter map[string]bool
			if cfg.ProbeProviders != "" {
				filter = map[string]bool{}
				for _, n := range strings.Split(cfg.ProbeProviders, ",") {
					if n = strings.TrimSpace(n); n != "" {
						filter[n] = true
					}
				}
			}
			var targets []probe.ProbeTarget
			for _, row := range rosterClient.Rows(context.Background()) {
				if filter != nil && !filter[row.Name] {
					continue
				}
				if b, ok := build[row.Name]; ok {
					targets = append(targets, b())
					continue
				}
				if row.ScraperOperated && row.Group == "en" {
					// Default: EN scraper provider — shared spotlight set + scraper resolver.
					targets = append(targets, probe.ProbeTarget{Provider: row.Name, AnimeSet: spotlight, Resolver: scraperRes})
					continue
				}
				// Intentionally unprobeable (kodik-iframe: no direct stream; hanime/
				// animelib/18anime: no probe resolver built). Logged, not gauged —
				// see plan deviation #2.
				log.Infow("probe target skipped — no resolver for roster row", "provider", row.Name, "group", row.Group)
			}
```

Preserve the exact current target set: with the DB roster this yields gogoanime, animepahe(custom), allanime-okru, miruro, nineanime, animekai, allanime, animefever (EN default — allanime/animefever are tombstones whose plan entries are policy=disabled-excluded so they're never probed; harmless) + ae, kodik-noads, animejoy-sibnet, animejoy-allvideo (custom). In `config.go:117` change the default to `getEnv("PROBE_PROVIDERS", "")` and update its comment to "optional filter; empty = all wirable roster rows".

- [ ] **Step 7: Tests + build** — `cd services/analytics && go test ./... && go build ./...` → PASS.

- [ ] **Step 8: Commit**

```bash
git add services/analytics/
git commit -m "feat(analytics): DB-roster-driven telemetry whitelist, playability filter, probe targets (AUTO-608)" -- services/analytics/
git push origin HEAD:main
```

### Task 6: Grafana — unwired panel + alert; P1 deploy + live verify

**Files:**
- Modify: `docker/grafana/provisioning/alerting/rules.yml` (new rule, copy an existing Prometheus-datasource rule's structure, e.g. the ProviderFleet rules)
- Modify: `infra/grafana/alerts/scraper.yaml` IF the fleet rules are mirrored there (`grep -n "ProviderFleet" infra/grafana/alerts/*.yaml` — follow the same keep-in-sync convention)
- Modify: `docker/grafana/dashboards/playback-health.json` (one new table panel)

- [ ] **Step 1: Add the alert rule** to `rules.yml`, mirroring the exact YAML shape of `ProviderFleetNoAutoPlayable` (same datasource UID, folder, evaluation group):
  - title: `ProviderRowUnwired`
  - expr: `max by (provider, seam) (provider_unwired == 1)`
  - `for: 30m` (a row created before its code deploys mid-rollout must not page instantly)
  - labels: `severity: warning` (routes to maintenance-webhook via the default route — verify `policies.yml` routes warning there; the AePlayerPlaybackFailures rule from 2026-07-11 is the precedent)
  - annotations summary: `Provider roster row {{ $labels.provider }} has no code at seam {{ $labels.seam }} — a DB row was added/enabled without a matching implementation, or a deploy is missing.`

- [ ] **Step 2: Add the dashboard panel** to `playback-health.json`: a Prometheus-datasource table titled "Unwired roster rows (DB row, no code)", query `max by (provider, seam) (provider_unwired == 1)`, format table, placed next to the roster table; description: "Empty is healthy. AUTO-608: rows here exist in stream_providers but no service registered code for them." Copy panel JSON scaffolding from an existing Prometheus table panel in the same dashboard (match datasource UID by reference, not hardcoded).

- [ ] **Step 3: Deploy P1 + live verify**

```bash
make redeploy-catalog && make redeploy-scraper && make redeploy-analytics
make restart-grafana
make health
```

Expected: all services healthy. Then verify each seam live:

```bash
# 1. Columns backfilled:
docker exec -i animeenigma-postgres psql -U postgres -d animeenigma \
  -c "SELECT name, display_name, player_key, anime_level FROM stream_providers ORDER BY name;"
# Expected: every row has display_name + player_key per the Task 3 table.

# 2. Capabilities still serve (pick a real anime UUID from the DB):
AID=$(docker exec -i animeenigma-postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM animes ORDER BY sort_priority DESC, score DESC LIMIT 1")
curl -s "http://localhost:8081/api/anime/$AID/capabilities" | head -c 2000
# Expected: families incl. ourenglish + kodik + ae; NO kodik-iframe provider.

# 3. Scraper loaded remote config fail-open + gauges present:
make logs-scraper 2>&1 | head -50 | grep -i "provider management config loaded"
curl -s http://localhost:8087/metrics 2>/dev/null | grep provider_unwired | head
# (Scraper metrics port: check compose; use the service's /metrics.)
# Expected: provider_unwired{...seam="scraper"} series, all 0.

# 4. Analytics whitelist still ingests (watch a player event land) + probe targets logged:
make logs-analytics 2>&1 | grep -iE "probe target|roster" | head
```

- [ ] **Step 4: END-TO-END existence test (the whole point).** Insert a synthetic row, watch it appear everywhere, then remove it:

```bash
docker exec -i animeenigma-postgres psql -U postgres -d animeenigma -c \
  "INSERT INTO stream_providers (name, status, policy, health, \"group\", display_name, player_key, sub_delivery, engine) \
   VALUES ('auto608-test', 'enabled', 'auto', 'up', 'ru', 'AUTO608 Test', '', 'hard', 'http');"
# Capabilities (cache is 10min — use a DIFFERENT anime id than step 2, or flush redis key capabilities:<id>):
AID2=$(docker exec -i animeenigma-postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM animes ORDER BY sort_priority DESC, score DESC OFFSET 1 LIMIT 1")
curl -s "http://localhost:8081/api/anime/$AID2/capabilities" | grep -o "auto608-test"
# Expected: auto608-test present (generic family).
# Scraper: refresh ≤60s; it is NOT scraper_operated so nothing changes there (correct).
# Cleanup:
docker exec -i animeenigma-postgres psql -U postgres -d animeenigma -c "DELETE FROM stream_providers WHERE name='auto608-test';"
```

- [ ] **Step 5: Commit**

```bash
git add docker/grafana/ infra/grafana/
git commit -m "feat(grafana): ProviderRowUnwired alert + unwired-rows panel (AUTO-608)" -- docker/grafana/ infra/grafana/
git push origin HEAD:main
```

---

# Phase 2 — player-key namespace bridge

### Task 7: Notifications — hotcombos roster subselect

**Files:**
- Modify: `services/notifications/internal/job/hotcombos.go:63` (+ the mirrored comment at :26)
- Test: the package's existing test file (`ls services/notifications/internal/job/*_test.go`)

- [ ] **Step 1: Write the failing test.** Reuse the package's DB test helper (sqlite or testcontainers — inspect existing hotcombos tests and mirror their fixture style). Seed `stream_providers` rows (`english`-keyed EN row with `anime_level=true`, a `hanime` row with `anime_level=false`) plus `watch_history`/`anime_list`/`animes` fixtures, then assert:

```go
func TestHotCombos_RosterDrivenAnimeLevelPlayers(t *testing.T) {
	db := newJobTestDB(t) // package helper; must AutoMigrate domain.ScraperProvider too
	seedProvider(t, db, "gogoanime", "english", true /*anime_level*/, "enabled")
	seedProvider(t, db, "hanime", "hanime", false, "enabled")
	seedProvider(t, db, "animelib", "animelib", true, "disabled") // disabled → excluded
	// combo with empty translation_id and player 'english' → collected
	seedWatchRow(t, db, "english", "")
	// empty translation_id + player 'hanime' (not anime-level) → NOT collected
	seedWatchRow(t, db, "hanime", "")
	// empty translation_id + player 'animelib' (row disabled) → NOT collected (deviation #6)
	seedWatchRow(t, db, "animelib", "")
	// non-empty translation_id always collected regardless of player
	seedWatchRow(t, db, "hanime", "tr-1")

	combos, err := NewHotCombosCollector(db, testLogger()).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range combos {
		got[c.Player+"|"+c.TranslationID] = true
	}
	if !got["english|"] || !got["hanime|tr-1"] {
		t.Fatalf("expected english| and hanime|tr-1 collected, got %v", got)
	}
	if got["hanime|"] || got["animelib|"] {
		t.Fatalf("non-anime-level / disabled players with empty translation must be excluded, got %v", got)
	}
}
```

Write `seedProvider`/`seedWatchRow` helpers matching the real column names (watch_history needs user_id/anime_id joined to anime_list status='watching' + animes status='ongoing' — copy the existing test's fixture rows).

- [ ] **Step 2: Run to verify failure** — `cd services/notifications && go test ./internal/job/ -run TestHotCombos_Roster -v` → FAIL (hanime| and animelib| leak through via… actually the OLD literal excludes hanime already; animelib| will wrongly be collected → that's the failing assertion).

- [ ] **Step 3: Implement.** In `hotcombos.go` replace the literal IN-list (both the query and the :26 doc comment):

```go
		  AND (wh.translation_id != '' OR wh.player IN (
		    SELECT DISTINCT player_key FROM stream_providers
		    WHERE anime_level AND status <> 'disabled' AND player_key <> ''))
```

Comment above it: `-- AUTO-608: anime-level players derive from the roster (player_key/anime_level columns), not a literal list. A disabled row's player_key leaves the set.`

- [ ] **Step 4: Run** — `cd services/notifications && go test ./... && go build ./...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/
git commit -m "feat(notifications): hot-combos anime-level players from roster subselect (AUTO-608)" -- services/notifications/
git push origin HEAD:main
```

### Task 8: Catalog — roster-derived `validPlayers`

**Files:**
- Modify: `services/catalog/internal/service/episodes_validate.go` (:58-72)
- Modify: the constructor + callers (`grep -rn "IsValidPlayer\|NewEpisodesValidateService" services/catalog/ --include="*.go" | grep -v _test`)
- Test: `services/catalog/internal/service/episodes_validate_test.go` (existing — extend)

- [ ] **Step 1: Write the failing test** (in the existing test file, following its fake-injection style):

```go
func TestValidPlayer_RosterDriven(t *testing.T) {
	svc := newValidateServiceForTest(t /* existing helper */)
	svc.playerKeys = func(ctx context.Context) map[string]struct{} {
		return map[string]struct{}{"english": {}, "kodik": {}, "newplayer": {}}
	}
	for _, p := range []string{"english", "kodik", "newplayer", "ourenglish", "aeplayer"} {
		if !svc.ValidPlayer(context.Background(), p) {
			t.Fatalf("player %q must be valid (roster or protocol alias)", p)
		}
	}
	if svc.ValidPlayer(context.Background(), "bogus") {
		t.Fatal("unknown player must stay invalid")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `cd services/catalog && go test ./internal/service/ -run TestValidPlayer_RosterDriven -v` → FAIL.

- [ ] **Step 3: Implement.** In `episodes_validate.go`:

```go
// protocolPlayers are Watch-Together protocol surface names that are NOT
// watch_history player_keys ('ourenglish' = the EN scraper surface name in the
// WT protocol, 'aeplayer' = the unified player surface). Static by design —
// they are wire-protocol constants, not roster rows.
var protocolPlayers = map[string]struct{}{"ourenglish": {}, "aeplayer": {}}

// playerKeysFn returns the roster-derived player_key set (DISTINCT player_key
// FROM stream_providers WHERE player_key <> ''). Injected; a 60s in-process
// TTL cache lives at the wiring site. AUTO-608: a new roster row's player_key
// becomes a valid player with no code change here.
type playerKeysFn func(ctx context.Context) map[string]struct{}
```

Replace the `validPlayers` var + `IsValidPlayer` func with a method on the service (add `playerKeys playerKeysFn` field to `EpisodesValidateService`, defaulting in the constructor to a DB-backed impl passed in from `main.go`):

```go
// ValidPlayer reports whether p is a roster player_key or a protocol surface
// name. Replaces the closed v1.0 validPlayers set (AUTO-608).
func (s *EpisodesValidateService) ValidPlayer(ctx context.Context, p string) bool {
	if _, ok := protocolPlayers[p]; ok {
		return true
	}
	if s.playerKeys == nil {
		return false
	}
	_, ok := s.playerKeys(ctx)[p]
	return ok
}
```

Wiring (catalog main or wherever `NewEpisodesValidateService` is constructed): a small cached closure —

```go
	playerKeys := cachedPlayerKeys(db.DB, time.Minute)
```

with, next to the constructor:

```go
// cachedPlayerKeys returns a playerKeysFn backed by a 60s-TTL DISTINCT query.
func cachedPlayerKeys(db *gorm.DB, ttl time.Duration) playerKeysFn {
	var (
		mu   sync.Mutex
		set  map[string]struct{}
		when time.Time
	)
	return func(ctx context.Context) map[string]struct{} {
		mu.Lock()
		defer mu.Unlock()
		if set != nil && time.Since(when) < ttl {
			return set
		}
		var keys []string
		if err := db.WithContext(ctx).Model(&domain.ScraperProvider{}).
			Where("player_key <> ''").Distinct().Pluck("player_key", &keys).Error; err != nil {
			return set // stale-but-usable on DB error; nil on cold error → invalid
		}
		next := make(map[string]struct{}, len(keys))
		for _, k := range keys {
			next[k] = struct{}{}
		}
		set, when = next, time.Now()
		return set
	}
}
```

Update the two `IsValidPlayer` call sites (`episodes_validate.go:149` internal use + `handler/internal_episodes_validate.go:96`) to the method with the request ctx. Note the validation set (unlike hotcombos) does NOT filter `status <> 'disabled'`: validating an episode for a currently-disabled provider's player is harmless and keeps historical WT rooms working.

- [ ] **Step 4: Run** — `cd services/catalog && go test ./... && go build ./...` → PASS (update tests that used `IsValidPlayer`).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/
git commit -m "feat(catalog): roster-derived valid players for episode validation (AUTO-608)" -- services/catalog/
git push origin HEAD:main
```

### Task 9: Capability feed exposes `player_key`; P2 deploy

**Files:**
- Modify: `services/catalog/internal/domain/capability.go` (ProviderCap)
- Modify: `services/catalog/internal/service/capability/families_ru.go` (`applyFeedFields`)
- Modify: `frontend/web/src/types/capabilities.ts` (FE mirror)
- Test: extend a capability service test (Task 4's file)

- [ ] **Step 1: Failing test** (capability package):

```go
func TestFeed_ExposesPlayerKey(t *testing.T) {
	svc, db := newTestService(t)
	db.Create(&domain.ScraperProvider{Name: "hanime", Status: domain.StatusEnabled,
		Group: "adult", PlayerKey: "hanime", DisplayName: "Hanime"})
	report, err := svc.Report(context.Background(), "anime-x")
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range report.Families {
		for _, cap := range fam.Providers {
			if cap.Provider == "hanime" && cap.PlayerKey != "hanime" {
				t.Fatalf("player_key not on the wire: %+v", cap)
			}
		}
	}
}
```

- [ ] **Step 2: Verify failure** → `undefined: cap.PlayerKey`.

- [ ] **Step 3: Implement.** `domain/capability.go` ProviderCap, after `Reason`:

```go
	// PlayerKey is the legacy watch_history.player namespace key for this
	// provider ('english', 'kodik', 'ae', …) from the roster row. The FE uses
	// it to persist watch combos without a hardcoded provider→player switch
	// (AUTO-608). Empty when the row has none.
	PlayerKey string `json:"player_key,omitempty"`
```

`applyFeedFields` (families_ru.go): add `cap.PlayerKey = row.PlayerKey` next to `cap.Group = ...`.

FE `types/capabilities.ts` ProviderCap, after `reason?`:

```ts
  /** Legacy watch_history.player namespace key for persistence
   *  ('english' | 'kodik' | 'ae' | …) — from the roster row (AUTO-608).
   *  Absent when the provider has no legacy-player identity. */
  player_key?: string
```

- [ ] **Step 4: Run** — `cd services/catalog && go test ./internal/service/capability/ -v` → PASS; `cd frontend/web && bunx tsc --noEmit` (expect clean — but note vue-tsc false-pass memory; a plain field addition is safe).

- [ ] **Step 5: Deploy P2 + verify**

```bash
make redeploy-catalog && make redeploy-notifications && make health
curl -s "http://localhost:8081/api/anime/$AID/capabilities" | grep -o '"player_key":"[a-z-]*"' | sort | uniq -c
# Expected: english/kodik/ae/hanime/animejoy-* keys on the wire.
```

- [ ] **Step 6: Commit**

```bash
git add services/catalog/ frontend/web/src/types/capabilities.ts
git commit -m "feat(capabilities): expose roster player_key on the feed (AUTO-608)" -- services/catalog/ frontend/web/src/types/capabilities.ts
git push origin HEAD:main
```

---

# Phase 3 — Frontend

### Task 10: comboMapping reads feed `player_key`

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/comboMapping.ts`
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts` (add `playerKeyOfProvider`)
- Modify: `frontend/web/src/types/preference.ts` (widen `WatchCombo.player`)
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (call sites :1135 + every `comboToWatchCombo`/`providerToLegacyPlayer` use — `grep -n "comboToWatchCombo\|providerToLegacyPlayer" frontend/web/src/components/player/aePlayer/AePlayer.vue`)
- Test: `frontend/web/src/composables/aePlayer/comboMapping.spec.ts` (or `__tests__` sibling — locate first)

- [ ] **Step 1: Widen the type** in `types/preference.ts`:

```ts
/** Known legacy player keys. The union documents the historical set; the
 *  `(string & {})` arm keeps any roster-supplied player_key assignable while
 *  preserving autocomplete (AUTO-608 — the roster, not this type, is the
 *  authority for which players exist). */
export type LegacyPlayerKey = 'kodik' | 'animelib' | 'hanime' | 'english' | 'ae'
export interface WatchCombo {
  player: LegacyPlayerKey | (string & {})
  ...
```

(Keep the rest of the interface untouched.)

- [ ] **Step 2: Failing tests** (comboMapping spec):

```ts
it('prefers the feed player_key over the hardcoded switch', () => {
  expect(providerToLegacyPlayer('brand-new-prov', 'ru', 'newkey')).toBe('newkey')
})
it('falls back to the switch when player_key is absent (offline/feed-less)', () => {
  expect(providerToLegacyPlayer('18anime', 'adult')).toBe('hanime')
  expect(providerToLegacyPlayer('someprov', 'en')).toBe('english')
  expect(providerToLegacyPlayer('unknown', 'ru')).toBeNull()
})
```

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts` → FAIL (arity).

- [ ] **Step 3: Implement.** `comboMapping.ts`:

```ts
/** Map a granular unified provider id -> legacy WatchCombo.player (or null).
 *  AUTO-608: the capability feed's per-cap `player_key` is authoritative when
 *  present (roster-driven — a new provider needs no edit here). The group/id
 *  switch below survives ONLY as the fallback for feed-less contexts (the
 *  offline synthetic provider, tests). */
export function providerToLegacyPlayer(
  providerId: string,
  group?: string,
  playerKey?: string,
): LegacyPlayer | null {
  if (playerKey) return playerKey as LegacyPlayer
  if (group === 'en') return 'english'
  switch (providerId) {
    case 'kodik': return 'kodik'
    case 'ae': return 'ae'
    case '18anime': return 'hanime'
    case 'hanime': return 'hanime'
    case 'animelib': return 'animelib'
    default: return null
  }
}
```

`comboToWatchCombo(combo, group?)` → `comboToWatchCombo(combo, group?, playerKey?)` threading the third arg. In `useProviderFeed.ts` add next to `groupOfProvider`:

```ts
/** The roster player_key for a provider id, from the capability report (AUTO-608). */
export function playerKeyOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  if (!report || !Array.isArray(report.families)) return undefined
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.provider === providerId) return cap.player_key
    }
  }
  return undefined
}
```

In `AePlayer.vue`: `:1135` becomes `providerToLegacyPlayer(cap.provider, cap.group, cap.player_key)`; every `comboToWatchCombo(...)` call site gains `playerKeyOfProvider(report.value, <combo>.provider)` as the third arg (import from useProviderFeed; find all sites first).

- [ ] **Step 4: Run** — `cd frontend/web && bunx vitest run src/composables/aePlayer/ && bunx tsc --noEmit` → PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/
git commit -m "feat(web): watch-combo persistence keyed on feed player_key (AUTO-608)" -- frontend/web/src/
git push origin HEAD:main
```

### Task 11: Delete `UNAVAILABLE_PROVIDERS`

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (:600-608 + the header's "NOT wired" doc block)
- Test: `frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts:129` (the pinned animelib case)

- [ ] **Step 1: Update the test first.** The spec case `throws a typed error for a disabled/unwired provider` currently uses `animelib`; repoint it to a never-wired id (keep the behavior contract):

```ts
it('throws a typed error for an unwired provider', async () => {
  // AUTO-608: no FE denylist — feed omission is the truth for disabled
  // providers. Only a provider with NO adapter code throws.
  await expect(resolver.listEpisodes('no-such-provider', 'a1')).rejects.toThrow(NotAvailableError)
})
```

Run: `bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts` → this passes already (fall-through throw) but the animelib expectation must be REPLACED, and add the inverse:

```ts
it('animelib resolves via its adapter when the feed serves it (no FE denylist)', async () => {
  // Wire an animelibApi-analog… — if no animelib adapter exists in makeResolver,
  // expect NotAvailableError still (code-unwired), but NOT via a denylist.
  await expect(resolver.listEpisodes('animelib', 'a1')).rejects.toThrow(/not available/)
})
```

(Check `makeResolver` — animelib has NO adapter today, so it lands on the fall-through throw; the observable change is only the error message. Keep the test asserting the typed error and delete the denylist.)

- [ ] **Step 2: Implement** — delete the `UNAVAILABLE_PROVIDERS` set and its `if` block in `getAdapter`; update the header comment: `'animelib' has no adapter yet (falls through to NotAvailableError); the FEED decides availability — there is no FE denylist (AUTO-608).` Add a dev-console warn at the fall-through:

```ts
    if (import.meta.env.DEV) console.warn(`[resolver] no adapter for provider "${provider}" — roster row without FE code?`)
    throw new NotAvailableError(provider)
```

- [ ] **Step 3: Run** — `bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts` → PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/composables/aePlayer/
git commit -m "fix(web): drop FE provider denylist — feed omission is the truth (AUTO-608)" -- frontend/web/src/composables/aePlayer/
git push origin HEAD:main
```

### Task 12: playbackFailure — first-party by group

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/playbackFailure.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (:911 + `reportIfTerminal` inputs, ~:962)
- Test: `frontend/web/src/components/player/aePlayer/playbackFailure.spec.ts`

- [ ] **Step 1: Failing tests** (spec):

```ts
it('tags ae_failed for ANY first-party provider (group, not id)', () => {
  const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae2', firstParty: true, candidateExists: true })
  expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
})
it('does not treat a non-firstparty provider named ae-like as first-party', () => {
  const d = classifyPlaybackFailure({ ...base, failingProvider: 'aegis', firstParty: false, candidateExists: true })
  expect(d.emit).toBe(false)
})
```

Update the existing `base` fixture to include `firstParty: false` and existing ae cases to `firstParty: true`.

- [ ] **Step 2: Implement.** `FailureInputs` gains:

```ts
  /** Whether failingProvider's capability group is 'firstparty' (AUTO-608 —
   *  a second first-party provider must trip the same alert as 'ae'). */
  firstParty: boolean
```

and the classifier body swaps `const isAe = i.failingProvider === 'ae'` for `const isFirstParty = i.firstParty` (tag stays the wire-compatible `'ae_failed'` — the Grafana alert counts `effect_kind='player_failed'` rows and is unaffected; document that in a comment).

In `AePlayer.vue`: the `reportIfTerminal` call (Task context: `advanceToNextSource`, ~line 962) passes `firstParty: groupOfProvider(report.value, failingProvider) === 'firstparty'` (with a `|| failingProvider === 'ae'` safety net ONLY if `report.value` can be null mid-failure — check; if the report is always resolved before playback, skip the net). Line :911 becomes `is_first_party: inputs.firstParty`.

- [ ] **Step 3: Run** — `bunx vitest run src/components/player/aePlayer/playbackFailure.spec.ts src/components/player/aePlayer/__tests__/` → PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/
git commit -m "fix(web): playback-failure first-party tag keys on capability group (AUTO-608)" -- frontend/web/src/components/player/aePlayer/
git push origin HEAD:main
```

### Task 13: useOverrideTracker — raw player key, no kodik default

**Files:**
- Modify: `frontend/web/src/composables/useOverrideTracker.ts` (:28 type, :99 default)
- Test: its spec file if one exists (`ls frontend/web/src/composables/useOverrideTracker*`)

- [ ] **Step 1: Implement** (small enough for direct edit + type-check):
- `export type PlayerName = WatchCombo['player']` (drops the closed 3-value union; import type from preference).
- Line :99 `?? 'kodik'` → `?? opts.player` (the tracked player is the composable's own player — never mislabel an override as kodik).

- [ ] **Step 2: Run** — `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/composables/` → PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/composables/
git commit -m "fix(web): override tracker emits real player key, no kodik fallback (AUTO-608)" -- frontend/web/src/composables/
git push origin HEAD:main
```

### Task 14: P3 verify + full landing

- [ ] **Step 1: `/frontend-verify`** (skill) — DS-lint, i18n parity (no new user-facing strings expected — the dev-console warn is not i18n), real `bun run build`.
- [ ] **Step 2: Deploy** — `make redeploy-web && make health`.
- [ ] **Step 3: Live smoke (API-level, Chrome opt-in only):** load an anime page, confirm playback + saved-combo restore works (watch `/api/users/*/watch-preferences` traffic in logs), confirm a `playback_failed` event still ingests (analytics logs).
- [ ] **Step 4: `/animeenigma-after-update`** — simplify pass over the changed code, changelog (Trump-mode), final commit + push. Scope note for the changelog: "любой провайдер из БД теперь подключён ВЕЗДЕ автоматически".

## Self-review checklist (run after writing, fixed inline)

- Spec §1 → Tasks 1-2 (deviation 1 documented); §2 → Tasks 3-4; §3 → Task 5; §4 → Tasks 3, 7, 8, 9; §5 → Tasks 10-13; §6 → Tasks 1, 2, 6 (deviations 2-4). Decisions 1-3 honored (kodik alias unchanged — synthetic set documents it; columns added; three landings).
- Type consistency: `ProviderUnwired{provider,seam}` (T1↔T2↔T6); `AllNames()` (T2); `DisplayName/PlayerKey/AnimeLevel` (T3↔T4↔T7↔T8↔T9); `player_key` wire field (T9↔T10); `firstParty` input (T12); `LegacyPlayerKey | (string & {})` (T10↔T13).
- No TBD/TODO placeholders; every code step has code; helper names the executor must locate are given as grep commands.
