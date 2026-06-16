# Player Capabilities Source-Menu — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enrich the live player's Source menu with `GET /api/anime/{uuid}/capabilities` data — declutter the provider list to the selected (best) source with hacker-mode reveal + a playback-error escape hatch, and decorate provider/team chips with sub/dub/quality/team labels.

**Architecture:** A pure **labels + collapse** enrichment layer over the existing `useProviderHealth` rows. A `useCapabilities` composable fetches the report and yields a `Map<providerId, ProviderCap>`. Pure helpers (`flattenCapabilities`, `deriveCapLabels`) do the logic, unit-tested in isolation. `SourcePanel` collapses the chip list and decorates each chip from `capMap`; `UnifiedPlayer` wires the composable + three props. If `/capabilities` is absent the map is empty and every chip renders exactly as today.

**RECONCILIATION (2026-06-16) — ordering is NOT ours.** A concurrent "Smart Source Selection Stage 2" effort already owns provider ORDERING on origin/main: it fetches `/api/anime/{id}/source-ranking` (telemetry: global + per-anime reliability + same-day srcfix), flattens it via `rankingToOrder()`, passes `[...rankingOrder, ...CURATED_TIER]` to `pickSmartDefault`, and auto-switches providers on playback failure (`fallbackToNextSource`). Telemetry is a better ordering signal than the capability `rank` field, so **this plan does NOT touch ordering**: no `rankedProviderIds`, no `pickSmartDefault` edits, no `orderedProviderIds`. The collapse's "top-1" is simply the already-**selected** provider (the one Stage-2's smart default chose), so we inherit Stage-2's ordering for free. Our footprint in the actively-churned `UnifiedPlayer.vue` shrinks to one composable call + three `SourcePanel` props — orthogonal to the ranking code.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest (`@vue/test-utils`), vue-i18n. Frontend uses `bun`/`bunx`. DS lint exemption applies to `components/player/`.

**Spec:** `docs/superpowers/specs/2026-06-16-player-capabilities-source-menu-design.md` (the spec's §4.3 ranking and the ranking half of §5.1 are superseded by the reconciliation above; labels §5.2/§5.3 and the collapse/try-another UX of §5.1 stand).

**Convention:** every commit includes the co-author trailer (Claude Opus 4.6 / 0neymik0 / NANDIorg). **Commit with an explicit pathspec — `git commit <paths> -m …` — NEVER `git add` + bare `git commit`** (the shared index holds other agents' staged work; a bare commit captures it). Run `git show --stat HEAD` after every commit and confirm ONLY your files are present. The controller lands commits on origin/main (worktree cherry-pick).

**Base:** branch off the latest **origin/main** (it has Stage-2; the local tree may be mid-churn). Find edit anchors by CONTENT (quoted strings below), not line numbers — they will have drifted.

---

## File Structure

**Create:**
- `frontend/web/src/types/capabilities.ts` — TS mirror of the Go `CapabilityReport`.
- `frontend/web/src/composables/unifiedPlayer/useCapabilities.ts` — fetch + `flattenCapabilities` pure helper + composable.
- `frontend/web/src/composables/unifiedPlayer/useCapabilities.spec.ts`
- `frontend/web/src/composables/unifiedPlayer/capLabels.ts` — pure label derivation.
- `frontend/web/src/composables/unifiedPlayer/capLabels.spec.ts`
- `frontend/web/src/components/player/unified/ProviderChip.spec.ts`
- `frontend/web/src/components/player/unified/SourcePanel.spec.ts`

**Modify:**
- `frontend/web/src/api/client.ts` — add `capabilitiesApi`.
- `frontend/web/src/components/player/unified/ProviderChip.vue` — optional `cap`/`best` props + label row.
- `frontend/web/src/components/player/unified/SourcePanel.vue` — collapse, labels, team tags, try-another disclosure.
- `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — wire `useCapabilities`, pass three props to `SourcePanel`. NO ordering edits.
- `frontend/web/src/locales/{en,ru,ja}.json` — `player.sources.*` keys.

**Working dir for all commands:** `cd /data/animeenigma/frontend/web`

---

## Task 1: Capability types + API client

**Files:**
- Create: `frontend/web/src/types/capabilities.ts`
- Modify: `frontend/web/src/api/client.ts` (add `capabilitiesApi` right after the `scraperApi` object closes)

- [ ] **Step 1: Create the types file**

`frontend/web/src/types/capabilities.ts`:
```ts
// TS mirror of the Go domain.CapabilityReport (services/catalog/internal/domain/capability.go).
// Snake_case keys match the JSON wire shape exactly.

export interface CapabilityReport {
  anime_id: string
  families: SourceFamily[]
}

export interface SourceFamily {
  family: string // 'ourenglish' | 'kodik' | 'animelib' | 'hanime'
  providers: ProviderCap[]
}

export interface ProviderCap {
  provider: string
  display_name: string
  enabled: boolean
  health: 'up' | 'down' | 'unknown'
  playable?: boolean
  rank: number
  variants: CapVariant[]
}

export interface CapVariant {
  category: 'sub' | 'dub' | 'raw'
  team?: { id?: string; name: string }
  sub_delivery: 'soft' | 'hard' | 'none'
  qualities?: string[]
  quality_source: 'hls_master' | 'discrete' | 'unknown' | 'trait'
  source: 'trait' | 'discovered'
}
```

- [ ] **Step 2: Add the API client method**

In `frontend/web/src/api/client.ts`, find the `scraperApi` object (it ends with `getHealth: () => apiClient.get(\`/anime/_/scraper/health\`),` then `}`). Immediately after its closing `}`, add:
```ts
/**
 * Assembled, ranked per-anime capability report (catalog P4). Families:
 * ourenglish (EN scrapers), kodik, animelib, hanime. The catalog wraps the
 * payload in the {success,data} envelope — callers read res.data.data.
 */
export const capabilitiesApi = {
  get: (animeId: string) => apiClient.get(`/anime/${animeId}/capabilities`),
}
```

- [ ] **Step 3: Type-check**

Run: `bunx tsc --noEmit`
Expected: PASS. The new export is unused so far — fine.

- [ ] **Step 4: Commit**
```bash
git commit src/types/capabilities.ts src/api/client.ts -m "feat(player): capability report types + capabilitiesApi client

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then: `git show --stat HEAD` — confirm ONLY these two files are in the commit.

---

## Task 2: `flattenCapabilities` + `useCapabilities`

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/useCapabilities.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useCapabilities.spec.ts`

The pure `flattenCapabilities(report)` returns the provider→cap map (labels only — NO ranking). The composable is a thin fetch wrapper.

- [ ] **Step 1: Write the failing test**

`frontend/web/src/composables/unifiedPlayer/useCapabilities.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { flattenCapabilities } from './useCapabilities'
import type { CapabilityReport } from '@/types/capabilities'

const report: CapabilityReport = {
  anime_id: 'uuid-1',
  families: [
    {
      family: 'ourenglish',
      providers: [
        { provider: 'gogoanime', display_name: 'Gogoanime', enabled: true, health: 'up', playable: true, rank: 120, variants: [] },
        { provider: 'allanime', display_name: 'AllAnime', enabled: true, health: 'up', rank: 75, variants: [] },
      ],
    },
    {
      family: 'kodik',
      providers: [
        { provider: 'kodik', display_name: 'Kodik', enabled: true, health: 'unknown', rank: 0, variants: [] },
      ],
    },
  ],
}

describe('flattenCapabilities', () => {
  it('flattens every family into a provider map', () => {
    const capMap = flattenCapabilities(report)
    expect(capMap.size).toBe(3)
    expect(capMap.get('gogoanime')?.rank).toBe(120)
    expect(capMap.get('kodik')?.display_name).toBe('Kodik')
  })

  it('degrades to an empty map on null/malformed report', () => {
    expect(flattenCapabilities(null).size).toBe(0)
    expect(flattenCapabilities({ anime_id: 'x' } as unknown as CapabilityReport).size).toBe(0)
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/composables/unifiedPlayer/useCapabilities.spec.ts`
Expected: FAIL — `flattenCapabilities` not exported / module missing.

- [ ] **Step 3: Create the composable**

`frontend/web/src/composables/unifiedPlayer/useCapabilities.ts`:
```ts
import { ref, watch, type Ref } from 'vue'
import { capabilitiesApi } from '@/api/client'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

/**
 * Flatten a capability report into a providerId→ProviderCap map (for chip
 * labels). Pure + defensive: a null/malformed report yields an empty map.
 * Ordering is intentionally NOT derived here — Stage-2 owns provider order.
 */
export function flattenCapabilities(report: CapabilityReport | null): Map<string, ProviderCap> {
  const capMap = new Map<string, ProviderCap>()
  if (!report || !Array.isArray(report.families)) return capMap
  for (const fam of report.families) {
    for (const p of fam.providers ?? []) capMap.set(p.provider, p)
  }
  return capMap
}

/**
 * Fetch the capability report for an anime id (re-fetches when the id changes).
 * Decoration only — every failure degrades to an empty map, never throws.
 */
export function useCapabilities(animeId: Ref<string>) {
  const capMap = ref<Map<string, ProviderCap>>(new Map())
  const loaded = ref(false)
  const error = ref(false)

  let lastId = ''
  async function load(id: string) {
    if (!id || id === lastId) return
    lastId = id
    loaded.value = false
    error.value = false
    try {
      const res = await capabilitiesApi.get(id)
      // catalog {success,data} envelope
      const report = (res.data?.data ?? res.data ?? null) as CapabilityReport | null
      capMap.value = flattenCapabilities(report)
    } catch {
      capMap.value = new Map()
      error.value = true
    } finally {
      loaded.value = true
    }
  }

  watch(animeId, (id) => { void load(id) }, { immediate: true })

  return { capMap, loaded, error }
}
```

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/composables/unifiedPlayer/useCapabilities.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**
```bash
git commit src/composables/unifiedPlayer/useCapabilities.ts src/composables/unifiedPlayer/useCapabilities.spec.ts -m "feat(player): useCapabilities composable (capMap for chip labels)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY these two files.

---

## Task 3: `deriveCapLabels` (chip label derivation)

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/capLabels.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/capLabels.spec.ts`

- [ ] **Step 1: Write the failing test**

`frontend/web/src/composables/unifiedPlayer/capLabels.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { deriveCapLabels } from './capLabels'
import type { ProviderCap } from '@/types/capabilities'

function cap(variants: ProviderCap['variants']): ProviderCap {
  return { provider: 'x', display_name: 'X', enabled: true, health: 'up', rank: 1, variants }
}

describe('deriveCapLabels', () => {
  it('collects categories in sub,dub,raw order and the max quality', () => {
    const l = deriveCapLabels(cap([
      { category: 'dub', sub_delivery: 'none', qualities: ['720p'], quality_source: 'trait', source: 'trait' },
      { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    ]))
    expect(l?.categories).toEqual(['sub', 'dub'])
    expect(l?.quality).toBe('1080p')
    expect(l?.subDelivery).toBe('hard')
  })

  it('reports soft sub delivery and omits unknown quality', () => {
    const l = deriveCapLabels(cap([
      { category: 'sub', sub_delivery: 'soft', quality_source: 'unknown', source: 'discovered' },
    ]))
    expect(l?.subDelivery).toBe('soft')
    expect(l?.quality).toBeNull()
  })

  it('returns null for missing/empty variants', () => {
    expect(deriveCapLabels(undefined)).toBeNull()
    expect(deriveCapLabels(cap([]))).toBeNull()
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/composables/unifiedPlayer/capLabels.spec.ts`
Expected: FAIL — module missing.

- [ ] **Step 3: Create the file**

`frontend/web/src/composables/unifiedPlayer/capLabels.ts`:
```ts
import type { ProviderCap } from '@/types/capabilities'

export interface CapLabels {
  categories: ('sub' | 'dub' | 'raw')[]
  quality: string | null // max quality ceiling across variants, e.g. '1080p'
  subDelivery: 'soft' | 'hard' | null // delivery of the SUB variant, if any
}

// Highest → lowest. Index 0 is the best resolution.
const Q_ORDER = ['2160p', '1440p', '1080p', '720p', '480p', '360p']

/**
 * Derive the compact chip labels from a provider's capability variants:
 * the category set (SUB/DUB/RAW), the best quality ceiling, and the SUB
 * variant's subtitle delivery (soft=selectable / hard=burned-in). Returns null
 * when there's nothing to show.
 */
export function deriveCapLabels(cap: ProviderCap | undefined): CapLabels | null {
  if (!cap || !Array.isArray(cap.variants) || cap.variants.length === 0) return null

  const categories: ('sub' | 'dub' | 'raw')[] = []
  for (const c of ['sub', 'dub', 'raw'] as const) {
    if (cap.variants.some((v) => v.category === c)) categories.push(c)
  }

  let quality: string | null = null
  let bestIdx = Number.MAX_SAFE_INTEGER
  for (const v of cap.variants) {
    for (const q of v.qualities ?? []) {
      const idx = Q_ORDER.indexOf(q)
      if (idx !== -1 && idx < bestIdx) {
        bestIdx = idx
        quality = q
      }
    }
  }

  const sub = cap.variants.find((v) => v.category === 'sub')
  const subDelivery =
    sub && (sub.sub_delivery === 'soft' || sub.sub_delivery === 'hard') ? sub.sub_delivery : null

  return { categories, quality, subDelivery }
}
```

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/composables/unifiedPlayer/capLabels.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**
```bash
git commit src/composables/unifiedPlayer/capLabels.ts src/composables/unifiedPlayer/capLabels.spec.ts -m "feat(player): deriveCapLabels for provider chip labels

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY these two files.

---

## Task 4: `ProviderChip.vue` — capability label row

**Files:**
- Modify: `frontend/web/src/components/player/unified/ProviderChip.vue`
- Test: `frontend/web/src/components/player/unified/ProviderChip.spec.ts`

Add optional `cap` + `best` props. When `cap` resolves labels, render a second line under the name: category tags (`data-test="cap-cat"`), a quality tag (`data-test="cap-quality"`), and a "best" pill (`data-test="cap-best"`). i18n via the global `$t` (stubbed in tests).

- [ ] **Step 1: Write the failing test**

`frontend/web/src/components/player/unified/ProviderChip.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderChip from './ProviderChip.vue'
import type { ProviderRow, ProviderDef } from '@/types/unifiedPlayer'
import type { ProviderCap } from '@/types/capabilities'

const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
function row(id = 'gogoanime'): ProviderRow {
  return { def: { id, name: 'Gogoanime', hue: '#00d4ff' } as ProviderDef, state: 'active' }
}
const cap: ProviderCap = {
  provider: 'gogoanime', display_name: 'Gogoanime', enabled: true, health: 'up', rank: 120,
  variants: [
    { category: 'sub', sub_delivery: 'hard', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
    { category: 'dub', sub_delivery: 'none', qualities: ['1080p'], quality_source: 'trait', source: 'trait' },
  ],
}

describe('ProviderChip', () => {
  it('renders category + quality tags when cap is present', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap }, ...mountOpts })
    expect(w.findAll('[data-test="cap-cat"]').length).toBe(2)
    expect(w.find('[data-test="cap-quality"]').text()).toContain('1080p')
  })

  it('renders no label row without cap', () => {
    const w = mount(ProviderChip, { props: { row: row() }, ...mountOpts })
    expect(w.find('[data-test="cap-cat"]').exists()).toBe(false)
  })

  it('shows the best pill when best=true', () => {
    const w = mount(ProviderChip, { props: { row: row(), cap, best: true }, ...mountOpts })
    expect(w.find('[data-test="cap-best"]').exists()).toBe(true)
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/components/player/unified/ProviderChip.spec.ts`
Expected: FAIL — no `cap-cat`/`cap-best` elements.

- [ ] **Step 3: Edit the component**

In `frontend/web/src/components/player/unified/ProviderChip.vue`, replace the single name span (currently `<span class="flex-1 font-semibold truncate">{{ row.def.name }}</span>`) with this name+labels block:
```vue
      <!-- Provider name + capability labels -->
      <span class="flex-1 min-w-0 flex flex-col gap-[2px]">
        <span class="font-semibold truncate">{{ row.def.name }}</span>
        <span v-if="labels" class="flex items-center gap-[5px] flex-wrap">
          <span
            v-for="c in labels.categories"
            :key="c"
            data-test="cap-cat"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide px-[4px] py-px rounded bg-white/[0.08] text-[var(--muted-foreground)]"
          >
            {{ c === 'sub' ? $t('player.sub') : c === 'dub' ? $t('player.dub') : $t('player.sources.raw') }}<template v-if="c === 'sub' && labels.subDelivery"> · {{ labels.subDelivery === 'hard' ? $t('player.sources.subBurnedIn') : $t('player.sources.subSelectable') }}</template>
          </span>
          <span
            v-if="labels.quality"
            data-test="cap-quality"
            class="text-[9px] font-semibold font-mono text-[var(--muted-foreground)]"
          >{{ labels.quality }}</span>
          <span
            v-if="best"
            data-test="cap-best"
            class="text-[9px] font-semibold font-mono uppercase tracking-wide text-[var(--brand-cyan)]"
          >{{ $t('player.sources.best') }}</span>
        </span>
      </span>
```

Then update the `<script setup>` block — replace the imports + `defineProps` + the `isSelected` computed with:
```ts
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'
import type { ProviderRow } from '@/types/unifiedPlayer'
import type { ProviderCap } from '@/types/capabilities'
import { deriveCapLabels } from '@/composables/unifiedPlayer/capLabels'

const props = defineProps<{
  row: ProviderRow
  selected?: boolean
  cap?: ProviderCap
  best?: boolean
}>()

const emit = defineEmits<{
  (e: 'select'): void
}>()

const isSelected = computed(() => props.selected === true)
const labels = computed(() => deriveCapLabels(props.cap))

function onClick() {
  if (props.row.state === 'active') {
    emit('select')
  }
}
```
> The button is `inline-flex items-center`; the name span changing to `flex-col` stays vertically centred. The wip/down badges + selected check remain after this span, unchanged.

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/components/player/unified/ProviderChip.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**
```bash
git commit src/components/player/unified/ProviderChip.vue src/components/player/unified/ProviderChip.spec.ts -m "feat(player): ProviderChip capability label row (sub/dub/quality/best)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY these two files.

---

## Task 5: `SourcePanel.vue` — collapse, labels, team tags, try-another

**Files:**
- Modify: `frontend/web/src/components/player/unified/SourcePanel.vue`
- Test: `frontend/web/src/components/player/unified/SourcePanel.spec.ts`

Add three props (`capMap`, `hackerMode`, `playbackError`). Collapse the provider list to the **selected** provider (Stage-2 already picked the best into `provider`), fall back to first-active when none selected. Hacker mode → full list; playback error → a `Try another source ▾` disclosure expands the active rows. Decorate chips from `capMap`; tag team chips.

- [ ] **Step 1: Write the failing test**

`frontend/web/src/components/player/unified/SourcePanel.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SourcePanel from './SourcePanel.vue'
import type { ProviderRow, ProviderDef } from '@/types/unifiedPlayer'
import type { ProviderCap } from '@/types/capabilities'

const mountOpts = { global: { mocks: { $t: (k: string) => k } } }
function row(id: string, state: ProviderRow['state'] = 'active'): ProviderRow {
  return { def: { id, name: id, hue: '#00d4ff' } as ProviderDef, state }
}
const base = {
  audio: 'sub' as const, lang: 'en' as const, team: null, server: '',
  servers: [] as { id: string; label: string }[], teams: [] as string[],
  capMap: new Map<string, ProviderCap>(),
}
const rows = [row('gogoanime'), row('allanime'), row('miruro')]

describe('SourcePanel collapse', () => {
  it('default shows only the selected provider', () => {
    const w = mount(SourcePanel, { props: { ...base, rows, provider: 'allanime', hackerMode: false, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('allanime')
  })

  it('falls back to the first active provider when none selected', () => {
    const w = mount(SourcePanel, { props: { ...base, rows, provider: '', hackerMode: false, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('gogoanime')
  })

  it('hacker mode shows the full list', () => {
    const w = mount(SourcePanel, { props: { ...base, rows, provider: 'gogoanime', hackerMode: true, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })

  it('shows try-another only on playback error, and expands on click', async () => {
    const w = mount(SourcePanel, { props: { ...base, rows, provider: 'gogoanime', hackerMode: false, playbackError: true }, ...mountOpts })
    const btn = w.find('[data-test="try-another"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/components/player/unified/SourcePanel.spec.ts`
Expected: FAIL — panel renders all rows / unknown props.

- [ ] **Step 3: Edit the template — provider list**

In `frontend/web/src/components/player/unified/SourcePanel.vue`, replace the entire "Provider list" block (the `<div class="flex flex-col gap-1">` containing the "Provider" header span and the `ProviderChip` `v-for`) with:
```vue
    <!-- Provider list (collapsed to the selected source unless hacker mode / error-expanded) -->
    <div class="flex flex-col gap-1">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-[2px]">
        Provider
      </span>
      <div class="flex flex-col gap-1">
        <ProviderChip
          v-for="r in visibleRows"
          :key="r.def.id"
          :row="r"
          :cap="capMap.get(r.def.id)"
          :best="!hackerMode && !expanded && r.def.id === topRow?.def.id"
          :selected="r.def.id === provider"
          @select="emit('select-provider', r.def.id)"
        />
      </div>
      <!-- Error escape hatch: reveal the rest without full hacker mode -->
      <button
        v-if="playbackError && !hackerMode && !expanded && hiddenCount > 0"
        data-test="try-another"
        class="mt-1 self-start text-[12px] font-semibold text-[var(--brand-cyan)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)] rounded px-1"
        @click="expanded = true"
      >
        {{ $t('player.sources.tryAnother') }} ({{ hiddenCount }})
      </button>
    </div>
```

- [ ] **Step 4: Edit the template — team chip tag**

In the team-chips `<button v-for="t in teams">`, replace the chip's inner `{{ t }}` text with a name + optional category tag:
```vue
          <span>{{ t }}</span>
          <span
            v-if="teamCategory(t)"
            class="ml-[5px] text-[9px] font-semibold font-mono uppercase tracking-wide opacity-80"
          >{{ teamCategory(t) === 'dub' ? $t('player.dub') : $t('player.sub') }}</span>
```

- [ ] **Step 5: Edit the `<script setup>` block**

Replace the imports + `defineProps` + `computed` section with:
```ts
import { computed, ref, watch } from 'vue'
import type { AudioKind, TrackLang, ProviderRow } from '@/types/unifiedPlayer'
import type { ProviderCap } from '@/types/capabilities'
import ProviderChip from './ProviderChip.vue'

const props = defineProps<{
  rows: ProviderRow[]
  audio: AudioKind
  lang: TrackLang
  team: string | null
  provider: string
  server: string
  servers: { id: string; label: string }[]
  teams: string[]
  capMap: Map<string, ProviderCap>
  hackerMode: boolean
  playbackError: boolean
}>()

const emit = defineEmits<{
  (e: 'update:audio', v: AudioKind): void
  (e: 'update:lang', v: TrackLang): void
  (e: 'update:team', v: string | null): void
  (e: 'select-provider', id: string): void
  (e: 'select-server', id: string): void
}>()

const audioOptions: { value: AudioKind; label: string }[] = [
  { value: 'sub', label: 'SUB' },
  { value: 'dub', label: 'DUB' },
]

const langOptions: { value: TrackLang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ru', label: 'Русский' },
  { value: 'ja', label: '日本語' },
]

const audioIndex = computed(() => audioOptions.findIndex((o) => o.value === props.audio))
const langIndex = computed(() => langOptions.findIndex((o) => o.value === props.lang))

const activeRows = computed(() => props.rows.filter((r) => r.state === 'active'))
const activeCount = computed(() => activeRows.value.length)

// Collapse: by default show only the selected provider (Stage-2's smart default
// already picked the best into `provider`); fall back to the first active row
// while nothing is selected. Hacker mode → full list; error-expanded → all active.
const topRow = computed(
  () =>
    activeRows.value.find((r) => r.def.id === props.provider) ?? activeRows.value[0] ?? null,
)
const expanded = ref(false)
watch(() => props.provider, () => { expanded.value = false })

const visibleRows = computed(() => {
  if (props.hackerMode) return props.rows
  if (expanded.value) return activeRows.value
  return topRow.value ? [topRow.value] : []
})
const hiddenCount = computed(() => Math.max(0, activeCount.value - 1))

// Team → category tag from the selected provider's capability variants.
function teamCategory(name: string): 'sub' | 'dub' | null {
  const cap = props.capMap.get(props.provider)
  if (!cap) return null
  const v = cap.variants.find((x) => x.team?.name === name)
  if (!v) return null
  return v.category === 'dub' ? 'dub' : v.category === 'sub' ? 'sub' : null
}
```
> The header still reads `{{ activeCount }} available` — `activeCount` now counts active rows (was `rows.filter(active).length` before — identical). The `team`/`server` props stay used in the template (team-chip selected styling + server list).

- [ ] **Step 6: Run, confirm pass**

Run: `bunx vitest run src/components/player/unified/SourcePanel.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**
```bash
git commit src/components/player/unified/SourcePanel.vue src/components/player/unified/SourcePanel.spec.ts -m "feat(player): SourcePanel collapse + cap labels + team tags + try-another

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY these two files.

---

## Task 6: `UnifiedPlayer.vue` — wire `useCapabilities` + pass props

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

Minimal footprint: construct the composable, pass three props to `SourcePanel`. **Do NOT touch** `pickSmartDefault`, `rankingOrder`, `fallbackToNextSource`, or any ordering code — that's Stage-2's.

- [ ] **Step 1: Add the import**

After the existing `useProviderHealth` import line (`import { useProviderHealth } from '@/composables/unifiedPlayer/useProviderHealth'`), add:
```ts
import { useCapabilities } from '@/composables/unifiedPlayer/useCapabilities'
```
Confirm `computed` is already imported from `vue` (it is). 

- [ ] **Step 2: Construct the composable**

Immediately after the `const { rows, start } = useProviderHealth(filter)` line, add:
```ts
// Capability report → per-provider sub/dub/quality/team labels for the Source
// panel. Decoration only (ordering stays with Stage-2); degrades to empty.
const animeIdRef = computed(() => props.animeId)
const { capMap } = useCapabilities(animeIdRef)
```

- [ ] **Step 3: Pass the new props to `SourcePanel`**

In the `<SourcePanel ... />` usage, add three bindings after the existing `:teams="teams"` line:
```vue
        :cap-map="capMap"
        :hacker-mode="state.hackerMode.value"
        :playback-error="Boolean(sourceError)"
```
> `sourceError` is the existing error ref (a string, set in the resolve catch). In the template it is auto-unwrapped, so `Boolean(sourceError)` is `true` whenever a message is set. Grep `sourceError` in the file to confirm the name; if the template references it as `sourceError` elsewhere (e.g. an error banner), reuse that exact identifier.

- [ ] **Step 4: Type-check**

Run: `bunx tsc --noEmit`
Expected: PASS. If vue-tsc flags `sourceError` unwrap, bind an explicit computed `const hasSourceError = computed(() => Boolean(sourceError.value))` and pass `:playback-error="hasSourceError"`.

- [ ] **Step 5: Commit**
```bash
git commit src/components/player/unified/UnifiedPlayer.vue -m "feat(player): wire UnifiedPlayer to useCapabilities (chip labels + collapse props)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY `UnifiedPlayer.vue` is in the commit (the file is actively edited by another agent — a wrong file count here is the disaster signal).

---

## Task 7: i18n keys (en / ru / ja)

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

All three MUST get the keys (i18n-lint hard-fails `make redeploy-web` on any missing key).

- [ ] **Step 1: Add the `player.sources` sub-namespace**

In each locale file, inside the existing `"player": { ... }` object (adjacent to the `"sub"`/`"dub"` keys), add a `"sources"` key:

`en.json`:
```json
    "sources": {
      "best": "Best",
      "tryAnother": "Try another source",
      "raw": "RAW",
      "subBurnedIn": "burned-in",
      "subSelectable": "selectable"
    }
```

`ru.json`:
```json
    "sources": {
      "best": "Лучший",
      "tryAnother": "Другой источник",
      "raw": "RAW",
      "subBurnedIn": "вшитые",
      "subSelectable": "отдельные"
    }
```

`ja.json`:
```json
    "sources": {
      "best": "ベスト",
      "tryAnother": "別のソースを試す",
      "raw": "RAW",
      "subBurnedIn": "焼き込み",
      "subSelectable": "選択可能"
    }
```
> Mind JSON commas — add a trailing comma to the preceding sibling key if needed.

- [ ] **Step 2: Run the i18n lint**

Run (from repo root): `bash frontend/web/scripts/i18n-lint.sh`
Expected: PASS (no missing keys across en/ru/ja).

- [ ] **Step 3: Type-check**

Run (from `frontend/web`): `bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 4: Commit**
```bash
git commit src/locales/en.json src/locales/ru.json src/locales/ja.json -m "i18n(player): player.sources keys (best/tryAnother/raw/sub delivery) en+ru+ja

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
Then `git show --stat HEAD` — confirm ONLY the three locale files.

---

## Task 8: Full verification + deploy

**Files:** none (gates only).

- [ ] **Step 1: Run the full player test surface + type-check**

Run (from `frontend/web`):
```bash
bunx vitest run src/composables/unifiedPlayer/ src/components/player/unified/ && bunx tsc --noEmit
```
Expected: all specs PASS, tsc clean.

- [ ] **Step 2: DS lint**

Run (from repo root): `bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS: 0`. New chip/panel markup uses tokens + brand-exempt hues only; player components are exempt from the Select-primitive rule. If a hardcoded hex slipped in, migrate it to a token (don't edit the allowlist).

- [ ] **Step 3: Build**

Run (from `frontend/web`): `bun run build`
Expected: build succeeds (Vite + vue-tsc).

- [ ] **Step 4: Deploy (controller / owner)**

Run (from repo root): `make redeploy-web && make health`
Expected: web rebuilt, `web` healthy. i18n-lint + DS-lint run as redeploy prerequisites — both must pass.

- [ ] **Step 5: Live smoke (optional — owner opt-in per feedback_chrome_smoke_opt_in)**

Non-small visual change in cascade-sensitive player chrome. OFFER the owner a Chrome checkup (don't run automatically): open a watch page, open the Source panel, confirm (a) only the selected provider shows by default with SUB/DUB/quality labels + "Best", (b) hacker mode reveals the full list, (c) forcing a stream error surfaces "Try another source" which expands the list, (d) a Kodik/AniLib title shows team chips with SUB/DUB tags. Caveat: jsdom/vitest can't catch Tailwind-v4 cascade bugs, so the label row's flex-col layout is worth an eyeball.

---

## Self-Review (completed during authoring + reconciliation)

- **Spec coverage (reconciled):** sub/dub/quality/sub-delivery labels ✔ (T3+T4), team tags ✔ (T5 `teamCategory`), declutter/collapse to the selected source + hacker reveal ✔ (T5 `visibleRows`/`topRow`), try-another escape hatch ✔ (T5), graceful degradation ✔ (empty `capMap` ⇒ chips render as today, collapse still works off `state`), i18n 3 locales ✔ (T7), tests ✔ (T2–T5). **Ordering deliberately NOT implemented** — superseded by Stage-2 (documented in the reconciliation header); the spec's §4.3 + ranking half of §5.1 are explicitly out of scope.
- **Type consistency:** `ProviderCap`/`CapVariant`/`CapabilityReport` (T1) used identically in `flattenCapabilities` (T2), `deriveCapLabels` (T3), `ProviderChip` (T4), `SourcePanel` (T5). `flattenCapabilities` returns `Map<string,ProviderCap>` surfaced as `capMap` in T2 and threaded through T6→T5. `deriveCapLabels` returns `CapLabels{categories,quality,subDelivery}` rendered in T4. No `rankedProviderIds`/`orderedProviderIds`/`pickSmartDefault` references remain anywhere in this plan.
- **Conflict surface:** T6 adds 4 lines to `UnifiedPlayer.vue` (1 import, 2 composable lines, 3 props) and touches NO ranking/fallback code — minimal overlap with the concurrent Stage-2 churn. Each commit is pathspec-explicit + `git show --stat`-verified to avoid re-capturing the shared index (the 2026-06-16 incident).
- **Placeholder scan:** none — every step has concrete code/commands. The single integration-boundary check (`sourceError` unwrap in the SourcePanel binding) is flagged with an explicit fallback in T6 Step 4.
- **Risk:** the collapse shows the selected provider; during the brief window before Stage-2's smart default selects one, `topRow` falls back to the first active row (T5) — never empty when an active provider exists. If Stage-2's ranking churn lands a `rows` reordering, the collapse still shows the correct selected chip (it keys off `provider`, not row position).
