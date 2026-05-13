import { watch, onScopeDispose, getCurrentScope, type Ref } from 'vue'

/**
 * Reference-counted body scroll lock.
 *
 * Multiple consumers (e.g. mobile drawer + a Modal opened on top of it) can
 * activate the lock concurrently. The underlying `document.body.style.overflow`
 * is only mutated on the 0→1 (lock) and 1→0 (unlock) transitions of the
 * refcount, so consumers cannot accidentally stomp each other's state.
 *
 * On the 0→1 transition, the prior value of `document.body.style.overflow`
 * (often an empty string, but may be set by app CSS or 3rd-party scripts) is
 * captured and restored on the 1→0 transition — instead of unconditionally
 * resetting to `''`.
 *
 * Consumers pass a `Ref<boolean>`; this composable watches it and adjusts the
 * refcount. On scope dispose, an active lock is released so a host that
 * unmounts mid-lock does not leak the lock state indefinitely.
 *
 * Usage:
 *   const open = ref(false)
 *   useBodyScrollLock(open)
 */

// Module-level refcount + captured prior value. Singleton-per-page intentional —
// every consumer in the app shares this so refcounting works across components.
let lockCount = 0
let priorOverflow = ''

function acquire(): void {
  if (lockCount === 0) {
    priorOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  }
  lockCount += 1
}

function release(): void {
  if (lockCount === 0) return // defensive: paired release with no acquire
  lockCount -= 1
  if (lockCount === 0) {
    document.body.style.overflow = priorOverflow
  }
}

export function useBodyScrollLock(active: Ref<boolean>): void {
  // Track whether THIS consumer is currently holding a lock, so we don't
  // double-acquire or double-release if the ref toggles unexpectedly.
  let holdingLock = false

  watch(active, (isActive) => {
    if (isActive && !holdingLock) {
      acquire()
      holdingLock = true
    } else if (!isActive && holdingLock) {
      release()
      holdingLock = false
    }
  }, { immediate: true })

  if (getCurrentScope()) {
    onScopeDispose(() => {
      if (holdingLock) {
        release()
        holdingLock = false
      }
    })
  }
}
