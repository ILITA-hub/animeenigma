<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop
      variant="poster-blur"
      accent="amber"
      :poster-url="data.anime.poster_url"
    />
    <!-- Amber secondary overlay — the warm "nostalgia / reminder" wash that
         distinguishes this card from FeaturedCard's cyan. -->
    <div
      aria-hidden="true"
      class="absolute inset-0 bg-gradient-to-r from-amber-500/30 via-transparent to-transparent"
    />

    <div
      class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-6 lg:p-8 md:items-center"
    >
      <!-- Poster -->
      <router-link
        :to="`/anime/${data.anime.id}`"
        class="flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-52 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-amber-500/20"
        >
          <img
            :src="data.anime.poster_url || '/placeholder.svg'"
            :alt="title"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
        </div>
      </router-link>

      <!-- Meta -->
      <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
        <div>
          <!-- Clock-icon header -->
          <div class="flex items-center gap-2 mb-3">
            <SpotlightIcon
              name="clock"
              class="w-5 h-5 text-amber-300"
            />
            <p
              class="text-amber-200 text-sm font-semibold uppercase tracking-[0.15em]"
            >
              {{ t('spotlight.notTimeYet.title') }}
            </p>
          </div>

          <!-- Status pill -->
          <span
            class="inline-flex items-center gap-1 mb-2 px-2.5 py-1 rounded-md text-xs font-semibold"
            :class="statusPillClass"
          >
            {{ statusLabel }}
          </span>

          <h3
            class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2"
          >
            {{ title }}
          </h3>

          <p
            v-if="data.anime.episodes_count"
            class="mt-2 text-sm nty-muted font-medium"
          >
            {{
              t('spotlight.featured.episodesLabel', {
                n: data.anime.episodes_count,
              })
            }}
          </p>

          <p
            v-if="addedAtLabel"
            class="mt-1 text-xs text-amber-300/70 font-medium"
          >
            {{ addedAtLabel }}
          </p>
        </div>

        <!-- Direct-to-watch CTA — the user already bookmarked this anime, so
             deep-link straight to the player rather than the detail page. -->
        <router-link
          :to="`/anime/${data.anime.id}/watch`"
          class="cta-hero"
          data-accent="amber"
        >
          {{ t('spotlight.notTimeYet.watchCta') }}
          <SpotlightIcon
            name="play"
            class="w-4 h-4"
          />
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import { getLocalizedTitle } from '@/utils/title'
import { formatAgo } from '@/utils/time'
import type { NotTimeYetData } from '@/types/spotlight'

const props = defineProps<{ data: NotTimeYetData }>()
const { t, locale: i18nLocale } = useI18n()

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const title = computed<string>(() =>
  getLocalizedTitle(
    props.data.anime.name,
    props.data.anime.name_ru,
    props.data.anime.name_jp,
  ),
)

const statusLabel = computed<string>(() =>
  props.data.status === 'planned'
    ? t('spotlight.notTimeYet.statusPlanned')
    : t('spotlight.notTimeYet.statusPostponed'),
)

const statusPillClass = computed<string>(() =>
  props.data.status === 'planned'
    ? 'bg-yellow-500/20 text-yellow-200'
    : 'bg-slate-500/20 text-slate-300',
)

const addedAtLabel = computed<string | null>(() => {
  if (!props.data.added_at) return null
  return t('spotlight.notTimeYet.addedAt', {
    ago: formatAgo(props.data.added_at, localeStr.value),
  })
})
</script>

<style scoped>
/* Neon Tokyo token replacements (feat/homepage-neon-tokyo-redesign). */
.nty-muted { color: var(--ink-3); }
</style>
