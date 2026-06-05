import { getLocalizedTitle, getLocalizedGenre } from '@/utils/title'
import type { AnimeCardModel, CardExtras } from '@/types/card'

// Minimal structural shapes the mappers accept. We intentionally keep these
// local + loose (rather than importing the full Anime/HomeAnime interfaces) so
// the normalizer has one job and does not couple to every optional API field.

interface CatalogAnimeLike {
  id: string | number
  title: string
  name?: string
  nameRu?: string
  nameJp?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  totalEpisodes?: number
  episodesAired?: number
  nextEpisodeAt?: string
  status?: string
  quality?: string
  hasDub?: boolean
  genres?: string[]
  rawGenres?: { name?: string; nameRu?: string }[]
}

interface HomeAnimeLike {
  id: string | number
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  next_episode_at?: string
}

interface ContinueWatchingLike {
  anime: HomeAnimeLike
  episode_number: number
  progress?: number
  duration?: number
}

const PLACEHOLDER = '/placeholder.svg'

function isAiring(status?: string): boolean {
  return status === 'ongoing'
}

export function fromCatalogAnime(a: CatalogAnimeLike, extras?: CardExtras): AnimeCardModel {
  const id = String(a.id)
  const primaryGenre = a.rawGenres?.length
    ? getLocalizedGenre(a.rawGenres[0].name, a.rawGenres[0].nameRu)
    : a.genres?.[0]
  const airing = isAiring(a.status)
  return {
    id,
    href: `/anime/${id}`,
    title:
      a.name || a.nameRu || a.nameJp
        ? getLocalizedTitle(a.name, a.nameRu, a.nameJp)
        : a.title,
    coverImage: a.coverImage || PLACEHOLDER,
    year: a.releaseYear || undefined,
    episodes: a.totalEpisodes || undefined,
    primaryGenre: primaryGenre || undefined,
    malScore: a.rating || undefined,
    siteScore: extras?.siteScore,
    quality: a.quality || undefined,
    hasDub: a.hasDub || undefined,
    listStatus: extras?.listStatus ?? null,
    progress: extras?.progress ?? null,
    airing,
    nextEpisode:
      airing && a.nextEpisodeAt
        ? { ep: (a.episodesAired || 0) + 1, when: a.nextEpisodeAt }
        : null,
  }
}

export function fromHomeAnime(a: HomeAnimeLike, extras?: CardExtras): AnimeCardModel {
  const id = String(a.id)
  const airing = isAiring(a.status)
  return {
    id,
    href: `/anime/${id}`,
    title: getLocalizedTitle(a.name, a.name_ru, a.name_jp) || '',
    coverImage: a.poster_url || PLACEHOLDER,
    year: a.year || undefined,
    episodes: a.episodes_count || undefined,
    malScore: a.score || undefined,
    siteScore: extras?.siteScore,
    listStatus: extras?.listStatus ?? null,
    progress: extras?.progress ?? null,
    airing,
    nextEpisode:
      airing && a.next_episode_at
        ? { ep: (a.episodes_aired || 0) + 1, when: a.next_episode_at }
        : null,
  }
}

export function fromContinueWatching(item: ContinueWatchingLike): AnimeCardModel {
  const a = item.anime
  const id = String(a.id)
  return {
    id,
    href: `/anime/${id}?episode=${item.episode_number}`,
    title: getLocalizedTitle(a.name, a.name_ru, a.name_jp) || '',
    coverImage: a.poster_url || PLACEHOLDER,
    episodes: a.episodes_count || undefined,
    // For continue-watching the "next episode" slot carries the resume episode;
    // `when` is unused by MediaTile variant ② so we leave it empty.
    nextEpisode: { ep: item.episode_number, when: '' },
    progress: { current: item.episode_number, total: a.episodes_count || null },
    listStatus: null,
    airing: false,
  }
}
