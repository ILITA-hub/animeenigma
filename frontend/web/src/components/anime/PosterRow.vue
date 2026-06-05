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
      <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
        <circle cx="10" cy="4" r="1.5" />
        <circle cx="10" cy="10" r="1.5" />
        <circle cx="10" cy="16" r="1.5" />
      </svg>
    </button>

    <img :src="posterSrc" :alt="model.title" class="poster" loading="lazy" @error="onPosterError" />

    <div class="body">
      <div class="title truncate" data-testid="row-title">{{ model.title }}</div>

      <div class="meta">
        <span v-if="model.year">{{ model.year }}</span>
        <template v-if="model.year && model.episodes"><span class="sep">·</span></template>
        <span v-if="model.episodes">{{ episodeCountLabel }}</span>
      </div>

      <div class="chips">
        <span v-if="variant === 'ongoing' && model.airing" class="chip airing" data-testid="airing">● {{ airingLabel }}</span>
        <template v-if="variant === 'announced'">
          <span class="chip announced">{{ announcedLabel }}</span>
          <span v-if="season" class="chip season" data-testid="season">{{ seasonLabel }}</span>
        </template>
        <template v-if="variant !== 'announced'">
          <span v-if="model.malScore" class="chip score tabular-nums">
            <svg width="10" height="10" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ model.malScore.toFixed(1) }}
          </span>
          <span v-if="model.siteScore" class="chip score site-score tabular-nums">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 2l9 10-9 10L3 12z" />
            </svg>
            {{ model.siteScore.toFixed(1) }}
          </span>
        </template>
      </div>

      <div v-if="variant === 'ongoing' && model.nextEpisode" class="next-ep" data-testid="next-ep">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ nextEpLabel }}
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
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

const posterFailed = ref(false)
const posterSrc = computed(() => (!posterFailed.value && props.model.coverImage ? props.model.coverImage : '/placeholder.svg'))
function onPosterError() { posterFailed.value = true }

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
  const timeStr = date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' })
  if (diffDays === 0) return t('home.todayAt', { time: timeStr })
  if (diffDays === 1) return t('home.tomorrowAt', { time: timeStr })
  if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('home.dayAt', { day: t(`schedule.daysShort.${dayKeys[date.getDay()]}`), time: timeStr })
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
}
.prow:hover { background: rgba(255, 255, 255, 0.03); }

.poster {
  width: 56px;
  aspect-ratio: 2 / 3;
  height: 84px;
  overflow: hidden;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid var(--line);
  flex-shrink: 0;
  position: relative;
  z-index: 1;
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
}
.prow:hover .title { color: var(--brand-cyan); }

.meta { font-size: 11px; color: var(--muted-foreground); display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.sep { opacity: 0.5; }

.chips { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; margin-top: 2px; }
.chip {
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  padding: 2px 6px;
  border-radius: 4px;
  text-transform: uppercase;
}
.chip.airing    { background: rgba(0, 255, 157, 0.14); color: var(--color-success); }
.chip.announced { background: rgba(0, 212, 255, 0.14); color: var(--brand-cyan); }
.chip.season    { background: rgba(167, 139, 250, 0.14); color: var(--violet); }
.chip.score { background: rgba(255, 214, 0, 0.14); color: var(--color-warning); display: inline-flex; align-items: center; gap: 4px; }
.chip.site-score { background: rgba(0, 212, 255, 0.14); color: var(--brand-cyan); }

.next-ep {
  font-family: var(--f-mono);
  font-size: 10px;
  color: var(--brand-cyan);
  letter-spacing: 0.04em;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 2px;
}

.rank {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  font-family: var(--f-display);
  font-weight: 800;
  font-size: 56px;
  letter-spacing: -0.04em;
  color: rgba(255, 255, 255, 0.04);
  pointer-events: none;
  line-height: 1;
  z-index: 0;
  user-select: none;
}
.is-top3 .rank { color: rgba(0, 212, 255, 0.08); }

/* Centered-glass kebab — vertically centered on the right edge, hover reveal */
.rkc {
  position: absolute;
  top: 50%;
  right: 8px;
  z-index: 6;
  width: 34px;
  height: 34px;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(6px);
  color: var(--foreground);
  display: grid;
  place-items: center;
  transform: translateY(-50%);
  opacity: 0;
  transition: opacity 0.18s ease, background 0.18s ease;
}
.prow:hover .rkc { opacity: 1; }
.rkc:hover { background: rgba(0, 212, 255, 0.9); }
</style>
