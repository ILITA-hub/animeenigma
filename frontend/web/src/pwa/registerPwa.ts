// Self-managed SW registration. Not using virtual:pwa-register — its autoUpdate
// client reloads unconditionally on controllerchange, which would kill playback
// mid-episode. Policy: a visible tab is never reloaded spontaneously — the
// update lands when the tab is hidden (and nothing defers) or on the next
// cross-page router navigation (shouldFullReloadOnNav guard in the router),
// where a full load is indistinguishable from the SPA transition anyway.

/** Injected lazily to avoid a static pwa→offline dependency; set by the
 *  offline engine's module when it loads. Absent ⇒ no downloads happening. */
let activeDownloadProbe: () => boolean = () => false
export function setActiveDownloadProbe(probe: () => boolean): void {
  activeDownloadProbe = probe
}

/** Injected by the router (which owns route meta): true while the current
 *  route hosts a live WS session (`meta.liveSession` — watch-together / game
 *  rooms) that a reload would kick the user out of. */
let liveSessionProbe: () => boolean = () => false
export function setLiveSessionProbe(probe: () => boolean): void {
  liveSessionProbe = probe
}

/** Field types whose value is user-typed — losing it on reload hurts. */
const DRAFT_FIELD_TYPES = new Set([
  'textarea', 'text', 'search', 'email', 'url', 'tel', 'password', 'number',
])

/** Unsent user-typed content anywhere in the document — not just the focused
 *  element: ctrl+clicking a link blurs the field, but the draft still exists. */
function hasTextDraft(doc: Document): boolean {
  for (const f of doc.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('textarea, input')) {
    if (DRAFT_FIELD_TYPES.has(f.type) && !f.disabled && !f.readOnly && f.value.trim() !== '') {
      return true
    }
  }
  const editables = doc.querySelectorAll(
    '[contenteditable="true"], [contenteditable=""], [contenteditable="plaintext-only"]',
  )
  for (const e of editables) {
    if ((e.textContent ?? '').trim() !== '') return true
  }
  return false
}

/** True while a reload would interrupt or destroy something the user cares
 *  about: an in-flight offline download (a reload kills the foreground engine
 *  and orphans the record), a live watch-together / game room session, a
 *  playing <video>, a mounted Kodik iframe (its playback state is opaque, so
 *  its mere presence defers), an open modal ([role=dialog] — closed modals
 *  are v-if'd out of the DOM), or an unsent text draft. */
export function shouldDeferReload(doc: Document): boolean {
  if (activeDownloadProbe()) return true
  if (liveSessionProbe()) return true
  for (const v of doc.querySelectorAll('video')) {
    if (!v.paused && !v.ended && v.readyState > 2) return true
  }
  if (doc.querySelector('iframe[src*="kodik"]')) return true
  if (doc.querySelector('[role="dialog"], [role="alertdialog"]')) return true
  return hasTextDraft(doc)
}

let cancelPending: (() => void) | null = null

export function cancelPendingReload(): void {
  cancelPending?.()
  cancelPending = null
}

/** Router-guard hook: with an update pending, a cross-page navigation becomes
 *  a full page load of `to.fullPath` — the user already expects a screen
 *  change, and it retires the stale-chunk window early. Route-local state
 *  (playback, drafts, rooms) dies with the SPA transition anyway; only an
 *  in-flight download must survive the page, so only it blocks. Same-path
 *  navigations (in-player episode/provider query sync) never qualify — a full
 *  load there would kill active playback. */
export function shouldFullReloadOnNav(to: { path: string }, from: { path: string }): boolean {
  return cancelPending !== null && !activeDownloadProbe() && to.path !== from.path
}

/** Arm the deferred reload: fire as soon as the tab is hidden and nothing in
 *  shouldDeferReload objects — immediately if already hidden, on
 *  visibilitychange otherwise, with a 15s poll to catch deferral conditions
 *  (playback, drafts) clearing while the tab stays hidden. */
export function scheduleReload(doc: Document, reload: () => void): void {
  cancelPendingReload()
  const attempt = () => {
    if (doc.visibilityState !== 'hidden' || shouldDeferReload(doc)) return
    cancelPendingReload()
    reload()
  }
  const timer = setInterval(attempt, 15_000)
  doc.addEventListener('visibilitychange', attempt)
  cancelPending = () => {
    clearInterval(timer)
    doc.removeEventListener('visibilitychange', attempt)
  }
  attempt()
}

async function killSwitchActive(): Promise<boolean> {
  try {
    const r = await fetch('/sw-config.json', { cache: 'no-cache' })
    if (!r.ok) return false
    const cfg = (await r.json()) as { kill?: boolean }
    return cfg.kill === true
  } catch {
    return false // config unreachable ≠ kill
  }
}

/** Cache Storage keys the kill-switch purges: the workbox app-shell precache,
 *  and ae-seg-* (segmentCache.ts) — the scrub-preview segment tee, disposable
 *  and rebuilt from network on demand. ae-offline-* (user-downloaded episodes)
 *  deliberately does NOT match — the kill-switch disables a broken SW, it
 *  must never destroy user data (downloads become unplayable until
 *  re-registration, not gone). */
export function shouldPurgeCacheKey(key: string): boolean {
  return key.startsWith('workbox-') || key.startsWith('ae-seg-')
}

async function unregisterAll(): Promise<void> {
  const regs = await navigator.serviceWorker.getRegistrations()
  await Promise.all(regs.map((r) => r.unregister()))
  const keys = await caches.keys()
  await Promise.all(keys.filter(shouldPurgeCacheKey).map((k) => caches.delete(k)))
}

export async function initPwa(): Promise<void> {
  if (!('serviceWorker' in navigator)) return
  if (!import.meta.env.PROD) return // sw.js only exists in built output

  if (await killSwitchActive()) {
    await unregisterAll().catch(() => {})
    return
  }

  let hadController = !!navigator.serviceWorker.controller
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (!hadController) {
      hadController = true // first install claiming the page — not an update
      return
    }
    scheduleReload(document, () => window.location.reload())
  })

  try {
    const reg = await navigator.serviceWorker.register('/sw.js', { scope: '/' })
    // Long-lived SPA sessions: re-check hourly and whenever the tab returns.
    setInterval(() => void reg.update(), 60 * 60 * 1000)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') void reg.update()
    })
  } catch {
    // registration failure is non-fatal — app works SW-less
  }
}
