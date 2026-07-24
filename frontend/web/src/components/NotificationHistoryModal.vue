<template>
  <Modal
    :model-value="store.historyOpen"
    :title="$t('notifications.history.title')"
    size="lg"
    @update:model-value="onModelUpdate"
  >
    <div
      ref="listRef"
      class="overflow-y-auto max-h-[min(560px,60svh)]"
      role="region"
      :aria-label="$t('notifications.history.title')"
    >
      <NotificationList
        :notifications="store.historyItems"
        :hide-empty="store.historyLoading || !!store.historyError"
        @close="store.closeHistory()"
      />

      <div
        v-if="store.historyLoading"
        class="flex items-center justify-center gap-2 py-3 text-sm text-white/50"
      >
        <Spinner size="sm" tone="mono" />
        {{ $t('notifications.dropdown.loading') }}
      </div>
      <div
        v-else-if="store.historyError"
        class="flex flex-col items-center gap-2 py-3"
      >
        <p class="text-sm text-white/50">{{ $t('notifications.history.error') }}</p>
        <Button variant="soft" size="xs" @click="store.fetchMoreHistory()">
          {{ $t('common.retry') }}
        </Button>
      </div>
      <p
        v-else-if="!store.hasMoreHistory && store.historyItems.length > 0"
        class="py-3 text-center text-xs text-white/40"
      >
        {{ $t('notifications.history.end') }}
      </p>
    </div>
  </Modal>
</template>

<script setup lang="ts">
/**
 * "View older notifications" modal — the full paged notification history,
 * opened from the dropdown footer (store.openHistory()). All list state
 * lives in the notifications store so read/dismiss stay consistent with
 * the dropdown; this component only owns the scroll trigger.
 *
 * Mounted once in App.vue (next to NotificationToast) so it survives both
 * trigger surfaces: the desktop bell dropdown and the mobile-drawer Modal.
 */
import { provide, ref, watch } from 'vue'
import { useInfiniteScroll } from '@vueuse/core'

import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import Spinner from '@/components/ui/Spinner.vue'
import NotificationList from '@/components/NotificationList.vue'
import { notificationSurfaceKey } from '@/components/notifications/surface'
import { useNotificationsStore } from '@/stores/notifications'

const store = useNotificationsStore()

// Every card rendered inside this modal is on the "history" surface, so its
// trailing action is the delete bin (not the dismiss ×) — see
// NotificationRowActions. Ambient, so it need not thread through NotificationList.
provide(notificationSurfaceKey, 'history')

const listRef = ref<HTMLElement | null>(null)

function onModelUpdate(open: boolean): void {
  if (!open) store.closeHistory()
}

// vueuse drives the scroll trigger + its "keep loading until the container
// is actually full" loop. canLoadMore is strict (the fetched total must say
// more exist) so an empty history can't re-poll offset 0 forever — the
// first page is fired by openHistory(), not the scroller.
const { reset } = useInfiniteScroll(
  listRef,
  () => store.fetchMoreHistory(),
  {
    distance: 200,
    canLoadMore: () => !store.historyError && store.hasMoreHistory,
  },
)

// Re-arm the fill-check whenever a fetch settles (open's first page, the
// retry button): canLoadMore only turns true after a page lands, and
// without a re-check a short first page could never grow — the container
// isn't scrollable yet, so no scroll event would ever fire.
watch(
  () => store.historyLoading,
  (loading) => {
    if (!loading) reset()
  },
)
</script>
