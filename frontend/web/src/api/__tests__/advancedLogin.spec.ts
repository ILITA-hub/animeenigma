/**
 * Vitest spec for the advanced-login API client (spec 2026-07-24, Task 8) and
 * the pure base64→Blob helper the modal uses to download the issued .p12.
 * Mocks `apiClient` so the envelope-tolerant `unwrap` + endpoint wiring are
 * verified without a network.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { getMock, postMock, deleteMock, putMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  deleteMock: vi.fn(),
  putMock: vi.fn(),
}))
vi.mock('@/api/client', () => ({
  apiClient: {
    defaults: { baseURL: '/api' },
    get: getMock,
    post: postMock,
    delete: deleteMock,
    put: putMock,
  },
}))

import { advancedLoginApi, unwrap } from '../advancedLogin'
import { base64ToBlob } from '@/composables/useAdvancedLogin'

beforeEach(() => {
  getMock.mockReset()
  postMock.mockReset()
  deleteMock.mockReset()
  putMock.mockReset()
})

describe('unwrap', () => {
  it('unwraps a {data:...} envelope', () => {
    expect(unwrap<number[]>({ data: { data: [1, 2] } })).toEqual([1, 2])
  })
  it('tolerates a bare payload', () => {
    expect(unwrap<number[]>({ data: [3, 4] })).toEqual([3, 4])
  })
})

describe('advancedLoginApi.listPasskeys', () => {
  it('returns the unwrapped rows for an enveloped response', async () => {
    getMock.mockResolvedValue({ data: { data: [{ id: 'p1', name: 'Key', created_at: 'x' }] } })
    const rows = await advancedLoginApi.listPasskeys()
    expect(getMock).toHaveBeenCalledWith('/auth/passkeys')
    expect(rows).toEqual([{ id: 'p1', name: 'Key', created_at: 'x' }])
  })
  it('returns [] when the payload is empty', async () => {
    getMock.mockResolvedValue({ data: null })
    expect(await advancedLoginApi.listPasskeys()).toEqual([])
  })
})

describe('advancedLoginApi.finishPasskey', () => {
  it('encodes ceremony + name into the query string and posts the attestation', async () => {
    postMock.mockResolvedValue({ data: { data: { id: 'p2' } } })
    const attestation = { id: 'cred', response: {} }
    await advancedLoginApi.finishPasskey('cer 1', 'My Laptop', attestation)
    expect(postMock).toHaveBeenCalledWith(
      '/auth/passkey/register/finish?ceremony=cer%201&name=My%20Laptop',
      attestation,
    )
  })
})

describe('advancedLoginApi.deletePasskey', () => {
  it('URL-encodes the id', async () => {
    deleteMock.mockResolvedValue({})
    await advancedLoginApi.deletePasskey('a/b')
    expect(deleteMock).toHaveBeenCalledWith('/auth/passkeys/a%2Fb')
  })
})

describe('advancedLoginApi.issueCert', () => {
  it('posts the name and returns the issue result', async () => {
    postMock.mockResolvedValue({
      data: { certificate: { id: 'c1' }, p12_base64: 'AA==', password: 'secret' },
    })
    const res = await advancedLoginApi.issueCert('Laptop')
    expect(postMock).toHaveBeenCalledWith('/auth/cert/issue', { name: 'Laptop' })
    expect(res.password).toBe('secret')
  })
})

describe('advancedLoginApi.setCertAutoLogin', () => {
  it('PUTs the enabled flag and unwraps the result', async () => {
    putMock.mockResolvedValue({ data: { data: { cert_auto_login: true } } })
    const res = await advancedLoginApi.setCertAutoLogin(true)
    expect(putMock).toHaveBeenCalledWith('/auth/profile/cert-auto-login', { enabled: true })
    expect(res.cert_auto_login).toBe(true)
  })
})

describe('base64ToBlob', () => {
  it('decodes base64 into a Blob of the expected byte length + type', async () => {
    // "hi" → base64 "aGk="
    const blob = base64ToBlob('aGk=')
    expect(blob.type).toBe('application/x-pkcs12')
    expect(blob.size).toBe(2)
    expect(await blob.text()).toBe('hi')
  })
})
