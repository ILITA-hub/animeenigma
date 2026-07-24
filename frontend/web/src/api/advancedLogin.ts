import { apiClient } from './client'

/**
 * Advanced-login API client (spec 2026-07-24, Task 8): passkeys (WebAuthn
 * credentials) + TLS client certificates. Mirrors the `sessions.ts` idiom —
 * a thin object of typed methods over `apiClient`, envelope-tolerant so it
 * works whether the backend wraps rows in `{data:...}` or returns them bare.
 */

export interface Passkey {
  id: string
  name: string
  created_at: string
  last_used_at?: string | null
}

export interface Cert {
  id: string
  name: string
  serial?: string
  created_at: string
  not_after: string
  last_used_at?: string | null
  revoked_at?: string | null
}

/** Result of `POST /auth/cert/issue` — the .p12 bytes + one-time password. */
export interface CertIssueResult {
  certificate: Cert
  p12_base64: string
  password: string
}

/** Platform CA identity — shown so users can verify the OS trust prompt. */
export interface CAInfo {
  subject: string
  fingerprint_sha256: string
  fingerprint_sha1: string
}

/** Unwrap a `{data: T}` envelope, tolerating a bare `T` payload. */
export function unwrap<T>(res: { data?: unknown }): T {
  const body = res.data as { data?: unknown } | undefined
  return ((body?.data ?? body) as T)
}

export const advancedLoginApi = {
  // ── Passkeys ──
  async listPasskeys(): Promise<Passkey[]> {
    const res = await apiClient.get('/auth/passkeys')
    return unwrap<Passkey[]>(res) ?? []
  },

  /** Returns the ceremony id + WebAuthn creation options for registration. */
  async beginPasskey(): Promise<{ ceremony_id: string; options: { publicKey: unknown } }> {
    const res = await apiClient.post('/auth/passkey/register/begin')
    return unwrap<{ ceremony_id: string; options: { publicKey: unknown } }>(res)
  },

  /** Completes registration with the browser attestation for a labelled key. */
  async finishPasskey(ceremony: string, name: string, attestation: unknown): Promise<Passkey> {
    const res = await apiClient.post(
      `/auth/passkey/register/finish?ceremony=${encodeURIComponent(ceremony)}&name=${encodeURIComponent(name)}`,
      attestation,
    )
    return unwrap<Passkey>(res)
  },

  async deletePasskey(id: string): Promise<void> {
    await apiClient.delete(`/auth/passkeys/${encodeURIComponent(id)}`)
  },

  // ── TLS certificates ──
  async listCerts(): Promise<Cert[]> {
    const res = await apiClient.get('/auth/certs')
    return unwrap<Cert[]>(res) ?? []
  },

  async caInfo(): Promise<CAInfo> {
    const res = await apiClient.get('/auth/certs/ca')
    return unwrap<CAInfo>(res)
  },

  async issueCert(name: string): Promise<CertIssueResult> {
    const res = await apiClient.post('/auth/cert/issue', { name })
    return unwrap<CertIssueResult>(res)
  },

  async revokeCert(id: string): Promise<void> {
    await apiClient.delete(`/auth/certs/${encodeURIComponent(id)}`)
  },

  /** Toggle server-side "auto-login when a client cert is presented". */
  async setCertAutoLogin(enabled: boolean): Promise<{ cert_auto_login: boolean }> {
    const res = await apiClient.put('/auth/profile/cert-auto-login', { enabled })
    return unwrap<{ cert_auto_login: boolean }>(res)
  },
}
