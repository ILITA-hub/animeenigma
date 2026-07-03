import { userApi } from '@/api/client'
import { enqueuePending, drainPending } from './registry'

/** Fire-and-forget: buffer a failed watch-progress write for later sync.
 *  Duplicates are harmless — the endpoint upserts per (anime, episode). */
export function queueProgress(payload: Record<string, unknown>): void {
  void enqueuePending(payload).catch(() => {})
}

export async function flushPendingProgress(
  post: (payload: Record<string, unknown>) => Promise<unknown> = (p) => userApi.updateProgress(p),
): Promise<boolean> {
  return drainPending(async (payload) => {
    try {
      await post(payload as Record<string, unknown>)
      return true
    } catch {
      return false
    }
  })
}

let installed = false
export function installProgressFlush(): void {
  if (installed) return
  installed = true
  window.addEventListener('online', () => void flushPendingProgress())
  void flushPendingProgress() // app start — drain anything left from last session
}
