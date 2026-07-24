import { describe, it, expect, vi, beforeEach } from 'vitest'

const listUsers = vi.fn()
const updateUserRole = vi.fn()
vi.mock('@/api/client', () => ({
  adminApi: {
    listUsers: (...a: unknown[]) => listUsers(...a),
    updateUserRole: (...a: unknown[]) => updateUserRole(...a),
  },
}))

import { useAdminUsers } from '@/composables/useAdminUsers'

describe('useAdminUsers', () => {
  beforeEach(() => {
    listUsers.mockReset()
    updateUserRole.mockReset()
  })

  it('maps the list envelope and normalizes filters (all -> undefined, trims q)', async () => {
    listUsers.mockResolvedValue({
      data: { success: true, data: { items: [{ id: 'u1', username: 'a', public_id: 'p', role: 'user', created_at: '' }], total: 1, page: 1, page_size: 25 } },
    })
    const u = useAdminUsers()
    u.query.value = '  neo  '
    u.roleFilter.value = 'all'
    await u.refresh()
    expect(listUsers).toHaveBeenCalledWith({ q: 'neo', role: undefined, page: 1, page_size: 25 })
    expect(u.items.value).toHaveLength(1)
    expect(u.total.value).toBe(1)
  })

  it('maps a 403 to the "403" sentinel and clears items', async () => {
    listUsers.mockRejectedValue({ response: { status: 403 } })
    const u = useAdminUsers()
    await u.refresh()
    expect(u.error.value).toBe('403')
    expect(u.items.value).toEqual([])
  })

  it('changeRole replaces the row with the updated user', async () => {
    listUsers.mockResolvedValue({
      data: { data: { items: [{ id: 'u1', username: 'a', public_id: 'p', role: 'user', created_at: '' }], total: 1, page: 1, page_size: 25 } },
    })
    updateUserRole.mockResolvedValue({ data: { data: { id: 'u1', username: 'a', public_id: 'p', role: 'admin', created_at: '' } } })
    const u = useAdminUsers()
    await u.refresh()
    await u.changeRole('u1', 'admin')
    expect(updateUserRole).toHaveBeenCalledWith('u1', 'admin')
    expect(u.items.value[0].role).toBe('admin')
  })
})
