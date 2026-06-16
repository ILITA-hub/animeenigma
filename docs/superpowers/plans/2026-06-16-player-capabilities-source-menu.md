# Player Capabilities Source-Menu — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the live player's Source menu to `GET /api/anime/{uuid}/capabilities` — server-driven ranking, declutter-to-top-1 (reveal via hacker mode + a playback-error escape hatch), and sub/dub/quality/team chip labels.

**Architecture:** An enrichment layer over the existing `useProviderHealth` pipeline. A new `useCapabilities` composable fetches the report and yields a `Map<providerId, ProviderCap>` + a ranked id list. Pure helpers (`flattenCapabilities`, `rankedProviderIds`, `deriveCapLabels`) do the logic and are unit-tested in isolation. `SourcePanel` merges the capability data into the chip list (sort + collapse + labels); `UnifiedPlayer` owns the wiring. If `/capabilities` is absent the map is empty and everything degrades to today's `CURATED_TIER` behaviour.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest (`@vue/test-utils`), vue-i18n. Frontend uses `bun`/`bunx`. DS lint exemption applies to `components/player/`.

**Spec:** `docs/superpowers/specs/2026-06-16-player-capabilities-source-menu-design.md`

**Convention:** every commit includes the co-author trailer (Claude Opus 4.6 / 0neymik0 / NANDIorg). Path-scope every `git add`; no `git add -A`. The controller lands commits on origin/main (worktree cherry-pick) — task commits are local.

---

## File Structure

**Create:**
- `frontend/web/src/types/capabilities.ts` — TS mirror of the Go `CapabilityReport`.
- `frontend/web/src/composables/unifiedPlayer/useCapabilities.ts` — fetch + `flattenCapabilities` pure helper + composable.
- `frontend/web/src/composables/unifiedPlayer/useCapabilities.spec.ts`
- `frontend/web/src/composables/unifiedPlayer/rankedProviderIds.ts` — pure ordering merge.
- `frontend/web/src/composables/unifiedPlayer/rankedProviderIds.spec.ts`
- `frontend/web/src/composables/unifiedPlayer/capLabels.ts` — pure label derivation.
- `frontend/web/src/composables/unifiedPlayer/capLabels.spec.ts`
- `frontend/web/src/components/player/unified/ProviderChip.spec.ts`
- `frontend/web/src/components/player/unified/SourcePanel.spec.ts`

**Modify:**
- `frontend/web/src/api/client.ts` — add `capabilitiesApi`.
- `frontend/web/src/components/player/unified/ProviderChip.vue` — optional `cap`/`best` props + label row.
- `frontend/web/src/components/player/unified/SourcePanel.vue` — collapse, labels, team tags, try-another disclosure.
- `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — wire `useCapabilities`, compute ordered ids, pass new props, swap the two `pickSmartDefault` array args.
- `frontend/web/src/locales/{en,ru,ja}.json` — `player.sources.*` keys.

**Working dir for all commands:** `cd /data/animeenigma/frontend/web`

---

## Task 1: Capability types + API client

**Files:**
- Create: `frontend/web/src/types/capabilities.ts`
- Modify: `frontend/web/src/api/client.ts` (add `capabilitiesApi` next to `scraperApi`, ~line 746)

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

In `frontend/web/src/api/client.ts`, immediately after the `scraperApi` object closes (the `getHealth: () => apiClient.get(\`/anime/_/scraper/health\`),` line and its closing `}` near line 746), add:
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
Expected: PASS (no errors). The new export is unused so far — that's fine.

- [ ] **Step 4: Commit**
```bash
git add src/types/capabilities.ts src/api/client.ts
git commit -m "feat(player): capability report types + capabilitiesApi client

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: `flattenCapabilities` + `useCapabilities`

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/useCapabilities.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/useCapabilities.spec.ts`

The pure `flattenCapabilities(report)` does all the testable logic; the composable is a thin fetch wrapper. We unit-test the pure function (no Vue reactivity needed).

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
    const { capMap } = flattenCapabilities(report)
    expect(capMap.size).toBe(3)
    expect(capMap.get('gogoanime')?.rank).toBe(120)
    expect(capMap.get('kodik')?.display_name).toBe('Kodik')
  })

  it('ranks ids by rank desc with name tiebreak', () => {
    const { rankedIds } = flattenCapabilities(report)
    expect(rankedIds).toEqual(['gogoanime', 'allanime', 'kodik'])
  })

  it('degrades to empty on null/malformed report', () => {
    expect(flattenCapabilities(null).capMap.size).toBe(0)
    expect(flattenCapabilities(null).rankedIds).toEqual([])
    // missing families array
    expect(flattenCapabilities({ anime_id: 'x' } as unknown as CapabilityReport).rankedIds).toEqual([])
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/composables/unifiedPlayer/useCapabilities.spec.ts`
Expected: FAIL — `flattenCapabilities` is not exported / module missing.

- [ ] **Step 3: Create the composable**

`frontend/web/src/composables/unifiedPlayer/useCapabilities.ts`:
```ts
import { ref, watch, type Ref } from 'vue'
import { capabilitiesApi } from '@/api/client'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

/**
 * Flatten a capability report into a providerId→ProviderCap map plus a
 * best-first ranked id list (rank desc, name tiebreak — mirrors the backend
 * stable sort). Pure + defensive: a null/malformed report yields empties.
 */
export function flattenCapabilities(report: CapabilityReport | null): {
  capMap: Map<string, ProviderCap>
  rankedIds: string[]
} {
  const capMap = new Map<string, ProviderCap>()
  if (!report || !Array.isArray(report.families)) return { capMap, rankedIds: [] }
  for (const fam of report.families) {
    for (const p of fam.providers ?? []) capMap.set(p.provider, p)
  }
  const rankedIds = [...capMap.values()]
    .sort((a, b) => b.rank - a.rank || a.provider.localeCompare(b.provider))
    .map((p) => p.provider)
  return { capMap, rankedIds }
}

/**
 * Fetch the capability report for an anime id (re-fetches when the id changes).
 * Decoration + ordering only — every failure degrades to empty, never throws.
 */
export function useCapabilities(animeId: Ref<string>) {
  const capMap = ref<Map<string, ProviderCap>>(new Map())
  const rankedIds = ref<string[]>([])
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
      const flat = flattenCapabilities(report)
      capMap.value = flat.capMap
      rankedIds.value = flat.rankedIds
    } catch {
      capMap.value = new Map()
      rankedIds.value = []
      error.value = true
    } finally {
      loaded.value = true
    }
  }

  watch(animeId, (id) => { void load(id) }, { immediate: true })

  return { capMap, rankedIds, loaded, error }
}
```

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/composables/unifiedPlayer/useCapabilities.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**
```bash
git add src/composables/unifiedPlayer/useCapabilities.ts src/composables/unifiedPlayer/useCapabilities.spec.ts
git commit -m "feat(player): useCapabilities composable + flattenCapabilities

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: `rankedProviderIds` (merge ordering)

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/rankedProviderIds.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/rankedProviderIds.spec.ts`

- [ ] **Step 1: Write the failing test**

`frontend/web/src/composables/unifiedPlayer/rankedProviderIds.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { rankedProviderIds } from './rankedProviderIds'
import type { ProviderRow, ProviderDef } from '@/types/unifiedPlayer'

function row(id: string, state: ProviderRow['state'] = 'active'): ProviderRow {
  return { def: { id } as ProviderDef, state }
}

const CURATED = ['ae', 'allanime', 'gogoanime', 'raw', '18anime']

describe('rankedProviderIds', () => {
  it('puts capability-ranked rows first, in ranked order', () => {
    const rows = [row('allanime'), row('gogoanime'), row('ae'), row('raw')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    // ranked first (gogo, allanime), then registry-only by CURATED order (ae, raw)
    expect(out).toEqual(['gogoanime', 'allanime', 'ae', 'raw'])
  })

  it('appends rows absent from the ranking in CURATED order then alpha', () => {
    const rows = [row('raw'), row('ae'), row('zzz')]
    const out = rankedProviderIds(rows, [], CURATED)
    expect(out).toEqual(['ae', 'raw', 'zzz']) // ae<raw by CURATED, zzz unknown→alpha last
  })

  it('ignores ranked ids that are not present as rows', () => {
    const rows = [row('ae')]
    const out = rankedProviderIds(rows, ['gogoanime', 'allanime'], CURATED)
    expect(out).toEqual(['ae'])
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/composables/unifiedPlayer/rankedProviderIds.spec.ts`
Expected: FAIL — module missing.

- [ ] **Step 3: Create the file**

`frontend/web/src/composables/unifiedPlayer/rankedProviderIds.ts`:
```ts
import type { ProviderRow } from '@/types/unifiedPlayer'

/**
 * Merge the server capability ranking with the registry rows into one ordered
 * id list for the smart default and the panel sort:
 *   1. capability-ranked ids that exist as rows (best first),
 *   2. rows absent from the ranking (ae/raw/18anime…), in `curated` order,
 *      with unknown ids alphabetised last.
 */
export function rankedProviderIds(
  rows: ProviderRow[],
  rankedIds: string[],
  curated: string[],
): string[] {
  const rowIds = new Set(rows.map((r) => r.def.id))
  const out: string[] = []
  const seen = new Set<string>()
  for (const id of rankedIds) {
    if (rowIds.has(id) && !seen.has(id)) {
      out.push(id)
      seen.add(id)
    }
  }
  const curatedPos = (id: string) => {
    const i = curated.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  const remaining = [...rowIds].filter((id) => !seen.has(id))
  remaining.sort((a, b) => curatedPos(a) - curatedPos(b) || a.localeCompare(b))
  out.push(...remaining)
  return out
}
```

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/composables/unifiedPlayer/rankedProviderIds.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**
```bash
git add src/composables/unifiedPlayer/rankedProviderIds.ts src/composables/unifiedPlayer/rankedProviderIds.spec.ts
git commit -m "feat(player): rankedProviderIds merge ordering

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: `deriveCapLabels` (chip label derivation)

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
git add src/composables/unifiedPlayer/capLabels.ts src/composables/unifiedPlayer/capLabels.spec.ts
git commit -m "feat(player): deriveCapLabels for provider chip labels

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: `ProviderChip.vue` — capability label row

**Files:**
- Modify: `frontend/web/src/components/player/unified/ProviderChip.vue`
- Test: `frontend/web/src/components/player/unified/ProviderChip.spec.ts`

Add optional `cap` + `best` props. When `cap` resolves labels, render a second line under the name with category tags (`data-test="cap-cat"`), a quality tag (`data-test="cap-quality"`), and a "best" pill (`data-test="cap-best"`). i18n via the global `$t` (stubbed in tests).

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
Expected: FAIL — no `cap-cat`/`cap-best` elements (props ignored).

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

Then update the `<script setup>` block. Replace the existing imports + `defineProps` with:
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

> Note: the button is `inline-flex items-center`; the name span changing to `flex-col` is fine — it stays vertically centred. The wip/down badges and the selected check remain after this span, unchanged.

- [ ] **Step 4: Run, confirm pass**

Run: `bunx vitest run src/components/player/unified/ProviderChip.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**
```bash
git add src/components/player/unified/ProviderChip.vue src/components/player/unified/ProviderChip.spec.ts
git commit -m "feat(player): ProviderChip capability label row (sub/dub/quality/best)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: `SourcePanel.vue` — collapse, labels, team tags, try-another

**Files:**
- Modify: `frontend/web/src/components/player/unified/SourcePanel.vue`
- Test: `frontend/web/src/components/player/unified/SourcePanel.spec.ts`

Add three props (`capMap`, `rankedIds`, `hackerMode`, `playbackError`), compute `visibleRows` (collapse), pass `cap`/`best` to each chip, tag team chips, and render the try-another disclosure.

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
  audio: 'sub' as const, lang: 'en' as const, team: null, provider: '', server: '',
  servers: [] as { id: string; label: string }[], teams: [] as string[],
  capMap: new Map<string, ProviderCap>(), rankedIds: ['gogoanime', 'allanime', 'miruro'],
}
const rows = [row('gogoanime'), row('allanime'), row('miruro')]

describe('SourcePanel collapse', () => {
  it('default (hacker off) shows only the top active provider', () => {
    const w = mount(SourcePanel, { props: { ...base, rows, hackerMode: false, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('gogoanime')
  })

  it('hacker mode shows the full ranked list', () => {
    const w = mount(SourcePanel, { props: { ...base, rows, hackerMode: true, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })

  it('shows try-another only on playback error, and expands on click', async () => {
    const w = mount(SourcePanel, { props: { ...base, rows, hackerMode: false, playbackError: true }, ...mountOpts })
    const btn = w.find('[data-test="try-another"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(3)
  })

  it('promotes the next active provider when the top one is down', () => {
    const downTop = [row('gogoanime', 'down'), row('allanime'), row('miruro')]
    const w = mount(SourcePanel, { props: { ...base, rows: downTop, hackerMode: false, playbackError: false }, ...mountOpts })
    expect(w.findAll('[data-test="provider-chip"]').length).toBe(1)
    expect(w.find('[data-test="provider-chip"]').attributes('data-id')).toBe('allanime')
  })
})
```

- [ ] **Step 2: Run, confirm fail**

Run: `bunx vitest run src/components/player/unified/SourcePanel.spec.ts`
Expected: FAIL — panel renders all rows / unknown props.

- [ ] **Step 3: Edit the template — provider list**

In `frontend/web/src/components/player/unified/SourcePanel.vue`, replace the entire "Provider list" block (the `<div class="flex flex-col gap-1">` containing the header span and the `ProviderChip` `v-for`, lines ~112–126) with:
```vue
    <!-- Provider list (collapsed to top-1 unless hacker mode or error-expanded) -->
    <div class="flex flex-col gap-1">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-[2px]">
        Provider
      </span>
      <div class="flex flex-col gap-1">
        <ProviderChip
          v-for="(r, i) in visibleRows"
          :key="r.def.id"
          :row="r"
          :cap="capMap.get(r.def.id)"
          :best="!hackerMode && !expanded && i === 0"
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

In the team-chips `<button v-for="t in teams">` (lines ~94–108), replace the chip's inner `{{ t }}` text with a name + optional category tag:
```vue
          <span>{{ t }}</span>
          <span
            v-if="teamCategory(t)"
            class="ml-[5px] text-[9px] font-semibold font-mono uppercase tracking-wide opacity-80"
          >{{ teamCategory(t) === 'dub' ? $t('player.dub') : $t('player.sub') }}</span>
```

- [ ] **Step 5: Edit the `<script setup>` block**

Replace the imports + `defineProps` + `computed` section. New script:
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
  rankedIds: string[]
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

// Rows sorted by the merged capability ranking (registry order for unranked).
const sortedRows = computed(() => {
  const pos = (id: string) => {
    const i = props.rankedIds.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  return [...props.rows].sort((a, b) => pos(a.def.id) - pos(b.def.id))
})
const activeRows = computed(() => sortedRows.value.filter((r) => r.state === 'active'))
const activeCount = computed(() => activeRows.value.length)

// Collapse: top-1 playable by default; hacker mode → full list; error-expanded → all active.
const expanded = ref(false)
watch(() => props.provider, () => { expanded.value = false })

const visibleRows = computed(() => {
  if (props.hackerMode) return sortedRows.value
  if (expanded.value) return activeRows.value
  return activeRows.value.slice(0, 1)
})
const hiddenCount = computed(() => activeRows.value.length - 1)

// Team → category tag from the selected provider's capability variants.
function teamCategory(name: string): 'sub' | 'dub' | null {
  const cap = props.capMap.get(props.provider)
  if (!cap) return null
  const v = cap.variants.find((x) => x.team?.name === name)
  if (!v) return null
  return v.category === 'dub' ? 'dub' : v.category === 'sub' ? 'sub' : null
}
```

> `activeCount` now counts active rows (was all-rows length before, but the header reads "{{ activeCount }} available" which is the active count — semantics preserved). The `team`/`server` props are still used in the template (team-chip selected styling, server list); keep them.

- [ ] **Step 6: Run, confirm pass**

Run: `bunx vitest run src/components/player/unified/SourcePanel.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**
```bash
git add src/components/player/unified/SourcePanel.vue src/components/player/unified/SourcePanel.spec.ts
git commit -m "feat(player): SourcePanel collapse + cap labels + team tags + try-another

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: `UnifiedPlayer.vue` — wire it together

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

No new test (integration is covered by the component specs + Task 9 build). Five edits.

- [ ] **Step 1: Add imports**

After the existing `import { pickSmartDefault } from '@/composables/unifiedPlayer/smartDefault'` (line ~333), add:
```ts
import { useCapabilities } from '@/composables/unifiedPlayer/useCapabilities'
import { rankedProviderIds } from '@/composables/unifiedPlayer/rankedProviderIds'
```
Confirm `computed` is already imported from `vue` at the top of the script (it is — used widely). If not, add it.

- [ ] **Step 2: Construct the composable + ordered ids**

Immediately after the `const { rows, start } = useProviderHealth(filter)` line (line ~388), add:
```ts
// Capability report (server ranking + sub/dub/quality/team labels). Decoration
// + ordering only — degrades to CURATED_TIER when absent.
const animeIdRef = computed(() => props.animeId)
const { capMap, rankedIds: capRankedIds } = useCapabilities(animeIdRef)
// Merged order: capability-ranked first, registry-only (ae/raw/18anime) in CURATED order.
const orderedProviderIds = computed(() =>
  rankedProviderIds(rows.value, capRankedIds.value, CURATED_TIER),
)
```

- [ ] **Step 3: Swap the two `pickSmartDefault` array args**

At line ~498, change:
```ts
    void pickSmartDefault(rows.value, CURATED_TIER, {
```
to:
```ts
    void pickSmartDefault(rows.value, orderedProviderIds.value, {
```

At line ~894–897, change:
```ts
        const next = await pickSmartDefault(
          rows.value.filter((r) => r.def.id !== failed),
          CURATED_TIER,
          { needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable },
        )
```
to:
```ts
        const next = await pickSmartDefault(
          rows.value.filter((r) => r.def.id !== failed),
          orderedProviderIds.value,
          { needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable },
        )
```
> Leave the `import { providerById, CURATED_TIER } from './providerRegistry'` import as-is — `CURATED_TIER` is still referenced by `rankedProviderIds` as the fallback order.

- [ ] **Step 4: Pass the new props to `SourcePanel`**

In the `<SourcePanel ... />` usage (lines ~211–225), add four bindings (e.g. after `:teams="teams"`):
```vue
        :cap-map="capMap"
        :ranked-ids="orderedProviderIds"
        :hacker-mode="state.hackerMode.value"
        :playback-error="!!sourceError"
```
> `sourceError` is the existing string ref (set in the resolve catch); `!!sourceError` would always be true since it's a ref object. Bind the unwrapped value: use `:playback-error="Boolean(sourceError)"` only if `sourceError` is auto-unwrapped in template — it is (top-level ref in `<script setup>` is unwrapped in template), so `!!sourceError` evaluates the string. Confirm in template `sourceError` renders as the message elsewhere; if it's referenced as `sourceError` (not `sourceError.value`) in the template, then `:playback-error="!!sourceError"` is correct.

- [ ] **Step 5: Clear `sourceError` on successful load**

Find where a resolve succeeds. In `loadEpisodesAndStream` (the `try` block starting ~line 827), add a reset at the very top of the `try` (right after `try {`):
```ts
    sourceError.value = ''
```
> This clears a stale error when a new provider resolve begins, so the try-another disclosure and `playbackError` flag reset on every fresh attempt. Verify `sourceError` is a `ref('')` (string) — it is (assigned string literals throughout). If the variable is named differently, grep `sourceError` and use the actual name.

- [ ] **Step 6: Type-check + build**

Run: `bunx tsc --noEmit`
Expected: PASS. If vue-tsc flags `sourceError` unwrap or a missing `computed` import, fix per the notes above.

- [ ] **Step 7: Commit**
```bash
git add src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(player): wire UnifiedPlayer to /capabilities (ranking + collapse + labels)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 8: i18n keys (en / ru / ja)

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

All three MUST get the keys (i18n-lint hard-fails `make redeploy-web` on any missing key).

- [ ] **Step 1: Add the `player.sources` sub-namespace**

In each locale file, inside the existing `"player": { ... }` object, add a `"sources"` key. Use the values below.

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
> Place it adjacent to the existing `player.sub` / `player.dub` keys. Mind JSON commas — add a trailing comma to the preceding sibling key if needed.

- [ ] **Step 2: Run the i18n lint**

Run: `bash frontend/web/scripts/i18n-lint.sh` (from repo root) — or `cd /data/animeenigma && bash frontend/web/scripts/i18n-lint.sh`
Expected: PASS (no missing keys across en/ru/ja).

- [ ] **Step 3: Type-check (locales are typed via the i18n schema if present)**

Run (from `frontend/web`): `bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 4: Commit**
```bash
git add src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit -m "i18n(player): player.sources keys (best/tryAnother/raw/sub delivery) en+ru+ja

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 9: Full verification + deploy

**Files:** none (gates only).

- [ ] **Step 1: Run the full player test surface + type-check**

Run (from `frontend/web`):
```bash
bunx vitest run src/composables/unifiedPlayer/ src/components/player/unified/ && bunx tsc --noEmit
```
Expected: all specs PASS, tsc clean.

- [ ] **Step 2: DS lint**

Run (from repo root): `bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS: 0`. The new chip/panel markup uses tokens + brand-exempt hues only; player components are exempt from the Select-primitive rule. If a new hardcoded hex slipped in, migrate it to a token (do not edit the allowlist).

- [ ] **Step 3: Build**

Run (from `frontend/web`): `bun run build`
Expected: build succeeds (Vite + vue-tsc).

- [ ] **Step 4: Deploy (controller / owner)**

Run (from repo root): `make redeploy-web && make health`
Expected: web rebuilt (brotli + manualChunks), `web` healthy. i18n-lint + DS-lint run as redeploy prerequisites — both must pass.

- [ ] **Step 5: Live smoke (optional — owner opt-in per feedback_chrome_smoke_opt_in)**

This is a non-small visual change in cascade-sensitive player chrome. OFFER the owner a Chrome checkup (don't run automatically): open a watch page, open the Source panel, confirm (a) only the top-ranked provider shows by default with SUB/DUB/quality labels + "Best", (b) enabling hacker mode reveals the full ranked list, (c) forcing a stream error surfaces "Try another source" which expands the list, (d) a Kodik/AniLib title shows team chips with SUB/DUB tags. Caveat: jsdom/vitest can't catch Tailwind-v4 cascade bugs, so the label row's flex-col layout is worth an eyeball.

---

## Self-Review (completed during authoring)

- **Spec coverage:** server ranking ✔ (T2 `flattenCapabilities` + T3 `rankedProviderIds` + T7 swap), declutter/collapse tied to hacker mode + top-1 + dead-source promote ✔ (T6 `visibleRows`/`activeRows.slice(0,1)`), try-another escape hatch ✔ (T6), sub/dub/quality/sub-delivery labels ✔ (T4 + T5), team tags ✔ (T6 `teamCategory`), graceful degradation ✔ (empty `capMap` ⇒ `orderedProviderIds` falls back to CURATED via T3), i18n 3 locales ✔ (T8), tests ✔ (T2–T6), coverage-gap providers (ae/raw/18anime) ✔ (T3 appends them, T5 `capMap.get` returns undefined → no labels).
- **Type consistency:** `ProviderCap`/`CapVariant`/`CapabilityReport` (T1) used identically in `flattenCapabilities` (T2), `deriveCapLabels` (T4), `ProviderChip` (T5), `SourcePanel` (T6). `flattenCapabilities` returns `{ capMap, rankedIds }` consumed by `useCapabilities` (T2) and surfaced as `capMap`/`capRankedIds` in T7. `rankedProviderIds(rows, rankedIds, curated)` signature (T3) matches the T7 call. `deriveCapLabels` returns `CapLabels{categories,quality,subDelivery}` rendered in T5.
- **Placeholder scan:** none — every step has concrete code/commands. The two integration-boundary checks (`sourceError` unwrap in the SourcePanel binding; `computed` already imported in UnifiedPlayer) are flagged with explicit verify-and-adjust notes, as they depend on the exact current file state.
- **Risk:** `sourceError` is a string ref; `!!sourceError` in the template evaluates the unwrapped string (truthy when a message is set). If a future refactor makes it an object, switch to an explicit boolean computed. Documented in T7 Step 4.
