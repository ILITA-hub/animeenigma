/**
 * Anonymous user identity for instrumentation and anon-friendly endpoints.
 *
 * - Mints a UUIDv4 the first time the function is called and stores it in localStorage
 *   under key `aenig_anon_id`. Subsequent calls return the same value.
 * - On localStorage failure (private browsing, quota exceeded, disabled storage),
 *   returns an ephemeral UUID without persisting. The caller still gets a stable
 *   value within the current page lifecycle thanks to the module-scoped cache.
 * - Per CONTEXT D-11 / D-13 / D-14: no PII; UUIDv4 only; cleared with cookies.
 *
 * Phase 1 introduces this file so Phase 7 (D-01: anonymous localStorage preferences)
 * inherits it without re-inventing the key.
 */

const STORAGE_KEY = 'aenig_anon_id'
let cached: string | null = null

export function getOrCreateAnonId(): string {
  if (cached) return cached

  try {
    let id = localStorage.getItem(STORAGE_KEY)
    if (!id) {
      id = crypto.randomUUID()
      localStorage.setItem(STORAGE_KEY, id)
    }
    cached = id
    return id
  } catch {
    // localStorage unavailable (private mode, etc.) — generate ephemeral.
    // We still cache in module scope so the same value is reused for the rest
    // of the session, preventing axios interceptor + composable from minting
    // two different anon ids within one page lifecycle (Pitfall 5).
    const ephemeral = crypto.randomUUID()
    cached = ephemeral
    return ephemeral
  }
}
