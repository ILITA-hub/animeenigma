<template>
  <article
    class="w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6"
  >
    <header class="md:hidden">
      <p
        class="text-xs font-medium text-cyan-400 uppercase tracking-wider mb-1"
      >
        {{ t('spotlight.animeOfDay.title') }}
      </p>
    </header>

    <router-link
      :to="`/anime/${data.anime.id}`"
      class="flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-48 group"
    >
      <div
        class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3]"
      >
        <img
          :src="data.anime.poster_url || '/placeholder.svg'"
          :alt="getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp)"
          class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
          loading="lazy"
        />
        <div
          v-if="data.anime.score"
          class="absolute top-2 right-2 px-2 py-1 rounded-md bg-black/70 backdrop-blur-sm text-xs font-semibold text-yellow-400 flex items-center gap-1"
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
        </div>
      </div>
    </router-link>

    <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
      <div>
        <p
          class="hidden md:block text-xs font-medium text-cyan-400 uppercase tracking-wider mb-2"
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
        <p
          v-if="data.anime.episodes_count"
          class="mt-2 text-sm text-gray-400 font-medium"
        >
          {{
            t('spotlight.animeOfDay.episodesLabel', {
              n: data.anime.episodes_count,
            })
          }}
        </p>

        <div
          v-if="data.anime.genres?.length"
          class="mt-3 flex flex-wrap gap-1"
        >
          <span
            v-for="g in data.anime.genres.slice(0, 3)"
            :key="g.id"
            class="px-2 py-0.5 text-xs font-medium rounded bg-white/10 text-gray-300"
          >
            {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
          </span>
        </div>
      </div>

      <div class="flex flex-wrap gap-2 mt-3">
        <router-link
          :to="`/anime/${data.anime.id}/watch`"
          class="btn btn-primary text-sm md:text-base"
        >
          {{ t('spotlight.animeOfDay.watchCta') }}
        </router-link>
        <button
          type="button"
          class="btn btn-ghost text-sm md:text-base"
          @click="onAdd"
        >
          {{ t('spotlight.animeOfDay.addCta') }}
        </button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { AnimeOfDayData } from '@/types/spotlight'

defineProps<{ data: AnimeOfDayData }>()

const { t, locale: i18nLocale } = useI18n()

// Normalize locale to a plain string for the genre-name selector. useI18n's
// locale is a Ref<string | Composer> depending on legacy mode; we read .value
// when available so the template can compare with `locale === 'ru'`.
const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

function onAdd() {
  // Phase 2: stubbed handler. Phase 3 will wire to the watchlist API.
  // Kept intentionally silent — no console.log noise in production.
}
</script>
