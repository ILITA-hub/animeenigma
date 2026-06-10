<template>
  <!--
    Workstream hero-spotlight — v1.1-polish Phase 03 (HSB-V11-RT-01..04).

    Discovery-themed refactor of the RandomTailCard:
      - <article> hosts a SpotlightBackdrop (variant="poster-blur") layer
        plus a purple secondary overlay so the card reads as "discovery"
        rather than the cyan "anime of day" sibling.
      - Promoted kicker now leads with a <SpotlightIcon name="shuffle" />
        in brand-violet, with a brand-violet label.
      - Desktop subtitle is one of 4 rotating taglines (i18n
        spotlight.randomTail.taglines[]), picked at mount via Math.random.
        Falls back to spotlight.randomTail.subtitle when the array is
        absent / empty so we never render a raw key.
      - Mount-time shuffle-deck animation (5 staggered gradient cards),
        gated on `prefers-reduced-motion: reduce`. Self-clears 1000ms
        post-mount.
      - Primary CTA is a purple .cta-hero with the shuffle icon.
  -->
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop
      variant="poster-blur"
      :poster-url="data.anime.poster_url"
    />
    <!-- Purple-tinted secondary overlay differentiates RandomTail from
         the cyan FeaturedCard backdrop without re-fetching the poster. -->
    <div
      aria-hidden="true"
      class="absolute inset-0 bg-gradient-to-r from-brand-violet/30 via-transparent to-transparent"
    />

    <!-- Mount-time shuffle-deck animation. Skipped entirely when the user
         has opted into reduced motion, both via the v-if (no DOM cost) and
         via the setTimeout self-clear at +1000ms. -->
    <div
      v-if="!reducedMotion && showShuffle"
      class="absolute inset-0 z-20 flex items-center justify-center pointer-events-none"
      aria-hidden="true"
      data-testid="shuffle-deck"
    >
      <div class="shuffle-deck">
        <div
          v-for="n in 5"
          :key="n"
          class="shuffle-card"
          :style="`--delay: ${n * 60}ms`"
        />
      </div>
    </div>

    <div
      class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6 md:items-center"
    >
      <header class="md:hidden">
        <div class="flex items-center gap-2 mb-1">
          <SpotlightIcon name="shuffle" class="w-4 h-4 text-brand-violet" />
          <p
            class="text-brand-violet text-[10px] uppercase tracking-[0.18em] font-semibold"
          >
            {{ t('spotlight.randomTail.title') }}
          </p>
        </div>
      </header>

      <router-link
        :to="`/anime/${data.anime.id}`"
        class="flex-shrink-0 self-center md:self-center w-32 md:w-44 lg:w-56 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-brand-violet/20 transition-shadow duration-300 group-hover:shadow-brand-violet/40"
        >
          <img
            :src="cardPosterUrl(data.anime.poster_url, 256)"
            :alt="getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp)"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
        </div>
      </router-link>

      <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
        <div>
          <div class="hidden md:flex items-center gap-2 mb-2">
            <SpotlightIcon name="shuffle" class="w-4 h-4 text-brand-violet" />
            <p
              class="text-brand-violet text-[10px] uppercase tracking-[0.18em] font-semibold"
            >
              {{ t('spotlight.randomTail.title') }}
            </p>
          </div>
          <p
            class="hidden md:block text-sm rt-muted mb-2 font-medium"
            data-testid="random-tail-tagline"
          >
            {{ tagline }}
          </p>
          <h3
            class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2"
          >
            {{
              getLocalizedTitle(
                data.anime.name,
                data.anime.name_ru,
                data.anime.name_jp,
              )
            }}
          </h3>

          <div class="mt-2 flex flex-wrap items-center gap-2">
            <span
              v-if="data.anime.score"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold bg-warning/20 text-warning"
            >
              <Star class="size-3" fill="currentColor" aria-hidden="true" />
              {{ data.anime.score?.toFixed(1) }}
            </span>
            <p
              v-if="data.anime.episodes_count"
              class="text-sm rt-muted font-medium"
            >
              {{
                t('spotlight.featured.episodesLabel', {
                  n: data.anime.episodes_count,
                })
              }}
            </p>
          </div>

          <div
            v-if="data.anime.genres?.length"
            class="mt-3 flex flex-wrap gap-1"
          >
            <span
              v-for="g in data.anime.genres.slice(0, 3)"
              :key="g.id"
              class="px-2 py-0.5 text-xs font-medium rt-chip"
            >
              {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
            </span>
          </div>
        </div>

        <div class="flex flex-wrap gap-2 mt-3">
          <router-link
            :to="`/anime/${data.anime.id}`"
            class="cta-hero"
            data-accent="purple"
          >
            {{ t('spotlight.randomTail.discoverCta') }}
            <SpotlightIcon name="shuffle" class="w-4 h-4" />
          </router-link>
        </div>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import type { RandomTailData } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import { cardPosterUrl } from '@/composables/useImageProxy'

defineProps<{ data: RandomTailData }>()

const { t, tm, locale: i18nLocale } = useI18n()

// Normalize locale to a plain string for the genre-name selector (same
// pattern as FeaturedCard — useI18n's locale is Ref<string|Composer>).
const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// Tagline: one of 4 rotating candidates per locale, picked at mount.
// Falls back to the scalar `spotlight.randomTail.subtitle` key when the
// array is missing or empty so we never render a raw i18n key string.
const tagline = ref('')

// Reduced-motion guard for the shuffle-deck mount animation. Same util
// already used by Hero.vue + Carousel.vue + HeroSpotlightBlock.vue.
const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
const showShuffle = ref(true)

// Track the cleanup timer so an unmount before the 1000ms tick doesn't
// flip showShuffle on a destroyed instance.
let shuffleTimer: ReturnType<typeof setTimeout> | null = null

onMounted(() => {
  // tm() returns the raw message structure — for an array key it returns
  // a JS array. Defensive: if any locale ships without the array, fall
  // back to the scalar subtitle so the card still renders a sensible
  // line instead of a raw key string.
  const candidates = tm('spotlight.randomTail.taglines') as unknown
  if (Array.isArray(candidates) && candidates.length > 0) {
    const arr = candidates as string[]
    tagline.value = arr[Math.floor(Math.random() * arr.length)]
  } else {
    tagline.value = t('spotlight.randomTail.subtitle')
  }

  if (reducedMotion.value) {
    // Honor reduced-motion immediately — no deck, no timer.
    showShuffle.value = false
  } else {
    shuffleTimer = setTimeout(() => {
      showShuffle.value = false
      shuffleTimer = null
    }, 1000)
  }
})

onBeforeUnmount(() => {
  if (shuffleTimer !== null) {
    clearTimeout(shuffleTimer)
    shuffleTimer = null
  }
})
</script>

<style scoped>
/* Neon Tokyo token replacements (feat/homepage-neon-tokyo-redesign).
   Swap hardcoded gray values for semantic design tokens. */
.rt-muted { color: var(--muted-foreground); }
.rt-chip {
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid var(--line);
  color: var(--ink-2);
  border-radius: var(--r-sm);
}
</style>
