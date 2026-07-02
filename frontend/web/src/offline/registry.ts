import type { OfflineDownload } from './types'

const DB_NAME = 'ae-offline'
const DB_VERSION = 1
const DOWNLOADS = 'downloads'
const PENDING = 'pending_progress'

let dbPromise: Promise<IDBDatabase> | null = null

function openDb(): Promise<IDBDatabase> {
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION)
      req.onupgradeneeded = () => {
        const db = req.result
        if (!db.objectStoreNames.contains(DOWNLOADS)) db.createObjectStore(DOWNLOADS, { keyPath: 'id' })
        if (!db.objectStoreNames.contains(PENDING)) db.createObjectStore(PENDING, { autoIncrement: true })
      }
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
    })
  }
  return dbPromise
}

/** Test hook: fake-indexeddb persists per-process; reset between specs. */
export async function _resetDbForTests(): Promise<void> {
  if (dbPromise) (await dbPromise).close()
  dbPromise = null
  await new Promise<void>((resolve) => {
    const req = indexedDB.deleteDatabase(DB_NAME)
    req.onsuccess = req.onerror = req.onblocked = () => resolve()
  })
}

function reqAsPromise<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

async function store(name: string, mode: IDBTransactionMode): Promise<IDBObjectStore> {
  return (await openDb()).transaction(name, mode).objectStore(name)
}

export async function putDownload(d: OfflineDownload): Promise<void> {
  await reqAsPromise((await store(DOWNLOADS, 'readwrite')).put(d))
}

export async function getDownload(id: string): Promise<OfflineDownload | undefined> {
  return reqAsPromise((await store(DOWNLOADS, 'readonly')).get(id))
}

export async function listDownloads(): Promise<OfflineDownload[]> {
  return reqAsPromise((await store(DOWNLOADS, 'readonly')).getAll())
}

export async function deleteDownloadRecord(id: string): Promise<void> {
  await reqAsPromise((await store(DOWNLOADS, 'readwrite')).delete(id))
}

// ── pending watch-progress queue (offline playback → flushed when online) ──

export async function enqueuePending(payload: unknown): Promise<void> {
  await reqAsPromise((await store(PENDING, 'readwrite')).add({ payload, queuedAt: Date.now() }))
}

/** FIFO-drain: handler returns true ⇒ entry deleted; false ⇒ stop, keep rest.
 *  Returns true when the queue fully drained. */
export async function drainPending(handler: (payload: unknown) => Promise<boolean>): Promise<boolean> {
  const s = await store(PENDING, 'readonly')
  const keys = await reqAsPromise(s.getAllKeys())
  const values = await reqAsPromise(s.getAll())
  for (let i = 0; i < keys.length; i++) {
    const ok = await handler((values[i] as { payload: unknown }).payload).catch(() => false)
    if (!ok) return false
    await reqAsPromise((await store(PENDING, 'readwrite')).delete(keys[i]))
  }
  return true
}
