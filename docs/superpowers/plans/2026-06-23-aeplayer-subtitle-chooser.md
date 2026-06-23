# aePlayer subtitle chooser rework — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the aePlayer's broken/confusing subtitle controls with a YouTube-like quick chooser (`[provider] [Off] [RU] [EN] [JP]`) and a working Browse menu scoped to RU/EN/JP + "More languages".

**Architecture:** Frontend-only. A pure selector helper (`pickBestForLang`) picks the best track per language by provider rank. `SubtitlesMenu.vue` becomes a fixed YouTube-style row driven by real available-language data + a provider chip. `BrowseSubsModal.vue` defaults to RU/EN/JP groups with a "More languages" reveal. `AePlayer.vue` wires the real merged track list into both, fixes auto-select to prefer provider-bundled subs, and closes the floating menu when Browse opens (the interactivity bug).

**Tech Stack:** Vue 3 `<script setup>` + TypeScript, Vitest + `@vue/test-utils`, vue-i18n, Tailwind v4 + Neon-Tokyo design tokens.

## Global Constraints

- **Worktree only.** All work happens in this worktree (`.claude/worktrees/aeplayer-subs-chooser`); never edit the `/data/animeenigma` base tree. Commit with the three co-authors (see footer of each commit step).
- **DS-lint is build-enforced.** No off-palette Tailwind colors, no raw hex/rgba/hsl, no deprecated `var()` aliases, only `font-medium`/`font-semibold`, no NEW arbitrary spacing classes (reuse already-allowlisted `px-[9px]`/`py-[9px]`/`gap-[5px]` for `SubtitlesMenu.vue` and the existing `BrowseSubsModal.vue` set, or token-scale values). `bg-white/[0.0x]` opacity utilities are allowed (already used).
- **i18n parity gate.** Every new key MUST exist in all three of `en.json`, `ru.json`, `ja.json` with identical ICU placeholders, or the build fails.
- **Vue rule:** no non-conditional element between `v-if`/`v-else-if` branches.
- **Lang codes:** RU=`ru`, EN=`en`, JP=`ja`. Fast-button labels are bare codes (`RU`/`EN`/`JP`), not translated.
- **vue-tsc:** named TYPE imports from `.vue` files break the build (TS2614). Alias `.ts` types instead (the codebase already follows this — see `useSubtitleTracks.ts`).
- **Final verification:** `/frontend-verify` must pass before handoff to `/animeenigma-after-update`.

---

### Task 1: `pickBestForLang` selector

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts`
- Test: `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts` (create)

**Interfaces:**
- Consumes: `SubtitleTrack` type from `@/types/aePlayer` (existing).
- Produces: `export function pickBestForLang(tracks: SubTrack[], lang: string): SubTrack | null` — best track whose `lang === lang` by provider rank (jimaku < provider-own < opensubtitles), or `null` if no track matches that lang.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { pickDefaultSubtitle, pickBestForLang } from './pickDefaultSubtitle'

const tracks = [
  { url: 'en-os', provider: 'opensubtitles', lang: 'en', label: 'EN OpenSubs', format: 'srt' },
  { url: 'en-own', provider: 'gogoanime', lang: 'en', label: 'EN provider', format: 'vtt' },
  { url: 'ja-ji', provider: 'jimaku', lang: 'ja', label: 'JA Jimaku', format: 'ass' },
]

describe('pickBestForLang', () => {
  it('returns null when no track matches the language', () => {
    expect(pickBestForLang(tracks, 'ru')).toBeNull()
  })
  it('prefers provider-own over opensubtitles for the same language', () => {
    expect(pickBestForLang(tracks, 'en')?.url).toBe('en-own')
  })
  it('returns the only match for a language', () => {
    expect(pickBestForLang(tracks, 'ja')?.url).toBe('ja-ji')
  })
  it('does NOT fall back to another language (unlike pickDefaultSubtitle)', () => {
    expect(pickBestForLang([tracks[2]], 'en')).toBeNull()
    expect(pickDefaultSubtitle([tracks[2]], { lang: 'en' })?.url).toBe('ja-ji')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts`
Expected: FAIL — `pickBestForLang is not a function` / not exported.

- [ ] **Step 3: Add the selector**

Edit `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts`. Make `best` reusable and export the new function. The full file becomes:

```ts
import type { SubtitleTrack as SubTrack } from '@/types/aePlayer'

function providerRank(provider: string): number {
  if (provider === 'jimaku') return 0
  if (provider === 'opensubtitles') return 2
  return 1 // provider-own (gogoanime, etc.)
}

function best(tracks: SubTrack[]): SubTrack | null {
  if (tracks.length === 0) return null
  return [...tracks].sort((a, b) => providerRank(a.provider) - providerRank(b.provider))[0]
}

export function pickDefaultSubtitle(tracks: SubTrack[], opts: { lang: string }): SubTrack | null {
  if (tracks.length === 0) return null
  const matches = tracks.filter((t) => t.lang === opts.lang)
  return best(matches) ?? best(tracks)
}

// Best track for EXACTLY this language (no cross-language fallback). Used by the
// quick chooser's RU/EN/JP fast buttons.
export function pickBestForLang(tracks: SubTrack[], lang: string): SubTrack | null {
  return best(tracks.filter((t) => t.lang === lang))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/pickDefaultSubtitle.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts frontend/web/src/composables/aePlayer/pickDefaultSubtitle.spec.ts
git commit -m "feat(aePlayer): add pickBestForLang subtitle selector

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: SubtitlesMenu — fixed Off/RU/EN/JP row + provider chip

**Files:**
- Modify (full rewrite): `frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue`
- Test: `frontend/web/src/components/player/aePlayer/SubtitlesMenu.spec.ts` (create)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Produces (props the parent in Task 4 must supply):
  - `subLang: string` — active lang or `'off'`
  - `availableSubLangs: string[]` — real distinct langs with a track
  - `providerChip: { provider: string } | null` — bundled-subs provider (null hides chip)
  - `providerActive: boolean` — is the active sub the bundled track
  - `hardsubNote?: string | null`, `subSize/subBg/subOffset: number`
- Produces (events): `pick-lang` (payload `'off' | 'ru' | 'en' | 'ja'`), `select-provider`, `update:subSize`, `update:subBg`, `update:subOffset`, `open-browse`.

- [ ] **Step 1: Add i18n keys (all three locales)**

In `frontend/web/src/locales/en.json`, inside `player.aePlayer.subs`, add:
```json
"noTrack": "No subtitles for this language",
"moreLanguages": "More languages ({count})",
"fromProvider": "Subtitles from {provider}"
```
In `ru.json` `player.aePlayer.subs`:
```json
"noTrack": "Нет субтитров для этого языка",
"moreLanguages": "Ещё языки ({count})",
"fromProvider": "Субтитры от {provider}"
```
In `ja.json` `player.aePlayer.subs`:
```json
"noTrack": "この言語の字幕はありません",
"moreLanguages": "他の言語 ({count})",
"fromProvider": "{provider}の字幕"
```
(`moreLanguages` is used in Task 3, added here so all keys land in one parity-safe edit.)

- [ ] **Step 2: Write the failing test**

Create `frontend/web/src/components/player/aePlayer/SubtitlesMenu.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SubtitlesMenu from './SubtitlesMenu.vue'

const base = {
  subLang: 'off',
  availableSubLangs: ['en', 'ja'],
  providerChip: null as { provider: string } | null,
  providerActive: false,
  subSize: 100,
  subBg: 50,
  subOffset: 0,
}

const stubs = { Stepper: true }

describe('SubtitlesMenu', () => {
  it('renders fixed Off/RU/EN/JP fast buttons', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    const labels = w.findAll('[data-test="fast-lang"]').map((b) => b.text())
    expect(labels).toEqual(['RU', 'EN', 'JP'])
    expect(w.find('[data-test="subs-off"]').exists()).toBe(true)
  })

  it('disables a language with no available track (RU here)', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    const ru = w.find('[data-test="fast-lang"][data-lang="ru"]')
    expect(ru.attributes('disabled')).toBeDefined()
    const en = w.find('[data-test="fast-lang"][data-lang="en"]')
    expect(en.attributes('disabled')).toBeUndefined()
  })

  it('emits pick-lang with the language code when an enabled button is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="fast-lang"][data-lang="en"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['en'])
  })

  it('emits pick-lang off when Off is clicked', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="subs-off"]').trigger('click')
    expect(w.emitted('pick-lang')?.[0]).toEqual(['off'])
  })

  it('hides the provider chip when there are no bundled subs', () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    expect(w.find('[data-test="sub-provider-chip"]').exists()).toBe(false)
  })

  it('shows the provider chip and emits select-provider when bundled subs exist', async () => {
    const w = mount(SubtitlesMenu, {
      props: { ...base, providerChip: { provider: 'gogoanime' } },
      global: { stubs },
    })
    const chip = w.find('[data-test="sub-provider-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('gogoanime')
    await chip.trigger('click')
    expect(w.emitted('select-provider')).toBeTruthy()
  })

  it('highlights the provider chip (not the lang button) when providerActive', () => {
    const w = mount(SubtitlesMenu, {
      props: { ...base, subLang: 'en', providerChip: { provider: 'gogoanime' }, providerActive: true },
      global: { stubs },
    })
    expect(w.find('[data-test="sub-provider-chip"]').classes().join(' ')).toContain('text-[var(--brand-cyan)]')
    expect(w.find('[data-test="fast-lang"][data-lang="en"]').classes().join(' ')).not.toContain('text-[var(--brand-cyan)]')
  })

  it('emits open-browse', async () => {
    const w = mount(SubtitlesMenu, { props: { ...base }, global: { stubs } })
    await w.find('[data-test="open-browse"]').trigger('click')
    expect(w.emitted('open-browse')).toBeTruthy()
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SubtitlesMenu.spec.ts`
Expected: FAIL — selectors like `[data-test="fast-lang"]` not found (old template).

- [ ] **Step 4: Rewrite the component**

Replace the entire contents of `frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue` with:

```vue
<template>
  <div class="flex flex-col min-w-[264px]">
    <!-- Header -->
    <div class="px-2.5 pt-2 pb-1">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitles
      </span>
    </div>

    <!-- Fast chooser: [provider] Off RU EN JP -->
    <div class="flex flex-wrap items-center gap-[5px] px-2.5 py-[9px]">
      <!-- Provider chip (bundled subs from the resolved stream) -->
      <button
        v-if="providerChip"
        data-test="sub-provider-chip"
        :title="$t('player.aePlayer.subs.fromProvider', { provider: providerChip.provider })"
        :class="pillClass(providerActive)"
        :style="providerActive ? 'background: var(--cyan-a20)' : ''"
        @click="emit('select-provider')"
      >
        {{ providerChip.provider }}
      </button>

      <!-- Off -->
      <button
        data-test="subs-off"
        :class="pillClass(subLang === 'off')"
        :style="subLang === 'off' ? 'background: var(--cyan-a20)' : ''"
        @click="emit('pick-lang', 'off')"
      >
        {{ $t('player.aePlayer.subs.off') }}
      </button>

      <!-- Fixed RU / EN / JP fast buttons -->
      <button
        v-for="lang in FAST_LANGS"
        :key="lang"
        data-test="fast-lang"
        :data-lang="lang"
        :disabled="!availableSubLangs.includes(lang)"
        :title="availableSubLangs.includes(lang) ? undefined : $t('player.aePlayer.subs.noTrack')"
        :class="[
          pillClass(!providerActive && subLang === lang),
          availableSubLangs.includes(lang) ? '' : 'opacity-40 cursor-not-allowed',
        ]"
        :style="(!providerActive && subLang === lang) ? 'background: var(--cyan-a20)' : ''"
        @click="onFast(lang)"
      >
        {{ FAST_LABELS[lang] }}
      </button>
    </div>

    <!-- Hardsub note: no soft tracks at all, subs are burned in by the provider -->
    <div
      v-if="hardsubNote && availableSubLangs.length === 0"
      class="px-2.5 pb-1.5 text-[11px] leading-snug text-[var(--muted-foreground)]"
      data-test="hardsub-note"
    >
      {{ hardsubNote }}
    </div>

    <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

    <!-- Subtitle settings sub-section -->
    <div class="px-2.5 pb-1 pt-0.5">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitle settings
      </span>
    </div>

    <!-- Text size -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Text size</label>
      <input
        type="range"
        :value="subSize"
        min="50"
        max="200"
        step="5"
        class="flex-1"
        style="accent-color: var(--brand-cyan);"
        @input="emit('update:subSize', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="text-[12px] text-[var(--muted-foreground)] w-8 text-right flex-shrink-0">
        {{ subSize }}%
      </span>
    </div>

    <!-- Background -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Background</label>
      <input
        type="range"
        :value="subBg"
        min="0"
        max="100"
        step="5"
        class="flex-1"
        style="accent-color: var(--brand-cyan);"
        @input="emit('update:subBg', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="text-[12px] text-[var(--muted-foreground)] w-8 text-right flex-shrink-0">
        {{ subBg }}%
      </span>
    </div>

    <!-- Timing offset (DS Stepper primitive) -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Timing</label>
      <div class="flex flex-col items-end gap-1 ml-auto">
        <Stepper
          :model-value="subOffset"
          :step="0.1"
          suffix="s"
          label="offset"
          @update:model-value="emit('update:subOffset', $event)"
        />
        <span class="text-[11px] text-[var(--muted-foreground)]">
          {{ offsetHint }}
          <button
            v-if="subOffset !== 0"
            class="ml-1 text-[var(--brand-cyan)] bg-transparent border-0 text-[11px] underline p-0 cursor-pointer"
            @click="emit('update:subOffset', 0)"
          >Reset</button>
        </span>
      </div>
    </div>

    <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

    <!-- Browse all subtitles -->
    <button
      data-test="open-browse"
      class="w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-left transition-colors hover:bg-white/[0.08] hover:text-white text-[var(--brand-cyan)]"
      @click="emit('open-browse')"
    >
      <List class="size-4 flex-shrink-0" aria-hidden="true" />
      <span class="flex-1 text-[var(--brand-cyan)]">Browse all subtitles</span>
      <ChevronRight class="size-3" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { List, ChevronRight } from 'lucide-vue-next'
import Stepper from '@/components/ui/Stepper.vue'

const { t: $t } = useI18n()

const props = defineProps<{
  subLang: string
  /** Real distinct languages that have a loaded track. */
  availableSubLangs: string[]
  /** Bundled-subs provider from the resolved stream; null hides the chip. */
  providerChip: { provider: string } | null
  /** True when the active subtitle is the provider's bundled track. */
  providerActive: boolean
  /** Shown only when there are NO soft tracks but the stream has burned-in subs. */
  hardsubNote?: string | null
  subSize: number
  subBg: number
  subOffset: number
}>()

const emit = defineEmits<{
  (e: 'pick-lang', value: string): void
  (e: 'select-provider'): void
  (e: 'update:subSize', value: number): void
  (e: 'update:subBg', value: number): void
  (e: 'update:subOffset', value: number): void
  (e: 'open-browse'): void
}>()

const FAST_LANGS = ['ru', 'en', 'ja'] as const
const FAST_LABELS: Record<string, string> = { ru: 'RU', en: 'EN', ja: 'JP' }

function pillClass(active: boolean): string[] {
  return [
    'px-[9px] py-1 rounded-full text-[12px] font-medium border transition-all',
    active
      ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
      : 'bg-white/[0.08] border-transparent text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white',
  ]
}

function onFast(lang: string) {
  if (!props.availableSubLangs.includes(lang)) return
  emit('pick-lang', lang)
}

const offsetHint = computed(() => {
  const v = props.subOffset
  if (v === 0) return 'In sync'
  const abs = Math.abs(v).toFixed(1)
  return v > 0 ? `${abs}s later` : `${abs}s earlier`
})
</script>
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SubtitlesMenu.spec.ts`
Expected: PASS (8 tests).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue frontend/web/src/components/player/aePlayer/SubtitlesMenu.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(aePlayer): YouTube-like fixed subtitle quick chooser + provider chip

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: BrowseSubsModal — RU/EN/JP default + "More languages"

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue`
- Modify: `frontend/web/src/components/player/aePlayer/BrowseSubsModal.spec.ts`

**Interfaces:**
- Consumes: `subs.moreLanguages` i18n key (added in Task 2).
- Produces: no prop/event signature changes (still `tracks`, `selectedUrl`, `loading`, `error`, `providersDown`; emits `select`/`close`/`retry`/`off`). Internal `showAllLangs` ref + `PRIMARY_LANGS` scoping.

- [ ] **Step 1: Write the failing tests (append to existing spec)**

Add these cases inside the `describe('BrowseSubsModal', ...)` block in `frontend/web/src/components/player/aePlayer/BrowseSubsModal.spec.ts`:

```ts
  it('hides non RU/EN/JP language groups by default', () => {
    const mixed = [
      { url: 'en1', provider: 'opensubtitles', lang: 'en', label: 'EN', format: 'srt' },
      { url: 'de1', provider: 'opensubtitles', lang: 'de', label: 'DE', format: 'srt' },
    ]
    const w = mount(BrowseSubsModal, { props: { tracks: mixed, selectedUrl: null } })
    const langs = w.findAll('[data-test="lang-group"]').map((g) => g.find('h3').text())
    expect(langs.some((l) => l.startsWith('EN'))).toBe(true)
    expect(langs.some((l) => l.startsWith('DE'))).toBe(false)
    expect(w.find('[data-test="more-languages"]').exists()).toBe(true)
  })

  it('reveals other-language groups when "More languages" is clicked', async () => {
    const mixed = [
      { url: 'en1', provider: 'opensubtitles', lang: 'en', label: 'EN', format: 'srt' },
      { url: 'de1', provider: 'opensubtitles', lang: 'de', label: 'DE', format: 'srt' },
    ]
    const w = mount(BrowseSubsModal, { props: { tracks: mixed, selectedUrl: null } })
    await w.find('[data-test="more-languages"]').trigger('click')
    const langs = w.findAll('[data-test="lang-group"]').map((g) => g.find('h3').text())
    expect(langs.some((l) => l.startsWith('DE'))).toBe(true)
  })

  it('does not show "More languages" when only RU/EN/JP exist', () => {
    const w = mount(BrowseSubsModal, { props: { tracks, selectedUrl: null } })
    expect(w.find('[data-test="more-languages"]').exists()).toBe(false)
  })
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/BrowseSubsModal.spec.ts`
Expected: FAIL — `[data-test="more-languages"]` not found; DE group still rendered.

- [ ] **Step 3: Add scoping state to the script**

In `BrowseSubsModal.vue` `<script setup>`, after the `activeLang` ref (line ~250), add:

```ts
const PRIMARY_LANGS = ['ru', 'en', 'ja']
const showAllLangs = ref(false)

const otherLangCount = computed(
  () => new Set(props.tracks.filter((t) => !PRIMARY_LANGS.includes(t.lang)).map((t) => t.lang)).size,
)

const visibleLangChips = computed(() =>
  showAllLangs.value ? distinctLangs.value : distinctLangs.value.filter((l) => PRIMARY_LANGS.includes(l)),
)
```

Then change `groupedTracks` so it only groups in-scope tracks. Replace the `groupedTracks` computed body's source from `filteredTracks.value` to a scoped list — update it to:

```ts
const groupedTracks = computed<TrackGroup[]>(() => {
  const inScope = filteredTracks.value.filter(
    (t) => showAllLangs.value || PRIMARY_LANGS.includes(t.lang) || t.lang === activeLang.value,
  )
  const map = new Map<string, SubTrack[]>()
  for (const track of inScope) {
    const existing = map.get(track.lang)
    if (existing) {
      existing.push(track)
    } else {
      map.set(track.lang, [track])
    }
  }
  return [...map.entries()].map(([lang, tracks]) => ({ lang, tracks }))
})
```

- [ ] **Step 4: Wire the template — constrain chips + add "More languages"**

In the language-chips block (currently `v-for="lang in distinctLangs"`), change the loop source to `visibleLangChips` and add a "More languages" chip after the loop. Replace the whole `<!-- Language chips -->` block with:

```vue
        <!-- Language chips -->
        <div v-if="distinctLangs.length > 1" class="flex flex-wrap items-center gap-[7px]">
          <span class="text-[11px] uppercase tracking-[0.05em] text-[var(--muted-foreground)] mr-0.5">Lang</span>
          <button
            v-for="lang in visibleLangChips"
            :key="lang"
            :class="[
              'px-3 py-[5px] rounded-full text-[13px] border transition-all',
              activeLang === lang
                ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
                : 'bg-white/[0.06] border-transparent text-[var(--ink-2)] hover:bg-white/[0.12] hover:text-white',
            ]"
            :style="activeLang === lang ? 'background: var(--cyan-a20)' : ''"
            @click="activeLang = activeLang === lang ? null : lang"
          >
            {{ lang.toUpperCase() }}
          </button>
          <button
            v-if="!showAllLangs && otherLangCount > 0"
            data-test="more-languages"
            class="px-3 py-[5px] rounded-full text-[13px] border border-transparent bg-white/[0.06] text-[var(--ink-2)] hover:bg-white/[0.12] hover:text-white transition-all"
            @click="showAllLangs = true"
          >
            {{ $t('player.aePlayer.subs.moreLanguages', { count: otherLangCount }) }}
          </button>
        </div>
```

Then add a bottom-of-list affordance for discoverability when the chip row is hidden (only one total language but others exist is impossible; still keep it inside the groups `<template>`). After the `</div>` that closes the `v-for="group in groupedTracks"` loop (just before the closing `</template>` of the body), add:

```vue
          <!-- More languages (bottom affordance) -->
          <button
            v-if="!showAllLangs && otherLangCount > 0 && distinctLangs.length <= 1"
            data-test="more-languages"
            class="w-full mt-1 px-3 py-[7px] rounded-[var(--r-md)] border border-transparent bg-white/[0.05] text-[14px] text-[var(--ink-2)] hover:bg-white/[0.1] hover:text-white transition-all text-left"
            @click="showAllLangs = true"
          >
            {{ $t('player.aePlayer.subs.moreLanguages', { count: otherLangCount }) }}
          </button>
```

> NOTE: there are two `data-test="more-languages"` controls but they are mutually exclusive by their `v-if` guards (`distinctLangs.length > 1` vs `<= 1`), so at most one renders. The Task-3 tests use `mixed` (2 distinct langs → chip-row variant).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/BrowseSubsModal.spec.ts`
Expected: PASS (all original + 3 new).

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue frontend/web/src/components/player/aePlayer/BrowseSubsModal.spec.ts
git commit -m "feat(aePlayer): scope Browse Subtitles to RU/EN/JP with More-languages reveal

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Wire AePlayer — real available langs, provider chip, fixed auto-select, browse interactivity

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `pickBestForLang` (Task 1); `SubtitlesMenu` props/events (Task 2).
- Produces: no external interface change.

- [ ] **Step 1: Import `pickBestForLang`**

Find the existing import of `pickDefaultSubtitle` in `AePlayer.vue` `<script setup>` and extend it:

```ts
import { pickDefaultSubtitle, pickBestForLang } from '@/composables/aePlayer/pickDefaultSubtitle'
```
(If `pickDefaultSubtitle` is imported on its own line, just add `pickBestForLang` to the same braces.)

- [ ] **Step 2: Replace availability + add provider-chip computeds**

Replace the `subLangsAvailable` computed (around `AePlayer.vue:2230`):

```ts
const subLangsAvailable = computed(() =>
  chosenSub.value ? [chosenSub.value.lang] : [],
)
```

with:

```ts
// Real distinct languages that have a loaded soft track (provider-bundled + aggregated).
const availableSubLangs = computed(() =>
  [...new Set(subtitleTracks.value.map((t) => t.lang))],
)

// Provider-bundled soft subs that shipped with the resolved stream.
const providerBundledTracks = computed<SubTrack[]>(
  () => (providerSubtitles.value ?? []) as SubTrack[],
)
const providerChip = computed<{ provider: string } | null>(() =>
  providerBundledTracks.value.length > 0
    ? { provider: providerBundledTracks.value[0].provider }
    : null,
)
const providerBundledUrls = computed(() => new Set(providerBundledTracks.value.map((t) => t.url)))
const providerActive = computed(
  () => !!chosenSub.value && providerBundledUrls.value.has(chosenSub.value.url),
)
```

- [ ] **Step 3: Prefer bundled subs in auto-select + fix hardsubNote**

Replace `autoSelectSubtitle` (around `AePlayer.vue:2195`) with:

```ts
function autoSelectSubtitle() {
  if (subUserDecided || chosenSub.value) return
  if (state.combo.value.audio !== 'sub') return // only SUB/raw cuts
  if (hardsubNote.value) return                 // burned-in already
  // Prefer the provider's own bundled track; else best match for the audio language.
  const pick =
    providerBundledTracks.value[0] ??
    pickDefaultSubtitle(subtitleTracks.value, { lang: state.combo.value.lang })
  if (pick) {
    chosenSub.value = pick
    state.subLang.value = pick.lang
  }
}
```

In the `hardsubNote` computed (around `AePlayer.vue:2236`), add a guard so it only fires when there are genuinely no soft tracks. Change:

```ts
const hardsubNote = computed(() => {
  if (chosenSub.value) return null
  if (state.combo.value.audio !== 'sub') return null
  const prov = activeProviderName.value
```
to:
```ts
const hardsubNote = computed(() => {
  if (chosenSub.value) return null
  if (state.combo.value.audio !== 'sub') return null
  if (subtitleTracks.value.length > 0) return null // soft tracks exist → not hardsubbed
  const prov = activeProviderName.value
```

- [ ] **Step 4: Add the menu event handlers**

Near `onSelectSubTrack` / `onSubtitlesOff` (around `AePlayer.vue:2248`), add:

```ts
function onPickSubLang(v: string) {
  if (v === 'off') { onSubtitlesOff(); return }
  const track = pickBestForLang(subtitleTracks.value, v)
  if (track) onSelectSubTrack(track)
}
function onSelectProviderSub() {
  const t = providerBundledTracks.value[0]
  if (t) onSelectSubTrack(t)
}
```

- [ ] **Step 5: Update the `<SubtitlesMenu>` wiring + fix browse interactivity**

Replace the `<SubtitlesMenu .../>` block (around `AePlayer.vue:274-286`) with:

```vue
      <SubtitlesMenu
        :sub-lang="state.subLang.value"
        :available-sub-langs="availableSubLangs"
        :provider-chip="providerChip"
        :provider-active="providerActive"
        :hardsub-note="hardsubNote"
        :sub-size="state.subSize.value"
        :sub-bg="state.subBg.value"
        :sub-offset="state.subOffset.value"
        @pick-lang="onPickSubLang"
        @select-provider="onSelectProviderSub"
        @update:sub-size="v => { state.subSize.value = v }"
        @update:sub-bg="v => { state.subBg.value = v }"
        @update:sub-offset="v => { state.subOffset.value = v }"
        @open-browse="() => { openMenu = null; browseOpen = true; void ensureSubsLoaded() }"
      />
```

(The key fix for the dead Browse modal: `openMenu = null` closes the floating menu so `onClickOutside` no longer fires inside the modal.)

- [ ] **Step 6: Type-check + unit sanity**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors referencing `SubtitlesMenu`, `availableSubLangs`, `providerChip`, `pickBestForLang`, or `subLangsAvailable` (the old name must be fully removed).

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/ src/composables/aePlayer/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -m "feat(aePlayer): wire real subtitle langs, provider chip, bundled-first auto-select; fix dead Browse modal

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Full frontend verification

**Files:** none (verification only).

- [ ] **Step 1: DS-lint**

Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0`. If a new arbitrary-spacing/color class trips it, migrate to a token or an already-allowlisted class (do NOT add new allowlist entries unless no token reproduces the value, per CLAUDE.md).

- [ ] **Step 2: i18n parity**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__` (and any i18n-lint invoked by `/frontend-verify`).
Expected: PASS — `noTrack`, `moreLanguages`, `fromProvider` present in en/ru/ja with matching `{count}` / `{provider}` placeholders.

- [ ] **Step 3: Real build + type-check (the gate jsdom can't replace)**

Run: `cd frontend/web && bun run build`
Expected: build succeeds (this is the authoritative vue-tsc + Tailwind-v4 cascade check; `bunx tsc --noEmit` alone can false-pass).

- [ ] **Step 4: Run `/frontend-verify`**

Invoke the `/frontend-verify` command for `frontend/web/`. Address any DS-lint / i18n / build / lucide / TS2614 findings it surfaces.

- [ ] **Step 5: (Owner-gated) Chrome smoke**

Offer the owner a Chrome smoke (do not run automatically). If accepted: open the aePlayer on a real anime with EN subs, confirm (a) the quick row shows `[provider] Off RU EN JP` with unavailable langs greyed, (b) clicking EN/JP changes the on-screen subtitle, (c) the provider chip auto-selects on load and is hidden when the stream has no bundled subs, (d) "Browse all subtitles" opens an interactible modal showing RU/EN/JP groups + "More languages", and selecting a track drives the overlay.

- [ ] **Step 6: Hand off to after-update**

Once `/frontend-verify` is green, invoke `/animeenigma-after-update` (it redeploys `web`, writes the Russian Trump-mode changelog entry, and pushes). Then remove the worktree per the git workflow.

---

## Self-Review

**Spec coverage:**
- Quick chooser `[provider] Off RU EN JP` → Task 2. ✓
- No "Auto" button, but bundled subs auto-selected → Task 4 Step 3 (`autoSelectSubtitle` prefers bundled). ✓
- Provider chip hidden when no bundled subs → Task 2 (`v-if="providerChip"`) + Task 4 (`providerChip` computed). ✓
- Disabled greyed buttons for missing langs → Task 2 (`:disabled` + `opacity-40`). ✓
- Best-by-relevance per lang → Task 1 (`pickBestForLang` reusing `providerRank`). ✓
- "Language off" removed → Task 2 (no `Language` label). ✓
- Browse interactivity fix → Task 4 Step 5 (`openMenu = null`). ✓
- Browse RU/EN/JP default + More languages + keep search + lang chips → Task 3. ✓
- i18n en/ru/ja parity → Task 2 Step 1 + Task 5 Step 2. ✓
- Frontend-only / no backend → all tasks confined to `frontend/web/`. ✓

**Placeholder scan:** No TBD/TODO; every code step shows full code or exact old→new blocks.

**Type consistency:** `pickBestForLang(tracks, lang)` defined in Task 1, consumed identically in Task 4. `SubtitlesMenu` props (`availableSubLangs`, `providerChip`, `providerActive`) and events (`pick-lang`, `select-provider`) defined in Task 2, supplied identically in Task 4. `providerBundledTracks`/`providerChip`/`providerActive`/`availableSubLangs` names consistent across Task 4 steps. `showAllLangs`/`PRIMARY_LANGS`/`otherLangCount`/`visibleLangChips` consistent within Task 3.
