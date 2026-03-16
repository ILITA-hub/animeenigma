import { ref } from 'vue'
import { animeApi, episodeApi, userApi } from '@/api/client'
import { getLocalizedTitle, getLocalizedGenre } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'

interface ApiGenre {
  name_ru?: string
  name?: string
}

interface ApiAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  description?: string
  poster_url?: string
  banner_url?: string
  genres?: ApiGenre[]
  status?: string
  year?: number
  score?: number
  episodes_count?: number
  episodes_aired?: number
  next_episode_at?: string | null
  shikimori_id?: string | null
  mal_id?: string | null
}

export interface Anime {
  id: string
  title: string
  name?: string
  nameRu?: string
  nameJp?: string
  description: string
  coverImage: string
  bannerImage?: string
  genres: string[]
  rawGenres?: { name?: string; nameRu?: string }[]
  status: string
  releaseYear: number
  rating: number
  totalEpisodes: number
  episodesAired: number
  nextEpisodeAt?: string
  shikimoriId?: string
  malId?: string
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
function transformAnime(apiAnime: ApiAnime): Anime {
  return {
    id: apiAnime.id,
    title: getLocalizedTitle(apiAnime.name, apiAnime.name_ru, apiAnime.name_jp),
    name: apiAnime.name || undefined,
    nameRu: apiAnime.name_ru || undefined,
    nameJp: apiAnime.name_jp || undefined,
    description: apiAnime.description || '',
    coverImage: getImageUrl(apiAnime.poster_url),
    bannerImage: apiAnime.banner_url,
    genres: apiAnime.genres?.map((g: ApiGenre) => getLocalizedGenre(g.name, g.name_ru)) || [],
    rawGenres: apiAnime.genres?.map((g: ApiGenre) => ({ name: g.name, nameRu: g.name_ru })) || [],
    status: apiAnime.status || '',
    releaseYear: apiAnime.year || 0,
    rating: apiAnime.score || 0,
    totalEpisodes: apiAnime.episodes_count || 0,
    episodesAired: apiAnime.episodes_aired || 0,
    nextEpisodeAt: apiAnime.next_episode_at || undefined,
    shikimoriId: apiAnime.shikimori_id || undefined,
    malId: apiAnime.mal_id || undefined
  }
}

function transformAnimeList(apiList: { data?: ApiAnime[] } | ApiAnime[] | null | undefined): Anime[] {
  if (!apiList) return []
  const list = Array.isArray(apiList) ? apiList : apiList.data
  if (!Array.isArray(list)) return []
  return list.map(transformAnime)
}

interface PaginationMeta {
  page: number
  page_size: number
  total_count: number
  total_pages: number
}

export function useAnime() {
  const anime = ref<Anime | null>(null)
  const animeList = ref<Anime[]>([])
  const episodes = ref<Episode[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const paginationMeta = ref<PaginationMeta | null>(null)

  const fetchAnime = async (id: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getById(id)
      const data = response.data?.data || response.data
      anime.value = transformAnime(data)
      return anime.value
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch anime'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchAnimeList = async (params?: Record<string, unknown>) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getAll(params)
      animeList.value = transformAnimeList(response.data)
      paginationMeta.value = response.data?.meta || null
      return animeList.value
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch anime list'
      throw err
    } finally {
      loading.value = false
    }
  }

  const searchAnime = async (query: string, source?: string) => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.search(query, source)
      animeList.value = transformAnimeList(response.data)
      return animeList.value
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Search failed'
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
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch trending'
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
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch popular'
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
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch schedule'
      throw err
    } finally {
      loading.value = false
    }
  }

  const fetchOngoing = async () => {
    loading.value = true
    error.value = null
    try {
      const response = await animeApi.getOngoing()
      return transformAnimeList(response.data)
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch ongoing'
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
    } catch (err: unknown) {
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
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to fetch episode'
      throw err
    } finally {
      loading.value = false
    }
  }

  const addToWatchlist = async (animeId: string) => {
    try {
      await userApi.addToWatchlist(animeId, 'plan_to_watch')
      return true
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to add to watchlist'
      return false
    }
  }

  const removeFromWatchlist = async (animeId: string) => {
    try {
      await userApi.removeFromWatchlist(animeId)
      return true
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Failed to remove from watchlist'
      return false
    }
  }

  return {
    anime,
    animeList,
    paginationMeta,
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
