// Storage port for downloaded media bytes. The download engine performs ALL
// byte I/O through this interface so future standalone apps (Capacitor/Tauri)
// can swap in a filesystem adapter without touching the engine, registry, or
// UI. The web adapter below is Cache Storage + SW-served /__offline/* URLs.
import { offlineCacheName, offlinePath } from './types'

export interface OfflineMediaStore {
  put(id: string, path: string, resp: Response): Promise<void>
  has(id: string, path: string): Promise<boolean>
  /** Drop the whole container for a download id. */
  remove(id: string): Promise<boolean>
  /** Container still present? (eviction scan) */
  exists(id: string): Promise<boolean>
  persist(): Promise<void>
  estimate(): Promise<{ usage: number; quota: number } | null>
  /** Playable local URL for an entry. Web: /__offline/{id}/{rest} (SW-served).
   *  A native adapter returns its own scheme (file/asset URL) here. */
  entryUrl(id: string, rest: string): string
}

export function cacheStorageMediaStore(cachesImpl: CacheStorage = caches): OfflineMediaStore {
  return {
    async put(id, path, resp) {
      const cache = await cachesImpl.open(offlineCacheName(id))
      await cache.put(path, resp)
    },
    async has(id, path) {
      const cache = await cachesImpl.open(offlineCacheName(id))
      return !!(await cache.match(path))
    },
    async remove(id) {
      return cachesImpl.delete(offlineCacheName(id))
    },
    async exists(id) {
      return cachesImpl.has(offlineCacheName(id))
    },
    async persist() {
      try {
        await (navigator as Navigator & { storage?: { persist?: () => Promise<boolean> } }).storage?.persist?.()
      } catch { /* best-effort */ }
    },
    async estimate() {
      try {
        const est = await navigator.storage?.estimate?.()
        if (!est?.quota) return null
        return { usage: est.usage ?? 0, quota: est.quota }
      } catch {
        return null
      }
    },
    entryUrl(id, rest) {
      return offlinePath(id, rest)
    },
  }
}
