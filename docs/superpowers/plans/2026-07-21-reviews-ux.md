# Reviews UX Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the four review asks from feedback `2026-07-21T01-16-37_tNeymik_telegram`: safe mini-markdown in reviews, a real editor, collapsed long reviews, and a profile Reviews tab.

**Architecture:** A dependency-free token parser (`reviewMarkdown.ts`, pattern-follows `renderFanfic.ts` — text nodes only, never `v-html`) feeds a shared `ReviewMarkdown.vue` renderer with click-to-reveal spoilers and an overflow-measured collapse. `ReviewEditor.vue` wraps an auto-growing textarea with a formatting toolbar + preview. `MyReviewsTab.vue` lists own reviews via the existing `GET /api/users/reviews`.

**Tech Stack:** Vue 3 `<script setup>` + TS, Tailwind (Neon-Tokyo tokens), vitest, lucide-vue-next (NAMED imports only), vue-i18n (en/ru/ja parity REQUIRED).

## Global Constraints

- Worktree: `/data/animeenigma/.claude/worktrees/reviews-ux` — all paths below relative to `frontend/web/` inside it. NEVER touch `/data/animeenigma` base tree.
- NO `v-html` anywhere. Rendering is typed blocks → text interpolation (house XSS rule, see `src/components/fanfic/renderFanfic.ts`).
- DS rules: semantic tokens, no new hex/rgba/inline color styles; `cyan`/`pink` brand hues are exempt. A PostToolUse DS-lint hook runs on every edit — fix violations immediately.
- i18n: every new key added to **all three** of `src/locales/en.json`, `ru.json`, `ja.json`.
- lucide icons: named imports from `lucide-vue-next`.
- Vue: never place a non-conditional element between `v-if` and `v-else-if` siblings.
- Commits: pathspec commits (`git commit -- <paths>`), NEVER `git add -A`, never amend. Commit per task, do NOT push (orchestrator pushes).
- Run tests with `bun run test:unit -- <file>` from `frontend/web/`.

---

### Task 1: `reviewMarkdown.ts` parser + `ReviewMarkdown.vue` renderer

**Files:**
- Create: `src/utils/reviewMarkdown.ts`
- Create: `src/utils/reviewMarkdown.spec.ts`
- Create: `src/components/anime/ReviewMarkdown.vue`

**Interfaces:**
- Produces: `parseReviewMarkdown(src: string): ReviewBlock[]`; types `InlineToken { kind: 'text'|'bold'|'italic'|'strike'|'spoiler'; text: string }`, `ReviewLine = InlineToken[]`, `ReviewBlock = { type: 'p'; lines: ReviewLine[] } | { type: 'ul'; items: ReviewLine[] }`.
- Produces: `ReviewMarkdown.vue` props `{ source: string; collapsible?: boolean }` (default `collapsible: false`).

- [ ] **Step 1: Write the failing tests** — `src/utils/reviewMarkdown.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { parseReviewMarkdown } from './reviewMarkdown'

describe('parseReviewMarkdown', () => {
  it('plain text becomes one paragraph with one text token', () => {
    expect(parseReviewMarkdown('hello world')).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: 'hello world' }]] },
    ])
  })

  it('blank lines split paragraphs, single newlines split lines', () => {
    const blocks = parseReviewMarkdown('a\nb\n\nc')
    expect(blocks).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: 'a' }], [{ kind: 'text', text: 'b' }]] },
      { type: 'p', lines: [[{ kind: 'text', text: 'c' }]] },
    ])
  })

  it('parses bold, italic, strike, spoiler inline tokens', () => {
    const [p] = parseReviewMarkdown('x **b** *i* ~~s~~ ||sp|| y')
    expect(p).toEqual({
      type: 'p',
      lines: [[
        { kind: 'text', text: 'x ' },
        { kind: 'bold', text: 'b' },
        { kind: 'text', text: ' ' },
        { kind: 'italic', text: 'i' },
        { kind: 'text', text: ' ' },
        { kind: 'strike', text: 's' },
        { kind: 'text', text: ' ' },
        { kind: 'spoiler', text: 'sp' },
        { kind: 'text', text: ' y' },
      ]],
    })
  })

  it('consecutive "- " / "* " lines form a ul block', () => {
    expect(parseReviewMarkdown('- one\n- **two**\ntail')).toEqual([
      { type: 'ul', items: [[{ kind: 'text', text: 'one' }], [{ kind: 'bold', text: 'two' }]] },
      { type: 'p', lines: [[{ kind: 'text', text: 'tail' }]] },
    ])
  })

  it('unclosed markers stay literal text', () => {
    expect(parseReviewMarkdown('**open *and ~~more')).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: '**open *and ~~more' }]] },
    ])
  })

  it('html in input stays inert text (XSS)', () => {
    const [p] = parseReviewMarkdown('<img src=x onerror=alert(1)> **<b>bold</b>**')
    expect(p.type).toBe('p')
    const line = (p as { lines: unknown[][] }).lines[0] as { kind: string; text: string }[]
    expect(line[0]).toEqual({ kind: 'text', text: '<img src=x onerror=alert(1)> ' })
    expect(line[1]).toEqual({ kind: 'bold', text: '<b>bold</b>' })
  })

  it('empty / whitespace input yields no blocks', () => {
    expect(parseReviewMarkdown('')).toEqual([])
    expect(parseReviewMarkdown('  \n\n ')).toEqual([])
  })
})
```

- [ ] **Step 2: Run to verify failure** — `cd frontend/web && bun run test:unit -- src/utils/reviewMarkdown.spec.ts` → FAIL (module not found).

- [ ] **Step 3: Implement `src/utils/reviewMarkdown.ts`:**

```ts
/**
 * Safe review mini-markdown parser (feedback 2026-07-21, @realMiZZeR).
 *
 * Same safety contract as renderFanfic.ts: dependency-free, returns typed
 * tokens that ReviewMarkdown.vue renders as TEXT nodes ({{ t.text }}, never
 * v-html) — that projection is the entire XSS defense. Flat subset, no
 * nesting: **bold**, *italic*, ~~strike~~, ||spoiler||, "- " bullet lists,
 * blank-line paragraphs with single-newline line breaks.
 */

export type InlineToken = {
  kind: 'text' | 'bold' | 'italic' | 'strike' | 'spoiler'
  text: string
}
export type ReviewLine = InlineToken[]
export type ReviewBlock =
  | { type: 'p'; lines: ReviewLine[] }
  | { type: 'ul'; items: ReviewLine[] }

// Longest markers first so ** wins over *. Italic forbids inner newlines so a
// line-leading "* " list marker never pairs with a later asterisk.
const INLINE_RE = /\*\*([^*]+)\*\*|\*([^*\n]+)\*|~~([^~]+)~~|\|\|([^|]+)\|\|/g

function parseInline(line: string): ReviewLine {
  const tokens: ReviewLine = []
  let last = 0
  for (const m of line.matchAll(INLINE_RE)) {
    const idx = m.index ?? 0
    if (idx > last) tokens.push({ kind: 'text', text: line.slice(last, idx) })
    if (m[1] !== undefined) tokens.push({ kind: 'bold', text: m[1] })
    else if (m[2] !== undefined) tokens.push({ kind: 'italic', text: m[2] })
    else if (m[3] !== undefined) tokens.push({ kind: 'strike', text: m[3] })
    else tokens.push({ kind: 'spoiler', text: m[4] })
    last = idx + m[0].length
  }
  if (last < line.length) tokens.push({ kind: 'text', text: line.slice(last) })
  return tokens
}

const LIST_ITEM_RE = /^[-*]\s+/

export function parseReviewMarkdown(src: string): ReviewBlock[] {
  const blocks: ReviewBlock[] = []
  for (const raw of src.split(/\n{2,}/)) {
    const chunk = raw.trim()
    if (!chunk) continue
    const lines = chunk.split('\n').map((l) => l.trim()).filter(Boolean)
    let para: ReviewLine[] = []
    let list: ReviewLine[] = []
    const flushPara = () => {
      if (para.length) blocks.push({ type: 'p', lines: para })
      para = []
    }
    const flushList = () => {
      if (list.length) blocks.push({ type: 'ul', items: list })
      list = []
    }
    for (const line of lines) {
      if (LIST_ITEM_RE.test(line)) {
        flushPara()
        list.push(parseInline(line.replace(LIST_ITEM_RE, '')))
      } else {
        flushList()
        para.push(parseInline(line))
      }
    }
    flushPara()
    flushList()
  }
  return blocks
}
```

- [ ] **Step 4: Run tests** → all PASS.

- [ ] **Step 5: Implement `src/components/anime/ReviewMarkdown.vue`:**

```vue
<!--
  ReviewMarkdown — renders review mini-markdown via parseReviewMarkdown()
  typed tokens and TEXT interpolation only (never v-html — the XSS defense).
  Spoilers render as redacted chips until clicked. `collapsible` clamps long
  content (measured overflow, not char count) behind a Show-more toggle.
-->
<template>
  <div>
    <div
      ref="bodyEl"
      class="relative"
      :class="collapsed ? 'max-h-56 overflow-hidden' : ''"
    >
      <template v-for="(block, bi) in blocks" :key="bi">
        <ul v-if="block.type === 'ul'" class="list-disc pl-5 space-y-0.5 my-1.5">
          <li v-for="(item, ii) in block.items" :key="ii">
            <ReviewInline :tokens="item" :revealed="revealed" :block="`${bi}:${ii}`" @reveal="reveal" />
          </li>
        </ul>
        <p v-else class="my-1.5 first:mt-0 last:mb-0">
          <template v-for="(line, li) in block.lines" :key="li">
            <br v-if="li > 0" />
            <ReviewInline :tokens="line" :revealed="revealed" :block="`${bi}:${li}`" @reveal="reveal" />
          </template>
        </p>
      </template>
      <div
        v-if="collapsed && overflowing"
        class="pointer-events-none absolute inset-x-0 bottom-0 h-14 bg-gradient-to-t from-black/60 to-transparent"
      />
    </div>
    <button
      v-if="collapsible && overflowing"
      type="button"
      class="mt-1.5 text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
      @click="expanded = !expanded"
    >
      {{ expanded ? $t('anime.reviewFmt.showLess') : $t('anime.reviewFmt.showMore') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { parseReviewMarkdown } from '@/utils/reviewMarkdown'
import ReviewInline from './ReviewInline.vue'

const props = withDefaults(defineProps<{ source: string; collapsible?: boolean }>(), {
  collapsible: false,
})

const blocks = computed(() => parseReviewMarkdown(props.source))

// Per-spoiler reveal state, keyed "block:line:token".
const revealed = ref<Set<string>>(new Set())
function reveal(key: string) {
  revealed.value = new Set(revealed.value).add(key)
}

// Collapse: clamp only when the rendered body actually overflows the clamp
// height (max-h-56 = 224px), re-measured when the source changes.
const bodyEl = ref<HTMLElement | null>(null)
const expanded = ref(false)
const overflowing = ref(false)
const collapsed = computed(() => props.collapsible && !expanded.value)

async function measure() {
  if (!props.collapsible) return
  await nextTick()
  const el = bodyEl.value
  if (el) overflowing.value = el.scrollHeight > 240
}
onMounted(measure)
watch(() => props.source, () => {
  expanded.value = false
  revealed.value = new Set()
  void measure()
})
</script>
```

Note on measurement: measure `scrollHeight` against 240 (clamp 224 + tolerance) while collapsed; when `expanded`, keep the last `overflowing` value so the Show-less button stays.
**Caveat:** with `expanded=true` the body isn't clamped, so re-measuring `scrollHeight > 240` still works (scrollHeight is content height either way). So `measure()` is safe in both states.

- [ ] **Step 6: Implement `src/components/anime/ReviewInline.vue`** (leaf renderer for one token line):

```vue
<!-- ReviewInline — one line of InlineTokens as text nodes. Spoiler tokens are
     <button> chips until revealed (per-token key handled by the parent). -->
<template>
  <template v-for="(t, ti) in tokens" :key="ti">
    <strong v-if="t.kind === 'bold'" class="font-semibold text-white/90">{{ t.text }}</strong>
    <em v-else-if="t.kind === 'italic'">{{ t.text }}</em>
    <s v-else-if="t.kind === 'strike'" class="text-white/50">{{ t.text }}</s>
    <span
      v-else-if="t.kind === 'spoiler' && revealed.has(`${block}:${ti}`)"
      class="rounded bg-white/10 px-1"
      >{{ t.text }}</span
    >
    <button
      v-else-if="t.kind === 'spoiler'"
      type="button"
      class="mx-0.5 rounded bg-white/10 px-2 py-0.5 text-xs text-white/60 hover:bg-white/15 hover:text-white/80 transition-colors align-baseline"
      :aria-label="$t('anime.reviewFmt.spoilerReveal')"
      @click="emit('reveal', `${block}:${ti}`)"
    >
      {{ $t('anime.reviewFmt.spoiler') }}
    </button>
    <template v-else>{{ t.text }}</template>
  </template>
</template>

<script setup lang="ts">
import type { InlineToken } from '@/utils/reviewMarkdown'

defineProps<{ tokens: InlineToken[]; revealed: Set<string>; block: string }>()
const emit = defineEmits<{ reveal: [key: string] }>()
</script>
```

(Files list amendment: this task also creates `src/components/anime/ReviewInline.vue`.)

- [ ] **Step 7: Type-check + lint** — `bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | grep -i review` (expect nothing) and confirm the DS-lint hook reported no errors on edits. i18n keys used here (`anime.reviewFmt.*`) land in Task 3 — that's fine for tsc, but do NOT run the full i18n gate until Task 3.

- [ ] **Step 8: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/reviews-ux
git commit -m "feat(reviews): safe mini-markdown parser + ReviewMarkdown renderer with spoilers and collapse" -- frontend/web/src/utils/reviewMarkdown.ts frontend/web/src/utils/reviewMarkdown.spec.ts frontend/web/src/components/anime/ReviewMarkdown.vue frontend/web/src/components/anime/ReviewInline.vue
```

---

### Task 2: `ReviewEditor.vue`

**Files:**
- Create: `src/components/anime/ReviewEditor.vue`

**Interfaces:**
- Consumes: `ReviewMarkdown.vue` (`{ source }`) from Task 1.
- Produces: `ReviewEditor.vue` props `{ modelValue: string; placeholder?: string }`, emits `update:modelValue`. Pure v-model wrapper — no submit logic.

- [ ] **Step 1: Implement `src/components/anime/ReviewEditor.vue`:**

```vue
<!--
  ReviewEditor — review writing surface (feedback 2026-07-21): auto-growing
  textarea (~8 rows → 60vh), formatting toolbar wrapping the current selection
  with review mini-markdown markers, Write/Preview toggle. Pure v-model.
-->
<template>
  <div class="rounded-lg border border-white/10 bg-white/5 focus-within:ring-2 focus-within:ring-cyan-500/50 transition-shadow">
    <!-- Toolbar -->
    <div class="flex items-center gap-1 border-b border-white/10 px-2 py-1.5">
      <button
        v-for="a in actions"
        :key="a.key"
        type="button"
        class="rounded p-1.5 text-white/60 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40"
        :title="$t(`anime.reviewFmt.${a.key}`)"
        :aria-label="$t(`anime.reviewFmt.${a.key}`)"
        :disabled="mode === 'preview'"
        @click="a.run"
      >
        <component :is="a.icon" class="size-4" />
      </button>
      <div class="ml-auto">
        <SegmentedControl
          :model-value="mode"
          :options="[
            { value: 'write', label: $t('anime.reviewFmt.write') },
            { value: 'preview', label: $t('anime.reviewFmt.preview') },
          ]"
          size="sm"
          @update:model-value="mode = $event as 'write' | 'preview'"
        />
      </div>
    </div>

    <!-- Write -->
    <textarea
      v-if="mode === 'write'"
      ref="ta"
      :value="modelValue"
      :placeholder="placeholder"
      class="block w-full resize-none bg-transparent px-4 py-3 text-white placeholder-white/30 focus-visible:outline-none"
      :style="{ minHeight: '11rem', maxHeight: '60vh', height: taHeight }"
      @input="onInput"
      @keydown="onKeydown"
    ></textarea>

    <!-- Preview -->
    <div v-else class="min-h-44 px-4 py-3 text-white/80">
      <ReviewMarkdown v-if="modelValue.trim()" :source="modelValue" />
      <p v-else class="text-white/30">{{ placeholder }}</p>
    </div>

    <p class="border-t border-white/10 px-3 py-1.5 text-xs text-white/40">
      {{ $t('anime.reviewFmt.hint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { Bold, Italic, Strikethrough, EyeOff, List } from 'lucide-vue-next'
import { SegmentedControl } from '@/components/ui'
import ReviewMarkdown from './ReviewMarkdown.vue'

const props = defineProps<{ modelValue: string; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const ta = ref<HTMLTextAreaElement | null>(null)
const mode = ref<'write' | 'preview'>('write')
const taHeight = ref('11rem')

function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  taHeight.value = `${el.scrollHeight}px`
  el.style.height = ''
}

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
  autoGrow()
}

// External value changes (e.g. loading an existing review) also resize.
watch(
  () => props.modelValue,
  () => void nextTick(autoGrow),
  { immediate: true },
)

function wrapSelection(before: string, after = before) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const next = value.slice(0, s) + before + value.slice(s, e) + after + value.slice(e)
  emit('update:modelValue', next)
  void nextTick(() => {
    el.focus()
    el.setSelectionRange(s + before.length, e + before.length)
    autoGrow()
  })
}

function prefixLines() {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e, value } = el
  const start = value.lastIndexOf('\n', s - 1) + 1
  const segment = value.slice(start, e)
  const prefixed = segment
    .split('\n')
    .map((l) => (l.trim() ? `- ${l.replace(/^[-*]\s+/, '')}` : l))
    .join('\n')
  emit('update:modelValue', value.slice(0, start) + prefixed + value.slice(e))
  void nextTick(() => {
    el.focus()
    autoGrow()
  })
}

const actions = [
  { key: 'bold', icon: Bold, run: () => wrapSelection('**') },
  { key: 'italic', icon: Italic, run: () => wrapSelection('*') },
  { key: 'strike', icon: Strikethrough, run: () => wrapSelection('~~') },
  { key: 'spoiler', icon: EyeOff, run: () => wrapSelection('||') },
  { key: 'list', icon: List, run: prefixLines },
] as const

function onKeydown(e: KeyboardEvent) {
  if (!(e.ctrlKey || e.metaKey)) return
  if (e.key === 'b') {
    e.preventDefault()
    wrapSelection('**')
  } else if (e.key === 'i') {
    e.preventDefault()
    wrapSelection('*')
  }
}
</script>
```

Check first that `SegmentedControl` accepts `{ options, model-value, size }` in this shape (see usage in `src/views/Profile.vue` around line 200); if its API differs, adapt to the real API rather than changing the component.

- [ ] **Step 2: Type-check** — `bunx vue-tsc --noEmit -p tsconfig.app.json` scoped grep as in Task 1.

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(reviews): ReviewEditor — auto-grow textarea, formatting toolbar, preview" -- frontend/web/src/components/anime/ReviewEditor.vue
```

---

### Task 3: Anime.vue integration + i18n keys

**Files:**
- Modify: `src/views/Anime.vue` (~lines 622-631 textarea block; ~line 696 review text `<p>`; imports ~line 976)
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json` (inside the existing `"anime"` object)

**Interfaces:**
- Consumes: `ReviewEditor` (v-model) and `ReviewMarkdown` (`{ source, collapsible }`).

- [ ] **Step 1: Add i18n keys.** In `en.json` inside `"anime"` (near `"reviewOptional"`), add:

```json
"reviewFmt": {
  "bold": "Bold",
  "italic": "Italic",
  "strike": "Strikethrough",
  "spoiler": "Spoiler",
  "spoilerReveal": "Reveal spoiler",
  "list": "List",
  "write": "Write",
  "preview": "Preview",
  "hint": "**bold** · *italic* · ~~strikethrough~~ · ||spoiler|| · \"- \" list",
  "showMore": "Show more",
  "showLess": "Show less"
}
```

`ru.json`:

```json
"reviewFmt": {
  "bold": "Жирный",
  "italic": "Курсив",
  "strike": "Зачёркнутый",
  "spoiler": "Спойлер",
  "spoilerReveal": "Показать спойлер",
  "list": "Список",
  "write": "Текст",
  "preview": "Предпросмотр",
  "hint": "**жирный** · *курсив* · ~~зачёркнутый~~ · ||спойлер|| · «- » список",
  "showMore": "Показать полностью",
  "showLess": "Свернуть"
}
```

`ja.json`:

```json
"reviewFmt": {
  "bold": "太字",
  "italic": "斜体",
  "strike": "取り消し線",
  "spoiler": "ネタバレ",
  "spoilerReveal": "ネタバレを表示",
  "list": "箇条書き",
  "write": "書く",
  "preview": "プレビュー",
  "hint": "**太字** · *斜体* · ~~取り消し線~~ · ||ネタバレ|| · 「- 」リスト",
  "showMore": "すべて表示",
  "showLess": "折りたたむ"
}
```

- [ ] **Step 2: Replace the review textarea** in `src/views/Anime.vue` (current lines 622-631):

```vue
          <!-- Review Text -->
          <div class="mb-4">
            <label class="block text-white/60 text-sm mb-2">{{ $t('anime.reviewOptional') }}</label>
            <ReviewEditor v-model="reviewForm.text" :placeholder="$t('anime.reviewPlaceholder')" />
          </div>
```

- [ ] **Step 3: Replace the feed text renderer** (current line 696):

```vue
            <ReviewMarkdown v-if="review.review_text" :source="review.review_text" collapsible class="text-white/70" />
```

- [ ] **Step 4: Add imports** next to the existing `ReviewReactions` import (~line 976):

```ts
import ReviewEditor from '@/components/anime/ReviewEditor.vue'
import ReviewMarkdown from '@/components/anime/ReviewMarkdown.vue'
```

- [ ] **Step 5: Verify** — `bunx vue-tsc --noEmit -p tsconfig.app.json` (grep for errors in changed files; remember vue-tsc false-pass caveat: also eyeball `bunx eslint src/views/Anime.vue src/components/anime/ReviewEditor.vue`), then `bun run test:unit -- src/utils/reviewMarkdown.spec.ts`.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(reviews): markdown rendering + real editor on anime page, i18n en/ru/ja" -- frontend/web/src/views/Anime.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```

---

### Task 4: Profile Reviews tab

**Files:**
- Create: `src/components/profile/MyReviewsTab.vue`
- Modify: `src/views/Profile.vue` (tabs computed ~line 1090; template — add `<template #reviews>` after the `#watchlist` template block which ends near line 356; imports ~line 896)
- Modify: `src/locales/{en,ru,ja}.json` (`profile.tabs.reviews` + `profile.reviews.*`)

**Interfaces:**
- Consumes: `reviewApi.getMyReviews()` (`GET /users/reviews`, returns `reviewResponse[]` with `anime: { id, name, name_ru, name_jp, poster_url }`), `ReviewMarkdown`, `PosterImage` (`@/components/anime/PosterImage.vue`), `getLocalizedTitle` (`@/utils/title`).

- [ ] **Step 1: i18n.** `en.json` inside `"profile"`: add `"reviews": "Reviews"` to `"tabs"`, and a sibling group:

```json
"reviews": {
  "empty": "No reviews yet. Rate an anime and write your first one!",
  "openAnime": "Open anime page"
}
```

`ru.json`: tabs `"reviews": "Рецензии"`; group `{ "empty": "Пока нет рецензий. Оцените аниме и напишите первую!", "openAnime": "Открыть страницу аниме" }`.
`ja.json`: tabs `"reviews": "レビュー"`; group `{ "empty": "まだレビューがありません。アニメを評価して最初のレビューを書きましょう！", "openAnime": "アニメページを開く" }`.

- [ ] **Step 2: Create `src/components/profile/MyReviewsTab.vue`:**

```vue
<!--
  MyReviewsTab — own-profile Reviews tab (feedback 2026-07-21): the viewer's
  written reviews (review_text != '') via GET /users/reviews (JWT-claims
  endpoint — own profile only). Lazy: parent mounts it on first tab open.
-->
<template>
  <div v-if="loading" class="flex justify-center py-12">
    <Spinner />
  </div>

  <EmptyState v-else-if="reviews.length === 0" :description="$t('profile.reviews.empty')" />

  <div v-else class="space-y-4">
    <div v-for="r in reviews" :key="r.id" class="glass-card p-4">
      <div class="flex gap-4">
        <router-link :to="`/anime/${r.anime_id}`" class="w-16 shrink-0" :aria-label="$t('profile.reviews.openAnime')">
          <PosterImage :src="r.anime?.poster_url || ''" :alt="title(r)" ratio="2/3" rounded="lg" :proxy-width="128" />
        </router-link>
        <div class="min-w-0 flex-1">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <router-link
                :to="`/anime/${r.anime_id}`"
                class="block truncate font-medium text-white hover:text-cyan-400 transition-colors"
              >
                {{ title(r) }}
              </router-link>
              <p class="text-sm text-white/60">{{ formatDate(r.created_at) }}</p>
            </div>
            <div v-if="r.score > 0" class="flex items-center gap-1 text-cyan-400">
              <ScoreDiamond class="size-5" />
              <span class="font-semibold">{{ r.score }}</span>
            </div>
          </div>
          <ReviewMarkdown :source="r.review_text" collapsible class="mt-2 text-white/70" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { reviewApi } from '@/api/client'
import { EmptyState, ScoreDiamond, Spinner } from '@/components/ui'
import PosterImage from '@/components/anime/PosterImage.vue'
import ReviewMarkdown from '@/components/anime/ReviewMarkdown.vue'
import { getLocalizedTitle } from '@/utils/title'
import { formatDate } from '@/composables/anime/animeFormatters'

interface MyReview {
  id: string
  anime_id: string
  score: number
  review_text: string
  created_at: string
  anime?: { name?: string; name_ru?: string; name_jp?: string; poster_url?: string }
}

const reviews = ref<MyReview[]>([])
const loading = ref(true)

function title(r: MyReview): string {
  return getLocalizedTitle(r.anime?.name, r.anime?.name_ru, r.anime?.name_jp) || 'Anime'
}

onMounted(async () => {
  try {
    const res = await reviewApi.getMyReviews()
    const all: MyReview[] = res.data?.data || res.data || []
    reviews.value = all.filter((r) => (r.review_text || '').trim() !== '')
  } catch (err) {
    console.error('Failed to fetch my reviews:', err)
  } finally {
    loading.value = false
  }
})
</script>
```

Check the real exports first: `formatDate` may live elsewhere (`grep -rn "export function formatDate" src/`) and `EmptyState`'s prop may be `description`/`message` — match the actual API (see usage in `Profile.vue`). If `EmptyState` needs a `title`, reuse the pattern already in Profile's empty watchlist state.

- [ ] **Step 3: Wire into `Profile.vue`.** In the `tabs` computed (~line 1090), add after the watchlist push:

```ts
  if (isOwnProfile.value) {
    baseTabs.push({ value: 'reviews', label: t('profile.tabs.reviews') })
  }
```

(Keep it BEFORE the gacha/settings pushes so order is: showcase · watchlist · reviews · collection · settings.)

In the template, directly after the closing of `<template #watchlist>` (before `<template #settings>`), add:

```vue
          <!-- Reviews Tab — own profile only (endpoint is JWT-claims-based) -->
          <template v-if="isOwnProfile" #reviews>
            <MyReviewsTab />
          </template>
```

Import next to `WatchlistFilters` (~line 896):

```ts
import MyReviewsTab from '@/components/profile/MyReviewsTab.vue'
```

Note: `Tabs` only mounts the active pane's slot content — verify by checking `src/components/ui/Tabs.vue` for `v-if`/`v-show` slot handling. If it renders all panes eagerly (`v-show`), gate the fetch instead: wrap `<MyReviewsTab v-if="reviewsTabOpened" />` where `reviewsTabOpened` is a `ref(false)` set `true` the first time `activeTab === 'reviews'` (add a small `watch(activeTab, ...)`).

- [ ] **Step 4: Verify** — `bunx vue-tsc --noEmit -p tsconfig.app.json`, `bunx eslint src/components/profile/MyReviewsTab.vue src/views/Profile.vue`.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(profile): Reviews tab — own written reviews with posters and collapse" -- frontend/web/src/components/profile/MyReviewsTab.vue frontend/web/src/views/Profile.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```

---

### Task 5: Full verification gate

**Files:** none (verification only)

- [ ] **Step 1:** `cd frontend/web && bun run test:unit` → all suites PASS.
- [ ] **Step 2:** Run the `/frontend-verify` skill (DS-lint, i18n en/ru/ja parity, real `bun run build`).
- [ ] **Step 3:** Fix anything it flags; commit fixes with pathspec commits.
