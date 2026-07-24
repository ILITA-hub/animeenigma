import type { InjectionKey } from 'vue'

/**
 * Which surface a notification card is rendered in. Provided by the host
 * (the history modal provides `'history'`; the bell dropdown leaves the
 * default `'dropdown'`), injected by `NotificationRowActions` to pick the
 * right trailing action:
 *
 *   - `dropdown` → the dismiss × ("clear from the bell, keep in history")
 *   - `history`  → the delete bin ("remove from history too")
 *
 * Ambient context, not a prop, so it does not have to thread through the
 * `<component :is>` renderer dispatch in `NotificationList`.
 */
export type NotificationSurface = 'dropdown' | 'history'

export const notificationSurfaceKey: InjectionKey<NotificationSurface> =
  Symbol('notificationSurface')
