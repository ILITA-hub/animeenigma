import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import {
  listNotifications,
  markRead as apiMarkRead,
  markAllRead as apiMarkAllRead,
  dismiss as apiDismiss,
  deleteNotification as apiDelete,
} from '@/api/notifications'
import { useNotificationsStore, translateWatchUrl } from './notifications'
import type { UserNotification } from '@/types/notification'

vi.mock('@/api/notifications', () => ({
  listNotifications: vi.fn(),
  markRead: vi.fn(),
  markAllRead: vi.fn(),
  dismiss: vi.fn(),
  deleteNotification: vi.fn(),
  click: vi.fn(),
}))

/** Flush the fire-and-forget fetch chains (mocked promises settle in a tick). */
const flush = (): Promise<void> => new Promise((resolve) => setTimeout(resolve))

const STORAGE_KEY = 'notif:shownToasts'

/** Minimal unread notification for getter tests. */
function notif(id: string): UserNotification {
  return {
    id,
    user_id: 'u1',
    type: 'new_episode',
    payload: {},
    read_at: null,
    dismissed_at: null,
    clicked_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as unknown as UserNotification
}

describe('notifications store — toast persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('persists shown-toast ids to localStorage', () => {
    const store = useNotificationsStore()
    store.markToastShown('id-1')

    const raw = localStorage.getItem(STORAGE_KEY)
    expect(raw).not.toBeNull()
    expect(JSON.parse(raw as string)).toContain('id-1')
  })

  it('rehydrates shown-toast ids from localStorage on a fresh store (page reload)', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(['id-1']))
    // Simulate a reload: brand-new pinia + store instance.
    setActivePinia(createPinia())
    const store = useNotificationsStore()
    expect(store.shownToastIds.has('id-1')).toBe(true)
  })

  it('does not re-toast a notification already shown before reload', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(['id-1']))
    setActivePinia(createPinia())
    const store = useNotificationsStore()
    // id-1 was already toasted (persisted); id-2 is fresh.
    store.notifications = [notif('id-1'), notif('id-2')]
    expect(store.latestUndismissedToast?.id).toBe('id-2')
  })

  it('clears persisted shown-toast ids on stop() (logout)', () => {
    const store = useNotificationsStore()
    store.markToastShown('id-1')
    expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull()

    store.stop()
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
  })
})

describe('translateWatchUrl — notification deep-link params', () => {
  it('unwraps /watch and preserves provider/team/episode', () => {
    expect(
      translateWatchUrl('/anime/abc/watch?provider=kodik&team=AniLibria&episode=12'),
    ).toEqual({
      path: '/anime/abc',
      query: { provider: 'kodik', team: 'AniLibria', episode: '12' },
    })
  })

  it('decodes an encoded team title with a space', () => {
    expect(
      translateWatchUrl('/anime/abc/watch?provider=kodik&team=Studio+Band&episode=3'),
    ).toEqual({
      path: '/anime/abc',
      query: { provider: 'kodik', team: 'Studio Band', episode: '3' },
    })
  })
})

describe('notifications store — history pagination', () => {
  /** Page response builder matching the backend ListResponse shape. */
  function page(items: UserNotification[], total: number, unread = 0) {
    return { notifications: items, unread_count: unread, total }
  }

  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.mocked(listNotifications).mockReset()
    vi.mocked(apiMarkRead).mockReset().mockResolvedValue(undefined)
    vi.mocked(apiMarkAllRead).mockReset().mockResolvedValue(0)
    vi.mocked(apiDismiss).mockReset().mockResolvedValue(undefined)
    vi.mocked(apiDelete).mockReset().mockResolvedValue(undefined)
  })

  it('openHistory loads the first page from offset 0', async () => {
    vi.mocked(listNotifications).mockResolvedValue(page([notif('a'), notif('b')], 5, 2))
    const store = useNotificationsStore()

    store.openHistory()
    expect(store.historyOpen).toBe(true)
    await flush()

    expect(listNotifications).toHaveBeenCalledWith('history', 30, 0)
    expect(store.historyItems.map((n) => n.id)).toEqual(['a', 'b'])
    expect(store.historyTotal).toBe(5)
    expect(store.hasMoreHistory).toBe(true)
    expect(store.unreadCount).toBe(2)
  })

  it('fetchMoreHistory appends the next page at offset = loaded rows', async () => {
    vi.mocked(listNotifications).mockResolvedValueOnce(page([notif('a')], 2))
    const store = useNotificationsStore()
    store.openHistory()
    await flush()

    vi.mocked(listNotifications).mockResolvedValueOnce(page([notif('b')], 2))
    await store.fetchMoreHistory()

    expect(listNotifications).toHaveBeenLastCalledWith('history', 30, 1)
    expect(store.historyItems.map((n) => n.id)).toEqual(['a', 'b'])
    expect(store.hasMoreHistory).toBe(false)
  })

  it('does not fetch again once every row is loaded', async () => {
    vi.mocked(listNotifications).mockResolvedValue(page([notif('a')], 1))
    const store = useNotificationsStore()
    store.openHistory()
    await flush()

    await store.fetchMoreHistory()
    expect(listNotifications).toHaveBeenCalledTimes(1)
  })

  it('clamps total when an offset-drifted page is all duplicates', async () => {
    vi.mocked(listNotifications).mockResolvedValueOnce(page([notif('a')], 10))
    const store = useNotificationsStore()
    store.openHistory()
    await flush()

    // Same row again (new notifications shifted the offset window).
    vi.mocked(listNotifications).mockResolvedValueOnce(page([notif('a')], 10))
    await store.fetchMoreHistory()

    expect(store.historyItems.map((n) => n.id)).toEqual(['a'])
    expect(store.hasMoreHistory).toBe(false) // stops instead of spinning
  })

  it('records an error and recovers on retry', async () => {
    vi.mocked(listNotifications).mockRejectedValueOnce(new Error('boom'))
    const store = useNotificationsStore()
    store.openHistory()
    await flush()

    expect(store.historyError).toBe('boom')
    expect(store.historyItems).toEqual([])

    vi.mocked(listNotifications).mockResolvedValueOnce(page([notif('a')], 1))
    await store.fetchMoreHistory()
    expect(store.historyError).toBeNull()
    expect(store.historyItems.map((n) => n.id)).toEqual(['a'])
  })

  it('markRead flips the same id in both the dropdown and history lists', async () => {
    const store = useNotificationsStore()
    store.notifications = [notif('a')]
    store.historyItems = [notif('a'), notif('old')]
    store.unreadCount = 2

    await store.markRead('a')

    expect(store.notifications[0].read_at).not.toBeNull()
    expect(store.historyItems[0].read_at).not.toBeNull()
    expect(store.historyItems[1].read_at).toBeNull()
    expect(store.unreadCount).toBe(1)
  })

  it('markRead works for a history-only (older) row', async () => {
    const store = useNotificationsStore()
    store.notifications = []
    store.historyItems = [notif('old')]
    store.unreadCount = 1

    await store.markRead('old')

    expect(store.historyItems[0].read_at).not.toBeNull()
    expect(store.unreadCount).toBe(0)
  })

  it('dismiss removes the dropdown row but stamps (keeps) the history row', async () => {
    const store = useNotificationsStore()
    store.notifications = [notif('a')]
    store.historyItems = [notif('a'), notif('b')]
    store.historyTotal = 2
    store.unreadCount = 1

    await store.dismiss('a')

    expect(store.notifications).toEqual([])
    expect(store.historyItems.map((n) => n.id)).toEqual(['a', 'b'])
    expect(store.historyItems[0].dismissed_at).not.toBeNull()
    expect(store.historyTotal).toBe(2) // dismissed rows still count in history
    expect(store.unreadCount).toBe(0)
  })

  it('dismiss restores the un-stamped history row on API failure', async () => {
    vi.mocked(apiDismiss).mockRejectedValueOnce(new Error('boom'))
    const store = useNotificationsStore()
    store.historyItems = [notif('a')]
    store.historyTotal = 1
    store.unreadCount = 1

    await expect(store.dismiss('a')).rejects.toThrow('boom')

    expect(store.historyItems[0].dismissed_at).toBeNull()
    expect(store.unreadCount).toBe(1)
  })

  it('dismiss and markRead are no-ops on an already-dismissed history row', async () => {
    const dismissed = { ...notif('a'), dismissed_at: new Date().toISOString() }
    const store = useNotificationsStore()
    store.historyItems = [dismissed]
    store.unreadCount = 3

    await store.dismiss('a')
    await store.markRead('a')

    expect(apiDismiss).not.toHaveBeenCalled()
    expect(apiMarkRead).not.toHaveBeenCalled()
    expect(store.historyItems[0]).toEqual(dismissed)
    expect(store.unreadCount).toBe(3) // dismissed rows never touch the badge
  })

  it('markAllRead stamps active history rows but skips dismissed ones', async () => {
    const dismissed = { ...notif('gone'), dismissed_at: new Date().toISOString() }
    const store = useNotificationsStore()
    store.notifications = [notif('a')]
    store.historyItems = [notif('a'), notif('old'), dismissed]
    store.unreadCount = 2

    await store.markAllRead()

    expect(store.historyItems[0].read_at).not.toBeNull()
    expect(store.historyItems[1].read_at).not.toBeNull()
    expect(store.historyItems[2].read_at).toBeNull() // dismissed left untouched
    expect(store.unreadCount).toBe(0)
  })

  it('delete removes the row from both lists, shrinks history total, drops the badge', async () => {
    const store = useNotificationsStore()
    store.notifications = [notif('a')]
    store.historyItems = [notif('a'), notif('b')]
    store.historyTotal = 2
    store.unreadCount = 1

    await store.delete('a')

    expect(apiDelete).toHaveBeenCalledWith('a')
    expect(store.notifications).toEqual([])
    expect(store.historyItems.map((n) => n.id)).toEqual(['b'])
    expect(store.historyTotal).toBe(1) // deleted rows leave the history scope
    expect(store.unreadCount).toBe(0)
  })

  it('delete of a dismissed history row never touches the badge', async () => {
    const dismissed = { ...notif('a'), dismissed_at: new Date().toISOString() }
    const store = useNotificationsStore()
    store.historyItems = [dismissed, notif('b')]
    store.historyTotal = 2
    store.unreadCount = 3

    await store.delete('a')

    expect(store.historyItems.map((n) => n.id)).toEqual(['b'])
    expect(store.historyTotal).toBe(1)
    expect(store.unreadCount).toBe(3) // dismissed row wasn't counting
  })

  it('delete rolls back both lists, total, and badge on API failure', async () => {
    vi.mocked(apiDelete).mockRejectedValueOnce(new Error('boom'))
    const store = useNotificationsStore()
    store.notifications = [notif('a')]
    store.historyItems = [notif('a'), notif('b')]
    store.historyTotal = 2
    store.unreadCount = 1

    await expect(store.delete('a')).rejects.toThrow('boom')

    expect(store.notifications.map((n) => n.id)).toEqual(['a'])
    expect(store.historyItems.map((n) => n.id)).toEqual(['a', 'b'])
    expect(store.historyTotal).toBe(2)
    expect(store.unreadCount).toBe(1)
  })

  it('delete is a no-op on an already-deleted row', async () => {
    const deleted = { ...notif('a'), deleted_at: new Date().toISOString() }
    const store = useNotificationsStore()
    store.historyItems = [deleted]
    store.unreadCount = 2

    await store.delete('a')

    expect(apiDelete).not.toHaveBeenCalled()
    expect(store.historyItems.map((n) => n.id)).toEqual(['a'])
    expect(store.unreadCount).toBe(2)
  })

  it('stop() clears history state and closes the modal', async () => {
    vi.mocked(listNotifications).mockResolvedValue(page([notif('a')], 1))
    const store = useNotificationsStore()
    store.openHistory()
    await flush()

    store.stop()

    expect(store.historyOpen).toBe(false)
    expect(store.historyItems).toEqual([])
    expect(store.historyTotal).toBe(0)
    expect(store.historyError).toBeNull()
  })
})
