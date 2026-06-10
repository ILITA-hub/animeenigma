<template>
  <!--
    Workstream hero-spotlight — DS alignment 2026-06-10, batch 1 (spec:
    2026-06-10-spotlight-ds-alignment-design.md). Discovery card rebuilt on
    SpotlightCardShell: single shell kicker (violet, shuffle icon) replaces
    the old mobile/desktop dual kicker, score + genres render as overlay
    Badges (the backdrop is poster imagery), the CTA is a Button-variant
    router-link pinned to the bottom-left corner. The mount-time
    shuffle-deck animation and the rotating tagline are kept as-is.
  -->
  <SpotlightCardShell
    accent="violet"
    icon="shuffle"
    :kicker="t('spotlight.randomTail.title')"
    backdrop="poster-blur"
    :poster-url="data.anime.poster_url"
  >
    <!-- Violet-tinted secondary overlay differentiates RandomTail from
         the cyan FeaturedCard backdrop without re-fetching the poster. -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-brand-violet/30 via-transparent to-transparent"
      />
    </template>

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

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-6 md:items-center">
      <router-link
        :to="`/anime/${data.anime.id}`"
        class="flex-shrink-0 self-center w-24 md:w-32 lg:w-40 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-brand-violet/20 transition-shadow duration-300 group-hover:shadow-brand-violet/40"
        >
          <img
            :src="cardPosterUrl(data.anime.poster_url, 256)"
            :alt="getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp)"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
            decoding="async"
          />
        </div>
      </router-link>

      <div class="flex-1 min-w-0">
        <p
          class="hidden md:block text-sm text-muted-foreground mb-2 font-medium"
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
          <Badge v-if="data.anime.score" variant="warning" size="sm" overlay>
            <template #icon>
              <Star class="size-3" fill="currentColor" aria-hidden="true" />
            </template>
            {{ data.anime.score?.toFixed(1) }}
          </Badge>
          <p
            v-if="data.anime.episodes_count"
            class="text-sm text-muted-foreground font-medium"
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
          class="mt-3 flex flex-wrap gap-1.5"
        >
          <Badge
            v-for="g in data.anime.genres.slice(0, 3)"
            :key="g.id"
            size="sm"
            overlay
          >
            {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
          </Badge>
        </div>
      </div>
    </div>

    <template #cta>
      <router-link
        :to="`/anime/${data.anime.id}`"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.randomTail.discoverCta') }}
        <Shuffle class="w-4 h-4" aria-hidden="true" />
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { Shuffle, Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import type { RandomTailData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
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
