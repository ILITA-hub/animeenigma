import { ref, computed } from 'vue'
import { advancedLoginApi, type Passkey, type Cert, type CAInfo } from '@/api/advancedLogin'
import { useAuthStore } from '@/stores/auth'
import { clearCertSuppressionFlags } from '@/composables/certLoginFlags'

/**
 * Orchestration for the «Продвинутый логин» modal (spec 2026-07-24, Task 8):
 * loads/mutates passkeys + TLS certs and keeps the auth store's
 * `cert_auto_login` flag in sync. Pure API calls live in `@/api/advancedLogin`;
 * this layer holds reactive state + the browser-only side effects (WebAuthn
 * ceremony, .p12 download, suppression-flag clearing).
 *
 * Fresh state per call (mirrors `useSessions`), so the modal owns its own copy.
 */

/** Is WebAuthn available in this browser? (Section is hidden otherwise.) */
export function isPasskeySupported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential
}

/**
 * Decode base64 → Blob. Pure + DOM-free (Uint8Array only) so it's unit
 * testable; the caller wraps it in a URL.createObjectURL download.
 */
export function base64ToBlob(base64: string, type = 'application/x-pkcs12'): Blob {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return new Blob([bytes], { type })
}

/** Trigger a browser download of `blob` under `filename`. */
function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** Slugify a cert name into a filename-safe stem (fallback: "cert"). */
function certFilename(name: string): string {
  const stem = name.trim().replace(/[^\w.-]+/g, '-').replace(/^-+|-+$/g, '') || 'cert'
  return `animeenigma-${stem}.p12`
}

export function useAdvancedLogin() {
  const auth = useAuthStore()

  const passkeys = ref<Passkey[]>([])
  const certs = ref<Cert[]>([])
  const passkeysLoading = ref(false)
  const certsLoading = ref(false)
  const passkeyBusy = ref(false)
  const certBusy = ref(false)
  const autoLoginBusy = ref(false)
  const error = ref<string | null>(null)

  /** Password from the most recent issue — shown ONCE; cleared on close. */
  const issuedPassword = ref<string | null>(null)

  /** Platform CA identity (fingerprints for the OS trust-prompt check). */
  const caInfo = ref<CAInfo | null>(null)

  const hasActiveCert = computed(() =>
    certs.value.some(c => !c.revoked_at),
  )
  const certAutoLogin = computed(() => !!auth.user?.cert_auto_login)

  function fail(e: unknown): void {
    error.value = (e as { response?: { data?: { error?: { message?: string } } }; message?: string })
      ?.response?.data?.error?.message
      ?? (e as Error)?.message
      ?? 'error'
  }

  // ── Passkeys ──
  async function refreshPasskeys(): Promise<void> {
    if (!isPasskeySupported()) return
    passkeysLoading.value = true
    error.value = null
    try {
      passkeys.value = await advancedLoginApi.listPasskeys()
    } catch (e) {
      fail(e)
    } finally {
      passkeysLoading.value = false
    }
  }

  /** Register a new passkey. Returns true on success. */
  async function addPasskey(name: string): Promise<boolean> {
    const label = name.trim()
    if (!label || passkeyBusy.value) return false
    passkeyBusy.value = true
    error.value = null
    try {
      const { startRegistration } = await import('@simplewebauthn/browser')
      const begin = await advancedLoginApi.beginPasskey()
      const attestation = await startRegistration({
        optionsJSON: (begin.options.publicKey as Parameters<typeof startRegistration>[0]['optionsJSON']),
      })
      await advancedLoginApi.finishPasskey(begin.ceremony_id, label, attestation)
      await refreshPasskeys()
      return true
    } catch (e) {
      // User dismissed the browser prompt — not an error banner.
      if ((e as { name?: string })?.name === 'NotAllowedError') return false
      fail(e)
      return false
    } finally {
      passkeyBusy.value = false
    }
  }

  async function deletePasskey(id: string): Promise<void> {
    error.value = null
    try {
      await advancedLoginApi.deletePasskey(id)
      passkeys.value = passkeys.value.filter(p => p.id !== id)
    } catch (e) {
      fail(e)
    }
  }

  // ── TLS certificates ──
  async function refreshCerts(): Promise<void> {
    certsLoading.value = true
    error.value = null
    try {
      // CA identity is immutable per installation — fetch once per session,
      // in parallel with the certs list (independent requests).
      const [certsResult, caInfoResult] = await Promise.all([
        advancedLoginApi.listCerts(),
        caInfo.value ? Promise.resolve(caInfo.value) : advancedLoginApi.caInfo(),
      ])
      certs.value = certsResult
      caInfo.value = caInfoResult
    } catch (e) {
      fail(e)
    } finally {
      certsLoading.value = false
    }
  }

  /** Issue a cert, download the .p12, surface the one-time password. */
  async function issueCert(name: string): Promise<boolean> {
    const label = name.trim()
    if (!label || certBusy.value) return false
    certBusy.value = true
    error.value = null
    try {
      const result = await advancedLoginApi.issueCert(label)
      downloadBlob(base64ToBlob(result.p12_base64), certFilename(label))
      issuedPassword.value = result.password
      // This browser now clearly wants cert login — drop the suppression flags
      // so the next visit's silent auto-login probe is allowed to run.
      clearCertSuppressionFlags()
      await refreshCerts()
      return true
    } catch (e) {
      fail(e)
      return false
    } finally {
      certBusy.value = false
    }
  }

  async function revokeCert(id: string): Promise<void> {
    error.value = null
    try {
      await advancedLoginApi.revokeCert(id)
      await refreshCerts()
    } catch (e) {
      fail(e)
    }
  }

  /** Persist the auto-login toggle + reflect it in the auth store. */
  async function setCertAutoLogin(enabled: boolean): Promise<void> {
    if (autoLoginBusy.value) return
    autoLoginBusy.value = true
    error.value = null
    try {
      const { cert_auto_login } = await advancedLoginApi.setCertAutoLogin(enabled)
      if (auth.user) auth.setUser({ ...auth.user, cert_auto_login })
      // Re-enabling the toggle should let a fresh silent probe run right away —
      // otherwise a stale 24h negative cache from before the toggle was flipped
      // back on would keep suppressing auto-login until it expires.
      if (enabled && cert_auto_login) clearCertSuppressionFlags()
    } catch (e) {
      fail(e)
    } finally {
      autoLoginBusy.value = false
    }
  }

  /** Clear the one-time password (call when the modal closes). */
  function clearIssuedPassword(): void {
    issuedPassword.value = null
  }

  return {
    passkeys,
    certs,
    passkeysLoading,
    certsLoading,
    passkeyBusy,
    certBusy,
    autoLoginBusy,
    error,
    issuedPassword,
    caInfo,
    hasActiveCert,
    certAutoLogin,
    refreshPasskeys,
    addPasskey,
    deletePasskey,
    refreshCerts,
    issueCert,
    revokeCert,
    setCertAutoLogin,
    clearIssuedPassword,
  }
}
