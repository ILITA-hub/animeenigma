import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import i18n from '@/i18n'

export const CERT_SUPPRESS_KEY = 'ae_cert_suppress'
export const CERT_NEG_CACHE_KEY = 'ae_cert_nolgn_until'
const NEG_CACHE_MS = 24 * 60 * 60 * 1000
const PROBE_TIMEOUT_MS = 2500

function lsGet(key: string): string | null {
  try { return localStorage.getItem(key) } catch { return null }
}
function lsSet(key: string, val: string) {
  try { localStorage.setItem(key, val) } catch { /* privacy modes */ }
}

/**
 * Clears the logout-suppression flag and the 24h negative probe cache. The
 * store's `clearCertSuppressionFlags` (called from login/checkDeepLink/
 * passkeyLogin) covers the in-app manual-login paths; this export exists for
 * any other caller (and the spec) that needs to reset auto-login state
 * without going through the store.
 */
export function clearCertSuppression() {
  try {
    localStorage.removeItem(CERT_SUPPRESS_KEY)
    localStorage.removeItem(CERT_NEG_CACHE_KEY)
  } catch { /* privacy modes */ }
}

/**
 * Silently probes the mTLS vhost and, when the browser presents a valid
 * client certificate for a user with the auto-login toggle ON, exchanges the
 * returned one-time token for a session. Returns true when the user ended up
 * authenticated. Never throws.
 *
 * Skips (returns false immediately) when:
 *  - VITE_CERT_LOGIN_BASE is unset (feature off / dev)
 *  - logout suppression flag is set (cleared by the next manual login)
 *  - a recent probe answered "auto_login_disabled" (24h negative cache —
 *    avoids re-prompting toggled-off cert holders with the browser picker)
 */
export async function tryCertAutoLogin(): Promise<boolean> {
  const base = import.meta.env.VITE_CERT_LOGIN_BASE as string | undefined
  if (!base) return false
  if (lsGet(CERT_SUPPRESS_KEY) === '1') return false
  const negUntil = Number(lsGet(CERT_NEG_CACHE_KEY) || 0)
  if (negUntil && Date.now() < negUntil) return false

  try {
    const res = await fetch(`${base.replace(/\/$/, '')}/cert-login`, {
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    })
    if (res.status === 403) {
      const body = await res.json().catch(() => null)
      if (body?.reason === 'auto_login_disabled') {
        lsSet(CERT_NEG_CACHE_KEY, String(Date.now() + NEG_CACHE_MS))
      }
      return false
    }
    if (!res.ok) return false
    const payload = await res.json().catch(() => null)
    const token = payload?.data?.token ?? payload?.token
    if (!token) return false
    const ok = await useAuthStore().consumeCertToken(token)
    if (ok) notifyCertLogin()
    return ok
  } catch {
    // Timeout, TLS failure, user dismissed the cert picker, network error —
    // all mean "no auto-login this time"; fall through to the login page.
    return false
  }
}

/**
 * Small success toast after a silent cert login (owner request 2026-07-24):
 * the login page never shows, so this is the only feedback the user gets.
 * Reuses the project's existing global toast queue (`useToast`, rendered by
 * `<Toaster />` in App.vue) rather than inventing a second notification
 * surface — no sessionStorage/App.vue banner needed.
 */
function notifyCertLogin() {
  useToast().push(i18n.global.t('auth.certAutoLoginSuccess'), 'success')
}
