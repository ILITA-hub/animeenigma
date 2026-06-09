import { afterEach, describe, expect, it } from 'vitest'
import { getLocalizedTitle } from '@/utils/title'

const NAME = 'Sousou no Frieren'
const RU = 'Провожающая в последний путь Фрирен'
const JP = '葬送のフリーレン'

afterEach(() => localStorage.clear())

describe('getLocalizedTitle', () => {
  it('follows the global locale when no override given', () => {
    localStorage.setItem('locale', 'ru')
    expect(getLocalizedTitle(NAME, RU, JP)).toBe(RU)
    localStorage.setItem('locale', 'en')
    expect(getLocalizedTitle(NAME, RU, JP)).toBe(NAME)
  })

  it('"auto" override defers to the global locale', () => {
    localStorage.setItem('locale', 'ru')
    expect(getLocalizedTitle(NAME, RU, JP, 'auto')).toBe(RU)
  })

  it('a concrete override wins over the global locale', () => {
    localStorage.setItem('locale', 'ru')
    expect(getLocalizedTitle(NAME, RU, JP, 'en')).toBe(NAME)
    localStorage.setItem('locale', 'en')
    expect(getLocalizedTitle(NAME, RU, JP, 'ru')).toBe(RU)
  })

  it('falls back through the priority chain when a field is missing', () => {
    // EN override but no romaji name -> next in en chain is ru.
    expect(getLocalizedTitle(null, RU, JP, 'en')).toBe(RU)
    // RU override but only jp present.
    expect(getLocalizedTitle(null, null, JP, 'ru')).toBe(JP)
  })
})
