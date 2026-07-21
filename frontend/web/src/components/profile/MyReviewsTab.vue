<!--
  MyReviewsTab — own-profile Reviews tab (feedback 2026-07-21): the viewer's
  written reviews (review_text != '') via GET /users/reviews (JWT-claims
  endpoint — own profile only). Lazy: parent (Tabs.vue) only renders the
  active named slot, so this only mounts/fetches on first tab open.
-->
<template>
  <div v-if="loading" class="flex justify-center py-12">
    <Spinner />
  </div>

  <EmptyState v-else-if="reviews.length === 0" :description="$t('profile.reviews.empty')" />

  <div v-else class="space-y-4">
    <div v-for="r in reviews" :key="r.id" class="glass-card p-4">
      <div class="flex gap-4">
        <router-link :to="`/anime/${r.anime_id}`" class="w-16 shrink-0" :aria-label="$t('profile.reviews.openAnime')">
          <PosterImage :src="r.anime?.poster_url || ''" :alt="title(r)" ratio="2/3" rounded="lg" :proxy-width="128" />
        </router-link>
        <div class="min-w-0 flex-1">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <router-link
                :to="`/anime/${r.anime_id}`"
                class="block truncate font-medium text-white hover:text-cyan-400 transition-colors"
              >
                {{ title(r) }}
              </router-link>
              <p class="text-sm text-white/60">{{ formatDate(r.created_at) }}</p>
            </div>
            <div v-if="r.score > 0" class="flex items-center gap-1 text-cyan-400">
              <ScoreDiamond class="size-5" />
              <span class="font-semibold">{{ r.score }}</span>
            </div>
          </div>
          <ReviewMarkdown :source="r.review_text" collapsible class="mt-2 text-white/70" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { userApi } from '@/api/client'
import { EmptyState, ScoreDiamond, Spinner } from '@/components/ui'
import PosterImage from '@/components/anime/PosterImage.vue'
import ReviewMarkdown from '@/components/anime/ReviewMarkdown.vue'
import { getLocalizedTitle } from '@/utils/title'

interface MyReview {
  id: string
  anime_id: string
  score: number
  review_text: string
  created_at: string
  anime?: { name?: string; name_ru?: string; name_jp?: string; poster_url?: string }
}

const { locale } = useI18n()

const reviews = ref<MyReview[]>([])
const loading = ref(true)

function title(r: MyReview): string {
  return getLocalizedTitle(r.anime?.name, r.anime?.name_ru, r.anime?.name_jp) || 'Anime'
}

// No shared standalone `formatDate` export exists (the closest, Anime.vue's,
// is a closure inside useAnimeDisplay() bound to a single anime ref, not
// reusable for a per-row list) — mirrors the same locale-aware pattern
// (composables/animePage/useAnimeDisplay.ts, components/LastUpdates.vue).
function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return date.toLocaleDateString(loc, { day: 'numeric', month: 'long', year: 'numeric' })
}

onMounted(async () => {
  try {
    const res = await userApi.getMyReviews()
    const all: MyReview[] = res.data?.data || res.data || []
    reviews.value = all.filter((r) => (r.review_text || '').trim() !== '')
  } catch (err) {
    console.error('Failed to fetch my reviews:', err)
  } finally {
    loading.value = false
  }
})
</script>
