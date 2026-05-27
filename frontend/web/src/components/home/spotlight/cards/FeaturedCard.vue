<template>
  <!--
    Workstream hero-spotlight — Neon Tokyo redesign (feat/homepage-neon-tokyo-redesign).

    Full-bleed cinematic hero card for the "featured" spotlight card type.
    Full-bleed cinematic hero for the featured spotlight card type
    (type: 'featured') as the backend resolver was also renamed in the same
    release. Status-aware eyebrow/CTA:
      - ongoing  → "Now airing" + "Watch · ep. N" (aired+1)
      - announced → "Season announcement" + "Remind me" (links to detail)
      - released/other → "Featured today" + "Start watching"
  -->
  <article class="featured-hero">
    <div class="featured-bg" :style="{ backgroundImage: posterBg }" />
    <div class="featured-content">
      <p class="featured-eyebrow">
        <span class="pulse" aria-hidden="true" />
        {{ eyebrow }}
        <template v-if="data.anime.season"><span class="sep">·</span>{{ data.anime.season }}</template>
      </p>
      <h1 class="featured-title">
        {{ getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp) }}
        <span v-if="data.anime.name_jp" class="jp">{{ data.anime.name_jp }}</span>
      </h1>
      <div class="featured-meta">
        <span v-if="data.anime.score" class="score">
          <SpotlightIcon name="play" class="w-3.5 h-3.5" /> {{ data.anime.score.toFixed(1) }}
        </span>
        <span v-if="data.anime.year">{{ data.anime.year }}</span>
        <span v-if="data.anime.episodes_count" class="dot" />
        <span v-if="data.anime.episodes_count">{{ t('spotlight.featured.episodesLabel', { n: data.anime.episodes_count }) }}</span>
        <span
          v-for="g in (data.anime.genres || []).slice(0, 3)"
          :key="g.id"
          class="chip-genre"
        >
          {{ locale === 'ru' ? (g.russian || g.name) : (g.name || g.russian) }}
        </span>
      </div>
      <p v-if="data.anime.description" class="featured-desc">{{ data.anime.description }}</p>
      <div class="featured-actions">
        <router-link :to="watchTo" class="btn-primary-hero">
          <SpotlightIcon name="play" class="w-4 h-4" /> {{ primaryCta }}
        </router-link>
        <router-link :to="`/anime/${data.anime.id}`" class="btn-secondary-hero">
          {{ t('spotlight.featured.addCta') }}
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { FeaturedData } from '@/types/spotlight'
import SpotlightIcon from '../SpotlightIcon.vue'

const props = defineProps<{ data: FeaturedData }>()
const { t, locale: i18nLocale } = useI18n()

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const posterBg = computed(() =>
  props.data.anime.poster_url ? `url("${props.data.anime.poster_url}")` : 'none',
)

const eyebrow = computed(() => {
  switch (props.data.anime.status) {
    case 'ongoing': return t('spotlight.featured.eyebrowOngoing')
    case 'announced': return t('spotlight.featured.eyebrowAnnounced')
    default: return t('spotlight.featured.eyebrowDefault')
  }
})

const primaryCta = computed(() => {
  switch (props.data.anime.status) {
    case 'ongoing':
      return t('spotlight.featured.watchEpisode', { n: (props.data.anime.episodes_aired || 0) + 1 })
    case 'announced':
      return t('spotlight.featured.remindCta')
    default:
      return t('spotlight.featured.watchCta')
  }
})

const watchTo = computed(() =>
  props.data.anime.status === 'announced'
    ? `/anime/${props.data.anime.id}`
    : `/anime/${props.data.anime.id}/watch`,
)
</script>

<style scoped>
/* Values transcribed from the design handoff (.hero* rules). */
.featured-hero { position: relative; width: 100%; height: 100%; overflow: hidden; }
.featured-bg { position: absolute; inset: 0; background-size: cover; background-position: center 30%; filter: saturate(105%); }
.featured-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, rgba(8,8,15,.92) 0%, rgba(8,8,15,.72) 35%, rgba(8,8,15,.25) 65%, rgba(8,8,15,0) 100%),
    linear-gradient(180deg, rgba(8,8,15,0) 50%, rgba(8,8,15,.65) 100%);
}
.featured-content {
  position: absolute; inset: 0; z-index: 1;
  display: flex; flex-direction: column; justify-content: flex-end;
  padding: 40px 48px; gap: 20px; max-width: 720px;
}
.featured-eyebrow {
  display: inline-flex; align-items: center; gap: 10px;
  font-family: var(--f-mono); font-size: 11px; letter-spacing: .12em;
  text-transform: uppercase; color: var(--accent);
}
.featured-eyebrow .pulse { width: 6px; height: 6px; border-radius: 999px; background: var(--accent); box-shadow: 0 0 8px var(--accent); animation: featured-pulse 1.6s ease-in-out infinite; }
.featured-eyebrow .sep { opacity: .5; }
@keyframes featured-pulse { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: .5; transform: scale(.8); } }
.featured-title { font-family: var(--f-display); font-weight: 800; font-size: clamp(36px, 4vw, 56px); line-height: 1.02; letter-spacing: -.025em; text-wrap: balance; }
.featured-title .jp { display: block; font-family: var(--f-jp); font-weight: 500; font-size: .42em; letter-spacing: .02em; color: var(--ink-3); margin-top: 8px; }
.featured-meta { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; color: var(--ink-3); font-size: 13px; }
.featured-meta .dot { width: 3px; height: 3px; border-radius: 999px; background: currentColor; opacity: .4; }
.featured-meta .score { display: inline-flex; align-items: center; gap: 6px; color: var(--color-warning); font-weight: 600; }
.featured-meta .chip-genre { padding: 4px 10px; border-radius: 999px; border: 1px solid var(--line-strong); font-size: 12px; color: var(--ink-2); }
.featured-desc { font-size: 15px; line-height: 1.6; color: var(--ink-2); max-width: 540px; text-wrap: pretty; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.featured-actions { display: flex; gap: 10px; align-items: center; }
.btn-primary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: var(--accent); color: #001218; border-radius: 12px; font-weight: 700; font-size: 14px; transition: filter .15s ease, box-shadow .15s ease; }
.btn-primary-hero:hover { filter: brightness(1.08); box-shadow: var(--accent-glow); }
.btn-secondary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: rgba(255,255,255,.06); border: 1px solid var(--line-strong); border-radius: 12px; font-weight: 600; font-size: 14px; color: var(--ink); }
.btn-secondary-hero:hover { background: rgba(255,255,255,.1); }
@media (max-width: 640px) { .featured-content { padding: 24px; } }
</style>
