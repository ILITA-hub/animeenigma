/**
 * Vitest spec for tryCertAutoLogin() — TLS-cert auto-login probe (spec
 * 2026-07-24, Task 7). Mocks `fetch` + the auth store's `consumeCertToken`
 * and `useToast`'s `push` to verify the skip/negative-cache/success/failure
 * contract without hitting a real mTLS vhost.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import {
  tryCertAutoLogin,
  tryCertAutoLoginOnce,
  CERT_SUPPRESS_KEY,
  CERT_NEG_CACHE_KEY,
  CERT_BOOTSTRAP_PROBED_KEY,
} from '../useCertAutoLogin'

const fetchMock = vi.fn()

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  sessionStorage.clear()
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  vi.stubEnv('VITE_CERT_LOGIN_BASE', 'https://cert.example')
  // Drain any toasts left over from a previous test.
  const { toasts, dismiss } = useToast()
  ;[...toasts.value].forEach(t => dismiss(t.id))
})

describe('tryCertAutoLogin', () => {
  it('(a) returns false and never calls fetch when VITE_CERT_LOGIN_BASE is unset', async () => {
    vi.stubEnv('VITE_CERT_LOGIN_BASE', '')
    const result = await tryCertAutoLogin()
    expect(result).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('(b) returns false and never calls fetch when the logout suppression flag is set', async () => {
    localStorage.setItem(CERT_SUPPRESS_KEY, '1')
    const result = await tryCertAutoLogin()
    expect(result).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('(c) on a 403 auto_login_disabled response, returns false and sets the 24h negative cache', async () => {
    fetchMock.mockResolvedValue({
      status: 403,
      ok: false,
      json: async () => ({ reason: 'auto_login_disabled' }),
    })
    const before = Date.now()
    const result = await tryCertAutoLogin()
    expect(result).toBe(false)
    const cached = Number(localStorage.getItem(CERT_NEG_CACHE_KEY))
    expect(cached).toBeGreaterThan(before)
  })

  it('(d) on 200 {token}, calls consumeCertToken and returns its result', async () => {
    fetchMock.mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({ token: 'one-time-token' }),
    })
    const auth = useAuthStore()
    const consumeSpy = vi.spyOn(auth, 'consumeCertToken').mockResolvedValue(true)

    const result = await tryCertAutoLogin()

    expect(consumeSpy).toHaveBeenCalledWith('one-time-token')
    expect(result).toBe(true)
  })

  it('(d2) also unwraps an envelope-wrapped {data:{token}} response', async () => {
    fetchMock.mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({ data: { token: 'wrapped-token' } }),
    })
    const auth = useAuthStore()
    const consumeSpy = vi.spyOn(auth, 'consumeCertToken').mockResolvedValue(true)

    const result = await tryCertAutoLogin()

    expect(consumeSpy).toHaveBeenCalledWith('wrapped-token')
    expect(result).toBe(true)
  })

  it('(e) returns false when fetch throws (timeout/TLS failure/network error)', async () => {
    fetchMock.mockRejectedValue(new Error('network error'))
    const result = await tryCertAutoLogin()
    expect(result).toBe(false)
  })

  it('(f) pushes the success toast when consumeCertToken succeeds', async () => {
    fetchMock.mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({ token: 'tok' }),
    })
    const auth = useAuthStore()
    vi.spyOn(auth, 'consumeCertToken').mockResolvedValue(true)

    await tryCertAutoLogin()

    const { toasts } = useToast()
    expect(toasts.value).toHaveLength(1)
    expect(toasts.value[0].type).toBe('success')
  })

  it('(f) does NOT push a toast when consumeCertToken fails', async () => {
    fetchMock.mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({ token: 'tok' }),
    })
    const auth = useAuthStore()
    vi.spyOn(auth, 'consumeCertToken').mockResolvedValue(false)

    const result = await tryCertAutoLogin()

    expect(result).toBe(false)
    const { toasts } = useToast()
    expect(toasts.value).toHaveLength(0)
  })

  it('(g) concurrent probes share one in-flight fetch (bootstrap + guard race)', async () => {
    fetchMock.mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({ token: 'tok' }),
    })
    const auth = useAuthStore()
    vi.spyOn(auth, 'consumeCertToken').mockResolvedValue(true)

    const [a, b] = await Promise.all([tryCertAutoLogin(), tryCertAutoLogin()])

    expect(a).toBe(true)
    expect(b).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('tryCertAutoLoginOnce', () => {
  it('probes on the first call, then skips for the rest of the session', async () => {
    fetchMock.mockResolvedValue({ status: 401, ok: false, json: async () => ({}) })

    await tryCertAutoLoginOnce()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sessionStorage.getItem(CERT_BOOTSTRAP_PROBED_KEY)).toBe('1')

    await tryCertAutoLoginOnce()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
