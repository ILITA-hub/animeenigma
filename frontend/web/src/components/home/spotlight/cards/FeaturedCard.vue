<template>
  <!--
    Workstream hero-spotlight — Neon Tokyo redesign (feat/homepage-neon-tokyo-redesign).

    Full-bleed cinematic hero card for the "featured" spotlight card type
    (type: 'featured'). Status-aware eyebrow/CTA:
      - ongoing  → "Now airing" + "Watch · ep. N" (aired+1)
      - announced → "Season announcement" + "Remind me" (links to detail)
      - released/other → "Featured today" + "Start watching"
  -->
  <article class="featured-hero">
    <div class="featured-bg" aria-hidden="true">
      <img v-if="posterSrc" :src="posterSrc" alt="" loading="lazy" decoding="async" />
    </div>
    <div class="featured-content">
      <p class="featured-eyebrow">
        <span class="pulse" aria-hidden="true" />
        {{ eyebrow }}
        <template v-if="data.anime.season"><span class="sep">·</span>{{ data.anime.season }}</template>
      </p>
      <h3 class="featured-title">
        <span class="main">{{ getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp) }}</span>
        <span v-if="data.anime.name_jp" class="jp">{{ data.anime.name_jp }}</span>
      </h3>
      <div class="featured-meta">
        <span v-if="data.anime.score" class="score">
          <SpotlightIcon name="star" class="w-3.5 h-3.5" /> {{ data.anime.score.toFixed(1) }}
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
      <!-- eslint-disable-next-line vue/no-v-html -->
      <p v-if="data.anime.description" class="featured-desc" v-html="parsedDescription" />
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
import { parseDescription } from '@/utils/description-parser'
import { cardPosterUrl } from '@/composables/useImageProxy'
import type { FeaturedData } from '@/types/spotlight'
import SpotlightIcon from '../SpotlightIcon.vue'

const props = defineProps<{ data: FeaturedData }>()
const { t, locale: i18nLocale } = useI18n()

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// w=640 ≈ the native width of Shikimori originals (~700px) — the hero was
// already upscaling, so this trades nothing visually for a ~7× smaller LCP.
const posterSrc = computed(() =>
  props.data.anime.poster_url ? cardPosterUrl(props.data.anime.poster_url, 640) : '',
)

const parsedDescription = computed(() =>
  props.data.anime.description ? parseDescription(props.data.anime.description) : '',
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
.featured-bg { position: absolute; inset: 0; }
.featured-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(105%); }
.featured-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, rgba(8,8,15,.92) 0%, rgba(8,8,15,.72) 35%, rgba(8,8,15,.25) 65%, rgba(8,8,15,0) 100%),
    linear-gradient(180deg, rgba(8,8,15,0) 50%, rgba(8,8,15,.65) 100%);
}
.featured-content {
  position: absolute; inset: 0; z-index: 1;
  display: flex; flex-direction: column; justify-content: flex-end;
  /* Tightened from 40px/20px so the title (now up to 3 lines) fits the
     fixed 400px desktop / 470px mobile hero without pushing the eyebrow
     off the top. See the .featured-title note below. */
  padding: 32px 48px; gap: 14px; max-width: 720px;
}
.featured-eyebrow {
  display: inline-flex; align-items: center; gap: 10px;
  font-family: var(--f-mono); font-size: 11px; letter-spacing: .12em;
  text-transform: uppercase; color: var(--brand-cyan);
}
.featured-eyebrow .pulse { width: 6px; height: 6px; border-radius: 999px; background: var(--brand-cyan); box-shadow: 0 0 8px var(--brand-cyan); animation: featured-pulse 1.6s ease-in-out infinite; }
.featured-eyebrow .sep { opacity: .5; }
@keyframes featured-pulse { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: .5; transform: scale(.8); } }
.featured-title { font-family: var(--f-display); font-weight: 800; font-size: clamp(28px, 2.6vw, 34px); line-height: 1.1; letter-spacing: -.02em; text-wrap: balance; }
/* Long titles are clamped so they can't overflow the fixed-height hero
   (content is bottom-anchored, so an unclamped title would push the eyebrow
   off the top). The previous clamp(36px,4vw,56px) at only 2 lines cropped
   long titles brutally — e.g. "Реинкарнация безработного: …" was cut right
   after the colon, hiding the informative half. The smaller fluid size +
   3-line clamp shows the full title for typical names and truncates the
   longest ones gracefully mid-word (with ellipsis) rather than at a colon.
   Verified against the real 73-char Mushoku Tensei title at 400px height
   (incl. the jp subtitle + a 2-line description): eyebrow stays un-clipped. */
.featured-title .main { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.featured-title .jp { display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; font-family: var(--f-jp); font-weight: 500; font-size: .42em; letter-spacing: .02em; color: var(--muted-foreground); margin-top: 8px; }
.featured-meta { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; color: var(--muted-foreground); font-size: 13px; }
.featured-meta .dot { width: 3px; height: 3px; border-radius: 999px; background: currentColor; opacity: .4; }
.featured-meta .score { display: inline-flex; align-items: center; gap: 6px; color: var(--color-warning); font-weight: 600; }
.featured-meta .chip-genre { padding: 4px 10px; border-radius: 999px; border: 1px solid var(--line-strong); font-size: 12px; color: var(--ink-2); }
/* 2 lines (was 3) — reclaims vertical room for the now-3-line title clamp. */
.featured-desc { font-size: 15px; line-height: 1.6; color: var(--ink-2); max-width: 540px; text-wrap: pretty; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.featured-actions { display: flex; gap: 10px; align-items: center; }
.btn-primary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: var(--brand-cyan); color: #001218; border-radius: 12px; font-weight: 700; font-size: 14px; transition: filter .15s ease, box-shadow .15s ease; }
.btn-primary-hero:hover { filter: brightness(1.08); box-shadow: var(--accent-glow); }
.btn-secondary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: rgba(255,255,255,.06); border: 1px solid var(--line-strong); border-radius: 12px; font-weight: 600; font-size: 14px; color: var(--foreground); }
.btn-secondary-hero:hover { background: rgba(255,255,255,.1); }
@media (max-width: 640px) { .featured-content { padding: 24px; } }
</style>
