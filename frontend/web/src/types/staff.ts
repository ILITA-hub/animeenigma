// Raw API shape (snake_case, mirrors Go json tags on domain.AnimePersonRole).
export interface ApiPersonRole {
  id: string
  shikimori_person_id?: string
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  role: string        // canonical EN, e.g. "Director"
  role_ru?: string
  is_producer?: boolean
  is_mangaka?: boolean
  position: number
}

// Frontend view-model for one staff row in the Details table.
export interface StaffMember {
  id: string          // shikimori_person_id (may be "")
  name: string        // localized (RU fallback EN)
  roleEn: string      // canonical EN role
  roleKey: string     // i18n key suffix under anime.roles.* (camelCased EN)
  roleRu?: string     // Shikimori RU label, used as fallback if no i18n key
}
