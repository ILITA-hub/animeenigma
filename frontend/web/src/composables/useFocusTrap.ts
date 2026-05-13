import { watch, nextTick, onScopeDispose, getCurrentScope, type Ref } from 'vue'

/**
 * Naive focus trap for modal-like containers.
 *
 * Cycles Tab / Shift+Tab between the first and last focusable descendants of
 * `container` while `active` is true. On activation, focuses the first focusable
 * child. On deactivation, restores focus to `returnFocusTo` if provided.
 *
 * Cleanup: the global `keydown` listener is unconditionally detached on scope
 * disposal (parent component unmount) — even if `active` is still true. This
 * prevents listener leaks when a host tears down mid-open.
 *
 * Zero dependencies — no `focus-trap` npm package.
 *
 * Usage:
 *   const drawerRef = ref<HTMLElement | null>(null)
 *   const hamburgerButtonRef = ref<HTMLButtonElement | null>(null)
 *   const open = ref(false)
 *   useFocusTrap({ active: open, container: drawerRef, returnFocusTo: hamburgerButtonRef })
 */
const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

export interface UseFocusTrapOptions {
  active: Ref<boolean>
  container: Ref<HTMLElement | null>
  returnFocusTo?: Ref<HTMLElement | null>
}

export function useFocusTrap(options: UseFocusTrapOptions): void {
  const { active, container, returnFocusTo } = options

  function getFocusables(): HTMLElement[] {
    const root = container.value
    if (!root) return []
    // The selector already excludes [disabled] for button/input/select/textarea.
    // Keep only the visibility filter (offsetParent !== null skips display:none
    // and elements detached from layout).
    return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
      .filter(el => el.offsetParent !== null)
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Tab') return
    const focusables = getFocusables()
    if (focusables.length === 0) {
      event.preventDefault()
      return
    }
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    const activeEl = document.activeElement as HTMLElement | null

    if (event.shiftKey) {
      if (activeEl === first || !container.value?.contains(activeEl)) {
        event.preventDefault()
        last.focus()
      }
    } else {
      if (activeEl === last || !container.value?.contains(activeEl)) {
        event.preventDefault()
        first.focus()
      }
    }
  }

  watch(active, async (isActive, wasActive) => {
    if (isActive) {
      await nextTick()
      document.addEventListener('keydown', onKeydown)
      const focusables = getFocusables()
      if (focusables.length > 0) {
        focusables[0].focus()
      }
    } else if (wasActive) {
      document.removeEventListener('keydown', onKeydown)
      if (returnFocusTo?.value) {
        returnFocusTo.value.focus()
      }
    }
  }, { immediate: true })

  // Unconditional cleanup on parent unmount. Guard against being called outside
  // a reactive scope (e.g. ad-hoc invocation in tests): onScopeDispose is a no-op
  // when there is no current scope, but we check explicitly for clarity.
  if (getCurrentScope()) {
    onScopeDispose(() => {
      document.removeEventListener('keydown', onKeydown)
    })
  }
}
