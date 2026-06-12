import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
}))

import { postKeepalive } from '../authBeacon'

describe('postKeepalive', () => {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true })

  beforeEach(() => {
    fetchMock.mockClear()
    vi.stubGlobal('fetch', fetchMock)
    localStorage.clear()
  })

  it('POSTs with keepalive and the Bearer token from localStorage', () => {
    localStorage.setItem('token', 'jwt-123')
    postKeepalive('/users/progress', { anime_id: 'a1', progress: 42 })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/users/progress')
    expect(init.method).toBe('POST')
    expect(init.keepalive).toBe(true)
    expect(init.headers.Authorization).toBe('Bearer jwt-123')
    expect(JSON.parse(init.body)).toEqual({ anime_id: 'a1', progress: 42 })
  })

  it('omits the Authorization header when no token is stored', () => {
    postKeepalive('/users/progress', { x: 1 })
    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.Authorization).toBeUndefined()
  })

  it('never throws — even when fetch itself blows up at unload time', () => {
    fetchMock.mockImplementation(() => {
      throw new Error('document is unloading')
    })
    expect(() => postKeepalive('/users/progress', { x: 1 })).not.toThrow()
  })
})
