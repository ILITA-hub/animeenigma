// Self-managed SW registration. Not using virtual:pwa-register — its autoUpdate
// client reloads unconditionally on controllerchange, which would kill playback
// mid-episode. We reload only when nothing is playing (deploy mid-anime waits).

/** Injected lazily to avoid a static pwa→offline dependency; set by the
 *  offline engine's module when it loads. Absent ⇒ no downloads happening. */
let activeDownloadProbe: () => boolean = () => false
export function setActiveDownloadProbe(probe: () => boolean): void {
  activeDownloadProbe = probe
}

/** True while media is actively playing: any HTML5 <video> mid-playback, a
 *  Kodik iframe mounted (classic fallback — its playback state is opaque, so
 *  its mere presence defers), or an offline download is in flight (a deploy
 *  reload mid-download kills the foreground engine and orphans the record). */
export function shouldDeferReload(doc: Document): boolean {
  const videos = Array.from(doc.querySelectorAll('video'))
  if (videos.some((v) => !v.paused && !v.ended && v.readyState > 2)) return true
  if (activeDownloadProbe()) return true
  if (doc.querySelector('iframe[src*="kodik"]')) return true
  return false
}

/** Reload now, or poll every 15s until playback stops (deferred deploy pickup). */
export function scheduleReload(doc: Document, reload: () => void): void {
  if (!shouldDeferReload(doc)) {
    reload()
    return
  }
  const timer = setInterval(() => {
    if (!shouldDeferReload(doc)) {
      clearInterval(timer)
      reload()
    }
  }, 15_000)
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

async function unregisterAll(): Promise<void> {
  const regs = await navigator.serviceWorker.getRegistrations()
  await Promise.all(regs.map((r) => r.unregister()))
  // ONLY the workbox app-shell caches. ae-offline-* holds user-downloaded
  // episodes — the kill-switch disables a broken SW, it must never destroy
  // user data (downloads become unplayable until re-registration, not gone).
  const keys = await caches.keys()
  await Promise.all(keys.filter((k) => k.startsWith('workbox-')).map((k) => caches.delete(k)))
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
