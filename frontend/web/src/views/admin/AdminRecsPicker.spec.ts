import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminRecsPicker from './AdminRecsPicker.vue'
import { useAuthStore } from '@/stores/auth'

// RBAC-and-roulette Task 5: the picker no longer parses free text itself —
// it delegates to the shared UserResolveInput and only reacts to its
// `resolve` event, so we stub the child and drive the event directly.
const UserResolveInputStub = {
  name: 'UserResolveInput',
  props: ['mode'],
  emits: ['resolve'],
  template: '<div data-testid="user-resolve-input-stub" />',
}

const pushMock = vi.fn()
vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return { ...actual, useRouter: () => ({ push: pushMock }) }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})

function mountView() {
  return mount(AdminRecsPicker, {
    global: {
      stubs: { UserResolveInput: UserResolveInputStub },
      mocks: { $t: (k: string) => k },
    },
  })
}

describe('AdminRecsPicker', () => {
  beforeEach(() => {
    pushMock.mockClear()
    setActivePinia(createPinia())
  })

  it('renders the shared UserResolveInput in nav mode', () => {
    const w = mountView()
    const resolver = w.findComponent(UserResolveInputStub)
    expect(resolver.exists()).toBe(true)
    expect(resolver.props('mode')).toBe('nav')
  })

  it('navigates to /admin/recs/<uuid> when UserResolveInput resolves a user', async () => {
    const w = mountView()
    const resolver = w.findComponent(UserResolveInputStub)
    resolver.vm.$emit('resolve', {
      id: 'resolved-uuid-123',
      username: 'oronemu',
      public_id: 'oronemu-public',
    })
    await w.vm.$nextTick()
    expect(pushMock).toHaveBeenCalledWith('/admin/recs/resolved-uuid-123')
  })

  it('shows the self quick-action when an admin is logged in and navigates to their own id', async () => {
    const authStore = useAuthStore()
    authStore.user = {
      id: 'self-uuid-456',
      username: 'admin',
      email: 'a@a.com',
      role: 'admin',
    }

    const w = mountView()
    const selfButton = w.find('button')
    expect(selfButton.exists()).toBe(true)
    expect(selfButton.text()).toContain('admin.recs.pickerSelf')

    await selfButton.trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/admin/recs/self-uuid-456')
  })

  it('hides the self quick-action when no user is logged in', () => {
    const w = mountView()
    expect(w.find('button').exists()).toBe(false)
  })
})
