import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { charactersApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import type {
  ApiAnimeCharacter,
  ApiCharacter,
  CharacterCardModel,
  CharacterDetail,
} from '@/types/character'

function toCardModel(c: ApiAnimeCharacter): CharacterCardModel {
  return {
    id: c.shikimori_id,
    name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
    image: getImageUrl(c.poster_url),
    role: c.role === 'main' ? 'main' : 'supporting',
  }
}

function toDetail(c: ApiCharacter): CharacterDetail {
  return {
    shikimoriId: c.shikimori_id,
    name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
    nameRu: c.name_ru || undefined,
    nameJp: c.name_jp || undefined,
    synonyms: c.synonyms || undefined,
    image: getImageUrl(c.poster_url),
    description: c.description || undefined,
  }
}

// List of an anime's characters. Fails soft (no global error) — the section
// just stays empty if the fetch fails, like the related rail.
export function useCharacters() {
  const characters = ref<CharacterCardModel[]>([])

  const fetchCharacters = async (animeId: string) => {
    try {
      const resp = await charactersApi.getAnimeCharacters(animeId)
      const data = (resp.data?.data || resp.data) as ApiAnimeCharacter[]
      characters.value = Array.isArray(data) ? data.map(toCardModel) : []
      return characters.value
    } catch (err) {
      console.warn('Failed to fetch characters:', err)
      characters.value = []
      return []
    }
  }

  return { characters, fetchCharacters }
}

// Single character detail (for /characters/:id).
export function useCharacter() {
  const { t } = useI18n()
  const character = ref<CharacterDetail | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const fetchCharacter = async (id: string) => {
    loading.value = true
    error.value = null
    try {
      const resp = await charactersApi.getCharacter(id)
      const data = (resp.data?.data || resp.data) as ApiCharacter
      character.value = toDetail(data)
      return character.value
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || t('characters.fetchError')
      throw err
    } finally {
      loading.value = false
    }
  }

  return { character, loading, error, fetchCharacter }
}
