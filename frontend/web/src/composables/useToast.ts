import { ref, readonly } from 'vue'

/**
 * Toast — small global reactive toast queue. Phase 13 introduces this for
 * watchlist rollback errors; future phases (Phase 20) will harden it with
 * undo affordances and per-type styling.
 *
 * Usage:
 *   const { push, dismiss } = useToast()
 *   push('Couldn\'t update — please retry', 'error')
 *   push('Saved', 'success', 2000)
 *
 * State is module-level so every call site shares the same queue. The
 * `<Toaster />` component (mounted in App.vue) renders the list.
 */

export type ToastType = 'error' | 'success' | 'info'

export interface Toast {
  id: number
  message: string
  type: ToastType
  duration: number
}

const toasts = ref<Toast[]>([])
let nextId = 1

function dismiss(id: number): void {
  const idx = toasts.value.findIndex(t => t.id === id)
  if (idx >= 0) toasts.value.splice(idx, 1)
}

function push(message: string, type: ToastType = 'error', duration = 3000): number {
  const id = nextId++
  const toast: Toast = { id, message, type, duration }
  toasts.value.push(toast)
  if (duration > 0 && typeof window !== 'undefined') {
    window.setTimeout(() => dismiss(id), duration)
  }
  return id
}

export function useToast() {
  return {
    toasts: readonly(toasts),
    push,
    dismiss,
  }
}
