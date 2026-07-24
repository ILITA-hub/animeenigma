<template>
  <!-- Empty state -->
  <EmptyState v-if="notifications.length === 0 && !hideEmpty" size="sm" class="text-sm">
    <template #icon><Bell class="size-10" /></template>
    {{ $t('notifications.dropdown.empty') }}
  </EmptyState>

  <!-- Notification list. Read rows stay visible but tinted; dismissed rows
       (history modal only — the dropdown never receives them) are dimmed
       harder + desaturated. This is the single point of control for the
       read/unread/dismissed visual split across every surface (bell
       dropdown + history modal), so the card renderers stay
       presentation-agnostic. -->
  <ul v-else class="divide-y divide-white/5">
    <li
      v-for="n in notifications"
      :key="n.id"
      :class="n.dismissed_at ? 'opacity-50 saturate-50' : n.read_at ? 'opacity-70' : undefined"
      :title="n.dismissed_at ? $t('notifications.history.dismissed') : undefined"
    >
      <component
        :is="resolveRenderer(n.type)"
        :notification="n"
        @close="$emit('close')"
      />
    </li>
  </ul>
</template>

<script setup lang="ts">
/**
 * Shared notification rows — renderer-registry dispatch + read-tint,
 * consumed by NotificationDropdown and NotificationHistoryModal. The
 * host owns the scroll container and any footer/tail states.
 *
 * `hideEmpty` suppresses the empty placeholder while a host is still
 * loading its first page (the history modal shows a spinner instead).
 */
import { Bell } from 'lucide-vue-next'
import EmptyState from '@/components/ui/EmptyState.vue'
import { resolveRenderer } from '@/lib/notification-renderers'
import type { UserNotification } from '@/types/notification'

defineProps<{
  notifications: UserNotification[]
  hideEmpty?: boolean
}>()

defineEmits<{
  (e: 'close'): void
}>()
</script>
