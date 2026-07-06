# allanime-okru Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the scraper's `okru` provider and the degraded `allanime` provider into one folded `allanime-okru` provider (AllAnime GraphQL discovery + ok.ru streams), tombstone the old `allanime` DB row, and make the frontend EN-chain routing feed-driven so provider renames need zero FE edits.

**Architecture:** One new Go package `services/scraper/internal/providers/allanimeokru/` absorbs AllAnime's discovery half and ok.ru's resolution half, dropping AllAnime's dead clock/probe stream path. The catalog DB roster renames `okru`→`allanime-okru` (guarded migration) and disables `allanime` (tombstone kept in `KnownProviders`). The frontend deletes its two hardcoded EN-provider-id sets and instead asks the capability feed "is this provider's `family` `ourenglish`?".

**Tech Stack:** Go 1.2x (scraper, catalog, analytics microservices, GORM), Vue 3 + TypeScript + Vitest (frontend), Redis cache.

## Global Constraints

- **Provider id (wire/DB/registry):** `allanime-okru` (hyphenated). **Go package name:** `allanimeokru` (no hyphen — Go identifiers can't contain `-`). The hyphen lives ONLY in `Name()`'s return string and DB/wire ids.
- **Chip label** (capability feed `display_name`): `AllAnime (OK.ru)`.
- **`allanime` is TOMBSTONED, not deleted:** its DB row is set `disabled`; `"allanime"` STAYS in scraper `KnownProviders` and catalog `scraperOperatedNames` (the remote loader hard-fails on an unrecognized `scraper_operated` roster name).
- **EN failover order unchanged:** gogoanime → animepahe → allanime-okru → miruro → nineanime → animekai. The merged provider takes okru's old slot (registered after animepahe).
- **Commit co-authors** (every commit):
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Worktree hygiene:** all edits go to worktree-root absolute paths (`.../.claude/worktrees/allanime-okru-merge/...`), NEVER `/data/animeenigma/...` (that silently edits the base tree). Commit by pathspec, never `git add -A`.
- **Deploy order (later, not a code task): catalog first, then scraper** — the scraper remote loader crash-loops if it boots against an un-migrated DB still naming `okru` while `okru` is gone from `KnownProviders`.

---

## File Structure

**scraper (`services/scraper/`):**
- Create `internal/providers/allanimeokru/{doc.go, client.go, discovery.go, cache.go, decrypt.go, dto.go, queries.go}` + tests + `testdata/` (folded from the two old dirs).
- Delete `internal/providers/okru/` and `internal/providers/allanime/`.
- Modify `cmd/scraper-api/main.go` (registration), `internal/config/providers.go` (`KnownProviders`), `internal/providers/nineanime/{cache,client,doc}.go` (comment path strings).

**catalog (`services/catalog/`):**
- Modify `internal/service/scraperprovider/seed.go` (`defaultProviders`, `scraperOperatedNames`), `internal/service/scraperprovider/migrate.go` (+ `migrate_test.go`), `cmd/catalog-api/main.go` (migration wiring), `internal/service/capability/rank.go` (`displayName`), `internal/service/sourceranking/writer.go` (`knownProviders`).

**analytics (`services/analytics/`):**
- Modify `internal/config/config.go` (`PROBE_PROVIDERS` default) + `internal/config/config_test.go`.

**frontend (`frontend/web/src/`):**
- Modify `composables/aePlayer/useProviderFeed.ts` (+ new `familyOfProvider` helper & test), `composables/aePlayer/useProviderResolver.ts` (+ spec), `composables/aePlayer/comboMapping.ts` (+ spec), `components/player/aePlayer/AePlayer.vue` (3 call sites).

**docs:** `CLAUDE.md`, `docs/scraper-framework.md`.

---

## Task 1: Scraper — fold `okru`+`allanime` into `allanimeokru`, rewire `main.go`, update `KnownProviders`

**Files:**
- Create: `services/scraper/internal/providers/allanimeokru/{doc.go, client.go, discovery.go, cache.go, decrypt.go, dto.go, queries.go, client_test.go, discovery_test.go, queries_test.go}` + `testdata/`
- Delete: `services/scraper/internal/providers/okru/`, `services/scraper/internal/providers/allanime/`
- Modify: `services/scraper/cmd/scraper-api/main.go`, `services/scraper/internal/config/providers.go`, `services/scraper/internal/providers/nineanime/{cache.go,client.go,doc.go}`

**Interfaces:**
- Produces: package `allanimeokru` exporting `New(Deps) (*Provider, error)`, `Deps{HTTP *domain.BaseHTTPClient; Cache cache.Cache; Log *logger.Logger; BaseURL string}`, and `Provider` implementing `domain.Provider` with `Name() == "allanime-okru"`.
- Consumes: `domain`, `embeds.NewOkruExtractor`, `health`, `cache`, `logger` (unchanged).

The merge splits into a **discovery half** (renamed from `allanime`'s `Provider`, stripped to pure discovery) and a **provider half** (`okru`'s `Provider`, now the real one). Every colliding exported identifier from the allanime half is unexported/renamed per this table:

| allanime (old) | allanimeokru (new) | Note |
|---|---|---|
| `type Provider` | `type discovery` | pure discovery client |
| `func New(Deps)` | `func newDiscovery(discoveryDeps)` | |
| `type Deps` | `type discoveryDeps` | avoids collision with okru's `Deps` |
| `const providerName = "allanime"` | *(deleted)* | |
| `var stageNames`, `markStage`, `HealthCheck`, `stages`, `stagesMu` | *(deleted)* | health lives on the provider half only |
| `Name()`, `GetStream()`, `ListServers()`, `materializeServers()`, `classify()`, `orderCandidates()`, `resolveSourceURL()`, `streamType()`, consts `streamGateBudget`/`maxStreamProbes`, the `sourceprobe` import | *(deleted — dead clock/probe path)* | |
| `EpisodeSourceURLs()` (exported) | `episodeSourceURLs()` (unexported) | same package now |
| `type NamedSource` | `type namedSource` | |
| `FindID`, `ListEpisodes`, `fetchSources`, `fetchShowDetail`, `categoriesFor`, `categoriesFromDetail`, `materializeEpisodes`, `splitEpisodeID`, `decodeSourceURL`, `translationTypeFor`, `doGraphQL`, cache/decrypt/dto/queries | KEEP (rename receiver `p *Provider`→`d *discovery`; strip `markStage` calls from FindID/ListEpisodes) | discovery half |

The **provider half** (from `okru/client.go`) keeps `Provider`, `Deps`, `New`, `providerName = "allanime-okru"`, `stageNames`, `markStage`, `HealthCheck`, `FindID`/`ListEpisodes` (delegate + mark stage), `isOk`, `ListServers`, `GetStream`. Its `sourceLister` interface drops the `allanime.` qualifier (`allanime.NamedSource` → `namedSource`), and `New` calls `newDiscovery(discoveryDeps{...})` instead of `allanime.New(allanime.Deps{...})`.

- [ ] **Step 1: Move the allanime files into the new package (git mv preserves history)**

```bash
cd services/scraper/internal/providers
git mv allanime allanimeokru
git mv allanimeokru/client.go allanimeokru/discovery.go
git mv allanimeokru/client_test.go allanimeokru/discovery_test.go
git mv allanimeokru/doc.go allanimeokru/doc_allanime.go   # temp; merged into doc.go in step 6
```

- [ ] **Step 2: Move the okru files in**

```bash
cd services/scraper/internal/providers
git mv okru/client.go allanimeokru/client.go
git mv okru/client_test.go allanimeokru/client_test.go
git mv okru/doc.go allanimeokru/doc.go
rmdir okru
```

- [ ] **Step 3: Rename the package declaration in every moved file**

```bash
cd services/scraper/internal/providers/allanimeokru
# allanime half was `package allanime`; okru half was `package okru`
sed -i 's/^package allanime$/package allanimeokru/; s/^package okru$/package allanimeokru/' *.go
grep -L '^package allanimeokru' *.go   # expect: no output (all files converted)
```

- [ ] **Step 4: Reduce the discovery half (`discovery.go`) to a pure discovery client**

In `discovery.go` apply the rename table above:
- `type Provider struct { ... }` → `type discovery struct { ... }`; DELETE the `stagesMu sync.Mutex` and `stages map[string]domain.StageHealth` fields (discovery carries no health).
- `func New(d Deps) (*Provider, error)` → `func newDiscovery(d discoveryDeps) (*discovery, error)`; drop the `stages`/`stageNames` seeding loop and the `providerName` references in error strings (use `"allanimeokru discovery:"`).
- `type Deps struct { ... }` → `type discoveryDeps struct { ... }` (keep the `BaseURL/HTTP/Cache/Log` fields).
- DELETE: `const providerName = "allanime"`, `var stageNames`, `func (p *Provider) Name()`, `func (p *Provider) markStage(...)`, `func (p *Provider) HealthCheck(...)`, `func (p *Provider) GetStream(...)`, `func (p *Provider) ListServers(...)`, `func materializeServers(...)`, `func (p *Provider) classify(...)`, `func orderCandidates(...)`, `func resolveSourceURL(...)`, `func streamType(...)`, `const streamGateBudget`, `const maxStreamProbes`, and the `"github.com/ILITA-hub/animeenigma/services/scraper/internal/sourceprobe"` import.
- On the surviving receivers change `p *Provider` → `d *discovery` (sed: `s/(p \*Provider)/(d *discovery)/` then fix `p\.`→`d.` inside those bodies), and DELETE every `p.markStage(...)` / `d.markStage(...)` line in `FindID` and `ListEpisodes` (the provider half marks stages).
- Rename `func (d *discovery) EpisodeSourceURLs(...)` → `func (d *discovery) episodeSourceURLs(...)` and `type NamedSource` → `type namedSource` (update the return type and the struct literal in that method).
- Delete the compile-time assertion `var _ domain.Provider = (*Provider)(nil)` (discovery is not a Provider).

- [ ] **Step 5: Wire the provider half (`client.go`) to the same-package discovery**

In `client.go` (the okru half):
- Change `const providerName = "okru"` → `const providerName = "allanime-okru"`.
- Remove the import `"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime"`.
- In the `sourceLister` interface, change `EpisodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]allanime.NamedSource, error)` → `episodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]namedSource, error)`, and change `FindID`/`ListEpisodes` method names to match the discovery's exported-vs-unexported surface (they stay `FindID`/`ListEpisodes` — discovery still exports those). So the interface becomes:
  ```go
  type sourceLister interface {
      FindID(ctx context.Context, ref domain.AnimeRef) (string, error)
      ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error)
      episodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]namedSource, error)
  }
  ```
- In `New`, replace:
  ```go
  disc, err := allanime.New(allanime.Deps{HTTP: d.HTTP, Cache: d.Cache, Log: d.Log})
  if err != nil {
      return nil, fmt.Errorf("okru: internal allanime: %w", err)
  }
  ```
  with:
  ```go
  disc, err := newDiscovery(discoveryDeps{HTTP: d.HTTP, Cache: d.Cache, Log: d.Log})
  if err != nil {
      return nil, fmt.Errorf("allanime-okru: internal discovery: %w", err)
  }
  ```
- In `ListServers`/`GetStream`, change the two `p.disc.EpisodeSourceURLs(...)` calls → `p.disc.episodeSourceURLs(...)`.
- Update error-context strings `"okru: ..."` → `"allanime-okru: ..."` (cosmetic; keep them consistent).

- [ ] **Step 6: Merge the two doc files**

Replace `doc.go` with a single package doc, and delete the temp file:
```bash
cd services/scraper/internal/providers/allanimeokru
rm doc_allanime.go
```
Write `doc.go`:
```go
// Package allanimeokru is a single EN-sub domain.Provider (id "allanime-okru")
// that pairs AllAnime's GraphQL discovery with ok.ru stream resolution.
//
// Discovery (FindID / ListEpisodes / episodeSourceURLs) hits AllAnime's
// api.allanime.day GraphQL, which works from our datacenter egress. Streaming
// keeps ONLY the "Ok" (ok.ru) sources and resolves them with the ok.ru
// extractor (static data-options → okcdn.ru HLS), deliberately avoiding
// AllAnime's Cloudflare-Turnstile-walled /apivtwo/clock endpoint (unsolvable
// from our egress). EN sub/dub only — never raw.
//
// Folded 2026-07-06 from the former `okru` provider + `allanime` discovery
// package; AllAnime's own clock/probe stream path was dropped as dead code.
package allanimeokru
```

- [ ] **Step 7: Fix the moved tests**

- `discovery_test.go` (was allanime's `client_test.go`): it constructs `New(Deps{...})` and references `*Provider`/`Name()`/`HealthCheck`/`GetStream`/`ListServers`. Retarget the DISCOVERY-only assertions to `newDiscovery(discoveryDeps{...})` and `*discovery`; DELETE tests that exercised the now-removed `GetStream`/`ListServers`/`classify`/`orderCandidates` (that behavior is gone — the provider half's `client_test.go` covers Ok streaming). Keep `FindID`/`ListEpisodes`/`fetchSources`/`episodeSourceURLs`/`decodeSourceURL`/decrypt/queries tests.
- `client_test.go` (was okru's): remove the `allanime` import (line 11); its fake `sourceLister` now returns `[]namedSource` and implements `episodeSourceURLs`. Update the fake's method name/signature and any `allanime.NamedSource{...}` literals → `namedSource{...}`.
- `queries_test.go`: package rename only (already done in step 3).
- `testdata/` moved with the dir in step 1 — no change.

- [ ] **Step 8: Rewire `cmd/scraper-api/main.go`**

- Change the import line 29 `.../providers/okru` → `.../providers/allanimeokru`, and DELETE the import line 22 `.../providers/allanime`.
- DELETE the entire `allAnimeProvider` block (the `allAnimeBaseHTTP` construction, `allanime.New(...)`, its fatal check, and `registerByStatus(allAnimeProvider)` — lines ~404–425).
- Replace the okru construction block (lines ~427–443) with the merged provider, folding the discovery per-host RPS in:
  ```go
  // allanime-okru — AllAnime GraphQL discovery + ok.ru ("Ok") stream resolution,
  // clock-free (no api.allanime.day /apivtwo/clock). Folded 2026-07-06 from the
  // former okru + allanime providers. EN failover slot: after animepahe.
  allanimeOkruBaseHTTP := domain.NewBaseHTTPClient(log,
      domain.WithPerHostRPS("api.allanime.day", 1.0, 2),
      domain.WithPerHostRPS("allmanga.to", 1.0, 2),
      domain.WithProvider("allanime-okru"),
      domain.WithTransport(egressTransport),
  )
  allanimeOkruProvider, err := allanimeokru.New(allanimeokru.Deps{
      BaseURL: cfg.AllAnime.BaseURL,
      HTTP:    allanimeOkruBaseHTTP,
      Cache:   redisCache,
      Log:     log,
  })
  if err != nil {
      log.Fatalw("failed to construct allanime-okru provider", "error", err)
  }
  registerByStatus(allanimeOkruProvider)
  ```
  (Note: `okru.Deps` had no `BaseURL` field; the merged `Deps` DOES — it's the okru-half `Deps` PLUS a `BaseURL string` field threaded to `newDiscovery`. Add that field in `client.go`'s `Deps` and pass it through in `New`.)
- Fix the failover-order comments at lines ~429, ~454, ~644 (`allanime → okru` → `allanime-okru`).

- [ ] **Step 9: Add `BaseURL` to the merged `Deps` and thread it**

In `client.go`, extend `Deps` and `New`:
```go
type Deps struct {
    BaseURL string // forwarded to the AllAnime discovery client; empty ⇒ default
    HTTP    *domain.BaseHTTPClient
    Cache   cache.Cache
    Log     *logger.Logger
}
```
and in `New`, pass it: `newDiscovery(discoveryDeps{BaseURL: d.BaseURL, HTTP: d.HTTP, Cache: d.Cache, Log: d.Log})`.

- [ ] **Step 10: Update `internal/config/providers.go` `KnownProviders`**

Change:
```go
var KnownProviders = []string{
	"gogoanime", "animepahe", "allanime", "okru", "animefever", "miruro", "nineanime", "animekai",
	"18anime",
}
```
to:
```go
var KnownProviders = []string{
	"gogoanime", "animepahe", "allanime", "allanime-okru", "animefever", "miruro", "nineanime", "animekai",
	"18anime",
}
```
(Removed `"okru"`, added `"allanime-okru"`. `"allanime"` KEPT — it is now a codeless tombstone whose disabled DB row must still validate, exactly like `"animefever"`.)

- [ ] **Step 11: Fix nineanime "adapted-from" comment paths**

```bash
cd services/scraper/internal/providers/nineanime
sed -i 's#providers/allanime/#providers/allanimeokru/#g; s#providers/allanime\b#providers/allanimeokru#g' cache.go client.go doc.go
```

- [ ] **Step 12: Build, vet, and run the scraper tests**

Run:
```bash
cd services/scraper && go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30
```
Expected: build + vet clean; tests PASS. Pre-existing known-red test `TestOrchestrator_StaleCache_DoesNotSkip` (scraper/internal/service) may still fail — confirm it fails identically on `origin/main`, otherwise investigate. The moved `allanimeokru` package tests PASS.

- [ ] **Step 13: Commit**

```bash
git add services/scraper/internal/providers/allanimeokru services/scraper/internal/providers/okru services/scraper/internal/providers/allanime services/scraper/cmd/scraper-api/main.go services/scraper/internal/config/providers.go services/scraper/internal/providers/nineanime
git commit -F - <<'EOF'
refactor(scraper): fold okru+allanime into one allanime-okru provider

Merge the AllAnime GraphQL discovery half and the ok.ru resolution half into
a single `allanimeokru` package (Name() "allanime-okru"), dropping AllAnime's
dead clock/probe stream path. Remove the standalone degraded allanime provider
registration; swap KnownProviders okru->allanime-okru (allanime kept as
tombstone). EN failover order unchanged.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 2: Catalog — roster seed, display label, srcfix allowlist

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go`
- Modify: `services/catalog/internal/service/capability/rank.go:79`
- Modify: `services/catalog/internal/service/sourceranking/writer.go:17`

**Interfaces:**
- Produces: a fresh-DB roster where `allanime-okru` is `enabled` (weight 35) and `allanime` is `disabled` (tombstone); capability feed emits `display_name="AllAnime (OK.ru)"` for `allanime-okru`.

- [ ] **Step 1: Rename the okru default-roster entry → allanime-okru**

In `seed.go` `defaultProviders`, replace the `okru` entry (currently `Name: "okru", Status: domain.StatusEnabled, ...`) with:
```go
{
	Name: "allanime-okru", Status: domain.StatusEnabled,
	Reason: "AllAnime discovery + ok.ru ('Ok') CDN streams (clock-free)",
	Description: "Folded okru+allanime (2026-07-06). Reuses AllAnime's GraphQL discovery " +
		"(api.allanime.day) and resolves ONLY its ok.ru ('Ok') sources via ok.ru data-options " +
		"metadata → okcdn.ru HLS, bypassing the Cloudflare-Turnstile-walled /apivtwo/clock " +
		"endpoint. EN sub/dub, hardsubbed (ok.ru has no soft-sub track).",
	SupportsSub: true, SupportsDub: true, SubDelivery: "unknown",
	QualityCeiling: "1080p", PreferenceWeight: 35,
},
```

- [ ] **Step 2: Tombstone the allanime default-roster entry**

In `seed.go` `defaultProviders`, change the `allanime` entry's `Status: domain.StatusDegraded` → `Status: domain.StatusDisabled`, and update its `Reason`/`Description`:
```go
{
	Name: "allanime", Status: domain.StatusDisabled,
	Reason: "Folded into allanime-okru (2026-07-06) — clock stream path was dead",
	Description: "AllAnime discovery + ok.ru streams now ship as the single 'allanime-okru' " +
		"provider. AllAnime's own primary sources decode to /apivtwo/clock.json behind a " +
		"Cloudflare Turnstile (unsolvable from our egress), so the standalone provider was " +
		"dead. Disabled tombstone; kept as the historical record. Existing DBs flipped via " +
		"AllanimeOkruMerge.",
	SupportsSub: true, SupportsDub: true, SubDelivery: "unknown",
	QualityCeiling: "1080p", PreferenceWeight: 90,
},
```

- [ ] **Step 3: Update `scraperOperatedNames`**

In `seed.go`, change the map (keep `"allanime"`, drop `"okru"`, add `"allanime-okru"`):
```go
var scraperOperatedNames = map[string]bool{
	"gogoanime": true, "animepahe": true, "allanime": true, "allanime-okru": true, "animefever": true,
	"miruro": true, "nineanime": true, "animekai": true, "18anime": true,
}
```

- [ ] **Step 4: Add the display label in `capability/rank.go`**

In the `displayName` map (line ~79), remove the `"okru": "OK.ru"` entry and add `"allanime-okru"`; keep `"allanime"`:
```go
"allanime": "AllAnime", "allanime-okru": "AllAnime (OK.ru)", "gogoanime": "GogoAnime", "animepahe": "AnimePahe",
```

- [ ] **Step 5: Update the srcfix allowlist in `sourceranking/writer.go`**

In `knownProviders` (line ~17), replace `"okru": {}` with `"allanime-okru": {}` (keep `"allanime": {}`):
```go
"ae": {}, "allanime": {}, "allanime-okru": {}, "gogoanime": {}, "miruro": {}, "animepahe": {},
```
(This is a Redis-only 24h-TTL override allowlist — no data migration; stale `okru` srcfix keys self-expire. The stale `SYNC: ... providerRegistry.ts CURATED_TIER` comment above it is already wrong — leave it or trim, don't chase it.)

- [ ] **Step 6: Build + test catalog roster/capability**

Run:
```bash
cd services/catalog && go build ./... && go test ./internal/service/scraperprovider/... ./internal/service/capability/... ./internal/service/sourceranking/... -count=1 2>&1 | tail -20
```
Expected: build clean; PASS. Fix any seed/capability test that hardcoded `"okru"`/`"OK.ru"` (search: `grep -rn '"okru"\|OK\.ru' services/catalog/internal/service/*/`*_test.go`).

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/service/scraperprovider/seed.go services/catalog/internal/service/capability/rank.go services/catalog/internal/service/sourceranking/writer.go
git commit -F - <<'EOF'
feat(catalog): seed allanime-okru, tombstone allanime in the roster

Fresh-DB roster now ships allanime-okru enabled (label "AllAnime (OK.ru)") and
allanime disabled. Update scraperOperatedNames, capability displayName, and the
srcfix allowlist. Existing DBs are carried by the AllanimeOkruMerge migration.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 3: Catalog — guarded `AllanimeOkruMerge` migration (TDD)

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go`
- Test: `services/catalog/internal/service/scraperprovider/migrate_test.go` (create if absent)
- Modify: `services/catalog/cmd/catalog-api/main.go` (wire the migration)

**Interfaces:**
- Consumes: `domain.ScraperProvider`, `migrationGuard`, `domain.StatusDisabled` (existing).
- Produces: `func AllanimeOkruMerge(db *gorm.DB) error` — renames the existing `okru` row to `allanime-okru` and disables the `allanime` row, once, guard-gated.

- [ ] **Step 1: Write the failing migration test**

Add to `services/catalog/internal/service/scraperprovider/migrate_test.go` (mirror the file's existing sqlite/gorm harness if present; otherwise use `gorm.io/driver/sqlite` in-memory). Test that an existing `okru`(enabled)+`allanime`(degraded) pair becomes `allanime-okru`(enabled)+`allanime`(disabled), and that a second run is a no-op:
```go
func TestAllanimeOkruMerge(t *testing.T) {
	db := newTestDB(t) // in-memory gorm with ScraperProvider + migrationGuard auto-migrated
	db.Create(&domain.ScraperProvider{Name: "okru", Status: domain.StatusEnabled, PreferenceWeight: 35})
	db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusDegraded, PreferenceWeight: 90})

	if err := AllanimeOkruMerge(db); err != nil {
		t.Fatalf("AllanimeOkruMerge: %v", err)
	}

	var okru int64
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "okru").Count(&okru)
	if okru != 0 {
		t.Errorf("okru row still present, want renamed")
	}
	var merged domain.ScraperProvider
	if err := db.Where("name = ?", "allanime-okru").First(&merged).Error; err != nil {
		t.Fatalf("allanime-okru row missing: %v", err)
	}
	if merged.Status != domain.StatusEnabled || merged.PreferenceWeight != 35 {
		t.Errorf("allanime-okru = %v/%d, want enabled/35", merged.Status, merged.PreferenceWeight)
	}
	var old domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&old)
	if old.Status != domain.StatusDisabled {
		t.Errorf("allanime status = %v, want disabled", old.Status)
	}

	// Idempotent: second run does not error and changes nothing.
	if err := AllanimeOkruMerge(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestAllanimeOkruMerge -count=1`
Expected: FAIL — `undefined: AllanimeOkruMerge` (or a helper `newTestDB` if you must add it — copy the harness from a sibling `*_test.go` in this package).

- [ ] **Step 3: Implement the migration**

Add to `migrate.go` (guard const near the others, function near `AllAnimeDegrade`):
```go
// allanimeOkruMergeGuardKey marks AllanimeOkruMerge as applied.
const allanimeOkruMergeGuardKey = "allanime_okru_merge"

// AllanimeOkruMerge carries the okru+allanime fold to live DBs, exactly once.
// It renames the existing `okru` row to `allanime-okru` (preserving status /
// weight / engine) and disables the standalone `allanime` row (tombstone —
// its clock stream path is dead; discovery+ok.ru now ship as allanime-okru).
// On a fresh DB the seed already wrote both rows correctly, so either UPDATE
// may affect 0 rows — that is EXPECTED, not an error. Guard-gated + idempotent.
func AllanimeOkruMerge(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeOkruMergeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-okru-merge guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator edit
	}
	// 1) Rename okru -> allanime-okru (only if no allanime-okru row exists yet,
	//    so we never collide with a fresh-DB seed row).
	var already int64
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime-okru").Count(&already).Error; err != nil {
		return fmt.Errorf("check allanime-okru presence: %w", err)
	}
	if already == 0 {
		if err := db.Model(&domain.ScraperProvider{}).
			Where("name = ?", "okru").
			Updates(map[string]interface{}{
				"name":        "allanime-okru",
				"reason":      "AllAnime discovery + ok.ru ('Ok') CDN streams (clock-free)",
				"description": "Folded okru+allanime (2026-07-06). AllAnime GraphQL discovery + ok.ru data-options → okcdn.ru HLS, bypassing the Cloudflare-Turnstile /apivtwo/clock endpoint. EN sub/dub, hardsubbed.",
			}).Error; err != nil {
			return fmt.Errorf("rename okru->allanime-okru: %w", err)
		}
	}
	// 2) Tombstone the standalone allanime row (degraded -> disabled).
	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime").
		Updates(map[string]interface{}{
			"status": domain.StatusDisabled,
			"reason": "Folded into allanime-okru (2026-07-06) — clock stream path was dead",
		}).Error; err != nil {
		return fmt.Errorf("disable allanime: %w", err)
	}
	if err := db.Create(&migrationGuard{Key: allanimeOkruMergeGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-okru-merge guard: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestAllanimeOkruMerge -count=1`
Expected: PASS.

- [ ] **Step 5: Wire the migration into catalog boot**

In `services/catalog/cmd/catalog-api/main.go`, add after the `RemoveRawProvider` call (line ~327), mirroring the existing pattern:
```go
	if err := scraperprovider.AllanimeOkruMerge(db.DB); err != nil {
		log.Fatalw("AllanimeOkruMerge migration failed", "error", err)
	}
```
(Runs after the seed + the other guarded migrations, so on a fresh DB the seed's rows are already correct and this no-ops.)

- [ ] **Step 6: Build + full scraperprovider test**

Run: `cd services/catalog && go build ./... && go test ./internal/service/scraperprovider/... -count=1`
Expected: build clean; PASS.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/service/scraperprovider/migrate.go services/catalog/internal/service/scraperprovider/migrate_test.go services/catalog/cmd/catalog-api/main.go
git commit -F - <<'EOF'
feat(catalog): AllanimeOkruMerge migration renames okru, tombstones allanime

Guarded run-once migration carries the okru->allanime-okru rename (preserving
status/weight) and the allanime degraded->disabled tombstone to existing DBs.
No-op on fresh DBs (seed already correct). Wired into catalog boot.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 4: Analytics — probe roster + fix the stale test

**Files:**
- Modify: `services/analytics/internal/config/config.go:117`
- Modify: `services/analytics/internal/config/config_test.go:24`

**Interfaces:** none exported; changes a default string constant.

- [ ] **Step 1: Update the `PROBE_PROVIDERS` default**

In `config.go`, change the default (remove `allanime` and `okru`, insert `allanime-okru` in okru's slot):
```go
ProbeProviders: getEnv("PROBE_PROVIDERS", "gogoanime,miruro,allanime-okru,nineanime,animepahe,animefever,ae,kodik-noads,animejoy-sibnet,animejoy-allvideo"),
```

- [ ] **Step 2: Fix the assertion in `config_test.go`**

The current assertion is stale (it omits the `animejoy-*` legs, so it fails today). Replace it with the exact new default:
```go
	if cfg.ProbeProviders != "gogoanime,miruro,allanime-okru,nineanime,animepahe,animefever,ae,kodik-noads,animejoy-sibnet,animejoy-allvideo" {
		t.Errorf("ProbeProviders = %q, want gogoanime,miruro,allanime-okru,...,animejoy-allvideo", cfg.ProbeProviders)
	}
```

- [ ] **Step 3: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/config/... -count=1`
Expected: PASS (was failing before due to the stale animejoy omission).

- [ ] **Step 4: Commit**

```bash
git add services/analytics/internal/config/config.go services/analytics/internal/config/config_test.go
git commit -F - <<'EOF'
fix(analytics): probe allanime-okru; correct stale ProbeProviders test

Swap okru->allanime-okru in the probe roster and drop the tombstoned allanime.
Also fixes the pre-existing config_test assertion that omitted the animejoy legs.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 5: Frontend — feed-driven adapter routing (TDD)

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts` (add `familyOfProvider`)
- Test: `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts` (create/extend)
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (delete `SCRAPER_IDS`, add `familyOf` dep, EN branch)
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue:585`

**Interfaces:**
- Produces: `familyOfProvider(report: CapabilityReport | null, providerId: string): string | undefined`; `ResolverDeps.familyOf?: (providerId: string) => string | undefined`; `useProviderResolver(familyOf?: (id: string) => string | undefined)`.

- [ ] **Step 1: Write the failing helper test**

Add `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts` (or a new `describe` if the file exists):
```ts
import { describe, it, expect } from 'vitest'
import { familyOfProvider } from './useProviderFeed'
import type { CapabilityReport } from '@/types/capabilities'

const report = {
  anime_id: 'x',
  families: [
    { family: 'ourenglish', providers: [{ provider: 'gogoanime' }, { provider: 'allanime-okru' }] },
    { family: 'kodik', providers: [{ provider: 'kodik' }] },
  ],
} as unknown as CapabilityReport

describe('familyOfProvider', () => {
  it('returns the family for a known provider', () => {
    expect(familyOfProvider(report, 'allanime-okru')).toBe('ourenglish')
    expect(familyOfProvider(report, 'kodik')).toBe('kodik')
  })
  it('returns undefined for an unknown provider or null report', () => {
    expect(familyOfProvider(report, 'nope')).toBeUndefined()
    expect(familyOfProvider(null, 'gogoanime')).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: FAIL — `familyOfProvider is not a function`.

- [ ] **Step 3: Implement `familyOfProvider`**

Add to `useProviderFeed.ts`:
```ts
import type { CapabilityReport } from '@/types/capabilities'

/** Provider id → its backend `family` ('ourenglish' | 'kodik' | …), read straight
 *  from the capability feed. Single source of truth for "which backend serves this
 *  provider" — the FE no longer hardcodes provider-id membership lists. */
export function familyOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  if (!report || !Array.isArray(report.families)) return undefined
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.provider === providerId) return fam.family
    }
  }
  return undefined
}
```
(`CapabilityReport` may already be imported at the top — if so, don't duplicate the import.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: PASS.

- [ ] **Step 5: Delete `SCRAPER_IDS`, add the `familyOf` dep + EN branch in `useProviderResolver.ts`**

- DELETE the whole `export const SCRAPER_IDS = new Set<string>([...])` block (lines ~228–240).
- In `ResolverDeps`, add:
  ```ts
  export interface ResolverDeps {
    scraperApi?: typeof scraperApi
    anime18Api?: typeof anime18Api
    kodikApi?: typeof kodikApi
    aeApi?: typeof aeApi
    hanimeApi?: typeof hanimeApi
    animejoyApi?: typeof animejoyApi
    /** provider id → backend family (from the capability feed). Feed-driven EN-chain routing. */
    familyOf?: (providerId: string) => string | undefined
  }
  ```
- In `getAdapter`, replace the `if (SCRAPER_IDS.has(provider)) { ... }` block with:
  ```ts
  if (deps.familyOf?.(provider) === 'ourenglish') {
    if (!deps.scraperApi) {
      throw new NotAvailableError(provider, 'not available (scraperApi dep missing)')
    }
    return makeScraperAdapter(deps.scraperApi, provider)
  }
  ```
- Update the composable signature:
  ```ts
  export function useProviderResolver(familyOf?: (id: string) => string | undefined): ProviderResolver {
    return makeResolver({ scraperApi, anime18Api, kodikApi, aeApi, hanimeApi, animejoyApi, familyOf })
  }
  ```
- Update the header doc comment (line ~11) `covers all SCRAPER_IDS` → `covers the EN scraper chain (capability family "ourenglish")`, and the dispatch-rules comment (line ~586) `provider in SCRAPER_IDS` → `familyOf(provider) === 'ourenglish'`.

- [ ] **Step 6: Update `useProviderResolver.spec.ts`**

Every scraper-routing test builds `makeResolver({ scraperApi } as any)` and calls `resolver.listEpisodes('allanime', ...)`. Add `familyOf` to the deps and switch the id to `allanime-okru`:
- Replace `makeResolver({ scraperApi } as any)` → `makeResolver({ scraperApi, familyOf: () => 'ourenglish' } as any)` in the scraper tests (the blocks around lines 39, 83, 117, 189, 304).
- Replace the provider id `'allanime'` with `'allanime-okru'` in those tests' `listEpisodes`/`resolveStream`/`combo.provider`/`toHaveBeenCalledWith` assertions (lines ~40–57, ~118–120).
- The kodik/ae/hanime/18anime tests need no `familyOf` (those branches are id-exact) — leave them, but for the "scraper NOT called for hanime/18anime" tests (lines ~189, ~304) add `familyOf: () => undefined` so the EN branch is definitively skipped.

- [ ] **Step 7: Wire `AePlayer.vue` to pass `familyOf` from the live report**

In `AePlayer.vue`, import the helper and thread it into the resolver:
- Add `familyOfProvider` to the existing import from `useProviderFeed` (line 476): `import { rowsFromReport, familyOfProvider } from '@/composables/aePlayer/useProviderFeed'`.
- Change line 585:
  ```ts
  const resolver = props.offline ? makeOfflineResolver(props.offline) : useProviderResolver((id) => familyOfProvider(report.value, id))
  ```
  (`report` is the `computed<CapabilityReport | null>` at line 745. The closure reads `report.value` lazily at resolve time — after the feed has loaded — so routing always sees the current roster. `report` is declared later in the file but the closure only executes at call time, so hoisting is not an issue; verify tsc is clean in step 8.)

- [ ] **Step 8: Type-check + run the frontend tests**

Run:
```bash
cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts src/composables/aePlayer/useProviderResolver.spec.ts && bunx tsc --noEmit 2>&1 | tail -20
```
Expected: vitest PASS; tsc clean.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useProviderFeed.ts frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts frontend/web/src/composables/aePlayer/useProviderResolver.ts frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -F - <<'EOF'
refactor(web): route EN scraper adapter by capability family, drop SCRAPER_IDS

The resolver no longer hardcodes EN provider ids: it asks the capability feed
(familyOf(provider) === 'ourenglish'). Provider renames/adds need no FE change.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 6: Frontend — feed-driven combo persistence, drop `EN_SCRAPER_IDS`

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/comboMapping.ts`
- Modify: `frontend/web/src/composables/aePlayer/comboMapping.spec.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (lines ~976, ~1146)

**Interfaces:**
- Consumes: `familyOfProvider` (Task 5).
- Produces: `providerToLegacyPlayer(providerId: string, family?: string): LegacyPlayer | null`; `comboToWatchCombo(combo: Combo, family?: string): WatchCombo | null`.

- [ ] **Step 1: Update the `comboMapping.spec.ts` assertions (write the new expectation first)**

- The EN-ids loop (lines 5–8) becomes a family check:
  ```ts
  it('maps ourenglish-family providers to english', () => {
    for (const id of ['gogoanime', 'allanime-okru', 'animepahe', 'nineanime', 'miruro']) {
      expect(providerToLegacyPlayer(id, 'ourenglish')).toBe('english')
    }
  })
  ```
- The single-id cases (lines 11–14) stay but pass no family (undefined): `expect(providerToLegacyPlayer('kodik')).toBe('kodik')` still holds.
- The unmappable case (line 17): `expect(providerToLegacyPlayer('nope')).toBeNull()` still holds.
- The full round-trip (lines 47–52): change `provider: 'allanime'` → `provider: 'allanime-okru'` and pass the family into `comboToWatchCombo(combo, 'ourenglish')`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts`
Expected: FAIL (signature/behavior mismatch — `providerToLegacyPlayer` ignores the 2nd arg / `EN_SCRAPER_IDS` still gates).

- [ ] **Step 3: Rewrite `comboMapping.ts` to be family-driven for the EN case**

- DELETE the `EN_SCRAPER_IDS` const and the `// EN scraper chain -> coarse 'english'. Keep in sync with SCRAPER_IDS.` comment (lines 6–7).
- Change `providerToLegacyPlayer`:
  ```ts
  /** Map a granular unified provider id -> coarse legacy WatchCombo.player (or null).
   *  EN-chain membership is backend-driven: pass the provider's capability `family`
   *  (from familyOfProvider) — family 'ourenglish' ⇒ 'english'. The remaining
   *  single-provider families stay keyed on id. */
  export function providerToLegacyPlayer(providerId: string, family?: string): LegacyPlayer | null {
    if (family === 'ourenglish') return 'english'
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
- Change `comboToWatchCombo` to thread the family:
  ```ts
  export function comboToWatchCombo(combo: Combo, family?: string): WatchCombo | null {
    const player = providerToLegacyPlayer(combo.provider, family)
    if (!player) return null
    return {
      player,
      language: langToLanguage[combo.lang],
      watch_type: combo.audio,
      translation_id: '',
      translation_title: combo.team ?? '',
    }
  }
  ```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts`
Expected: PASS.

- [ ] **Step 5: Update the two `AePlayer.vue` call sites**

- Line ~976 (inside `for (const fam of rep.families) { for (const cap of fam.providers ...)`): `fam.family` is in scope, so:
  ```ts
  const player = providerToLegacyPlayer(cap.provider, fam.family)
  ```
- Line ~1146:
  ```ts
  () => comboToWatchCombo(state.combo.value, familyOfProvider(report.value, state.combo.value.provider)),
  ```
  (`familyOfProvider` is already imported from Task 5, step 7.)

- [ ] **Step 6: Type-check + run comboMapping tests + broader player specs**

Run:
```bash
cd frontend/web && bunx vitest run src/composables/aePlayer/comboMapping.spec.ts && bunx tsc --noEmit 2>&1 | tail -20
```
Expected: vitest PASS; tsc clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/composables/aePlayer/comboMapping.ts frontend/web/src/composables/aePlayer/comboMapping.spec.ts frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -F - <<'EOF'
refactor(web): drive combo persistence by capability family, drop EN_SCRAPER_IDS

providerToLegacyPlayer takes the provider's capability family; family
'ourenglish' -> legacy 'english'. Removes the last hardcoded EN provider-id list.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 7: Docs — update provider references

**Files:**
- Modify: `CLAUDE.md`, `docs/scraper-framework.md`

- [ ] **Step 1: Update `CLAUDE.md`**

Search for `okru` and the scraper failover chain / source-family table. Replace the standalone `okru` provider references with `allanime-okru`:
- The EN scraper failover order lines (`gogoanime → animepahe → allanime → okru → miruro …`) → `… → allanime-okru → miruro …` (allanime and okru collapse to one entry).
- The "5 source families" / AllAnime-raw notes that mention `okru` as a distinct provider → `allanime-okru`.
Run `grep -n 'okru\|allanime' CLAUDE.md` and reconcile each hit (leave the `SupportsRaw`/`has_raw` and AllAnime-raw-library notes that are about the deleted `raw` provider, not this merge).

- [ ] **Step 2: Update `docs/scraper-framework.md`**

Run `grep -n 'okru\|allanime' docs/scraper-framework.md`; replace standalone `okru` provider references with `allanime-okru`, and note the fold (okru+allanime → allanime-okru, clock path dropped) where the provider roster is described.

- [ ] **Step 3: Verify no stale standalone refs remain**

Run:
```bash
grep -rn '\bokru\b' CLAUDE.md docs/scraper-framework.md
```
Expected: only historical/embed-extractor mentions (the `embeds/okru.go` ok.ru host extractor keeps its name) — no references to `okru` as a *provider*.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/scraper-framework.md
git commit -F - <<'EOF'
docs: reflect okru+allanime -> allanime-okru merge

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Post-implementation (not code tasks)

- **Frontend pre-flight:** run `/frontend-verify` (DS-lint, i18n parity, real `bun run build`) before deploy.
- **Deploy order:** `redeploy.sh catalog` FIRST (runs `AllanimeOkruMerge` → roster shows `allanime-okru`), verify the roster, THEN `redeploy.sh scraper`, then `analytics` + `web`. Deploy from a clean `origin/main` worktree with `docker/.env`.
- **Live verify:** `prefer=allanime-okru` → episodes → servers (OK.ru sub+dub) → stream 200 (`*.okcdn.ru/*.m3u8`, valid `#EXTM3U`); capability feed chip reads "AllAnime (OK.ru)", no active `okru`/`allanime` rows; probe surfaces `allanime-okru`.
- **After-update:** run `/animeenigma-after-update` (simplify, changelog in RU Trump-mode, redeploy, health, push).

---

## Self-Review

**Spec coverage:** §A fold → Task 1. §B registration/KnownProviders → Task 1 (steps 8–10). §C seed/migration/rank/sourceranking → Tasks 2+3. §D analytics → Task 4. §E frontend feed-driven → Tasks 5+6. Back-compat (srcfix Redis TTL) → Task 2 step 5. Deploy-order hazard → Post-implementation. Docs → Task 7. **Deviation from spec §E:** the spec suggested propagating `family` onto `ProviderRow`; the plan instead uses a single `familyOfProvider(report, id)` helper (both consumers have the report in scope), which is strictly simpler and needs no type change — the intent (feed-driven, zero per-provider coupling) is fully met.

**Placeholder scan:** none — every code step shows the code; migration, helper, and spec edits are complete.

**Type consistency:** `familyOfProvider(report, id): string | undefined` used identically in Task 5 (resolver `familyOf`) and Task 6 (AePlayer 1146). `providerToLegacyPlayer(id, family?)` and `comboToWatchCombo(combo, family?)` signatures match between Task 6's definition and its callers. Merged Go `Deps{BaseURL, HTTP, Cache, Log}` (Task 1 steps 8–9) matches the `allanimeokru.New` call in main.go. Migration `AllanimeOkruMerge(db)` name matches its test (Task 3) and boot wiring (Task 3 step 5).
