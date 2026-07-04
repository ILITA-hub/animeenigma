/**
 * createFetchCache — module-level TTL + in-flight-coalescing bookkeeping for
 * composable fetch caches (extracted 2026-07-04 after useSpotlight and
 * useContinueWatching landed line-identical copies; ActivityFeed shares it
 * too). It owns ONLY staleness + request coalescing — callers keep their own
 * data refs, error handling, auth gates, and param keys, which genuinely
 * differ per site. Pinia stores keep their existing isFresh() variants.
 */
export function createFetchCache(ttlMs: number) {
  let cachedAt = 0
  let inFlight: Promise<void> | null = null
  return {
    isFresh: () => Date.now() - cachedAt < ttlMs,
    /** Call after a successful fetch stores its data. */
    markFresh: () => {
      cachedAt = Date.now()
    },
    invalidate: () => {
      cachedAt = 0
    },
    /** Run `produce` unless one is already in flight; concurrent callers
     *  share the same promise instead of racing duplicate requests. */
    share(produce: () => Promise<void>): Promise<void> {
      if (!inFlight) {
        inFlight = produce().finally(() => {
          inFlight = null
        })
      }
      return inFlight
    },
  }
}
