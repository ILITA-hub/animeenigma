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
  episodesAired: number
  nextEpisodeAt?: string
  shikimoriId?: string
}

export interface Episode {
  id: string
  animeId: string
  episodeNumber: number
  title: string
  thumbnail?: string
  duration: number
}

// Transform API response to frontend Anime interface
function transformAnime(apiAnime: any): Anime {
  return {
    id: apiAnime.id,
    title: apiAnime.name_ru || apiAnime.name || apiAnime.name_jp || '',
    description: apiAnime.description || '',
    coverImage: apiAnime.poster_url || '',
    bannerImage: apiAnime.banner_url,
    genres: apiAnime.genres?.map((g: any) => g.name_ru || g.name) || [],
    status: apiAnime.status || '',
    releaseYear: apiAnime.year || 0,
    rating: apiAnime.score || 0,
    totalEpisodes: apiAnime.episodes_count || 0,
    episodesAired: apiAnime.episodes_aired || 0,
    nextEpisodeAt: apiAnime.next_episode_at || null,
    shikimoriId: apiAnime.shikimori_id || null
  }
}

function transformAnimeList(apiList: any): Anime[] {
  if (!apiList) return []
  const list = apiList.data || apiList
  if (!Array.isArray(list)) return []
  return list.map(transformAnime)
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
      const data = response.data?.data || response.data
      anime.value = transformAnime(data)
      return anime.value
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
      animeList.value = transformAnimeList(response.data)
      return animeList.value
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
      animeList.value = transformAnimeList(response.data)
      return animeList.value
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
      animeList.value = transformAnimeList(response.data)
      return animeList.value
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
      return transformAnimeList(response.data)
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch popular'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchSchedule = async () => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getSchedule()
      const data = response.data?.data || response.data
      return Array.isArray(data) ? data : []
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch schedule'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchOngoing = async () => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getOngoing(50)
      return transformAnimeList(response.data)
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Failed to fetch ongoing'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchEpisodes = async (animeId: string) => {
    // Don't set global loading for episodes - use separate loadingEpisodes in component
    try {
      const response = await episodeApi.getByAnimeId(animeId)
      // Extract data array from response wrapper
      const data = response.data?.data || response.data
      episodes.value = Array.isArray(data) ? data : []
      return episodes.value
    } catch (err: any) {
      // Don't set global error for episodes failure
      console.error('Failed to fetch episodes:', err)
      episodes.value = []
      return []
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

  const addToWatchlist = async (animeId: string, animeTitle?: string, animeCover?: string, totalEpisodes?: number) => {
    try {
      // Use current anime data if available and not provided
      const title = animeTitle ?? anime.value?.title
      const cover = animeCover ?? anime.value?.coverImage
      const episodes = totalEpisodes ?? anime.value?.totalEpisodes ?? anime.value?.episodesAired
      await userApi.addToWatchlist(animeId, 'plan_to_watch', title, cover, episodes)
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
    fetchSchedule,
    fetchOngoing,
    fetchEpisodes,
    fetchEpisode,
    addToWatchlist,
    removeFromWatchlist
  }
}
