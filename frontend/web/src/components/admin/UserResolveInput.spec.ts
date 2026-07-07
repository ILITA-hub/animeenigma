import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import ja from '@/locales/ja.json'
import UserResolveInput from './UserResolveInput.vue'

// Mirrors AdminSecretFeatures.spec.ts / useAdminFeedback.spec.ts: mock the
// shared apiClient's adminApi export, keep everything else real.
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminApi: {
      resolveUser: vi.fn(),
    },
  }
})

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru, ja } })

const MOCK_USER = {
  id: 'user-1',
  username: 'oronemu',
  public_id: 'oronemu-public',
  telegram_id: '123456789',
}

function mountComponent() {
  return mount(UserResolveInput, {
    global: { plugins: [i18n] },
  })
}

describe('UserResolveInput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('typing a value + Enter resolves the user, emits it, and clears the input', async () => {
    const { adminApi } = await import('@/api/client')
    vi.mocked(adminApi.resolveUser).mockResolvedValue({
      data: { success: true, data: MOCK_USER },
    } as never)

    const wrapper = mountComponent()
    const input = wrapper.find('input')
    await input.setValue('oronemu')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(vi.mocked(adminApi.resolveUser)).toHaveBeenCalledWith('oronemu')
    expect(wrapper.emitted('resolve')).toBeTruthy()
    expect(wrapper.emitted('resolve')![0]).toEqual([MOCK_USER])
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('')
  })

  it('shows the not-found message on 404 and does not emit', async () => {
    const { adminApi } = await import('@/api/client')
    vi.mocked(adminApi.resolveUser).mockRejectedValue({
      response: { status: 404 },
    } as never)

    const wrapper = mountComponent()
    const input = wrapper.find('input')
    await input.setValue('ghost-user')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.text()).toContain('No user found for "ghost-user"')
    expect(wrapper.emitted('resolve')).toBeFalsy()
  })

  it('issues no request when the input is empty and Enter is pressed', async () => {
    const { adminApi } = await import('@/api/client')
    const wrapper = mountComponent()
    const input = wrapper.find('input')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(vi.mocked(adminApi.resolveUser)).not.toHaveBeenCalled()
    expect(wrapper.emitted('resolve')).toBeFalsy()
  })

  it('also resolves via the add Button click (not just Enter)', async () => {
    const { adminApi } = await import('@/api/client')
    vi.mocked(adminApi.resolveUser).mockResolvedValue({
      data: { success: true, data: MOCK_USER },
    } as never)

    const wrapper = mountComponent()
    await wrapper.find('input').setValue('oronemu')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(vi.mocked(adminApi.resolveUser)).toHaveBeenCalledWith('oronemu')
    expect(wrapper.emitted('resolve')![0]).toEqual([MOCK_USER])
  })
})
