<template>
  <router-link :to="model.href" class="mtile group">
    <PosterImage :src="model.coverImage" :alt="model.title" ratio="16/9" rounded="lg" scrim>
      <!-- Hover dim so the play control reads against bright posters -->
      <div class="mtile-ovl" aria-hidden="true" />

      <!-- Centered play, hover reveal -->
      <span class="mtile-play" aria-hidden="true">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><path d="M5 3l14 9-14 9V3z" /></svg>
      </span>

      <!-- Info overlay (bottom) -->
      <div class="mtile-info">
        <div v-if="model.nextEpisode" class="mtile-kicker" data-testid="kicker">
          {{ kickerLabel }}
          <template v-if="model.episodes"> · {{ model.episodes }}</template>
        </div>
        <div class="mtile-title">{{ model.title }}</div>
      </div>

      <!-- Progress bar -->
      <div v-if="(progressPct ?? 0) > 0" class="mtile-progress" data-testid="progress">
        <div class="mtile-progress-fill" :style="{ width: progressPct + '%', minWidth: '4px' }" />
      </div>
    </PosterImage>
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PosterImage from './PosterImage.vue'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{
  model: AnimeCardModel
  progressPct?: number
}>()

const { t } = useI18n()

// Script-level computed — avoids $t() in the template branch asserted in unit
// tests where the global i18n plugin is NOT installed (mirrors PosterCard pattern).
const kickerLabel = computed(() =>
  t('home.continueWatchingEpisode', { n: props.model.nextEpisode?.ep ?? 0 }),
)
</script>

<style scoped>
.mtile {
  position: relative;
  scroll-snap-align: start;
  display: block;
  border-radius: var(--r-lg);
  cursor: pointer;
  text-decoration: none;
  color: inherit;
  border: 1px solid var(--line);
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;
}
.mtile:hover { transform: translateY(-2px); box-shadow: var(--accent-glow); border-color: var(--accent-line); }
.mtile:focus-visible { outline: 2px solid var(--brand-cyan); outline-offset: 2px; }

.mtile-ovl { position: absolute; inset: 0; background: rgba(0, 0, 0, 0.45); opacity: 0; transition: opacity 0.2s ease; z-index: 1; pointer-events: none; }
.mtile:hover .mtile-ovl { opacity: 1; }

.mtile-play {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 52px;
  height: 52px;
  border-radius: 999px;
  background: rgba(0, 212, 255, 0.95);
  display: grid;
  place-items: center;
  color: var(--background);
  opacity: 0;
  transition: opacity 0.2s ease, transform 0.2s ease;
  box-shadow: 0 0 24px rgba(0, 212, 255, 0.5);
  z-index: 2;
}
.mtile:hover .mtile-play { opacity: 1; transform: translate(-50%, -50%) scale(1.06); }

.mtile-info { position: absolute; left: 14px; right: 14px; bottom: 12px; z-index: 2; }
.mtile-kicker {
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.1em;
  color: var(--brand-cyan);
  text-transform: uppercase;
  margin-bottom: 4px;
}
.mtile-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mtile-progress { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgba(255, 255, 255, 0.08); z-index: 3; }
.mtile-progress-fill { height: 100%; background: var(--brand-cyan); box-shadow: 0 0 8px var(--brand-cyan); transition: width 0.3s ease; }
</style>
