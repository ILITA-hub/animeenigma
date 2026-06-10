import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}::${JSON.stringify(p)}` : k) }),
}))

import PosterRow from './PosterRow.vue'
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

describe('PosterRow poster skeleton + resized proxy', () => {
  it('shows the glass skeleton until the poster loads, then hides it', async () => {
    const w = mountRow()
    expect(w.find('[data-testid="poster-skeleton"]').exists()).toBe(true)
    expect(w.find('img.poster').classes()).toContain('opacity-0')

    await w.find('img.poster').trigger('load')
    expect(w.find('[data-testid="poster-skeleton"]').exists()).toBe(false)
    expect(w.find('img.poster').classes()).toContain('opacity-100')
  })

  it('routes shikimori posters through the resizing image-proxy (w=128)', () => {
    const shiki = 'https://shikimori.io/uploads/poster/animes/1/abc.jpeg'
    const w = mountRow({ coverImage: shiki })
    const src = w.find('img.poster').attributes('src')!
    expect(src).toContain('/api/streaming/image-proxy?url=')
    expect(src).toContain('w=128')
    expect(src).toContain(encodeURIComponent(shiki))
  })

  it('keeps non-proxyable poster URLs untouched', () => {
    const w = mountRow({ coverImage: 'http://x/p.jpg' })
    expect(w.find('img.poster').attributes('src')).toBe('http://x/p.jpg')
  })

  it('falls back proxied → original → placeholder on consecutive errors', async () => {
    const shiki = 'https://shikimori.io/uploads/poster/animes/1/abc.jpeg'
    const w = mountRow({ coverImage: shiki })

    await w.find('img.poster').trigger('error')
    expect(w.find('img.poster').attributes('src')).toBe(shiki)

    await w.find('img.poster').trigger('error')
    expect(w.find('img.poster').attributes('src')).toBe('/placeholder.svg')
  })
})
