/**
 * Route-level data prefetch (page-load waterfall optimization, 2026-06-11).
 *
 * The anime page used to discover its API calls only after the route chunk
 * downloaded AND executed — at a ~300ms RTT that's a full extra round-trip
 * before `/api/anime/{id}` even started. A route guard now fires the request
 * at navigation start and stashes the promise here; the view consumes it
 * instead of re-issuing the request.
 *
 * Semantics:
 *  - single-use: consuming a key removes it (post-mutation refetches must hit
 *    the network);
 *  - short TTL: an unconsumed entry expires so a stale response can never be
 *    served to a much later visit;
 *  - rejections are preserved for the consumer (the view owns error UX), but
 *    pre-handled here so an unconsumed failure doesn't surface as an
 *    unhandledrejection (which would trigger the chunk-reload listener).
 */

const TTL_MS = 15_000

interface StashEntry {
  promise: Promise<unknown>
  ts: number
}

const stash = new Map<string, StashEntry>()

/** Fire `factory()` and stash the promise under `key` (no-op if already stashed). */
export function stashPrefetch(key: string, factory: () => Promise<unknown>): void {
  const existing = stash.get(key)
  if (existing && Date.now() - existing.ts < TTL_MS) return
  let promise: Promise<unknown>
  try {
    promise = factory()
  } catch {
    return
  }
  // Swallow at the stash level only — the consumer still receives the
  // original (rejected) promise and handles the error itself.
  promise.catch(() => undefined)
  stash.set(key, { promise, ts: Date.now() })
}

/** Take the stashed promise for `key`, or null when absent/expired. Single-use. */
export function consumePrefetch<T>(key: string): Promise<T> | null {
  const entry = stash.get(key)
  if (!entry) return null
  stash.delete(key)
  if (Date.now() - entry.ts >= TTL_MS) return null
  return entry.promise as Promise<T>
}
