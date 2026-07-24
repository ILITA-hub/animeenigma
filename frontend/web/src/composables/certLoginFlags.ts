/**
 * Shared TLS-cert auto-login suppression flag keys + clearer (spec
 * 2026-07-24). Leaf module — no app-local imports — so both the auth store
 * and useCertAutoLogin can depend on it without a circular import between
 * the two.
 */
export const CERT_SUPPRESS_KEY = 'ae_cert_suppress'
export const CERT_NEG_CACHE_KEY = 'ae_cert_nolgn_until'

/**
 * Clears the logout-suppression flag and the 24h negative probe cache.
 * Called from login/register/passkeyLogin/checkDeepLink (manual auth paths)
 * so a previous session's auto-login suppression doesn't linger, and from
 * anywhere else that needs to reset auto-login state.
 */
export function clearCertSuppressionFlags() {
  try {
    localStorage.removeItem(CERT_SUPPRESS_KEY)
    localStorage.removeItem(CERT_NEG_CACHE_KEY)
  } catch { /* privacy modes */ }
}
