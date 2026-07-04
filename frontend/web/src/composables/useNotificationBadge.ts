import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNotificationsStore } from '@/stores/notifications'

/**
 * Shared presentation contract for the notification unread badge — the
 * "99+" cap and the counted aria-label — consumed by both the desktop
 * bell (NotificationBell.vue) and the mobile drawer row (Navbar.vue) so
 * the two surfaces cannot drift.
 */
export function useNotificationBadge() {
  const { t } = useI18n()
  const store = useNotificationsStore()

  const badgeText = computed<string>(() => {
    const n = store.unreadCount
    return n > 99 ? '99+' : String(n)
  })

  const ariaLabel = computed<string>(() => {
    if (store.unreadCount > 0) {
      return t('notifications.bell.ariaLabelWithCount', { count: store.unreadCount })
    }
    return t('notifications.bell.ariaLabel')
  })

  return { badgeText, ariaLabel }
}
