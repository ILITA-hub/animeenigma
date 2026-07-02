/**
 * Vitest spec for NotificationDropdown.vue — the bell dropdown body.
 *
 * The list shows ALL active notifications (store fetches status=all);
 * read rows stay visible but tinted. The tint is applied once at the
 * <li> wrapper — the single point of control — so it must hold for every
 * renderer type without per-card logic.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

const pushMock = vi.fn()
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRouter: () => ({ push: pushMock }),
}))

import NotificationDropdown from './NotificationDropdown.vue'
import { useNotificationsStore } from '@/stores/notifications'
import type { UserNotification } from '@/types/notification'

function notif(id: string, readAt: string | null): UserNotification {
  return {
    id,
    user_id: 'u-1',
    type: 'feedback_created',
    dedupe_key: `feedback:${id}:created`,
    payload: { report_id: id, status: 'created' },
    read_at: readAt,
    dismissed_at: null,
    clicked_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as UserNotification
}

function mountDropdown() {
  return mount(NotificationDropdown, {
    global: {
      mocks: { $t: (key: string) => key },
    },
  })
}

describe('NotificationDropdown', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    pushMock.mockClear()
  })

  it('tints read rows and leaves unread rows untinted', () => {
    const store = useNotificationsStore()
    store.notifications = [
      notif('n-read', new Date().toISOString()),
      notif('n-unread', null),
    ]

    const rows = mountDropdown().findAll('li')
    expect(rows).toHaveLength(2)
    expect(rows[0].classes()).toContain('opacity-70')
    expect(rows[1].classes()).not.toContain('opacity-70')
  })

  it('renders the empty state when there are no notifications', () => {
    const wrapper = mountDropdown()
    expect(wrapper.find('li').exists()).toBe(false)
    expect(wrapper.text()).toContain('notifications.dropdown.empty')
  })
})
