import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
const mocks = vi.hoisted(() => ({
  recs: { __v_isRef: true, value: [] as Array<Record<string, unknown>> },
  isLoading: { __v_isRef: true, value: false },
  error: { __v_isRef: true, value: null as string | null },
  refresh: vi.fn(),
  emitRecClick: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/composables/useRecs', () => ({
  useRecs: () => ({
    recs: mocks.recs,
    isLoading: mocks.isLoading,
    error: mocks.error,
    refresh: mocks.refresh,
  }),
}))

vi.mock('@/utils/recsAnalytics', () => ({ emitRecClick: mocks.emitRecClick }))

vi.mock('@/utils/toCardModel', () => ({
  fromHomeAnime: (anime: { id: string; name?: string }) => ({
    href: `/anime/${anime.id}`,
    title: anime.name ?? '',
  }),
}))

vi.mock('@/components/anime', () => ({
  PosterCard: {
    props: ['model'],
    template:
      '<a data-testid="poster-card" :href="model.href" @click.prevent>{{ model.title }}</a>',
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

import Recommendations from './Recommendations.vue'

function mountView() {
  return mount(Recommendations, {
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        Badge: { template: '<span><slot /></span>' },
        Button: { template: '<button><slot /></button>' },
        EmptyState: { props: ['title', 'description'], template: '<div>{{ title }} {{ description }}<slot name="action" /></div>' },
        Spinner: { template: '<span data-testid="spinner" />' },
      },
    },
  })
}

beforeEach(() => {
  mocks.recs.value = []
  mocks.isLoading.value = false
  mocks.error.value = null
  mocks.refresh.mockClear()
  mocks.emitRecClick.mockClear()
})

describe('Recommendations view', () => {
  it('renders the authenticated recommendation list with rank and reason', () => {
    mocks.recs.value = [
      {
        anime: { id: 'anime-1', name: 'First pick' },
        rank: 1,
        pinned: false,
        top_contributor: 's2',
      },
      {
        anime: { id: 'anime-2', name: 'Pinned pick' },
        rank: 2,
        pinned: true,
        pin_reason_key: 'recs.pinReason.becauseYouFinished',
        pin_reason_data: { name: 'Seed anime' },
      },
    ]

    const wrapper = mountView()

    expect(wrapper.findAll('[data-testid="poster-card"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('#1')
    expect(wrapper.text()).toContain('recs.reason.s2')
    expect(wrapper.text()).toContain('recs.pinReason.becauseYouFinished')
  })

  it('attributes anime navigation to the recommendations page', async () => {
    mocks.recs.value = [
      {
        anime: { id: 'anime-1', name: 'First pick' },
        rank: 3,
        pinned: false,
        top_contributor: 's4',
      },
    ]

    const wrapper = mountView()
    await wrapper.get('[data-testid="poster-card"]').trigger('click')

    expect(mocks.emitRecClick).toHaveBeenCalledWith({
      event_type: 'rec_click',
      anime_id: 'anime-1',
      signal_id: 's4',
      pinned: false,
      pin_source: undefined,
      pin_seed_anime_id: undefined,
      source_route: '/recs',
      rank: 3,
    })
  })

  it('offers a retry when recommendations fail to load', async () => {
    mocks.error.value = 'network error'
    const wrapper = mountView()

    await wrapper.get('button').trigger('click')

    expect(wrapper.text()).toContain('recs.loadErrorTitle')
    expect(mocks.refresh).toHaveBeenCalledOnce()
  })
})
