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
      class="glass-card rounded-2xl min-h-[420px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[400px] overflow-hidden"
    >
      <div class="w-full h-full min-h-[420px] md:min-h-[340px] lg:min-h-[320px] skeleton-shimmer" />
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
      class="relative glass-card rounded-2xl overflow-hidden flex flex-col min-h-[420px] md:min-h-[340px] lg:min-h-[320px] lg:max-h-[400px]"
    >
      <div
        class="relative w-full flex-1 min-h-0 pb-10"
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
          @after-enter="onAfterEnter"
        >
          <!-- Per-type branches keep card prop types strictly checked under
               vue-tsc — a bare <component :is=cardFor(...)> widens the data
               prop to the union and breaks the build. -->
          <AnimeOfDayCard
            v-if="active.type === 'anime_of_day'"
            :key="`anime_of_day:${currentIndex}`"
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
      <CarouselControls
        :current-index="currentIndex"
        :card-count="cards.length"
        @prev="prev"
        @next="next"
        @goto="goTo"
      />
      <!-- SR-only pause announcement (UI-SPEC §A11y; F1.3/F6.1 resolution).
           aria-live=polite so it speaks at the screen reader's next idle. -->
      <span class="sr-only" aria-live="polite">
        {{ paused ? t('spotlight.pauseAutoplay') : '' }}
      </span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIntervalFn, useMediaQuery } from '@vueuse/core'
import { useSpotlight } from '@/composables/useSpotlight'
import type { SpotlightCard } from '@/types/spotlight'
import CarouselControls from './CarouselControls.vue'
import AnimeOfDayCard from './cards/AnimeOfDayCard.vue'
import RandomTailCard from './cards/RandomTailCard.vue'
import LatestNewsCard from './cards/LatestNewsCard.vue'
import PlatformStatsCard from './cards/PlatformStatsCard.vue'
import PersonalPickCard from './cards/PersonalPickCard.vue'
import TelegramNewsCard from './cards/TelegramNewsCard.vue'
import NowWatchingCard from './cards/NowWatchingCard.vue'
import NotTimeYetCard from './cards/NotTimeYetCard.vue'
import ContinueWatchingNewCard from './cards/ContinueWatchingNewCard.vue'
import { getLocalizedTitle } from '@/utils/title'

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

// v1.1-polish Phase 01 Task 4 (HSB-V11-CC-05) — transition lock.
//
// The Phase 03 UAT surfaced a "blank-card" bug: hammering ArrowRight 10×
// rapidly outraced the 400ms cross-fade and left the carousel stuck in
// the leave-to opacity:0 state — no card visible. The lock ignores nav
// input while the fade is in flight; the watchdog backstop force-clears
// the flag after 600ms in case @after-enter never fires (e.g. transition
// gets cancelled by a route change).
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

function onAfterEnter(): void {
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
  resume()
}

function stopCycle(): void {
  pause()
}

function restart(): void {
  pause()
  startCycle()
}

function next(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value = (currentIndex.value + 1) % cards.value.length
  restart()
}

function prev(): void {
  if (cards.value.length === 0 || isTransitioning.value) return
  currentIndex.value =
    (currentIndex.value - 1 + cards.value.length) % cards.value.length
  restart()
}

function goTo(i: number): void {
  if (isTransitioning.value) return
  if (i < 0 || i >= cards.value.length) return
  currentIndex.value = i
  restart()
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
    case 'anime_of_day':
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
