# aePlayer RAW/DUB + combo/URL + top-3 + subtitle fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the aePlayer so the top slider is RAW/DUB (language RU/EN only under DUB), the saved watch combo + URL params are respected on open, the top-3 providers stay selectable without hacker mode, Japanese is reachable, and subtitles default OFF + persist across episode change + the CC icon reflects enabled state.

**Architecture:** Pure FE change in `frontend/web/`. `Combo.audio` stays `'sub' | 'dub'`; "RAW"⇔`'sub'`, "DUB"⇔`'dub'`. Under RAW the provider relevance filter ignores language and matches original-audio caps (`sub` or `raw`); `combo.lang` is derived from the chosen provider's group. The backend `/capabilities` feed is unchanged (still the single source of truth for state/selectable/hacker_only).

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest, vue-i18n, Tailwind v4 + Neon-Tokyo DS tokens, `bun`/`bunx`.

## Global Constraints

- Work only inside this worktree. Run all FE commands from `frontend/web/` with `bun`/`bunx` (never npm/npx).
- `Combo.audio` type stays `'sub' | 'dub'`; `Combo.lang` stays `'en' | 'ru' | 'ja'`. No type changes to these.
- i18n parity is build-enforced: every new key MUST exist in `en.json`, `ru.json`, AND `ja.json` or the parity spec + redeploy gate fail.
- DS-lint is build-enforced: bind colors to tokens (`text-[var(--brand-cyan)]`, `text-[var(--muted-foreground)]`), only `font-medium`/`font-semibold`, no off-palette Tailwind color classes. brand cyan/pink are exempt.
- Subtitle default is **OFF with no exceptions** (incl. pure raw-JP) — this deliberately reverses the prior "raw/JP auto-enables Jimaku" rule.
- Top-3 is **FE-side** (pad-to-3); do NOT change the backend feed.
- Commit after every task with the three co-authors:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

---

### Task 1: i18n key for the RAW label

**Files:**
- Modify: `frontend/web/src/locales/en.json` (`player.aePlayer` object)
- Modify: `frontend/web/src/locales/ru.json` (`player.aePlayer` object)
- Modify: `frontend/web/src/locales/ja.json` (`player.aePlayer` object)
- Test: `frontend/web/src/locales/__tests__/` (existing parity spec)

**Interfaces:**
- Produces: i18n key `player.aePlayer.audioRaw`. The DUB label reuses the existing `player.dub` key (already `Dub` / `Озвучка` / `吹替`).

- [ ] **Step 1: Add the key to all three locales.** In each file, inside the existing `"player": { ... "aePlayer": { ... } }` object, add a sibling key next to `"audio"`:
  - `en.json`: `"audioRaw": "Original",`
  - `ru.json`: `"audioRaw": "Оригинал",`
  - `ja.json`: `"audioRaw": "オリジナル",`

- [ ] **Step 2: Run the locale parity test.**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__`
Expected: PASS (all three locales agree on key sets).

- [ ] **Step 3: Commit.**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "i18n(aeplayer): add audioRaw label (Original/Оригинал/オリジナル)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Provider feed — RAW/DUB relevance + raw→sub row mapping

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts`
- Modify: `frontend/web/src/composables/aePlayer/providerGroups.ts`
- Test: `frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts`

**Interfaces:**
- Consumes: `ProviderCap` (`{ provider, display_name, group, state, selectable, hacker_only, order, audios, reason }`, `audios: string[]` may include `'raw'`), `GROUP_LANGS`, `GROUP_CONTENT`.
- Produces:
  - `GROUP_PRIMARY_LANG: Record<ProviderGroup, TrackLang>` and `langForProviderUnderRaw(group: ProviderGroup, currentLang: TrackLang): TrackLang` exported from `providerGroups.ts`.
  - `rowsFromReport(report, filter)` unchanged signature; new RAW/DUB semantics.

- [ ] **Step 1: Add the new failing tests** to `useProviderFeed.spec.ts`. Read the existing file first to match its `ProviderCap`/report fixture helpers, then add:

```ts
// RAW (audio:'sub') shows every original-audio source regardless of language,
// and maps a raw-only cap to a 'sub' row.
it('RAW lists en, ru and jp original sources and hides dub-only', () => {
  const report = mkReport([
    cap({ provider: 'gogoanime', group: 'en', audios: ['sub', 'dub'], order: 90 }),
    cap({ provider: 'kodik', group: 'ru', audios: ['sub', 'dub'], order: 80 }),
    cap({ provider: 'raw', group: 'jp', audios: ['raw'], order: 70 }),
    cap({ provider: 'dubonly', group: 'en', audios: ['dub'], order: 60 }),
  ])
  const rows = rowsFromReport(report, { audio: 'sub', lang: 'en', content: 'common' })
  expect(rows.map(r => r.id)).toEqual(['gogoanime', 'kodik', 'raw'])
  expect(rows.find(r => r.id === 'raw')!.audios).toEqual(['sub']) // raw→sub
})

// DUB keeps the language gate.
it('DUB only shows dub sources for the selected language', () => {
  const report = mkReport([
    cap({ provider: 'gogoanime', group: 'en', audios: ['sub', 'dub'], order: 90 }),
    cap({ provider: 'kodik', group: 'ru', audios: ['sub', 'dub'], order: 80 }),
  ])
  expect(rowsFromReport(report, { audio: 'dub', lang: 'en', content: 'common' }).map(r => r.id)).toEqual(['gogoanime'])
  expect(rowsFromReport(report, { audio: 'dub', lang: 'ru', content: 'common' }).map(r => r.id)).toEqual(['kodik'])
})
```

(`mkReport`/`cap` are the spec's existing fixture helpers — reuse them; if they don't exist, write minimal ones matching `CapabilityReport`/`ProviderCap`.)

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: FAIL (RAW currently requires `audios.includes('sub')` + lang match, so `raw` is excluded and `kodik` is excluded under lang 'en').

- [ ] **Step 3: Add group helpers** to `providerGroups.ts` (append after the existing `GROUP_CONTENT`):

```ts
// Primary served language per group — the lang a RAW (original-audio) pick
// resolves to when the current lang isn't in the provider's group.
export const GROUP_PRIMARY_LANG: Record<ProviderGroup, TrackLang> = {
  en: 'en', ru: 'ru', adult: 'en', jp: 'ja', firstparty: 'ja',
}

// Under RAW the language slider is hidden — combo.lang follows the chosen
// provider's group. Keep the current lang if the group serves it, else fall
// back to the group's primary language.
export function langForProviderUnderRaw(group: ProviderGroup, currentLang: TrackLang): TrackLang {
  return GROUP_LANGS[group].includes(currentLang) ? currentLang : GROUP_PRIMARY_LANG[group]
}
```

- [ ] **Step 4: Rewrite `relevant` and `toRow`** in `useProviderFeed.ts`:

```ts
function relevant(cap: ProviderCap, f: RowFilter): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  if (!GROUP_CONTENT[g].includes(f.content)) return false
  if (f.audio === 'dub') {
    return cap.audios.includes('dub') && GROUP_LANGS[g].includes(f.lang)
  }
  // RAW (audio === 'sub'): original voices — any language group, sub OR raw caps.
  return cap.audios.includes('sub') || cap.audios.includes('raw')
}

function toRow(cap: ProviderCap): ProviderRow {
  // 'raw' caps are original-audio → surface as a 'sub' (RAW) row.
  const audios = [...new Set(cap.audios.map((a) => (a === 'dub' ? 'dub' : 'sub')))]
    .filter((a): a is AudioKind => a === 'sub' || a === 'dub')
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios, reason: cap.reason,
  }
}
```

- [ ] **Step 5: Run tests to verify pass.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts`
Expected: PASS (all cases, including any pre-existing ones — fix any that asserted the old lang-gated RAW behavior to reflect the new model).

- [ ] **Step 6: Commit.**

```bash
git add frontend/web/src/composables/aePlayer/useProviderFeed.ts frontend/web/src/composables/aePlayer/providerGroups.ts frontend/web/src/composables/aePlayer/useProviderFeed.spec.ts
git commit -m "feat(aeplayer): RAW shows all original-audio sources, DUB keeps lang gate

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: SourcePanel slider RAW/DUB + conditional RU/EN language slider

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SourcePanel.vue`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (`audioLabel` computed only)
- Test: `frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts`

**Interfaces:**
- Consumes: `player.aePlayer.audioRaw` (Task 1), `player.dub` i18n keys.
- Produces: audio slider labelled Original/Dub; language slider rendered only when `audio === 'dub'` with options EN/RU.

- [ ] **Step 1: Add failing tests** to `SourcePanel.spec.ts` (read it first for its mount helper `mountOpts` and `cb` base props):

```ts
it('audio slider shows the RAW/DUB labels, not SUB/DUB', () => {
  const w = mount(SourcePanel, { props: { ...cb, rows: [], audio: 'sub' } as any, ...mountOpts })
  expect(w.find('[data-test="audio-sub"]').text()).toBe('Original')
  expect(w.find('[data-test="audio-dub"]').text()).toBe('Dub')
})

it('hides the language slider under RAW and shows EN/RU (no JA) under DUB', () => {
  const raw = mount(SourcePanel, { props: { ...cb, rows: [], audio: 'sub' } as any, ...mountOpts })
  expect(raw.find('[data-test="lang-en"]').exists()).toBe(false)

  const dub = mount(SourcePanel, { props: { ...cb, rows: [], audio: 'dub', lang: 'en' } as any, ...mountOpts })
  expect(dub.find('[data-test="lang-en"]').exists()).toBe(true)
  expect(dub.find('[data-test="lang-ru"]').exists()).toBe(true)
  expect(dub.find('[data-test="lang-ja"]').exists()).toBe(false)
})
```

If the spec's i18n mock returns keys verbatim, assert `'player.aePlayer.audioRaw'`/`'player.dub'` instead of `'Original'`/`'Dub'` — match the file's existing i18n stubbing.

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts`
Expected: FAIL (labels are `SUB`/`DUB`; language slider always renders 3 options incl. JA).

- [ ] **Step 3: Update the audio options + add `useI18n`** in `SourcePanel.vue` `<script setup>`. Add the import and translate the labels:

```ts
import { useI18n } from 'vue-i18n'
const { t } = useI18n()

const audioOptions = computed<{ value: AudioKind; label: string }[]>(() => [
  { value: 'sub', label: t('player.aePlayer.audioRaw') },
  { value: 'dub', label: t('player.dub') },
])

const langOptions: { value: TrackLang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ru', label: 'Русский' },
]
```

(Delete the old `const audioOptions = [...]` and the `ja` entry from `langOptions`. `audioOptions` is now a `computed`, so its template `v-for` is unchanged.)

- [ ] **Step 4: Make the language slider conditional + 2-column** in the template. Wrap the whole `<!-- Language slider -->` `<div>` block in `v-if="audio === 'dub'"`, and change the grid to 2 columns + the thumb width to `/ 2`:
  - On the language slider container `<div ... style="... grid-template-columns: repeat(3, 1fr);">` change `repeat(3, 1fr)` → `repeat(2, 1fr)`.
  - On the language sliding thumb `<span ... :style="{ width: 'calc((100% - 8px) / 3)', ... }">` change `/ 3` → `/ 2`.

```html
<!-- Language slider (DUB only — RAW is original voices) -->
<div v-if="audio === 'dub'">
  ...
  <div class="relative rounded-full p-1" style="background: var(--white-a8); display: grid; grid-template-columns: repeat(2, 1fr);" ...>
    <span ... :style="{ width: 'calc((100% - 8px) / 2)', ... }" />
    ...
  </div>
</div>
```

- [ ] **Step 5: Translate the control-bar audio label** in `AePlayer.vue`. Replace the `audioLabel` computed (currently `return a === 'dub' ? 'DUB' : 'SUB'`) with:

```ts
const audioLabel = computed(() =>
  state.combo.value.audio === 'dub' ? t('player.dub') : t('player.aePlayer.audioRaw'),
)
```

(`t` is already in scope — `hardsubNote` uses it.)

- [ ] **Step 6: Run tests to verify pass.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add frontend/web/src/components/player/aePlayer/SourcePanel.vue frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts
git commit -m "feat(aeplayer): RAW/DUB audio slider, RU/EN language only under DUB

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Top-3 provider fallback (pad-to-3, selectable without hacker mode)

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/SourcePanel.vue`
- Modify: `frontend/web/src/components/player/aePlayer/ProviderChip.vue`
- Test: `frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts`, `frontend/web/src/components/player/aePlayer/ProviderChip.spec.ts`

**Interfaces:**
- Produces: `SourcePanel` pads its collapsed list to 3 with degraded/recovering rows and passes `:forced` to chips; `ProviderChip` gains a `forced?: boolean` prop that unlocks selectability for a hacker-only row.

- [ ] **Step 1: Add the failing SourcePanel test** (reuse the spec's `a(id, state?)` row factory + `mountOpts`):

```ts
it('pads the collapsed list to 3 with degraded rows when fewer than 3 are active', () => {
  const rows = [a('gogoanime', 'active'), a('kodik', 'degraded'), a('miruro', 'degraded'), a('raw', 'degraded')]
  const w = mount(SourcePanel, { props: { ...cb, rows, provider: '', hackerMode: false } as any, ...mountOpts })
  const chips = w.findAll('[data-test="provider-chip"]')
  expect(chips.map(c => c.attributes('data-id'))).toEqual(['gogoanime', 'kodik', 'miruro'])
})
```

- [ ] **Step 2: Add the failing ProviderChip test** (read the spec for its `mkRow` helper):

```ts
it('a forced degraded row is selectable without hacker mode', async () => {
  const row = mkRow({ id: 'kodik', state: 'degraded', selectable: true, hackerOnly: true })
  const w = mount(ProviderChip, { props: { row, hackerMode: false, forced: true } as any, ...mountOpts })
  await w.find('button').trigger('click')
  expect(w.emitted('select')).toBeTruthy()
})
```

- [ ] **Step 3: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts src/components/player/aePlayer/ProviderChip.spec.ts`
Expected: FAIL (collapsed list shows only the 1 active row; forced prop doesn't exist so the degraded chip ignores the click).

- [ ] **Step 4: Add the `forced` prop to `ProviderChip.vue`.** In `defineProps`, add `forced?: boolean`, and widen the `selectable` computed:

```ts
const props = defineProps<{
  row: ProviderRow
  selected?: boolean
  cap?: ProviderCap
  best?: boolean
  hackerMode?: boolean
  /** Top-3 fallback: force selectability for a hacker-only row when too few are active. */
  forced?: boolean
}>()

const selectable = computed(
  () => props.row.selectable && (!props.row.hackerOnly || props.hackerMode === true || props.forced === true),
)
```

(If the chip greys/dims non-selectable rows via a class bound to `selectable`, that binding now also clears when `forced` — no extra change needed since it reads the same computed.)

- [ ] **Step 5: Pad the collapsed list + expose forced ids** in `SourcePanel.vue`. Replace `collapsedRows` and add `forcedSelectableIds`:

```ts
const collapsedRows = computed(() => {
  const top = activeRows.value.slice(0, TOP_N)
  // Pad with the next best non-active sources so the user always has up to
  // TOP_N selectable providers, even when nothing is active (degraded fleet).
  if (top.length < TOP_N) {
    for (const r of sortedRows.value) {
      if (top.length >= TOP_N) break
      if (r.state === 'active' || r.state === 'no_content') continue
      if (!top.some(x => x.id === r.id)) top.push(r)
    }
  }
  const selected = sortedRows.value.find(r => r.id === props.provider)
  if (selected && !top.some(r => r.id === selected.id)) top.push(selected)
  return top
})

// The padded (non-active, non-no_content) rows are made selectable without
// hacker mode so a fully-degraded fleet is never a dead end.
const forcedSelectableIds = computed(() => {
  const ids = new Set<string>()
  for (const r of collapsedRows.value) {
    if (r.state !== 'active' && r.state !== 'no_content') ids.add(r.id)
  }
  return ids
})
```

- [ ] **Step 6: Pass `:forced` to the chip** in the SourcePanel template `<ProviderChip ... />`:

```html
<ProviderChip
  v-for="r in visibleRows"
  :key="r.id"
  :row="r"
  :cap="capMap.get(r.id)"
  :best="!hackerMode && !expanded && r.id === topRow?.id"
  :selected="r.id === provider"
  :hacker-mode="hackerMode"
  :forced="forcedSelectableIds.has(r.id)"
  @select="emit('select-provider', r.id)"
/>
```

- [ ] **Step 7: Run tests to verify pass.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts src/components/player/aePlayer/ProviderChip.spec.ts`
Expected: PASS.

- [ ] **Step 8: Commit.**

```bash
git add frontend/web/src/components/player/aePlayer/SourcePanel.vue frontend/web/src/components/player/aePlayer/ProviderChip.vue frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts frontend/web/src/components/player/aePlayer/ProviderChip.spec.ts
git commit -m "feat(aeplayer): keep top-3 providers selectable without hacker mode

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Facet glue — RAW lang follows provider, language bias, dead-player fallback

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/usePlayerState.ts`
- Modify: `frontend/web/src/composables/aePlayer/smartDefault.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Test: `frontend/web/src/composables/aePlayer/smartDefault.spec.ts`

**Interfaces:**
- Consumes: `langForProviderUnderRaw`, `GROUP_LANGS` (Task 2); `rows` computed; `state`.
- Produces:
  - `usePlayerState` exports `setServedLang(l: TrackLang)` — sets `combo.lang` WITHOUT resetting `team`.
  - `smartDefault.ts` exports `pickRawBiased(rows: ProviderRow[], lang: TrackLang): ProviderRow | null` and `pickSelectableFallback(rows: ProviderRow[]): ProviderRow | null`.

- [ ] **Step 1: Add failing tests** to `smartDefault.spec.ts` (reuse its `row(...)` factory; rows carry `state`, `selectable`, `order`, `group`):

```ts
import { pickRawBiased, pickSelectableFallback } from './smartDefault'

it('pickRawBiased prefers the best active row in the requested language group', () => {
  const rows = [
    row({ id: 'gogoanime', group: 'en', state: 'active', selectable: true, order: 90 }),
    row({ id: 'kodik', group: 'ru', state: 'active', selectable: true, order: 80 }),
  ]
  expect(pickRawBiased(rows, 'ru')!.id).toBe('kodik')   // honored language
  expect(pickRawBiased(rows, 'en')!.id).toBe('gogoanime')
  expect(pickRawBiased(rows, 'ja')!.id).toBe('gogoanime') // no JA group → global best
})

it('pickSelectableFallback returns the top-ranked selectable row even if degraded', () => {
  const rows = [
    row({ id: 'kodik', group: 'ru', state: 'degraded', selectable: true, order: 80 }),
    row({ id: 'raw', group: 'jp', state: 'degraded', selectable: true, order: 70 }),
  ]
  expect(pickSelectableFallback(rows)!.id).toBe('kodik')
  expect(pickSelectableFallback([])).toBeNull()
})
```

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/smartDefault.spec.ts`
Expected: FAIL (functions not exported).

- [ ] **Step 3: Add the helpers** to `smartDefault.ts` (keep `pickSmartDefault` as-is, append):

```ts
import { GROUP_LANGS } from './providerGroups'

// RAW (original-audio) default: prefer the best active source whose group serves
// the requested language (don't yank a RU-sub watcher onto an EN source); fall
// back to the global best active source.
export function pickRawBiased(rows: ProviderRow[], lang: TrackLang): ProviderRow | null {
  const inLang = rows.filter((r) => GROUP_LANGS[r.group].includes(lang))
  return pickSmartDefault(inLang) ?? pickSmartDefault(rows)
}

// Dead-player guard: when no source is `active`, take the top-ranked SELECTABLE
// row (degraded/recovering included) so the player still attempts playback.
export function pickSelectableFallback(rows: ProviderRow[]): ProviderRow | null {
  return [...rows].filter((r) => r.selectable).sort((a, b) => b.order - a.order)[0] ?? null
}
```

(Add `TrackLang` + `ProviderRow` to the existing type import from `@/types/aePlayer` if not already present.)

- [ ] **Step 4: Run to verify pass.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/smartDefault.spec.ts`
Expected: PASS.

- [ ] **Step 5: Add `setServedLang`** to `usePlayerState.ts`. After `setLang` (line 17) add:

```ts
  // Set lang WITHOUT resetting team — used under RAW where combo.lang is derived
  // from the chosen provider's group, not a user facet.
  const setServedLang = (l: TrackLang) => { combo.value = { ...combo.value, lang: l } }
```

And export it in both the return object and (if present) the type — add `setServedLang` to the returned object next to `setLang`.

- [ ] **Step 6: Make RAW lang changes inert + add the lang-follows-provider watcher + biased/fallback picks** in `AePlayer.vue`.

  (a) In the combo facet watcher (the `watch(() => [audio, lang, server, team, episode], (newVal, oldVal) => {...})` around line 1330), replace the `facetChanged` block:

```ts
    const audioChanged = newVal[0] !== oldVal[0]
    const langChanged = newVal[1] !== oldVal[1]
    // Under RAW the language slider is hidden — combo.lang is a derived value that
    // follows the chosen provider. A RAW-only lang change must NOT repick or
    // re-resolve (it would churn the source); language is a real facet only under DUB.
    if (!audioChanged && langChanged && newVal[0] === 'sub') return
    const facetChanged = audioChanged || (newVal[0] === 'dub' && langChanged)
    if (facetChanged && !roomPinned.value) {
      void repickProviderForFacet()
      return
    }
    void resolveStreamForCurrentEpisode()
```

  (b) Add the `import { langForProviderUnderRaw } from '@/composables/aePlayer/providerGroups'` and `import { pickRawBiased, pickSelectableFallback } from '@/composables/aePlayer/smartDefault'` to the existing imports (merge with the current `pickSmartDefault` import line).

  (c) Add a watcher right after the `rows`/smart-default watchers (after line ~848) to sync `combo.lang` to the selected provider under RAW:

```ts
// Under RAW (audio:'sub') the language slider is hidden — derive combo.lang from
// the chosen provider's group so persistence + the subtitle menu stay correct.
// setServedLang preserves team; the facet watcher ignores RAW lang changes so
// this never churns the source.
watch(
  () => state.combo.value.provider,
  (id) => {
    if (!id || state.combo.value.audio !== 'sub') return
    const row = rows.value.find((r) => r.id === id)
    if (!row) return
    const want = langForProviderUnderRaw(row.group, state.combo.value.lang)
    if (want !== state.combo.value.lang) state.setServedLang(want)
  },
)
```

  (d) Use the biased pick + dead-player fallback at the two auto-pick sites. In the smart-default watcher (line ~840) replace `const pick = pickSmartDefault(rows.value)` with:

```ts
    const facetPick =
      state.combo.value.audio === 'sub'
        ? pickRawBiased(rows.value, state.combo.value.lang)
        : pickSmartDefault(rows.value)
    const pick = facetPick ?? pickSelectableFallback(rows.value)
```

  And in `repickProviderForFacet()` (line ~1386) replace `const pick = pickSmartDefault(rows.value)` with the same two lines (compute `facetPick` then `const pick = facetPick ?? pickSelectableFallback(rows.value)`).

- [ ] **Step 7: Run the full aePlayer composable + integration specs.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer src/components/player/aePlayer`
Expected: PASS (fix any integration spec that asserted the old SUB/lang-gated selection; the new RAW model lists cross-language sources).

- [ ] **Step 8: Commit.**

```bash
git add frontend/web/src/composables/aePlayer/usePlayerState.ts frontend/web/src/composables/aePlayer/smartDefault.ts frontend/web/src/composables/aePlayer/smartDefault.spec.ts frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(aeplayer): RAW lang follows provider + language-biased default + dead-player fallback

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: Respect saved watch combo + URL params on open

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/views/Anime.vue`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.urlsync.spec.ts`

**Interfaces:**
- Consumes: `report` (raw `CapabilityReport`), `GROUP_LANGS`, `providerToLegacyPlayer`.
- Produces: `AePlayer` props `initialAudio?: string`, `initialLang?: string`; `buildAvailable()` enumerates cross-facet combos from the full report.

- [ ] **Step 1: Add a failing test** to `AePlayer.urlsync.spec.ts` (read it for the mount/report fixtures). Add a case asserting a saved `dub/ru` combo (or `?audio=dub&lang=ru`) is restored, not collapsed to SUB EN. Sketch (adapt to the spec's harness):

```ts
it('restores a saved dub/ru combo instead of collapsing to SUB EN', async () => {
  // mount with a report that has a dub/ru source (e.g. kodik) and a tier1 saved combo {player:'kodik',language:'ru',watch_type:'dub'}
  const w = await mountAePlayer({ tier1Combo: { player: 'kodik', language: 'ru', watch_type: 'dub' } })
  await flush()
  expect(w.vm.__combo.value.audio).toBe('dub')
  expect(w.vm.__combo.value.lang).toBe('ru')
})

it('honors ?audio/?lang props when no saved combo', async () => {
  const w = await mountAePlayer({ initialAudio: 'dub', initialLang: 'ru' })
  await flush()
  expect(w.vm.__combo.value.audio).toBe('dub')
  expect(w.vm.__combo.value.lang).toBe('ru')
})
```

(`__combo` is exposed via `defineExpose` at `AePlayer.vue:570`. If the spec lacks a `mountAePlayer` helper with a configurable report/tier1, extend the existing harness minimally.)

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.urlsync.spec.ts`
Expected: FAIL (combo collapses to sub/en; `initialAudio`/`initialLang` props don't exist).

- [ ] **Step 3: Rebuild `buildAvailable()`** in `AePlayer.vue` to enumerate from the full report (replace the current `rows.value`-based body, lines ~790-812):

```ts
// Enumerate EVERY real source's facet (across all families, not just the current
// audio/lang filter) so a saved/URL combo in any language/audio can be matched —
// otherwise the facet-filtered rows only ever offer the default RAW/EN options
// and every restore collapses to it.
const buildAvailable = (): WatchCombo[] => {
  const combos: WatchCombo[] = []
  const seen = new Set<string>()
  const rep = report.value
  if (!rep || !Array.isArray(rep.families)) return combos
  for (const fam of rep.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.state === 'no_content') continue
      const player = providerToLegacyPlayer(cap.provider)
      if (!player) continue
      const langs = GROUP_LANGS[cap.group]
      const audios = [...new Set((cap.audios ?? []).map((a) => (a === 'dub' ? 'dub' : 'sub')))]
      for (const audio of audios) {
        for (const language of langs) {
          const key = `${player}:${audio}:${language}`
          if (seen.has(key)) continue
          seen.add(key)
          combos.push({
            player,
            language: language as WatchCombo['language'],
            watch_type: audio as WatchCombo['watch_type'],
            translation_id: '',
            translation_title: '',
          })
        }
      }
    }
  }
  return combos
}
```

(`GROUP_LANGS` is imported in Task 5 step 6b; ensure `providerToLegacyPlayer` is imported from `@/composables/aePlayer/comboMapping` — it already is, used elsewhere.)

- [ ] **Step 4: Add `initialAudio`/`initialLang` props + apply them.** In `defineProps`, add `initialAudio?: string` and `initialLang?: string`. Add an apply function next to `applyInitialProvider` (after line ~787):

```ts
// URL facet override (?audio=raw|sub|dub, ?lang=en|ru|ja) — applied after the
// saved combo, before the ?provider clamp. Precedence: URL > saved > smart default.
function applyUrlFacet() {
  if (state.combo.value.provider) return
  const a = props.initialAudio
  if (a === 'dub') state.setAudio('dub')
  else if (a === 'raw' || a === 'sub') state.setAudio('sub')
  const l = props.initialLang
  if (l === 'en' || l === 'ru' || l === 'ja') state.setLang(l)
}
```

In the `resolvePreference(available).finally(() => {...})` block (line ~825), insert the call between `applyResolvedCombo()` and `applyInitialProvider()`:

```ts
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    applyUrlFacet()
    applyInitialProvider()
    preferenceSettled.value = true
  })
```

- [ ] **Step 5: Pass the params from `Anime.vue`.** Add query computeds near the existing `queryProvider`/`queryTeam` (Anime.vue ~1213):

```ts
const queryAudio = computed<string | undefined>(() => {
  const v = route.query.audio
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})
const queryLang = computed<string | undefined>(() => {
  const v = route.query.lang
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' && s !== '' ? s : undefined
})
```

And pass them on the `<AePlayer>` tag (near `:initial-provider`):

```html
  :initial-audio="queryAudio"
  :initial-lang="queryLang"
```

- [ ] **Step 6: Run the urlsync spec + a quick typecheck of the two files.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.urlsync.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/views/Anime.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.urlsync.spec.ts
git commit -m "fix(aeplayer): respect saved watch combo + ?audio/?lang on open (no more forced SUB EN)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: Subtitles OFF by default

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Test: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts`

**Interfaces:**
- Produces: `pickAutoSubtitle(...)` always returns `null` (no auto-enable).

- [ ] **Step 1: Update the failing tests** in `pickDefaultSubtitle.spec.ts`. Replace the `pickAutoSubtitle` describe block's expectations so every case (bundled present, raw JP, EN/RU) asserts `null`:

```ts
describe('pickAutoSubtitle', () => {
  const ja = { url: 'j', provider: 'jimaku', lang: 'ja', label: 'JP', format: 'srt' }
  const bundled = { url: 'b', provider: 'gogoanime', lang: 'en', label: 'EN', format: 'vtt' }
  it('never auto-enables — subtitles default off', () => {
    expect(pickAutoSubtitle({ lang: 'ja', bundled: [], aggregated: [ja] })).toBeNull()
    expect(pickAutoSubtitle({ lang: 'en', bundled: [bundled], aggregated: [] })).toBeNull()
    expect(pickAutoSubtitle({ lang: 'en', bundled: [], aggregated: [] })).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts`
Expected: FAIL (bundled + raw JP currently return a track).

- [ ] **Step 3: Neuter `pickAutoSubtitle`** in `pickDefaultSubtitle.ts`. Replace its body with a hard `null` and update the doc comment:

```ts
/**
 * aePlayer subtitles default OFF, with no exceptions (incl. pure raw-JP). The
 * player never auto-enables an overlay — the user opts in via the Subtitles menu.
 * Kept as a stable export so call sites/specs don't churn.
 */
export function pickAutoSubtitle(_opts: {
  lang: string
  bundled: SubTrack[]
  aggregated: SubTrack[]
}): SubTrack | null {
  return null
}
```

- [ ] **Step 4: Remove the auto-enable effect** in `AePlayer.vue`. Delete the `autoSelectSubtitle()` function (lines ~2121-2136) and drop its invocation from the `watch([currentStream, audio], ...)` (keep `ensureSubsLoaded()` so the menu still populates) and remove the standalone `watch(subtitleTracks, () => autoSelectSubtitle())` (Task 8 replaces it). Resulting stream watcher:

```ts
// Eager-load the subtitle aggregation when a RAW (sub) stream resolves so the
// menu's language list is ready — but DO NOT auto-enable (subs default off).
watch(
  [currentStream, () => state.combo.value.audio],
  async () => {
    if (!currentStream.value || state.combo.value.audio !== 'sub') return
    await ensureSubsLoaded()
  },
)
```

Also remove the now-unused `let subUserDecided = false` and its assignments in `onSelectSubTrack`/`onSubtitlesOff` (they were only read by the deleted auto-select). Keep `import { pickAutoSubtitle } ...` only if still referenced; otherwise drop it to satisfy `vue-tsc`/eslint no-unused.

- [ ] **Step 5: Run the subtitle specs.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`
Expected: PASS (update any subtitles-spec case that asserted auto-on for raw/JP → now off).

- [ ] **Step 6: Commit.**

```bash
git add frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts
git commit -m "fix(aeplayer): subtitles default OFF (no auto-enable, incl. raw-JP)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 8: Subtitles persist across episode change + CC icon reflects enabled state

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/components/player/aePlayer/PlayerControlBar.vue`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts`, `frontend/web/src/components/player/aePlayer/PlayerControlBar.spec.ts`

**Interfaces:**
- Consumes: `state.subLang`, `chosenSub`, `pickBestForLang`, `subtitleTracks`, `subEpisode`.
- Produces: `PlayerControlBar` prop `subsOn?: boolean`; CC button shows an enabled affordance.

- [ ] **Step 1: Add failing tests.**

  In `AePlayer.subtitles.spec.ts` (adapt to the harness): after the user enables RU subs, changing episode keeps subs enabled and re-binds the RU track for the new episode (assert `state.subLang === 'ru'` persists and `chosenSub` is the new episode's RU track once tracks load).

  In `PlayerControlBar.spec.ts`:

```ts
it('CC button shows the enabled affordance when subsOn', () => {
  const on = mount(PlayerControlBar, { props: { ...base, subsOn: true } as any, ...mountOpts })
  expect(on.find('[data-test="toggle-subs"]').attributes('aria-pressed')).toBe('true')

  const off = mount(PlayerControlBar, { props: { ...base, subsOn: false } as any, ...mountOpts })
  expect(off.find('[data-test="toggle-subs"]').attributes('aria-pressed')).toBe('false')
})
```

- [ ] **Step 2: Run to verify failure.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts src/components/player/aePlayer/PlayerControlBar.spec.ts`
Expected: FAIL (episode change nulls the pick; `subsOn`/`aria-pressed` absent).

- [ ] **Step 3: Persist subtitle decision across episodes** in `AePlayer.vue`. Replace the episode-reset watcher (lines ~2141-2144) and add the re-bind watcher:

```ts
// A NEW episode drops the stale track URL but KEEPS the user's subtitle choice
// (state.subLang). The re-bind watcher below re-resolves a track in that language
// from the new episode's list once it loads. Subs default off → stays off here.
watch(subEpisode, () => { chosenSub.value = null })

// Re-bind the chosen track to the persisted subtitle language whenever the track
// list changes (new episode or late arrival). Off stays off — no auto-enable.
watch(subtitleTracks, () => {
  const lang = state.subLang.value
  if (lang === 'off') return
  const t = pickBestForLang(subtitleTracks.value, lang)
  if (t) chosenSub.value = t
})
```

- [ ] **Step 4: Compute `subsOn` and pass it to the control bar.** Add near the subtitle computeds (~line 2098):

```ts
// Whether a subtitle overlay is actually rendering — drives the CC button's
// enabled affordance (distinct from the menu-open highlight).
const subsOn = computed(() => state.subLang.value !== 'off' && !!chosenSubUrl.value)
```

On the `<PlayerControlBar ... />` tag (line ~183), add the prop:

```html
      :subs-on="subsOn"
```

- [ ] **Step 5: Render the enabled affordance** in `PlayerControlBar.vue`. Add `subsOn?: boolean` to `defineProps`, and update the CC `PlayerIconButton` (lines ~134-142):

```html
      <!-- Subtitles (CC) -->
      <PlayerIconButton
        :active="openMenu === 'subs'"
        aria-label="Subtitles"
        :aria-expanded="openMenu === 'subs'"
        :aria-pressed="subsOn === true"
        data-test="toggle-subs"
        @click="emit('toggle-subs')"
      >
        <Captions class="size-5" :class="subsOn ? 'text-[var(--brand-cyan)]' : ''" aria-hidden="true" />
      </PlayerIconButton>
```

- [ ] **Step 6: Run the specs.**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts src/components/player/aePlayer/PlayerControlBar.spec.ts`
Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/PlayerControlBar.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.subtitles.spec.ts frontend/web/src/components/player/aePlayer/PlayerControlBar.spec.ts
git commit -m "feat(aeplayer): persist subtitle choice across episodes + CC icon reflects enabled state

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 9: Full FE verification gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full aePlayer test surface.**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer src/components/player/aePlayer src/locales/__tests__`
Expected: PASS. Fix any spec that still asserts the pre-change SUB/lang model.

- [ ] **Step 2: Typecheck.**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no errors. (vitest does NOT type-check — this is the real gate; watch for unused-import errors from Task 7.)

- [ ] **Step 3: DS-lint.**

Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0`.

- [ ] **Step 4: Real production build.**

Run: `cd frontend/web && bun run build`
Expected: build succeeds (catches Tailwind-v4 cascade + TS2614 traps vitest/jsdom miss).

- [ ] **Step 5: Invoke `/frontend-verify`** for the consolidated DS-lint + i18n parity + build + trap checks, then proceed to `/animeenigma-after-update` (simplify → lint/build → redeploy-web → health → changelog (Russian Trump-mode) → commit/push) per project workflow. No separate commit here.

---

## Self-Review

**Spec coverage:**
- F1 RAW/DUB slider + conditional RU/EN → Task 3. ✓
- F2 JA reachable / RAW all original sources → Task 2. ✓
- F3 respect saved combo + URL (buildAvailable full report + ?audio/?lang) → Task 6. ✓
- F4 top-3 fallback → Task 4 (view) + Task 5 (dead-player auto-pick). ✓
- F5 subtitles off by default → Task 7. ✓
- F6 subtitle persistence across episodes → Task 8. ✓
- F7 CC icon enabled state → Task 8. ✓
- F8 i18n RAW/DUB labels → Task 1 (+ used in Task 3). ✓
- Invariants I1/I2 → Task 2; I3 → Task 5 step 6a; I4 → Task 5 (setServedLang + watcher); I5 (DUB clamps JA) → see note below.
- Edge: language bias → Task 5 (`pickRawBiased`). ✓

**I5 (DUB clamps lang when switching RAW→DUB while lang='ja'):** the DUB language slider only offers EN/RU, but `combo.lang` could be `'ja'` after a RAW pick on a JP source. Under DUB, `relevant()` requires `GROUP_LANGS[g].includes(f.lang)` — with `lang='ja'` and no `jp`-group dub provider, rows would be empty → `pickSelectableFallback` still returns null and the dead-player guard shows nothing. **Add to Task 3 (or Task 5):** when the audio slider switches to `'dub'` and `combo.lang === 'ja'`, clamp to `'en'`. Implement in `AePlayer.vue` by wrapping the `@update:audio` handler: replace `@update:audio="state.setAudio"` with `@update:audio="onSelectAudio"` and add:

```ts
function onSelectAudio(a: AudioKind) {
  state.setAudio(a)
  if (a === 'dub' && state.combo.value.lang === 'ja') state.setLang('en')
}
```

This is folded into **Task 3 step 5** (do it alongside the `audioLabel` change). Coverage closed.

**Placeholder scan:** no TBD/TODO; every code step shows code. ✓

**Type consistency:** `setServedLang(l: TrackLang)`, `pickRawBiased(rows, lang)`, `pickSelectableFallback(rows)`, `forced?: boolean`, `subsOn?: boolean`, `initialAudio?/initialLang?: string` are defined where introduced and consumed with matching names. ✓
