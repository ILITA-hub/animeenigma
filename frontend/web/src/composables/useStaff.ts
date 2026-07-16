import { ref } from 'vue'
import { staffApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import type { ApiPersonRole, StaffMember } from '@/types/staff'

// Canonical EN role → i18n key suffix under anime.roles.*.
function roleKeyFromEn(en: string): string {
  const map: Record<string, string> = {
    'Director': 'director',
    'Original Creator': 'originalCreator',
    'Series Composition': 'seriesComposition',
    'Script': 'script',
    'Character Design': 'characterDesign',
    'Chief Animation Director': 'chiefAnimationDirector',
    'Art Director': 'artDirector',
    'Music': 'music',
    'Sound Director': 'soundDirector',
    'Director of Photography': 'directorOfPhotography',
    'Producer': 'producer',
    'Executive Producer': 'executiveProducer',
  }
  return map[en] ?? ''
}

function toMember(r: ApiPersonRole): StaffMember {
  return {
    id: r.shikimori_person_id || '',
    name: getLocalizedTitle(r.name, r.name_ru, r.name_jp),
    roleEn: r.role,
    roleKey: roleKeyFromEn(r.role),
    roleRu: r.role_ru || undefined,
    image: getImageUrl(r.poster_url),
  }
}

// Staff/crew for an anime. Fails soft (empty on error) like useCharacters.
export function useStaff() {
  const staff = ref<StaffMember[]>([])

  const fetchStaff = async (animeId: string) => {
    try {
      const resp = await staffApi.getAnimeStaff(animeId)
      const data = (resp.data?.data || resp.data) as ApiPersonRole[]
      staff.value = Array.isArray(data) ? data.map(toMember) : []
      return staff.value
    } catch (err) {
      console.warn('Failed to fetch staff:', err)
      staff.value = []
      return []
    }
  }

  return { staff, fetchStaff }
}
