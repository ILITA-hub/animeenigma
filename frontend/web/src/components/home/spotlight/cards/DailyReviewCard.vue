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

    <router-link
      :to="animePath"
      class="review-title font-display font-semibold text-white hover:text-cyan-400 transition-colors"
    >
      {{ animeTitle }}
    </router-link>

    <blockquote
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

      <Badge v-if="data.score > 0" size="sm" overlay class="ml-auto">
        <template #icon>
          <ScoreDiamond class="size-3 text-cyan-400" />
        </template>
        {{ data.score }}/10
      </Badge>
    </div>

    <template #cta>
      <router-link
        :to="reviewPath"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        <MessageSquareQuote class="size-4" aria-hidden="true" />
        {{ t('spotlight.dailyReview.openCta') }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessageSquareQuote } from 'lucide-vue-next'
import { Avatar, Badge, ScoreDiamond } from '@/components/ui'
import { buttonVariants } from '@/components/ui/button-variants'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { getLocalizedTitle } from '@/utils/title'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import type { DailyReviewData } from '@/types/spotlight'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: DailyReviewData }>()
const { t } = useI18n()

const animeTitle = computed(() =>
  getLocalizedTitle(
    props.data.anime.name,
    props.data.anime.name_ru,
    props.data.anime.name_jp,
  ),
)
const animePath = computed(() => `/anime/${props.data.anime.id}`)
const reviewPath = computed(
  () => `${animePath.value}?ugc=reviews#section-comments`,
)
const posterSrc = computed(() =>
  props.data.anime.poster_url ? cardPosterUrl(props.data.anime.poster_url, 640) : '',
)
const bgLoaded = ref(isImageWarm(posterSrc.value))

function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(posterSrc.value)
}
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
</style>
