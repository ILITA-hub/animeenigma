import { defineStore } from 'pinia'
import { ref } from 'vue'
import { animeApi, reviewApi } from '@/api/client'

export interface HomeAnime {
  id: string
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  season?: string
  next_episode_at?: string
  updated_at?: string
}

export const useHomeStore = defineStore('home', () => {
  const announcedAnime = ref<HomeAnime[]>([])
  const ongoingAnime = ref<HomeAnime[]>([])
  const topAnime = ref<HomeAnime[]>([])
  const siteRatings = ref<Record<string, { average_score: number; total_reviews: number }>>({})
  const ongoingUpdatedAt = ref<string | null>(null)
  const lastFetched = ref<number>(0)
  const loading = ref(false)
  const loadingAnnounced = ref(true)
  const loadingOngoing = ref(true)
  const loadingTop = ref(true)

  const CACHE_TTL = 3 * 60 * 1000 // 3 minutes

  const isFresh = () => Date.now() - lastFetched.value < CACHE_TTL

  const fetchAll = async (force = false) => {
    if (!force && isFresh() && ongoingAnime.value.length > 0) return
    if (loading.value) return

    loading.value = true
    loadingAnnounced.value = true
    loadingOngoing.value = true
    loadingTop.value = true

    try {
      await Promise.allSettled([
        animeApi.getAnnounced(15).then(response => {
          announcedAnime.value = response.data?.data || []
        }).catch(err => {
          console.error('Failed to load announced anime:', err)
        }).finally(() => {
          loadingAnnounced.value = false
        }),

        animeApi.getOngoing().then(response => {
          const animes = (response.data?.data || []).slice(0, 20)
          ongoingAnime.value = animes
          if (animes.length > 0) {
            const maxUpdated = animes.reduce((max: string | null, anime: HomeAnime) => {
              if (!anime.updated_at) return max
              if (!max) return anime.updated_at
              return new Date(anime.updated_at) > new Date(max) ? anime.updated_at : max
            }, null as string | null)
            ongoingUpdatedAt.value = maxUpdated
          }
        }).catch(err => {
          console.error('Failed to load ongoing anime:', err)
        }).finally(() => {
          loadingOngoing.value = false
        }),

        animeApi.getTop(15).then(response => {
          topAnime.value = response.data?.data || []
        }).catch(err => {
          console.error('Failed to load top anime:', err)
        }).finally(() => {
          loadingTop.value = false
        }),
      ])

      // Fetch site ratings after all columns are loaded (needs their IDs)
      const allIds = [
        ...ongoingAnime.value.map(a => a.id),
        ...topAnime.value.map(a => a.id),
      ]
      if (allIds.length > 0) {
        try {
          const uniqueIds = [...new Set(allIds)]
          const response = await reviewApi.getBatchRatings(uniqueIds)
          siteRatings.value = response.data?.data?.ratings || response.data?.ratings || {}
        } catch (err) {
          console.error('Failed to load site ratings:', err)
        }
      }

      lastFetched.value = Date.now()
    } finally {
      loading.value = false
    }
  }

  return {
    announcedAnime,
    ongoingAnime,
    topAnime,
    siteRatings,
    ongoingUpdatedAt,
    loading,
    loadingAnnounced,
    loadingOngoing,
    loadingTop,
    fetchAll,
  }
})
