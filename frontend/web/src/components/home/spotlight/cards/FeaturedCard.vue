<template>
  <SpotlightCardShell
    accent="cyan"
    backdrop="none"
    justify="end"
    :kicker="eyebrow"
    content-class="max-w-[720px]"
  >
  <!--
    Workstream hero-spotlight — DS alignment 2026-06-10, batch 1 (spec:
    2026-06-10-spotlight-ds-alignment-design.md, blocks B/D, user-approved).

    Full-bleed cinematic hero card for the "featured" spotlight card type
    (type: 'featured'). Status-aware eyebrow/CTA:
      - ongoing  → "Now airing" + "Watch · ep. N" (aired+1)
      - announced → "Season announcement" + "Remind me" (links to detail)
      - released/other → "Featured today" + "Start watching"

    DS contract: SpotlightCardShell anatomy (kicker / body / CTA bottom-left),
    Button primitive classes on the CTAs, overlay Badge pills for score /
    status / genres (locked inline-vs-overlay rule), PLAIN text for year and
    episode count, amber lucide Star for the Shikimori score (DS §6),
    font-medium/semibold only.
  -->
    <template #background>
      <div class="featured-bg" aria-hidden="true">
        <!-- DS shimmer until the hero poster decodes (2026-06-11) — only
             on COLD loads: warm URLs (session registry) render instantly
             so carousel re-mounts don't replay the fade. Eager loading:
             this IS the active slide's LCP. -->
        <div v-if="posterSrc && !bgLoaded" class="absolute inset-0 skeleton-shimmer" />
        <img
          v-if="posterSrc"
          :src="posterSrc"
          alt=""
          decoding="async"
          class="transition-opacity duration-300"
          :class="bgLoaded ? 'opacity-100' : 'opacity-0'"
          @load="onBgLoad"
          @error="onBgLoad"
        />
      </div>
    </template>

    <template #kicker-lead>
      <span class="pulse" aria-hidden="true" />
    </template>
    <template #kicker-extra>
      <template v-if="data.anime.season"
        ><span class="opacity-50">·</span>{{ data.anime.season }}</template
      >
    </template>

    <h3 class="featured-title font-display font-semibold">
      <span class="main">{{ getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp) }}</span>
      <span v-if="data.anime.name_jp" class="jp font-medium">{{ data.anime.name_jp }}</span>
    </h3>

    <div class="flex items-center gap-2 flex-wrap text-[13px] text-muted-foreground">
      <Badge v-if="data.anime.score" variant="warning" size="sm" overlay>
        <template #icon>
          <Star class="size-3" fill="currentColor" aria-hidden="true" />
        </template>
        {{ data.anime.score.toFixed(1) }}
      </Badge>
      <Badge
        v-if="data.anime.status === 'ongoing' && data.anime.episodes_aired"
        variant="success"
        size="sm"
        overlay
      >
        {{ t('spotlight.featured.airedLabel', { n: data.anime.episodes_aired }) }}
      </Badge>
      <span v-if="data.anime.year">{{ data.anime.year }}</span>
      <span v-if="data.anime.episodes_count" class="opacity-40" aria-hidden="true">·</span>
      <span v-if="data.anime.episodes_count">{{
        t('spotlight.featured.episodesLabel', { n: data.anime.episodes_count })
      }}</span>
      <Badge
        v-for="g in (data.anime.genres || []).slice(0, 3)"
        :key="g.id"
        size="sm"
        overlay
      >
        {{ locale === 'ru' ? (g.russian || g.name) : (g.name || g.russian) }}
      </Badge>
    </div>

    <!-- eslint-disable-next-line vue/no-v-html -->
    <p
      v-if="data.anime.description"
      data-testid="featured-desc"
      class="text-[15px] leading-relaxed text-white/70 max-w-[540px] line-clamp-2 [text-wrap:pretty]"
      v-html="parsedDescription"
    />

    <template #cta>
      <router-link :to="watchTo" :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']">
        <Play class="w-4 h-4" fill="currentColor" aria-hidden="true" />
        {{ primaryCta }}
      </router-link>
      <router-link
        :to="`/anime/${data.anime.id}`"
        :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.featured.addCta') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play, Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import { parseDescription } from '@/utils/description-parser'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import type { FeaturedData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

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
const bgLoaded = ref(isImageWarm(posterSrc.value))
function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(posterSrc.value)
}

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
/* Bespoke keeps (DS alignment 2026-06-10): the sharp-poster background +
   readability scrim and the pulse dot are full-bleed hero specifics no
   primitive models; everything else moved to SpotlightCardShell, Badge,
   and Button-variant classes. */
.featured-bg { position: absolute; inset: 0; }
.featured-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(105%); }
.featured-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, rgba(8,8,15,.92) 0%, rgba(8,8,15,.72) 35%, rgba(8,8,15,.25) 65%, rgba(8,8,15,0) 100%),
    linear-gradient(180deg, rgba(8,8,15,0) 50%, rgba(8,8,15,.65) 100%);
}
.pulse { width: 6px; height: 6px; border-radius: 999px; background: var(--brand-cyan); box-shadow: 0 0 8px var(--brand-cyan); animation: featured-pulse 1.6s ease-in-out infinite; }
@keyframes featured-pulse { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: .5; transform: scale(.8); } }
/* Long titles are clamped so they can't overflow the fixed-height hero
   (content is bottom-anchored, so an unclamped title would push the eyebrow
   off the top). Smaller fluid size + 3-line clamp shows the full title for
   typical names and truncates the longest ones gracefully mid-word. */
.featured-title { font-size: clamp(28px, 2.6vw, 34px); line-height: 1.1; letter-spacing: -.02em; text-wrap: balance; }
.featured-title .main { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.featured-title .jp { display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; font-family: var(--font-jp); font-size: .42em; letter-spacing: .02em; color: var(--muted-foreground); margin-top: 8px; }
</style>
