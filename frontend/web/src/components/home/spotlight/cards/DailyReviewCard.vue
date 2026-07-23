<template>
  <SpotlightCardShell
    accent="pink"
    backdrop="none"
    justify="end"
    :kicker="t('spotlight.dailyReview.title')"
    content-class="max-w-[720px]"
  >
    <template #background>
      <div class="review-bg" aria-hidden="true">
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
      <MessageSquareQuote class="size-3" aria-hidden="true" />
    </template>

    <!--
      Verdict row (design 2026-07-22, owner-approved "линейка" variant): the
      score is the point of a review card, so it leads at display scale
      instead of hiding in a 12px pill at the end of the author row. The glyph
      is the canonical cyan ScoreDiamond (the amber Star stays reserved for
      Shikimori/MAL). score === 0 (text-only review) drops the numeral AND the
      hairline so the title spans the full column.
    -->
    <div class="flex items-center gap-4 md:gap-5 min-w-0">
      <template v-if="hasScore">
        <p class="review-score flex items-end gap-1.5 flex-none" data-testid="daily-review-score">
          <ScoreDiamond class="review-score-glyph size-3.5 text-cyan-400" />
          <span class="review-score-num font-display font-semibold text-white">{{ data.score }}</span>
          <span class="review-score-den font-mono text-muted-foreground">/10</span>
        </p>
        <span class="review-rule flex-none self-stretch" aria-hidden="true" />
      </template>

      <router-link
        :to="animePath"
        class="review-title font-display font-semibold text-white hover:text-cyan-400 transition-colors"
      >
        {{ animeTitle }}
      </router-link>
    </div>

    <blockquote
      ref="quoteEl"
      data-testid="daily-review-text"
      class="review-copy text-[15px] leading-relaxed text-white/80 whitespace-pre-line [text-wrap:pretty]"
    >{{ data.review_text }}</blockquote>

    <div class="flex items-center gap-3 min-w-0">
      <router-link
        v-if="data.author.public_id"
        :to="`/user/${data.author.public_id}`"
        class="inline-flex items-center gap-3 min-w-0 group"
        data-testid="daily-review-author-link"
      >
        <Avatar :src="data.author.avatar" :name="data.author.username" size="md" />
        <span class="text-sm font-semibold text-white truncate group-hover:text-cyan-400 transition-colors">
          {{ data.author.username }}
        </span>
      </router-link>
      <div v-else class="inline-flex items-center gap-3 min-w-0">
        <Avatar :src="data.author.avatar" :name="data.author.username" size="md" />
        <span class="text-sm font-semibold text-white truncate">{{ data.author.username }}</span>
      </div>

      <span class="flex-none text-[13px] text-muted-foreground whitespace-nowrap">{{ reviewDate }}</span>
    </div>

    <template #cta>
      <router-link
        :to="animePath"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
        data-testid="daily-review-anime-cta"
      >
        <Play class="size-4" aria-hidden="true" />
        {{ t('spotlight.dailyReview.goToAnime') }}
      </router-link>
      <button
        v-if="truncated"
        type="button"
        :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
        data-testid="daily-review-readfull-cta"
        @click="modalOpen = true"
      >
        <BookOpen class="size-4" aria-hidden="true" />
        {{ t('spotlight.dailyReview.readFull') }}
      </button>
    </template>

    <!--
      The modal lives INSIDE the shell on purpose: Modal renders through
      reka's DialogPortal (a Teleport) and contributes no layout node here,
      while a sibling of <SpotlightCardShell> would make this SFC a fragment
      and wedge the carousel's <Transition mode="out-in">.
    -->
    <Modal v-model="modalOpen" size="xl" :title="animeTitle">
      <div class="flex items-center gap-4 pb-4 mb-4 border-b border-border">
        <template v-if="hasScore">
          <p class="review-score flex items-end gap-1.5 flex-none">
            <ScoreDiamond class="review-score-glyph size-3 text-cyan-400" />
            <span class="review-score-num review-score-num--modal font-display font-semibold text-white">
              {{ data.score }}
            </span>
            <span class="review-score-den font-mono text-muted-foreground">/10</span>
          </p>
          <span class="review-rule flex-none self-stretch" aria-hidden="true" />
        </template>
        <span class="flex flex-col min-w-0">
          <span class="text-sm font-semibold text-white truncate">{{ data.author.username }}</span>
          <span class="text-[13px] text-muted-foreground">{{ reviewDate }}</span>
        </span>
      </div>

      <ReviewMarkdown :source="data.review_text" class="text-white/80" />

      <template #footer>
        <router-link
          :to="reviewPath"
          class="mr-auto self-center text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
          data-testid="daily-review-all-reviews"
          @click="modalOpen = false"
        >
          {{ t('spotlight.dailyReview.allReviews') }}
        </router-link>
        <Button variant="ghost" size="md" class="text-sm" @click="modalOpen = false">
          {{ t('common.close') }}
        </Button>
      </template>
    </Modal>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookOpen, MessageSquareQuote, Play } from 'lucide-vue-next'
import { Avatar, Button, Modal, ScoreDiamond } from '@/components/ui'
import { buttonVariants } from '@/components/ui/button-variants'
import ReviewMarkdown from '@/components/anime/ReviewMarkdown.vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { getLocalizedTitle } from '@/utils/title'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import { formatAgo } from '@/utils/time'
import type { DailyReviewData } from '@/types/spotlight'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: DailyReviewData }>()
const { t, locale } = useI18n()

const animeTitle = computed(() =>
  getLocalizedTitle(
    props.data.anime.name,
    props.data.anime.name_ru,
    props.data.anime.name_jp,
  ),
)
// Written reviews may carry no rating at all — the verdict block is gated on
// this in both the card and the modal.
const hasScore = computed(() => props.data.score > 0)
const animePath = computed(() => `/anime/${props.data.anime.id}`)
const reviewPath = computed(
  () => `${animePath.value}?ugc=reviews#section-comments`,
)
const reviewDate = computed(() => formatAgo(props.data.created_at, locale.value))
const posterSrc = computed(() =>
  props.data.anime.poster_url ? cardPosterUrl(props.data.anime.poster_url, 640) : '',
)
const bgLoaded = ref(isImageWarm(posterSrc.value))

function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(posterSrc.value)
}

const modalOpen = ref(false)

// "Read full" appears only when the 4-line clamp actually swallowed
// something. Measured, never counted: a character threshold mis-fires across
// Cyrillic, Japanese and hard line breaks alike. Mirrors ReviewMarkdown's
// measure() — same reason, same technique.
const quoteEl = ref<HTMLElement | null>(null)
const truncated = ref(false)
let observer: ResizeObserver | null = null

async function measure(): Promise<void> {
  await nextTick()
  const el = quoteEl.value
  truncated.value = !!el && el.scrollHeight > el.clientHeight + 1
}

onMounted(() => {
  void measure()
  // Re-measure on width changes (viewport resize, font swap): the clamp is
  // line-based, so a narrower card truncates text that fit a moment ago.
  // jsdom ships no ResizeObserver — specs that need one stub it themselves.
  if (typeof ResizeObserver !== 'undefined' && quoteEl.value) {
    observer = new ResizeObserver(() => void measure())
    observer.observe(quoteEl.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})

// Slides mount and unmount constantly, but the same instance can also be
// handed a new review (snapshot refresh) — re-measure instead of trusting the
// previous verdict, and never leave a stale review open in the modal.
watch(() => props.data.review_text, () => {
  modalOpen.value = false
  void measure()
})
</script>

<style scoped>
.review-bg { position: absolute; inset: 0; }
.review-bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 30%; filter: saturate(90%); }
.review-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, var(--scrim-bg-strong) 0%, var(--scrim-bg-strong) 42%, var(--scrim-bg-soft) 72%, transparent 100%),
    linear-gradient(180deg, transparent 45%, var(--scrim-bg-strong) 100%);
}
.review-title { font-size: clamp(24px, 2.2vw, 30px); line-height: 1.15; letter-spacing: -.01em; text-wrap: balance; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.review-copy { max-width: 580px; display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }

/* Verdict — display-scale numeral, tabular-nums so 8/9/10 never shift the
   title sideways as the carousel rotates through reviews. */
.review-score { line-height: 1; }
.review-score-num { font-size: clamp(40px, 5vw, 64px); line-height: .86; letter-spacing: -.035em; font-variant-numeric: tabular-nums; }
.review-score-num--modal { font-size: 40px; }
.review-score-glyph { margin-bottom: 14px; }
.review-score-den { font-size: 13px; margin-bottom: 6px; }
.review-rule { width: 1px; background: linear-gradient(180deg, transparent, var(--white-a20) 22%, var(--white-a20) 78%, transparent); }
</style>
