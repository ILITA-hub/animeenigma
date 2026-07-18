<template>
  <SpotlightCardShell
    accent="pink"
    backdrop="none"
    justify="end"
    :kicker="t('spotlight.dailyFanfic.title')"
    content-class="max-w-[720px]"
  >
    <!--
      Workstream daily-fanfic-spotlight — Task 15. Mirrors CuratedCard/
      FeaturedCard's cinematic-hero anatomy (poster-blur #background with
      warm-registry shimmer+fade, SpotlightCardShell body/CTA slots,
      Button-variant CTAs, overlay Badge pills) with fanfic-specific body:
      title + anime subline, credited/anon author line, an AI-generated
      badge, a content-rating + part-count badge row, and an excerpt that
      is REPLACED by an auth-gated line for explicit picks (never render
      explicit excerpt text, credited or not).
    -->
    <template #background>
      <div class="fanfic-bg" aria-hidden="true">
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
      <BookOpen class="size-3" aria-hidden="true" />
    </template>
    <template #kicker-extra>
      <Badge v-if="data.ai_generated" variant="secondary" size="sm">
        {{ t('spotlight.dailyFanfic.aiBadge') }}
      </Badge>
    </template>

    <h3 class="fanfic-title font-display font-semibold">{{ data.fanfic_title }}</h3>

    <p class="text-[13px] text-muted-foreground font-medium">
      {{ data.anime_title }}
      <span v-if="data.anime_japanese" class="anime-jp font-medium">{{ data.anime_japanese }}</span>
    </p>

    <div class="flex items-center gap-2 flex-wrap text-[13px] text-muted-foreground">
      <Badge :variant="ratingVariant" size="sm" overlay>
        {{ t(`fanfic.rating.${data.rating}`) }}
      </Badge>
      <Badge v-if="data.part_count > 1" size="sm" overlay>
        {{ t('spotlight.dailyFanfic.partsLabel', { n: data.part_count }) }}
      </Badge>
    </div>

    <p
      v-if="!data.explicit && data.excerpt"
      data-testid="fanfic-excerpt"
      class="text-[15px] leading-relaxed text-white/70 max-w-[540px] line-clamp-3 [text-wrap:pretty]"
    >{{ data.excerpt }}</p>
    <p
      v-else-if="data.explicit"
      data-testid="fanfic-explicit-gate"
      class="flex items-center gap-2 text-[15px] leading-relaxed text-white/70 max-w-[540px]"
    >
      <Badge variant="destructive" size="sm" overlay>18+</Badge>
      <span>{{ explicitGateText }}</span>
    </p>

    <p class="text-[13px] text-muted-foreground font-medium">
      <template v-if="data.credited">{{ data.author_username }}</template>
      <template v-else>{{ t('spotlight.dailyFanfic.anonAuthor') }}</template>
    </p>

    <template #cta>
      <router-link
        :to="DAILY_FANFIC_LINK"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        <BookOpen class="w-4 h-4" aria-hidden="true" />
        {{ t('spotlight.dailyFanfic.read') }}
      </router-link>
      <router-link to="/fanfics" :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']">
        {{ t('spotlight.dailyFanfic.writeOwn') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookOpen } from 'lucide-vue-next'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import { useAuthStore } from '@/stores/auth'
import { DAILY_FANFIC_LINK } from '@/utils/fanficGate'
import type { DailyFanficData } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: DailyFanficData }>()
const { t } = useI18n()
const auth = useAuthStore()

const posterSrc = computed(() =>
  props.data.anime_poster ? cardPosterUrl(props.data.anime_poster, 640) : '',
)
const bgLoaded = ref(isImageWarm(posterSrc.value))
function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(posterSrc.value)
}

// Mirrors LibraryGrid.vue's ratingVariant() — same 'teen'/'mature'/'explicit'
// FanficRating enum, so the pill color language stays consistent across the
// fanfic UI (destructive red / warning amber / neutral default).
const ratingVariant = computed<'default' | 'warning' | 'destructive'>(() => {
  if (props.data.rating === 'explicit') return 'destructive'
  if (props.data.rating === 'mature') return 'warning'
  return 'default'
})

// Explicit picks never render the excerpt (credited or not) — gate behind
// auth instead: authed users get a "read the full part" nudge, anonymous
// visitors get a login nudge.
const explicitGateText = computed(() =>
  auth.isAuthenticated
    ? t('spotlight.dailyFanfic.explicitReader')
    : t('spotlight.dailyFanfic.explicitLogin'),
)
</script>

<style scoped>
.fanfic-bg { position: absolute; inset: 0; }
.fanfic-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(105%); }
.fanfic-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, var(--scrim-bg-strong) 0%, var(--scrim-bg-strong) 35%, var(--scrim-bg-soft) 65%, transparent 100%),
    linear-gradient(180deg, transparent 50%, var(--scrim-bg-strong) 100%);
}
.fanfic-title { font-size: clamp(24px, 2.2vw, 30px); line-height: 1.15; letter-spacing: -.01em; text-wrap: balance; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.anime-jp { font-family: var(--font-jp); margin-left: 6px; }
</style>
