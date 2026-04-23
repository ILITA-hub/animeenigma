<template>
  <Teleport to="body">
    <Transition name="context-menu">
      <div
        v-if="visible"
        ref="menuRef"
        role="menu"
        class="fixed z-[9999] min-w-[240px] max-w-[320px] bg-slate-900/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-xl shadow-black/30 overflow-hidden"
        :style="{ left: `${adjustedX}px`, top: `${adjustedY}px` }"
        @click.stop
        @mouseenter="onMenuEnter"
        @mouseleave="onMenuLeave"
        @keydown="onMenuKeydown"
      >
        <slot :close-with-reason="closeWithReason" />
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted, provide } from 'vue'

type CloseReason = 'esc' | 'tab' | 'item' | 'outside' | 'hover-out' | 'grace' | 'scroll' | null

const props = defineProps<{
  visible: boolean
  x: number
  y: number
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const menuRef = ref<HTMLElement | null>(null)
const adjustedX = ref(0)
const adjustedY = ref(0)

const closeReason = ref<CloseReason>(null)
let lastFocused: HTMLElement | null = null

let leaveTimer: number | null = null
let graceTimer: number | null = null
let hasEntered = false

const FINE_POINTER =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(hover: hover) and (pointer: fine)')
    : null

function clearTimers() {
  if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null }
  if (graceTimer !== null) { clearTimeout(graceTimer); graceTimer = null }
}

function closeWithReason(reason: CloseReason) {
  closeReason.value = reason
  emit('update:visible', false)
}

// expose to AnimeContextMenu (and anyone else in the slot)
provide('ctxMenuClose', closeWithReason)

function armLeaveTimer() {
  if (leaveTimer !== null) clearTimeout(leaveTimer)
  leaveTimer = window.setTimeout(() => closeWithReason('hover-out'), 600)
}

function armGrace() {
  graceTimer = window.setTimeout(() => {
    if (!hasEntered) closeWithReason('grace')
  }, 1800)
}

function onMenuEnter() {
  hasEntered = true
  clearTimers()
}

function onMenuLeave() {
  if (FINE_POINTER && FINE_POINTER.matches) armLeaveTimer()
}

function onMenuKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    closeWithReason('esc')
    return
  }
  if (e.key === 'Tab') {
    // Let Tab continue naturally to the next focusable element on the page;
    // we just close the menu so the focus return logic can decide what to do.
    closeWithReason('tab')
  }
}

function handleOutsidePointerDown(e: PointerEvent) {
  if (!menuRef.value) return
  if (menuRef.value.contains(e.target as Node)) return
  // NON-consuming: do NOT preventDefault / stopPropagation. The click reaches
  // its target (search bar, navbar, another card's kebab) — feels native and
  // matches Chrome / macOS menu behavior.
  closeWithReason('outside')
}

// Document-level Esc fallback — keeps Esc working even if menu lost focus.
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) {
    closeWithReason('esc')
  }
}

// Viewport clamping after render
watch(() => props.visible, async (isVisible) => {
  if (isVisible) {
    closeReason.value = null
    lastFocused = (document.activeElement as HTMLElement) ?? null
    hasEntered = false
    adjustedX.value = props.x
    adjustedY.value = props.y
    if (FINE_POINTER && FINE_POINTER.matches) armGrace()
    document.addEventListener('pointerdown', handleOutsidePointerDown, true)
    await nextTick()
    clampToViewport()
    // Move focus to the first menuitem (rendered by AnimeContextMenu).
    const first = menuRef.value?.querySelector<HTMLElement>('[role="menuitem"]')
    first?.focus()
  } else {
    clearTimers()
    document.removeEventListener('pointerdown', handleOutsidePointerDown, true)
    // Only restore focus on intentional close. Restoring on outside-click
    // would steal focus from whatever the user clicked.
    if (
      lastFocused &&
      (closeReason.value === 'esc' ||
        closeReason.value === 'tab' ||
        closeReason.value === 'item')
    ) {
      // Tab is a special case: focus has already moved on by the time we run;
      // restoring would yank it back. So skip 'tab' restore — Tab's natural
      // focus advance is the right behavior.
      if (closeReason.value !== 'tab') {
        lastFocused.focus()
      }
    }
    lastFocused = null
  }
})

watch(() => [props.x, props.y], () => {
  adjustedX.value = props.x
  adjustedY.value = props.y
  if (props.visible) {
    nextTick(clampToViewport)
  }
})

function clampToViewport() {
  if (!menuRef.value) return
  const rect = menuRef.value.getBoundingClientRect()
  const vw = window.innerWidth
  const vh = window.innerHeight
  const pad = 8

  if (rect.right > vw - pad) {
    adjustedX.value = vw - rect.width - pad
  }
  if (rect.bottom > vh - pad) {
    adjustedY.value = vh - rect.height - pad
  }
  if (adjustedX.value < pad) adjustedX.value = pad
  if (adjustedY.value < pad) adjustedY.value = pad
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('pointerdown', handleOutsidePointerDown, true)
  clearTimers()
})
</script>

<style scoped>
.context-menu-enter-active {
  transition: opacity 0.12s ease, transform 0.12s ease;
}
.context-menu-leave-active {
  transition: opacity 0.08s ease, transform 0.08s ease;
}
.context-menu-enter-from {
  opacity: 0;
  transform: scale(0.95);
}
.context-menu-leave-to {
  opacity: 0;
  transform: scale(0.95);
}
</style>
