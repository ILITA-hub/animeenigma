import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import AdminGacha from './AdminGacha.vue'
import type { GachaCard } from '@/api/gacha'

// ── Mock API ──────────────────────────────────────────────────────────────────
vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return {
    ...actual,
    gachaAdminApi: {
      listCards:            vi.fn(),
      listGroups:           vi.fn(),
      listBanners:          vi.fn(),
      createCard:           vi.fn(),
      updateCard:           vi.fn(),
      deleteCard:           vi.fn(),
      createGroup:          vi.fn(),
      renameGroup:          vi.fn(),
      deleteGroup:          vi.fn(),
      createBanner:         vi.fn(),
      updateBanner:         vi.fn(),
      deleteBanner:         vi.fn(),
      addBannerCards:       vi.fn(),
      addGroupCardsToBanner: vi.fn(),
      uploadFile:           vi.fn(),
      uploadUrl:            vi.fn(),
      getBanner:            vi.fn(),
      setBannerCards:       vi.fn(),
      addCardsToGroup:      vi.fn(),
      removeCardFromGroup:  vi.fn(),
    },
  }
})

// ── i18n ─────────────────────────────────────────────────────────────────────
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru } })

// ── Fixtures ──────────────────────────────────────────────────────────────────
function makeCard(overrides: Partial<GachaCard> = {}): GachaCard {
  return {
    id: 'card-1',
    name: 'Test Hero',
    source_title: 'Test Anime',
    image_path: 'cards/test.webp',
    rarity: 'SSR',
    enabled: true,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    ...overrides,
  }
}

function emptyListResponse() {
  return { data: { success: true, data: [] } }
}

describe('AdminGacha', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue(emptyListResponse() as never)
    vi.mocked(gachaAdminApi.listGroups).mockResolvedValue(emptyListResponse() as never)
    vi.mocked(gachaAdminApi.listBanners).mockResolvedValue(emptyListResponse() as never)
  })

  function mountComponent() {
    return mount(AdminGacha, {
      global: {
        plugins: [i18n, pinia],
        stubs: {
          Spinner: { template: '<div data-testid="spinner" />' },
          Modal: {
            props: ['modelValue', 'title'],
            template: '<div data-testid="modal" v-if="modelValue"><slot /><slot name="footer" /></div>',
          },
          Input: {
            props: ['modelValue'],
            template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          },
          Select: { props: ['modelValue', 'options'], template: '<select />' },
          Checkbox: { props: ['modelValue'], template: '<input type="checkbox" :checked="modelValue" />' },
          Pencil: { template: '<span />' },
          Trash2: { template: '<span />' },
          Upload: { template: '<span />' },
        },
      },
    })
  }

  it('renders the page heading', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    expect(wrapper.html()).toContain('Gacha Manager')
  })

  it('shows Cards tab table after loading a card', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({ data: { success: true, data: [makeCard()] } } as never)
    const wrapper = mountComponent()
    await flushPromises()
    expect(wrapper.find('[data-testid="cards-tab-table"]').exists()).toBe(true)
  })

  it('renders a card row with name and rarity', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { success: true, data: [makeCard({ name: 'Dragon Knight', rarity: 'SSR' })] },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    expect(wrapper.html()).toContain('Dragon Knight')
    expect(wrapper.html()).toContain('SSR')
  })

  it('opens card create dialog on "+ New Card" button click', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    const buttons = wrapper.findAll('button')
    const createBtn = buttons.find(b => b.text().includes('New Card'))
    expect(createBtn).toBeDefined()
    await createBtn!.trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="modal"]').exists()).toBe(true)
  })

  it('calls gachaAdminApi.listCards on mount', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    mountComponent()
    await flushPromises()
    expect(vi.mocked(gachaAdminApi.listCards)).toHaveBeenCalledOnce()
  })

  it('shows spinner while loading cards', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    // Keep all three pending
    vi.mocked(gachaAdminApi.listCards).mockReturnValue(new Promise(() => {}) as never)
    vi.mocked(gachaAdminApi.listGroups).mockReturnValue(new Promise(() => {}) as never)
    vi.mocked(gachaAdminApi.listBanners).mockReturnValue(new Promise(() => {}) as never)
    const wrapper = mountComponent()
    // Allow onMounted to set loadingCards = true (one microtask tick)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="spinner"]').exists()).toBe(true)
  })
})
