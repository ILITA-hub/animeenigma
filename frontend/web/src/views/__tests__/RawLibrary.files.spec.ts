/**
 * Task 8 — Files section of RawLibrary.vue (admin file manager: torrent work
 * dir + MinIO + S3 browse/download/delete, add-torrent-by-hand).
 *
 * Mocks @/api/client (adminLibraryApi) and @/composables/useConfirm so the
 * confirm dialog resolves deterministically without mounting the real
 * <ConfirmDialogHost>. vue-i18n is mocked via the importOriginal-spread
 * pattern (see AdminRecs.spec.ts / AdminGacha.spec.ts) — a bare `vi.mock`
 * factory without spreading the real module crashes createI18n when the
 * component also pulls in the `@/components/ui` barrel (project trap).
 */
import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import RawLibrary from '@/views/admin/RawLibrary.vue'
import { adminLibraryApi } from '@/api/client'
import type { BrowseResponse } from '@/types/library'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})

const { confirmMock } = vi.hoisted(() => ({ confirmMock: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminLibraryApi: {
      ...actual.adminLibraryApi,
      healthExtended: vi.fn().mockResolvedValue({ data: { success: true, data: {
        disk_free_bytes: 0, disk_total_bytes: 0, active_torrents: 0, active_jobs_by_status: {},
      } } }),
      listJobs: vi.fn().mockResolvedValue({ data: { success: true, data: { jobs: [] } } }),
      browseFiles: vi.fn(),
      deleteFile: vi.fn().mockResolvedValue({ data: { success: true, data: { deleted: true } } }),
      downloadFile: vi.fn(),
      createJob: vi.fn(),
    },
  }
})

// Cast `as never` at call sites — mocking axios's full AxiosResponse shape
// (status/statusText/headers/config) isn't worth it for a body the component
// only ever reads via `.data`; matches the project's established mock pattern
// (see AdminGacha.spec.ts).
function browseResponse(overrides: Partial<BrowseResponse> = {}): { data: { success: true, data: BrowseResponse } } {
  return {
    data: {
      success: true,
      data: {
        domain: 'minio',
        prefix: '',
        breadcrumb: [],
        entries: [
          { name: 'season1', kind: 'dir', size: 2048 },
          { name: 'episode01.mp4', kind: 'file', size: 1024, key: 'episode01.mp4' },
        ],
        ...overrides,
      },
    },
  }
}

let wrapper: VueWrapper | null = null

async function mountView() {
  wrapper = mount(RawLibrary, {
    global: {
      mocks: { $t: (k: string) => k },
    },
  })
  await flushPromises()
  return wrapper
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('RawLibrary — Files section', () => {
  it('renders the browsed entries (dir + file)', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)

    const w = await mountView()

    expect(adminLibraryApi.browseFiles).toHaveBeenCalledWith('minio', '')
    const filesSection = w.get('[aria-label="files"]')
    expect(filesSection.text()).toContain('season1')
    expect(filesSection.text()).toContain('episode01.mp4')
  })

  it('clicking a directory browses into it with the new prefix', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const dirButton = filesSection.findAll('button').find((b) => b.text().includes('season1'))
    expect(dirButton).toBeTruthy()

    await dirButton!.trigger('click')
    await flushPromises()

    expect(adminLibraryApi.browseFiles).toHaveBeenLastCalledWith('minio', 'season1/')
  })

  it('clicking Delete on a file confirms then calls deleteFile with the entry key', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)
    confirmMock.mockResolvedValue(true)

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const deleteButtons = filesSection.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    // Only the file row renders a Delete button next to Download; the dir row
    // also gets one (Delete applies to both kinds) — target the last (file) row.
    expect(deleteButtons.length).toBeGreaterThan(0)
    await deleteButtons[deleteButtons.length - 1].trigger('click')
    await flushPromises()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(adminLibraryApi.deleteFile).toHaveBeenCalledWith('minio', 'episode01.mp4', true)
  })

  it('does not delete when the confirm dialog is cancelled', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)
    confirmMock.mockResolvedValue(false)

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const deleteButtons = filesSection.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    await deleteButtons[deleteButtons.length - 1].trigger('click')
    await flushPromises()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(adminLibraryApi.deleteFile).not.toHaveBeenCalled()
  })

  it('renders the empty state when a domain has no entries', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse({ entries: [] }) as never)

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    expect(filesSection.text()).toContain('player.adminLibrary.files.empty')
  })

  it('sets the errorBanner to the episode-member message on a 409 with reason=episode_member', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)
    confirmMock.mockResolvedValue(true)
    vi.mocked(adminLibraryApi.deleteFile).mockRejectedValue({
      response: { status: 409, data: { reason: 'episode_member' } },
    })

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const deleteButtons = filesSection.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    await deleteButtons[deleteButtons.length - 1].trigger('click')
    await flushPromises()

    expect(w.text()).toContain('player.adminLibrary.files.error.episodeMember')
  })

  it('sets the errorBanner to the active-torrent message on a 409 with reason=torrent_active', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)
    confirmMock.mockResolvedValue(true)
    vi.mocked(adminLibraryApi.deleteFile).mockRejectedValue({
      response: { status: 409, data: { reason: 'torrent_active' } },
    })

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const deleteButtons = filesSection.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    await deleteButtons[deleteButtons.length - 1].trigger('click')
    await flushPromises()

    expect(w.text()).toContain('player.adminLibrary.files.error.active')
  })

  it('sets the errorBanner to a generic delete-failed message on a 500/network error, without throwing', async () => {
    vi.mocked(adminLibraryApi.browseFiles).mockResolvedValue(browseResponse() as never)
    confirmMock.mockResolvedValue(true)
    vi.mocked(adminLibraryApi.deleteFile).mockRejectedValue(new Error('network down'))
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    const w = await mountView()
    const filesSection = w.get('[aria-label="files"]')
    const deleteButtons = filesSection.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    await deleteButtons[deleteButtons.length - 1].trigger('click')
    await flushPromises()

    expect(w.text()).toContain('player.adminLibrary.files.error.deleteFailed')
    expect(warnSpy).toHaveBeenCalled()
    warnSpy.mockRestore()
  })
})
