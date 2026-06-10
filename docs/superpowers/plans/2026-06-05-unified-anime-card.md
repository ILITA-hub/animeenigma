# Unified Anime Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ~12 bespoke anime-card renderings with 3 shared layout components (`PosterCard`, `PosterRow`, `MediaTile`) built over shared primitives and one data contract (`AnimeCardModel` + mappers), de-fragilizing the home/browse CSS without overengineering.

**Architecture:** A pure normalizer layer (`toCardModel.*`) maps the three existing source shapes (catalog `Anime`, `HomeAnime`, continue-watching item) into one `AnimeCardModel`. Three thin layout SFCs render that model, composing four reused primitives: `PosterImage` (lazy img + drift skeleton + scrims), `Badge` (extended with an `overlay` treatment), `AnimeKebab` (made geometry-flexible), and `AnimeContextMenu` (unchanged except a C-top item reorder). Rollout is incremental — each phase ships independently and is fully reversible.

**Tech Stack:** Vue 3 `<script setup lang="ts">`, Tailwind v4, `class-variance-authority` + `cn()` (tailwind-merge), Vitest + `@vue/test-utils`, vue-tsc. Frontend tooling is `bun`/`bunx` (never npm).

**Frozen design source:** `docs/superpowers/specs/2026-06-05-unified-anime-card-design.md` (and hosted gallery `design-v17-gallery.html`).

**Key facts established during research (do not re-litigate):**
- There are **three** locales (`en.json`, `ru.json`, `ja.json`) with a strict parity test at `src/locales/__tests__/spotlight-keys.spec.ts`. This plan reuses existing keys and adds **no** new i18n keys.
- `AnimeContextMenu.vue` **already ships** an "Open in new tab" action (`contextMenu.openInNewTab`). The "C-top" decision is a **reorder**, not a new item.
- The drift-skeleton cascade bug is solved by: base poster uses `background-color` only (never the `background` shorthand), the skeleton paints a `background-image` gradient and animates `background-position`, and **no `!important`** anywhere. The skeleton is always its **own element**, so it never shares a `background` declaration with the poster.
- The design-system lint (`scripts/design-system-lint.sh`) scans only `src/**/*.vue`. `.ts` and `.css` files are exempt. `cyan`/`pink`/`rose`/`violet` Tailwind hues are exempt brand colors; `amber`/`emerald`/`red`/etc. are NOT (use semantic tokens or existing `badge-variants` entries).

**Working directory for all commands:** `/data/animeenigma/frontend/web`

---

## File structure

**New files:**
- `src/types/card.ts` — `AnimeCardModel` + `CardExtras` + `ListStatus` types (one responsibility: the data contract).
- `src/utils/toCardModel.ts` — three pure mappers: `fromCatalogAnime`, `fromHomeAnime`, `fromContinueWatching`.
- `src/utils/__tests__/toCardModel.spec.ts` — mapper tests.
- `src/components/anime/PosterImage.vue` — lazy image + drift skeleton + scrims + slot.
- `src/components/anime/PosterImage.spec.ts`
- `src/components/anime/PosterCard.vue` — 2/3 catalog/grid card.
- `src/components/anime/PosterCard.spec.ts`
- `src/components/anime/PosterRow.vue` — hero-rail row (variants ongoing/top/announced).
- `src/components/anime/PosterRow.spec.ts`
- `src/components/anime/MediaTile.vue` — 16/9 continue-watching tile.
- `src/components/anime/MediaTile.spec.ts`

**Modified files:**
- `src/styles/main.css` — add `@keyframes drift` + `.sk-drift` class.
- `src/components/ui/Skeleton.vue` — add `variant: 'pulse' | 'drift'`.
- `src/components/ui/Skeleton.spec.ts` — (create if absent) variant test.
- `src/components/ui/badge-variants.ts` — add `overlay` variant.
- `src/components/ui/Badge.vue` — add `overlay` prop.
- `src/components/ui/__tests__/Badge.spec.ts` — (create if absent) overlay test.
- `src/components/anime/AnimeKebab.vue` — accept a `class` passthrough via `cn()`; keep current defaults.
- `src/components/anime/AnimeContextMenu.vue` — reorder actions to C-top.
- `src/components/anime/AnimeContextMenu.spec.ts` — assert new order.
- `src/components/anime/index.ts` — export new components; drop dead ones.
- `src/views/Browse.vue` — render `PosterCard`.
- `src/views/Anime.vue` — render `PosterCard` in related carousel.
- `src/views/Home.vue` — render `PosterRow`.
- `src/components/home/ContinueWatchingRow.vue` — render `MediaTile`.

**Deleted files (Phase 6, after zero-usage re-verification):**
- `src/components/anime/AnimeCard.vue`
- `src/components/anime/AnimeCardSkeleton.vue`

---

## Phase 1 — Foundation primitives (additive, no migrations)

Each task here is pure addition; nothing renders differently yet. Ship-safe on its own.

### Task 1: Drift skeleton — global CSS + Skeleton variant

**Files:**
- Modify: `src/styles/main.css` (append after the existing `@keyframes` block, near line ~115)
- Modify: `src/components/ui/Skeleton.vue`
- Test: `src/components/ui/Skeleton.spec.ts` (create)

- [ ] **Step 1: Add the drift keyframes + class to main.css**

Append this block immediately after the existing `@keyframes kebab-glow { ... }` definition (around line 113). It must be **unlayered** (a plain top-level rule) so it reliably applies to the dedicated skeleton element — it sets only `background-*`/`animation`, so it does not compete with utility classes for unrelated properties.

```css
/* Drift skeleton — animated gradient sweep for loading placeholders.
   CRITICAL: the loading element paints a background-IMAGE gradient and animates
   background-POSITION. The element it sits on must NOT use the `background`
   shorthand (which resets background-position and kills the animation), and we
   never use !important (author !important overrides CSS animations). See
   docs/superpowers/specs/2026-06-05-unified-anime-card-design.md. */
@keyframes drift {
  0%   { background-position: 0% 0; }
  100% { background-position: 100% 0; }
}
.sk-drift {
  background-color: rgba(255, 255, 255, 0.05);
  background-image: linear-gradient(
    115deg,
    rgba(255, 255, 255, 0.06),
    rgba(120, 225, 255, 0.22),
    rgba(255, 255, 255, 0.06)
  );
  background-size: 300% 100%;
  background-repeat: no-repeat;
  animation: drift 2.6s ease-in-out infinite alternate;
}
@media (prefers-reduced-motion: reduce) {
  .sk-drift { animation: none; }
}
```

- [ ] **Step 2: Write the failing Skeleton variant test**

Create `src/components/ui/Skeleton.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Skeleton from './Skeleton.vue'

describe('Skeleton', () => {
  it('defaults to the pulse treatment', () => {
    const w = mount(Skeleton)
    expect(w.classes()).toContain('animate-pulse')
    expect(w.classes()).not.toContain('sk-drift')
  })

  it('renders the drift treatment when variant="drift"', () => {
    const w = mount(Skeleton, { props: { variant: 'drift' } })
    expect(w.classes()).toContain('sk-drift')
    expect(w.classes()).not.toContain('animate-pulse')
    expect(w.classes()).not.toContain('bg-white/10')
  })

  it('still applies rounded + custom className', () => {
    const w = mount(Skeleton, { props: { variant: 'drift', rounded: 'lg', className: 'h-20' } })
    expect(w.classes()).toContain('rounded-lg')
    expect(w.classes()).toContain('h-20')
  })
})
```

- [ ] **Step 3: Run the test, confirm it fails**

Run: `bunx vitest run src/components/ui/Skeleton.spec.ts`
Expected: FAIL — the drift test fails because `Skeleton.vue` has no `variant` prop yet.

- [ ] **Step 4: Implement the variant in Skeleton.vue**

Replace the entire file contents with:

```vue
<template>
  <div
    :class="[
      variant === 'drift' ? 'sk-drift' : 'animate-pulse bg-white/10',
      'rounded',
      roundedClass,
      className,
    ]"
    :style="style"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  width?: string | number
  height?: string | number
  rounded?: 'none' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full'
  className?: string
  variant?: 'pulse' | 'drift'
}

const props = withDefaults(defineProps<Props>(), {
  rounded: 'md',
  className: '',
  variant: 'pulse',
})

const roundedClass = computed(() => {
  const map = {
    none: 'rounded-none',
    sm: 'rounded-sm',
    md: 'rounded-md',
    lg: 'rounded-lg',
    xl: 'rounded-xl',
    '2xl': 'rounded-2xl',
    full: 'rounded-full',
  }
  return map[props.rounded]
})

const style = computed(() => ({
  width: typeof props.width === 'number' ? `${props.width}px` : props.width,
  height: typeof props.height === 'number' ? `${props.height}px` : props.height,
}))
</script>
```

- [ ] **Step 5: Run the test, confirm it passes**

Run: `bunx vitest run src/components/ui/Skeleton.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/styles/main.css src/components/ui/Skeleton.vue src/components/ui/Skeleton.spec.ts
git commit -m "feat(ui): add drift skeleton variant + global .sk-drift class"
```

---

### Task 2: Badge overlay treatment

**Files:**
- Modify: `src/components/ui/badge-variants.ts`
- Modify: `src/components/ui/Badge.vue`
- Test: `src/components/ui/__tests__/Badge.spec.ts` (create)

- [ ] **Step 1: Write the failing overlay test**

Create `src/components/ui/__tests__/Badge.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Badge from '../Badge.vue'

describe('Badge', () => {
  it('renders the inline (tinted) treatment by default', () => {
    const w = mount(Badge, { props: { variant: 'warning' }, slots: { default: '9.9' } })
    expect(w.classes()).toContain('bg-amber-500/20')
    expect(w.classes()).toContain('text-amber-400')
    expect(w.text()).toBe('9.9')
  })

  it('swaps to dark-glass when overlay is set, keeping the accent text', () => {
    const w = mount(Badge, { props: { variant: 'warning', overlay: true }, slots: { default: '9.9' } })
    // tailwind-merge drops the tinted bg in favour of the glass bg
    expect(w.classes()).not.toContain('bg-amber-500/20')
    expect(w.classes()).toContain('bg-black/[0.62]')
    expect(w.classes()).toContain('backdrop-blur-[6px]')
    expect(w.classes()).toContain('text-amber-400')
  })
})
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/ui/__tests__/Badge.spec.ts`
Expected: FAIL — `overlay` prop / variant does not exist yet.

- [ ] **Step 3: Add the overlay variant to badge-variants.ts**

Replace the entire file contents with:

```ts
import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva('inline-flex items-center font-medium', {
  variants: {
    variant: {
      default: 'bg-white/10 text-white/80',
      primary: 'bg-cyan-500/20 text-cyan-400',
      secondary: 'bg-pink-500/20 text-pink-400',
      success: 'bg-emerald-500/20 text-emerald-400',
      warning: 'bg-amber-500/20 text-amber-400',
      rating: 'bg-black/60 text-amber-400 backdrop-blur-sm',
      // Phase 5 (LIB-09): purple for Nyaa provider chip — intentional literal color.
      info: 'bg-purple-500/20 text-purple-400',
      // Phase 5 (LIB-09): red for failed-job status badges — intentional literal color.
      destructive: 'bg-red-500/20 text-red-400',
    },
    size: {
      sm: 'px-2 py-0.5 text-xs rounded',
      md: 'px-2.5 py-1 text-sm rounded-md',
      lg: 'px-3 py-1.5 text-base rounded-lg',
    },
    // Overlay treatment for badges sitting on top of posters/imagery: dark glass
    // + blur + inset hairline. Declared AFTER `variant` so its bg wins the
    // tailwind-merge conflict resolution; the variant's accent TEXT colour is
    // preserved (text/bg are separate merge groups). Pair with:
    //   variant="warning" → amber ★ (MAL score)
    //   variant="primary" → cyan  ◆ (AnimeEnigma score)
    //   variant="success" → green   (ONGOING)
    //   variant="default" → white   (quality / neutral)
    overlay: {
      true: 'bg-black/[0.62] backdrop-blur-[6px] ring-1 ring-inset ring-white/10',
      false: '',
    },
  },
  defaultVariants: { variant: 'default', size: 'md', overlay: false },
})

export type BadgeVariants = VariantProps<typeof badgeVariants>
```

- [ ] **Step 4: Add the overlay prop to Badge.vue**

Replace the entire file contents with:

```vue
<template>
  <span :class="cn(badgeVariants({ variant, size, overlay }), props.class)">
    <span v-if="$slots.icon" class="mr-1">
      <slot name="icon" />
    </span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { badgeVariants, type BadgeVariants } from './badge-variants'

interface Props {
  variant?: NonNullable<BadgeVariants['variant']>
  size?: NonNullable<BadgeVariants['size']>
  overlay?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'md',
  overlay: false,
})
</script>
```

- [ ] **Step 5: Run the test, confirm it passes**

Run: `bunx vitest run src/components/ui/__tests__/Badge.spec.ts`
Expected: PASS (2 tests).

> If the overlay test fails on the `bg-amber-500/20` assertion (i.e. tailwind-merge did NOT drop it), it means `cn()` isn't resolving the conflict. Verify `cn` in `src/lib/utils.ts` wraps `tailwind-merge`'s `twMerge`. It does today; if that ever changes, switch the overlay variant to `cva` `compoundVariants` that set the final `bg-black/[0.62]` per variant instead of relying on merge order.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/ui/badge-variants.ts src/components/ui/Badge.vue src/components/ui/__tests__/Badge.spec.ts
git commit -m "feat(ui): add overlay (dark-glass) treatment to Badge"
```

---

### Task 3: Make AnimeKebab geometry-flexible

The new cards need the kebab in non-default positions (centered cluster on PosterCard, vertically-centered glass on PosterRow). Add a `class` passthrough merged via `cn()` so callers fully control geometry, while existing callers (`AnimeCardNew`, `ColumnItem`) keep today's exact look.

**Files:**
- Modify: `src/components/anime/AnimeKebab.vue`
- Test: `src/components/anime/AnimeKebab.spec.ts` (create)

- [ ] **Step 1: Write the failing test**

Create `src/components/anime/AnimeKebab.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AnimeKebab from './AnimeKebab.vue'

describe('AnimeKebab', () => {
  it('keeps the default top-right geometry', () => {
    const w = mount(AnimeKebab)
    const cls = w.find('button').classes()
    expect(cls).toContain('absolute')
    expect(cls).toContain('top-2')
    expect(cls).toContain('right-2')
    expect(cls).toContain('w-9')
    expect(cls).toContain('h-9')
  })

  it('lets a caller override size/position via class', () => {
    const w = mount(AnimeKebab, { props: { class: 'static w-12 h-12' } })
    const cls = w.find('button').classes()
    // tailwind-merge resolves the conflicts in favour of the caller
    expect(cls).toContain('w-12')
    expect(cls).toContain('h-12')
    expect(cls).not.toContain('w-9')
    expect(cls).not.toContain('h-9')
    expect(cls).toContain('static')
    expect(cls).not.toContain('absolute')
  })

  it('emits open with the button element on click', async () => {
    const w = mount(AnimeKebab)
    await w.find('button').trigger('click')
    const ev = w.emitted('open')
    expect(ev).toBeTruthy()
    expect(ev![0][0]).toBeInstanceOf(HTMLButtonElement)
  })
})
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/AnimeKebab.spec.ts`
Expected: FAIL — the override test fails (no `class` merge; `extraClass` does not use tailwind-merge so `w-9` survives).

- [ ] **Step 3: Refactor AnimeKebab.vue to merge via cn()**

Replace the entire file contents with:

```vue
<template>
  <button
    ref="btnEl"
    type="button"
    :class="cn(
      'absolute z-20 w-9 h-9 rounded-full',
      'bg-black/65 backdrop-blur flex items-center justify-center',
      'text-white opacity-0 scale-90',
      'group-hover:opacity-100 group-hover:scale-100',
      'group-hover:animate-kebab-glow',
      'focus-visible:opacity-100 focus-visible:scale-100 focus-visible:animate-kebab-glow',
      'transition-all duration-200',
      'hover:bg-cyan-500/90 hover:rotate-[12deg] hover:scale-110',
      'pointer-events-auto',
      positionClass,
      props.extraClass,
      props.class,
    )"
    :aria-label="$t('contextMenu.openMenu')"
    aria-haspopup="menu"
    :aria-expanded="menuOpen"
    @click.prevent.stop="onActivate"
    @keydown.enter.prevent="onActivate"
    @keydown.space.prevent="onActivate"
  >
    <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
      <circle cx="10" cy="4" r="1.5" />
      <circle cx="10" cy="10" r="1.5" />
      <circle cx="10" cy="16" r="1.5" />
    </svg>
  </button>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    menuOpen?: boolean
    position?: 'top-right' | 'top-left' | 'bottom-right'
    extraClass?: string
    class?: HTMLAttributes['class']
  }>(),
  { menuOpen: false, position: 'top-right', extraClass: '' }
)

const emit = defineEmits<{ open: [el: HTMLElement] }>()

const btnEl = ref<HTMLButtonElement | null>(null)

const positionClass = computed(() => {
  switch (props.position) {
    case 'top-left': return 'top-2 left-2'
    case 'bottom-right': return 'bottom-2 right-2'
    case 'top-right':
    default: return 'top-2 right-2'
  }
})

function onActivate() {
  if (btnEl.value) emit('open', btnEl.value)
}

if (import.meta.env.DEV) {
  onMounted(() => {
    // Walk ancestors until we find a `.group` element. group-hover: matches
    // ANY ancestor with the class, not just the direct parent.
    let el: HTMLElement | null = btnEl.value?.parentElement ?? null
    let found = false
    while (el) {
      if (el.classList.contains('group')) { found = true; break }
      el = el.parentElement
    }
    if (!found) {
      console.warn(
        '[AnimeKebab] no ancestor element has the `group` Tailwind class — ' +
        'hover/focus reveal will not work. Add `class="group relative"` to the wrapper.'
      )
    }
  })
}
</script>
```

> Note: `positionClass` and `extraClass`/`class` are all passed into a single `cn()`, so `cn` (tailwind-merge) resolves geometry conflicts with the caller's `class` winning (it is listed last).

- [ ] **Step 4: Run the test, confirm it passes**

Run: `bunx vitest run src/components/anime/AnimeKebab.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Guard against a regression in existing callers**

Run the broader anime-component suite to confirm `AnimeCardNew`/`ColumnItem` rendering is unaffected:

Run: `bunx vitest run src/components/anime src/components/home`
Expected: PASS (no new failures vs. baseline).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/AnimeKebab.vue src/components/anime/AnimeKebab.spec.ts
git commit -m "refactor(anime): let AnimeKebab callers override geometry via cn() class passthrough"
```

---

## Phase 2 — Data contract

### Task 4: AnimeCardModel + mappers

**Files:**
- Create: `src/types/card.ts`
- Create: `src/utils/toCardModel.ts`
- Test: `src/utils/__tests__/toCardModel.spec.ts`

- [ ] **Step 1: Create the type contract**

Create `src/types/card.ts`:

```ts
// The single view-model every unified card (PosterCard / PosterRow / MediaTile)
// renders. Source-API shapes are normalised into this by src/utils/toCardModel.ts.
// Design contract: docs/superpowers/specs/2026-06-05-unified-anime-card-design.md

export type ListStatus =
  | 'watching'
  | 'plan_to_watch'
  | 'completed'
  | 'on_hold'
  | 'dropped'

export interface AnimeCardModel {
  id: string
  href: string                 // /anime/:id  (continue-watching appends ?episode=N)
  title: string                // already localized
  coverImage: string
  year?: number
  episodes?: number
  primaryGenre?: string        // already localized
  malScore?: number            // ★ amber  (Shikimori/MAL)
  siteScore?: number           // ◆ diamond cyan (AnimeEnigma reviews)
  quality?: string             // e.g. "1080p" — neutral overlay badge
  hasDub?: boolean             // DUB overlay badge
  listStatus?: ListStatus | null
  progress?: { current: number; total: number } | null
  airing?: boolean             // ONGOING (green + pulse) — only while true
  nextEpisode?: { ep: number; when: string } | null  // Row / MediaTile
}

// Per-user / per-listing overlays that don't live on the raw anime object
// (site rating map, watchlist status map, progress map). Merged in by the
// parent when it builds a model.
export interface CardExtras {
  siteScore?: number
  listStatus?: ListStatus | null
  progress?: { current: number; total: number } | null
}
```

- [ ] **Step 2: Write the failing mapper tests**

Create `src/utils/__tests__/toCardModel.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'

// getLocalizedTitle/getLocalizedGenre read the active i18n locale; stub them to
// deterministic identity so the mappers are tested in isolation.
vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, ru?: string, jp?: string) => name || ru || jp || '',
  getLocalizedGenre: (name?: string, ru?: string) => name || ru || '',
}))

import { fromCatalogAnime, fromHomeAnime, fromContinueWatching } from '../toCardModel'

describe('fromCatalogAnime', () => {
  const base = {
    id: 'a1',
    title: 'Fallback',
    name: 'Frieren',
    coverImage: 'http://x/p.jpg',
    rating: 8.9,
    releaseYear: 2023,
    totalEpisodes: 28,
    episodesAired: 28,
    rawGenres: [{ name: 'Adventure' }],
    status: 'released',
    hasVideo: true,
    description: '',
    genres: [],
  }

  it('maps intrinsic fields', () => {
    const m = fromCatalogAnime(base as never)
    expect(m.id).toBe('a1')
    expect(m.href).toBe('/anime/a1')
    expect(m.title).toBe('Frieren')
    expect(m.coverImage).toBe('http://x/p.jpg')
    expect(m.year).toBe(2023)
    expect(m.episodes).toBe(28)
    expect(m.primaryGenre).toBe('Adventure')
    expect(m.malScore).toBe(8.9)
  })

  it('merges per-user extras', () => {
    const m = fromCatalogAnime(base as never, {
      siteScore: 9.4,
      listStatus: 'watching',
      progress: { current: 12, total: 28 },
    })
    expect(m.siteScore).toBe(9.4)
    expect(m.listStatus).toBe('watching')
    expect(m.progress).toEqual({ current: 12, total: 28 })
  })

  it('flags airing only for ongoing status', () => {
    const ongoing = fromCatalogAnime({ ...base, status: 'ongoing', nextEpisodeAt: '2026-06-10T12:00:00Z', episodesAired: 5 } as never)
    expect(ongoing.airing).toBe(true)
    expect(ongoing.nextEpisode).toEqual({ ep: 6, when: '2026-06-10T12:00:00Z' })
    expect(fromCatalogAnime(base as never).airing).toBe(false)
  })
})

describe('fromHomeAnime', () => {
  const h = {
    id: 'h1',
    name: 'Bocchi',
    poster_url: 'http://x/h.jpg',
    score: 8.2,
    episodes_count: 12,
    episodes_aired: 3,
    year: 2022,
    status: 'ongoing',
    next_episode_at: '2026-06-12T15:00:00Z',
  }

  it('maps fields and derives next episode', () => {
    const m = fromHomeAnime(h as never)
    expect(m.id).toBe('h1')
    expect(m.title).toBe('Bocchi')
    expect(m.coverImage).toBe('http://x/h.jpg')
    expect(m.episodes).toBe(12)
    expect(m.malScore).toBe(8.2)
    expect(m.airing).toBe(true)
    expect(m.nextEpisode).toEqual({ ep: 4, when: '2026-06-12T15:00:00Z' })
  })

  it('falls back to /placeholder.svg when poster missing', () => {
    const m = fromHomeAnime({ ...h, poster_url: undefined } as never)
    expect(m.coverImage).toBe('/placeholder.svg')
  })
})

describe('fromContinueWatching', () => {
  const item = {
    anime: { id: 'c1', name: 'Dandadan', poster_url: 'http://x/c.jpg', episodes_count: 12 },
    episode_number: 5,
    progress: 600,
    duration: 1400,
  }

  it('builds an episode-deep href and progress', () => {
    const m = fromContinueWatching(item as never)
    expect(m.id).toBe('c1')
    expect(m.title).toBe('Dandadan')
    expect(m.href).toBe('/anime/c1?episode=5')
    expect(m.nextEpisode).toEqual({ ep: 5, when: '' })
    expect(m.progress).toEqual({ current: 5, total: 12 })
  })
})
```

- [ ] **Step 3: Run the tests, confirm they fail**

Run: `bunx vitest run src/utils/__tests__/toCardModel.spec.ts`
Expected: FAIL — `../toCardModel` does not exist.

- [ ] **Step 4: Implement the mappers**

Create `src/utils/toCardModel.ts`:

```ts
import { getLocalizedTitle, getLocalizedGenre } from '@/utils/title'
import type { AnimeCardModel, CardExtras, ListStatus } from '@/types/card'

// Minimal structural shapes the mappers accept. We intentionally keep these
// local + loose (rather than importing the full Anime/HomeAnime interfaces) so
// the normalizer has one job and does not couple to every optional API field.

interface CatalogAnimeLike {
  id: string | number
  title: string
  name?: string
  nameRu?: string
  nameJp?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  totalEpisodes?: number
  episodesAired?: number
  nextEpisodeAt?: string
  status?: string
  quality?: string
  hasDub?: boolean
  genres?: string[]
  rawGenres?: { name?: string; nameRu?: string }[]
}

interface HomeAnimeLike {
  id: string | number
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  next_episode_at?: string
}

interface ContinueWatchingLike {
  anime: HomeAnimeLike
  episode_number: number
  progress?: number
  duration?: number
}

const PLACEHOLDER = '/placeholder.svg'

function isAiring(status?: string): boolean {
  return status === 'ongoing'
}

export function fromCatalogAnime(a: CatalogAnimeLike, extras?: CardExtras): AnimeCardModel {
  const id = String(a.id)
  const primaryGenre = a.rawGenres?.length
    ? getLocalizedGenre(a.rawGenres[0].name, a.rawGenres[0].nameRu)
    : a.genres?.[0]
  const airing = isAiring(a.status)
  return {
    id,
    href: `/anime/${id}`,
    title:
      a.name || a.nameRu || a.nameJp
        ? getLocalizedTitle(a.name, a.nameRu, a.nameJp)
        : a.title,
    coverImage: a.coverImage || PLACEHOLDER,
    year: a.releaseYear || undefined,
    episodes: a.totalEpisodes || undefined,
    primaryGenre: primaryGenre || undefined,
    malScore: a.rating || undefined,
    siteScore: extras?.siteScore,
    quality: a.quality || undefined,
    hasDub: a.hasDub || undefined,
    listStatus: extras?.listStatus ?? null,
    progress: extras?.progress ?? null,
    airing,
    nextEpisode:
      airing && a.nextEpisodeAt
        ? { ep: (a.episodesAired || 0) + 1, when: a.nextEpisodeAt }
        : null,
  }
}

export function fromHomeAnime(a: HomeAnimeLike, extras?: CardExtras): AnimeCardModel {
  const id = String(a.id)
  const airing = isAiring(a.status)
  return {
    id,
    href: `/anime/${id}`,
    title: getLocalizedTitle(a.name, a.name_ru, a.name_jp) || '',
    coverImage: a.poster_url || PLACEHOLDER,
    year: a.year || undefined,
    episodes: a.episodes_count || undefined,
    malScore: a.score || undefined,
    siteScore: extras?.siteScore,
    listStatus: extras?.listStatus ?? null,
    progress: extras?.progress ?? null,
    airing,
    nextEpisode:
      airing && a.next_episode_at
        ? { ep: (a.episodes_aired || 0) + 1, when: a.next_episode_at }
        : null,
  }
}

export function fromContinueWatching(item: ContinueWatchingLike): AnimeCardModel {
  const a = item.anime
  const id = String(a.id)
  const total = a.episodes_count || 0
  return {
    id,
    href: `/anime/${id}?episode=${item.episode_number}`,
    title: getLocalizedTitle(a.name, a.name_ru, a.name_jp) || '',
    coverImage: a.poster_url || PLACEHOLDER,
    // For continue-watching the "next episode" slot carries the resume episode;
    // `when` is unused by MediaTile variant ② so we leave it empty.
    nextEpisode: { ep: item.episode_number, when: '' },
    progress: { current: item.episode_number, total },
    listStatus: null,
    airing: false,
  }
}

export type { ListStatus }
```

- [ ] **Step 5: Run the tests, confirm they pass**

Run: `bunx vitest run src/utils/__tests__/toCardModel.spec.ts`
Expected: PASS (all describe blocks green).

- [ ] **Step 6: Type-check**

Run: `bunx vue-tsc --noEmit`
Expected: no errors introduced by the new files.

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/types/card.ts src/utils/toCardModel.ts src/utils/__tests__/toCardModel.spec.ts
git commit -m "feat(cards): add AnimeCardModel contract + source mappers"
```

---

## Phase 3 — PosterCard + migrations (lowest-risk, proves parity)

### Task 5: PosterImage primitive

**Files:**
- Create: `src/components/anime/PosterImage.vue`
- Test: `src/components/anime/PosterImage.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `src/components/anime/PosterImage.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PosterImage from './PosterImage.vue'

describe('PosterImage', () => {
  it('shows the drift skeleton until the image loads', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/p.jpg', alt: 'P' } })
    expect(w.find('.sk-drift').exists()).toBe(true)
    expect(w.find('img').classes()).toContain('opacity-0')
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
    expect(w.find('img').classes()).toContain('opacity-100')
  })

  it('applies the requested aspect ratio', () => {
    const w = mount(PosterImage, { props: { src: 'x', alt: 'a', ratio: '16/9' } })
    expect(w.classes()).toContain('aspect-[16/9]')
  })

  it('renders default-slot overlay content', () => {
    const w = mount(PosterImage, {
      props: { src: 'x', alt: 'a' },
      slots: { default: '<span class="ov-test">hi</span>' },
    })
    expect(w.find('.ov-test').exists()).toBe(true)
  })

  it('swaps to the fallback url once on error', async () => {
    const w = mount(PosterImage, { props: { src: 'http://x/bad.jpg', alt: 'a' } })
    const img = w.find('img')
    await img.trigger('error')
    // dataset guard prevents an infinite error loop
    expect((img.element as HTMLImageElement).dataset.fallback).toBe('1')
  })
})
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/PosterImage.spec.ts`
Expected: FAIL — component does not exist.

- [ ] **Step 3: Implement PosterImage.vue**

Create `src/components/anime/PosterImage.vue`:

```vue
<template>
  <div
    class="relative overflow-hidden"
    :class="[ratioClass, roundedClass]"
    :style="{ backgroundColor: 'var(--color-surface)' }"
  >
    <!-- Drift skeleton placeholder — its OWN element, so it never shares a
         `background` declaration with the container (the cascade bug). -->
    <div
      v-if="!loaded"
      class="absolute inset-0 sk-drift"
      :class="roundedClass"
      aria-hidden="true"
    />

    <img
      v-if="src"
      :src="src"
      :alt="alt"
      loading="lazy"
      class="absolute inset-0 w-full h-full object-cover transition-opacity duration-300"
      :class="loaded ? 'opacity-100' : 'opacity-0'"
      @load="loaded = true"
      @error="onError"
    />

    <!-- Optional scrims for legible overlay content on bright posters -->
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 top-0 h-16 bg-gradient-to-b from-black/55 to-transparent" />
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-black/75 to-transparent" />

    <slot />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { getImageFallbackUrl } from '@/composables/useImageProxy'

const props = withDefaults(
  defineProps<{
    src: string
    alt: string
    ratio?: '2/3' | '16/9'
    rounded?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
    scrim?: boolean
  }>(),
  { ratio: '2/3', rounded: 'none', scrim: false }
)

const loaded = ref(false)

const ratioClass = computed(() => (props.ratio === '16/9' ? 'aspect-[16/9]' : 'aspect-[2/3]'))
const roundedClass = computed(() => {
  const map = { none: '', sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl' }
  return map[props.rounded]
})

function onError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.dataset.fallback) {
    img.dataset.fallback = '1'
    img.src = getImageFallbackUrl(props.src)
  }
}
</script>
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `bunx vitest run src/components/anime/PosterImage.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/PosterImage.vue src/components/anime/PosterImage.spec.ts
git commit -m "feat(anime): add PosterImage primitive (lazy img + drift skeleton + scrims)"
```

---

### Task 6: PosterCard component

Faithful to `AnimeCardNew` behavior plus the locked design deltas: overlay (dark-glass) score badges that **stay visible on hover**, ★ amber MAL + ◆ diamond cyan site score (snug, `tabular-nums`), a centered **equal-size** play+kebab cluster, and a drift skeleton via `PosterImage`. Renders a single `AnimeCardModel`.

**Files:**
- Create: `src/components/anime/PosterCard.vue`
- Test: `src/components/anime/PosterCard.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `src/components/anime/PosterCard.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterCard from './PosterCard.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountCard(model: Partial<AnimeCardModel> = {}) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1', title: 'Frieren', coverImage: 'http://x/p.jpg',
    year: 2023, episodes: 28, primaryGenre: 'Adventure',
    malScore: 8.9, siteScore: 9.4, listStatus: null, progress: null, airing: false,
    ...model,
  }
  return mount(PosterCard, {
    props: { model: full },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('PosterCard', () => {
  it('renders title, meta and a link to the anime', () => {
    const w = mountCard()
    expect(w.text()).toContain('Frieren')
    expect(w.text()).toContain('2023')
    expect(w.text()).toContain('Adventure')
    expect(w.find('a').attributes('href')).toBe('/anime/1')
  })

  it('shows both scores with tabular-nums (alignment)', () => {
    const w = mountCard()
    const scoreEls = w.findAll('[data-testid="score"]')
    expect(scoreEls.length).toBe(2)
    scoreEls.forEach((el) => expect(el.classes()).toContain('tabular-nums'))
  })

  it('keeps scores visible on hover (no opacity-0 on the score cluster)', () => {
    const w = mountCard()
    const cluster = w.find('[data-testid="score-cluster"]')
    expect(cluster.classes()).not.toContain('opacity-0')
    expect(cluster.classes()).not.toContain('group-hover:opacity-0')
  })

  it('renders the centered play+kebab cluster with the kebab', () => {
    const w = mountCard()
    expect(w.find('[data-testid="play-cluster"]').exists()).toBe(true)
    expect(w.findComponent({ name: 'AnimeKebab' }).exists()).toBe(true)
  })

  it('emits openMenu with the kebab element', async () => {
    const w = mountCard()
    await w.findComponent({ name: 'AnimeKebab' }).find('button').trigger('click')
    expect(w.emitted('openMenu')).toBeTruthy()
  })

  it('renders the ongoing badge only while airing', () => {
    expect(mountCard({ airing: true }).find('[data-testid="ongoing"]').exists()).toBe(true)
    expect(mountCard({ airing: false }).find('[data-testid="ongoing"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/PosterCard.spec.ts`
Expected: FAIL — component does not exist.

- [ ] **Step 3: Implement PosterCard.vue**

Create `src/components/anime/PosterCard.vue`:

```vue
<template>
  <div class="group block relative">
    <!-- Full-card SPA link, sits behind the kebab cluster -->
    <router-link
      :to="model.href"
      class="absolute inset-0 z-0 rounded-xl"
      :aria-label="model.title"
    />

    <div class="card-hover rounded-xl overflow-hidden bg-white/5 border border-white/10 pointer-events-none">
      <PosterImage
        :src="model.coverImage"
        :alt="model.title"
        ratio="2/3"
        scrim
      >
        <!-- Top-left: quality + DUB stack -->
        <div class="absolute top-2 left-2 flex flex-col items-start gap-1">
          <Badge v-if="model.quality" variant="default" size="sm" overlay>{{ model.quality }}</Badge>
          <Badge v-if="model.hasDub" variant="default" size="sm" overlay>{{ $t('card.dubBadge') }}</Badge>
          <Badge v-if="model.airing" variant="success" size="sm" overlay data-testid="ongoing" class="gap-1">
            <span class="inline-block w-1.5 h-1.5 rounded-full bg-success animate-pulse" />
            {{ $t('home.airing') }}
          </Badge>
        </div>

        <!-- Top-right: score cluster — STAYS visible on hover -->
        <div
          v-if="model.malScore || model.siteScore"
          data-testid="score-cluster"
          class="absolute top-2 right-2 flex flex-col items-end gap-1"
        >
          <Badge
            v-if="model.malScore"
            variant="warning"
            size="sm"
            overlay
            data-testid="score"
            class="gap-1 tabular-nums"
          >
            <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ model.malScore.toFixed(1) }}
          </Badge>
          <Badge
            v-if="model.siteScore"
            variant="primary"
            size="sm"
            overlay
            data-testid="score"
            class="gap-1 tabular-nums"
          >
            <!-- ◆ diamond = AnimeEnigma score -->
            <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 2l9 10-9 10L3 12z" />
            </svg>
            {{ model.siteScore.toFixed(1) }}
          </Badge>
        </div>

        <!-- Bottom-left: watchlist status + progress -->
        <div
          v-if="model.listStatus || progressText"
          class="absolute bottom-2 left-2 flex flex-col gap-1 items-start"
        >
          <Badge v-if="model.listStatus" :variant="statusVariant" size="sm" overlay>
            {{ $t(statusKey) }}
          </Badge>
          <Badge v-if="progressText" variant="default" size="sm" overlay>{{ progressText }}</Badge>
        </div>

        <!-- Centered play + kebab cluster, equal size, hover reveal -->
        <div
          data-testid="play-cluster"
          class="absolute inset-0 flex items-center justify-center gap-3 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none"
        >
          <span
            class="w-12 h-12 rounded-full bg-cyan-500/90 flex items-center justify-center shadow-[0_0_20px_rgba(0,212,255,0.5)]"
            aria-hidden="true"
          >
            <svg class="w-5 h-5 text-white ml-0.5" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
          </span>
          <AnimeKebab
            :menu-open="menuOpen"
            class="static opacity-100 scale-100 w-12 h-12"
            @open="(el: HTMLElement) => emit('openMenu', el)"
          />
        </div>
      </PosterImage>

      <!-- Content -->
      <div class="p-3">
        <h3 class="font-medium text-white line-clamp-2 mb-1 group-hover:text-cyan-400 transition-colors">
          {{ model.title }}
        </h3>
        <div class="flex items-center gap-2 text-xs text-white/50">
          <span v-if="model.year">{{ model.year }}</span>
          <span v-if="model.year && model.episodes" class="text-white/30">•</span>
          <span v-if="model.episodes">{{ model.episodes }} {{ $t('anime.episode') }}</span>
          <span v-if="model.episodes && model.primaryGenre" class="text-white/30">•</span>
          <span v-if="model.primaryGenre">{{ model.primaryGenre }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from '@/components/ui/Badge.vue'
import AnimeKebab from './AnimeKebab.vue'
import PosterImage from './PosterImage.vue'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{
  model: AnimeCardModel
  menuOpen?: boolean
}>()

const emit = defineEmits<{ openMenu: [el: HTMLElement] }>()

const { t } = useI18n()

const statusKey = computed(() => {
  const map: Record<string, string> = {
    watching: 'profile.watchlist.watching',
    plan_to_watch: 'profile.watchlist.planToWatch',
    completed: 'profile.watchlist.completed',
    on_hold: 'profile.watchlist.onHold',
    dropped: 'profile.watchlist.dropped',
  }
  return map[props.model.listStatus || ''] || ''
})

const statusVariant = computed<'primary' | 'success' | 'warning' | 'destructive' | 'default'>(() => {
  switch (props.model.listStatus) {
    case 'watching': return 'primary'
    case 'completed': return 'success'
    case 'on_hold': return 'warning'
    case 'dropped': return 'destructive'
    default: return 'default'
  }
})

const progressText = computed(() => {
  const p = props.model.progress
  if (!p || p.current <= 0) return ''
  if (props.model.listStatus === 'completed') return ''
  return t('card.episodeProgress', { n: p.current, total: p.total || '?' })
})
</script>
```

> Note `t` is referenced in `<script>` (progressText/statusKey via `$t` in template and `t(...)` in script). Both `t` and `$t` resolve to the same i18n instance — the spec test mocks `useI18n().t`, and `$t` in templates is provided by the i18n plugin which is not installed in the unit test, so the template uses `$t` only for static labels asserted via key strings. To keep the test green without the i18n plugin, the test stubs `useI18n`; `$t` calls in the template will throw only if invoked. Guard: in the test, all `$t`-bearing branches that are asserted (`ongoing`) pass literal keys. If `$t` is undefined under test, switch those template `$t(...)` calls to the script-level `t(...)` via computed labels. **Verify by running the test in Step 4; if `$t` is not a function under test, refactor the asserted labels to computeds** (e.g. add `const airingLabel = computed(() => t('home.airing'))` and render `{{ airingLabel }}`).

- [ ] **Step 4: Run the test, confirm it passes**

Run: `bunx vitest run src/components/anime/PosterCard.spec.ts`
Expected: PASS (7 tests). If any `$t`-dependent assertion fails because the i18n plugin isn't mounted, apply the Step 3 guard (move asserted labels to script-level `t()` computeds) and re-run.

- [ ] **Step 5: Type-check**

Run: `bunx vue-tsc --noEmit`
Expected: no new errors.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/PosterCard.vue src/components/anime/PosterCard.spec.ts
git commit -m "feat(anime): add PosterCard (2/3 unified catalog card)"
```

---

### Task 7: Export PosterCard + migrate Browse.vue

**Files:**
- Modify: `src/components/anime/index.ts`
- Modify: `src/views/Browse.vue`

- [ ] **Step 1: Add the export**

In `src/components/anime/index.ts`, add after the `AnimeCardNew` line:

```ts
export { default as PosterCard } from './PosterCard.vue'
export { default as PosterRow } from './PosterRow.vue'
export { default as MediaTile } from './MediaTile.vue'
export { default as PosterImage } from './PosterImage.vue'
```

> `PosterRow`/`MediaTile` files are created in later phases. If executing strictly phase-by-phase and committing between phases, add only the `PosterCard` + `PosterImage` exports now and add the others in their phases. (vue-tsc will error on a re-export of a not-yet-created file.)

- [ ] **Step 2: Migrate the Browse grid**

In `src/views/Browse.vue`, change the import on line 219:

```ts
import { PosterCard, AnimeContextMenu } from '@/components/anime'
```

Then replace the grid block (lines 130-142) with:

```vue
              <PosterCard
                v-for="anime in animeList"
                :key="anime.id"
                :model="browseCardModel(anime)"
                :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
                @open-menu="(el: HTMLElement) => openContextMenuAt(el, anime, { listStatus: getListStatus(anime.id), siteRating: siteRatings[String(anime.id)] })"
                @touchstart="onTouchstart($event, anime, { listStatus: getListStatus(anime.id), siteRating: siteRatings[String(anime.id)] })"
                @touchmove="onTouchmove"
                @touchend="onTouchend"
              />
```

- [ ] **Step 3: Add the model builder to Browse's script**

Add this import near the other imports in `src/views/Browse.vue`:

```ts
import { fromCatalogAnime } from '@/utils/toCardModel'
import type { ListStatus } from '@/types/card'
```

Add this helper in the `<script setup>` body (after `getListStatus` is defined; reuse the existing `animeList`, `siteRatings`, `browseProgress`, `getListStatus` references):

```ts
function browseCardModel(anime: (typeof animeList.value)[number]) {
  const id = String(anime.id)
  const sr = siteRatings[id]
  const pe = browseProgress.value?.get?.(id) ?? browseProgress.get?.(id) ?? null
  return fromCatalogAnime(anime as never, {
    siteScore: sr && sr.total_reviews > 0 ? sr.average_score : undefined,
    listStatus: (getListStatus(anime.id) as ListStatus | null) ?? null,
    progress: pe && pe.latest_episode > 0
      ? { current: pe.latest_episode, total: pe.episodes_count || pe.episodes_aired || 0 }
      : null,
  })
}
```

> Adjust the `browseProgress` access to match its actual type (it is a `Map` per the template `browseProgress.get(...)`). If `browseProgress` is a plain `Map` (not a ref), use `browseProgress.get(id) ?? null` only. Confirm by reading the declaration of `browseProgress` in `Browse.vue` before writing this; the `??` chain above is defensive and should be reduced to the correct single form.

- [ ] **Step 4: Type-check + unit suite**

Run: `bunx vue-tsc --noEmit && bunx vitest run src/components/anime`
Expected: no type errors; anime suite green.

- [ ] **Step 5: In-browser smoke (REQUIRED — DS-NF-06)**

jsdom cannot catch Tailwind cascade bugs. Verify the live Browse grid at desktop + mobile widths:

```bash
cd /data/animeenigma && make redeploy-web
```

Then open the site, go to `/browse`, and confirm: posters load with the drift shimmer beforehand, both score badges are visible (including on hover), the centered play+kebab cluster appears on hover, the kebab opens the context menu, and progress/status badges render. Check a narrow viewport too.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/index.ts src/views/Browse.vue
git commit -m "feat(browse): render unified PosterCard via toCardModel"
```

---

### Task 8: Migrate Anime.vue related carousel

**Files:**
- Modify: `src/views/Anime.vue`

- [ ] **Step 1: Read the current usage**

Read `src/views/Anime.vue` lines 1000-1060 to capture the exact `AnimeCardNew` block, its surrounding `CarouselItem`, the props passed, and the `openMenu` handler name + the `anime` variable in scope.

- [ ] **Step 2: Swap the import**

Change line 1057:

```ts
import { GenreChip, PosterCard, AnimeContextMenu } from '@/components/anime'
```

- [ ] **Step 3: Replace the card usage**

Replace the `<AnimeCardNew .../>` element (around line 1006) with a `PosterCard` that builds its model from the related-anime item in scope. Use the exact prop/handler names found in Step 1. Template shape:

```vue
              <PosterCard
                :model="fromCatalogAnime(related, {
                  siteRating ? undefined : undefined
                })"
                :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(related.id)"
                @open-menu="(el: HTMLElement) => /* existing openMenu handler */(el, related)"
              />
```

> Replace `related` with the actual v-for item identifier and `/* existing openMenu handler */` with the real handler name. The related carousel does not carry per-user site ratings/progress in this view, so pass `fromCatalogAnime(related)` with no extras (drop the empty extras object). Confirm whether the related items are catalog `Anime` shaped (they are, since this view previously fed them straight to `AnimeCardNew`).

- [ ] **Step 4: Add the mapper import**

```ts
import { fromCatalogAnime } from '@/utils/toCardModel'
```

- [ ] **Step 5: Type-check + smoke**

Run: `bunx vue-tsc --noEmit`
Then `cd /data/animeenigma && make redeploy-web` and verify the related carousel on an anime detail page renders identically (posters, scores, hover cluster, kebab menu).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/views/Anime.vue
git commit -m "feat(anime-detail): render related carousel with unified PosterCard"
```

---

## Phase 4 — PosterRow + Home rails

### Task 9: PosterRow component

Models the hero rail (`ColumnItem`): 56×84 poster + body, with three variants (`ongoing`, `top`, `announced`), a **1-line title + fixed row height** so info blocks align across columns, the hero info set (ongoing chip · ★/◆ scores · next-ep line · season chip · rank numeral), and a **centered-glass kebab** vertically centered on the right edge.

**Files:**
- Create: `src/components/anime/PosterRow.vue`
- Test: `src/components/anime/PosterRow.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `src/components/anime/PosterRow.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterRow from './PosterRow.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountRow(model: Partial<AnimeCardModel> = {}, variant: 'ongoing' | 'top' | 'announced' = 'ongoing', rank?: number) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1', title: 'Frieren: Beyond Journey\'s End',
    coverImage: 'http://x/p.jpg', year: 2023, episodes: 28,
    malScore: 8.9, siteScore: 9.4, airing: true,
    nextEpisode: { ep: 6, when: '2026-06-10T12:00:00Z' }, listStatus: null, progress: null,
    ...model,
  }
  return mount(PosterRow, {
    props: { model: full, variant, rank },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('PosterRow', () => {
  it('renders a single-line title (truncate class)', () => {
    const w = mountRow()
    expect(w.find('[data-testid="row-title"]').classes()).toContain('truncate')
  })

  it('shows the ongoing chip + next-ep line for the ongoing variant', () => {
    const w = mountRow({}, 'ongoing')
    expect(w.find('[data-testid="airing"]').exists()).toBe(true)
    expect(w.find('[data-testid="next-ep"]').exists()).toBe(true)
  })

  it('shows the rank numeral for the top variant', () => {
    const w = mountRow({}, 'top', 1)
    expect(w.find('[data-testid="rank"]').text()).toBe('1')
  })

  it('shows the season chip for the announced variant', () => {
    const w = mountRow({ season: 'winter' } as never, 'announced')
    expect(w.find('[data-testid="season"]').exists()).toBe(true)
  })

  it('renders the centered-glass kebab and emits openMenu', async () => {
    const w = mountRow()
    const kebab = w.find('[data-testid="row-kebab"]')
    expect(kebab.exists()).toBe(true)
    await kebab.trigger('click')
    expect(w.emitted('openMenu')).toBeTruthy()
  })
})
```

> The `season` field is variant-specific and not on `AnimeCardModel`. PosterRow receives `season` via a dedicated optional prop (not the model) — see implementation. Update the test's `mountRow` to pass `season` as a prop. (The `{ season: 'winter' } as never` above is a placeholder; in Step 3 the season comes from a `season?: string` prop, so change that test to `mount(PosterRow, { props: { model: full, variant: 'announced', season: 'winter' }, ... })`.)

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/PosterRow.spec.ts`
Expected: FAIL — component does not exist.

- [ ] **Step 3: Implement PosterRow.vue**

Create `src/components/anime/PosterRow.vue`:

```vue
<template>
  <router-link :to="model.href" class="prow group" :class="{ 'is-top3': variant === 'top' && rank !== undefined && rank <= 3 }">
    <!-- rank numeral (top variant) -->
    <div v-if="variant === 'top' && rank !== undefined" class="rank" aria-hidden="true" data-testid="rank">{{ rank }}</div>

    <!-- centered-glass kebab -->
    <button
      ref="kebabEl"
      type="button"
      class="rkc"
      data-testid="row-kebab"
      :aria-label="$t('contextMenu.openMenu')"
      aria-haspopup="menu"
      :aria-expanded="menuOpen"
      @click.prevent.stop="onKebab"
    >
      <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <circle cx="10" cy="4" r="1.5" />
        <circle cx="10" cy="10" r="1.5" />
        <circle cx="10" cy="16" r="1.5" />
      </svg>
    </button>

    <img :src="posterSrc" :alt="model.title" class="poster" loading="lazy" @error="onPosterError" />

    <div class="body">
      <div class="title" data-testid="row-title">{{ model.title }}</div>

      <div class="meta">
        <span v-if="model.year">{{ model.year }}</span>
        <template v-if="model.year && model.episodes"><span class="sep">·</span></template>
        <span v-if="model.episodes">{{ $t('home.episodeCount', { count: model.episodes }) }}</span>
      </div>

      <div class="chips">
        <span v-if="variant === 'ongoing' && model.airing" class="chip airing" data-testid="airing">● {{ $t('home.airing') }}</span>
        <template v-if="variant === 'announced'">
          <span class="chip announced">{{ $t('home.announced') }}</span>
          <span v-if="season" class="chip season" data-testid="season">{{ $t(`seasons.${season}`) }}</span>
        </template>
        <template v-if="variant !== 'announced'">
          <span v-if="model.malScore" class="chip score tabular-nums">
            <svg width="10" height="10" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ model.malScore.toFixed(1) }}
          </span>
          <span v-if="model.siteScore" class="chip score site-score tabular-nums">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 2l9 10-9 10L3 12z" />
            </svg>
            {{ model.siteScore.toFixed(1) }}
          </span>
        </template>
      </div>

      <div v-if="variant === 'ongoing' && model.nextEpisode" class="next-ep" data-testid="next-ep">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ $t('home.nextEpisodeLine', { n: model.nextEpisode.ep, when: formattedNextEp }) }}
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{
  model: AnimeCardModel
  variant: 'ongoing' | 'top' | 'announced'
  rank?: number
  menuOpen?: boolean
  season?: string
}>()

const emit = defineEmits<{ openMenu: [el: HTMLElement] }>()

const { t } = useI18n()
const kebabEl = ref<HTMLButtonElement | null>(null)

function onKebab() {
  if (kebabEl.value) emit('openMenu', kebabEl.value)
}

const posterFailed = ref(false)
const posterSrc = computed(() => (!posterFailed.value && props.model.coverImage ? props.model.coverImage : '/placeholder.svg'))
function onPosterError() { posterFailed.value = true }

const formattedNextEp = computed(() => {
  const when = props.model.nextEpisode?.when
  if (!when) return ''
  const date = new Date(when)
  const now = new Date()
  const diffDays = Math.floor((date.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
  const timeStr = date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' })
  if (diffDays === 0) return t('home.todayAt', { time: timeStr })
  if (diffDays === 1) return t('home.tomorrowAt', { time: timeStr })
  if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('home.dayAt', { day: t(`schedule.daysShort.${dayKeys[date.getDay()]}`), time: timeStr })
  }
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
})
</script>

<style scoped>
.prow {
  position: relative;
  display: grid;
  grid-template-columns: 56px 1fr;
  gap: 12px;
  padding: 10px;
  border-radius: 12px;
  transition: background 0.15s ease;
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  overflow: hidden;
  flex-shrink: 0;
  align-items: start;
}
.prow:hover { background: rgba(255, 255, 255, 0.03); }

.poster {
  width: 56px;
  aspect-ratio: 2 / 3;
  height: 84px;
  overflow: hidden;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid var(--line);
  flex-shrink: 0;
  position: relative;
  z-index: 1;
}

.body { min-width: 0; display: flex; flex-direction: column; gap: 4px; position: relative; z-index: 1; }

/* 1-line title → fixed body rhythm → info aligns across columns */
.title {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.prow:hover .title { color: var(--brand-cyan); }

.meta { font-size: 11px; color: var(--muted-foreground); display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.sep { opacity: 0.5; }

.chips { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; margin-top: 2px; }
.chip {
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  padding: 2px 6px;
  border-radius: 4px;
  text-transform: uppercase;
}
.chip.airing    { background: rgba(0, 255, 157, 0.14); color: var(--color-success); }
.chip.announced { background: rgba(0, 212, 255, 0.14); color: var(--brand-cyan); }
.chip.season    { background: rgba(167, 139, 250, 0.14); color: var(--violet); }
.chip.score { background: rgba(255, 214, 0, 0.14); color: var(--color-warning); display: inline-flex; align-items: center; gap: 4px; }
.chip.site-score { background: rgba(0, 212, 255, 0.14); color: var(--brand-cyan); }

.next-ep {
  font-family: var(--f-mono);
  font-size: 10px;
  color: var(--brand-cyan);
  letter-spacing: 0.04em;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 2px;
}

.rank {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  font-family: var(--f-display);
  font-weight: 800;
  font-size: 56px;
  letter-spacing: -0.04em;
  color: rgba(255, 255, 255, 0.04);
  pointer-events: none;
  line-height: 1;
  z-index: 0;
  user-select: none;
}
.is-top3 .rank { color: rgba(0, 212, 255, 0.08); }

/* Centered-glass kebab — vertically centered on the right edge, hover reveal */
.rkc {
  position: absolute;
  top: 50%;
  right: 8px;
  z-index: 6;
  width: 34px;
  height: 34px;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(6px);
  color: #fff;
  display: grid;
  place-items: center;
  transform: translateY(-50%);
  opacity: 0;
  transition: opacity 0.18s ease, background 0.18s ease;
}
.prow:hover .rkc { opacity: 1; }
.rkc:hover { background: rgba(0, 212, 255, 0.9); }
</style>
```

> The `.rkc` hover-reveal uses `.prow:hover` (CSS), not `group-hover`, so it works without Tailwind's group. The `#fff` literal in `.rkc` is inside a `<style scoped>` block of a `.vue` file — the DS lint scans `.vue` files for hex. **Either** use `color: var(--foreground)` instead of `#fff` (preferred — no allowlist needed), **or** add an allowlist line. Use `var(--foreground)`; update `.rkc { color: var(--foreground); }`. Re-confirm no other raw hex remains in this file before committing (the `rgba(...)` values are fine — the lint only flags `#hex`).

- [ ] **Step 4: Fix the season test + run**

Update `PosterRow.spec.ts` `mountRow` so the announced test passes `season` as a prop (per the note in Step 1), then:

Run: `bunx vitest run src/components/anime/PosterRow.spec.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: DS lint this file**

Run: `cd /data/animeenigma && bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS=0`. If it flags a hex in `PosterRow.vue`, replace it with a token per Step 3's note and re-run.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/PosterRow.vue src/components/anime/PosterRow.spec.ts
git commit -m "feat(anime): add PosterRow (hero-rail row, 3 variants, centered-glass kebab)"
```

---

### Task 10: Migrate Home.vue rails to PosterRow

**Files:**
- Modify: `src/views/Home.vue`

- [ ] **Step 1: Read the Home script context**

Read `src/views/Home.vue` lines 145-200 to capture: the `AnimeContextMenu` binding, `openHomeMenuAt(el, anime)` (what shape it adapts `HomeAnime` → menu `anime`), the touch handler names, and the `siteRatings` shape. The PosterRow migration must keep all menu/touch plumbing intact.

- [ ] **Step 2: Swap the import**

Replace line 182:

```ts
import { PosterRow } from '@/components/anime'
```

(Remove the now-unused `import ColumnItem from '@/components/home/ColumnItem.vue'` line.)

Add the mapper import near the other imports:

```ts
import { fromHomeAnime } from '@/utils/toCardModel'
```

- [ ] **Step 3: Replace the three `ColumnItem` blocks**

Ongoing (lines 64-75):

```vue
          <PosterRow
            v-for="anime in ongoingAnime"
            :key="anime.id"
            :model="fromHomeAnime(anime, { siteScore: siteRatings[anime.id] && siteRatings[anime.id].total_reviews > 0 ? siteRatings[anime.id].average_score : undefined })"
            variant="ongoing"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
```

Top (lines 89-101) — same but `variant="top"` and `:rank="index + 1"`, iterating `v-for="(anime, index) in topAnime"`:

```vue
          <PosterRow
            v-for="(anime, index) in topAnime"
            :key="anime.id"
            :model="fromHomeAnime(anime, { siteScore: siteRatings[anime.id] && siteRatings[anime.id].total_reviews > 0 ? siteRatings[anime.id].average_score : undefined })"
            variant="top"
            :rank="index + 1"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
```

Announced (lines 115-125) — `variant="announced"` with `:season="anime.season"`:

```vue
          <PosterRow
            v-for="anime in announcedAnime"
            :key="anime.id"
            :model="fromHomeAnime(anime)"
            variant="announced"
            :season="anime.season"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
```

> The ongoing variant's episode-deep route (ColumnItem appended `?episode=N` for ongoing) is **not** carried by `fromHomeAnime`'s href. If preserving that deep-link is desired, add it back: in PosterRow, when `variant === 'ongoing' && model.nextEpisode`, link to `${model.href}?episode=${model.nextEpisode.ep}`. Decide per product intent; if keeping parity with ColumnItem, add a computed `to` in PosterRow:
> ```ts
> const to = computed(() => (props.variant === 'ongoing' && props.model.nextEpisode ? `${props.model.href}?episode=${props.model.nextEpisode.ep}` : props.model.href))
> ```
> and bind `:to="to"` instead of `:to="model.href"`. Add this to PosterRow in Task 9 if parity is required (recommended).

- [ ] **Step 4: Type-check + suite**

Run: `bunx vue-tsc --noEmit && bunx vitest run src/components/anime src/components/home`
Expected: green.

- [ ] **Step 5: In-browser smoke (REQUIRED)**

`cd /data/animeenigma && make redeploy-web`, then load `/` and verify all three home rails (ongoing / top / announced): rows align across columns, 1-line titles truncate, scores/chips/next-ep/rank render, the centered-glass kebab reveals on hover and opens the menu, and long titles don't break alignment. Check mobile width.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/views/Home.vue
git commit -m "feat(home): render rails with unified PosterRow"
```

---

## Phase 5 — MediaTile + continue-watching

### Task 11: MediaTile component

16/9 tile, variant ② (kicker + title + next-ep) overlaid on the poster, with an **overlay drift skeleton** (placeholder bars over a drifting poster) and a progress bar.

**Files:**
- Create: `src/components/anime/MediaTile.vue`
- Test: `src/components/anime/MediaTile.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `src/components/anime/MediaTile.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import MediaTile from './MediaTile.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountTile(model: Partial<AnimeCardModel> = {}, progressPct = 0) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1?episode=5', title: 'Dandadan', coverImage: 'http://x/p.jpg',
    episodes: 12, nextEpisode: { ep: 5, when: '' }, progress: { current: 5, total: 12 },
    listStatus: null, airing: false,
    ...model,
  }
  return mount(MediaTile, { props: { model: full, progressPct }, global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('MediaTile', () => {
  it('links to the episode-deep href', () => {
    expect(mountTile().find('a').attributes('href')).toBe('/anime/1?episode=5')
  })

  it('renders the kicker (episode) and the title', () => {
    const w = mountTile()
    expect(w.find('[data-testid="kicker"]').exists()).toBe(true)
    expect(w.text()).toContain('Dandadan')
  })

  it('renders the progress bar only when pct > 0', () => {
    expect(mountTile({}, 42).find('[data-testid="progress"]').exists()).toBe(true)
    expect(mountTile({}, 0).find('[data-testid="progress"]').exists()).toBe(false)
  })

  it('shows the drift skeleton until the image loads', async () => {
    const w = mountTile()
    expect(w.find('.sk-drift').exists()).toBe(true)
    await w.find('img').trigger('load')
    expect(w.find('.sk-drift').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/MediaTile.spec.ts`
Expected: FAIL — component does not exist.

- [ ] **Step 3: Implement MediaTile.vue**

Create `src/components/anime/MediaTile.vue`:

```vue
<template>
  <router-link :to="model.href" class="mtile group">
    <PosterImage :src="model.coverImage" :alt="model.title" ratio="16/9" rounded="lg" scrim>
      <!-- Centered play, hover reveal -->
      <span class="mtile-play" aria-hidden="true">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><path d="M5 3l14 9-14 9V3z" /></svg>
      </span>

      <!-- Info overlay (bottom) -->
      <div class="mtile-info">
        <div class="mtile-kicker" data-testid="kicker">
          {{ $t('home.continueWatchingEpisode', { n: model.nextEpisode ? model.nextEpisode.ep : 0 }) }}
          <template v-if="model.episodes"> · {{ model.episodes }}</template>
        </div>
        <div class="mtile-title">{{ model.title }}</div>
      </div>

      <!-- Progress bar -->
      <div v-if="progressPct > 0" class="mtile-progress" data-testid="progress">
        <div class="mtile-progress-fill" :style="{ width: progressPct + '%', minWidth: '4px' }" />
      </div>
    </PosterImage>
  </router-link>
</template>

<script setup lang="ts">
import PosterImage from './PosterImage.vue'
import type { AnimeCardModel } from '@/types/card'

defineProps<{
  model: AnimeCardModel
  progressPct?: number
}>()
</script>

<style scoped>
.mtile {
  position: relative;
  scroll-snap-align: start;
  display: block;
  border-radius: var(--r-lg);
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}
.mtile:hover { transform: translateY(-2px); box-shadow: var(--accent-glow); }
.mtile:focus-visible { outline: 2px solid var(--brand-cyan); outline-offset: 2px; }

.mtile-play {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 52px;
  height: 52px;
  border-radius: 999px;
  background: rgba(0, 212, 255, 0.95);
  display: grid;
  place-items: center;
  color: var(--background);
  opacity: 0;
  transition: opacity 0.2s ease, transform 0.2s ease;
  box-shadow: 0 0 24px rgba(0, 212, 255, 0.5);
  z-index: 2;
}
.mtile:hover .mtile-play { opacity: 1; transform: translate(-50%, -50%) scale(1.06); }

.mtile-info { position: absolute; left: 14px; right: 14px; bottom: 12px; z-index: 2; }
.mtile-kicker {
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.1em;
  color: var(--brand-cyan);
  text-transform: uppercase;
  margin-bottom: 4px;
}
.mtile-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mtile-progress { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgba(255, 255, 255, 0.08); z-index: 3; }
.mtile-progress-fill { height: 100%; background: var(--brand-cyan); box-shadow: 0 0 8px var(--brand-cyan); transition: width 0.3s ease; }
</style>
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `bunx vitest run src/components/anime/MediaTile.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: DS lint**

Run: `cd /data/animeenigma && bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS=0` (no raw hex in MediaTile.vue — all colors are tokens/rgba).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/MediaTile.vue src/components/anime/MediaTile.spec.ts
git commit -m "feat(anime): add MediaTile (16/9 continue-watching tile, variant 2)"
```

---

### Task 12: Migrate ContinueWatchingRow to MediaTile

Keep the section header, horizontal `cw-row` scroller, and loading skeleton; replace only the per-item `<router-link class="cw-card">…</router-link>` with `MediaTile`.

**Files:**
- Modify: `src/components/home/ContinueWatchingRow.vue`

- [ ] **Step 1: Replace the card loop**

Replace the `<router-link v-for ... class="cw-card"> … </router-link>` block (lines 26-63) with:

```vue
      <MediaTile
        v-for="item in items"
        :key="item.anime.id + ':' + item.episode_number"
        :model="fromContinueWatching(item)"
        :progress-pct="progressPct(item)"
        class="cw-card-tile"
      />
```

- [ ] **Step 2: Update the script imports**

In `<script setup>`, add:

```ts
import MediaTile from '@/components/anime/MediaTile.vue'
import { fromContinueWatching } from '@/utils/toCardModel'
```

Keep `progressPct(item)` and `useContinueWatching`. The `progressBarStyle` and `getLocalizedTitle` helpers become unused — remove them (and the now-unused `getLocalizedTitle` import) to keep the file clean. Keep `ContinueWatchingItem` type import only if still referenced by `progressPct`.

- [ ] **Step 3: Trim dead CSS**

The `.cw-card`, `.cw-img`, `.cw-play`, `.cw-info`, `.cw-ep`, `.cw-title-line`, `.cw-progress`, `.cw-progress-fill` rules are now owned by `MediaTile`. Remove those scoped style blocks (lines ~177-291). Keep `.cw-section-head`, `.cw-title`, `.cw-count`, `.cw-see-all`, `.cw-row` (+ scrollbar rules), and `.cw-card-skeleton` (+ `@keyframes pulse`). Add a minimal grid-cell rule so MediaTile fills the scroller column:

```css
.cw-card-tile { scroll-snap-align: start; }
```

- [ ] **Step 4: Type-check + suite**

Run: `bunx vue-tsc --noEmit && bunx vitest run src/components/home`
Expected: green.

- [ ] **Step 5: In-browser smoke (REQUIRED)**

`cd /data/animeenigma && make redeploy-web`, load `/` as a logged-in user with in-progress anime, and verify the Continue Watching rail: 16/9 tiles, drift shimmer before load, kicker + title overlay, hover play, progress bar widths correct, episode-deep links work. Mobile width too.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/home/ContinueWatchingRow.vue
git commit -m "feat(home): render Continue Watching with unified MediaTile"
```

---

## Phase 6 — Context menu C-top reorder + cleanup

### Task 13: Reorder AnimeContextMenu to C-top

"C-top" = **Open in new tab** pinned at the top of the actions list in its own separator group, above the status options.

**Files:**
- Modify: `src/components/anime/AnimeContextMenu.vue`
- Test: `src/components/anime/AnimeContextMenu.spec.ts`

- [ ] **Step 1: Read the existing spec to find the order assertion**

Read `src/components/anime/AnimeContextMenu.spec.ts`. If it already asserts action order, note the test name; otherwise add one in Step 2.

- [ ] **Step 2: Add/adjust the order test**

Add this test to `AnimeContextMenu.spec.ts` (inside the top-level `describe`):

```ts
it('pins Open in new tab as the first action (C-top)', async () => {
  // Mount with the existing helper/pattern in this file; ensure authenticated
  // so status options are present. The first rendered menu item must be newtab.
  const w = /* existing mount helper with anime + authenticated state */ mountMenu()
  const labels = w.findAll('[role="menuitem"], .data-\\[highlighted\\]\\:bg-white\\/5, button, [data-testid]')
  // Prefer asserting on the computed actions order via the first DropdownMenuItem text:
  const items = w.findAllComponents({ name: 'DropdownMenuItem' })
  expect(items[0].text()).toContain('contextMenu.openInNewTab')
})
```

> Use this file's existing mount helper and i18n stub (the stub returns the key string, so the label is `contextMenu.openInNewTab`). If the file lacks a helper, mirror the mount pattern already present in the spec. Adjust the selector to however items are queryable in this suite.

- [ ] **Step 3: Run the test, confirm it fails**

Run: `bunx vitest run src/components/anime/AnimeContextMenu.spec.ts`
Expected: FAIL — `newtab` is currently appended last.

- [ ] **Step 4: Reorder the actions computed**

In `src/components/anime/AnimeContextMenu.vue`, change the `actions` computed (lines 227-264) so `newtab` is pushed **first** and `goto` stays near it as the navigation group, with status/list actions after. Replace the computed body with:

```ts
const actions = computed<MenuAction[]>(() => {
  const out: MenuAction[] = []
  // C-top: open-in-new-tab pinned first, its own navigation group.
  out.push({ key: 'newtab', kind: 'newtab', label: t('contextMenu.openInNewTab'), onActivate: openInNewTab })
  out.push({ key: 'goto', kind: 'goto', label: t('contextMenu.goToPage'), onActivate: goToPage })
  if (authStore.isAuthenticated) {
    for (const s of statusOptions) {
      out.push({
        key: `status-${s.value}`,
        kind: 'status',
        label: t(s.i18nKey),
        current: props.listStatus === s.value,
        onActivate: () => setStatus(s.value),
      })
    }
    if (props.listStatus) {
      out.push({
        key: 'remove',
        kind: 'remove',
        label: t('profile.actions.removeFromList'),
        danger: true,
        onActivate: removeFromList,
      })
    }
    const ep = props.episodesWatched ?? 0
    const total = props.episodesTotal ?? 0
    if (props.listStatus === 'watching' && (total === 0 || ep < total)) {
      out.push({
        key: 'mark-next',
        kind: 'mark-next',
        label: t('contextMenu.markNextWatched', { n: ep + 1 }),
        onActivate: markNextWatched,
      })
    }
  }
  return out
})
```

- [ ] **Step 5: Add the separator after the navigation group**

To render the C-top "own separator group", add a top border to the first status item (or a divider after `goto`). Simplest, lint-safe approach: in `itemClasses`, give the first status item a top divider. Add a `group-start` flag to the first status action and a class. In the template's `DropdownMenuItem`, append `:class` handling. Minimal change — add to the status push: `firstStatus: index === 0` is overkill; instead, in `itemClasses`, detect `action.kind === 'status' && action.current` already styles current. For the divider, add in the template a wrapper: render a `<div class="border-t border-white/10 my-1" />` between the `goto` item and the rest. Implement by splitting the v-for: keep the single v-for but insert the divider via a computed boundary. The low-risk implementation: add `dividerBefore?: boolean` to the first status action and render a divider when present.

Add `dividerBefore` to the first status action:

```ts
    for (const [i, s] of statusOptions.entries()) {
      out.push({
        key: `status-${s.value}`,
        kind: 'status',
        label: t(s.i18nKey),
        current: props.listStatus === s.value,
        dividerBefore: i === 0,
        onActivate: () => setStatus(s.value),
      })
    }
```

Add `dividerBefore?: boolean` to the `MenuAction` interface. In the template, wrap the item with a leading divider:

```vue
        <template v-for="action in actions" :key="action.key">
          <div v-if="action.dividerBefore" class="border-t border-white/10 my-1" aria-hidden="true" />
          <DropdownMenuItem :class="itemClasses(action)" @select="activate(action)">
            <!-- existing icon switch + {{ action.label }} unchanged -->
          </DropdownMenuItem>
        </template>
```

> This wraps the existing `DropdownMenuItem` in a `<template>` loop and prepends a divider before the first status item, giving newtab+goto their own group at the top. Keep the icon `<svg>` switch exactly as-is inside the `DropdownMenuItem`.

- [ ] **Step 6: Run the test + full anime suite**

Run: `bunx vitest run src/components/anime/AnimeContextMenu.spec.ts src/components/anime`
Expected: PASS, including the new C-top order test and all pre-existing menu tests.

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/AnimeContextMenu.vue src/components/anime/AnimeContextMenu.spec.ts
git commit -m "feat(anime): reorder context menu to C-top (Open in new tab pinned, own group)"
```

---

### Task 14: Delete dead components

**Files:**
- Delete: `src/components/anime/AnimeCard.vue`
- Delete: `src/components/anime/AnimeCardSkeleton.vue`
- Modify: `src/components/anime/index.ts`

- [ ] **Step 1: Re-verify zero usages (do not trust the earlier scan blindly)**

Run:

```bash
cd /data/animeenigma/frontend/web
grep -rn --include=*.vue --include=*.ts -E "AnimeCardSkeleton|\bAnimeCard\b" src | grep -v "AnimeCardNew" | grep -v "src/components/anime/AnimeCard.vue" | grep -v "src/components/anime/AnimeCardSkeleton.vue" | grep -v "index.ts"
```

Expected: **no output** (the only references are the files themselves and the barrel). If anything else appears, STOP and migrate that usage to `PosterCard` first.

- [ ] **Step 2: Remove the barrel exports**

In `src/components/anime/index.ts`, delete these two lines:

```ts
export { default as AnimeCard } from './AnimeCard.vue'
export { default as AnimeCardSkeleton } from './AnimeCardSkeleton.vue'
```

- [ ] **Step 3: Delete the files**

```bash
cd /data/animeenigma/frontend/web
git rm src/components/anime/AnimeCard.vue src/components/anime/AnimeCardSkeleton.vue
```

- [ ] **Step 4: Type-check + full unit run**

Run: `bunx vue-tsc --noEmit && bunx vitest run`
Expected: no missing-module errors; full suite green.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/frontend/web
git add src/components/anime/index.ts
git commit -m "chore(anime): remove dead AnimeCard + AnimeCardSkeleton (superseded by PosterCard)"
```

---

### Task 15: Final gate — lint, type-check, e2e, after-update

- [ ] **Step 1: Design-system lint (build gate)**

Run: `cd /data/animeenigma && bash frontend/web/scripts/design-system-lint.sh`
Expected: `ERRORS=0`. Fix any flagged hex/off-palette class in the new `.vue` files (prefer tokens; only allowlist with a justified reason as a last resort).

- [ ] **Step 2: ESLint + type-check**

Run: `cd /data/animeenigma/frontend/web && bunx eslint src/ && bunx vue-tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Full unit suite**

Run: `bunx vitest run`
Expected: green.

- [ ] **Step 4: Decide on optional decommission of ColumnItem**

`ColumnItem.vue` is now unused by Home. Re-run the zero-usage grep:

```bash
grep -rn --include=*.vue --include=*.ts "ColumnItem" /data/animeenigma/frontend/web/src | grep -v "src/components/home/ColumnItem.vue"
```

If empty, optionally `git rm src/components/home/ColumnItem.vue` (+ its spec if any) and commit `chore(home): remove ColumnItem (superseded by PosterRow)`. If any usage remains, leave it.

- [ ] **Step 5: Targeted e2e**

Run the existing relevant Playwright specs to confirm no regression in home/spotlight/watchlist flows:

Run: `cd /data/animeenigma/frontend/web && bunx playwright test spotlight watchlist accessibility --reporter=list`
Expected: pass (or no NEW failures vs. the pre-change baseline; capture the baseline first if unsure).

- [ ] **Step 6: After-update skill**

Invoke `/animeenigma-after-update` to lint/build/redeploy the web service, run health checks, add a Russian-Trump-mode `changelog.json` entry for the unified card system, commit with the standard co-authors, and push.

---

## Self-review notes (author checklist applied)

- **Spec coverage:** 3 components (PosterCard T6, PosterRow T9, MediaTile T11) ✓; `toCardModel` + `AnimeCardModel` (T4) ✓; `PosterImage` (T5) ✓; Badge overlay (T2) ✓; drift skeleton (T1) ✓; AnimeKebab geometry (T3, enables centered cluster + glass kebab) ✓; C-top reorder (T13) ✓; rollout Browse/Anime → PosterCard (T7–T8), Home → PosterRow (T10), Continue-watching → MediaTile (T12) ✓; delete dead components (T14) ✓; token-units honored (px radius / rem spacing / tokens for color, all via main.css + scoped styles) ✓.
- **No new i18n keys:** every `$t(...)`/`t(...)` call reuses an existing key (`card.dubBadge`, `card.episodeProgress`, `home.airing`, `home.announced`, `home.episodeCount`, `home.nextEpisodeLine`, `seasons.*`, `home.todayAt/tomorrowAt/dayAt`, `schedule.daysShort.*`, `home.continueWatchingEpisode`, `profile.watchlist.*`, `contextMenu.*`, `anime.episode`). No locale-parity test impact.
- **Type consistency:** `AnimeCardModel` field names are used identically across mappers and all three cards (`malScore`, `siteScore`, `nextEpisode.ep/when`, `progress.current/total`, `listStatus`, `airing`). `CardExtras.siteScore` is a number; parents pass `average_score` only when `total_reviews > 0`.
- **Known soft spots flagged inline (verify during execution):** (a) `$t` availability in unit tests vs. script-level `t` — guard in T6; (b) `browseProgress` exact type — confirm in T7; (c) Home `openHomeMenuAt` HomeAnime→menu adapter is unchanged and must keep working — confirm in T10; (d) ongoing deep-link parity in PosterRow — recommended addition noted in T9/T10.

---

## Deferred (not in this plan)

- Hero Spotlight 9-card redesign (separate session, per the frozen spec).
- Profile watchlist grid / Collections grid / Schedule / ActivityFeed migrations to `PosterCard`/`PosterRow` — these are additional rollout surfaces beyond the spec's 4 locked steps; tackle in a follow-up once the 3 components are proven in production.
