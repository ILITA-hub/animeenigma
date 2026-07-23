import { describe, it, expect } from 'vitest'
import { secondaryTitleForms } from './animeFormatters'

describe('secondaryTitleForms', () => {
  const frieren = {
    name: 'Sousou no Frieren',
    nameRu: 'Провожающая в последний путь Фрирен',
    nameJp: '葬送のフリーレン',
  }

  it('lists the other-language forms in JP → romaji → RU order', () => {
    // 'en' locale puts the romaji `name` in the h1, so it drops out.
    expect(secondaryTitleForms(frieren, 'Sousou no Frieren')).toEqual([
      '葬送のフリーレン',
      'Провожающая в последний путь Фрирен',
    ])
  })

  it('excludes whichever form the h1 already renders', () => {
    expect(secondaryTitleForms(frieren, '葬送のフリーレン')).toEqual([
      'Sousou no Frieren',
      'Провожающая в последний путь Фрирен',
    ])
    expect(secondaryTitleForms(frieren, 'Провожающая в последний путь Фрирен')).toEqual([
      '葬送のフリーレン',
      'Sousou no Frieren',
    ])
  })

  it('collapses gracefully when a field is empty', () => {
    expect(
      secondaryTitleForms({ name: 'Bocchi the Rock!', nameRu: 'Одинокий рокер!' }, 'Одинокий рокер!'),
    ).toEqual(['Bocchi the Rock!'])
    expect(secondaryTitleForms({ name: 'Bocchi the Rock!' }, 'Bocchi the Rock!')).toEqual([])
    expect(secondaryTitleForms({}, 'Anything')).toEqual([])
  })

  it('dedupes untranslated fields that repeat the romaji name', () => {
    // Shikimori leaves name_ru == name when there is no RU translation.
    expect(
      secondaryTitleForms(
        { name: 'Zenshuu', nameRu: 'Zenshuu', nameJp: 'ゼンシュウ' },
        'Zenshuu',
      ),
    ).toEqual(['ゼンシュウ'])
  })

  it('ignores whitespace and case when matching the primary title', () => {
    expect(
      secondaryTitleForms({ name: '  Naruto  ', nameJp: 'ナルト' }, 'naruto'),
    ).toEqual(['ナルト'])
  })

  it('keeps every form when the primary title is missing', () => {
    expect(secondaryTitleForms(frieren)).toEqual([
      '葬送のフリーレン',
      'Sousou no Frieren',
      'Провожающая в последний путь Фрирен',
    ])
  })
})
