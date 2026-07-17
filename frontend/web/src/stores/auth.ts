import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiClient } from '@/api/client'
import i18n from '@/i18n'

export interface User {
  id: string
  username: string
  email: string
  avatar?: string
  role: string
  public_id?: string
  public_statuses?: string[]
  /** What of the user's activity other users can see (server-enforced). */
  activity_visibility?: 'all' | 'non_hentai' | 'none'
  created_at?: string
  timezone?: string
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface RegisterData {
  username: string
  password: string
  /** IANA zone captured at sign-up; register() fills it from the browser when omitted. */
  timezone?: string
}

// Helper to safely parse user from localStorage
function loadUserFromStorage(): User | null {
  try {
    const stored = localStorage.getItem('user')
    if (stored) {
      return JSON.parse(stored)
    }
  } catch {
    localStorage.removeItem('user')
  }
  return null
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(loadUserFromStorage())
  const token = ref<string | null>(localStorage.getItem('token'))
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isRefreshing = ref(false)

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  // Raw-library operator surface (/admin/raw-library): admins plus the
  // dedicated `librarian` role. Librarian grants nothing else admin-gated —
  // mirrors the gateway's LibraryRoleMiddleware on /api/library/*.
  const canAccessLibrary = computed(
    () => isAdmin.value || user.value?.role === 'librarian',
  )

  // ── Watch Together guest identity ──
  // DELIBERATELY separate from `token` / isAuthenticated: a guest holds a
  // role=guest JWT used ONLY for the Watch Together WS + WT REST calls.
  // Keeping it out of `token` means isAuthenticated stays false, so every
  // existing `v-if="isAuthenticated"` gate (lists, comments, reviews, the
  // Invite/create-room button) stays correctly hidden for guests — no
  // app-wide audit. sessionStorage-backed so a same-tab reload preserves the
  // guest's identity (and therefore their room member id).
  const GUEST_TOKEN_KEY = 'wt_guest_token'
  const GUEST_USER_KEY = 'wt_guest_user'

  function safeSessionGet(key: string): string | null {
    try {
      return sessionStorage.getItem(key)
    } catch {
      return null
    }
  }
  function safeSessionSet(key: string, val: string) {
    try {
      sessionStorage.setItem(key, val)
    } catch {
      // Privacy modes can throw — the guest identity just won't survive a reload.
    }
  }
  function loadGuestUser(): { id: string; username: string } | null {
    try {
      const s = sessionStorage.getItem(GUEST_USER_KEY)
      if (s) return JSON.parse(s)
    } catch {
      // ignore parse / access errors
    }
    return null
  }

  const wtGuestToken = ref<string | null>(safeSessionGet(GUEST_TOKEN_KEY))
  const wtGuestUser = ref<{ id: string; username: string } | null>(loadGuestUser())

  // Decode the guest JWT `exp` (seconds) and report whether it's within
  // `skewSeconds` of expiry. Mirrors the composable's tokenExpiringSoon so a
  // re-mint fires before a reconnect presents a stale token (ISS-019 family).
  function guestTokenExpiringSoon(skewSeconds = 60): boolean {
    const t = wtGuestToken.value
    if (!t) return true
    const parts = t.split('.')
    if (parts.length !== 3) return false
    try {
      const payload = JSON.parse(
        atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')),
      ) as { exp?: number }
      if (typeof payload.exp !== 'number') return false
      return payload.exp * 1000 <= Date.now() + skewSeconds * 1000
    } catch {
      return false
    }
  }

  // Returns a valid Watch Together guest JWT, minting a fresh one via
  // POST /auth/guest when absent or near expiry. Stored in `wtGuestToken`
  // (NOT `token`) so isAuthenticated stays false. Returns null on failure so
  // the caller can surface a connect error instead of looping. NOTE: a
  // re-mint creates a NEW guest identity (the server has no refresh path for
  // guests) — acceptable for the MVP given the 6h default TTL.
  async function ensureGuestToken(): Promise<string | null> {
    if (wtGuestToken.value && !guestTokenExpiringSoon()) {
      return wtGuestToken.value
    }
    try {
      const response = await apiClient.post('/auth/guest')
      const data = response.data?.data || response.data
      const tok = data?.access_token as string | undefined
      if (!tok) return null
      wtGuestToken.value = tok
      safeSessionSet(GUEST_TOKEN_KEY, tok)
      if (data.user?.id) {
        const gu = {
          id: String(data.user.id),
          username: String(data.user.username ?? 'Guest'),
        }
        wtGuestUser.value = gu
        safeSessionSet(GUEST_USER_KEY, JSON.stringify(gu))
      }
      return tok
    } catch {
      return null
    }
  }

  const setUser = (userData: User | null) => {
    user.value = userData
    if (userData) {
      localStorage.setItem('user', JSON.stringify(userData))
      // Clickstream identify (Plan 2) — tie subsequent events to this user.
      import('@/analytics').then(({ analytics }) => analytics.identify(userData.id)).catch(() => undefined)
    } else {
      localStorage.removeItem('user')
    }
  }

  const setToken = (accessToken: string) => {
    token.value = accessToken
    localStorage.setItem('token', accessToken)
  }

  const login = async (credentials: LoginCredentials) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/login', credentials)
      const data = response.data?.data || response.data
      setToken(data.access_token)
      setUser(data.user)
      // Phase 7 D-03 — auth state just changed; the previous user's resolved
      // combos must NOT survive into this user's session even if the prefs
      // version happens to match.
      clearPreferenceCache()
      return true
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
      error.value = e.response?.data?.error?.message || e.response?.data?.message || i18n.global.t('auth.loginError')
      return false
    } finally {
      loading.value = false
    }
  }

  // Phase 7 D-03 — wipe localStorage of cached preference resolutions and
  // the prefs_version stamp. Called on login/logout so a different user's
  // (or anonymous) cached combos don't leak across auth boundaries.
  function clearPreferenceCache() {
    for (let i = localStorage.length - 1; i >= 0; i--) {
      const key = localStorage.key(i)
      if (key && (key.startsWith('pref:') || key === 'prefs_version')) {
        localStorage.removeItem(key)
      }
    }
  }

  const register = async (data: RegisterData) => {
    loading.value = true
    error.value = null

    try {
      // Account timezone is set once at sign-up (browser-detected); later
      // changes go through Profile -> Settings only.
      const payload = { timezone: detectBrowserTimezone(), ...data }
      const response = await apiClient.post('/auth/register', payload)
      const respData = response.data?.data || response.data
      setToken(respData.access_token)
      setUser(respData.user)
      return true
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
      error.value = e.response?.data?.error?.message || e.response?.data?.message || i18n.global.t('auth.registerError')
      return false
    } finally {
      loading.value = false
    }
  }

  const refreshAccessToken = async (): Promise<boolean> => {
    if (isRefreshing.value) {
      return false
    }

    isRefreshing.value = true
    try {
      // Refresh token is sent automatically via httpOnly cookie
      const response = await apiClient.post('/auth/refresh')
      const data = response.data?.data || response.data
      setToken(data.access_token)
      if (data.user) {
        setUser(data.user)
      }
      return true
    } catch (err) {
      // Only logout when the server rejects the refresh token; transient
      // network/5xx errors (e.g. VPN reconnect) shouldn't drop the session.
      const status = (err as { response?: { status?: number } })?.response?.status
      if (status === 401 || status === 403) {
        logout()
      }
      return false
    } finally {
      isRefreshing.value = false
    }
  }

  const requestDeepLink = async (): Promise<{ token: string; deeplink_url: string; expires_in: number } | null> => {
    error.value = null
    try {
      const response = await apiClient.post('/auth/telegram/deeplink')
      return response.data?.data || response.data
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
      error.value = e.response?.data?.error?.message || e.response?.data?.message || i18n.global.t('auth.telegramLoginError')
      return null
    }
  }

  const checkDeepLink = async (token: string): Promise<{ status: string; access_token?: string; user?: User } | null> => {
    try {
      const response = await apiClient.get(`/auth/telegram/check?token=${token}`)
      const data = response.data?.data || response.data
      if (data.status === 'confirmed' && data.access_token) {
        setToken(data.access_token)
        setUser(data.user)
      }
      return data
    } catch {
      return { status: 'expired' }
    }
  }

  const logout = async () => {
    // Call logout endpoint to clear the httpOnly cookie
    try {
      await apiClient.post('/auth/logout')
    } catch {
      // Ignore errors, we're logging out anyway
    }
    setUser(null)
    token.value = null
    localStorage.removeItem('token')
    // Phase 7 D-03 — wipe pref:* + prefs_version on logout
    clearPreferenceCache()
    // Clickstream reset (Plan 2) — drop user id + rotate anon id.
    import('@/analytics').then(({ analytics }) => analytics.reset()).catch(() => undefined)
  }

  function detectBrowserTimezone(): string {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
    } catch {
      return 'UTC'
    }
  }

  // Accounts created before the timezone field existed (and Telegram sign-ups,
  // where the user is created server-side) have no zone — set it once from the
  // browser on first authenticated load. Fire-and-forget: display falls back
  // to the browser zone anyway, so a failed backfill costs nothing.
  function backfillTimezone(u: User | null) {
    if (!u || u.timezone) return
    const tz = detectBrowserTimezone()
    apiClient.put('/auth/profile/timezone', { timezone: tz })
      .then(() => { if (user.value && !user.value.timezone) setUser({ ...user.value, timezone: tz }) })
      .catch(() => { /* next load retries */ })
  }

  const fetchUser = async () => {
    if (!token.value) return

    loading.value = true
    try {
      const response = await apiClient.get('/auth/me')
      const userData = response.data?.data || response.data
      setUser(userData)
      backfillTimezone(userData)
    } catch (err) {
      // Only logout when the server says the token is invalid; transient
      // network/5xx errors (e.g. VPN reconnect) shouldn't drop the session.
      const status = (err as { response?: { status?: number } })?.response?.status
      if (status === 401 || status === 403) {
        logout()
      }
    } finally {
      loading.value = false
    }
  }

  const updateProfile = async (data: Partial<User>) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.patch('/auth/profile', data)
      const userData = response.data?.data || response.data
      setUser(userData)
      return true
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || 'Update failed'
      return false
    } finally {
      loading.value = false
    }
  }

  // Cross-tab + soft-logout sync. Two reasons this exists:
  //   1. Refresh tokens are non-rotating, so concurrent refreshes across tabs
  //      all succeed and there is no rotation race. But a tab that DIDN'T do
  //      the refresh still needs to adopt the new access token another tab
  //      minted; the 'storage' event is how it picks it up.
  //   2. On a confirmed 401 from /auth/refresh, client.ts dispatches
  //      'auth:expired' instead of window.location='/' redirecting. We listen
  //      here to clear in-memory refs without the abrupt page reload.
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (e) => {
      if (e.key === 'token') {
        token.value = e.newValue
        if (e.newValue === null) {
          user.value = null
        }
      } else if (e.key === 'user') {
        if (e.newValue === null) {
          user.value = null
        } else {
          try {
            user.value = JSON.parse(e.newValue)
          } catch {
            user.value = null
          }
        }
      } else if (e.key === null) {
        // localStorage.clear() from another tab
        token.value = null
        user.value = null
      }
    })
    window.addEventListener('auth:expired', () => {
      // localStorage was already wiped by the dispatcher (api/client.ts);
      // sync the in-memory refs so reactive components see the change.
      token.value = null
      user.value = null
    })
  }

  return {
    user,
    token,
    loading,
    error,
    isAuthenticated,
    isAdmin,
    canAccessLibrary,
    isRefreshing,
    wtGuestToken,
    wtGuestUser,
    ensureGuestToken,
    login,
    register,
    requestDeepLink,
    checkDeepLink,
    logout,
    fetchUser,
    setUser,
    updateProfile,
    refreshAccessToken
  }
})
