// Raw API shapes (snake_case, mirror the Go json tags).
export interface ApiCharacterSeyu {
  shikimori_id: string
  name: string
  name_ru?: string
  image_url?: string
  url?: string
}

export interface ApiCharacter {
  id: string
  shikimori_id: string
  mal_id?: string
  name: string
  name_ru?: string
  name_jp?: string
  synonyms?: string
  poster_url?: string
  description?: string
  url?: string
  seyu?: ApiCharacterSeyu[]
}

export interface ApiAnimeCharacter extends ApiCharacter {
  role: 'main' | 'supporting'
  position: number
}

// Frontend view-model for a character card on the anime page.
export interface CharacterCardModel {
  id: string          // shikimori_id — used in /characters/:id
  name: string        // already localized (RU fallback EN)
  image: string       // proxied poster url
  role: 'main' | 'supporting'
}

// Frontend view-model for a voice actor (seyu) on the character detail page.
export interface SeyuModel {
  id: string
  name: string       // localized
  image: string      // proxied
}

// Frontend model for the character detail page.
export interface CharacterDetail {
  shikimoriId: string
  name: string
  nameRu?: string
  nameJp?: string
  synonyms?: string
  image: string
  description?: string
  seyu: SeyuModel[]
}
