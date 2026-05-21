<template>
  <article
    class="w-full h-full flex flex-col gap-3 p-4 md:p-4 lg:p-6"
  >
    <header>
      <h3
        class="text-lg md:text-xl font-semibold text-white"
      >
        {{ t('spotlight.nowWatching.title') }}
      </h3>
    </header>

    <ul class="flex-1 flex flex-col gap-2 min-h-0">
      <li
        v-for="s in data.sessions.slice(0, 3)"
        :key="`${s.public_id}:${s.anime_id}:${s.episode_number}`"
        class="min-w-0"
      >
        <router-link
          :to="`/anime/${s.anime_id}`"
          class="flex items-center gap-3 p-2 rounded-lg hover:bg-white/5 transition-colors min-w-0"
        >
          <span
            aria-hidden="true"
            class="inline-block w-2 h-2 rounded-full bg-green-400 animate-pulse flex-shrink-0"
          />
          <img
            v-if="s.poster_url"
            :src="s.poster_url"
            :alt="''"
            class="w-8 h-11 object-cover rounded flex-shrink-0"
            loading="lazy"
          />
          <p
            class="flex-1 text-sm font-medium text-white truncate"
          >
            {{
              t('spotlight.nowWatching.sessionLabel', {
                username: s.username,
                anime: getLocalizedTitle(s.anime_name, s.anime_name_ru),
                n: s.episode_number,
              })
            }}
          </p>
          <span
            class="ml-auto text-xs font-semibold text-green-400 flex-shrink-0"
          >
            {{ t('spotlight.nowWatching.liveBadge') }}
          </span>
        </router-link>
      </li>
    </ul>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { NowWatchingData } from '@/types/spotlight'

defineProps<{ data: NowWatchingData }>()
const { t } = useI18n()
</script>
