import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'
import RawLibrary from '@/views/admin/RawLibrary.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
// Stub both panels so the host test doesn't pull their api/polling.
vi.mock('@/views/admin/rawlibrary/TorrentClient.vue', () => ({
  default: { name: 'TorrentClient', template: '<div class="tc-stub" />' },
}))
vi.mock('@/views/admin/rawlibrary/FileManager.vue', () => ({
  default: {
    name: 'FileManager',
    props: ['backend', 'prefix'],
    emits: ['navigate'],
    template: '<div class="fm-stub">{{ backend }}|{{ prefix }}</div>',
  },
}))

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/raw-library', name: 'admin-raw-library', component: RawLibrary },
      { path: '/admin/raw-library/file-manager/:backend/:filepath(.*)*', name: 'admin-raw-library-files', component: RawLibrary },
    ],
  })
}

afterEach(() => vi.clearAllMocks())

describe('RawLibrary host', () => {
  it('defaults to the Torrent Client tab at the base route', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router], mocks: { $t: (k: string) => k } } })
    await flushPromises()
    expect(wrapper.find('.tc-stub').exists()).toBe(true)
    expect(wrapper.find('.fm-stub').exists()).toBe(false)
  })

  it('deep-links a folder into the File Manager tab', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library/file-manager/minio/aeProvider/11981/RAW'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router], mocks: { $t: (k: string) => k } } })
    await flushPromises()
    expect(wrapper.find('.fm-stub').text()).toBe('minio|aeProvider/11981/RAW/')
  })

  it('pushes the file-manager route when a FileManager navigate is emitted', async () => {
    const router = makeRouter()
    router.push('/admin/raw-library/file-manager/minio'); await router.isReady()
    const wrapper = mount(RawLibrary, { global: { plugins: [router], mocks: { $t: (k: string) => k } } })
    await flushPromises()
    wrapper.findComponent({ name: 'FileManager' }).vm.$emit('navigate', { backend: 's3', prefix: 'aeProvider/4901/' })
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/raw-library/file-manager/s3/aeProvider/4901')
  })
})
