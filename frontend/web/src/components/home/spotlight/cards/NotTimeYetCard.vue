<template>
  <SpotlightCardShell
    accent="violet"
    icon="clock"
    :kicker="t('spotlight.notTimeYet.title')"
    backdrop="poster-blur"
    :poster-url="data.anime.poster_url"
  >
    <!-- Violet secondary overlay — the "reminder" wash that distinguishes
         this card from FeaturedCard's cyan (brand triad: meta/service). -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-brand-violet/25 via-transparent to-transparent"
      />
    </template>

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-6 md:items-center">
      <!-- Poster -->
      <router-link
        :to="`/anime/${data.anime.id}`"
        class="flex-shrink-0 self-center w-24 md:w-32 lg:w-40 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-brand-violet/20"
        >
          <img
            :src="cardPosterUrl(data.anime.poster_url, 256)"
            :alt="title"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
            decoding="async"
          />
        </div>
      </router-link>

      <!-- Meta -->
      <div class="flex-1 min-w-0">
        <Badge size="sm" overlay :class="statusAccentClass" class="mb-2">
          {{ statusLabel }}
        </Badge>

        <h3
          class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2"
        >
          {{ title }}
        </h3>

        <p
          v-if="data.anime.episodes_count"
          class="mt-2 text-sm text-muted-foreground font-medium"
        >
          {{
            t('spotlight.featured.episodesLabel', {
              n: data.anime.episodes_count,
            })
          }}
        </p>

        <p
          v-if="addedAtLabel"
          class="mt-1 text-xs text-brand-violet/80 font-medium"
        >
          {{ addedAtLabel }}
        </p>
      </div>
    </div>

    <!-- Direct-to-watch CTA — the user already bookmarked this anime, so
         deep-link straight to the player rather than the detail page. -->
    <template #cta>
      <router-link
        :to="`/anime/${data.anime.id}/watch`"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        <Play class="w-4 h-4" fill="currentColor" aria-hidden="true" />
        {{ t('spotlight.notTimeYet.watchCta') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
// Workstream hero-spotlight — DS alignment 2026-06-10: SpotlightCardShell
// anatomy (violet kicker, CTA bottom-left via Button default variant),
// status as an overlay Badge (the backdrop is poster imagery).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import { getLocalizedTitle } from '@/utils/title'
import { formatAgo } from '@/utils/time'
import type { NotTimeYetData } from '@/types/spotlight'
import { cardPosterUrl } from '@/composables/useImageProxy'

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

// Accent TEXT class layered over the overlay Badge's dark glass: planned →
// warning amber (a reminder), postponed → neutral muted.
const statusAccentClass = computed<string>(() =>
  props.data.status === 'planned' ? 'text-warning' : 'text-muted-foreground',
)

const addedAtLabel = computed<string | null>(() => {
  if (!props.data.added_at) return null
  return t('spotlight.notTimeYet.addedAt', {
    ago: formatAgo(props.data.added_at, localeStr.value),
  })
})
</script>
