<template>
  <SpotlightCardShell
    accent="violet"
    icon="clock"
    :kicker="t('spotlight.notTimeYet.title')"
  >
  <!--
    Workstream hero-spotlight — v4 G-3 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). The pinned-note
    sticker: a slightly tilted glass note with a 📌 pin, the poster, an
    italic guilt-trip line («Ты собирался это посмотреть ещё {date}…»)
    and the «Ну ладно, время пришло» CTA. The tilt is decorative-only
    (straightened under prefers-reduced-motion via the media query).
  -->
    <div class="flex-1 min-h-0 flex items-center justify-center py-1">
      <div
        class="sticker relative bg-white/[0.06] border border-white/[0.14] rounded-2xl p-4 md:p-6 flex flex-col md:flex-row items-center gap-4 md:gap-6 max-w-[620px] min-w-0"
        data-testid="sticker"
      >
        <span
          class="absolute -top-3 left-1/2 -translate-x-1/2 text-[22px] pin"
          aria-hidden="true"
          >📌</span
        >

        <router-link :to="`/anime/${data.anime.id}`" class="flex-shrink-0">
          <SpotlightPoster
            :poster-url="data.anime.poster_url"
            :alt="title"
            width-class="w-24 md:w-28"
            glow="violet"
            :proxy-width="256"
          />
        </router-link>

        <div class="min-w-0 text-center md:text-left">
          <p class="text-[13px] text-muted-foreground font-medium italic">
            {{ stickerNote }}
          </p>
          <h3 class="mt-1.5 text-xl md:text-2xl font-semibold text-white leading-tight line-clamp-2">
            {{ title }}
          </h3>
          <div class="mt-2 flex items-center gap-2 justify-center md:justify-start flex-wrap">
            <Badge size="sm" overlay :class="statusAccentClass">
              {{ statusLabel }}
            </Badge>
            <span v-if="data.anime.episodes_count" class="text-xs text-muted-foreground font-medium">
              {{ t('spotlight.featured.episodesLabel', { n: data.anime.episodes_count }) }}
            </span>
          </div>
          <router-link
            :to="`/anime/${data.anime.id}/watch`"
            :class="[buttonVariants({ variant: 'default', size: 'sm' }), 'mt-4 text-[13px]']"
          >
            <Play class="w-3.5 h-3.5" fill="currentColor" aria-hidden="true" />
            {{ t('spotlight.notTimeYet.stickerCta') }}
          </router-link>
        </div>
      </div>
    </div>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import { getLocalizedTitle } from '@/utils/title'
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

// Accent TEXT class layered over the overlay Badge's dark glass: planned →
// warning amber (a reminder), postponed → neutral muted.
const statusAccentClass = computed<string>(() =>
  props.data.status === 'planned' ? 'text-warning' : 'text-muted-foreground',
)

// «Ты собирался это посмотреть ещё 20 мая…» — added_at formatted as a
// short date; the no-date variant drops the date clause entirely.
const stickerNote = computed<string>(() => {
  if (!props.data.added_at) return t('spotlight.notTimeYet.stickerNoteNoDate')
  const d = new Date(props.data.added_at)
  if (Number.isNaN(d.getTime())) return t('spotlight.notTimeYet.stickerNoteNoDate')
  let date: string
  try {
    date = new Intl.DateTimeFormat(localeStr.value, { day: 'numeric', month: 'long' }).format(d)
  } catch {
    date = d.toISOString().slice(0, 10)
  }
  return t('spotlight.notTimeYet.stickerNote', { date })
})
</script>

<style scoped>
/* The note's tilt + heavy drop shadow — the "pinned to a corkboard"
   gesture. Straightened for reduced-motion users (a static tilt is not
   motion, but the hover-settle below is). */
.sticker {
  transform: rotate(-1.6deg);
  box-shadow: 0 24px 50px var(--black-a60);
  transition: transform 0.25s ease;
}
.sticker:hover {
  transform: rotate(-0.4deg);
}
.pin {
  filter: drop-shadow(0 4px 8px var(--black-a60));
}
@media (prefers-reduced-motion: reduce) {
  .sticker,
  .sticker:hover {
    transform: none;
    transition: none;
  }
}
</style>
