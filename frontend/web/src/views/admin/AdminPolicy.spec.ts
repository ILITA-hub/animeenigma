import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import AdminPolicy from './AdminPolicy.vue'
import type { FeatureFlag } from '@/composables/useAdminPolicy'
import type { ScraperProviderWire } from '@/composables/useAdminProviders'

// RBAC-and-roulette P3, Task 7 — AdminPolicy.vue Features tab.
//
// useAdminPolicy (Task 6) is mocked wholesale: list()/setFlag()/setRoulette()
// are plain async functions, not reactive state, so the view owns all local
// state (draft roles/allowUsers/denyUsers/roulette per flag + dirty tracking).
const ALLOW_UUID = '11111111-1111-1111-1111-111111111111'

const flagAdmin: FeatureFlag = {
  key: 'fanfic',
  roles: ['admin'],
  allowUsers: [ALLOW_UUID],
  denyUsers: [],
  roulette: false,
  failSafe: 'admin',
  label: 'Fanfic engine',
  updatedAt: '2026-07-01T00:00:00Z',
}

const flagEveryone: FeatureFlag = {
  key: 'anidle',
  roles: ['everyone'],
  allowUsers: [],
  denyUsers: [],
  roulette: true,
  failSafe: 'everyone',
  label: 'Anidle',
  updatedAt: '2026-07-01T00:00:00Z',
}

const providerUp: ScraperProviderWire = {
  name: 'gogoanime',
  status: 'enabled',
  policy: 'auto',
  health: 'up',
  health_since: '2026-07-01T00:00:00Z',
  policy_since: '2026-07-01T00:00:00Z',
  last_probed_at: '2026-07-07T00:00:00Z',
  group: 'en',
  reason: '',
  description: 'GogoAnime',
  scraper_operated: true,
  supports_sub: true,
  supports_dub: true,
  supports_raw: false,
  sub_delivery: 'embedded',
  quality_ceiling: '1080p',
  preference_weight: 10,
  engine: 'browser',
  base_url: 'https://gogoanime.example',
  last_tick_metrics: '',
  updated_at: '2026-07-07T00:00:00Z',
  derived_state: 'UP',
}

const providerDown: ScraperProviderWire = {
  ...providerUp,
  name: 'miruro',
  description: 'Miruro',
  health: 'down',
  reason: 'circuit open',
  derived_state: 'Down',
}

const mockList = vi.fn()
const mockSetFlag = vi.fn()
const mockSetRoulette = vi.fn()

vi.mock('@/composables/useAdminPolicy', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useAdminPolicy')>()
  return {
    ...actual,
    useAdminPolicy: () => ({
      list: mockList,
      setFlag: mockSetFlag,
      setRoulette: mockSetRoulette,
    }),
  }
})

// P5 Task 2 — Providers tab. useAdminProviders is mocked wholesale (mirrors
// useAdminPolicy above); useConfirm is hoisted so the mock factory (which
// vi.mock hoists above these const declarations) can close over it.
const mockProvidersList = vi.fn()
const mockSetPolicy = vi.fn()

vi.mock('@/composables/useAdminProviders', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useAdminProviders')>()
  return {
    ...actual,
    useAdminProviders: () => ({
      list: mockProvidersList,
      setPolicy: mockSetPolicy,
    }),
  }
})

const { confirmMock } = vi.hoisted(() => ({ confirmMock: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminApi: {
      ...actual.adminApi,
      resolveUser: vi.fn(),
    },
  }
})

// useOpenFeature() (Task 6) calls useRouter() internally for its standalone-PWA
// click intercept; the open-link itself is a plain <a>, so a bare useRouter
// stub is enough — no RouterLink/router plugin needed anywhere in this tree.
vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRouter: () => ({ push: vi.fn() }) }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

// Fallthrough-attrs stub so real-looking test hooks (data-testid, aria-label)
// land on the rendered <button>, mirroring AdminSecretFeatures.spec.ts.
const switchStub = {
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button v-bind="$attrs" @click="$emit(\'update:modelValue\', !modelValue)" />',
}

// Not clicked in these assertions (none of the 5 scenarios need to resolve a
// new user), but must render without exploding — mirrors AdminRecsPicker.spec.ts.
const userResolveInputStub = {
  name: 'UserResolveInput',
  props: ['mode'],
  emits: ['resolve'],
  template: '<div data-testid="user-resolve-stub" />',
}

function mountComponent() {
  return mount(AdminPolicy, {
    global: {
      plugins: [i18n],
      stubs: {
        Switch: switchStub,
        UserResolveInput: userResolveInputStub,
        ExternalLink: true,
      },
    },
  })
}

describe('AdminPolicy', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mockList.mockResolvedValue({ flags: [flagAdmin, flagEveryone], rouletteEnabled: true })
    mockSetFlag.mockResolvedValue(undefined)
    mockSetRoulette.mockResolvedValue(undefined)
    mockProvidersList.mockResolvedValue([providerUp, providerDown])
    mockSetPolicy.mockResolvedValue({ ...providerUp, policy: 'disabled', derived_state: 'Disabled' })
    confirmMock.mockResolvedValue(true)

    const { adminApi } = await import('@/api/client')
    vi.mocked(adminApi.resolveUser).mockImplementation((q: string) => {
      if (q === ALLOW_UUID) {
        return Promise.resolve({
          data: { success: true, data: { id: ALLOW_UUID, username: 'Alice', public_id: 'alice' } },
        }) as never
      }
      return Promise.reject({ response: { status: 404 } })
    })
  })

  it('renders one card per non-master flag with its label + failSafe badge', async () => {
    const w = mountComponent()
    await flushPromises()

    const cards = w.findAll('[data-testid="flag-card"]')
    expect(cards.length).toBe(2)
    expect(cards[0].text()).toContain('Fanfic engine')
    expect(cards[0].find('[data-testid="failsafe-badge"]').text()).toBe('Admin-only')
    expect(cards[1].text()).toContain('Anidle')
    expect(cards[1].find('[data-testid="failsafe-badge"]').text()).toBe('Everyone')
  })

  it('toggling a role chip and clicking Save calls setFlag with the mutated roles', async () => {
    const w = mountComponent()
    await flushPromises()

    await w.find('[data-testid="role-chip-fanfic-everyone"]').trigger('click')
    await w.find('[data-testid="save-button-fanfic"]').trigger('click')
    await flushPromises()

    expect(mockSetFlag).toHaveBeenCalledWith('fanfic', expect.objectContaining({
      roles: ['admin', 'everyone'],
      allowUsers: [ALLOW_UUID],
      denyUsers: [],
      roulette: false,
      failSafe: 'admin',
      label: 'Fanfic engine',
    }))
  })

  it('toggling the master switch calls setRoulette', async () => {
    const w = mountComponent()
    await flushPromises()

    await w.find('[data-testid="master-roulette-switch"]').trigger('click')
    await flushPromises()

    expect(mockSetRoulette).toHaveBeenCalledWith(false)
  })

  it('toggling a per-flag roulette Switch + Save includes roulette in the setFlag payload', async () => {
    const w = mountComponent()
    await flushPromises()

    await w.find('[data-testid="roulette-switch-anidle"]').trigger('click')
    await w.find('[data-testid="save-button-anidle"]').trigger('click')
    await flushPromises()

    expect(mockSetFlag).toHaveBeenCalledWith('anidle', expect.objectContaining({ roulette: false }))
  })

  it('access preview marks the everyone flag visible for an anonymous identity and the admin-only flag hidden', async () => {
    const w = mountComponent()
    await flushPromises()

    expect(w.find('[data-testid="preview-result-anidle"]').text()).toContain('Visible')
    expect(w.find('[data-testid="preview-result-fanfic"]').text()).toContain('Hidden')
  })

  it('ignores a stale selected target user once the preview identity is switched back to anonymous', async () => {
    const DENY_UUID = '22222222-2222-2222-2222-222222222222'
    const flagEveryoneDenyX: FeatureFlag = {
      key: 'previewDeny',
      roles: ['everyone'],
      allowUsers: [],
      denyUsers: [DENY_UUID],
      roulette: false,
      failSafe: 'everyone',
      label: 'Preview Deny Target',
      updatedAt: '2026-07-01T00:00:00Z',
    }
    mockList.mockResolvedValue({ flags: [flagEveryoneDenyX], rouletteEnabled: true })

    const w = mountComponent()
    await flushPromises()

    // Switch off anonymous and select a target user that is on the deny list —
    // deny wins, so the flag should read Hidden for that identity+user pair.
    await w.find('[data-value="user"]').trigger('click')
    const resolveInputs = w.findAllComponents({ name: 'UserResolveInput' })
    await resolveInputs[resolveInputs.length - 1]!.vm.$emit('resolve', {
      id: DENY_UUID,
      username: 'UserX',
      public_id: 'userx',
    })
    await flushPromises()
    expect(w.find('[data-testid="preview-result-previewDeny"]').text()).toContain('Hidden')

    // Switch back to anonymous — a real anonymous request carries no userID,
    // so the stale target-user selection must be ignored and the
    // roles:['everyone'] flag must read Visible, not Hidden.
    await w.find('[data-value="anonymous"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="preview-result-previewDeny"]').text()).toContain('Visible')
  })

  // ─── PROVIDERS TAB (P5 Task 2) ──────────────────────────────────────────
  describe('Providers tab', () => {
    async function mountAndOpenProvidersTab() {
      const w = mountComponent()
      await flushPromises()
      await w.find('#tab-providers').trigger('click')
      await flushPromises()
      return w
    }

    it('renders one row per provider with a status pill from derived_state', async () => {
      const w = await mountAndOpenProvidersTab()

      const cards = w.findAll('[data-testid="provider-card"]')
      expect(cards.length).toBe(2)
      expect(cards[0].text()).toContain('GogoAnime')
      expect(w.find('[data-testid="provider-state-gogoanime"]').text()).toContain('Up')
      expect(cards[1].text()).toContain('Miruro')
      expect(w.find('[data-testid="provider-state-miruro"]').text()).toContain('Down')
      // The reason surfaces for the down provider.
      expect(cards[1].text()).toContain('circuit open')
    })

    it('toggling a provider OFF opens the confirm dialog and, on confirm, calls setPolicy(name, "disabled")', async () => {
      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-switch-gogoanime"]').trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(mockSetPolicy).toHaveBeenCalledWith('gogoanime', 'disabled')
    })

    it('does not call setPolicy when the disable confirm is cancelled', async () => {
      confirmMock.mockResolvedValueOnce(false)
      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-switch-gogoanime"]').trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(mockSetPolicy).not.toHaveBeenCalled()
    })

    it('toggling a disabled provider ON calls setPolicy(name, "auto") with no confirm', async () => {
      const disabledProvider: ScraperProviderWire = {
        ...providerUp,
        name: 'nineanime',
        description: 'NineAnime',
        policy: 'disabled',
        derived_state: 'Disabled',
      }
      mockProvidersList.mockResolvedValue([disabledProvider])
      mockSetPolicy.mockResolvedValue({ ...disabledProvider, policy: 'auto', derived_state: 'UP' })

      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-switch-nineanime"]').trigger('click')
      await flushPromises()

      expect(confirmMock).not.toHaveBeenCalled()
      expect(mockSetPolicy).toHaveBeenCalledWith('nineanime', 'auto')
    })
  })
})
