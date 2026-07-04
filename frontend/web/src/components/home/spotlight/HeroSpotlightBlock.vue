<template>
  <!--
    Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-04.

    Outer wrapper + carousel state machine for the v1.0 HeroSpotlightBlock.

    Three top-level branches:
      1. Skeleton — `enabled && loading` (still allowed even before fetch
         resolves; matches the loaded block's min-heights to avoid CLS).
      2. Loaded — `enabled && cards.length > 0 && active`.
      3. Hidden — fetch error, empty cards, or feature-flag-off.

    State machine: see UI-SPEC §Interaction Contract for the canonical state
    table. RESEARCH.md Pitfalls 3, 4, and 7 drive the use of useIntervalFn
    (auto-cleanup on unmount + HMR), the cards.length watch (random init
    after fetch resolves), and the requestAnimationFrame deferred focusout
    handler (prevents flicker when Tab moves between dot buttons).
  -->

  <!-- Skeleton state -->
  <div
    v-if="enabled && loading"
    class="px-4 lg:px-8 max-w-7xl mx-auto mb-8 mt-8"
    aria-hidden="true"
  >
    <div
      class="glass-card rounded-2xl spotlight-frame overflow-hidden"
    >
      <div class="relative w-full h-full skeleton-shimmer" />
    </div>
    <!-- Reserved menu row (v4 A-1): mirrors CarouselDots' mt-3 + h-8
         geometry so the icon menu appearing after load causes ZERO
         layout shift (the old dots row pushed the page ~28px down). -->
    <!-- Mirrors the ARR-1 row: edge-pinned chevrons + 5 centered anchors
         (3rd is the active pill) — so load causes ZERO shift. -->
    <div class="relative mt-3 h-8 flex items-center justify-center gap-2" data-testid="menu-skeleton">
      <span class="skeleton-shimmer rounded-full w-8 h-8 absolute left-0 top-0" />
      <span
        v-for="n in 5"
        :key="n"
        class="relative skeleton-shimmer rounded-full"
        :class="n === 3 ? 'w-28 h-8' : 'w-8 h-8'"
      />
      <span class="skeleton-shimmer rounded-full w-8 h-8 absolute right-0 top-0" />
    </div>
  </div>

  <!-- Loaded state — at least one card and an active selection -->
  <section
    v-else-if="enabled && cards.length > 0 && active"
    ref="rootRef"
    role="region"
    aria-roledescription="carousel"
    :aria-label="t('spotlight.regionLabel')"
    class="px-4 lg:px-8 max-w-7xl mx-auto mb-8 mt-8"
    tabindex="0"
    @mouseenter="stopCycle"
    @mouseleave="startCycle"
    @focusin="stopCycle"
    @focusout="onFocusOut"
    @keydown.left="prev"
    @keydown.right="next"
  >
    <div
      class="relative glass-card rounded-2xl overflow-hidden flex flex-col spotlight-frame"
      @touchstart.passive="onTouchStart"
      @touchend.passive="onTouchEnd"
    >
      <div
        class="relative w-full flex-1 min-h-0"
        role="group"
        aria-roledescription="slide"
        :aria-label="
          t('spotlight.slideLabelWithTitle', {
            n: currentIndex + 1,
            total: cards.length,
            title: cardTitle(active),
          })
        "
        aria-live="polite"
        aria-atomic="true"
      >
        <transition
          :name="reducedMotion ? 'none' : 'spotlight-fade'"
          mode="out-in"
          @before-leave="onBeforeLeave"
          @after-leave="onAfterLeave"
        >
          <!-- Per-type branches keep card prop types strictly checked under
               vue-tsc — a bare <component :is=cardFor(...)> widens the data
               prop to the union and breaks the build. -->
          <FeaturedCard
            v-if="active.type === 'featured'"
            :key="`featured:${currentIndex}`"
            :data="active.data"
          />
          <RandomTailCard
            v-else-if="active.type === 'random_tail'"
            :key="`random_tail:${currentIndex}`"
            :data="active.data"
          />
          <LatestNewsCard
            v-else-if="active.type === 'latest_news'"
            :key="`latest_news:${currentIndex}`"
            :data="active.data"
          />
          <PlatformStatsCard
            v-else-if="active.type === 'platform_stats'"
            :key="`platform_stats:${currentIndex}`"
            :data="active.data"
          />
          <PersonalPickCard
            v-else-if="active.type === 'personal_pick'"
            :key="`personal_pick:${currentIndex}`"
            :data="active.data"
          />
          <TelegramNewsCard
            v-else-if="active.type === 'telegram_news'"
            :key="`telegram_news:${currentIndex}`"
            :data="active.data"
          />
          <NowWatchingCard
            v-else-if="active.type === 'now_watching'"
            :key="`now_watching:${currentIndex}`"
            :data="active.data"
          />
          <NotTimeYetCard
            v-else-if="active.type === 'not_time_yet'"
            :key="`not_time_yet:${currentIndex}`"
            :data="active.data"
          />
          <ContinueWatchingNewCard
            v-else-if="active.type === 'continue_watching_new'"
            :key="`continue_watching_new:${currentIndex}`"
            :data="active.data"
          />
        </transition>
      </div>
      <!-- SR-only pause announcement (UI-SPEC §A11y; F1.3/F6.1 resolution).
           aria-live=polite so it speaks at the screen reader's next idle. -->
      <span class="sr-only" aria-live="polite">
        {{ paused ? t('spotlight.pauseAutoplay') : '' }}
      </span>
    </div>

    <!-- ARR-1 (2026-06-11 lock): prev/next chevrons live in the menu row
         below the frame — the old in-frame edge overlays (CarouselControls)
         collided with cards whose content runs to the edges (terminal,
         deck, rec column). One always-visible control cluster instead. -->
    <CarouselDots
      :current-index="currentIndex"
      :cards="cards"
      @goto="goTo"
      @prev="prev"
      @next="next"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIntervalFn, useMediaQuery } from '@vueuse/core'
import { useSpotlight } from '@/composables/useSpotlight'
import type { SpotlightCard } from '@/types/spotlight'
import CarouselDots from './CarouselDots.vue'
import FeaturedCard from './cards/FeaturedCard.vue'
import RandomTailCard from './cards/RandomTailCard.vue'
import LatestNewsCard from './cards/LatestNewsCard.vue'
import PlatformStatsCard from './cards/PlatformStatsCard.vue'
import PersonalPickCard from './cards/PersonalPickCard.vue'
import TelegramNewsCard from './cards/TelegramNewsCard.vue'
import NowWatchingCard from './cards/NowWatchingCard.vue'
import NotTimeYetCard from './cards/NotTimeYetCard.vue'
import ContinueWatchingNewCard from './cards/ContinueWatchingNewCard.vue'
import { getLocalizedTitle } from '@/utils/title'
import { preloadImage } from '@/utils/preload-image'
import { cardPosterUrl } from '@/composables/useImageProxy'

// Locked at 7000 ms per HSB-FE-03. Do not parametrize — the cadence is part
// of the product spec, not a knob.
const AUTO_CYCLE_INTERVAL_MS = 7000

// Feature-flag semantics per RESEARCH.md Pattern 4 (precedent: App.vue:92
// for VITE_NOTIFICATIONS_ENABLED). Default is ON; only the literal string
// 'false' disables the block.
const enabled =
  (import.meta.env.VITE_HERO_SPOTLIGHT_ENABLED as string | undefined) !== 'false'

const { t } = useI18n()
const { cards, loading } = useSpotlight()
const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')

const rootRef = ref<HTMLElement | null>(null)
const currentIndex = ref(0)

// Pitfall 4 mitigation guard — only randomize once per mount cycle. Without
// this guard, a Phase 3 refresh() would re-randomize the slide under the
// user's feet.
let initialized = false

// Once the user clicks prev/next/a dot, they've taken manual control of the
// carousel; stop the 7s auto-cycle permanently for this mount. Previously
// next()/prev()/goTo() called restart() which immediately re-armed the timer,
// so 7s after a manual click the carousel jumped again — read as "switching
// by itself" right after the user clicked. Hover-based pause/resume still
// works freely up until the first click.
const userInteracted = ref(false)

// v1.1-polish Phase 01 Task 4 (HSB-V11-CC-05) — transition lock.
//
// The Phase 03 UAT surfaced a "blank-card" bug: hammering ArrowRight 10×
// rapidly outraced the 400ms cross-fade and left the carousel stuck in
// the leave-to opacity:0 state — no card visible. The lock ignores nav
// input while the OUTGOING card is fading out; once @after-leave fires
// the new card is already in the DOM (mid-enter), so subsequent nav can
// safely interrupt the enter without producing a blank frame.
//
// We deliberately use @after-leave (NOT @after-enter): the leave phase
// is when the blank-card bug originates, so that's the only window we
// must protect. Holding the lock until @after-enter would double the
// no-input window (leave 400ms + enter 400ms = 800ms) and break legit
// rapid-click flows (the user clicked through a card they recognized).
//
// The watchdog backstop force-clears the flag after 600ms in case the
// leave hook never fires (e.g. transition cancelled by a route change).
const TRANSITION_LOCK_WATCHDOG_MS = 600
const isTransitioning = ref(false)
let watchdogTimer: ReturnType<typeof setTimeout> | null = null

function onBeforeLeave(): void {
  isTransitioning.value = true
  if (watchdogTimer !== null) clearTimeout(watchdogTimer)
  watchdogTimer = setTimeout(() => {
    isTransitioning.value = false
    watchdogTimer = null
  }, TRANSITION_LOCK_WATCHDOG_MS)
}

function onAfterLeave(): void {
  isTransitioning.value = false
  if (watchdogTimer !== null) {
    clearTimeout(watchdogTimer)
    watchdogTimer = null
  }
}

onBeforeUnmount(() => {
  if (watchdogTimer !== null) {
    clearTimeout(watchdogTimer)
    watchdogTimer = null
  }
})

function advance(): void {
  if (cards.value.length === 0) return
  currentIndex.value = (currentIndex.value + 1) % cards.value.length
}

// RESEARCH.md Pattern 3 — useIntervalFn auto-cleans on unmount (also on HMR
// via onScopeDispose), eliminating the leak class hand-rolled setInterval
// requires us to manage. immediate:false because we wait for cards to load
// AND for the initial-index watch to fire before the first tick.
const { pause, resume, isActive } = useIntervalFn(advance, AUTO_CYCLE_INTERVAL_MS, {
  immediate: false,
})

// SR-only announcement when auto-cycle is paused. F1.3/F6.1 — the
// spotlight.pauseAutoplay i18n key shipped in EN/RU/JA but had no consumer.
// Computed false when: cycle still active, single-card mode, or reduced-motion.
const paused = computed<boolean>(
  () => !isActive.value && cards.value.length > 1 && !reducedMotion.value,
)

function startCycle(): void {
  // Reduced-motion users opt out of auto-advance entirely (HSB-FE-06).
  if (reducedMotion.value) return
  // Single-card responses are a no-op — cycling produces a useless flicker
  // every 7s. Per UI-SPEC §State Contract.
  if (cards.value.length <= 1) return
  // Once the user has manually navigated (clicked an arrow / dot), the
  // carousel is theirs — never resume the auto-cycle for the rest of this
  // mount, regardless of hover/focus state.
  if (userInteracted.value) return
  resume()
}

function stopCycle(): void {
  pause()
}

function next(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value = (currentIndex.value + 1) % cards.value.length
  userInteracted.value = true
  pause()
}

function prev(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value =
    (currentIndex.value - 1 + cards.value.length) % cards.value.length
  userInteracted.value = true
  pause()
}

function goTo(i: number): void {
  if (isTransitioning.value) return
  if (i < 0 || i >= cards.value.length) return
  currentIndex.value = i
  userInteracted.value = true
  pause()
}

// ── Touch swipe (ARR-1 companion, 2026-06-11 lock) ─────────────────────
// Horizontal swipe on the frame navigates slides. Listeners are passive
// (we never preventDefault — vertical page scroll stays native); a swipe
// counts only when horizontal travel ≥48px AND clearly dominates the
// vertical axis, so scroll gestures don't accidentally flip slides.
const SWIPE_MIN_PX = 48
let touchX = 0
let touchY = 0
let touchTracking = false

function onTouchStart(e: TouchEvent): void {
  const t = e.touches[0]
  if (!t) return
  touchX = t.clientX
  touchY = t.clientY
  touchTracking = true
}

function onTouchEnd(e: TouchEvent): void {
  if (!touchTracking) return
  touchTracking = false
  const t = e.changedTouches[0]
  if (!t) return
  const dx = t.clientX - touchX
  const dy = t.clientY - touchY
  if (Math.abs(dx) < SWIPE_MIN_PX || Math.abs(dx) < Math.abs(dy) * 1.2) return
  if (dx < 0) next()
  else prev()
}

// ── Slide image prefetch (2026-06-11 lock: «ленивая автозагрузка всего
// спотлайта под капотом») ───────────────────────────────────────────────
// After the cards arrive, warm every slide's images in the background —
// at the SAME proxy buckets the card components request, so flipping to
// any slide is a pure cache hit (no per-slide skeletons for the user who
// waits a few seconds). Runs once per mount, at browser idle, two lanes
// deep, starting from the slide AFTER the current one.
function cardImageUrls(card: SpotlightCard): string[] {
  switch (card.type) {
    case 'featured':
      return card.data.anime.poster_url
        ? [cardPosterUrl(card.data.anime.poster_url, 640)]
        : []
    case 'random_tail': {
      const p = card.data.anime.poster_url
      return p ? [cardPosterUrl(p, 256), cardPosterUrl(p, 128)] : []
    }
    case 'not_time_yet': {
      const p = card.data.anime.poster_url
      return p ? [cardPosterUrl(p, 256)] : []
    }
    case 'continue_watching_new': {
      const p = card.data.anime.poster_url
      return p ? [cardPosterUrl(p, 256), cardPosterUrl(p, 128)] : []
    }
    case 'personal_pick': {
      const urls: string[] = []
      for (const [idx, item] of (card.data.items ?? []).entries()) {
        const p = item.anime.poster_url
        if (!p) continue
        // Featured (first) item renders at 256 + feeds the 128 blur
        // backdrop; the list/swipe rows are all 128.
        if (idx === 0) urls.push(cardPosterUrl(p, 256))
        urls.push(cardPosterUrl(p, 128))
      }
      return urls
    }
    case 'now_watching':
      return (card.data.sessions ?? [])
        .filter((s) => s.poster_url)
        .map((s) => cardPosterUrl(s.poster_url, 128))
    case 'telegram_news': {
      const hero = (card.data.posts ?? [])[0]
      return hero?.image_url ? [hero.image_url] : []
    }
    default:
      return []
  }
}

let prefetched = false
function prefetchSlides(): void {
  if (prefetched || cards.value.length === 0) return
  prefetched = true
  const n = cards.value.length
  const start = currentIndex.value
  const ordered: string[] = []
  for (let k = 1; k <= n; k++) {
    const card = cards.value[(start + k) % n]
    if (card) ordered.push(...cardImageUrls(card))
  }
  const queue = [...new Set(ordered)]
  const lane = (): void => {
    const next = queue.shift()
    if (!next) return
    void preloadImage(next).then(lane)
  }
  lane()
  lane()
}

function schedulePrefetch(): void {
  if ('requestIdleCallback' in window) {
    window.requestIdleCallback(() => prefetchSlides(), { timeout: 3000 })
  } else {
    setTimeout(prefetchSlides, 800)
  }
}

// Pitfall 4 mitigation — randomize AFTER cards populate. The fetch is async,
// so synchronously seeding currentIndex inside onMounted always picks 0
// (length is 0 at that point). Watching the length transition from 0 → N
// guarantees we seed exactly once, after the response has populated.
watch(() => cards.value.length, (n) => {
  if (n > 0 && !initialized) {
    currentIndex.value = Math.floor(Math.random() * n)
    initialized = true
    startCycle()
    schedulePrefetch()
  }
}, { immediate: false })

// Pitfall 7 mitigation — focusout fires BEFORE document.activeElement
// settles to the new target. Without the rAF defer, Tab-ing between dot
// buttons inside the wrapper would pause/resume on each transition,
// producing a visible flicker. Per UI-SPEC §Interaction Contract — note
// the explicit "requestAnimationFrame (NOT setTimeout)" guidance.
function onFocusOut(): void {
  requestAnimationFrame(() => {
    if (!rootRef.value?.contains(document.activeElement)) {
      startCycle()
    }
  })
}

// Runtime reduced-motion toggle — if the OS-level setting flips while the
// component is mounted, honor it immediately (don't wait for the next
// timer tick to take effect).
watch(reducedMotion, (v) => {
  if (v) {
    pause()
  } else if (cards.value.length > 1) {
    resume()
  }
})

const active = computed<SpotlightCard | null>(() => {
  if (cards.value.length === 0) return null
  return cards.value[currentIndex.value] ?? null
})

// Resolves a human-readable title for the slide aria-label. Anime cards use
// the localized helper (Pitfall 8 — fields are snake_case end-to-end).
// Multi-item cards fall back to their card-level title key.
function cardTitle(card: SpotlightCard): string {
  switch (card.type) {
    case 'featured':
    case 'random_tail':
      return getLocalizedTitle(
        card.data.anime.name,
        card.data.anime.name_ru,
        card.data.anime.name_jp,
      )
    case 'latest_news':
      return t('spotlight.latestNews.title')
    case 'platform_stats':
      return t('spotlight.platformStats.title')
    case 'personal_pick':
      return card.data.source === 'trending'
        ? t('spotlight.personalPick.titleAnon')
        : t('spotlight.personalPick.title')
    case 'telegram_news':
      return t('spotlight.telegramNews.title')
    case 'now_watching':
      return t('spotlight.nowWatching.title')
    case 'not_time_yet':
      return getLocalizedTitle(
        card.data.anime.name,
        card.data.anime.name_ru,
        card.data.anime.name_jp,
      )
    case 'continue_watching_new':
      return getLocalizedTitle(
        card.data.anime.name,
        card.data.anime.name_ru,
        card.data.anime.name_jp,
      )
  }
}
</script>
