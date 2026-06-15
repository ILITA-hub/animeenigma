<template>
  <!--
    Workstream watch-together — Phase 02 (frontend-shell) Plan 02.4 Task 2.

    Chat round-trip surface for the room sidebar. Pure presentational
    component:
      - Props-only data flow (messages + currentUserId come from the
        parent's useWatchTogetherRoom composable).
      - Emits `send(body)` for outgoing messages — the parent bridges this
        to the composable's sendChat() so this component stays decoupled
        from the WS layer.
      - Auto-scroll-on-new is gated on "user was at-bottom" so we never
        yank a scroll-back user back down to fresh messages.

    UI-SPEC contract (CLAUDE.md):
      - Tailwind utility classes only
      - Font weights: font-medium / font-semibold only
      - Padding: p-4 md:p-6 on the outer <section>
  -->
  <section
    class="flex flex-col h-full p-4 md:p-6"
    :aria-label="t('watch_together.title')"
  >
    <h3 class="text-sm uppercase tracking-wider text-foreground/60 font-semibold mb-3 flex-shrink-0">
      {{ t('watch_together.title') }}
    </h3>

    <ul
      ref="listRef"
      class="flex-1 min-h-0 overflow-y-auto flex flex-col gap-2 pr-1"
    >
      <li
        v-for="msg in messages"
        :key="msg.id"
        :class="[
          'flex flex-col rounded-md p-2 max-w-[85%] gap-0.5',
          msg.user_id === currentUserId
            ? 'self-end bg-primary/10 items-end'
            : 'self-start bg-foreground/5 items-start',
        ]"
      >
        <div class="flex items-baseline gap-2">
          <span class="font-medium text-sm">{{ msg.username }}</span>
          <time
            :datetime="new Date(msg.ts).toISOString()"
            class="text-xs text-foreground/50 font-medium"
          >
            {{ formatTs(msg.ts) }}
          </time>
        </div>
        <p class="text-sm whitespace-pre-wrap break-words">{{ msg.body }}</p>
      </li>
    </ul>

    <p
      v-if="messages.length === 0"
      class="text-sm text-foreground/50 font-medium flex-1 flex items-center justify-center"
    >
      {{ t('watch_together.empty_chat') }}
    </p>

    <div class="flex-shrink-0 mt-3 flex flex-col gap-2">
      <textarea
        v-model="draft"
        :placeholder="t('watch_together.chat_input_placeholder')"
        :aria-label="t('watch_together.chat_input_aria')"
        rows="2"
        maxlength="500"
        class="w-full rounded-md bg-foreground/5 border border-foreground/10 p-2 text-sm font-medium resize-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
        @keydown="onKeydown"
      />

      <div class="flex items-center justify-between gap-2">
        <span
          v-if="showCounter"
          aria-live="polite"
          class="text-xs text-foreground/50 font-medium"
        >
          {{ t('watch_together.chat_char_count', { n: draft.length }) }}
        </span>
        <span v-else aria-hidden="true" />

        <Button
          variant="default"
          size="sm"
          :disabled="!canSend"
          @click="onSend"
        >
          {{ t('watch_together.send_button') }}
        </Button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'

import type { ChatMessage } from '@/api/watch-together'

const props = defineProps<{
  messages: ChatMessage[]
  currentUserId: string
}>()

const emit = defineEmits<{
  send: [body: string]
}>()

const { t } = useI18n()

const draft = ref('')
const listRef = useTemplateRef<HTMLUListElement>('listRef')

const MAX_CHARS = 500
const COUNTER_THRESHOLD = 400
const SCROLL_TOLERANCE_PX = 50

const canSend = computed(() => {
  const trimmed = draft.value.trim()
  return trimmed.length > 0 && trimmed.length <= MAX_CHARS
})

const showCounter = computed(() => draft.value.length > COUNTER_THRESHOLD)

function isAtBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < SCROLL_TOLERANCE_PX
}

function formatTs(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
}

function onSend() {
  if (!canSend.value) return
  const body = draft.value.trim()
  emit('send', body)
  draft.value = ''
}

function onKeydown(event: KeyboardEvent) {
  // Enter without Shift sends. Shift+Enter inserts a newline (default behavior).
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    onSend()
  }
}

// Auto-scroll on new messages, but ONLY if the user was already pinned at
// the bottom of the list. This avoids yanking a scroll-back reader back
// down when a new message arrives.
watch(
  () => props.messages.length,
  (newLen, oldLen) => {
    if (newLen <= oldLen) return
    const el = listRef.value
    if (!el) return
    const wasAtBottom = isAtBottom(el)
    if (!wasAtBottom) return
    void nextTick(() => {
      const elAfter = listRef.value
      if (elAfter) elAfter.scrollTop = elAfter.scrollHeight
    })
  },
)
</script>
