import { describe, it, expect } from 'vitest'
import { cacheStorageMediaStore } from './mediaStore'

function fakeCaches() {
  const stores = new Map<string, Map<string, Response>>()
  return {
    stores,
    impl: {
      async open(name: string) {
        if (!stores.has(name)) stores.set(name, new Map())
        const m = stores.get(name)!
        return {
          async put(k: string, r: Response) { m.set(k, r) },
          async match(k: string) { return m.get(k)?.clone() },
        } as unknown as Cache
      },
      async delete(name: string) { return stores.delete(name) },
      async has(name: string) { return stores.has(name) },
      async keys() { return [...stores.keys()] },
      async match() { return undefined },
    } as unknown as CacheStorage,
  }
}

describe('cacheStorageMediaStore', () => {
  it('put/has round-trips inside the per-id container', async () => {
    const { stores, impl } = fakeCaches()
    const s = cacheStorageMediaStore(impl)
    await s.put('d1', '/__offline/d1/r/0', new Response('x'))
    expect(await s.has('d1', '/__offline/d1/r/0')).toBe(true)
    expect(await s.has('d1', '/__offline/d1/r/1')).toBe(false)
    expect(stores.has('ae-offline-d1')).toBe(true)
  })
  it('remove/exists manage the container lifecycle', async () => {
    const { impl } = fakeCaches()
    const s = cacheStorageMediaStore(impl)
    await s.put('d1', '/p', new Response('x'))
    expect(await s.exists('d1')).toBe(true)
    expect(await s.remove('d1')).toBe(true)
    expect(await s.exists('d1')).toBe(false)
  })
  it('entryUrl emits the SW-served offline scheme', () => {
    const s = cacheStorageMediaStore(fakeCaches().impl)
    expect(s.entryUrl('a:1', 'master.m3u8')).toBe(`/__offline/${encodeURIComponent('a:1')}/master.m3u8`)
  })
})
