---
phase: 01-foundation
plan: 01
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-CC-01, HSB-V11-CC-02, HSB-V11-CC-03, HSB-V11-CC-04, HSB-V11-CC-05, HSB-V11-CC-06]
blocks: [02, 03, 04, 05, 06, 07, 08, 09, 10]
blocked_by: []
status: ready
---

# Plan 01: Foundation — tokens, backdrop, icons, CTA classes, transition lock, dot polish

## Goal

Ship the shared primitives every card phase will consume:

1. A `cardTokens` map (accents, kickers, per-type icons).
2. A `<SpotlightBackdrop>` component supporting two variants.
3. A `<SpotlightIcon>` inline-SVG sprite with 9 named icons.
4. Three CTA size classes (`cta-hero`, `cta-card`, `cta-text`).
5. A transition lock that fixes the blank-card bug surfaced during UAT.
6. Labeled-pill dots in `CarouselControls` with hover tooltips + active accent.

No existing card behavior changes in this phase — it adds primitives only.

## Tasks

### Task 1 — `cardTokens` + `SpotlightIcon`

Create `frontend/web/src/components/home/spotlight/tokens.ts`:

```ts
export type SpotlightAccent = 'cyan' | 'purple' | 'sky' | 'amber' | 'teal' | 'green'
export type SpotlightIconName =
  | 'telegram' | 'sparkles' | 'chart' | 'pulse'
  | 'clock' | 'play' | 'shuffle' | 'wrench' | 'lightning'

export interface CardToken {
  accent: SpotlightAccent
  kickerKey: string  // i18n key
  icon: SpotlightIconName
}

export const cardTokens: Record<SpotlightCardType, CardToken> = {
  anime_of_day:          { accent: 'cyan',   kickerKey: 'spotlight.animeOfDay.title',          icon: 'sparkles' },
  random_tail:           { accent: 'purple', kickerKey: 'spotlight.randomTail.title',          icon: 'shuffle'  },
  personal_pick:         { accent: 'cyan',   kickerKey: 'spotlight.personalPick.title',        icon: 'sparkles' },
  telegram_news:         { accent: 'sky',    kickerKey: 'spotlight.telegramNews.title',        icon: 'telegram' },
  latest_news:           { accent: 'amber',  kickerKey: 'spotlight.latestNews.title',          icon: 'sparkles' },
  platform_stats:        { accent: 'teal',   kickerKey: 'spotlight.platformStats.title',       icon: 'chart'    },
  now_watching:          { accent: 'green',  kickerKey: 'spotlight.nowWatching.title',         icon: 'pulse'    },
  not_time_yet:          { accent: 'amber',  kickerKey: 'spotlight.notTimeYet.title',          icon: 'clock'    },
  continue_watching_new: { accent: 'purple', kickerKey: 'spotlight.continueWatchingNew.title', icon: 'play'     },
}
```

Create `SpotlightIcon.vue` with inline `<svg>` for all 9 named icons. Each
icon takes a `class` prop (forwarded to the SVG root). No icon-library dep.

**Test:** `tokens.spec.ts` iterates `SpotlightCardType` union and asserts a
matching entry in `cardTokens` (parity guard so adding a 10th card type
trips the test).

### Task 2 — `SpotlightBackdrop.vue`

Single SFC with two variants:

```vue
<template>
  <div class="absolute inset-0 overflow-hidden pointer-events-none">
    <!-- poster-blur variant -->
    <img
      v-if="variant === 'poster-blur' && posterUrl"
      :src="posterUrl"
      aria-hidden="true"
      class="absolute inset-0 w-full h-full object-cover scale-110"
      style="filter: blur(40px) saturate(1.2); opacity: 0.4;"
    />
    <!-- gradient-mesh variant -->
    <div
      v-else
      aria-hidden="true"
      class="absolute inset-0"
      :class="meshClasses[accent]"
    />
    <!-- shared vignette -->
    <div class="absolute inset-0 bg-gradient-to-r from-transparent via-black/30 to-black/60" />
  </div>
</template>
```

**Props:** `variant: 'poster-blur' | 'gradient-mesh'`, `posterUrl?: string`,
`accent?: SpotlightAccent`.

**Test:** Vitest snapshots per (variant, accent) combination — 1 for
poster-blur with mock URL, 6 for gradient-mesh × 6 accents.

### Task 3 — CTA classes in main.css

Add to `frontend/web/src/styles/main.css`:

```css
.cta-hero  { @apply inline-flex items-center gap-2 px-6 py-3 rounded-xl font-semibold text-base bg-cyan-500 hover:bg-cyan-400 text-white shadow-lg shadow-cyan-500/30 transition; }
.cta-card  { @apply inline-flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-sm bg-white/10 hover:bg-white/20 text-white transition; }
.cta-text  { @apply inline-flex items-center gap-1 text-sm font-medium text-cyan-400 hover:text-cyan-300 transition; }

.cta-hero[data-accent="purple"] { @apply bg-purple-500 hover:bg-purple-400 shadow-purple-500/30; }
.cta-hero[data-accent="amber"]  { @apply bg-amber-500 hover:bg-amber-400 shadow-amber-500/30; }
.cta-hero[data-accent="green"]  { @apply bg-green-500 hover:bg-green-400 shadow-green-500/30; }
.cta-hero[data-accent="sky"]    { @apply bg-sky-500 hover:bg-sky-400 shadow-sky-500/30; }
.cta-hero[data-accent="teal"]   { @apply bg-teal-500 hover:bg-teal-400 shadow-teal-500/30; }
```

### Task 4 — Transition lock in HeroSpotlightBlock.vue

Add `isTransitioning` ref, wire to `<transition>` hooks, gate navigation:

```ts
const isTransitioning = ref(false)

function next(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value = (currentIndex.value + 1) % cards.value.length
  restart()
}
function prev(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value = (currentIndex.value - 1 + cards.value.length) % cards.value.length
  restart()
}
function goTo(i: number): void {
  if (isTransitioning.value) return
  if (i < 0 || i >= cards.value.length) return
  currentIndex.value = i
  restart()
}
```

And on the `<transition>`:

```vue
<transition
  :name="reducedMotion ? 'none' : 'spotlight-fade'"
  mode="out-in"
  @before-leave="isTransitioning = true"
  @after-enter="isTransitioning = false"
>
```

Update `main.css` `spotlight-fade` rule to use a CSS var:

```css
:root { --spotlight-fade-ms: 400ms; }
.spotlight-fade-enter-active,
.spotlight-fade-leave-active { transition: opacity var(--spotlight-fade-ms) ease; }
.spotlight-fade-enter-from,
.spotlight-fade-leave-to { opacity: 0; }
```

### Task 5 — Labeled-pill dots in CarouselControls.vue

Replace existing grey-dot row with pill buttons:

```vue
<button
  v-for="(card, i) in cards"
  :key="i"
  :aria-label="t(cardTokens[card.type].kickerKey)"
  :aria-current="i === currentIndex ? 'true' : 'false'"
  :title="t(cardTokens[card.type].kickerKey)"
  class="group relative inline-flex items-center justify-center w-8 h-8 rounded-full transition"
  :class="i === currentIndex
    ? accentBg(card.type) + ' scale-110'
    : 'bg-white/10 hover:bg-white/20'"
  @click="$emit('goto', i)"
>
  <SpotlightIcon :name="cardTokens[card.type].icon" class="w-3.5 h-3.5" />
</button>
```

Active dot pulls accent class from `cardTokens[card.type].accent`.

### Task 6 — E2E test for transition lock

`frontend/web/e2e/spotlight-transition-lock.spec.ts`:

```ts
test('10 rapid ArrowRight presses settle to discrete cards', async ({ page }) => {
  await page.goto('/')
  await page.locator('[role="region"][aria-roledescription="carousel"]').focus()
  const initial = await activeCardType(page)
  for (let i = 0; i < 10; i++) await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(600)  // wait for the LAST transition to settle

  const final = await activeCardType(page)
  expect(final).not.toBe(initial)

  // Critical: no card stuck mid-fade
  const stuckLeave = await page.locator('.spotlight-fade-leave-active').count()
  expect(stuckLeave).toBe(0)
})
```

## Verification

- `cd frontend/web && bunx vitest run src/components/home/spotlight/tokens.spec.ts src/components/home/spotlight/SpotlightBackdrop.spec.ts src/components/home/spotlight/SpotlightIcon.spec.ts` — green.
- `cd frontend/web && bunx tsc --noEmit` — clean.
- `cd frontend/web && bunx eslint src/components/home/spotlight/ src/styles/main.css` — clean.
- `cd frontend/web && bunx playwright test spotlight-transition-lock` — green.
- Existing `cd frontend/web && bunx playwright test spotlight spotlight-full` — green (no regression).
- Manual: load `https://animeenigma.ru/` after deploy; verify labeled-pill dots render correctly per card type; 10× ArrowRight no longer produces a blank screen.

## Threat surface

| Threat | Mitigation |
|---|---|
| T-V11-01: Backdrop image leaks an extra HTTP request per card | `poster-url` reuses the card's existing poster URL — browser cache hit. |
| T-V11-02: Inline `<svg>` sprite ballooning bundle | Pre-bundle check: `SpotlightIcon` adds <3 KB gzipped (9 small icons). |
| T-V11-03: Transition lock deadlock if `@after-enter` never fires | Set a 600ms watchdog timer that force-resets `isTransitioning`. |

## Metrics

`UXΔ = +3 (Better) · CDI = 0.04 * 13 · MVQ = Griffin 88%/85%`
