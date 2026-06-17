<template>
  <SpotlightCardShell
    accent="violet"
    icon="shuffle"
    :kicker="t('spotlight.randomTail.title')"
    backdrop="poster-blur"
    :poster-url="anime.poster_url"
  >
  <!--
    Workstream hero-spotlight — v4 B-1 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Deck-of-cards discovery
    surface: the poster sits on top of two rotated ghost "cards"; on mount
    (and on every reroll) the three deal IN and settle into the resting
    stack — replacing the old 5-card gradient overlay that covered the
    content for ~1s and vanished (read as a glitch). Density pass: year,
    status pill, description (clamp-3) joined the meta. The «Ещё разок»
    ghost CTA fetches a fresh pick from GET /home/spotlight/reroll.
  -->
    <!-- Violet-tinted secondary overlay differentiates RandomTail from
         the cyan FeaturedCard backdrop without re-fetching the poster. -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-brand-violet/30 via-transparent to-transparent"
      />
    </template>

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-8 md:items-center">
      <!-- Deck: poster + two ghost cards. The .deal class drives the
           deal-in animation; re-toggled on reroll. -->
      <router-link
        :to="`/anime/${anime.id}`"
        class="deck relative flex-shrink-0 self-center w-24 md:w-36 group"
        :class="{ deal: dealing && !reducedMotion, shuffling: shuffling }"
        data-testid="deck"
      >
        <span class="deck-ghost deck-g1" aria-hidden="true" />
        <span class="deck-ghost deck-g2" aria-hidden="true" />
        <SpotlightPoster
          :poster-url="anime.poster_url"
          :alt="title"
          width-class="deck-poster relative w-24 md:w-36"
          glow="violet"
          :proxy-width="256"
        />
      </router-link>

      <div class="flex-1 min-w-0 max-w-[600px]">
        <p class="text-sm text-muted-foreground font-medium" data-testid="random-tail-tagline">
          {{ t('spotlight.randomTail.dealtLabel') }}
        </p>
        <h3 class="mt-1 text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>

        <div class="mt-2.5 flex flex-wrap items-center gap-2">
          <Badge v-if="anime.score" variant="warning" size="sm" overlay>
            <template #icon>
              <Star class="size-3" fill="currentColor" aria-hidden="true" />
            </template>
            {{ anime.score?.toFixed(1) }}
          </Badge>
          <Badge v-if="statusLabel" size="sm" overlay :class="statusClass">
            {{ statusLabel }}
          </Badge>
          <span class="text-sm text-muted-foreground font-medium">
            <template v-if="anime.year">{{ anime.year }}</template>
            <template v-if="anime.year && anime.episodes_count"> · </template>
            <template v-if="anime.episodes_count">{{
              t('spotlight.featured.episodesLabel', { n: anime.episodes_count })
            }}</template>
          </span>
          <Badge
            v-for="g in (anime.genres || []).slice(0, 3)"
            :key="g.id"
            size="sm"
            overlay
          >
            {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
          </Badge>
        </div>

        <p
          v-if="anime.description"
          class="mt-2.5 text-[13px] leading-relaxed text-white/70 line-clamp-3"
          data-testid="random-tail-desc"
        >
          {{ plainDescription }}
        </p>
      </div>
    </div>

    <template #cta>
      <router-link
        :to="`/anime/${anime.id}`"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.randomTail.discoverCta') }}
      </router-link>
      <button
        type="button"
        :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
        :disabled="rerolling"
        data-testid="reroll-btn"
        @click="reroll"
      >
        <Shuffle class="w-4 h-4" aria-hidden="true" />
        {{ t('spotlight.randomTail.rerollCta') }}
      </button>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { Shuffle, Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import { parseDescription } from '@/utils/description-parser'
import { preloadImage } from '@/utils/preload-image'
import { cardPosterUrl } from '@/composables/useImageProxy'
import type { RandomTailData, SpotlightCard } from '@/types/spotlight'
import type { SpotlightAnime } from '@/types/spotlight'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import { apiClient } from '@/api/client'

const props = defineProps<{ data: RandomTailData }>()

const { t, locale: i18nLocale } = useI18n()

// Reroll swaps the shown anime locally without touching the parent's
// card list (the daily pick stays cached server-side for everyone).
const rerolled = ref<SpotlightAnime | null>(null)
const anime = computed<SpotlightAnime>(() => rerolled.value ?? props.data.anime)

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const title = computed(() =>
  getLocalizedTitle(anime.value.name, anime.value.name_ru, anime.value.name_jp),
)

// Description arrives with BB-code/HTML — parse, then strip tags: this
// card renders plain clamped text (no v-html surface needed here).
const plainDescription = computed(() => {
  if (!anime.value.description) return ''
  const el = document.createElement('div')
  el.innerHTML = parseDescription(anime.value.description)
  return el.textContent || ''
})

const statusLabel = computed<string>(() => {
  switch (anime.value.status) {
    case 'ongoing': return t('spotlight.randomTail.statusOngoing')
    case 'released': return t('spotlight.randomTail.statusReleased')
    case 'announced': return t('spotlight.randomTail.statusAnnounced')
    default: return ''
  }
})
const statusClass = computed(() =>
  anime.value.status === 'ongoing' ? 'text-success' : '',
)

// ── Deal-in animation ──────────────────────────────────────────────────
// Plays on mount and after every reroll. reduced-motion users see the
// resting stack immediately (the .deal class is never applied).
const reducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
const dealing = ref(false)
let dealTimer: ReturnType<typeof setTimeout> | null = null

function playDeal(): void {
  if (reducedMotion.value) return
  dealing.value = false
  requestAnimationFrame(() => {
    dealing.value = true
    if (dealTimer !== null) clearTimeout(dealTimer)
    dealTimer = setTimeout(() => {
      dealing.value = false
      dealTimer = null
    }, 900)
  })
}

onMounted(playDeal)
onBeforeUnmount(() => {
  if (dealTimer !== null) clearTimeout(dealTimer)
})

// ── «Ещё разок» reroll ────────────────────────────────────────────────
// The deck plays a looping shuffle while the fetch + poster preload run;
// the anime only swaps in once BOTH image buckets (256 deck poster + 128
// blurred backdrop) are in the browser cache — so the deal-in lands on a
// ready image and the backdrop crossfades instead of flashing (user
// feedback 2026-06-11: «криво подгружает картинку — пролаг»).
const rerolling = ref(false)
const shuffling = ref(false)
// Floor so a warm-cache reroll still reads as a shuffle, not a blink.
const MIN_SHUFFLE_MS = 650

async function reroll(): Promise<void> {
  if (rerolling.value) return
  rerolling.value = true
  shuffling.value = !reducedMotion.value
  const startedAt = Date.now()
  try {
    const res = await apiClient.get<SpotlightCard>(
      `/home/spotlight/reroll?exclude=${encodeURIComponent(anime.value.id)}`,
    )
    const card = res.data
    if (card && card.type === 'random_tail' && card.data?.anime) {
      const next = card.data.anime
      await Promise.all([
        preloadImage(cardPosterUrl(next.poster_url, 256)),
        preloadImage(cardPosterUrl(next.poster_url, 128)),
      ])
      const minLeft = MIN_SHUFFLE_MS - (Date.now() - startedAt)
      if (shuffling.value && minLeft > 0) {
        await new Promise((r) => setTimeout(r, minLeft))
      }
      shuffling.value = false
      rerolled.value = next
      playDeal()
    }
  } catch (e) {
    console.warn('[spotlight] reroll failed', e)
  } finally {
    shuffling.value = false
    rerolling.value = false
  }
}
</script>

<style scoped>
/* Deck resting stack: two ghost cards rotated behind the poster. */
.deck-ghost {
  position: absolute;
  inset: 0;
  border-radius: 12px;
  pointer-events: none;
}
.deck-g1 {
  background: rgba(167, 139, 250, 0.14);
  border: 1px solid rgba(167, 139, 250, 0.3);
  transform: rotate(-7deg) translate(-9px, 5px);
}
.deck-g2 {
  background: var(--white-a8);
  border: 1px solid var(--line-strong);
  transform: rotate(4deg) translate(7px, 2px);
}
/* Deal-in: each layer flies from a center pile and settles into the
   resting transform (no overlay layer → nothing covers the content,
   nothing can desync — v4 B-1 fix for the old glitchy 5-card overlay). */
.deck.deal .deck-g1 {
  animation: deck-deal-g1 0.55s cubic-bezier(0.3, 0.9, 0.3, 1.15) both;
}
.deck.deal .deck-g2 {
  animation: deck-deal-g2 0.55s cubic-bezier(0.3, 0.9, 0.3, 1.15) 0.08s both;
}
.deck.deal :deep(.deck-poster) {
  animation: deck-deal-poster 0.55s cubic-bezier(0.3, 0.9, 0.3, 1.15) 0.16s both;
}
@keyframes deck-deal-g1 {
  from { opacity: 0; transform: rotate(-26deg) translate(-60px, -80px); }
  to { opacity: 1; transform: rotate(-7deg) translate(-9px, 5px); }
}
@keyframes deck-deal-g2 {
  from { opacity: 0; transform: rotate(18deg) translate(60px, -90px); }
  to { opacity: 1; transform: rotate(4deg) translate(7px, 2px); }
}
@keyframes deck-deal-poster {
  from { opacity: 0; transform: rotate(-10deg) translateY(-70px) scale(0.92); }
  to { opacity: 1; transform: none; }
}

/* Shuffle loop — plays while the reroll fetch + poster preload are in
   flight: ghosts riffle out to the sides and back, the top card bobs.
   Loops until the swap, so slow networks just see a longer shuffle. */
.deck.shuffling .deck-g1 {
  animation: deck-shuffle-g1 0.65s ease-in-out infinite;
}
.deck.shuffling .deck-g2 {
  animation: deck-shuffle-g2 0.65s ease-in-out 0.08s infinite;
}
.deck.shuffling :deep(.deck-poster) {
  animation: deck-shuffle-poster 0.65s ease-in-out infinite;
}
@keyframes deck-shuffle-g1 {
  0%, 100% { transform: rotate(-7deg) translate(-9px, 5px); }
  50% { transform: rotate(-16deg) translate(-30px, -8px); }
}
@keyframes deck-shuffle-g2 {
  0%, 100% { transform: rotate(4deg) translate(7px, 2px); }
  50% { transform: rotate(13deg) translate(28px, -6px); }
}
@keyframes deck-shuffle-poster {
  0%, 100% { transform: none; }
  50% { transform: rotate(2.5deg) translateY(-9px) scale(0.97); }
}
</style>
