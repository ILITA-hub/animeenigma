import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import CharacterCard from './CharacterCard.vue'
import type { CharacterCardModel } from '@/types/character'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountCard(model: Partial<CharacterCardModel> = {}) {
  const full: CharacterCardModel = {
    id: '188176', name: 'Ферн', image: 'http://x/p.jpg', role: 'main', ...model,
  }
  return mount(CharacterCard, {
    props: { model: full },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('CharacterCard', () => {
  it('renders the character name', () => {
    expect(mountCard().text()).toContain('Ферн')
  })

  it('links to the character page by shikimori id', () => {
    expect(mountCard().find('a').attributes('href')).toBe('/characters/188176')
  })

  it('shows a Main role badge for main characters', () => {
    const w = mountCard({ role: 'main' })
    expect(w.find('[data-testid="role-badge"]').exists()).toBe(true)
    expect(w.text()).toContain('characters.main')
  })

  it('shows a Supporting role badge for supporting characters', () => {
    expect(mountCard({ role: 'supporting' }).text()).toContain('characters.supporting')
  })

  it('renders the portrait image with the character name as alt', () => {
    const img = mountCard().find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('alt')).toBe('Ферн')
  })
})
