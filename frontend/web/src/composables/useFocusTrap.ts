import { watch, nextTick, type Ref } from 'vue'

/**
 * Naive focus trap for modal-like containers.
 *
 * Cycles Tab / Shift+Tab between the first and last focusable descendants of
 * `container` while `active` is true. On activation, focuses the first focusable
 * child. On deactivation, restores focus to `returnFocusTo` if provided.
 *
 * Zero dependencies — no `focus-trap` npm package. ~30 LOC.
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
    return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
      .filter(el => !el.hasAttribute('disabled') && el.offsetParent !== null)
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
  })
}
