import { ref } from 'vue'
import { animeApi, apiClient } from '@/api/client'
import type { SpotlightCard, SpotlightResponse } from '@/types/spotlight'

// Cross-route hand-off for the secret player tour. Module state is
// intentionally session-only: reload cancels the hidden tour.
export const playerGuideRequested = ref(false)
export const playerGuideLaunching = ref(false)

export type PlayerGuideMenu = 'episodes' | 'source' | 'subs' | 'settings' | null
export const playerGuideMenu = ref<PlayerGuideMenu>(null)

function unwrapData(value: unknown): unknown {
  let current = value
  for (let depth = 0; depth < 3; depth += 1) {
    if (!current || typeof current !== 'object' || Array.isArray(current) || !('data' in current)) break
    current = (current as { data?: unknown }).data
  }
  return current
}

function firstPopularId(value: unknown): string | null {
  const list = unwrapData(value)
  if (!Array.isArray(list)) return null
  const first = list[0]
  if (!first || typeof first !== 'object') return null
  const id = (first as { id?: unknown }).id
  return typeof id === 'string' && id.length > 0 ? id : null
}

/** Resolve the player-tour title without asking the viewer to choose one. */
export async function resolvePlayerGuideAnimeId(): Promise<string | null> {
  try {
    const response = await apiClient.get<SpotlightResponse | { data: SpotlightResponse }>('/home/spotlight')
    const body = unwrapData(response.data) as SpotlightResponse | null
    const cards = Array.isArray(body?.cards) ? body.cards : []
    const curated = cards.find((card: SpotlightCard) => card.type === 'curated')
    if (curated?.type === 'curated') {
      const anime = curated.data.anime
      if (anime.id && anime.has_video !== false) return anime.id
    }
  } catch {
    // Curator is optional. Popular is the documented fallback below.
  }

  try {
    const response = await animeApi.getPopular()
    return firstPopularId(response.data)
  } catch {
    return null
  }
}
