/**
 * Vitest spec for useAdminFeedback() — the admin feedback browser composable.
 *
 * Stubs `@/api/client`'s adminApi and verifies the fetch / unwrap / filter /
 * status-mutation contract against admin_reports.go's response envelope.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'

vi.mock('@/api/client', () => ({
  adminApi: {
    listReports: vi.fn(),
    getReport: vi.fn(),
    setReportStatus: vi.fn(),
  },
}))

import { adminApi } from '@/api/client'
import { useAdminFeedback } from '../useAdminFeedback'

const listSpy = adminApi.listReports as ReturnType<typeof vi.fn>
const getSpy = adminApi.getReport as ReturnType<typeof vi.fn>
const setSpy = adminApi.setReportStatus as ReturnType<typeof vi.fn>

function listEnvelope(items: unknown[], total = items.length) {
  return { data: { success: true, data: { items, total, page: 1, page_size: 50 } } }
}

const sampleRow = {
  id: '2026-06-05T12-00-00_alice_feedback',
  timestamp: '2026-06-05T12:00:00Z',
  username: 'alice',
  user_id: 'u1',
  player_type: 'feedback',
  category: 'bug',
  anime_name: '',
  url: '/watch/x',
  description: 'broken',
  status: 'new',
}

beforeEach(() => {
  vi.clearAllMocks()
  // The status filter now persists to localStorage; clear it so each test starts
  // from the default active set instead of a value leaked by a prior test.
  localStorage.clear()
})

describe('useAdminFeedback', () => {
  it('refresh() unwraps the {success,data} envelope and populates items', async () => {
    listSpy.mockResolvedValue(listEnvelope([sampleRow]))
    const fb = useAdminFeedback()
    await fb.refresh()
    await flushPromises()
    expect(fb.items.value).toHaveLength(1)
    expect(fb.items.value[0].username).toBe('alice')
    expect(fb.total.value).toBe(1)
    expect(fb.error.value).toBeNull()
  })

  it('normalizes the "all" sentinel to undefined for category/type/username; status defaults to the active set as CSV', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    await fb.refresh()
    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({ category: undefined, status: 'new,in_progress,ai_done,resolved', type: undefined, username: undefined }),
    )
  })

  it('serializes the multi-select status array to a comma-separated param; empty = no filter', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    fb.filterStatuses.value = ['new', 'ai_done']
    await fb.refresh()
    expect(listSpy).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'new,ai_done' }))
    fb.filterStatuses.value = []
    await fb.refresh()
    expect(listSpy).toHaveBeenLastCalledWith(expect.objectContaining({ status: undefined }))
  })

  it('passes a trimmed username filter, omitting it when blank', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    fb.filterUsername.value = '  tNeymik '
    await fb.refresh()
    expect(listSpy).toHaveBeenCalledWith(expect.objectContaining({ username: 'tNeymik' }))
    fb.filterUsername.value = '   '
    await fb.refresh()
    expect(listSpy).toHaveBeenLastCalledWith(expect.objectContaining({ username: undefined }))
  })

  it('passes active filters as query params and resets to page 1 via applyFilters', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    fb.page.value = 3
    fb.filterCategory.value = 'feature'
    fb.filterStatuses.value = ['resolved']
    fb.filterType.value = 'kodik'
    await fb.applyFilters()
    expect(fb.page.value).toBe(1)
    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({ category: 'feature', status: 'resolved', type: 'kodik', page: 1 }),
    )
  })

  it('maps a 403 to the literal "403" and clears items', async () => {
    listSpy.mockRejectedValue({ response: { status: 403 } })
    const fb = useAdminFeedback()
    await fb.refresh()
    expect(fb.error.value).toBe('403')
    expect(fb.items.value).toHaveLength(0)
  })

  it('openDetail() loads and unwraps the full report', async () => {
    getSpy.mockResolvedValue({ data: { success: true, data: { ...sampleRow, page_html: '<html></html>' } } })
    const fb = useAdminFeedback()
    await fb.openDetail(sampleRow.id)
    expect(fb.detail.value?.page_html).toBe('<html></html>')
    expect(fb.detailError.value).toBeNull()
  })

  it('setStatus() optimistically updates the row and persists', async () => {
    listSpy.mockResolvedValue(listEnvelope([{ ...sampleRow }]))
    setSpy.mockResolvedValue({ data: { success: true } })
    const fb = useAdminFeedback()
    await fb.refresh()
    await fb.setStatus(sampleRow.id, 'resolved')
    expect(setSpy).toHaveBeenCalledWith(sampleRow.id, 'resolved')
    expect(fb.items.value[0].status).toBe('resolved')
  })

  it('setStatus() rolls back the optimistic update on error', async () => {
    listSpy.mockResolvedValue(listEnvelope([{ ...sampleRow, status: 'new' }]))
    setSpy.mockRejectedValue({ response: { status: 500 } })
    const fb = useAdminFeedback()
    await fb.refresh()
    await fb.setStatus(sampleRow.id, 'resolved')
    expect(fb.items.value[0].status).toBe('new')
  })

  it('persists the status filter to localStorage and restores it on a fresh instance', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    fb.filterStatuses.value = ['resolved', 'not_relevant']
    await flushPromises() // let the persistence watcher flush
    expect(JSON.parse(localStorage.getItem('admin_feedback_statuses')!)).toEqual(['resolved', 'not_relevant'])

    // A new instance (e.g. page revisit) restores the persisted selection.
    const fb2 = useAdminFeedback()
    expect(fb2.filterStatuses.value).toEqual(['resolved', 'not_relevant'])
  })

  it('treats a persisted empty selection as "all statuses" (not the active default)', async () => {
    localStorage.setItem('admin_feedback_statuses', '[]')
    const fb = useAdminFeedback()
    expect(fb.filterStatuses.value).toEqual([])
  })

  it('falls back to the active default when localStorage holds malformed data', () => {
    localStorage.setItem('admin_feedback_statuses', 'not json')
    const fb = useAdminFeedback()
    expect(fb.filterStatuses.value).toEqual(['new', 'in_progress', 'ai_done', 'resolved'])
  })

  it('defaults status to the active set and sends the source param', async () => {
    listSpy.mockResolvedValue(listEnvelope([sampleRow]))
    const fb = useAdminFeedback()
    expect(fb.filterStatuses.value).toEqual(['new', 'in_progress', 'ai_done', 'resolved'])
    fb.filterSource.value = 'manual'
    await fb.applyFilters()
    await flushPromises()
    const lastCall = listSpy.mock.calls.at(-1)![0]
    expect(lastCall).toMatchObject({ status: 'new,in_progress,ai_done,resolved', source: 'manual' })
  })
})
