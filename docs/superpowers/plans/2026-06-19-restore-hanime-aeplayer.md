# Restore Hanime Provider & Wire into aePlayer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore `hanime` as a selectable 18+ source inside aePlayer (alongside `18anime`), re-enabling its roster row and adding a slug-keyed resolver adapter, plus sort the source menu so available providers float to the top.

**Architecture:** All backend infra (parser, handlers, routes, HLS-proxy allowlist, creds) is already intact — this is a faithful reversal. Backend: a forward-only guarded migration re-enables the disabled roster row. Frontend: a new `makeHanimeAdapter` (modeled on the slug-keyed `18anime` adapter) + registry un-gate + a two-key sort in the Source panel.

**Tech Stack:** Go 1.x + GORM (catalog service), Vue 3 + TypeScript + Vitest (frontend), `make` for build/deploy.

## Global Constraints

- All work happens in the clean origin/main worktree `/tmp/ae-hanime-restore` (the shared `/data/animeenigma` tree is stale). Commit path-scoped; push after every commit (`git push origin HEAD:main`).
- Commit co-authors (every commit):
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Frontend uses `bun` / `bunx` (never npm/pnpm/npx).
- DS-lint: add no off-palette colors, arbitrary spacing, or off-scale font weights. `#ff4d8d` already exists in `providerRegistry.ts` (`.ts` hex is exempt — Rule 2 is `.vue`-only).
- aePlayer is `useI18n`-free (template `$t` only) — do NOT add `useI18n()` in any aePlayer composable/component.
- `animelib` stays `disabled` — do not touch it.
- Effort/impact metrics use UXΔ / CDI / MVQ (no time units) in any changelog entry.

---

### Task 1: Backend — re-enable the `hanime` roster row

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go` (hanime row, ~line 104-112)
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go` (add `ReEnableHanime` + guard key)
- Modify: `services/catalog/cmd/catalog-api/main.go` (wire `ReEnableHanime` after `RetireHanimeAnimelib`, ~line 167-169)
- Test: `services/catalog/internal/service/scraperprovider/migrate_test.go` (add two `ReEnableHanime` tests)

**Interfaces:**
- Produces: `func ReEnableHanime(db *gorm.DB) error` — guarded run-once; flips `hanime` row `status → enabled`; never clobbers a later operator disable. Boot order: must run AFTER `SeedDefaults` and `RetireHanimeAnimelib`.
- Consumes: existing `migrationGuard` struct + `catalog_migration_guards` table, `domain.StatusEnabled`, `domain.ScraperProvider`.

- [ ] **Step 1: Write the failing migration tests**

Append to `services/catalog/internal/service/scraperprovider/migrate_test.go`:

```go
func TestReEnableHanime_EnablesHanimeOnly(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Simulate the live-DB state: Plan B retired hanime + animelib.
	if err := scraperprovider.RetireHanimeAnimelib(db); err != nil {
		t.Fatalf("retire: %v", err)
	}

	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable: %v", err)
	}

	var hanime, animelib domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	db.First(&animelib, "name = ?", "animelib")
	if hanime.Status != domain.StatusEnabled {
		t.Errorf("hanime status = %q, want enabled", hanime.Status)
	}
	// animelib must stay retired — only hanime is restored.
	if animelib.Status != domain.StatusDisabled {
		t.Errorf("animelib status = %q, want disabled", animelib.Status)
	}
}

func TestReEnableHanime_GuardedDoesNotClobberOperatorDisable(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := scraperprovider.SeedDefaults(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable1: %v", err)
	}
	// Operator later disables hanime again.
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "hanime").
		Update("status", domain.StatusDisabled)
	// Second boot must NOT clobber the operator's disable (guard already set).
	if err := scraperprovider.ReEnableHanime(db); err != nil {
		t.Fatalf("reenable2: %v", err)
	}
	var hanime domain.ScraperProvider
	db.First(&hanime, "name = ?", "hanime")
	if hanime.Status != domain.StatusDisabled {
		t.Errorf("hanime status = %q, want disabled (guard clobbered operator disable)", hanime.Status)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail (compile error: `ReEnableHanime` undefined)**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestReEnableHanime -count=1`
Expected: FAIL — `undefined: scraperprovider.ReEnableHanime`

- [ ] **Step 3: Implement `ReEnableHanime`**

Append to `services/catalog/internal/service/scraperprovider/migrate.go` (after `RetireHanimeAnimelib`, before `BackfillScraperOperated`):

```go
// reEnableHanimeGuardKey marks ReEnableHanime as applied.
const reEnableHanimeGuardKey = "reenable_hanime"

// ReEnableHanime re-enables the hanime roster row exactly once. Forward-only
// counterpart to RetireHanimeAnimelib: hanime was retired in Plan B (2026-06-18)
// but restored as an in-aePlayer 18+ source (2026-06-19). RUN-ONCE guarded via
// the catalog_migration_guards ledger, so on every subsequent boot it is a no-op
// and an operator who later re-disables hanime is NOT clobbered. Must run AFTER
// SeedDefaults + RetireHanimeAnimelib so it wins the final status on fresh DBs
// (seed=enabled -> retire disables -> this re-enables). animelib is intentionally
// left disabled. Idempotent; safe to call every boot.
func ReEnableHanime(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", reEnableHanimeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check reenable-hanime guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-disable
	}

	if err := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "hanime").
		Update("status", domain.StatusEnabled).Error; err != nil {
		return fmt.Errorf("re-enable hanime (status=enabled): %w", err)
	}

	if err := db.Create(&migrationGuard{Key: reEnableHanimeGuardKey}).Error; err != nil {
		return fmt.Errorf("write reenable-hanime guard: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestReEnableHanime -count=1`
Expected: PASS (both tests)

- [ ] **Step 5: Flip the seed row to enabled (steady-state truth for fresh DBs)**

In `services/catalog/internal/service/scraperprovider/seed.go`, replace the `hanime` row:

```go
	{
		Name: "hanime", Status: domain.StatusEnabled,
		Reason:      "18+ source restored into aePlayer (2026-06-19)",
		Description: "Hanime HLS. Selectable 18+ source inside aePlayer (hentai titles); " +
			"catalog-operated parser via /hanime/* routes.",
		SubDelivery: "none", QualityCeiling: "1080p", PreferenceWeight: 0,
	},
```

- [ ] **Step 6: Wire `ReEnableHanime` into catalog boot**

In `services/catalog/cmd/catalog-api/main.go`, immediately after the `RetireHanimeAnimelib(db.DB)` block (the one ending `~line 169`) and before the `EmitCatalogSideRoster` block, insert:

```go
	// Forward-only: hanime was retired in Plan B (2026-06-18) but restored as an
	// in-aePlayer 18+ source (2026-06-19). MUST run after RetireHanimeAnimelib so
	// it wins the final status on fresh DBs. Run-once guarded; an operator who
	// later re-disables it is never clobbered.
	if err := scraperprovider.ReEnableHanime(db.DB); err != nil {
		log.Errorw("re-enable hanime failed (continuing)", "error", err)
	}
```

- [ ] **Step 7: Run the full package test suite + build**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -count=1 && go build ./...`
Expected: PASS — all scraperprovider tests green (existing `TestRetireHanimeAnimelib_*` and `TestSeedDefaults_*` still pass: seed count stays 13; retire still disables both), `go build` clean.

- [ ] **Step 8: Commit**

```bash
cd /tmp/ae-hanime-restore
git add services/catalog/internal/service/scraperprovider/seed.go \
        services/catalog/internal/service/scraperprovider/migrate.go \
        services/catalog/internal/service/scraperprovider/migrate_test.go \
        services/catalog/cmd/catalog-api/main.go
git commit services/catalog/internal/service/scraperprovider/seed.go \
           services/catalog/internal/service/scraperprovider/migrate.go \
           services/catalog/internal/service/scraperprovider/migrate_test.go \
           services/catalog/cmd/catalog-api/main.go \
  -m "feat(catalog): re-enable hanime roster row (forward-only ReEnableHanime migration)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -12
git push origin HEAD:main
```

---

### Task 2: Frontend — `makeHanimeAdapter` + dispatch + registry un-gate

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (import, types, adapter, deps, dispatch, `UNAVAILABLE_PROVIDERS`, default `makeResolver`, doc comment)
- Modify: `frontend/web/src/components/player/aePlayer/providerRegistry.ts` (remove `staticDisabled` from `hanime`)
- Test: `frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts` (add 3 hanime tests)

**Interfaces:**
- Consumes: `hanimeApi` from `@/api/client` (`getEpisodes(animeId)` → `{data:{data:[{name,slug}]}}`; `getStream(animeId, slug)` → `{data:{data:{sources:[{url,height,width,size_mb}]}}}`), `buildProxyUrl`, `NotAvailableError`, `EpisodeOption`, `StreamResult`, `ProviderAdapter`, `ResolverDeps`.
- Produces: a `'hanime'` dispatch case in `makeResolver().getAdapter`; `ResolverDeps.hanimeApi?`. Slug-keyed: `EpisodeOption.key = slug`, ordinal derived from index (hanime episodes carry no number).

- [ ] **Step 1: Write the failing resolver tests**

Append to `frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts` (inside the top `describe('useProviderResolver', …)` block; `proxyParams` helper already exists at the top of the file):

```ts
  it('routes hanime to the hanime adapter (slug-keyed, NOT the scraper)', async () => {
    const scraperApi = { getEpisodes: vi.fn(), getServers: vi.fn(), getStream: vi.fn() }
    const hanimeApi = {
      getEpisodes: vi.fn().mockResolvedValue({
        data: { data: [{ name: 'Episode 1', slug: 'show-1' }, { name: 'Episode 2', slug: 'show-2' }] },
      }),
      getStream: vi.fn().mockResolvedValue({
        data: { data: { sources: [
          { url: 'http://cdn/480.m3u8', height: '480', width: 854, size_mb: 100 },
          { url: 'http://cdn/1080.m3u8', height: '1080', width: 1920, size_mb: 500 },
        ] } },
      }),
    }
    const resolver = makeResolver({ scraperApi, hanimeApi } as any)
    const eps = await resolver.listEpisodes('hanime', 'uuid')
    expect(hanimeApi.getEpisodes).toHaveBeenCalledWith('uuid')
    expect(scraperApi.getEpisodes).not.toHaveBeenCalled()
    expect(eps.length).toBe(2)
    expect(eps[0].key).toBe('show-1') // slug-keyed
    expect(eps[0].number).toBe(1)     // ordinal derived from index
    const stream = await resolver.resolveStream('hanime', 'uuid', eps[0], {
      audio: 'dub', lang: 'ru', provider: 'hanime', server: '', team: null,
    })
    expect(hanimeApi.getStream).toHaveBeenCalledWith('uuid', 'show-1')
    const params = proxyParams(stream.url)
    expect(params.get('url')).toBe('http://cdn/1080.m3u8') // highest-res source
    expect(stream.type).toBe('hls')
  })

  it('throws NotAvailableError when hanime returns no sources', async () => {
    const hanimeApi = {
      getEpisodes: vi.fn().mockResolvedValue({ data: { data: [{ name: 'E1', slug: 's1' }] } }),
      getStream: vi.fn().mockResolvedValue({ data: { data: { sources: [] } } }),
    }
    const resolver = makeResolver({ hanimeApi } as any)
    const eps = await resolver.listEpisodes('hanime', 'uuid')
    await expect(
      resolver.resolveStream('hanime', 'uuid', eps[0], { audio: 'dub', lang: 'ru', provider: 'hanime', server: '', team: null }),
    ).rejects.toThrow(/no stream URL/)
  })

  it('throws NotAvailableError for hanime when the hanimeApi dep is missing', async () => {
    const resolver = makeResolver({} as any)
    await expect(resolver.listEpisodes('hanime', 'uuid')).rejects.toThrow()
  })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts`
Expected: FAIL — hanime routes to `NotAvailableError` (still in `UNAVAILABLE_PROVIDERS`); `getEpisodes` not called.

- [ ] **Step 3: Add the hanime import + types + adapter**

In `useProviderResolver.ts`, extend the `@/api/client` import (add `hanimeApi`):

```ts
import { scraperApi, rawApi, anime18Api, kodikApi, aeApi, hanimeApi } from '@/api/client'
```

Add the hanime types next to the Anime18 types block (after the `Anime18Source` interface, ~line 110):

```ts
// ─── Hanime types (mirrored from domain.HanimeEpisode / HanimeStream) ────────

interface HanimeEpisode {
  name: string
  slug: string
}

interface HanimeSource {
  url: string
  height: string
  width: number
  size_mb: number
}

interface HanimeStream {
  sources: HanimeSource[]
}
```

Add the adapter immediately after `makeAnime18Adapter` (~line 345):

```ts
function makeHanimeAdapter(api: typeof hanimeApi): ProviderAdapter {
  return {
    async listEpisodes(animeId: string): Promise<EpisodeOption[]> {
      const response = await api.getEpisodes(animeId)
      const data: HanimeEpisode[] = response.data?.data || response.data || []
      // Hanime episodes carry only {name, slug} — derive the ordinal from order.
      return (Array.isArray(data) ? data : []).map((ep, i) => ({
        key: ep.slug, // slug is the native identifier needed by getStream
        label: i + 1,
        number: i + 1,
        title: ep.name || undefined,
      }))
    },

    async resolveStream(animeId: string, ep: EpisodeOption): Promise<StreamResult> {
      const slug = String(ep.key)
      const response = await api.getStream(animeId, slug)
      const data: HanimeStream | undefined = response.data?.data || response.data
      const sources = data?.sources ?? []
      // Highest-resolution source first (width desc; height is a numeric string).
      const best = [...sources].sort(
        (a, b) => (b.width || parseInt(b.height, 10) || 0) - (a.width || parseInt(a.height, 10) || 0),
      )[0]
      if (!best?.url) {
        throw new NotAvailableError('hanime', 'returned no stream URL')
      }
      const type: 'hls' | 'mp4' = best.url.includes('.m3u8') ? 'hls' : 'mp4'
      // Hanime CDN hosts are allowlisted in the HLS proxy and the stream URLs are
      // token-signed, so no source Referer is required (verified at smoke time).
      return {
        url: buildProxyUrl(best.url, '', type),
        type,
      }
    },
  }
}
```

- [ ] **Step 4: Wire the dep, dispatch case, and un-gate**

In `ResolverDeps` (~line 168), add:

```ts
  hanimeApi?: typeof hanimeApi
```

In `makeResolver`, change `UNAVAILABLE_PROVIDERS` to drop `'hanime'` (keep `animelib`):

```ts
  const UNAVAILABLE_PROVIDERS = new Set<string>([
    'animelib', // upstream went Kodik-only
  ])
```

Add the dispatch case in `getAdapter`, immediately after the `'18anime'` block and before the final `throw`:

```ts
    if (provider === 'hanime') {
      if (!deps.hanimeApi) {
        throw new NotAvailableError(provider, 'not available (hanimeApi dep missing)')
      }
      return makeHanimeAdapter(deps.hanimeApi)
    }
```

Update the default `useProviderResolver()` factory (~line 540) to inject `hanimeApi`:

```ts
export function useProviderResolver(): ProviderResolver {
  return makeResolver({ scraperApi, rawApi, anime18Api, kodikApi, aeApi, hanimeApi })
}
```

Update the stale top-of-file doc comment: replace the `• 'hanime' — … kept as NotAvailableError until a slug-keyed adapter is wired.` bullet with a hanime adapter description mirroring the 18anime bullet, e.g.:

```ts
 * • hanimeAdapter   — covers 'hanime' (18+) via hanimeApi (/hanime/* catalog
 *   routes). Slug-keyed like anime18 (episode key = slug); the ordinal is
 *   derived from list order since hanime episodes carry no number.
```

(and remove the `NOT wired` note for hanime in that header comment).

- [ ] **Step 5: Run the resolver tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts`
Expected: PASS — all existing + 3 new hanime tests green.

- [ ] **Step 6: Un-gate the registry**

In `frontend/web/src/components/player/aePlayer/providerRegistry.ts`, replace the `hanime` entry (remove the `staticDisabled` block):

```ts
  { id: 'hanime',  name: 'Hanime', hue: '#ff4d8d', group: 'adult', audios: ['dub'], langs: ['ru'], content: ['hentai'], scraper: false },
```

(Leave its `CURATED_TIER` position unchanged — last, after `animelib`.)

- [ ] **Step 7: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: PASS — no type errors (new adapter + deps typed).

- [ ] **Step 8: Commit**

```bash
cd /tmp/ae-hanime-restore
git commit frontend/web/src/composables/aePlayer/useProviderResolver.ts \
           frontend/web/src/composables/aePlayer/useProviderResolver.spec.ts \
           frontend/web/src/components/player/aePlayer/providerRegistry.ts \
  -m "feat(aeplayer): wire hanime (18+) source via slug-keyed adapter; un-gate registry

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -10
git push origin HEAD:main
```

---

### Task 3: Frontend — sort the Source menu so available providers float to the top

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SourcePanel.vue` (`sortedRows` computed ~line 231; import `ChipState`)
- Test: `frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts` (add a sort test)

**Interfaces:**
- Consumes: `ProviderRow.state` (`ChipState`), `props.rankedIds`.
- Produces: `sortedRows` ordered by availability bucket first, capability ranking as tiebreak. `activeRows`/`topRow`/`visibleRows` are unchanged (they already filter `state === 'active'`), so the collapsed default view is unaffected; the change is visible in the full (hacker-mode / error-expanded) list.

- [ ] **Step 1: Write the failing sort test**

Append to `frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts` (inside the `describe('SourcePanel collapse', …)` block, which already defines the `a(id, state)` helper, `cb`, and `mountOpts`):

```ts
  it('sorts available rows above unavailable ones, ranking as tiebreak (hacker mode)', () => {
    // Ranking prefers gogoanime, but it is disabled → active rows must float above it.
    const rows = [a('gogoanime', 'disabled'), a('allanime', 'active'), a('miruro', 'active')]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, rankedIds: ['gogoanime', 'allanime', 'miruro'], provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['allanime', 'miruro', 'gogoanime'])
  })
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts -t "float"`
Expected: FAIL — current `sortedRows` orders purely by `rankedIds`, so it returns `['gogoanime','allanime','miruro']` (disabled gogoanime first).

- [ ] **Step 3: Implement the two-key sort**

In `SourcePanel.vue`, extend the type import (line 177) to include `ChipState`:

```ts
import type { AudioKind, TrackLang, ProviderRow, ChipState } from '@/types/aePlayer'
```

Replace the `sortedRows` computed (~line 231-238) with:

```ts
// Availability bucket → available sources float to the top of the full list,
// capability ranking is the tiebreak within each bucket. 'degraded' is
// selectable-but-not-auto (AUTO-484) so it ranks below 'active' but above the
// truly-unavailable states. Array.prototype.sort is stable, so equal keys keep
// input order.
const STATE_RANK: Record<ChipState, number> = {
  active: 0,
  degraded: 1,
  wip: 2,
  down: 3,
  disabled: 4,
  irrelevant: 5,
}
const sortedRows = computed(() => {
  const pos = (id: string) => {
    const i = props.rankedIds.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  return [...props.rows].sort(
    (a, b) =>
      STATE_RANK[a.state] - STATE_RANK[b.state] ||
      pos(a.def.id) - pos(b.def.id),
  )
})
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts`
Expected: PASS — new sort test green; all existing SourcePanel tests still pass (the collapse tests assert `active`-row behavior, which `STATE_RANK.active = 0` preserves; the existing `downTop` test still surfaces its active row).

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-hanime-restore
git commit frontend/web/src/components/player/aePlayer/SourcePanel.vue \
           frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts \
  -m "feat(aeplayer): sort Source menu available-first (ranking as tiebreak)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
git push origin HEAD:main
```

---

### Task 4: Verification & deploy

**Files:** none (verification + deploy only).

**Interfaces:** consumes the deployed catalog + web from Tasks 1-3.

- [ ] **Step 1: Run all build gates from the worktree**

```bash
cd /tmp/ae-hanime-restore/services/catalog && go test ./internal/service/scraperprovider/... -count=1 && go build ./...
cd /tmp/ae-hanime-restore/frontend/web && bunx vitest run src/composables/aePlayer/useProviderResolver.spec.ts src/components/player/aePlayer/SourcePanel.spec.ts && bunx vue-tsc --noEmit
bash /tmp/ae-hanime-restore/frontend/web/scripts/design-system-lint.sh
bash /tmp/ae-hanime-restore/frontend/web/scripts/i18n-lint.sh
```
Expected: all PASS (i18n/DS unchanged — no new keys/colors).

> Note: `vue-tsc` and the lint scripts need `node_modules` — if the worktree lacks them, symlink: `ln -s /data/animeenigma/frontend/web/node_modules /tmp/ae-hanime-restore/frontend/web/node_modules` (per [[feedback_deploy_from_clean_worktree]]).

- [ ] **Step 2: Deploy via the after-update skill**

Invoke `/animeenigma-after-update`. It lints, redeploys the changed services (**catalog** — runs `ReEnableHanime` live; **web** — ships the aePlayer adapter + sort), health-checks, writes the Trump-mode changelog entry (UXΔ/CDI/MVQ, no time units), and commits/pushes. Build from this clean worktree (copy `docker/.env`; compose project stays `docker`).

- [ ] **Step 3: Backend live verification (owner choice: verify live playback)**

After catalog redeploys, confirm the roster row flipped and the endpoints return real data for a known hentai title:

```bash
# Roster row now enabled:
DBU=$(grep -E '^DB_USER=' /data/animeenigma/docker/.env | cut -d= -f2)
DBN=$(grep -E '^DB_NAME=' /data/animeenigma/docker/.env | cut -d= -f2)
docker exec animeenigma-postgres psql -U "$DBU" -d "$DBN" -tAc \
  "select name,status from stream_providers where name='hanime';"
# Expect: hanime|enabled

# Episodes + stream for a hentai title (replace <UUID> with a known hentai anime UUID):
curl -s "http://localhost:8000/api/anime/<UUID>/hanime/episodes" | head -c 400
curl -s "http://localhost:8000/api/anime/<UUID>/hanime/stream?slug=<SLUG>" | head -c 400
```
Expected: episodes JSON `{success:true,data:[{name,slug}...]}` and a stream JSON with a non-empty `sources[].url`. If `sources` is empty / auth error, hanime upstream creds/availability are the blocker — report before claiming done (do NOT silently ship a dead source).

- [ ] **Step 4: Frontend playback smoke (offer Chrome checkup)**

Per the opt-in policy, offer the owner a Chrome smoke: open a hentai title in aePlayer, confirm the **Hanime** source appears in the Source menu, sorts among the available rows (above any unavailable ones), selects, and the video actually plays through the HLS proxy. If the stream 403s on the CDN, the fix is a hardcoded Referer in `makeHanimeAdapter` (`buildProxyUrl(best.url, 'https://hanime.tv', type)`) — note this as the one identified fallback.

- [ ] **Step 5: Report results**

Summarize: roster flipped (live DB), endpoints verified, playback confirmed (or blocker surfaced). Update memory with the outcome.

---

## Self-Review

**Spec coverage:**
- Goal 1 (re-enable roster, fresh + existing DBs) → Task 1 (seed flip + `ReEnableHanime` + boot wiring). ✓
- Goal 2 (slug-keyed adapter + dispatch) → Task 2 (Steps 3-5). ✓
- Goal 3 (un-gate registry, hentai-gated, ranked after 18anime) → Task 2 (Step 6; `CURATED_TIER` unchanged, `content:['hentai']` retained). ✓
- Goal 4 (available-on-top source sort) → Task 3. ✓
- Goal 5 (verify live playback) → Task 4 (Steps 3-4). ✓
- Non-goals (no `HanimePlayer.vue` restore, animelib stays disabled, no env/CDN/proxy work) → respected; no task touches them. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; `<UUID>`/`<SLUG>` in Task 4 Step 3 are runtime values the verifier substitutes (a known hentai title), not plan placeholders. ✓

**Type consistency:** `ReEnableHanime(db *gorm.DB) error` used identically in tests, impl, and boot wiring. `hanimeApi`, `makeHanimeAdapter`, `HanimeEpisode`/`HanimeSource`/`HanimeStream`, `ChipState`, `STATE_RANK` referenced consistently. Adapter returns `{key: slug, number: i+1}` matching the resolver tests' `eps[0].key === 'show-1'` / `eps[0].number === 1`. ✓
