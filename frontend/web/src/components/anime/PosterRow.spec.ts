import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// Keep the real module (auth store, pulled in via useUserTimezone, needs
// createI18n) and only stub the composable this component consumes.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterRow from './PosterRow.vue'
import PosterImage from './PosterImage.vue'
import type { AnimeCardModel } from '@/types/card'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountRow(model: Partial<AnimeCardModel> = {}, variant: 'ongoing' | 'top' | 'announced' = 'ongoing', rank?: number, season?: string) {
  const full: AnimeCardModel = {
    id: '1', href: '/anime/1', title: 'Frieren: Beyond Journey\'s End',
    coverImage: 'http://x/p.jpg', year: 2023, episodes: 28,
    malScore: 8.9, siteScore: 9.4, airing: true,
    nextEpisode: { ep: 6, when: '2026-06-10T12:00:00Z' }, listStatus: null, progress: null,
    ...model,
  }
  return mount(PosterRow, {
    props: { model: full, variant, rank, ...(season !== undefined ? { season } : {}) },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('PosterRow', () => {
  it('renders a single-line title (truncate class)', () => {
    const w = mountRow()
    expect(w.find('[data-testid="row-title"]').classes()).toContain('truncate')
  })

  it('shows the airing chip for ongoing variant even when airing flag is false', () => {
    // Locks Fix 1: chip must appear for ALL ongoing rows regardless of model.airing
    const w = mountRow({ airing: false }, 'ongoing')
    expect(w.find('[data-testid="airing"]').exists()).toBe(true)
  })

  it('shows the next-ep line when nextEpisode is present on ongoing variant', () => {
    const w = mountRow({ nextEpisode: { ep: 6, when: '2026-06-10T12:00:00Z' } }, 'ongoing')
    expect(w.find('[data-testid="next-ep"]').exists()).toBe(true)
  })

  it('shows the rank numeral for the top variant', () => {
    const w = mountRow({}, 'top', 1)
    expect(w.find('[data-testid="rank"]').text()).toBe('1')
  })

  it('shows the season chip for the announced variant', () => {
    const w = mountRow({}, 'announced', undefined, 'winter')
    expect(w.find('[data-testid="season"]').exists()).toBe(true)
  })

  it('renders the centered-glass kebab and emits openMenu', async () => {
    const w = mountRow()
    const kebab = w.find('[data-testid="row-kebab"]')
    expect(kebab.exists()).toBe(true)
    await kebab.trigger('click')
    expect(w.emitted('openMenu')).toBeTruthy()
  })
})

describe('PosterRow poster — delegates to PosterImage', () => {
  // The skeleton overlay, image-proxy resize (w=128) and proxied→original→
  // fallback chain now live in PosterImage and are covered by PosterImage.spec.
  // Here we only assert PosterRow wires the right props into it.
  it('renders a PosterImage with the model cover and the resized proxy width', () => {
    const shiki = 'https://shikimori.io/uploads/poster/animes/1/abc.jpeg'
    const pi = mountRow({ coverImage: shiki }).findComponent(PosterImage)
    expect(pi.exists()).toBe(true)
    expect(pi.props('src')).toBe(shiki)
    expect(pi.props('proxyWidth')).toBe(128)
  })

  it('falls back to the placeholder when the model has no cover', () => {
    const pi = mountRow({ coverImage: '' }).findComponent(PosterImage)
    expect(pi.props('src')).toBe('/placeholder.svg')
  })
})
