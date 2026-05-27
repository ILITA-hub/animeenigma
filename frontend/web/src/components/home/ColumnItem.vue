<template>
  <router-link
    :to="itemRoute"
    class="item"
    :class="{ 'top-3': variant === 'top' && rank !== undefined && rank <= 3 }"
    @touchstart="(e: TouchEvent) => emit('touchstart', e)"
    @touchmove="emit('touchmove')"
    @touchend="emit('touchend')"
  >
    <!-- rank numeral (top variant only) -->
    <div v-if="variant === 'top' && rank !== undefined" class="rank" aria-hidden="true">{{ rank }}</div>

    <!-- kebab context menu button -->
    <AnimeKebab
      :menu-open="menuOpen"
      @open="(el: HTMLElement) => emit('openMenu', el)"
    />

    <!-- poster -->
    <img
      :src="anime.poster_url || '/placeholder.svg'"
      :alt="localizedTitle"
      class="poster"
    />

    <!-- body -->
    <div class="body">
      <div class="title">{{ localizedTitle }}</div>
      <div class="meta">
        <span v-if="anime.year">{{ anime.year }}</span>
        <template v-if="anime.year && anime.episodes_count">
          <span class="sep">·</span>
        </template>
        <span v-if="anime.episodes_count">{{ $t('home.episodeCount', { count: anime.episodes_count }) }}</span>
      </div>

      <div class="chips">
        <!-- ongoing: airing chip -->
        <template v-if="variant === 'ongoing'">
          <span class="chip airing">● {{ $t('home.airing') }}</span>
        </template>

        <!-- announced: announce + season chips -->
        <template v-if="variant === 'announced'">
          <span class="chip announced">{{ $t('home.announced') }}</span>
          <span v-if="anime.season" class="chip season">{{ $t(`seasons.${anime.season}`) }}</span>
        </template>

        <!-- score chips (shikimori + site) — shown for ongoing and top -->
        <template v-if="variant !== 'announced'">
          <span v-if="anime.score" class="chip score">
            <!-- star icon -->
            <svg width="10" height="10" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ anime.score.toFixed(1) }}
          </span>
          <span v-if="siteRating && siteRating.total_reviews > 0" class="chip score site-score">
            <svg width="10" height="10" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ siteRating.average_score.toFixed(1) }}
          </span>
        </template>

        <!-- top variant: status chip -->
        <span v-if="variant === 'top' && anime.status" class="chip" style="background:rgba(167,139,250,0.14);color:var(--violet)">
          {{ $t(`anime.status.${anime.status}`) }}
        </span>
      </div>

      <!-- next-ep line (ongoing only) -->
      <div v-if="variant === 'ongoing' && anime.next_episode_at" class="next-ep">
        <!-- clock icon -->
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ $t('home.nextEpisodeLine', { n: (anime.episodes_aired || 0) + 1, when: formattedNextEp }) }}
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import { AnimeKebab } from '@/components/anime'
import type { HomeAnime } from '@/stores/home'

const props = defineProps<{
  anime: HomeAnime
  variant: 'ongoing' | 'top' | 'announced'
  rank?: number
  menuOpen?: boolean
  siteRating?: { average_score: number; total_reviews: number } | null
}>()

const emit = defineEmits<{
  (e: 'openMenu', el: HTMLElement): void
  (e: 'touchstart', event: TouchEvent): void
  (e: 'touchmove'): void
  (e: 'touchend'): void
}>()

const { t } = useI18n()

const localizedTitle = computed(() =>
  getLocalizedTitle(props.anime.name, props.anime.name_ru, props.anime.name_jp) || ''
)

const itemRoute = computed(() => {
  if (
    props.variant === 'ongoing' &&
    props.anime.next_episode_at &&
    props.anime.episodes_aired !== undefined
  ) {
    return `/anime/${props.anime.id}?episode=${(props.anime.episodes_aired || 0) + 1}`
  }
  return `/anime/${props.anime.id}`
})

const formattedNextEp = computed(() => {
  if (!props.anime.next_episode_at) return ''
  const date = new Date(props.anime.next_episode_at)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  const timeStr = date.toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Moscow',
  })

  if (diffDays === 0) {
    return t('home.todayAt', { time: timeStr })
  } else if (diffDays === 1) {
    return t('home.tomorrowAt', { time: timeStr })
  } else if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('home.dayAt', { day: t(`schedule.daysShort.${dayKeys[date.getDay()]}`), time: timeStr })
  } else {
    return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
  }
})
</script>

<style scoped>
/* Preserve kebab positioning from the parent context — it uses absolute
   positioning with z-index, so the item itself needs position:relative */
.item {
  position: relative;
  display: grid;
  grid-template-columns: 56px 1fr;
  gap: 12px;
  padding: 10px;
  border-radius: 12px; /* --r-md */
  transition: background 0.15s ease;
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  overflow: hidden;
}
.item:hover {
  background: rgba(255, 255, 255, 0.03);
}

.poster {
  width: 56px;
  aspect-ratio: 2 / 3;
  object-fit: cover;
  border-radius: 8px; /* --r-sm */
  border: 1px solid var(--line);
  flex-shrink: 0;
}

.body {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.title {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.item:hover .title {
  color: var(--accent);
}

.meta {
  font-size: 11px;
  color: var(--ink-3);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.sep { opacity: 0.5; }

.chips {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
  margin-top: 2px;
}

.chip {
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  padding: 2px 6px;
  border-radius: 4px;
  text-transform: uppercase;
}
.chip.airing    { background: rgba(0, 255, 157, 0.14); color: var(--color-success); }
.chip.announced { background: rgba(0, 212, 255, 0.14); color: var(--accent); }
.chip.season    { background: rgba(167, 139, 250, 0.14); color: var(--violet); }
.chip.score {
  background: rgba(255, 214, 0, 0.14);
  color: var(--color-warning);
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
/* Site score chip is cyan to distinguish from shikimori score */
.chip.site-score {
  background: rgba(0, 212, 255, 0.14);
  color: var(--accent);
}

.next-ep {
  font-family: var(--f-mono);
  font-size: 10px;
  color: var(--accent);
  letter-spacing: 0.04em;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 2px;
}

/* Top variant: rank numeral */
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
.top-3 .rank {
  color: rgba(0, 212, 255, 0.08);
}

/* Push content above the rank numeral */
.poster,
.body {
  position: relative;
  z-index: 1;
}
</style>
