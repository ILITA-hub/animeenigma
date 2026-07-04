// src/offline/network.ts
// Wi-Fi-only download default: cellular detection + the session-scoped
// "download over mobile data anyway" override. Session-scoped on purpose —
// the safe default returns on every app launch.
interface ConnectionLike {
  type?: string
  addEventListener?: (t: 'change', cb: () => void) => void
  removeEventListener?: (t: 'change', cb: () => void) => void
}

function connection(): ConnectionLike | undefined {
  return (navigator as Navigator & { connection?: ConnectionLike }).connection
}

/** True only when the Network Information API positively identifies cellular.
 *  Unknown/absent type (desktop Chrome, Safari, Firefox) → false: never nag
 *  users the API can't classify. */
export function isCellular(): boolean {
  return connection()?.type === 'cellular'
}

let allowCellular = false
export function allowCellularThisSession(): boolean { return allowCellular }
export function setAllowCellularThisSession(v: boolean): void { allowCellular = v }

/** Subscribe to connectivity-type changes; returns an unsubscribe (no-op when the API is absent). */
export function onConnectionChange(cb: () => void): () => void {
  const c = connection()
  if (!c?.addEventListener) return () => {}
  c.addEventListener('change', cb)
  return () => c.removeEventListener?.('change', cb)
}

export function _resetNetworkForTests(): void { allowCellular = false }
