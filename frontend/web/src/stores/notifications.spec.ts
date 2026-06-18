import { beforeEach, describe, expect, it } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import { useNotificationsStore, translateWatchUrl } from './notifications'
import type { UserNotification } from '@/types/notification'

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
