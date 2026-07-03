import { isAxiosError } from 'axios'
import { userApi } from '@/api/client'
import { enqueuePending, drainPending } from './registry'

/** 4xx = the server understood and refused — retrying the same payload can
 *  never succeed, so drop it rather than poison the FIFO queue. Exceptions
 *  that ARE retryable: 401 (token may refresh later), 408/429 (transient). */
function isNonRetryable(e: unknown): boolean {
  if (!isAxiosError(e) || !e.response) return false // network/CORS/5xx-less → retry
  const s = e.response.status
  if (s === 401 || s === 408 || s === 429) return false
  return s >= 400 && s < 500
}

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
    } catch (e) {
      return isNonRetryable(e)
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
