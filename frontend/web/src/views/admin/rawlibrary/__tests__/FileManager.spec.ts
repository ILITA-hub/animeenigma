import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import FileManager, { resetTitleCache } from '@/views/admin/rawlibrary/FileManager.vue'
import { adminLibraryApi } from '@/api/client'
import type { BrowseResponse } from '@/types/library'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})
const { confirmMock } = vi.hoisted(() => ({ confirmMock: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    adminLibraryApi: {
      ...actual.adminLibraryApi,
      browseFiles: vi.fn(),
      deleteFile: vi.fn().mockResolvedValue({ data: { success: true, data: { deleted: true } } }),
      downloadFile: vi.fn(),
    },
    animeApi: { ...actual.animeApi, resolveShikimori: vi.fn() },
  }
})

function browseResponse(overrides: Partial<BrowseResponse> = {}) {
  return { data: { success: true, data: {
    domain: 'minio', prefix: '', breadcrumb: [], entries: [
      { name: 'season1', kind: 'dir', size: 2048 },
      { name: 'episode01.mp4', kind: 'file', size: 1024, key: 'episode01.mp4' },
    ], ...overrides,
  } } }
}

afterEach(() => { vi.clearAllMocks(); confirmMock.mockReset(); resetTitleCache() })

async function mountFM(prefix = ''): Promise<VueWrapper> {
  (adminLibraryApi.browseFiles as ReturnType<typeof vi.fn>).mockResolvedValue(browseResponse({ prefix }))
  const wrapper = mount(FileManager, {
    props: { backend: 'minio', prefix },
    global: { mocks: { $t: (k: string) => k } },
  })
  await flushPromises()
  return wrapper
}

describe('FileManager.vue', () => {
  it('browses the backend/prefix from props', async () => {
    const wrapper = await mountFM('')
    expect(adminLibraryApi.browseFiles).toHaveBeenCalledWith('minio', '')
    expect(wrapper.text()).toContain('season1')
    expect(wrapper.text()).toContain('episode01.mp4')
    wrapper.unmount()
  })

  it('emits navigate when opening a folder', async () => {
    const wrapper = await mountFM('')
    await wrapper.findAll('button').find((b) => b.text().includes('season1'))!.trigger('click')
    expect(wrapper.emitted('navigate')![0]).toEqual([{ backend: 'minio', prefix: 'season1/' }])
    wrapper.unmount()
  })

  it('has NO add-torrent-by-hand form', async () => {
    const wrapper = await mountFM('')
    expect(wrapper.text()).not.toContain('player.adminLibrary.files.add')
    expect(wrapper.find('input[placeholder*="magnet" i]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows ../ only when not at root and navigates up one level', async () => {
    const atRoot = await mountFM('')
    expect(atRoot.text()).not.toContain('player.adminLibrary.files.parent')
    atRoot.unmount()

    const nested = await mountFM('aeProvider/11981/RAW/')
    const up = nested.findAll('button').find((b) => b.text().includes('player.adminLibrary.files.parent'))
    expect(up).toBeTruthy()
    await up!.trigger('click')
    expect(nested.emitted('navigate')![0]).toEqual([{ backend: 'minio', prefix: 'aeProvider/11981/' }])
    nested.unmount()
  })

  it('resolves anime titles for numeric folders under aeProvider/', async () => {
    (adminLibraryApi.browseFiles as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { success: true, data: { domain: 'minio', prefix: 'aeProvider/', breadcrumb: ['aeProvider'],
        entries: [{ name: '11981', kind: 'dir', size: 42 }, { name: 'pending', kind: 'dir', size: 1 }] } },
    })
    const { animeApi } = await import('@/api/client')
    ;(animeApi.resolveShikimori as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { success: true, data: { name: 'Frieren', name_ru: 'Фрирен' } },
    })
    const wrapper = mount(FileManager, { props: { backend: 'minio', prefix: 'aeProvider/' } , global: { mocks: { $t: (k: string) => k } } })
    await flushPromises(); await flushPromises()
    expect(animeApi.resolveShikimori).toHaveBeenCalledWith('11981')
    expect(animeApi.resolveShikimori).not.toHaveBeenCalledWith('pending') // non-numeric skipped
    expect(wrapper.text()).toContain('Frieren')
    wrapper.unmount()
  })

  it('shows both the ../ row and the empty label in an empty non-root dir', async () => {
    (adminLibraryApi.browseFiles as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { success: true, data: {
        domain: 'minio', prefix: 'aeProvider/11981/', breadcrumb: ['aeProvider', '11981'], entries: [],
      } },
    })
    const wrapper = mount(FileManager, {
      props: { backend: 'minio', prefix: 'aeProvider/11981/' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('player.adminLibrary.files.parent')
    expect(wrapper.text()).toContain('player.adminLibrary.files.empty')
    wrapper.unmount()
  })

  describe('delete', () => {
    function deleteButtons(wrapper: VueWrapper) {
      return wrapper.findAll('button').filter((b) => b.text() === 'player.adminLibrary.files.delete')
    }

    it('deletes a file after confirm and reloads the list', async () => {
      confirmMock.mockResolvedValue(true)
      const wrapper = await mountFM('')
      const buttons = deleteButtons(wrapper)
      await buttons[buttons.length - 1].trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(adminLibraryApi.deleteFile).toHaveBeenCalledWith('minio', 'episode01.mp4', true)
      expect(adminLibraryApi.browseFiles).toHaveBeenCalledTimes(2) // initial load + reload after delete
      wrapper.unmount()
    })

    it('does not delete when the confirm dialog is cancelled', async () => {
      confirmMock.mockResolvedValue(false)
      const wrapper = await mountFM('')
      const buttons = deleteButtons(wrapper)
      await buttons[buttons.length - 1].trigger('click')
      await flushPromises()

      expect(confirmMock).toHaveBeenCalledTimes(1)
      expect(adminLibraryApi.deleteFile).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('shows the episode-member error banner on a 409 with reason=episode_member', async () => {
      confirmMock.mockResolvedValue(true)
      ;(adminLibraryApi.deleteFile as ReturnType<typeof vi.fn>).mockRejectedValueOnce({
        response: { status: 409, data: { reason: 'episode_member' } },
      })
      const wrapper = await mountFM('')
      const buttons = deleteButtons(wrapper)
      await buttons[buttons.length - 1].trigger('click')
      await flushPromises()

      expect(wrapper.text()).toContain('player.adminLibrary.files.error.episodeMember')
      wrapper.unmount()
    })

    it('shows the active-torrent error banner on a 409 with reason=torrent_active', async () => {
      confirmMock.mockResolvedValue(true)
      ;(adminLibraryApi.deleteFile as ReturnType<typeof vi.fn>).mockRejectedValueOnce({
        response: { status: 409, data: { reason: 'torrent_active' } },
      })
      const wrapper = await mountFM('')
      const buttons = deleteButtons(wrapper)
      await buttons[buttons.length - 1].trigger('click')
      await flushPromises()

      expect(wrapper.text()).toContain('player.adminLibrary.files.error.active')
      wrapper.unmount()
    })

    it('shows a generic delete-failed error banner on a non-409 error, without throwing', async () => {
      confirmMock.mockResolvedValue(true)
      ;(adminLibraryApi.deleteFile as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network down'))
      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
      const wrapper = await mountFM('')
      const buttons = deleteButtons(wrapper)
      await buttons[buttons.length - 1].trigger('click')
      await flushPromises()

      expect(wrapper.text()).toContain('player.adminLibrary.files.error.deleteFailed')
      expect(warnSpy).toHaveBeenCalled()
      warnSpy.mockRestore()
      wrapper.unmount()
    })
  })
})
