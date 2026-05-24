<template>
  <!--
    Workstream hero-spotlight — v1.1-polish Phase 02 (HSB-V11-AOD-01..04).

    Cinematic refactor of the AnimeOfDayCard:
      - Bare <article> is now wrapped by a `relative` container that hosts a
        SpotlightBackdrop layer (variant="poster-blur"), with the existing
        content layered above under `relative z-10`.
      - Foreground poster widens to w-32 md:w-44 lg:w-56 and gains a subtle
        cyan glow on hover so the art reads as the hero element.
      - The dead disabled "Add to list" button is removed; a single cyan
        .cta-hero "Watch" link sits in its place.
      - Score badge moves from a poster-corner overlay (which obstructed art)
        to a meta-row pill alongside the episodes count.
      - Genre tags pull per-genre Tailwind classes from
        cardTokens.anime_of_day.genreColors, falling back to the neutral
        bg-white/10 / text-gray-300 pair when the ID isn't mapped.
      - The kicker (both mobile + desktop copies) is promoted with
        text-cyan-300, tighter tracking, and 10px sizing.
  -->
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop
      variant="poster-blur"
      :poster-url="data.anime.poster_url"
    />
    <div
      class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6 md:items-center"
    >
      <header class="md:hidden">
        <p
          class="text-cyan-300 text-[10px] uppercase tracking-[0.18em] font-semibold mb-1"
        >
          {{ t('spotlight.animeOfDay.title') }}
        </p>
      </header>

      <router-link
        :to="`/anime/${data.anime.id}`"
        class="flex-shrink-0 self-center md:self-center w-32 md:w-44 lg:w-56 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-cyan-500/20 transition-shadow duration-300 group-hover:shadow-cyan-500/40"
        >
          <img
            :src="data.anime.poster_url || '/placeholder.svg'"
            :alt="getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp)"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
        </div>
      </router-link>

      <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
        <div>
          <p
            class="hidden md:block text-cyan-300 text-[10px] uppercase tracking-[0.18em] font-semibold mb-2"
          >
            {{ t('spotlight.animeOfDay.title') }}
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
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold bg-yellow-500/20 text-yellow-200"
            >
              <svg
                class="w-3 h-3"
                fill="currentColor"
                viewBox="0 0 24 24"
                aria-hidden="true"
              >
                <path
                  d="M12 17.27L18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z"
                />
              </svg>
              {{ data.anime.score?.toFixed(1) }}
            </span>
            <p
              v-if="data.anime.episodes_count"
              class="text-sm text-gray-400 font-medium"
            >
              {{
                t('spotlight.animeOfDay.episodesLabel', {
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
              class="px-2 py-0.5 text-xs font-medium rounded"
              :class="genreColorClass(g.id)"
            >
              {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
            </span>
          </div>
        </div>

        <div class="flex flex-wrap gap-2 mt-3">
          <router-link
            :to="`/anime/${data.anime.id}/watch`"
            class="cta-hero"
          >
            {{ t('spotlight.animeOfDay.watchCta') }}
            <SpotlightIcon name="play" class="w-4 h-4" />
          </router-link>
        </div>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { AnimeOfDayData } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import { cardTokens } from '../tokens'

defineProps<{ data: AnimeOfDayData }>()

const { t, locale: i18nLocale } = useI18n()

// Normalize locale to a plain string for the genre-name selector. useI18n's
// locale is a Ref<string | Composer> depending on legacy mode; we read .value
// when available so the template can compare with `locale === 'ru'`.
const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// Map a Shikimori genre ID to a Tailwind bg+text class pair, falling back
// to a neutral pair so unmapped genres still render legibly (HSB-V11-AOD-04).
const GENRE_FALLBACK_CLASS = 'bg-white/10 text-gray-300'
function genreColorClass(id: string): string {
  return cardTokens.anime_of_day.genreColors[id] ?? GENRE_FALLBACK_CLASS
}
</script>
