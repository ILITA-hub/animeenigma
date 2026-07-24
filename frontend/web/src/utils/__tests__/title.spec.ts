import { afterEach, describe, expect, it } from 'vitest'
import { getLocalizedTitle } from '@/utils/title'
import { interfaceLocale } from '@/composables/useInterfaceLocale'

const NAME = 'Sousou no Frieren'
const RU = 'Провожающая в последний путь Фрирен'
const JP = '葬送のフリーレン'

// getLocalizedTitle derives the "no override" locale from the reactive
// interfaceLocale ref (so title/genre computeds re-render when the Navbar
// language switch flips it) rather than from localStorage — see title.ts.
afterEach(() => {
  interfaceLocale.value = 'ru'
})

describe('getLocalizedTitle', () => {
  it('follows the global locale when no override given', () => {
    interfaceLocale.value = 'ru'
    expect(getLocalizedTitle(NAME, RU, JP)).toBe(RU)
    interfaceLocale.value = 'en'
    expect(getLocalizedTitle(NAME, RU, JP)).toBe(NAME)
  })

  it('"auto" override defers to the global locale', () => {
    interfaceLocale.value = 'ru'
    expect(getLocalizedTitle(NAME, RU, JP, 'auto')).toBe(RU)
  })

  it('a concrete override wins over the global locale', () => {
    interfaceLocale.value = 'ru'
    expect(getLocalizedTitle(NAME, RU, JP, 'en')).toBe(NAME)
    interfaceLocale.value = 'en'
    expect(getLocalizedTitle(NAME, RU, JP, 'ru')).toBe(RU)
  })

  it('falls back through the priority chain when a field is missing', () => {
    // EN override but no romaji name -> next in en chain is ru.
    expect(getLocalizedTitle(null, RU, JP, 'en')).toBe(RU)
    // RU override but only jp present.
    expect(getLocalizedTitle(null, null, JP, 'ru')).toBe(JP)
  })
})
