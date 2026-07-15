import { ref, reactive, type Ref } from 'vue'
import type { Anime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore, type ViewerContextData } from '@/stores/viewerContext'
import { animeApi, reviewApi } from '@/api/client'
import type { Review, AnimeRating } from './types'

/**
 * Social / UGC-adjacent state for the anime page (extracted from Anime.vue):
 * the reviews feed, the viewer's own review + form, the site rating, and the
 * soft social-proof watchers count — plus the viewer-context aggregate
 * application and the legacy per-endpoint fallback fetches.
 */
export function useAnimeSocial(
  anime: Ref<Anime | null>,
  currentListStatus: Ref<string | null>,
  currentRewatchCount: Ref<number>,
) {
  const authStore = useAuthStore()
  const viewerCtxStore = useViewerContextStore()

  const reviews = ref<Review[]>([])
  const myReview = ref<Review | null>(null)
  const siteRating = ref<AnimeRating | null>(null)
  // Phase 14 / UX-28 — soft social-proof watchers count. Public endpoint,
  // no auth. Render badge only when count >= 5 (avoids embarrassingly empty
  // signals on niche or fresh titles).
  const watchersCount = ref(0)
  const reviewSubmitting = ref(false)
  const reviewForm = reactive({
    score: 0,
    text: '',
  })

  // Phase 14 / UX-28 — soft social-proof fetch. Public endpoint, no auth.
  // Errors are swallowed: missing badge is preferable to a noisy console for
  // a non-critical UI signal. Endpoint returns { count: number } (or wrapped
  // in { data: ... } depending on httputil response shape).
  const fetchWatchersCount = async () => {
    if (!anime.value) return
    try {
      const response = await animeApi.getWatchersCount(anime.value.id)
      const payload = (response.data as { data?: { count?: number }; count?: number } | undefined) ?? {}
      const raw = payload.data ? payload.data.count : payload.count
      watchersCount.value = typeof raw === 'number' && raw >= 0 ? raw : 0
    } catch {
      watchersCount.value = 0
    }
  }

  // applyViewerContext — populate the page's social/user state from the
  // aggregate viewer-context payload (page-fetch optimization 2026-06-11).
  // Replaces the separate rating / watchers-count / my-review / watchlist-status
  // fetches on page load and after review/list mutations.
  const applyViewerContext = (ctx: ViewerContextData) => {
    watchersCount.value = typeof ctx.watchers_count === 'number' && ctx.watchers_count >= 0
      ? ctx.watchers_count
      : 0
    siteRating.value = (ctx.rating as AnimeRating | null) ?? null
    if (authStore.isAuthenticated) {
      const review = ctx.my_review as Review | null
      if (review && review.id) {
        myReview.value = review
        reviewForm.score = review.score
        reviewForm.text = review.review_text || ''
      } else {
        myReview.value = null
      }
      currentListStatus.value = ctx.watchlist_entry?.status ?? null
      currentRewatchCount.value = ctx.watchlist_entry?.rewatch_count ?? 0
    }
  }

  // fetchReviewsList — the reviews FEED only. Rating / my-review come from the
  // viewer-context aggregate; this stays a separate (heavier) request, fetched
  // lazily when the UGC section scrolls near the viewport.
  const reviewsFetched = ref(false)
  const fetchReviewsList = async () => {
    if (!anime.value) return
    reviewsFetched.value = true
    try {
      const reviewsResponse = await reviewApi.getAnimeReviews(anime.value.id)
      reviews.value = reviewsResponse.data?.data || reviewsResponse.data || []
    } catch (err) {
      console.error('Failed to fetch reviews:', err)
    }
  }

  // Legacy full path — used only when the viewer-context aggregate failed
  // (older backend / transient error): falls back to the historical
  // per-endpoint fetches.
  const fetchReviews = async () => {
    if (!anime.value) return

    reviewsFetched.value = true
    try {
      // Fetch reviews
      const reviewsResponse = await reviewApi.getAnimeReviews(anime.value.id)
      reviews.value = reviewsResponse.data?.data || reviewsResponse.data || []

      // Fetch rating
      const ratingResponse = await reviewApi.getAnimeRating(anime.value.id)
      siteRating.value = ratingResponse.data?.data || ratingResponse.data

      // Fetch user's review if authenticated
      if (authStore.isAuthenticated) {
        try {
          const myReviewResponse = await reviewApi.getMyReview(anime.value.id)
          const review = myReviewResponse.data?.data || myReviewResponse.data
          if (review && review.id) {
            myReview.value = review
            reviewForm.score = review.score
            reviewForm.text = review.review_text || ''
          }
        } catch {
          // No review from this user
        }
      }
    } catch (err) {
      console.error('Failed to fetch reviews:', err)
    }
  }

  // refreshSocial — post-mutation refresh: one forced viewer-context reload
  // (rating + my-review + watchlist status + watchers) plus the reviews feed.
  const refreshSocial = async () => {
    if (!anime.value) return
    const [ctx] = await Promise.all([
      viewerCtxStore.load(anime.value.id, true),
      fetchReviewsList(),
    ])
    if (ctx) applyViewerContext(ctx)
  }

  const submitReview = async () => {
    if (!anime.value || reviewForm.score === 0) return

    reviewSubmitting.value = true
    try {
      await reviewApi.createReview(
        anime.value.id,
        reviewForm.score,
        reviewForm.text
      )
      await refreshSocial()
    } catch (err) {
      console.error('Failed to submit review:', err)
    } finally {
      reviewSubmitting.value = false
    }
  }

  const deleteMyReview = async () => {
    if (!anime.value) return

    try {
      await reviewApi.deleteReview(anime.value.id)
      myReview.value = null
      reviewForm.score = 0
      reviewForm.text = ''
      await refreshSocial()
    } catch (err) {
      console.error('Failed to delete review:', err)
    }
  }

  /** Per-anime reset (route param change) — mirrors the loadAnimeData reset block. */
  function reset() {
    reviews.value = []
    myReview.value = null
    siteRating.value = null
    watchersCount.value = 0
    reviewsFetched.value = false
    reviewForm.score = 0
    reviewForm.text = ''
  }

  return {
    reviews,
    myReview,
    siteRating,
    watchersCount,
    reviewSubmitting,
    reviewForm,
    reviewsFetched,
    fetchWatchersCount,
    applyViewerContext,
    fetchReviewsList,
    fetchReviews,
    refreshSocial,
    submitReview,
    deleteMyReview,
    reset,
  }
}
