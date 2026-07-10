<template>
  <SpotlightCardShell
    accent="cyan"
    backdrop="none"
    justify="end"
    :kicker="t('spotlight.curated.title')"
    content-class="max-w-[720px]"
  >
    <!--
      Curated «Куратор рекомендует» hero card (type: 'curated'). One
      env-pinned, airing-gated anime. Mirrors FeaturedCard's cinematic hero
      but with a fixed curated kicker (star lead) and a single ongoing CTA —
      the resolver only ever surfaces an `ongoing` anime, so no status switch.
      DS: SpotlightCardShell anatomy, Button-variant CTAs, overlay Badges,
      font-medium/semibold only.
    -->
    <template #background>
      <div class="curated-bg" aria-hidden="true">
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
      <Star class="size-3" fill="currentColor" aria-hidden="true" />
    </template>
    <template #kicker-extra>
      <template v-if="data.anime.season"
        ><span class="opacity-50">·</span>{{ data.anime.season }}</template
      >
    </template>

    <h3 class="curated-title font-display font-semibold">
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
      <Badge v-if="data.anime.episodes_aired" variant="success" size="sm" overlay>
        {{ t('spotlight.curated.airedLabel', { n: data.anime.episodes_aired }) }}
      </Badge>
      <span v-if="data.anime.year">{{ data.anime.year }}</span>
      <span v-if="data.anime.episodes_count" class="opacity-40" aria-hidden="true">·</span>
      <span v-if="data.anime.episodes_count">{{
        t('spotlight.curated.episodesLabel', { n: data.anime.episodes_count })
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
      data-testid="curated-desc"
      class="text-[15px] leading-relaxed text-white/70 max-w-[540px] line-clamp-2 [text-wrap:pretty]"
      v-html="parsedDescription"
    />

    <template #cta>
      <router-link :to="watchTo" :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']">
        <Play class="w-4 h-4" fill="currentColor" aria-hidden="true" />
        {{ t('spotlight.curated.watchEpisode', { n: (data.anime.episodes_aired || 0) + 1 }) }}
      </router-link>
      <router-link
        :to="`/anime/${data.anime.id}`"
        :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.curated.addCta') }}
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
import type { CuratedData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: CuratedData }>()
const { t, locale: i18nLocale } = useI18n()

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

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

const watchTo = computed(() => `/anime/${props.data.anime.id}/watch`)
</script>

<style scoped>
.curated-bg { position: absolute; inset: 0; }
.curated-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(105%); }
.curated-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, var(--scrim-bg-strong) 0%, var(--scrim-bg-strong) 35%, var(--scrim-bg-soft) 65%, transparent 100%),
    linear-gradient(180deg, transparent 50%, var(--scrim-bg-strong) 100%);
}
.curated-title { font-size: clamp(28px, 2.6vw, 34px); line-height: 1.1; letter-spacing: -.02em; text-wrap: balance; }
.curated-title .main { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.curated-title .jp { display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; font-family: var(--font-jp); font-size: .42em; letter-spacing: .02em; color: var(--muted-foreground); margin-top: 8px; }
</style>
