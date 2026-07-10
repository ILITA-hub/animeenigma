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

// Maintenance tab (Task 7) — useAdminMaintenance is mocked wholesale, mirroring
// useAdminProviders above.
const mockMaintenanceList = vi.fn()
const mockSetRoutine = vi.fn()

vi.mock('@/composables/useAdminMaintenance', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useAdminMaintenance')>()
  return {
    ...actual,
    useAdminMaintenance: () => ({ list: mockMaintenanceList, setRoutine: mockSetRoutine }),
  }
})

function maintRow(over: Record<string, unknown> = {}) {
  return {
    id: 'provider_recovery', enabled: true, settings: { model: 'sonnet' },
    lastRunAt: '2026-07-10T00:00:00Z', lastOk: true, lastSummary: 'adopted okru · exit 0',
    nextRunAt: null, updatedAt: '2026-07-10T00:00:00Z', ...over,
  }
}

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
    mockMaintenanceList.mockResolvedValue([maintRow()])
    mockSetRoutine.mockResolvedValue(undefined)
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

  it('renders one card per non-master flag with its label + derived audience badge', async () => {
    const w = mountComponent()
    await flushPromises()

    const cards = w.findAll('[data-testid="flag-card"]')
    expect(cards.length).toBe(2)
    expect(cards[0].text()).toContain('Fanfic engine')
    expect(cards[0].find('[data-testid="audience-badge"]').text()).toBe('Admin only')
    expect(cards[1].text()).toContain('Anidle')
    expect(cards[1].find('[data-testid="audience-badge"]').text()).toBe('Everyone')
  })

  it('updates the derived audience badge live as role chips toggle', async () => {
    const w = mountComponent()
    await flushPromises()

    // fanfic seeds roles:['admin'] -> "Admin only". Adding `everyone` should
    // immediately widen the badge to "Everyone" (no Save required — it mirrors
    // the mutable draft).
    const card = w.findAll('[data-testid="flag-card"]')[0]
    expect(card.find('[data-testid="audience-badge"]').text()).toBe('Admin only')
    await card.find('[data-testid="role-chip-fanfic-everyone"]').trigger('click')
    expect(card.find('[data-testid="audience-badge"]').text()).toBe('Everyone')
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

  it('toggling the roulette role chip + Save includes roulette in the setFlag payload', async () => {
    const w = mountComponent()
    await flushPromises()

    // anidle seeds roulette:true — toggling the chip flips it to false.
    await w.find('[data-testid="roulette-chip-anidle"]').trigger('click')
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

  it('evaluates a selected user by their REAL role (fixes the by-user no-op); clearing reverts to the identity control', async () => {
    const ADMIN_UUID = '33333333-3333-3333-3333-333333333333'
    const flagAdminOnly: FeatureFlag = {
      key: 'adminFlag',
      roles: ['admin'],
      allowUsers: [],
      denyUsers: [],
      roulette: false,
      failSafe: 'admin',
      label: 'Admin Flag',
      updatedAt: '2026-07-01T00:00:00Z',
    }
    mockList.mockResolvedValue({ flags: [flagAdminOnly], rouletteEnabled: true })

    const w = mountComponent()
    await flushPromises()

    // Identity control is on its 'anonymous' default, no user picked → the
    // admin-only flag reads Hidden.
    expect(w.find('[data-testid="preview-result-adminFlag"]').text()).toContain('Hidden')

    // Resolve an ADMIN user WITHOUT touching the identity control. The old code
    // dropped the userID under the anonymous default so this stayed Hidden (the
    // reported bug); now the user's real role grants access → Visible.
    const resolveInputs = w.findAllComponents({ name: 'UserResolveInput' })
    await resolveInputs[resolveInputs.length - 1]!.vm.$emit('resolve', {
      id: ADMIN_UUID,
      username: 'AdminX',
      public_id: 'adminx',
      role: 'admin',
    })
    await flushPromises()
    expect(w.find('[data-testid="preview-result-adminFlag"]').text()).toContain('Visible')

    // Clearing the user reverts to the (anonymous) identity control → Hidden.
    await w.find('[data-testid="preview-user-chip"] [data-testid="chip-remove"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="preview-result-adminFlag"]').text()).toContain('Hidden')
  })

  it('a selected user still loses to the deny-list, and a non-admin cannot reach an admin-only flag', async () => {
    const UUID = '44444444-4444-4444-4444-444444444444'
    const flags: FeatureFlag[] = [
      {
        key: 'adminFlag',
        roles: ['admin'],
        allowUsers: [],
        denyUsers: [],
        roulette: false,
        failSafe: 'admin',
        label: 'Admin Flag',
        updatedAt: '2026-07-01T00:00:00Z',
      },
      {
        key: 'denyFlag',
        roles: ['everyone'],
        allowUsers: [],
        denyUsers: [UUID],
        roulette: false,
        failSafe: 'everyone',
        label: 'Deny Flag',
        updatedAt: '2026-07-01T00:00:00Z',
      },
    ]
    mockList.mockResolvedValue({ flags, rouletteEnabled: true })

    const w = mountComponent()
    await flushPromises()

    // Resolve a regular USER who is on denyFlag's deny list.
    const resolveInputs = w.findAllComponents({ name: 'UserResolveInput' })
    await resolveInputs[resolveInputs.length - 1]!.vm.$emit('resolve', {
      id: UUID,
      username: 'UserX',
      public_id: 'userx',
      role: 'user',
    })
    await flushPromises()

    // admin-only flag: role 'user' doesn't grant → Hidden.
    expect(w.find('[data-testid="preview-result-adminFlag"]').text()).toContain('Hidden')
    // everyone flag but the user is deny-listed → deny wins → Hidden.
    expect(w.find('[data-testid="preview-result-denyFlag"]').text()).toContain('Hidden')
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
      // The provider machine name is always rendered (was previously buried when
      // description held long operator prose — case-sensitive so it can't match
      // the 'GogoAnime' description).
      expect(cards[0].text()).toContain('gogoanime')
      expect(w.find('[data-testid="provider-state-gogoanime"]').text()).toContain('Up')
      expect(cards[1].text()).toContain('Miruro')
      expect(cards[1].text()).toContain('miruro')
      expect(w.find('[data-testid="provider-state-miruro"]').text()).toContain('Down')
      // The reason surfaces for the down provider.
      expect(cards[1].text()).toContain('circuit open')
    })

    it('selecting "disabled" opens the confirm dialog and, on confirm, calls setPolicy(name, "disabled")', async () => {
      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-policy-gogoanime"] [data-value="disabled"]').trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(mockSetPolicy).toHaveBeenCalledWith('gogoanime', 'disabled')
    })

    it('does not call setPolicy when the disable confirm is cancelled', async () => {
      confirmMock.mockResolvedValueOnce(false)
      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-policy-gogoanime"] [data-value="disabled"]').trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(mockSetPolicy).not.toHaveBeenCalled()
    })

    it('selecting "auto" on a disabled provider calls setPolicy(name, "auto") with no confirm', async () => {
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

      await w.find('[data-testid="provider-policy-nineanime"] [data-value="auto"]').trigger('click')
      await flushPromises()

      expect(confirmMock).not.toHaveBeenCalled()
      expect(mockSetPolicy).toHaveBeenCalledWith('nineanime', 'auto')
    })

    // The old binary facade rendered `policy !== 'disabled'` as "Auto", so a
    // manual (admin-parked) provider FALSELY read Auto — the segmented control
    // must mark the manual segment active, not auto.
    it('a policy="manual" row renders with the Manual segment active (NOT Auto)', async () => {
      const manualProvider: ScraperProviderWire = {
        ...providerUp,
        name: 'miruro',
        description: 'Miruro',
        policy: 'manual',
        status: 'degraded',
        derived_state: 'Disabled',
      }
      mockProvidersList.mockResolvedValue([manualProvider])

      const w = await mountAndOpenProvidersTab()

      const control = w.find('[data-testid="provider-policy-miruro"]')
      expect(control.find('[data-value="manual"]').attributes('aria-checked')).toBe('true')
      expect(control.find('[data-value="auto"]').attributes('aria-checked')).toBe('false')
    })

    it('selecting "manual" calls setPolicy(name, "manual") directly — no confirm', async () => {
      mockSetPolicy.mockResolvedValue({
        ...providerUp,
        policy: 'manual',
        status: 'degraded',
        derived_state: 'Disabled',
      })

      const w = await mountAndOpenProvidersTab()

      await w.find('[data-testid="provider-policy-gogoanime"] [data-value="manual"]').trigger('click')
      await flushPromises()

      expect(confirmMock).not.toHaveBeenCalled()
      expect(mockSetPolicy).toHaveBeenCalledWith('gogoanime', 'manual')
    })

    it('re-selecting the current policy is a no-op (no setPolicy call)', async () => {
      const w = await mountAndOpenProvidersTab()

      // gogoanime seeds policy:'auto' — clicking Auto again does nothing.
      await w.find('[data-testid="provider-policy-gogoanime"] [data-value="auto"]').trigger('click')
      await flushPromises()

      expect(confirmMock).not.toHaveBeenCalled()
      expect(mockSetPolicy).not.toHaveBeenCalled()
    })
  })

  // ─── MAINTENANCE TAB (Task 7) ────────────────────────────────────────────
  describe('Maintenance tab', () => {
    async function mountAndOpenMaintenanceTab() {
      const w = mountComponent()
      await flushPromises()
      await w.find('#tab-maintenance').trigger('click')
      await flushPromises()
      return w
    }

    it('renders a card per routine from the composable', async () => {
      mockMaintenanceList.mockResolvedValue([
        maintRow({ lastRunAt: new Date().toISOString() }),
        maintRow({ id: 'git_autosync', settings: {}, lastRunAt: new Date().toISOString() }),
      ])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.findAll('[data-testid="routine-card"]').length).toBe(2)
    })

    it('confirms before pausing and calls setRoutine with enabled=false', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      confirmMock.mockResolvedValue(true)
      const w = await mountAndOpenMaintenanceTab()
      // Switch is stubbed as a <button> that emits update:modelValue(!current).
      await w.find('[data-testid="routine-switch-provider_recovery"]').trigger('click')
      await flushPromises()
      expect(confirmMock).toHaveBeenCalled()
      expect(mockSetRoutine).toHaveBeenCalledWith('provider_recovery', expect.objectContaining({ enabled: false }))
    })

    it('does NOT pause when confirm is declined', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      confirmMock.mockResolvedValue(false)
      const w = await mountAndOpenMaintenanceTab()
      await w.find('[data-testid="routine-switch-provider_recovery"]').trigger('click')
      await flushPromises()
      expect(mockSetRoutine).not.toHaveBeenCalled()
    })

    it('maps a failed last run to the destructive status badge', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastOk: false, lastRunAt: new Date().toISOString() })])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.find('[data-testid="routine-status-provider_recovery"]').text()).toContain('Failed')
    })

    it('disables Save until a knob changes', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.find('[data-testid="routine-save-provider_recovery"]').attributes('disabled')).toBeDefined()
    })

    it('toggle persists SAVED settings, not the unsaved knob draft (decoupled from Save)', async () => {
      // provider_recovery seeds settings.model:'sonnet' + a `model` select knob.
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      confirmMock.mockResolvedValue(true)
      const w = await mountAndOpenMaintenanceTab()

      // Edit the Model SegmentedControl sonnet -> opus WITHOUT clicking Save.
      // The rendered option button carries data-value; clicking it emits
      // update:modelValue('opus') into row.draft.model.
      await w.find('[data-testid="routine-card"] [data-value="opus"]').trigger('click')
      await flushPromises()
      // The edit made the knob dirty (Save now enabled) — proves the draft moved.
      expect(w.find('[data-testid="routine-save-provider_recovery"]').attributes('disabled')).toBeUndefined()

      // Now pause via the Switch (confirm -> true). The toggle must send the
      // SAVED model ('sonnet'), NOT the unsaved draft ('opus').
      await w.find('[data-testid="routine-switch-provider_recovery"]').trigger('click')
      await flushPromises()

      expect(mockSetRoutine).toHaveBeenCalledWith('provider_recovery', {
        enabled: false,
        settings: { model: 'sonnet' },
      })
    })
  })
})
