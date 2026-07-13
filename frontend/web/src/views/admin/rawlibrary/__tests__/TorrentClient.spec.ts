import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TorrentClient from '@/views/admin/rawlibrary/TorrentClient.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: vi.fn() }) }))
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminLibraryApi: {
      ...actual.adminLibraryApi,
      healthExtended: vi.fn().mockResolvedValue({ data: { success: true, data: {
        disk_free_bytes: 500, disk_total_bytes: 1000, active_torrents: 2, active_jobs_by_status: { downloading: 1 },
      } } }),
      listJobs: vi.fn().mockResolvedValue({ data: { success: true, data: { jobs: [] } } }),
    },
  }
})

afterEach(() => vi.clearAllMocks())

describe('TorrentClient.vue', () => {
  it('renders the stats strip from healthExtended', async () => {
    // $t (template global, injected by the real i18n plugin) isn't covered by
    // the useI18n() mock above — mirrors the fix in RawLibrary.files.spec.ts.
    const wrapper = mount(TorrentClient, {
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    // 500/1000 disk free = 50.0%
    expect(wrapper.text()).toContain('50.0')
    expect(wrapper.text()).toContain('player.adminLibrary.stats.activeTorrents')
    wrapper.unmount()
  })
})
