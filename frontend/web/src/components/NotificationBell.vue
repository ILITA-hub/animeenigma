<template>
  <div ref="containerRef" class="relative">
    <button
      type="button"
      class="relative p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400"
      :aria-label="ariaLabel"
      :title="$t('notifications.bell.tooltip')"
      aria-haspopup="true"
      :aria-expanded="open"
      @click="toggle"
    >
      <Bell class="size-5" aria-hidden="true" />

      <!-- Unread badge — aria-hidden because the screen-reader gets the
           count via the button's aria-label. Pink to match the project's
           pink-error accent (see App.vue's error svg + auth error text). -->
      <span
        v-if="store.unreadCount > 0"
        aria-hidden="true"
        class="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 rounded-full bg-pink-500 text-white text-[10px] font-semibold leading-none flex items-center justify-center pointer-events-none"
      >
        {{ badgeText }}
      </span>
    </button>

    <!-- Dropdown anchored right-edge of bell -->
    <Transition name="dropdown">
      <div
        v-if="open"
        class="absolute right-0 top-full mt-2 z-50"
      >
        <NotificationDropdown @close="open = false" />
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
/**
 * Header-mounted notification bell. Click toggles a dropdown anchored to
 * its right edge. The badge mirrors the project's pink-error accent and
 * caps at "99+". Outside-click + Esc close the dropdown.
 *
 * Phase 3 — workstream: notifications.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onClickOutside, useEventListener } from '@vueuse/core'
import { Bell } from 'lucide-vue-next'

import { useNotificationsStore } from '@/stores/notifications'
import NotificationDropdown from '@/components/NotificationDropdown.vue'

const { t } = useI18n()
const store = useNotificationsStore()

const open = ref(false)
const containerRef = ref<HTMLElement | null>(null)

const badgeText = computed<string>(() => {
  const n = store.unreadCount
  return n > 99 ? '99+' : String(n)
})

const ariaLabel = computed(() => {
  if (store.unreadCount > 0) {
    return t('notifications.bell.ariaLabelWithCount', { count: store.unreadCount })
  }
  return t('notifications.bell.ariaLabel')
})

function toggle(): void {
  open.value = !open.value
  if (open.value) {
    // Fire-and-forget refresh so the dropdown reflects the up-to-the-
    // second list — the 60s ticker can leave a stale state otherwise.
    void store.fetchNotifications()
  }
}

onClickOutside(containerRef, () => {
  if (open.value) open.value = false
})

useEventListener(document, 'keydown', (e: KeyboardEvent) => {
  if (e.key === 'Escape' && open.value) {
    open.value = false
  }
})
</script>

<style scoped>
/* Matches the Navbar.vue named transitions for visual consistency. */
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
