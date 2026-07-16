import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import {
  classifyConnection,
  SLOW_SUSTAINED_MS,
  type ConnectionState,
} from '@/components/player/aePlayer/connectionHealth'

export interface ConnectionHealthDeps {
  /** The player's buffering-visible flag (`showBuffering`). */
  buffering: Ref<boolean>
  /** First frame has played. */
  hasStarted: Ref<boolean>
  /** Terminal source-error message (null when none). */
  sourceError: Ref<string | null>
  /** Cumulative fragments loaded for the current source (>0 ⇒ bytes flowing). */
  fragLoadedCount: Ref<number>
}

/**
 * Reactive connection health for the aePlayer corner indicator: `'ok' | 'slow' |
 * 'offline'`. Tracks `navigator.onLine` live and arms a grace timer so a normal
 * in-buffer hiccup never trips the "slow" state — only a sustained buffer (with
 * bytes still flowing) does. Pure classification lives in `connectionHealth.ts`.
 */
export function useConnectionHealth(deps: ConnectionHealthDeps): {
  connectionState: Ref<ConnectionState>
} {
  const hasWindow = typeof window !== 'undefined'
  const online = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
  const setOnline = () => { online.value = true }
  const setOffline = () => { online.value = false }
  if (hasWindow) {
    window.addEventListener('online', setOnline)
    window.addEventListener('offline', setOffline)
  }

  // `sustained`: buffering has stayed true past the grace threshold. The timer
  // arms when buffering flips true and clears the instant it flips false, so a
  // seek into an unbuffered region (which resolves quickly) never trips it.
  const sustained = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null
  const clearTimer = () => {
    if (timer) { clearTimeout(timer); timer = null }
  }
  watch(deps.buffering, (on) => {
    clearTimer()
    if (on) {
      timer = setTimeout(() => { sustained.value = true }, SLOW_SUSTAINED_MS)
    } else {
      sustained.value = false
    }
  }, { immediate: true })

  const connectionState = computed<ConnectionState>(() =>
    classifyConnection({
      online: online.value,
      buffering: deps.buffering.value,
      hasStarted: deps.hasStarted.value,
      bytesFlowing: deps.fragLoadedCount.value > 0,
      sustained: sustained.value,
      hasError: deps.sourceError.value != null,
    }),
  )

  onScopeDispose(() => {
    clearTimer()
    if (hasWindow) {
      window.removeEventListener('online', setOnline)
      window.removeEventListener('offline', setOffline)
    }
  })

  return { connectionState }
}
