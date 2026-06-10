import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import AdminGacha from './AdminGacha.vue'
import type { GachaCard, GachaGroup, GachaBanner } from '@/api/gacha'

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
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

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

function makeGroup(overrides: Partial<GachaGroup> = {}): GachaGroup {
  return {
    id: 'group-1',
    name: 'Test Group',
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    ...overrides,
  }
}

function makeBanner(overrides: Partial<GachaBanner> = {}): GachaBanner {
  return {
    id: 'banner-1',
    name: 'Test Banner',
    description: '',
    art_path: '',
    is_standard: false,
    enabled: true,
    sort_order: 0,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    ...overrides,
  }
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
          Check: { template: '<span />' },
          X: { template: '<span />' },
          Info: { template: '<span />' },
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

  // ── I3: Payload-shape assertions for the critical call sites ─────────────────

  it('createCard receives image_path + group_ids in payload', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({
      data: { success: true, data: makeCard() },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    // Open create dialog
    type CreateVm = { openCardCreate: () => void; saveCard: () => Promise<void>; cardForm: { name: string; imagePath: string; groupIds: string[] } }
    const vm = wrapper.vm as unknown as CreateVm
    vm.openCardCreate()
    await wrapper.vm.$nextTick()
    // Set form values
    vm.cardForm.name = 'Hero'
    vm.cardForm.imagePath = 'cards/hero.webp'
    vm.cardForm.groupIds = ['group-1']
    await vm.saveCard()
    expect(vi.mocked(gachaAdminApi.createCard)).toHaveBeenCalledWith(
      expect.objectContaining({ image_path: 'cards/hero.webp', group_ids: ['group-1'] }),
    )
  })

  it('uploadFile receives File instance + "cards" kind', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { success: true, data: { image_path: 'cards/x.webp', image_url: '/api/gacha/images/cards/x.webp' } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type UploadVm = { onFileChange: (e: Event) => Promise<void> }
    const vm = wrapper.vm as unknown as UploadVm
    const file = new File(['x'], 'card.png', { type: 'image/png' })
    const input = document.createElement('input')
    input.type = 'file'
    Object.defineProperty(input, 'files', { value: [file] })
    await vm.onFileChange({ target: input } as unknown as Event)
    expect(vi.mocked(gachaAdminApi.uploadFile)).toHaveBeenCalledWith(file, 'cards')
  })

  it('uploadUrl receives string + "cards" kind', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.uploadUrl).mockResolvedValue({
      data: { success: true, data: { image_path: 'cards/x.webp', image_url: '/api/gacha/images/cards/x.webp' } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type UrlVm = { onImageUrlBlur: () => Promise<void>; cardForm: { imageUrl: string } }
    const vm = wrapper.vm as unknown as UrlVm
    vm.cardForm.imageUrl = 'https://example.com/card.png'
    await vm.onImageUrlBlur()
    expect(vi.mocked(gachaAdminApi.uploadUrl)).toHaveBeenCalledWith('https://example.com/card.png', 'cards')
  })

  it('createGroup receives a string name', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.createGroup).mockResolvedValue({
      data: { success: true, data: makeGroup() },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type GroupVm = { openGroupCreate: () => void; saveGroup: () => Promise<void>; groupForm: { name: string } }
    const vm = wrapper.vm as unknown as GroupVm
    vm.openGroupCreate()
    await wrapper.vm.$nextTick()
    vm.groupForm.name = 'Shonen Heroes'
    await vm.saveGroup()
    expect(vi.mocked(gachaAdminApi.createGroup)).toHaveBeenCalledWith('Shonen Heroes')
  })

  it('renameGroup receives (id, string)', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.renameGroup).mockResolvedValue({
      data: { success: true, data: { updated: true } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type RenameVm = { openGroupRename: (g: GachaGroup) => void; saveGroup: () => Promise<void>; groupForm: { name: string } }
    const vm = wrapper.vm as unknown as RenameVm
    vm.openGroupRename(makeGroup({ id: 'group-42' }))
    await wrapper.vm.$nextTick()
    vm.groupForm.name = 'New Name'
    await vm.saveGroup()
    expect(vi.mocked(gachaAdminApi.renameGroup)).toHaveBeenCalledWith('group-42', 'New Name')
  })

  it('addBannerCards API called with (bannerId, string[]) via picker confirmPickerAdd', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.addBannerCards).mockResolvedValue({
      data: { success: true, data: { updated: true } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type BannerVm = {
      editBanner: GachaBanner | null
      bannerCurrentCardIds: string[]
      bannerPickerOpen: boolean
      pickerSelected: Set<string>
      confirmPickerAdd: () => Promise<void>
    }
    const vm = wrapper.vm as unknown as BannerVm
    vm.editBanner = makeBanner({ id: 'banner-99' })
    vm.bannerCurrentCardIds = []
    vm.bannerPickerOpen = true
    vm.pickerSelected = new Set(['card-a', 'card-b'])
    await vm.confirmPickerAdd()
    expect(vi.mocked(gachaAdminApi.addBannerCards)).toHaveBeenCalledWith('banner-99', expect.arrayContaining(['card-a', 'card-b']))
  })

  // ── Card picker tests ─────────────────────────────────────────────────────

  it('card picker opens when "Добавить карточки" button is clicked in banner edit dialog', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.getBanner).mockResolvedValue({
      data: { success: true, data: { ...makeBanner({ id: 'banner-1' }), card_ids: [] } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { openBannerEdit: (b: GachaBanner) => Promise<void>; bannerPickerOpen: boolean }
    const vm = wrapper.vm as unknown as Vm
    await vm.openBannerEdit(makeBanner({ id: 'banner-1' }))
    await flushPromises()
    expect(vm.bannerPickerOpen).toBe(false)
    // Simulate clicking the open-picker button
    vm.bannerPickerOpen = true
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="banner-card-picker"]').exists()).toBe(true)
  })

  it('picker search filter narrows visible cards by name substring', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: {
        success: true,
        data: [
          makeCard({ id: 'c1', name: 'Dragon Knight', source_title: 'Anime A', rarity: 'SSR' }),
          makeCard({ id: 'c2', name: 'Fire Sprite', source_title: 'Anime B', rarity: 'R' }),
          makeCard({ id: 'c3', name: 'Ice Dragon', source_title: 'Anime C', rarity: 'SR' }),
        ],
      },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { pickerSearch: string; pickerFilteredCards: GachaCard[] }
    const vm = wrapper.vm as unknown as Vm
    vm.pickerSearch = 'dragon'
    await wrapper.vm.$nextTick()
    const names = vm.pickerFilteredCards.map(c => c.name)
    expect(names).toContain('Dragon Knight')
    expect(names).toContain('Ice Dragon')
    expect(names).not.toContain('Fire Sprite')
  })

  it('confirmPickerAdd calls addBannerCards with selected ids and closes picker', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.addBannerCards).mockResolvedValue({
      data: { success: true, data: { updated: true } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = {
      editBanner: GachaBanner | null
      bannerPickerOpen: boolean
      bannerCurrentCardIds: string[]
      pickerSelected: Set<string>
      confirmPickerAdd: () => Promise<void>
    }
    const vm = wrapper.vm as unknown as Vm
    vm.editBanner = makeBanner({ id: 'banner-55' })
    vm.bannerCurrentCardIds = []
    vm.bannerPickerOpen = true
    vm.pickerSelected = new Set(['card-x', 'card-y'])
    await vm.confirmPickerAdd()
    expect(vi.mocked(gachaAdminApi.addBannerCards)).toHaveBeenCalledWith('banner-55', expect.arrayContaining(['card-x', 'card-y']))
    expect(vm.bannerPickerOpen).toBe(false)
  })
})
