<template>
  <router-link :to="to" class="prow group" :class="{ 'is-top3': variant === 'top' && rank !== undefined && rank <= 3 }">
    <!-- rank numeral (top variant) -->
    <div v-if="variant === 'top' && rank !== undefined" class="rank" aria-hidden="true" data-testid="rank">{{ rank }}</div>

    <!-- centered-glass kebab -->
    <button
      ref="kebabEl"
      type="button"
      class="rkc"
      data-testid="row-kebab"
      :aria-label="openMenuLabel"
      aria-haspopup="menu"
      :aria-expanded="menuOpen"
      @click.prevent.stop="onKebab"
    >
      <EllipsisVertical class="size-4" />
    </button>

    <PosterImage
      :src="model.coverImage || '/placeholder.svg'"
      :alt="model.title"
      ratio="2/3"
      rounded="lg"
      :proxy-width="128"
      class="poster-wrap"
    />

    <div class="body">
      <div class="title truncate" data-testid="row-title">{{ model.title }}</div>

      <div class="meta">
        <span v-if="model.year">{{ model.year }}</span>
        <template v-if="model.year && model.episodes"><span class="sep">·</span></template>
        <span v-if="model.episodes">{{ episodeCountLabel }}</span>
      </div>

      <div class="chips">
        <span v-if="variant === 'ongoing'" class="chip airing" data-testid="airing">● {{ airingLabel }}</span>
        <template v-if="variant === 'announced'">
          <span class="chip announced">{{ announcedLabel }}</span>
          <span v-if="season" class="chip season" data-testid="season">{{ seasonLabel }}</span>
        </template>
        <template v-if="variant !== 'announced'">
          <span v-if="model.malScore" class="chip score tabular-nums">
            <Star class="size-[10px]" fill="currentColor" aria-hidden="true" />
            {{ model.malScore.toFixed(1) }}
          </span>
          <span v-if="model.siteScore" class="chip score site-score tabular-nums">
            <ScoreDiamond class="size-[10px]" />
            {{ model.siteScore.toFixed(1) }}
          </span>
        </template>
      </div>

      <div v-if="variant === 'ongoing' && model.nextEpisode" class="next-ep" data-testid="next-ep">
        <Clock class="size-[11px]" aria-hidden="true" />
        {{ nextEpLabel }}
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { EllipsisVertical, Star, Clock } from 'lucide-vue-next'
import ScoreDiamond from '@/components/ui/ScoreDiamond.vue'
import PosterImage from '@/components/anime/PosterImage.vue'
import { useUserTimezone } from '@/composables/useUserTimezone'
import { wallClockDate } from '@/composables/schedule/timezone'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{
  model: AnimeCardModel
  variant: 'ongoing' | 'top' | 'announced'
  rank?: number
  menuOpen?: boolean
  season?: string
}>()

const emit = defineEmits<{ openMenu: [el: HTMLElement] }>()

const { t } = useI18n()
const { timezone: userTimezone } = useUserTimezone()
const kebabEl = ref<HTMLButtonElement | null>(null)

function onKebab() {
  if (kebabEl.value) emit('openMenu', kebabEl.value)
}

// Ongoing deep-link parity: link to next episode when available
const to = computed(() =>
  props.variant === 'ongoing' && props.model.nextEpisode
    ? `${props.model.href}?episode=${props.model.nextEpisode.ep}`
    : props.model.href
)

// Script-level computed labels — avoids $t() in templates (no i18n plugin in unit tests)
const openMenuLabel = computed(() => t('contextMenu.openMenu'))
const airingLabel = computed(() => t('home.airing'))
const announcedLabel = computed(() => t('home.announced'))
const seasonLabel = computed(() => (props.season ? t(`seasons.${props.season}`) : ''))
const episodeCountLabel = computed(() => t('home.episodeCount', { count: props.model.episodes }))

const formattedNextEp = computed(() => {
  const when = props.model.nextEpisode?.when
  if (!when) return ''
  const date = new Date(when)
  const now = new Date()
  const diffDays = Math.floor((date.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
  const timeStr = date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: userTimezone.value })
  if (diffDays === 0) return t('home.todayAt', { time: timeStr })
  if (diffDays === 1) return t('home.tomorrowAt', { time: timeStr })
  if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('home.dayAt', { day: t(`schedule.daysShort.${dayKeys[wallClockDate(date, userTimezone.value).getDay()]}`), time: timeStr })
  }
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
})

const nextEpLabel = computed(() =>
  props.model.nextEpisode
    ? t('home.nextEpisodeLine', { n: props.model.nextEpisode.ep, when: formattedNextEp.value })
    : ''
)
</script>

<style scoped>
.prow {
  position: relative;
  display: grid;
  grid-template-columns: 56px 1fr;
  gap: 12px;
  padding: 10px;
  border-radius: 12px;
  transition: background 0.15s ease;
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  overflow: hidden;
  flex-shrink: 0;
  align-items: start;
  /* Skip layout/paint for rows scrolled out of the column list — cuts the
     whole-page style/layout cost that dominated the 379ms INP click in the
     2026-06-10 trace. Safe here: the row already clips (overflow:hidden),
     so the implied paint containment changes nothing visually. */
  content-visibility: auto;
  contain-intrinsic-size: 300px 104px;
}
.prow:hover { background: var(--white-a4); }

.poster-wrap {
  position: relative;
  width: 56px;
  height: 84px;
  aspect-ratio: 2 / 3;
  overflow: hidden;
  border-radius: 8px;
  border: 1px solid var(--line);
  flex-shrink: 0;
  z-index: 1;
  background: var(--color-surface);
}

.poster {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.body { min-width: 0; display: flex; flex-direction: column; gap: 4px; position: relative; z-index: 1; }

/* 1-line title → fixed body rhythm → info aligns across columns */
.title {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  /* reserve space so the 1-line title never collides with the rank / hover kebab */
  padding-right: 30px;
}
.prow:hover .title { color: var(--brand-cyan); }

/* nowrap meta + chips → fixed row height → info aligns across columns (design lock) */
.meta { font-size: 11px; color: var(--muted-foreground); display: flex; align-items: center; gap: 6px; white-space: nowrap; overflow: hidden; }
.sep { opacity: 0.5; }

.chips { display: flex; gap: 6px; align-items: center; flex-wrap: nowrap; overflow: hidden; }
.chip {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  padding: 2px 6px;
  border-radius: 4px;
  text-transform: uppercase;
}
.chip.airing    { background: var(--success-soft); color: var(--color-success); }
.chip.announced { background: var(--accent-soft); color: var(--brand-cyan); }
.chip.season    { background: rgba(167, 139, 250, 0.14); color: var(--brand-violet); }
.chip.score { background: var(--warning-soft); color: var(--color-warning); display: inline-flex; align-items: center; gap: 4px; }
.chip.site-score { background: var(--accent-soft); color: var(--brand-cyan); }

.next-ep {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--brand-cyan);
  letter-spacing: 0.04em;
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.rank {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  font-family: var(--font-display);
  font-weight: 800;
  font-size: 56px;
  letter-spacing: -0.04em;
  color: var(--white-a4);
  pointer-events: none;
  line-height: 1;
  z-index: 0;
  user-select: none;
}
.is-top3 .rank { color: var(--cyan-a08); }

/* Centered-glass kebab — vertically centered on the right edge, hover reveal.
   NOTE: backdrop-filter is deliberately NOT set at rest. A non-none
   backdrop-filter forces its element onto its own compositor layer even at
   opacity:0, so the idle (hover-hidden) kebab on every row would allocate a
   blur layer. A Home column renders ~15 rows, ×3 columns = ~45 idle blur
   layers, and content-visibility:auto creates them all in one Layerize pass
   the instant the column scrolls into view — the dominant cost in the
   2026-07-06 scroll trace. Applying the blur only on .prow:hover means the
   layer is created for the single row the pointer is over, on interaction,
   not for every off-screen row during scroll. */
.rkc {
  position: absolute;
  top: 50%;
  right: 8px;
  z-index: 6;
  width: 34px;
  height: 34px;
  border-radius: 9999px;
  background: var(--black-a60);
  color: var(--foreground);
  display: grid;
  place-items: center;
  transform: translateY(-50%);
  opacity: 0;
  transition: opacity 0.18s ease, background 0.18s ease;
}
.prow:hover .rkc {
  opacity: 1;
  -webkit-backdrop-filter: blur(6px);
  backdrop-filter: blur(6px);
}
.rkc:hover { background: var(--brand-cyan); }
</style>
