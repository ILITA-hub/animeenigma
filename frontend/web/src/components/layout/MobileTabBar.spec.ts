import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Ref } from 'vue'

// vi.hoisted runs before the vue import, so it can only hold plain values;
// the async mock factories below create the actual refs (they execute lazily
// at first import of the mocked module) and stash them here for the tests.
const h = vi.hoisted(() => ({
  refs: {} as { standalone?: unknown; isMobile?: unknown },
  isAuthenticated: true,
  back: vi.fn(),
  routePath: '/',
}))

vi.mock('@/pwa/standalone', async () => {
  const { ref } = await import('vue')
  const standalone = ref(true)
  h.refs.standalone = standalone
  return { useStandaloneDisplay: () => standalone }
})
vi.mock('@/composables/aePlayer/useMobilePlayer', async () => {
  const { ref } = await import('vue')
  const isMobile = ref(true)
  h.refs.isMobile = isMobile
  return { useMobilePlayer: () => ({ isMobile, isCoarse: ref(true) }) }
})
vi.mock('@/offline/flag', () => ({
  offlineDownloadsEnabled: true,
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return h.isAuthenticated
    },
  }),
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ path: h.routePath }),
  useRouter: () => ({ back: h.back }),
  RouterLink: {
    name: 'RouterLink',
    props: ['to'],
    template: '<a :href="to"><slot /></a>',
  },
}))

import MobileTabBar from './MobileTabBar.vue'

const standalone = () => h.refs.standalone as Ref<boolean>
const isMobile = () => h.refs.isMobile as Ref<boolean>

function mountBar() {
  return mount(MobileTabBar, {
    global: {
      mocks: { $t: (k: string) => k },
    },
  })
}

beforeEach(() => {
  standalone().value = true
  isMobile().value = true
  h.isAuthenticated = true
  h.routePath = '/'
  h.back.mockClear()
})

describe('MobileTabBar', () => {
  it('renders only in standalone + mobile', async () => {
    const w = mountBar()
    expect(w.find('[data-test="mobile-tabbar"]').exists()).toBe(true)

    standalone().value = false
    await w.vm.$nextTick()
    expect(w.find('[data-test="mobile-tabbar"]').exists()).toBe(false)

    standalone().value = true
    isMobile().value = false
    await w.vm.$nextTick()
    expect(w.find('[data-test="mobile-tabbar"]').exists()).toBe(false)
  })

  it('shows back/home/browse/downloads/profile tabs', () => {
    const w = mountBar()
    for (const key of ['back', 'home', 'browse', 'downloads', 'profile']) {
      expect(w.find(`[data-test="tab-${key}"]`).exists()).toBe(true)
    }
  })

  it('back button delegates to router.back', async () => {
    const w = mountBar()
    await w.find('[data-test="tab-back"]').trigger('click')
    expect(h.back).toHaveBeenCalledTimes(1)
  })

  it('profile tab targets /auth for anonymous users', async () => {
    h.isAuthenticated = false
    const w = mountBar()
    expect(w.find('[data-test="tab-profile"]').attributes('href')).toBe('/auth')
  })

  it('marks the active tab by route path', async () => {
    h.routePath = '/downloads'
    const w = mountBar()
    expect(w.find('[data-test="tab-downloads"]').classes()).toContain('tab-item--active')
    expect(w.find('[data-test="tab-home"]').classes()).not.toContain('tab-item--active')
  })
})
