/**
 * Best-effort image preloader: resolves when the image is decoded, errored,
 * or the timeout fires — NEVER rejects. Used to warm the browser cache
 * before swapping a visible <img>/backdrop src so the swap doesn't flash
 * an empty box while the network round-trip happens (spotlight reroll,
 * 2026-06-11).
 */
export function preloadImage(src: string, timeoutMs = 5000): Promise<void> {
  return new Promise((resolve) => {
    if (!src) {
      resolve()
      return
    }
    let settled = false
    const done = (): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      resolve()
    }
    const timer = setTimeout(done, timeoutMs)
    const img = new Image()
    img.onload = done
    img.onerror = done
    img.src = src
  })
}
