import { ref, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Anime } from '@/composables/useAnime'
import { useSiteRatings } from '@/composables/useSiteRatings'
import { getImageUrl } from '@/composables/useImageProxy'
import { fromCatalogAnime } from '@/utils/toCardModel'
import { animeApi } from '@/api/client'
import type { AnimeCardModel } from '@/types/card'
import type { RelatedAnime } from './types'

/**
 * Related-anime rail for the anime page (extracted from Anime.vue): the
 * Shikimori-backed related list, card-model mapping, and site-rating overlay
 * for related titles already in the local catalog.
 */
export function useRelatedAnime(anime: Ref<Anime | null>) {
  const { locale } = useI18n()

  const relatedAnime = ref<RelatedAnime[]>([])
  const relatedFetched = ref(false)

  const { ratings: relatedSiteRatings, fetchRatings: fetchRelatedSiteRatings } = useSiteRatings()

  function relatedCardModel(item: RelatedAnime): AnimeCardModel {
    const sr = relatedSiteRatings.value[String(item.id)]
    return fromCatalogAnime(
      { ...item, totalEpisodes: item.episodes },
      { siteScore: sr && sr.total_reviews > 0 ? sr.average_score : undefined },
    )
  }

  async function fetchRelatedAnime() {
    if (!anime.value?.id) return
    try {
      const resp = await animeApi.getRelated(anime.value.id as string)
      const data = (resp.data?.data || resp.data) as Array<{
        shikimori_id: string
        local_id?: string
        name: string
        name_ru: string
        relation_ru: string
        relation_en: string
        score: number
        status: string
        poster_url: string
        year?: number
        episodes?: number
      }>
      relatedAnime.value = data.map(r => ({
        id: r.local_id || `shiki_${r.shikimori_id}`,
        title: r.name,
        nameRu: r.name_ru,
        name: r.name,
        coverImage: getImageUrl(r.poster_url),
        rating: r.score || undefined,
        releaseYear: r.year || undefined,
        episodes: r.episodes || undefined,
        relationLabel: locale.value === 'ru' ? r.relation_ru : r.relation_en,
      }))
      // Site ratings exist only for anime already in the local catalog (UUID ids)
      void fetchRelatedSiteRatings(data.filter(r => r.local_id).map(r => String(r.local_id)))
    } catch (e) {
      console.warn('Failed to fetch related anime:', e)
    }
  }

  /** Per-anime reset (route param change) — mirrors the loadAnimeData reset block. */
  function reset() {
    relatedAnime.value = []
    relatedFetched.value = false
  }

  return { relatedAnime, relatedFetched, relatedCardModel, fetchRelatedAnime, reset }
}
