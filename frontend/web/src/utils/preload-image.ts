/**
 * Best-effort image preloader + session warm-URL registry.
 *
 * preloadImage resolves when the image is decoded, errored, or the timeout
 * fires — NEVER rejects. Used to warm the browser cache before swapping a
 * visible <img>/backdrop src (spotlight reroll + slide prefetch).
 *
 * The warm registry remembers every URL that finished loading this session
 * (via preload OR a real <img> @load). Consumers skip their skeleton/fade-in
 * for warm URLs — re-mounting a carousel slide must NOT replay the loading
 * choreography over an instant HTTP-cache hit (2026-06-11 feedback: «картинки
 * каждый раз перезагружаются заново» — they didn't, but the unconditional
 * shimmer+fade made cache hits LOOK like reloads).
 */

const warmUrls = new Set<string>()

/** True when `src` already finished loading once this session. */
export function isImageWarm(src: string): boolean {
  return warmUrls.has(src)
}

/** Record that `src` finished loading (call from <img> @load handlers). */
export function markImageWarm(src: string): void {
  if (src) warmUrls.add(src)
}

export function preloadImage(src: string, timeoutMs = 5000): Promise<void> {
  return new Promise((resolve) => {
    if (!src || warmUrls.has(src)) {
      resolve()
      return
    }
    let settled = false
    const done = (loadedOk: boolean): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      if (loadedOk) warmUrls.add(src)
      resolve()
    }
    const timer = setTimeout(() => done(false), timeoutMs)
    const img = new Image()
    img.onload = () => done(true)
    img.onerror = () => done(false)
    img.src = src
  })
}
