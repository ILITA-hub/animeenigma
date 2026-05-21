<template>
  <article
    class="w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6"
  >
    <header class="md:hidden">
      <p
        class="text-xs font-medium text-purple-300/90 uppercase tracking-wider mb-1"
      >
        {{ t('spotlight.continueWatchingNew.title') }}
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
          :alt="
            getLocalizedTitle(
              data.anime.name,
              data.anime.name_ru,
              data.anime.name_jp,
            )
          "
          class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
          loading="lazy"
        />
        <span
          class="absolute top-2 right-2 px-2 py-0.5 text-xs font-semibold bg-purple-500/90 text-white rounded shadow"
        >
          {{
            t('spotlight.continueWatchingNew.newEpisodeBadge', {
              n: data.new_episode_number,
            })
          }}
        </span>
      </div>
    </router-link>

    <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
      <div>
        <p
          class="hidden md:block text-xs font-medium text-purple-300/90 uppercase tracking-wider mb-2"
        >
          {{ t('spotlight.continueWatchingNew.title') }}
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
          class="mt-2 text-sm font-medium text-gray-400"
        >
          {{
            t('spotlight.animeOfDay.episodesLabel', {
              n: data.last_watched_episode,
            })
          }}
        </p>
      </div>

      <div class="flex flex-wrap gap-2 mt-3">
        <router-link
          :to="`/anime/${data.anime.id}`"
          class="btn btn-primary text-sm md:text-base"
        >
          {{ t('spotlight.continueWatchingNew.resumeCta') }}
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { ContinueWatchingNewData } from '@/types/spotlight'

defineProps<{ data: ContinueWatchingNewData }>()
const { t } = useI18n()
</script>
