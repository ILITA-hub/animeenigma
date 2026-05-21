/**
 * Renderer registry for notification types.
 *
 * The dropdown looks up the renderer for each row by `notification.type`;
 * unknown types fall back to `UnknownNotificationCard`. The toast layer
 * uses `isKnownType()` to suppress toasts for unrecognized types
 * (per Phase 3 plan NOTIF-UI-06 + R-03-02).
 *
 * Adding a new notification type in v1.1 is purely additive:
 *   1. Define the payload type in `src/types/notification.ts`
 *   2. Create a renderer component in `src/components/notifications/`
 *   3. Register it here under the new type string
 *
 * No bell / dropdown / toast / store changes required.
 *
 * Phase 3 — workstream: notifications.
 */
import type { Component } from 'vue'

import NewEpisodeCard from '@/components/notifications/NewEpisodeCard.vue'
import UnknownNotificationCard from '@/components/notifications/UnknownNotificationCard.vue'

/** Map of `notification.type` → Vue component. */
export const renderers: Record<string, Component> = {
  new_episode: NewEpisodeCard,
}

/**
 * Resolve a renderer component for the given notification type. Returns
 * `UnknownNotificationCard` for any type not in the registry.
 */
export function resolveRenderer(type: string): Component {
  return renderers[type] || UnknownNotificationCard
}

/**
 * Whether the given type has a dedicated renderer. The toast layer uses
 * this to suppress toasts for unknown types (the dropdown still renders
 * them via the fallback card).
 */
export function isKnownType(type: string): boolean {
  return type in renderers
}
