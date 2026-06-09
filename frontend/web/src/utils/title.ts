/**
 * Returns the localized anime title.
 *
 * Priority by language:
 *  - ru: name_ru > name > name_jp
 *  - en: name > name_ru > name_jp
 *  - ja: name_jp > name > name_ru
 *
 * `override` lets the catalog title-language toggle pin a language independent of
 * the global UI locale. 'auto' (or undefined/null) defers to the locale setting.
 */
export function getLocalizedTitle(
  name?: string | null,
  nameRu?: string | null,
  nameJp?: string | null,
  override?: 'auto' | 'ru' | 'en' | 'ja' | null,
): string {
  const locale =
    override && override !== 'auto' ? override : localStorage.getItem('locale') || 'ru'
  switch (locale) {
    case 'en':
      return name || nameRu || nameJp || ''
    case 'ja':
      return nameJp || name || nameRu || ''
    default: // 'ru'
      return nameRu || name || nameJp || ''
  }
}

/**
 * Returns the localized genre name based on the current locale setting.
 */
export function getLocalizedGenre(
  name?: string | null,
  nameRu?: string | null,
): string {
  const locale = localStorage.getItem('locale') || 'ru'
  if (locale === 'ru') return nameRu || name || ''
  return name || nameRu || ''
}
