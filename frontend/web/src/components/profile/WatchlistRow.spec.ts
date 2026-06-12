import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k),
    locale: { value: 'ru' },
  }),
  // The ui-index import chain reaches src/i18n.ts, which calls createI18n at
  // module scope — give it an inert instance.
  createI18n: () => ({
    install: () => {},
    global: { t: (k: string) => k, locale: { value: 'ru' }, availableLocales: ['ru'] },
  }),
}))

import WatchlistRow from './WatchlistRow.vue'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

const entry = {
  anime_id: 'a1',
  anime: {
    name: 'Frieren',
    name_ru: 'Фрирен',
    poster_url: 'http://x/p.jpg',
    episodes_count: 28,
  },
  status: 'watching',
  score: 8,
  episodes: 5,
  started_at: '2026-01-10T00:00:00Z',
  completed_at: null,
}

function mountRow(overrides: Record<string, unknown> = {}, props: Record<string, unknown> = {}) {
  return mount(WatchlistRow, {
    props: {
      entry: { ...entry, ...overrides },
      index: 3,
      isOwn: true,
      statusOptions: [{ value: 'watching', label: 'Смотрю' }],
      ...props,
    },
    global: { stubs: { RouterLink: RouterLinkStub, Select: true } },
  })
}

describe('WatchlistRow', () => {
  it('renders localized title, index and a link to the anime page', () => {
    const w = mountRow()
    expect(w.find('[data-testid="row-title"]').text()).toBe('Фрирен')
    expect(w.find('[data-testid="row-title"]').attributes('href')).toBe('/anime/a1')
    expect(w.text()).toContain('3')
  })

  it('score button opens an inline input; blur emits editScore with the value', async () => {
    const w = mountRow()
    await w.find('[data-testid="score-button"]').trigger('click')
    const input = w.find('[data-testid="score-input"] input, input[type="number"]')
    expect(input.exists()).toBe(true)
    await input.setValue('9')
    await input.trigger('blur')
    expect(w.emitted('editScore')).toBeTruthy()
    expect(w.emitted('editScore')![0][0]).toBe('9')
  })

  it('episode steppers emit updateEpisodes with the adjusted count', async () => {
    const w = mountRow()
    await w.find('[data-testid="ep-plus"]').trigger('click')
    expect(w.emitted('updateEpisodes')![0][0]).toBe(6)
    await w.find('[data-testid="ep-minus"]').trigger('click')
    expect(w.emitted('updateEpisodes')![1][0]).toBe(4)
  })

  it('plus stepper is disabled at the episode cap', () => {
    const w = mountRow({ episodes: 28 })
    expect(w.find('[data-testid="ep-plus"]').attributes('disabled')).toBeDefined()
  })

  it('remove button emits remove', async () => {
    const w = mountRow()
    await w.find('[data-testid="row-remove"]').trigger('click')
    expect(w.emitted('remove')).toBeTruthy()
  })

  it('public profile hides all editors and shows a status badge instead', () => {
    const w = mountRow({}, { isOwn: false })
    expect(w.find('[data-testid="score-button"]').exists()).toBe(false)
    expect(w.find('[data-testid="ep-plus"]').exists()).toBe(false)
    expect(w.find('[data-testid="row-remove"]').exists()).toBe(false)
    expect(w.text()).toContain('profile.watchlist.watching')
    // progress is still visible read-only
    expect(w.text()).toContain('5')
    expect(w.text()).toContain('28')
  })
})
