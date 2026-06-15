import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import type { SearchResultItem } from '@/api/anidle'

// Mock the anidle API module
vi.mock('@/api/anidle', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/anidle')>()
  return {
    ...actual,
    anidleApi: {
      ...actual.anidleApi,
      search: vi.fn(),
    },
  }
})

// Mock image proxy
vi.mock('@/composables/useImageProxy', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useImageProxy')>()
  return {
    ...actual,
    cardPosterUrl: (url: string) => url ?? '/placeholder.svg',
  }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

function makeItem(over: Partial<SearchResultItem> = {}): SearchResultItem {
  return {
    id: 'anime-1',
    name_ru: 'Атака Титанов',
    name_en: 'Attack on Titan',
    name_jp: '進撃の巨人',
    poster_url: 'https://shikimori.one/aot.jpg',
    year: 2013,
    episodes: 25,
    score: 9.0,
    status: 'released',
    rating: 'r',
    genres: [],
    studios: [],
    tags: [],
    ...over,
  }
}

async function mountSearch(props = {}) {
  const AnidleSearch = (await import('./AnidleSearch.vue')).default
  return mount(AnidleSearch, {
    props,
    global: {
      plugins: [i18n],
      stubs: { Spinner: { template: '<span class="spinner"/>' } },
    },
  })
}

describe('AnidleSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders an input element', async () => {
    const w = await mountSearch()
    expect(w.find('input').exists()).toBe(true)
  })

  it('does not emit select without user interaction', async () => {
    const w = await mountSearch()
    expect(w.emitted('select')).toBeFalsy()
  })

  it('disables the input when disabled prop is true', async () => {
    const w = await mountSearch({ disabled: true })
    const input = w.find('input')
    expect(input.attributes('disabled')).toBeDefined()
  })

  it('shows dropdown results after typing ≥2 chars and receiving results', async () => {
    const { anidleApi } = await import('@/api/anidle')
    const mockSearch = vi.mocked(anidleApi.search)
    mockSearch.mockResolvedValue({ data: { data: [makeItem()] } } as any)

    const w = await mountSearch()
    const input = w.find('input')
    await input.setValue('at')
    // Trigger debounce
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()

    expect(mockSearch).toHaveBeenCalledWith('at')
    // Should show dropdown
    expect(w.find('[role="listbox"]').exists()).toBe(true)
    expect(w.find('[role="option"]').exists()).toBe(true)
  })

  it('emits select with the item id when clicking a dropdown item', async () => {
    const { anidleApi } = await import('@/api/anidle')
    const mockSearch = vi.mocked(anidleApi.search)
    const item = makeItem({ id: 'aot-123' })
    mockSearch.mockResolvedValue({ data: { data: [item] } } as any)

    const w = await mountSearch()
    const input = w.find('input')
    await input.setValue('at')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()

    const option = w.find('[role="option"]')
    expect(option.exists()).toBe(true)
    await option.trigger('mousedown')

    expect(w.emitted('select')).toBeTruthy()
    expect(w.emitted('select')![0]).toEqual(['aot-123'])
  })

  it('clears the input after selecting an item', async () => {
    const { anidleApi } = await import('@/api/anidle')
    const mockSearch = vi.mocked(anidleApi.search)
    mockSearch.mockResolvedValue({ data: { data: [makeItem()] } } as any)

    const w = await mountSearch()
    const input = w.find('input')
    await input.setValue('at')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()

    await w.find('[role="option"]').trigger('mousedown')
    expect((input.element as HTMLInputElement).value).toBe('')
  })
})
