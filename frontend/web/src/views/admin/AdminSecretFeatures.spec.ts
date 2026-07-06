import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createPinia, setActivePinia } from 'pinia'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import AdminSecretFeatures from './AdminSecretFeatures.vue'
import { SECRET_FEATURES } from '@/utils/secretFeatures'

vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminApi: {
      getSecretFeatures: vi.fn(),
      setSecretRoulette: vi.fn(),
      setSecretFeature: vi.fn(),
    },
  }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

function configResponse(data: Record<string, unknown> = {}) {
  return { data: { success: true, data: { rouletteEnabled: true, features: {}, ...data } } }
}

const switchStub = {
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button data-testid="switch" @click="$emit(\'update:modelValue\', !modelValue)" />',
}

describe('AdminSecretFeatures', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
    const { adminApi } = await import('@/api/client')
    vi.mocked(adminApi.getSecretFeatures).mockResolvedValue(configResponse() as never)
    vi.mocked(adminApi.setSecretRoulette).mockResolvedValue(configResponse({ rouletteEnabled: false }) as never)
    vi.mocked(adminApi.setSecretFeature).mockResolvedValue(configResponse({ features: { anidle: false } }) as never)
  })

  function mountComponent() {
    return mount(AdminSecretFeatures, {
      global: {
        plugins: [i18n, pinia],
        stubs: {
          Spinner: { template: '<div data-testid="spinner" />' },
          Switch: switchStub,
          ExternalLink: true,
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })
  }

  it('loads the config on mount', async () => {
    const { adminApi } = await import('@/api/client')
    mountComponent()
    await flushPromises()
    expect(vi.mocked(adminApi.getSecretFeatures)).toHaveBeenCalledOnce()
  })

  it('renders one row per secret feature', async () => {
    const w = mountComponent()
    await flushPromises()
    expect(w.findAll('tbody tr').length).toBe(SECRET_FEATURES.length)
  })

  it('toggling the master switch calls setSecretRoulette', async () => {
    const { adminApi } = await import('@/api/client')
    const w = mountComponent()
    await flushPromises()
    // Switch order: [0] master, [1..] per-feature rows.
    await w.findAll('[data-testid="switch"]')[0].trigger('click')
    await flushPromises()
    expect(vi.mocked(adminApi.setSecretRoulette)).toHaveBeenCalledWith(false)
  })

  it('toggling a feature switch calls setSecretFeature with its key', async () => {
    const { adminApi } = await import('@/api/client')
    const w = mountComponent()
    await flushPromises()
    // First per-feature switch corresponds to SECRET_FEATURES[0] (anidle),
    // resolved-enabled by default ⇒ click emits false.
    await w.findAll('[data-testid="switch"]')[1].trigger('click')
    await flushPromises()
    expect(vi.mocked(adminApi.setSecretFeature)).toHaveBeenCalledWith(SECRET_FEATURES[0].key, false)
  })
})
