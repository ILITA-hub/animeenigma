<template>
  <article
    class="w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-4 lg:p-6"
  >
    <header class="md:hidden">
      <p
        class="text-xs font-medium text-cyan-300/80 uppercase tracking-wider mb-1"
      >
        {{ t('spotlight.notTimeYet.title') }}
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
      </div>
    </router-link>

    <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
      <div>
        <p
          class="hidden md:block text-xs font-medium text-cyan-300/80 uppercase tracking-wider mb-2"
        >
          {{ t('spotlight.notTimeYet.title') }}
        </p>
        <p
          class="text-sm font-medium text-gray-400 mb-2"
        >
          {{
            data.status === 'planned'
              ? t('spotlight.notTimeYet.subtitlePlanned')
              : t('spotlight.notTimeYet.subtitlePostponed')
          }}
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
      </div>

      <div class="flex flex-wrap gap-2 mt-3">
        <router-link
          :to="`/anime/${data.anime.id}`"
          class="btn btn-primary text-sm md:text-base"
        >
          {{ t('spotlight.notTimeYet.watchCta') }}
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { NotTimeYetData } from '@/types/spotlight'

defineProps<{ data: NotTimeYetData }>()
const { t } = useI18n()
</script>
