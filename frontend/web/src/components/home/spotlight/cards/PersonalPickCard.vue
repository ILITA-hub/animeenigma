<template>
  <article
    class="w-full h-full flex flex-col gap-3 p-4 md:p-4 lg:p-6"
  >
    <header>
      <h3
        class="text-sm font-medium text-cyan-400 uppercase tracking-wider"
      >
        {{
          data.source === 'trending'
            ? t('spotlight.personalPick.titleAnon')
            : t('spotlight.personalPick.title')
        }}
      </h3>
    </header>

    <div
      class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0"
    >
      <router-link
        v-for="(item, i) in data.items.slice(0, 3)"
        v-show="i === 0 || mdAndUp"
        :key="item.anime.id"
        :to="`/anime/${item.anime.id}`"
        class="flex flex-col gap-2 group min-w-0 min-h-0"
      >
        <div
          class="relative rounded-lg overflow-hidden bg-white/5 flex-1 min-h-0"
        >
          <img
            :src="item.anime.poster_url || '/placeholder.svg'"
            :alt="
              getLocalizedTitle(
                item.anime.name,
                item.anime.name_ru,
                item.anime.name_jp,
              )
            "
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
        </div>
        <h4
          class="text-sm font-medium text-white truncate"
        >
          {{
            getLocalizedTitle(
              item.anime.name,
              item.anime.name_ru,
              item.anime.name_jp,
            )
          }}
        </h4>
        <p
          v-if="item.reason_i18n_key"
          class="text-xs font-medium text-cyan-300/80 truncate"
        >
          {{ t(item.reason_i18n_key) }}
        </p>
      </router-link>
    </div>

    <router-link
      v-if="data.items.length > 1"
      :to="data.source === 'trending' ? '/browse?sort=trending' : '/recs'"
      class="md:hidden text-sm font-medium text-cyan-400 self-start"
    >
      {{
        t('spotlight.personalPick.moreLink', {
          n: data.items.length - 1,
        })
      }}
    </router-link>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { getLocalizedTitle } from '@/utils/title'
import type { PersonalPickData } from '@/types/spotlight'

defineProps<{ data: PersonalPickData }>()

const { t } = useI18n()

// Tailwind `md` breakpoint = 768px. On mobile (< md) we show only the first
// pick and surface the "+ N more →" router-link footer; on md+ we show the
// full row of up to 3 picks.
const mdAndUp = useMediaQuery('(min-width: 768px)')
</script>
