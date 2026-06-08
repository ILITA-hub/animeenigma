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

  it('normalizes the default "all" sentinel to no filter params', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    await fb.refresh()
    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({ category: undefined, status: undefined, type: undefined }),
    )
  })

  it('passes active filters as query params and resets to page 1 via applyFilters', async () => {
    listSpy.mockResolvedValue(listEnvelope([]))
    const fb = useAdminFeedback()
    fb.page.value = 3
    fb.filterCategory.value = 'feature'
    fb.filterStatus.value = 'resolved'
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
})
