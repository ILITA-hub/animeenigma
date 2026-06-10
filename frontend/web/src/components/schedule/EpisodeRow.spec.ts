// frontend/web/src/components/schedule/EpisodeRow.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeRow from './EpisodeRow.vue'

const occ = {
  anime: { id: '1', name: 'Kaiju No. 8', name_ru: 'Кайдзю №8', poster_url: '/p.jpg' },
  episode: 10,
  // Occurrence dates arrive pre-shifted into the display timezone (projection
  // layer), so the row formats LOCAL fields — construct via local fields here.
  date: new Date(2026, 5, 8, 17, 0, 0),
}

function mountRow() {
  return mount(EpisodeRow, {
    props: { occurrence: occ },
    global: { mocks: { $t: (k: string) => k } },
  })
}

describe('EpisodeRow', () => {
  it('renders the localized title, episode number and time', () => {
    const w = mountRow()
    expect(w.text()).toContain('Кайдзю №8')
    expect(w.text()).toContain('10')
    expect(w.text()).toContain('17:00')
  })
  it('renders the poster with alt text', () => {
    const w = mountRow()
    const img = w.get('img')
    expect(img.attributes('src')).toBe('/p.jpg')
    expect(img.attributes('alt')).toBe('Кайдзю №8')
  })
})
