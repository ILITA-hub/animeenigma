# Smart Source Selection — Stage 1a Implementation Plan (frontend-only)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the unified player default to the best *available* provider (never an empty first-party `ae`) and restore the Kodik dub-team picker — all in the frontend, no backend changes.

**Architecture:** A pure `pickSmartDefault` ranker selects the default provider from the live `ProviderRow[]` using a hand-ranked `CURATED_TIER`, skipping providers that fail an availability check (only `ae` today, via its existing `available` flag). A new optional `listTeams` adapter method surfaces Kodik translation teams into the `SourcePanel` Team chips, with `Combo.team` re-resolving the stream.

**Tech Stack:** Vue 3 (`<script setup>`), TypeScript, Vitest. Frontend dir: `frontend/web/` (use `bun`/`bunx`).

**Scope boundary:** This is Stage 1a of the spec `docs/superpowers/specs/2026-06-15-smart-source-selection-design.md`. It does NOT wire the watch-combo resolver, telemetry, or the daily ranking — those are Stage 1b and Stage 2 (separate plans). The smart default here still only fires when no provider is selected yet (`!state.combo.value.provider`), so it never overrides an in-session user choice.

---

## File Structure

- **Modify** `frontend/web/src/components/player/unified/providerRegistry.ts` — add exported `CURATED_TIER: string[]` (hand-ranked best-first provider ids).
- **Create** `frontend/web/src/composables/unifiedPlayer/smartDefault.ts` — pure `pickSmartDefault(...)` ranker.
- **Create** `frontend/web/src/composables/unifiedPlayer/smartDefault.spec.ts` — unit tests for the ranker.
- **Modify** `frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts` — add optional `listTeams` to `ProviderAdapter`, implement in `makeKodikAdapter`, expose `listTeams` on `ProviderResolver`.
- **Modify** `frontend/web/src/composables/unifiedPlayer/useProviderResolver.spec.ts` — tests for `listTeams`.
- **Modify** `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — replace the first-active default watcher with the smart default + `ae` availability probe; add a `teams` ref; bind `:teams`; add `team` to the re-resolve watcher.

---

### Task 1: Curated tier + pure smart-default ranker

**Files:**
- Modify: `frontend/web/src/components/player/unified/providerRegistry.ts`
- Create: `frontend/web/src/composables/unifiedPlayer/smartDefault.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/smartDefault.spec.ts`

- [ ] **Step 1: Add the curated tier to the registry**

Append to `providerRegistry.ts` (after `providerById`):

```ts
// Hand-ranked default-selection order, best-first. The smart default walks
// this list and picks the first provider whose row is `active` (and, for
// availability-gated providers like first-party `ae`, that actually has a
// local copy). Brand-exempt: order is reliability/quality judgement, not the
// registry array order. Tune as real telemetry lands (Stage 2).
export const CURATED_TIER: string[] = [
  'ae',         // first-party self-hosted — preferred WHEN the title is in the library
  'allanime',   // direct-MP4 / robust HLS
  'gogoanime',  // megaplay, ~78% popular coverage
  'miruro',
  'animepahe',
  'animefever',
  'nineanime',
  'kodik',      // RU
  'raw',        // JP
  '18anime',    // adult
  'animelib',
  'hanime',
]
```

- [ ] **Step 2: Write the failing test for the ranker**

Create `smartDefault.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { pickSmartDefault } from './smartDefault'
import type { ProviderRow } from '@/types/unifiedPlayer'

const row = (id: string, state: ProviderRow['state']): ProviderRow =>
  ({ def: { id, name: id, hue: '#000', group: 'en', audios: ['sub'], langs: ['en'], content: ['common'], scraper: true }, state })

const CURATED = ['ae', 'allanime', 'gogoanime', 'kodik']

describe('pickSmartDefault', () => {
  const alwaysAvailable = vi.fn(async () => true)

  it('picks the first active provider in curated order', async () => {
    const rows = [row('gogoanime', 'active'), row('allanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('allanime') // allanime precedes gogoanime in CURATED
  })

  it('skips non-active rows', async () => {
    const rows = [row('allanime', 'down'), row('gogoanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('gogoanime')
  })

  it('excludes an availability-gated provider when unavailable, picks next', async () => {
    const rows = [row('ae', 'active'), row('allanime', 'active')]
    const isAvailable = vi.fn(async () => false)
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(['ae']), isAvailable }))
      .toBe('allanime')
    expect(isAvailable).toHaveBeenCalledWith('ae')
  })

  it('includes an availability-gated provider when available', async () => {
    const rows = [row('ae', 'active'), row('allanime', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(['ae']), isAvailable: vi.fn(async () => true) }))
      .toBe('ae')
  })

  it('returns null when no rows are active', async () => {
    const rows = [row('ae', 'down'), row('allanime', 'irrelevant')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBeNull()
  })

  it('falls back to first active not in the curated list', async () => {
    const rows = [row('exotic', 'active')]
    expect(await pickSmartDefault(rows, CURATED, { needsCheck: new Set(), isAvailable: alwaysAvailable }))
      .toBe('exotic')
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/smartDefault.spec.ts`
Expected: FAIL — `Failed to resolve import './smartDefault'`.

- [ ] **Step 4: Implement the ranker**

Create `smartDefault.ts`:

```ts
import type { ProviderRow } from '@/types/unifiedPlayer'

export interface SmartDefaultOpts {
  /** Provider ids that must pass `isAvailable` before they can be picked (e.g. 'ae'). */
  needsCheck: Set<string>
  /** Async availability probe for gated providers. Resolves true ⇒ pickable. */
  isAvailable: (id: string) => Promise<boolean>
}

/**
 * Choose the default provider id from live rows.
 *
 * Walks `curated` order, considering only providers whose row state is
 * 'active'. For an id in `opts.needsCheck`, awaits `opts.isAvailable(id)` and
 * skips it when false (this is how an empty first-party `ae` auto-drops).
 * Providers active but absent from `curated` are tried last, in row order.
 * Returns null when nothing is selectable.
 */
export async function pickSmartDefault(
  rows: ProviderRow[],
  curated: string[],
  opts: SmartDefaultOpts,
): Promise<string | null> {
  const activeIds = new Set(rows.filter(r => r.state === 'active').map(r => r.def.id))

  const ordered = [
    ...curated.filter(id => activeIds.has(id)),
    ...rows.filter(r => r.state === 'active' && !curated.includes(r.def.id)).map(r => r.def.id),
  ]

  for (const id of ordered) {
    if (opts.needsCheck.has(id)) {
      if (await opts.isAvailable(id)) return id
      continue
    }
    return id
  }
  return null
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/smartDefault.spec.ts`
Expected: PASS (6 tests).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/unified/providerRegistry.ts \
        frontend/web/src/composables/unifiedPlayer/smartDefault.ts \
        frontend/web/src/composables/unifiedPlayer/smartDefault.spec.ts
git commit -m "feat(player): curated-tier smart default ranker (pure)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Kodik `listTeams` on the resolver

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useProviderResolver.spec.ts`

- [ ] **Step 1: Write the failing test**

Add to `useProviderResolver.spec.ts` (a new `describe` block; mirror the existing dep-injection style with `makeResolver`):

```ts
import { describe, it, expect } from 'vitest'
import { makeResolver } from './useProviderResolver'

describe('ProviderResolver.listTeams', () => {
  const kodikApi = {
    getTranslations: async () => ({ data: { data: [
      { id: 1, title: 'AniLibria', type: 'voice', episodes_count: 12 },
      { id: 2, title: 'AniDUB',    type: 'voice', episodes_count: 12 },
      { id: 3, title: 'AniLibria', type: 'voice', episodes_count: 8 }, // dup title
    ] } }),
    getStream: async () => ({ data: { data: {} } }),
  } as never

  it('returns unique Kodik translation titles as teams', async () => {
    const resolver = makeResolver({ kodikApi })
    expect(await resolver.listTeams('kodik', 'anime-1')).toEqual(['AniLibria', 'AniDUB'])
  })

  it('returns [] for providers without team support', async () => {
    const resolver = makeResolver({})
    expect(await resolver.listTeams('allanime', 'anime-1')).toEqual([])
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderResolver.spec.ts -t listTeams`
Expected: FAIL — `resolver.listTeams is not a function`.

- [ ] **Step 3: Add `listTeams` to the adapter interface**

In `useProviderResolver.ts`, extend the `ProviderAdapter` interface (after `resolveStream`):

```ts
  /**
   * Optional: provider-native selectable "teams" (e.g. Kodik translation
   * titles) for the Source panel Team chips. Adapters without sub-teams omit
   * this; the resolver returns [] for them.
   */
  listTeams?(animeId: string): Promise<string[]>
```

- [ ] **Step 4: Implement `listTeams` in the Kodik adapter**

In `makeKodikAdapter`, add a `listTeams` method (alongside `listEpisodes`/`resolveStream`):

```ts
    async listTeams(animeId: string): Promise<string[]> {
      const resp = await api.getTranslations(animeId)
      const translations: KodikTranslation[] = resp.data?.data ?? resp.data ?? []
      if (!Array.isArray(translations)) return []
      // Unique titles, preserving first-seen order.
      const seen = new Set<string>()
      const out: string[] = []
      for (const t of translations) {
        if (t.title && !seen.has(t.title)) { seen.add(t.title); out.push(t.title) }
      }
      return out
    },
```

- [ ] **Step 5: Expose `listTeams` on `ProviderResolver`**

Extend the `ProviderResolver` interface:

```ts
  listTeams(provider: string, animeId: string): Promise<string[]>
```

And in the object returned by `makeResolver` (after `resolveStream`). Note it must be resilient: `getAdapter` throws `NotAvailableError` for providers whose dep is missing or that are unwired — teams are best-effort, so swallow that and return `[]`:

```ts
    async listTeams(provider: string, animeId: string): Promise<string[]> {
      let adapter: ProviderAdapter
      try {
        adapter = getAdapter(provider)
      } catch {
        return [] // unwired / dep-missing provider has no teams
      }
      return adapter.listTeams ? adapter.listTeams(animeId) : []
    },
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/useProviderResolver.spec.ts -t listTeams`
Expected: PASS (2 tests). Then run the whole file to confirm no regressions: `bunx vitest run src/composables/unifiedPlayer/useProviderResolver.spec.ts`.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts \
        frontend/web/src/composables/unifiedPlayer/useProviderResolver.spec.ts
git commit -m "feat(player): expose Kodik translation teams via resolver.listTeams

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Wire the smart default + `ae` availability into UnifiedPlayer

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue:383-394` (the default watcher) and imports near `:218` (`import { providerById } ...`).

- [ ] **Step 1: Import the ranker, curated tier, and aeApi**

In the `<script setup>` import block, update the registry import and add the resolver/api imports:

```ts
import { providerById, CURATED_TIER } from './providerRegistry'
import { pickSmartDefault } from '@/composables/unifiedPlayer/smartDefault'
import { aeApi } from '@/api/client'
```

(`providerById` is already imported at `:331` — replace that line with the combined import; keep a single import statement.)

- [ ] **Step 2: Add a cached `ae` availability probe**

Immediately above the existing default watcher (currently `:383`), add:

```ts
// First-party (ae) availability — cached single probe per mount. The library
// only has a subset of titles encoded, so `ae` (top of CURATED_TIER) must be
// skipped when this anime isn't on-prem. aeApi.getEpisodes returns
// { episodes, available }; treat available=false OR an empty list as "no".
let aeAvailableCache: Promise<boolean> | null = null
function isProviderAvailable(id: string): Promise<boolean> {
  if (id !== 'ae') return Promise.resolve(true)
  if (!aeAvailableCache) {
    aeAvailableCache = aeApi
      .getEpisodes(props.animeId)
      .then((resp) => {
        const data = resp.data?.data ?? resp.data
        return Boolean(data?.available) && (data?.episodes?.length ?? 0) > 0
      })
      .catch(() => false)
  }
  return aeAvailableCache
}
```

- [ ] **Step 3: Replace the first-active default watcher with the smart default**

Replace the existing watcher block (`:383-394`):

```ts
watch(
  rows,
  () => {
    if (!state.combo.value.provider) {
      const first = rows.value.find((r) => r.state === 'active')
      if (first) {
        state.setProvider(first.def.id, '')
      }
    }
  },
  { immediate: true },
)
```

with:

```ts
watch(
  rows,
  () => {
    if (state.combo.value.provider) return
    const snapshot = rows.value
    void pickSmartDefault(snapshot, CURATED_TIER, {
      needsCheck: new Set(['ae']),
      isAvailable: isProviderAvailable,
    }).then((id) => {
      // Guard against a race: only apply if still unset and the chosen row is
      // still active in the latest rows (filter may have changed mid-probe).
      if (id && !state.combo.value.provider &&
          rows.value.some((r) => r.def.id === id && r.state === 'active')) {
        state.setProvider(id, '')
      }
    })
  },
  { immediate: true },
)
```

- [ ] **Step 4: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: PASS (no new errors).

- [ ] **Step 5: Run the existing UnifiedPlayer test suite**

Run: `cd frontend/web && bunx vitest run src/components/player/unified/`
Expected: PASS (no regressions; if a test asserted the old "first active" default, update it to await the smart default — the picker is async now).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(player): smart default selection + skip empty first-party ae

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Surface Kodik teams in the Source panel

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — add `teams` ref, populate it on resolve, bind `:teams`, add `team` to the re-resolve watcher.

- [ ] **Step 1: Add a `teams` ref**

Near the other player refs (e.g. beside `resolvedServers`), add:

```ts
const teams = ref<string[]>([])
```

- [ ] **Step 2: Populate teams inside `loadEpisodesAndStream`**

In `loadEpisodesAndStream` (`:689`), after `episodes.value = eps` (`:703`), add a best-effort team fetch (teams are provider-native; failure must not break playback):

```ts
    // Provider-native teams (e.g. Kodik translation titles) for the Source
    // panel. Best-effort — never blocks the stream resolve.
    resolver
      .listTeams(provider, props.animeId)
      .then((t) => { if (token === resolveToken) teams.value = t })
      .catch(() => { if (token === resolveToken) teams.value = [] })
```

- [ ] **Step 3: Bind `:teams` in the SourcePanel template**

Replace `:teams="[]"` (`:217`) with:

```vue
        :teams="teams"
```

- [ ] **Step 4: Re-resolve when the team changes**

The team-select emit already calls `state.setTeam` (`:220`), but `team` is not in the re-resolve watcher, so picking a team currently does nothing. Add `state.combo.value.team` to the watcher source array (`:766-772`):

```ts
  () => [
    state.combo.value.audio,
    state.combo.value.lang,
    state.combo.value.server,
    state.combo.value.team,
    selectedEpisode.value,
  ] as const,
```

**Also fix the episode-list guard index in the watcher body.** `selectedEpisode` moved from index 3 to index 4, so the guard that skips the mount-time placeholder transition must follow it. Change:

```ts
    if (newVal[3] !== oldVal[3] && episodes.value.length === 0) return
```

to:

```ts
    if (newVal[4] !== oldVal[4] && episodes.value.length === 0) return
```

No other body change is needed — any tracked change (including the new `team` at index 3) triggers `resolveStreamForCurrentEpisode`, and the Kodik adapter already reads `combo.team` in `resolveStream`.

- [ ] **Step 5: Type-check + run player tests**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/`
Expected: PASS.

- [ ] **Step 6: Manual sanity (optional, owner-gated)**

Per the project's opt-in Chrome-smoke rule, do NOT auto-run a browser check. If the owner wants one: open a Kodik-backed RU title, open the Source panel, confirm the **Team** chips list the dub teams and selecting one re-resolves the stream.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(player): restore Kodik dub-team picker in unified Source panel

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Final verification

- [ ] Run the full frontend unit suite for touched areas:
  `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/ src/components/player/unified/`
- [ ] `cd frontend/web && bunx tsc --noEmit`
- [ ] `cd frontend/web && bunx eslint src/composables/unifiedPlayer/smartDefault.ts src/components/player/unified/UnifiedPlayer.vue`
- [ ] Invoke `/animeenigma-after-update` to lint/build, redeploy web, update the changelog (Russian Trump-mode), and commit+push.

## Spec coverage (this plan)

| Spec section | Covered by |
|---|---|
| §6 Seamless first-party availability (no empty-`ae`) | Task 3 (`isProviderAvailable` + smart default) |
| §5 step 3 "first-party on top, else auto-skip" | Task 1 (`CURATED_TIER`) + Task 3 |
| §8 Granular options (Kodik dub team) | Tasks 2 & 4 |

## Deferred to later plans (NOT in Stage 1a)

- **Stage 1b:** wire unified player into `/preferences/resolve` (saved-combo respect + "last source unavailable" toast + additive `WatchCombo.player` enum), silent resolve-time fallback to next ranked provider.
- **Stage 2:** `player-events` telemetry (resolve + stall), ClickHouse panels, daily ranking job, Redis same-day override, ranking-aware default, playback-time fallback suggestion.
