import { ref } from 'vue'
import { animeApi, episodeApi, userApi } from '@/api/client'

export interface Anime {
  id: string
  title: string
  description: string
  coverImage: string
  bannerImage?: string
  genres: string[]
  status: string
  releaseYear: number
  rating: number
  totalEpisodes: number
}

export interface Episode {
  id: string
  animeId: string
  episodeNumber: number
  title: string
  thumbnail?: string
  duration: number
}

export function useAnime() {
  const anime = ref<Anime | null>(null)
  const animeList = ref<Anime[]>([])
  const episodes = ref<Episode[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  const fetchAnime = async (id: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getById(id)
      anime.value = response.data
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch anime'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchAnimeList = async (params?: any) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getAll(params)
      animeList.value = response.data
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch anime list'
      throw err
    } finally {
      loading.value = false
    }
  }

  const searchAnime = async (query: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.search(query)
      animeList.value = response.data
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Search failed'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchTrending = async () => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getTrending()
      animeList.value = response.data
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch trending'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchPopular = async () => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getPopular()
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch popular'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchEpisodes = async (animeId: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await episodeApi.getByAnimeId(animeId)
      episodes.value = response.data
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch episodes'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchEpisode = async (episodeId: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await episodeApi.getById(episodeId)
      return response.data
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch episode'
      throw err
    } finally {
      loading.value = false
    }
  }

  const addToWatchlist = async (animeId: string) => {
    try {
      await userApi.addToWatchlist(animeId)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to add to watchlist'
      return false
    }
  }

  const removeFromWatchlist = async (animeId: string) => {
    try {
      await userApi.removeFromWatchlist(animeId)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to remove from watchlist'
      return false
    }
  }

  return {
    anime,
    animeList,
    episodes,
    loading,
    error,
    fetchAnime,
    fetchAnimeList,
    searchAnime,
    fetchTrending,
    fetchPopular,
    fetchEpisodes,
    fetchEpisode,
    addToWatchlist,
    removeFromWatchlist
  }
}
