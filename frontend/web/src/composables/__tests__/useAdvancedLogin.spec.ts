/**
 * Vitest spec for useAdvancedLogin() — passkey + TLS-cert orchestration for
 * the "Продвинутый логин" modal (spec 2026-07-24, Task 8). Mocks
 * `@/api/advancedLogin` (the pure API layer, already covered by its own
 * spec), `@simplewebauthn/browser`'s dynamic-import target, and
 * `@/composables/certLoginFlags`'s `clearCertSuppressionFlags` so only the
 * composable's own state/sequencing logic is under test. Pinia is set up
 * fresh per test (mirrors useCertAutoLogin.spec.ts) since the composable
 * reads/writes the auth store's `cert_auto_login` field via `setUser`.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/api/advancedLogin', () => ({
  advancedLoginApi: {
    listPasskeys: vi.fn(),
    beginPasskey: vi.fn(),
    finishPasskey: vi.fn(),
    deletePasskey: vi.fn(),
    listCerts: vi.fn(),
    issueCert: vi.fn(),
    revokeCert: vi.fn(),
    setCertAutoLogin: vi.fn(),
  },
}))

vi.mock('@simplewebauthn/browser', () => ({
  startRegistration: vi.fn(),
}))

vi.mock('@/composables/certLoginFlags', () => ({
  clearCertSuppressionFlags: vi.fn(),
}))

import { advancedLoginApi } from '@/api/advancedLogin'
import { startRegistration } from '@simplewebauthn/browser'
import { clearCertSuppressionFlags } from '@/composables/certLoginFlags'
import { useAdvancedLogin } from '../useAdvancedLogin'
import type { Cert } from '@/api/advancedLogin'

const beginPasskeyMock = advancedLoginApi.beginPasskey as ReturnType<typeof vi.fn>
const finishPasskeyMock = advancedLoginApi.finishPasskey as ReturnType<typeof vi.fn>
const listCertsMock = advancedLoginApi.listCerts as ReturnType<typeof vi.fn>
const issueCertMock = advancedLoginApi.issueCert as ReturnType<typeof vi.fn>
const setCertAutoLoginMock = advancedLoginApi.setCertAutoLogin as ReturnType<typeof vi.fn>
const startRegistrationMock = startRegistration as ReturnType<typeof vi.fn>
const clearSuppressionMock = clearCertSuppressionFlags as ReturnType<typeof vi.fn>

function cert(overrides: Partial<Cert> = {}): Cert {
  return {
    id: 'c1',
    name: 'Laptop',
    created_at: '2026-07-01T00:00:00Z',
    not_after: '2027-07-01T00:00:00Z',
    revoked_at: null,
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  listCertsMock.mockResolvedValue([])
  // jsdom doesn't implement these — stub so issueCert's download side effect
  // can run without throwing.
  URL.createObjectURL = vi.fn(() => 'blob:mock')
  URL.revokeObjectURL = vi.fn()
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
})

describe('hasActiveCert', () => {
  it('is false when the cert list is empty', () => {
    const { certs, hasActiveCert } = useAdvancedLogin()
    certs.value = []
    expect(hasActiveCert.value).toBe(false)
  })

  it('is false when every cert is revoked', () => {
    const { certs, hasActiveCert } = useAdvancedLogin()
    certs.value = [cert({ revoked_at: '2026-07-02T00:00:00Z' }), cert({ id: 'c2', revoked_at: '2026-07-03T00:00:00Z' })]
    expect(hasActiveCert.value).toBe(false)
  })

  it('is true when at least one cert is not revoked', () => {
    const { certs, hasActiveCert } = useAdvancedLogin()
    certs.value = [cert({ id: 'c1', revoked_at: '2026-07-02T00:00:00Z' }), cert({ id: 'c2', revoked_at: null })]
    expect(hasActiveCert.value).toBe(true)
  })
})

describe('addPasskey', () => {
  it('swallows a NotAllowedError from startRegistration — no error state, no finish call', async () => {
    beginPasskeyMock.mockResolvedValue({ ceremony_id: 'cer1', options: { publicKey: {} } })
    const notAllowed = Object.assign(new Error('dismissed'), { name: 'NotAllowedError' })
    startRegistrationMock.mockRejectedValue(notAllowed)

    const { addPasskey, error, passkeyBusy } = useAdvancedLogin()
    const result = await addPasskey('My Key')

    expect(result).toBe(false)
    expect(error.value).toBeNull()
    expect(finishPasskeyMock).not.toHaveBeenCalled()
    expect(passkeyBusy.value).toBe(false)
  })

  it('sets error state for any other startRegistration failure', async () => {
    beginPasskeyMock.mockResolvedValue({ ceremony_id: 'cer1', options: { publicKey: {} } })
    startRegistrationMock.mockRejectedValue(new Error('boom'))

    const { addPasskey, error } = useAdvancedLogin()
    const result = await addPasskey('My Key')

    expect(result).toBe(false)
    expect(error.value).toBe('boom')
    expect(finishPasskeyMock).not.toHaveBeenCalled()
  })
})

describe('issueCert', () => {
  it('downloads the p12, exposes the one-time password, and clears cert suppression flags', async () => {
    issueCertMock.mockResolvedValue({
      certificate: cert(),
      p12_base64: 'aGk=',
      password: 'one-time-secret',
    })

    const { issueCert, issuedPassword, certBusy } = useAdvancedLogin()
    const result = await issueCert('Laptop')

    expect(result).toBe(true)
    expect(issueCertMock).toHaveBeenCalledWith('Laptop')
    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
    expect(issuedPassword.value).toBe('one-time-secret')
    expect(clearSuppressionMock).toHaveBeenCalledTimes(1)
    expect(listCertsMock).toHaveBeenCalledTimes(1) // refreshCerts() after issue
    expect(certBusy.value).toBe(false)
  })

  it('does not touch the download/password/suppression path when the API call fails', async () => {
    issueCertMock.mockRejectedValue(new Error('issue_failed'))

    const { issueCert, issuedPassword, error } = useAdvancedLogin()
    const result = await issueCert('Laptop')

    expect(result).toBe(false)
    expect(error.value).toBe('issue_failed')
    expect(issuedPassword.value).toBeNull()
    expect(clearSuppressionMock).not.toHaveBeenCalled()
  })
})

describe('setCertAutoLogin', () => {
  it('on a successful PUT, syncs the auth store user with the new flag', async () => {
    const auth = useAuthStore()
    auth.setUser({ id: 'u1', username: 'bob', email: 'b@x.com', role: 'user', cert_auto_login: false })
    const setUserSpy = vi.spyOn(auth, 'setUser')
    setCertAutoLoginMock.mockResolvedValue({ cert_auto_login: true })

    const { setCertAutoLogin, error } = useAdvancedLogin()
    await setCertAutoLogin(true)

    expect(setCertAutoLoginMock).toHaveBeenCalledWith(true)
    expect(setUserSpy).toHaveBeenCalledWith(expect.objectContaining({ id: 'u1', cert_auto_login: true }))
    expect(auth.user?.cert_auto_login).toBe(true)
    expect(error.value).toBeNull()
  })

  it('on a failed PUT, leaves the store untouched and sets error state', async () => {
    const auth = useAuthStore()
    auth.setUser({ id: 'u1', username: 'bob', email: 'b@x.com', role: 'user', cert_auto_login: false })
    const setUserSpy = vi.spyOn(auth, 'setUser')
    setCertAutoLoginMock.mockRejectedValue(new Error('put_failed'))

    const { setCertAutoLogin, error } = useAdvancedLogin()
    await setCertAutoLogin(true)

    expect(setUserSpy).not.toHaveBeenCalled()
    expect(auth.user?.cert_auto_login).toBe(false)
    expect(error.value).toBe('put_failed')
  })
})

describe('clearIssuedPassword (the reset the modal\'s open-watcher calls on close)', () => {
  it('clears the password state', async () => {
    issueCertMock.mockResolvedValue({
      certificate: cert(),
      p12_base64: 'aGk=',
      password: 'one-time-secret',
    })
    const { issueCert, issuedPassword, clearIssuedPassword } = useAdvancedLogin()
    await issueCert('Laptop')
    expect(issuedPassword.value).toBe('one-time-secret')

    clearIssuedPassword()

    expect(issuedPassword.value).toBeNull()
  })
})
