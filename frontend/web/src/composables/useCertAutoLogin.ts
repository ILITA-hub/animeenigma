import { useAuthStore } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import i18n from '@/i18n'
import { CERT_SUPPRESS_KEY, CERT_NEG_CACHE_KEY } from '@/composables/certLoginFlags'

// Re-exported for backward-compat with the spec's expected exports from
// this module; canonical definitions live in the leaf `certLoginFlags`
// module (shared with stores/auth.ts to avoid a duplicated/circular pair).
export { CERT_SUPPRESS_KEY, CERT_NEG_CACHE_KEY }

const NEG_CACHE_MS = 24 * 60 * 60 * 1000
const PROBE_TIMEOUT_MS = 2500

function lsGet(key: string): string | null {
  try { return localStorage.getItem(key) } catch { return null }
}
function lsSet(key: string, val: string) {
  try { localStorage.setItem(key, val) } catch { /* privacy modes */ }
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
    // credentials: 'include' is required even though this is a same-site-less
    // cross-origin GET with no cookies to send: an uncredentialed cross-origin
    // fetch never presents a TLS client certificate (Fetch spec §"HTTP-network
    // fetch" ties client-cert auth to the credentials mode; Chrome's socket
    // pools additionally won't reuse/open a cert-bearing connection for a
    // no-credentials request). Without this, the browser silently skips the
    // cert picker and the probe always falls through to a plain 401/403.
    const res = await fetch(`${base.replace(/\/$/, '')}/cert-login`, {
      credentials: 'include',
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
