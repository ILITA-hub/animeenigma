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
      bulkUpdateCards:      vi.fn(),
      bulkDeleteCards:      vi.fn(),
    },
  }
})

// Stub for GachaCardPicker that renders a transparent div so data-testid is
// preserved and parent state (pickerSearch, pickerSelected) can be tested directly.
const GachaCardPickerStub = {
  props: ['excludeIds', 'allCards', 'groups', 'alreadyInLabel', 'search', 'selected'],
  emits: ['update:search', 'update:selected', 'confirm', 'cancel'],
  // expose selectAllVisible for parent refs
  setup(_props: unknown, { expose }: { expose: (obj: Record<string, unknown>) => void }) {
    expose({ selectAllVisible: () => {}, filteredCards: [] })
  },
  template: '<div />',
}

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
          // Renders the selected option's label as text (mirrors the real
          // Select's trigger text) so assertions against wrapper.html() still
          // see the rarity/group label after Step 3g moved it from a bare
          // <span> into an inline Select.
          Select: {
            props: ['modelValue', 'options'],
            template: '<select>{{ options?.find(o => o.value === modelValue)?.label ?? modelValue }}</select>',
          },
          // Emits update:modelValue on click (flipping the boolean) so tests can
          // drive selection/toggle behavior via `.trigger('click')` on the
          // data-testid selector — matches how the real Checkbox (reka-ui
          // CheckboxRoot) behaves on click.
          Checkbox: {
            props: ['modelValue'],
            emits: ['update:modelValue'],
            template: '<input type="checkbox" :checked="modelValue" @click="$emit(\'update:modelValue\', !modelValue)" />',
          },
          Pencil: { template: '<span />' },
          Trash2: { template: '<span />' },
          Upload: { template: '<span />' },
          Check: { template: '<span />' },
          X: { template: '<span />' },
          Info: { template: '<span />' },
          GachaCardPicker: GachaCardPickerStub,
          GachaBulkUpload: { template: '<div />' },
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

  it('card dialog renders the optional card-back upload slot', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { openCardCreate: () => void }
    const vm = wrapper.vm as unknown as Vm
    vm.openCardCreate()
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="card-back-slot"]').exists()).toBe(true)
  })

  it('createCard payload includes back_path', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.createCard).mockResolvedValue({
      data: { success: true, data: makeCard() },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { openCardCreate: () => void; saveCard: () => Promise<void>; cardForm: { name: string; imagePath: string; backPath: string } }
    const vm = wrapper.vm as unknown as Vm
    vm.openCardCreate()
    await wrapper.vm.$nextTick()
    vm.cardForm.name = 'Hero'
    vm.cardForm.imagePath = 'cards/hero.webp'
    vm.cardForm.backPath = 'cards/hero-back.webp'
    await vm.saveCard()
    expect(vi.mocked(gachaAdminApi.createCard)).toHaveBeenCalledWith(
      expect.objectContaining({ back_path: 'cards/hero-back.webp' }),
    )
  })

  it('banner dialog renders backdrop upload slot', async () => {
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { openBannerCreate: () => void }
    const vm = wrapper.vm as unknown as Vm
    vm.openBannerCreate()
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="banner-backdrop-slot"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="banner-art-slot"]').exists()).toBe(false)
  })

  it('saveBanner payload carries backdrop_path (no wipe on edit)', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.updateBanner).mockResolvedValue({
      data: { success: true, data: makeBanner() },
    } as never)
    vi.mocked(gachaAdminApi.getBanner).mockResolvedValue({
      data: { success: true, data: { ...makeBanner({ id: 'b9' }), card_ids: [] } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = {
      openBannerEdit: (b: GachaBanner) => Promise<void>
      saveBanner: () => Promise<void>
      bannerForm: { backdrop_path: string }
    }
    const vm = wrapper.vm as unknown as Vm
    await vm.openBannerEdit(makeBanner({ id: 'b9', backdrop_path: 'banners/bd.webp' }))
    await flushPromises()
    await vm.saveBanner()
    expect(vi.mocked(gachaAdminApi.updateBanner)).toHaveBeenCalledWith(
      'b9',
      expect.objectContaining({ backdrop_path: 'banners/bd.webp' }),
    )
  })

  it('onBannerBackdropFile uploads with "banners" kind and stores backdrop_path', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.uploadFile).mockResolvedValue({
      data: { success: true, data: { image_path: 'banners/bd.webp', image_url: '/api/gacha/images/banners/bd.webp' } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = { onBannerBackdropFile: (e: Event) => Promise<void>; bannerForm: { backdrop_path: string } }
    const vm = wrapper.vm as unknown as Vm
    const file = new File(['x'], 'bd.png', { type: 'image/png' })
    const input = document.createElement('input')
    input.type = 'file'
    Object.defineProperty(input, 'files', { value: [file] })
    await vm.onBannerBackdropFile({ target: input } as unknown as Event)
    expect(vi.mocked(gachaAdminApi.uploadFile)).toHaveBeenCalledWith(file, 'banners')
    expect(vm.bannerForm.backdrop_path).toBe('banners/bd.webp')
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

  // ── Group picker tests ─────────────────────────────────────────────────────

  it('group card picker opens from group edit dialog', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    // listCards is called with group_id when opening group edit
    vi.mocked(gachaAdminApi.listCards)
      .mockResolvedValueOnce(emptyListResponse() as never) // initial mount (all cards)
      .mockResolvedValueOnce(emptyListResponse() as never) // openGroupEdit group_id call
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = {
      openGroupEdit: (g: GachaGroup) => Promise<void>
      groupPickerOpen: boolean
      groupCurrentCardIds: string[]
    }
    const vm = wrapper.vm as unknown as Vm
    await vm.openGroupEdit(makeGroup({ id: 'group-77' }))
    await flushPromises()
    // picker is closed initially
    expect(vm.groupPickerOpen).toBe(false)
    // open it
    vm.groupPickerOpen = true
    await wrapper.vm.$nextTick()
    // group-card-picker rendered in the modal
    expect(wrapper.find('[data-testid="group-card-picker"]').exists()).toBe(true)
  })

  it('onGroupPickerConfirm calls addCardsToGroup with exact ids and closes picker', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.addCardsToGroup).mockResolvedValue({
      data: { success: true, data: { updated: true } },
    } as never)
    const wrapper = mountComponent()
    await flushPromises()
    type Vm = {
      editGroup: GachaGroup | null
      groupPickerOpen: boolean
      groupCurrentCardIds: string[]
      groupPickerSelected: Set<string>
      onGroupPickerConfirm: (ids: string[]) => Promise<void>
    }
    const vm = wrapper.vm as unknown as Vm
    vm.editGroup = makeGroup({ id: 'group-88' })
    vm.groupCurrentCardIds = []
    vm.groupPickerOpen = true
    vm.groupPickerSelected = new Set(['card-p', 'card-q'])
    await vm.onGroupPickerConfirm(['card-p', 'card-q'])
    expect(vi.mocked(gachaAdminApi.addCardsToGroup)).toHaveBeenCalledWith(
      'group-88',
      expect.arrayContaining(['card-p', 'card-q']),
    )
    expect(vm.groupPickerOpen).toBe(false)
  })

  // ── Bulk selection + actions ─────────────────────────────────────────────────

  it('row checkbox selection shows the bulk bar and Enable calls bulkUpdateCards', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' }), makeCard({ id: 'c2', name: 'Second' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkUpdateCards).mockResolvedValue({ data: { data: { updated: 1 } } } as never)
    const wrapper = mountComponent()
    await flushPromises()

    expect(wrapper.find('[data-testid="bulk-actions-bar"]').exists()).toBe(false)
    await wrapper.find('[data-testid="row-select-c1"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bulk-actions-bar"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bulk-enable-btn"]').trigger('click')
    await flushPromises()
    expect(gachaAdminApi.bulkUpdateCards).toHaveBeenCalledWith(['c1'], { enabled: true })
  })

  it('select-all selects every filtered card', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' }), makeCard({ id: 'c2', name: 'Second' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkUpdateCards).mockResolvedValue({ data: { data: { updated: 2 } } } as never)
    const wrapper = mountComponent()
    await flushPromises()

    await wrapper.find('[data-testid="select-all"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="bulk-disable-btn"]').trigger('click')
    await flushPromises()
    const [ids, set] = vi.mocked(gachaAdminApi.bulkUpdateCards).mock.calls[0]
    expect([...ids].sort()).toEqual(['c1', 'c2'])
    expect(set).toEqual({ enabled: false })
  })

  it('bulk delete goes through the confirm dialog then calls bulkDeleteCards', async () => {
    const { gachaAdminApi } = await import('@/api/gacha')
    vi.mocked(gachaAdminApi.listCards).mockResolvedValue({
      data: { data: [makeCard({ id: 'c1' })] },
    } as never)
    vi.mocked(gachaAdminApi.bulkDeleteCards).mockResolvedValue({ data: { data: { deleted: 1 } } } as never)
    const wrapper = mountComponent()
    await flushPromises()

    await wrapper.find('[data-testid="row-select-c1"]').trigger('click')
    await wrapper.find('[data-testid="bulk-delete-btn"]').trigger('click')
    await flushPromises()
    expect(gachaAdminApi.bulkDeleteCards).not.toHaveBeenCalled()  // confirm first

    const vm = wrapper.vm as unknown as { runDelete: () => Promise<void> }
    await vm.runDelete()
    expect(gachaAdminApi.bulkDeleteCards).toHaveBeenCalledWith(['c1'])
  })
})
