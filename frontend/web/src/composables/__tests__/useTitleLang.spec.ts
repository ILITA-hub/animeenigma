import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { fromCatalogAnime } from '@/utils/toCardModel'
import { useTitleLang } from '@/composables/useTitleLang'

const ANIME = {
  id: '1',
  title: 'Sousou no Frieren',
  name: 'Sousou no Frieren',
  nameRu: 'Провожающая в последний путь Фрирен',
  nameJp: '葬送のフリーレン',
  coverImage: '',
}

afterEach(() => {
  // Reset to 'auto' so tests don't leak state through the module-level ref.
  useTitleLang().setTitleLang('auto')
  localStorage.clear()
})

describe('catalog title-language toggle reactivity', () => {
  it('re-renders card titles when the toggle flips (Vue tracks the dep through the mapper)', async () => {
    localStorage.setItem('locale', 'ru')
    const { setTitleLang } = useTitleLang()

    const Probe = defineComponent({
      setup: () => () => h('span', fromCatalogAnime(ANIME).title),
    })
    const wrapper = mount(Probe)

    // Default 'auto' + ru locale -> Russian title.
    expect(wrapper.text()).toBe(ANIME.nameRu)

    setTitleLang('en')
    await nextTick()
    expect(wrapper.text()).toBe(ANIME.name)

    setTitleLang('ru')
    await nextTick()
    expect(wrapper.text()).toBe(ANIME.nameRu)
  })

  it('persists the choice to localStorage and clears it for "auto"', () => {
    const { setTitleLang } = useTitleLang()
    setTitleLang('en')
    expect(localStorage.getItem('titleLang')).toBe('en')
    setTitleLang('auto')
    expect(localStorage.getItem('titleLang')).toBeNull()
  })
})
