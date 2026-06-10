<template>
  <div class="group block relative">
    <!-- Full-card SPA link, sits behind the kebab cluster -->
    <router-link
      :to="model.href"
      class="absolute inset-0 z-0 rounded-xl"
      :aria-label="model.title"
    />

    <div class="rounded-xl overflow-hidden bg-white/5 border border-white/10 pointer-events-none transition-[border-color,box-shadow] duration-200 group-hover:border-white/20 group-hover:shadow-[0_10px_30px_rgba(0,0,0,0.4)]">
      <PosterImage
        :src="model.coverImage"
        :alt="model.title"
        ratio="2/3"
        scrim
        :proxy-width="384"
      >
        <!-- Hover dim — lets the centered controls read against bright posters -->
        <div
          class="absolute inset-0 bg-black/45 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none"
          aria-hidden="true"
        />

        <!-- Top-left: quality + DUB + ONGOING stack -->
        <div class="absolute top-2 left-2 flex flex-col items-start gap-1">
          <Badge v-if="model.quality" variant="default" size="sm" :overlay="true">{{ model.quality }}</Badge>
          <Badge v-if="model.hasDub" variant="default" size="sm" :overlay="true">{{ dubLabel }}</Badge>
          <Badge v-if="model.airing" variant="success" size="sm" :overlay="true" data-testid="ongoing" class="gap-1">
            <span class="inline-block w-1.5 h-1.5 rounded-full bg-success animate-pulse" />
            {{ airingLabel }}
          </Badge>
        </div>

        <!-- Top-right: score cluster — STAYS visible on hover (no opacity-0) -->
        <div
          v-if="model.malScore || model.siteScore"
          data-testid="score-cluster"
          class="absolute top-2 right-2 flex flex-col items-end gap-1"
        >
          <Badge
            v-if="model.malScore"
            variant="warning"
            size="sm"
            :overlay="true"
            data-testid="score"
            class="gap-1 tabular-nums"
          >
            <Star class="size-3" fill="currentColor" aria-hidden="true" />
            {{ model.malScore.toFixed(1) }}
          </Badge>
          <Badge
            v-if="model.siteScore"
            variant="primary"
            size="sm"
            :overlay="true"
            data-testid="score"
            class="gap-1 tabular-nums"
          >
            <ScoreDiamond class="size-3" />
            {{ model.siteScore.toFixed(1) }}
          </Badge>
        </div>

        <!-- Bottom-left: watchlist status + progress -->
        <div
          v-if="model.listStatus || progressText"
          class="absolute bottom-2 left-2 flex flex-col gap-1 items-start"
        >
          <Badge v-if="model.listStatus" :variant="statusVariant" size="sm" :overlay="true">
            {{ statusLabel }}
          </Badge>
          <Badge v-if="progressText" variant="default" size="sm" :overlay="true">{{ progressText }}</Badge>
        </div>

        <!-- Centered play + kebab cluster, equal size, hover reveal -->
        <div
          data-testid="play-cluster"
          class="absolute inset-0 flex items-center justify-center gap-3 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none"
        >
          <router-link
            :to="model.href"
            :aria-label="model.title"
            class="w-12 h-12 rounded-full bg-cyan-500/90 flex items-center justify-center shadow-[0_0_20px_rgba(0,212,255,0.5)] pointer-events-auto transition-all duration-200 hover:bg-cyan-500 hover:rotate-[12deg] hover:scale-110"
          >
            <Play class="size-5 text-white ml-0.5" fill="currentColor" aria-hidden="true" />
          </router-link>
          <AnimeKebab
            :menu-open="menuOpen"
            class="static opacity-100 scale-100 w-12 h-12"
            @open="(el: HTMLElement) => emit('openMenu', el)"
          />
        </div>
      </PosterImage>

      <!-- Content -->
      <div class="px-2.5 pt-2.5 pb-3">
        <h3 class="font-medium text-white line-clamp-2 mb-1 text-[13px] leading-[1.3] min-h-[2.6em] group-hover:text-cyan-400 transition-colors">
          {{ model.title }}
        </h3>
        <div class="flex items-center gap-1 text-[11px] text-white/50 whitespace-nowrap overflow-hidden min-h-[1.5em]">
          <span v-if="model.year">{{ model.year }}</span>
          <span v-if="model.year && model.episodes" class="text-white/30">•</span>
          <span v-if="model.episodes">{{ model.episodes }} {{ episodeLabel }}</span>
          <span v-if="model.episodes && model.primaryGenre" class="text-white/30">•</span>
          <span v-if="model.primaryGenre">{{ model.primaryGenre }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Star, Play } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import ScoreDiamond from '@/components/ui/ScoreDiamond.vue'
import AnimeKebab from './AnimeKebab.vue'
import PosterImage from './PosterImage.vue'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{
  model: AnimeCardModel
  menuOpen?: boolean
}>()

const emit = defineEmits<{ openMenu: [el: HTMLElement] }>()

const { t } = useI18n()

// Script-level computed labels — avoids $t() in template branches that are
// asserted in unit tests where the global i18n plugin is NOT installed.
const airingLabel = computed(() => t('home.airing'))
const dubLabel = computed(() => t('card.dubBadge'))
const episodeLabel = computed(() => t('anime.episode'))

const statusKey = computed(() => {
  const map: Record<string, string> = {
    watching: 'profile.watchlist.watching',
    plan_to_watch: 'profile.watchlist.planToWatch',
    completed: 'profile.watchlist.completed',
    on_hold: 'profile.watchlist.onHold',
    dropped: 'profile.watchlist.dropped',
  }
  return map[props.model.listStatus || ''] || ''
})

const statusLabel = computed(() => statusKey.value ? t(statusKey.value) : '')

const statusVariant = computed<'primary' | 'success' | 'warning' | 'destructive' | 'default'>(() => {
  switch (props.model.listStatus) {
    case 'watching': return 'primary'
    case 'completed': return 'success'
    case 'on_hold': return 'warning'
    case 'dropped': return 'destructive'
    default: return 'default'
  }
})

const progressText = computed(() => {
  const p = props.model.progress
  if (!p || p.current <= 0) return ''
  if (props.model.listStatus === 'completed') return ''
  return t('card.episodeProgress', { n: p.current, total: p.total ?? '?' })
})
</script>
