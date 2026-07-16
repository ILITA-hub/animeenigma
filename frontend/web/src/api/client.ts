import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse, AxiosError } from 'axios'
import { hookAxiosDiagnostics } from '@/utils/diagnostics'
import { getOrCreateAnonId } from '@/utils/anonId'
import type { WatchCombo, ResolveResponse, ResolvedCombo } from '@/types/preference'
import type { CreateJobPayload } from '@/types/library'
import type { FeedbackListResponse, FeedbackDetail, FeedbackStatus, FeedbackCategory } from '@/types/feedback'
import type { SourceRanking } from '@/types/sourceRanking'
import type { ShowcaseBlock } from '@/types/showcase'
import { consumePrefetch } from '@/utils/pagePrefetch'
import { newTraceparent } from '@/analytics/traceparent'
import { stampTrace } from '@/analytics/traceContext'
import { analytics } from '@/analytics'
import { noteMaskedAnalyticsPath, probeAnalyticsReachability } from '@/utils/analyticsTransport'
import router from '@/router'

const TRACING_ON = import.meta.env.VITE_ANALYTICS_ENABLED !== 'false'

const BASE_URL = import.meta.env.VITE_API_URL || '/api'

export const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  },
  withCredentials: true // Send cookies with requests (for refresh token)
})

// Flag to prevent multiple refresh attempts
let isRefreshing = false
let refreshPromise: Promise<string | null> | null = null
// Latched once a refresh has confirmed the session is dead (401/403). The
// httpOnly refresh cookie survives a client-side logout, so without this latch
// a queued/duplicate refresh would re-POST the now-dead token and earn a second
// spurious 401. Cleared when a fresh token is written (e.g. after re-login).
let authExpired = false
let failedQueue: Array<{
  resolve: (token: string) => void
  reject: (error: AxiosError) => void
}> = []

const processQueue = (error: AxiosError | null, token: string | null = null) => {
  failedQueue.forEach(prom => {
    if (error) {
      prom.reject(error)
    } else {
      prom.resolve(token!)
    }
  })
  failedQueue = []
}

// Check if JWT token is expired or about to expire (within 30s)
function isTokenExpired(token: string): boolean {
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    return payload.exp * 1000 < Date.now() + 30_000
  } catch {
    return true
  }
}

// Wipes auth-related localStorage on a confirmed logout (refresh 401/403).
// Mirrors what stores/auth.ts logout() does to the same keys.
function clearAuthLocalStorage() {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && (key.startsWith('pref:') || key === 'prefs_version')) {
      localStorage.removeItem(key)
    }
  }
}

// Fires after localStorage is cleared so the Pinia auth store can drop its
// in-memory refs without a hard navigation. Same-tab notification only;
// other tabs observe the localStorage delta via the 'storage' event.
function dispatchAuthExpired() {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('auth:expired'))
  }
}

// Actually performs the /auth/refresh POST. Kept separate from doTokenRefresh
// so the still-valid-token short-circuit can skip this round-trip when another
// tab already minted a fresh access token.
async function performRefresh(): Promise<string | null> {
  try {
    const response = await axios.post(`${BASE_URL}/auth/refresh`, {}, {
      withCredentials: true,
    })
    const data = response.data?.data || response.data
    const newAccessToken = data.access_token
    localStorage.setItem('token', newAccessToken)
    authExpired = false // a fresh token re-enables refresh after any prior logout
    processQueue(null, newAccessToken)
    return newAccessToken as string
  } catch (refreshError) {
    processQueue(refreshError as AxiosError, null)
    // Only treat 401/403 as a real auth failure. Network/5xx errors
    // (VPN reconnect, transient backend hiccup) leave the session intact.
    const status = (refreshError as AxiosError)?.response?.status
    if (status === 401 || status === 403) {
      // Soft logout: clear storage + notify the Pinia store. No
      // window.location redirect — that interrupts mid-action recovery
      // and races with the cross-tab listener.
      authExpired = true // stop further refreshes re-POSTing the dead httpOnly cookie
      clearAuthLocalStorage()
      dispatchAuthExpired()
    }
    return null
  }
}

// Shared token refresh — single-flight within a tab via the module-level
// refreshPromise. Refresh tokens are non-rotating, so two tabs refreshing at
// once both succeed; no cross-tab coordination is required.
async function doTokenRefresh(): Promise<string | null> {
  if (refreshPromise) return refreshPromise
  // Session already confirmed dead — don't re-POST the surviving httpOnly
  // refresh cookie (it would just earn another 401). Returning null makes the
  // caller reject the original request; the response interceptor only queues
  // waiters while a refresh is in flight, so none can be stranded here. A
  // successful login re-enables refresh by clearing the latch in performRefresh.
  if (authExpired) {
    return null
  }

  isRefreshing = true
  refreshPromise = (async () => {
    try {
      // Non-rotating refresh tokens: concurrent refreshes (other tabs, the
      // gateway admin middleware) all present the SAME stable token and all
      // succeed, so no cross-tab coordination is needed. A still-valid token
      // written by another tab is reused to skip a redundant round-trip.
      const stored = localStorage.getItem('token')
      if (stored && !isTokenExpired(stored)) {
        processQueue(null, stored)
        return stored
      }
      return await performRefresh()
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
}

// Request interceptor — proactively refresh expired tokens before sending
apiClient.interceptors.request.use(
  async (config: InternalAxiosRequestConfig) => {
    // Skip refresh for auth endpoints to avoid loops
    if (config.url?.includes('/auth/refresh') || config.url?.includes('/auth/login')) {
      return config
    }

    let token = localStorage.getItem('token')
    if (token && !isTokenExpired(token)) {
      // A present, still-valid token (e.g. a just-completed login that wrote it
      // directly, not via performRefresh) means the session is live again —
      // clear any prior dead-session latch so refresh works for the new session.
      authExpired = false
    }
    if (token && isTokenExpired(token)) {
      const newToken = await doTokenRefresh()
      token = newToken
    }
    config.headers = config.headers || {}
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    // Always-set X-Anon-ID per PATTERNS.md. The backend OptionalAuthMiddleware reads
    // JWT first regardless of X-Anon-ID presence, so always-set is harmless on JWT
    // routes (handlers ignore unknown headers) and removes a class of subtle bugs
    // where X-Anon-ID would be missing if the user had a JWT but the JWT was rejected
    // downstream. See .planning/phases/01-instrumentation-baseline/01-PATTERNS.md
    // and 01-RESEARCH.md §Pattern 7.
    config.headers['X-Anon-ID'] = getOrCreateAnonId()
    // Distributed tracing: mint a W3C traceparent per call so the backend
    // trace roots with a known trace_id, and stamp that id onto the click
    // event that triggered this request (best-effort, ~1.5s window).
    if (TRACING_ON) {
      const { header, traceId } = newTraceparent()
      config.headers['traceparent'] = header
      stampTrace(traceId)
      // Emit a lightweight source='fe' register row carrying THIS call's
      // trace_id — the same id stampTrace just back-filled onto the pending
      // click — so the click, the FE call, and the downstream BE effects all
      // join on one trace_id (AR-FE-01/AR-FE-02). Best-effort: a no-op before
      // analytics.init(), and wrapped so it can never throw into the request
      // path.
      try {
        // Read the route from the singleton (router.currentRoute) — useRoute()
        // throws in interceptor/module scope (no active component, RESEARCH P4).
        const route = router.currentRoute.value
        // Prefer the pattern-like route.name over the concrete fullPath to bound
        // register cardinality (T-04-07).
        const routeLabel = (route?.name as string | undefined) ?? route?.fullPath
        // Opt-in semantic action — set only when a caller passes config.meta.action,
        // so poster/poll fetches stay unlabeled (AR-FE-01 "optional semantic action").
        const action = (config as unknown as { meta?: { action?: string } }).meta?.action
        const method = (config.method ?? 'GET').toUpperCase()
        analytics.track('fe.call', {
          source: 'fe',
          trace_id: traceId,
          route: routeLabel,
          action,
          // API path as target (e.g. /api/anime/123). target_kind='route' tags it.
          target: config.url,
          target_kind: 'route',
          // Coarse operation label (METHOD route) to bound cardinality.
          operation: `${method} ${routeLabel ?? ''}`.trim(),
        })
      } catch {
        // Never let analytics emission break the request.
      }
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Phase 7 D-03 — bust the 24h pref:* localStorage cache when the backend
// signals a new prefs_version on any preference-touching response. This makes
// cross-device prefs changes visible immediately on the next API call instead
// of after the 24h composable TTL.
const PREFS_VERSION_KEY = 'prefs_version'
function maybeBustPrefsCache(headerVal: string | undefined) {
  if (!headerVal) return
  const incoming = headerVal.trim()
  if (!incoming) return
  const cached = localStorage.getItem(PREFS_VERSION_KEY)
  if (cached === incoming) return
  // Version drifted — wipe every cached resolved-combo entry. The next
  // resolve call repopulates from server.
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && key.startsWith('pref:')) {
      localStorage.removeItem(key)
    }
  }
  localStorage.setItem(PREFS_VERSION_KEY, incoming)
}

// Response interceptor — handles 401 errors and X-Token-Expired fallback
apiClient.interceptors.response.use(
  async (response: AxiosResponse) => {
    // Cross-device prefs freshness signal (Phase 7 D-03)
    maybeBustPrefsCache(response.headers['x-prefs-version'] as string | undefined)

    // Track B5: learn the rotating masked analytics base + probe once.
    noteMaskedAnalyticsPath(response.headers['x-ae-cfg'] as string | undefined)
    probeAnalyticsReachability()

    // Fallback: backend signals token was present but expired on optional-auth endpoints
    if (response.headers['x-token-expired'] === 'true') {
      const newToken = await doTokenRefresh()
      if (newToken) {
        const config = response.config as InternalAxiosRequestConfig & { _tokenRetry?: boolean }
        if (!config._tokenRetry) {
          config._tokenRetry = true
          config.headers.Authorization = `Bearer ${newToken}`
          return apiClient(config)
        }
      }
    }
    return response
  },
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // Skip refresh for auth endpoints to avoid loops
    if (error.response?.status === 401 &&
        originalRequest &&
        !originalRequest._retry &&
        !originalRequest.url?.includes('/auth/refresh') &&
        !originalRequest.url?.includes('/auth/login')) {

      if (isRefreshing) {
        // Queue request while refresh is in progress (10s timeout to prevent hanging)
        return new Promise<string>((resolve, reject) => {
          const timeout = setTimeout(() => reject(new Error('Token refresh timeout')), 10_000)
          failedQueue.push({
            resolve: (token: string) => { clearTimeout(timeout); resolve(token) },
            reject: (err: AxiosError) => { clearTimeout(timeout); reject(err) }
          })
        }).then(token => {
          originalRequest.headers.Authorization = `Bearer ${token}`
          return apiClient(originalRequest)
        }).catch(err => {
          return Promise.reject(err)
        })
      }

      originalRequest._retry = true
      const newToken = await doTokenRefresh()
      if (newToken) {
        originalRequest.headers.Authorization = `Bearer ${newToken}`
        return apiClient(originalRequest)
      }
    }

    return Promise.reject(error)
  }
)

// Hook diagnostics capture for network logs
hookAxiosDiagnostics(apiClient)

// Phase 17 (UX-33) — editorial collections. Mirror the backend
// services/catalog/internal/domain/collection.go shape.
export interface CollectionItem {
  id: string
  collection_id: string
  anime_id: string
  // The backend preloads Anime on detail views; list views omit it.
  anime?: {
    id: string
    name?: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
    episodes_count?: number
    episodes_aired?: number
    score?: number
    year?: number
    status?: string
  }
  sort_order: number
  created_at: string
}

export interface Collection {
  id: string
  slug: string
  title: string
  title_ru?: string
  title_jp?: string
  description?: string
  description_ru?: string
  description_jp?: string
  cover_image_url?: string
  published: boolean
  created_by?: string
  items?: CollectionItem[]
  item_count: number
  created_at: string
  updated_at: string
}

export interface CreateCollectionRequest {
  slug?: string
  title: string
  title_ru?: string
  title_jp?: string
  description?: string
  description_ru?: string
  description_jp?: string
  cover_image_url?: string
  published?: boolean
}

export type UpdateCollectionRequest = Partial<CreateCollectionRequest>

export interface AddCollectionItemRequest {
  anime_id: string
  sort_order?: number
}

// API endpoints
export const animeApi = {
  getAll: (params?: Record<string, unknown>) => apiClient.get('/anime', { params }),
  // The anime route guard prefetches this request at navigation start (in
  // parallel with the route chunk download) — consume the stashed promise
  // when present instead of re-issuing. See src/utils/pagePrefetch.ts.
  getById: (id: string) =>
    consumePrefetch<AxiosResponse>(`anime:${id}`) ?? apiClient.get(`/anime/${id}`),
  search: (query: string, source?: string, pageSize?: number, signal?: AbortSignal) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }), ...(pageSize && { page_size: pageSize }) }, signal }),
  resolveShikimori: (shikimoriId: string) => apiClient.get(`/anime/shikimori/${shikimoriId}`),
  getTrending: () => apiClient.get('/anime/trending'),
  getPopular: () => apiClient.get('/anime/popular'),
  getRecent: () => apiClient.get('/anime/recent'),
  getSchedule: () => apiClient.get('/anime/schedule'),
  getOngoing: (params?: { sort?: string; order?: 'asc' | 'desc'; recent?: boolean }) =>
    apiClient.get('/anime/ongoing', { params }),
  getAnnounced: (limit = 20) => apiClient.get('/anime', { params: { status: 'announced', page_size: limit } }),
  getTop: (limit = 20) => apiClient.get('/anime/trending', { params: { page_size: limit } }),
  refresh: (id: string) => apiClient.post(`/anime/${id}/refresh`),
  resolveMAL: (malId: string) => apiClient.get(`/anime/mal/${malId}`),
  getGenres: () => apiClient.get("/genres"),
  getStudios: () => apiClient.get("/studios"),
  getNews: () => apiClient.get('/anime/news'),
  getRelated: (animeId: string) => apiClient.get(`/anime/${animeId}/related`),
  // Phase 14 / UX-28 — soft social-proof: how many users have this anime
  // in their list with status='watching'. Public, no auth.
  getWatchersCount: (animeId: string) =>
    apiClient.get<{ count: number } | { data: { count: number } }>(`/anime/${animeId}/watchers-count`),
  // Page-fetch optimization (2026-06-11) — aggregate anime-page context:
  // rating + watchers-count + the viewer's progress / watchlist entry /
  // review / saved combo in ONE optional-auth round-trip. Anonymous callers
  // get the public subset (user-scoped fields null).
  getViewerContext: (animeId: string, malId?: string | number) =>
    apiClient.get(`/anime/${animeId}/viewer-context`, {
      params: malId ? { mal_id: String(malId) } : undefined,
    }),
  // Phase 17 (UX-33) — editorial collections.
  listCollections: (limit = 12) =>
    apiClient.get<Collection[] | { data: Collection[] }>('/collections', { params: { limit } }),
  getCollection: (slug: string) =>
    apiClient.get<Collection | { data: Collection }>(`/collections/${encodeURIComponent(slug)}`),
  // Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTAs. Public, no auth.
  // Backend proxies api.aniskip.com with a 7d cache. Empty response shape
  // when MAL ID is missing from aniskip's crowdsourced DB:
  //   { found: false, results: [] }
  getSkipTimes: (malId: string, episode: number) =>
    apiClient.get(`/skip-times/${encodeURIComponent(malId)}/${episode}`),
}

export const episodeApi = {
  getByAnimeId: (animeId: string) => apiClient.get(`/anime/${animeId}/episodes`),
  getById: (id: string) => apiClient.get(`/episodes/${id}`),
  getSources: (id: string) => apiClient.get(`/episodes/${id}/sources`)
}

export const userApi = {
  getProfile: () => apiClient.get('/users/profile'),
  updateProfile: (data: Record<string, unknown>) => apiClient.patch('/users/profile', data),
  getWatchlist: (params?: { status?: string; page?: number; per_page?: number; sort?: string; order?: string; q?: string; genres?: string; kind?: string; year_min?: string; year_max?: string }) =>
    apiClient.get('/users/watchlist', { params }),
  getWatchlistStatuses: () => apiClient.get('/users/watchlist/statuses'),
  getWatchlistFacets: () => apiClient.get('/users/watchlist/facets'),
  bulkWatchlist: (body: { anime_ids: string[]; action: 'set_status' | 'remove'; status?: string }) =>
    apiClient.post('/users/watchlist/bulk', body),
  getWatchlistEntry: (animeId: string) => apiClient.get(`/users/watchlist/${animeId}`),
  addToWatchlist: (animeId: string, status: string = 'plan_to_watch') =>
    apiClient.post('/users/watchlist', { anime_id: animeId, status }),
  updateWatchlistStatus: (animeId: string, status: string) =>
    apiClient.put('/users/watchlist', { anime_id: animeId, status }),
  updateWatchlistEntry: (data: {
    anime_id: string
    status: string
    started_at?: string | null
    completed_at?: string | null
    score?: number
    episodes?: number
    notes?: string
    rewatch_count?: number
    is_rewatching?: boolean
  }) => apiClient.put('/users/watchlist', data),
  // Rewatch lifecycle (design 2026-06-05): resets a completed entry to a fresh
  // watching cycle server-side (episodes=0, progress reset, is_rewatching=true).
  startRewatch: (animeId: string) => apiClient.post(`/users/watchlist/${animeId}/rewatch`),
  removeFromWatchlist: (animeId: string) => apiClient.delete(`/users/watchlist/${animeId}`),
  markEpisodeWatched: (animeId: string, episode: number, combo?: Partial<WatchCombo>, sessionId?: string) =>
    apiClient.post(`/users/watchlist/${animeId}/episode`, {
      episode,
      ...combo,
      ...(sessionId ? { session_id: sessionId } : {}),
    }).then((res) => {
      // Marking an episode watched triggers an async server-side «Энигмы»
      // credit (player → gacha, fire-and-forget). Announce it so the gacha
      // store can refresh the balance chip shortly after — the listener is
      // GACHA_CREDIT_EVENT in stores/gacha.ts. Page-fetch optimization
      // 2026-06-11.
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new Event('gacha:maybe-credited'))
      }
      return res
    }),
  getWatchHistory: () => apiClient.get('/users/history'),
  updateProgress: (data: Record<string, unknown>) => apiClient.post('/users/progress', data),
  getProgress: (animeId: string) => apiClient.get(`/users/progress/${animeId}`),
  // Phase 8 (UX-15 / UA-061) — Continue-Watching row. JWT-protected; returns
  // one item per anime (latest in-progress episode), ordered by last_watched_at DESC.
  getContinueWatching: (limit?: number) =>
    apiClient.get('/users/continue-watching', {
      params: typeof limit === 'number' ? { limit } : undefined,
    }),
  // Phase 9 (UX-16) — bulk per-card progress map. JWT-protected; ids capped
  // at 50 server-side. Returns { [animeId]: ProgressEntry } for animes the
  // user has progress on (missing animes omitted).
  getAnimeProgress: (ids: string[]) =>
    apiClient.get('/users/anime-progress', {
      params: { ids: ids.join(',') },
    }),
  getMyReviews: () => apiClient.get('/users/reviews'),
  importMAL: (username: string) => apiClient.post('/users/import/mal', { username }),
  importShikimori: (nickname: string) => apiClient.post('/users/import/shikimori', { nickname }),
  getImportJobStatus: (jobId: string) => apiClient.get(`/users/import/${jobId}`),
  getSyncStatus: () => apiClient.get('/users/sync/status'),
  exportJSON: () => apiClient.get('/users/export/json', { responseType: 'blob' }),
  migrateListEntry: (oldAnimeId: string, newAnimeId: string) =>
    apiClient.post('/users/watchlist/migrate', {
      old_anime_id: oldAnimeId,
      new_anime_id: newAnimeId,
    }),
  // Privacy settings
  updatePublicId: (publicId: string) => apiClient.put('/auth/profile/public-id', { public_id: publicId }),
  updatePrivacy: (publicStatuses: string[]) => apiClient.put('/auth/profile/privacy', { public_statuses: publicStatuses }),
  updateActivityVisibility: (activityVisibility: 'all' | 'non_hentai' | 'none') =>
    apiClient.put('/auth/profile/activity-visibility', { activity_visibility: activityVisibility }),
  // Avatar
  updateAvatar: (avatar: string) => apiClient.put('/auth/profile/avatar', { avatar }),
  updateTimezone: (timezone: string) => apiClient.put('/auth/profile/timezone', { timezone }),
  // Error reporting
  reportError: (data: Record<string, unknown>) => apiClient.post('/users/report', data),
  listMyReports: (params?: { page?: number; page_size?: number; from?: string; to?: string }) => apiClient.get('/users/reports', { params }),
  // API Key management
  generateApiKey: () => apiClient.post('/auth/api-key'),
  revokeApiKey: () => apiClient.delete('/auth/api-key'),
  hasApiKey: () => apiClient.get('/auth/api-key'),
  // Watch preferences
  resolvePreference: (animeId: string, available: WatchCombo[]) =>
    apiClient.post<ResolveResponse>('/preferences/resolve', { anime_id: animeId, available }),
  recordOverride: (data: {
    anime_id: string
    load_session_id: string
    dimension: 'language' | 'player' | 'team' | 'episode'
    original_combo: ResolvedCombo | null
    new_combo: Partial<WatchCombo> & { episode?: number }
    ms_since_load: number
    tier: string | null
    tier_number: number | null
    player: WatchCombo['player']
  }) => apiClient.post('/preferences/override', data),
  getAnimePreference: (animeId: string) =>
    apiClient.get<WatchCombo & { updated_at: string }>(`/users/preferences/${animeId}`),
  getGlobalPreferences: () =>
    apiClient.get<{ top_combos: (WatchCombo & { count: number })[] }>('/users/preferences/global'),
  // Phase 7 — Advanced Settings + cross-device freshness
  getTier2DebugView: () =>
    apiClient.get<{
      coarse: Array<{ language: string; watch_type: string; weight: number }>
      fine: Array<{
        language: string
        watch_type: string
        player: string
        translation_id: string
        translation_title: string
        weight: number
      }>
      total_weight: number
      min_confidence: number
      half_life_days: number
      lock: {
        language: string
        watch_type: string
        top_translation_title: string
        confidence: number
      } | null
    }>('/users/preferences/tier2'),
  forceCombo: (animeId: string, combo: WatchCombo) =>
    apiClient.post(`/users/preferences/${animeId}/force`, combo),
  resetLearnedPreferences: () =>
    apiClient.delete<{ prefs_version: number }>('/users/preferences/learned'),
}

export const publicApi = {
  // Get public user profile by public_id
  getUserProfile: (publicId: string) => apiClient.get(`/auth/users/${publicId}`),
  // Get public watchlist
  getPublicWatchlist: (userId: string, params?: { status?: string; statuses?: string; page?: number; per_page?: number; sort?: string; order?: string; q?: string; genres?: string; kind?: string; year_min?: string; year_max?: string }) =>
    apiClient.get(`/users/${userId}/watchlist/public`, { params }),
  // Get public watchlist stats (avg score, total episodes)
  getPublicWatchlistStats: (userId: string, statuses?: string[]) =>
    apiClient.get(`/users/${userId}/watchlist/public/stats`, { params: statuses?.length ? { statuses: statuses.join(',') } : undefined }),
  getPublicWatchlistFacets: (userId: string) =>
    apiClient.get(`/users/${userId}/watchlist/facets`),
}

export const showcaseApi = {
  // Public read of a user's profile showcase blocks (+ visibility flag).
  getShowcase: (userId: string) =>
    apiClient.get<
      | { blocks: ShowcaseBlock[]; enabled: boolean }
      | { data: { blocks: ShowcaseBlock[]; enabled: boolean } }
    >(`/users/${userId}/showcase`),
  // Owner save (replaces the whole block array). "me" resolves to the JWT user.
  // `enabled` is coerced to false server-side when blocks is empty.
  saveShowcase: (blocks: ShowcaseBlock[], enabled: boolean) =>
    apiClient.put<
      | { blocks: ShowcaseBlock[]; enabled: boolean }
      | { data: { blocks: ShowcaseBlock[]; enabled: boolean } }
    >(`/users/me/showcase`, { blocks, enabled }),
  // Compatibility score between the viewer and the profile owner.
  // Player returns bare or {success,data} envelope — mirrored union type.
  getCompatibility: (userId: string) =>
    apiClient.get<
      | { percent: number; shared_count: number; shared_sample: string[]; self?: boolean }
      | { data: { percent: number; shared_count: number; shared_sample: string[]; self?: boolean } }
    >(`/users/${userId}/compatibility`),
}

/** Standard user-resolve result (auth service, RBAC-and-roulette P3). */
export interface ResolvedUser {
  id: string
  username: string
  public_id: string
  telegram_id?: number
  /** Canonical role ("user"/"admin"). Optional so older cached shapes don't
   *  break; consumers that need it fall back to "user". */
  role?: string
}

/** RBAC-and-roulette P1 — policy-service feature-flag runtime access rule.
 *  `failSafe` decides who can reach the feature when the policy service
 *  itself is unreachable (gateway FeatureGate fails static to this value).
 *  `allowUsers`/`denyUsers` are user ID (UUID) arrays, not usernames. */
export type FailSafe = 'admin' | 'everyone'

export interface FeatureFlag {
  key: string
  roles: string[]
  allowUsers: string[]
  denyUsers: string[]
  roulette: boolean
  failSafe: FailSafe
  label: string
  updatedAt: string
}

/** Body for PUT /admin/policy/flags/{key} — the same shape as FeatureFlag
 *  minus the server-owned `key`/`updatedAt` fields. */
export type FeatureFlagPayload = Omit<FeatureFlag, 'key' | 'updatedAt'>

export interface PolicyFlagsResponse {
  flags: FeatureFlag[]
  rouletteEnabled: boolean
}

/** Admin lever over a scraper provider's runtime policy. All three values are
 *  admin-set (the probe engine's auto demote/promote was retired 2026-07-08 —
 *  nothing machine-sets policy): `auto` = in the failover chain, `manual` =
 *  parked out of auto-failover but still manually selectable, `disabled` =
 *  dropped entirely. See services/catalog's SetPolicy. */
export type ScraperProviderPolicy = 'auto' | 'manual' | 'disabled'

/** RBAC-and-roulette P5 Task 2 — wire shape of `GET /api/admin/scraper-providers`
 *  / `PUT .../{name}/policy` (services/catalog/internal/handler/admin_scraper_providers.go
 *  `adminProviderWire`). Mirrors the internal scraper-facing `providerWire`
 *  1:1 plus `derived_state`, the 5-state dashboard health-lifecycle label
 *  (`UP|Recovering|Degrading|Down|Disabled`) the FE renders as a status pill
 *  without re-implementing the policy+health precedence itself. Only an explicit
 *  admin disable reads as `Disabled`; a parked manual provider shows its live
 *  health. */
export interface ScraperProviderWire {
  name: string
  status: string
  policy: ScraperProviderPolicy
  health: 'up' | 'degraded' | 'recovering' | 'down'
  health_since: string
  policy_since: string
  last_probed_at: string
  group: string
  reason: string
  description: string
  scraper_operated: boolean
  supports_sub: boolean
  supports_dub: boolean
  supports_raw: boolean
  sub_delivery: string
  quality_ceiling: string
  preference_weight: number
  engine: string
  base_url: string
  last_tick_metrics: string
  updated_at: string
  derived_state: 'UP' | 'Recovering' | 'Degrading' | 'Down' | 'Disabled'
}

export interface ScraperProvidersResponse {
  providers: ScraperProviderWire[]
}

// Maintenance routines (Maintenance tab). Nested under /admin/policy/* so it
// reuses the existing gateway admin-policy proxy group (no gateway change).
export interface MaintenanceRoutineWire {
  id: string
  enabled: boolean
  settings: Record<string, unknown>
  lastRunAt: string | null
  lastOk: boolean | null
  lastSummary: string
  nextRunAt: string | null
  updatedAt: string
}
export interface MaintenanceRoutinesResponse {
  routines: MaintenanceRoutineWire[]
}

/** RBAC-and-roulette P4 — per-user feature-visibility feed
 *  (`GET /api/policy/features/mine`). JWT OPTIONAL: an authenticated caller
 *  gets their resolved flags, an anonymous caller gets everyone-flags only.
 *  Fail-open server-side. This is what the frontend cuts over to in P4 —
 *  nav/dark-ship routes/profile-wall tab/footer roulette all read this
 *  instead of the old `VITE_*_ADMIN_ONLY` build flags + the retired
 *  secret-features admin facade (removed P4 Task 4). */
export interface FeaturesMineResponse {
  rouletteEnabled: boolean
  visible: string[]
  roulette: string[]
}

function unwrapFeaturesMine(response: AxiosResponse<unknown>): FeaturesMineResponse {
  const data = response.data as { data?: FeaturesMineResponse } | FeaturesMineResponse | undefined
  if (data && typeof data === 'object' && 'data' in data) {
    return data.data as FeaturesMineResponse
  }
  return data as FeaturesMineResponse
}

export const featuresApi = {
  getFeaturesMine: (): Promise<FeaturesMineResponse> =>
    apiClient.get<{ data: FeaturesMineResponse } | FeaturesMineResponse>('/policy/features/mine').then(unwrapFeaturesMine),
}

export const adminApi = {
  // Hide/unhide anime globally
  hideAnime: (animeId: string) => apiClient.post(`/admin/anime/${animeId}/hide`),
  // Standard user resolver — turns a typed identifier (username / public_id /
  // Telegram ID / UUID) into a user. 404 when no match; 400 on empty q.
  resolveUser: (q: string) =>
    apiClient.get<ResolvedUser | { data: ResolvedUser }>('/admin/users/resolve', {
      params: { q },
    }),
  unhideAnime: (animeId: string) => apiClient.delete(`/admin/anime/${animeId}/hide`),
  // Update shikimori_id
  updateShikimoriId: (animeId: string, shikimoriId: string) =>
    apiClient.patch(`/admin/anime/${animeId}/shikimori`, { shikimori_id: shikimoriId }),
  // Phase 17 (UX-33) — editorial collections admin CRUD + item picker.
  listCollections: () =>
    apiClient.get<Collection[] | { data: Collection[] }>('/admin/collections'),
  getCollection: (id: string) =>
    apiClient.get<Collection | { data: Collection }>(`/admin/collections/${id}`),
  createCollection: (body: CreateCollectionRequest) =>
    apiClient.post<Collection | { data: Collection }>('/admin/collections', body),
  updateCollection: (id: string, body: UpdateCollectionRequest) =>
    apiClient.put<Collection | { data: Collection }>(`/admin/collections/${id}`, body),
  deleteCollection: (id: string) =>
    apiClient.delete<void>(`/admin/collections/${id}`),
  addCollectionItem: (id: string, body: AddCollectionItemRequest) =>
    apiClient.post<CollectionItem | { data: CollectionItem }>(`/admin/collections/${id}/items`, body),
  removeCollectionItem: (id: string, animeId: string) =>
    apiClient.delete<void>(`/admin/collections/${id}/items/${animeId}`),
  // RBAC-and-roulette P1 — policy-service admin CRUD (services/policy:8098,
  // gateway-proxied at /api/admin/policy/*, admin-JWT-gated). This is the
  // runtime feature-access authority (the older roulette-only
  // /admin/secret-features facade was removed in P4 Task 4).
  getPolicyFlags: () =>
    apiClient.get<{ data: PolicyFlagsResponse } | PolicyFlagsResponse>('/admin/policy/flags'),
  setPolicyFlag: (key: string, payload: FeatureFlagPayload) =>
    apiClient.put<{ data: { key: string } } | { key: string }>(
      `/admin/policy/flags/${encodeURIComponent(key)}`,
      payload,
    ),
  setPolicyRoulette: (enabled: boolean) =>
    apiClient.put<{ data: { enabled: boolean } } | { enabled: boolean }>(
      '/admin/policy/roulette',
      { enabled },
    ),
  // RBAC-and-roulette P5 Task 2 — the Providers tab's facade over catalog's
  // stream_providers table (services/catalog/internal/handler/
  // admin_scraper_providers.go). As of 2026-07-13 the admin sends only the probe
  // status 'auto'|'disabled'; the auto↔manual failover axis is machine-set from
  // health server-side (the wire's `policy` field can still read 'manual').
  listScraperProviders: () =>
    apiClient.get<{ data: ScraperProvidersResponse } | ScraperProvidersResponse>(
      '/admin/scraper-providers',
    ),
  setScraperProviderPolicy: (name: string, policy: 'auto' | 'disabled') =>
    apiClient.put<{ data: ScraperProviderWire } | ScraperProviderWire>(
      `/admin/scraper-providers/${encodeURIComponent(name)}/policy`,
      { policy },
    ),
  // Maintenance tab — services/policy admin CRUD over background routines
  // (services/policy/internal/handler/admin_maintenance.go), nested under the
  // existing /admin/policy/* proxy group.
  getMaintenanceRoutines: () =>
    apiClient.get<{ data: MaintenanceRoutinesResponse } | MaintenanceRoutinesResponse>(
      '/admin/policy/maintenance/routines',
    ),
  setMaintenanceRoutine: (id: string, body: { enabled: boolean; settings: Record<string, unknown> }) =>
    apiClient.put<{ data: { id: string } } | { id: string }>(
      `/admin/policy/maintenance/routines/${encodeURIComponent(id)}`,
      body,
    ),
  // Admin feedback browser — user feedback/error reports (player service,
  // /api/admin/reports). Responses use the standard {success,data} envelope.
  listReports: (params?: { category?: string; status?: string; type?: string; source?: string; username?: string; from?: string; to?: string; page?: number; page_size?: number }) =>
    apiClient.get<FeedbackListResponse | { data: FeedbackListResponse }>('/admin/reports', { params }),
  createNote: (body: { category?: FeedbackCategory; description: string }) =>
    apiClient.post<{ id: string; status: string } | { data: { id: string; status: string } }>('/admin/reports', body),
  getReport: (id: string) =>
    apiClient.get<FeedbackDetail | { data: FeedbackDetail }>(`/admin/reports/${encodeURIComponent(id)}`),
  setReportStatus: (id: string, status: FeedbackStatus) =>
    apiClient.patch(`/admin/reports/${encodeURIComponent(id)}/status`, { status }),
  // Attachments need the Bearer header, so they're fetched as blobs (a plain
  // <img src> would arrive unauthenticated) and rendered via object URLs.
  getReportAttachment: (id: string, name: string) =>
    apiClient.get<Blob>(
      `/admin/reports/${encodeURIComponent(id)}/attachments/${encodeURIComponent(name)}`,
      { responseType: 'blob' },
    ),
}

// Phase 5 (LIB-09) — Raw Library admin API. All routes are admin-gated
// at the gateway (/api/library/* prefix); the frontend trusts the
// gateway's 401/403 to surface for non-admins. Responses are wrapped
// in the standard httputil envelope ({success, data}) — consumers
// unwrap via response.data.data.
export const adminLibraryApi = {
  search: (q: string, malId?: number, limit = 50) =>
    apiClient.get('/library/search', {
      params: { q, ...(typeof malId === 'number' ? { mal_id: malId } : {}), limit },
    }),
  listJobs: (status?: string, limit = 50) =>
    apiClient.get('/library/jobs', {
      params: { ...(status ? { status } : {}), limit },
    }),
  getJob: (id: string) => apiClient.get(`/library/jobs/${id}`),
  createJob: (payload: CreateJobPayload) => apiClient.post('/library/jobs', payload),
  cancelJob: (id: string) => apiClient.delete(`/library/jobs/${id}`),
  linkJob: (id: string, shikimoriId: string) =>
    apiClient.patch(`/library/jobs/${id}`, { shikimori_id: shikimoriId }),
  retryJob: (id: string) => apiClient.post(`/library/jobs/${id}/retry`),
  healthExtended: () => apiClient.get('/library/health/extended'),
  browseFiles: (domain: string, prefix = '') =>
    apiClient.get('/library/files', { params: { domain, ...(prefix ? { prefix } : {}) } }),
  deleteFile: (domain: string, key: string, confirm = false) =>
    apiClient.delete('/library/files', { params: { domain, key, ...(confirm ? { confirm: 1 } : {}) } }),
  downloadFile: (domain: string, key: string) =>
    apiClient.get('/library/files/download', { params: { domain, key }, responseType: 'blob' }),
}

export const reviewApi = {
  // Get all reviews for an anime (public)
  getAnimeReviews: (animeId: string) => apiClient.get(`/anime/${animeId}/reviews`),
  // Get average rating for an anime (public)
  getAnimeRating: (animeId: string) => apiClient.get(`/anime/${animeId}/rating`),
  // Get current user's review for an anime
  getMyReview: (animeId: string) => apiClient.get(`/anime/${animeId}/reviews/me`),
  // Create or update a review
  createReview: (animeId: string, score: number, reviewText: string) =>
    apiClient.post(`/anime/${animeId}/reviews`, {
      anime_id: animeId,
      score,
      review_text: reviewText,
    }),
  // Delete a review
  deleteReview: (animeId: string) => apiClient.delete(`/anime/${animeId}/reviews`),
  // Get batch ratings for multiple anime
  getBatchRatings: (animeIds: string[]) =>
    apiClient.post('/anime/ratings/batch', { anime_ids: animeIds }),
  // Toggle an emoji reaction on a review (auth required). The emoji is
  // percent-encoded into the path; returns { added, counts }. AUTO-408.
  toggleReaction: (animeId: string, reviewId: string, emoji: string) =>
    apiClient.post(
      `/anime/${animeId}/reviews/${reviewId}/reactions/${encodeURIComponent(emoji)}`,
    ),
  // Admin moderation: remove a specific user's reaction from a review
  // (admin-only, enforced server-side). Returns { counts }. AUTO-408.
  adminRemoveReaction: (animeId: string, reviewId: string, emoji: string, userId: string) =>
    apiClient.delete(
      `/anime/${animeId}/reviews/${reviewId}/reactions/${encodeURIComponent(emoji)}/users/${userId}`,
    ),
}

export const commentApi = {
  // Get paginated comments for an anime (public, newest-first; cursor is opaque)
  getAnimeComments: (animeId: string, params?: { cursor?: string; limit?: number }) =>
    apiClient.get(`/anime/${animeId}/comments`, { params }),
  // Create a new comment on an anime (auth required, 1–2000 chars, 10/hr/(user,anime))
  createComment: (animeId: string, body: string) =>
    apiClient.post(`/anime/${animeId}/comments`, { body }),
  // Update an existing comment (owner only)
  updateComment: (animeId: string, commentId: string, body: string) =>
    apiClient.patch(`/anime/${animeId}/comments/${commentId}`, { body }),
  // Soft-delete a comment (owner or admin)
  deleteComment: (animeId: string, commentId: string) =>
    apiClient.delete(`/anime/${animeId}/comments/${commentId}`),
}

export const activityApi = {
  getFeed: (limit: number = 10, before?: string) =>
    apiClient.get('/activity/feed', {
      params: { limit, ...(before && { before }) }
    }),
}

export const gameApi = {
  getRooms: () => apiClient.get('/game/rooms'),
  getRoom: (id: string) => apiClient.get(`/game/rooms/${id}`),
  createRoom: (data: Record<string, unknown>) => apiClient.post('/game/rooms', data),
  joinRoom: (id: string) => apiClient.post(`/game/rooms/${id}/join`),
  leaveRoom: (id: string) => apiClient.post(`/game/rooms/${id}/leave`)
}

export const kodikApi = {
  getTranslations: (animeId: string) => apiClient.get(`/anime/${animeId}/kodik/translations`),
  getVideo: (animeId: string, episode: number, translationId: number) =>
    apiClient.get(`/anime/${animeId}/kodik/video`, {
      params: { episode, translation: translationId }
    }),
  getStream: (animeId: string, episode: number, translation: number, quality?: number) =>
    apiClient.get(`/anime/${animeId}/kodik/stream`, {
      params: { episode, translation, ...(quality ? { quality } : {}) },
    }),
  search: (query: string) => apiClient.get('/kodik/search', { params: { q: query } }),
  getPinnedTranslations: (animeId: string) => apiClient.get(`/anime/${animeId}/pinned-translations`),
  pinTranslation: (animeId: string, translationId: number, title: string, type: string) =>
    apiClient.post(`/anime/${animeId}/pin-translation`, {
      translation_id: translationId,
      translation_title: title,
      translation_type: type
    }),
  unpinTranslation: (animeId: string, translationId: number) =>
    apiClient.delete(`/anime/${animeId}/pin-translation/${translationId}`)
}

/**
 * animejoy — RU-sub-only provider with two legs (sibnet | allvideo). The
 * resolved stream is a Referer-gated, tokened progressive MP4 served through
 * the HLS proxy with referer+exp+sig.
 */
export const animejoyApi = {
  getEpisodes: (animeId: string, leg: 'sibnet' | 'allvideo') =>
    apiClient.get(`/anime/${animeId}/animejoy-${leg}/episodes`),
  getStream: (animeId: string, leg: 'sibnet' | 'allvideo', episode: number, team?: string) =>
    apiClient.get(`/anime/${animeId}/animejoy-${leg}/stream`, {
      params: { episode, ...(team ? { team } : {}) },
    }),
}

/**
 * Phase 16 — scraperApi targets the /api/anime/{id}/scraper/* routes served
 * by the catalog→scraper pipeline. The single English-source entry point now
 * that the legacy direct clients are gone.
 *
 * The `prefer` parameter is a per-anime user override; when omitted, the
 * orchestrator picks its default.
 */
export const scraperApi = {
  getEpisodes: (animeId: string, prefer?: string, exclusive?: boolean) =>
    apiClient.get(`/anime/${animeId}/scraper/episodes`, {
      params: { ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  getServers: (animeId: string, episodeId: string, prefer?: string, exclusive?: boolean) =>
    apiClient.get(`/anime/${animeId}/scraper/servers`, {
      params: { episode: episodeId, ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  getStream: (
    animeId: string,
    episodeId: string,
    serverId: string,
    category: 'sub' | 'dub',
    prefer?: string,
    exclusive?: boolean,
  ) =>
    apiClient.get(`/anime/${animeId}/scraper/stream`, {
      params: { episode: episodeId, server: serverId, category, ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  // Health is per-service (not per-anime) but the catalog route is templated
  // on animeId for routing reasons. The catalog forwards to scraper without
  // touching animeId for the health path. Pass any UUID (e.g. an underscore
  // placeholder).
  getHealth: () => apiClient.get(`/anime/_/scraper/health`),
}

/**
 * Assembled, ranked per-anime capability report (catalog P4). Families:
 * ourenglish (EN scrapers), kodik, animelib, hanime. The catalog wraps the
 * payload in the {success,data} envelope — callers read res.data.data.
 */
export const capabilitiesApi = {
  get: (animeId: string) => apiClient.get(`/anime/${animeId}/capabilities`),
}

/**
 * Smart Source Selection — learned-reliability ranking + same-day override.
 * `getSourceRanking` feeds rankingToOrder → pickSmartDefault; `postSourceFix`
 * records a same-day override provider after a client-side fallback rescued a
 * failed resolve (fire-and-forget; the player never blocks on it).
 */
export const sourceRankingApi = {
  getSourceRanking: (animeId: string) =>
    apiClient.get<{ success: boolean; data: SourceRanking }>(
      `/anime/${animeId}/source-ranking`,
    ),
  postSourceFix: (animeId: string, provider: string) =>
    apiClient.post(`/anime/${animeId}/source-fix`, { provider }),
}

export const jimakuApi = {
  getSubtitles: (animeId: string, episode: number) =>
    apiClient.get(`/anime/${animeId}/jimaku/subtitles`, {
      params: { episode }
    }),
}

export const charactersApi = {
  getAnimeCharacters: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/characters`),
  getCharacter: (id: string) =>
    apiClient.get(`/characters/${id}`),
}

export const staffApi = {
  getAnimeStaff: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/staff`),
}

/**
 * First-party "AnimeEnigma" provider — self-hosted library (MinIO HLS).
 * Episodes/stream resolve STRICTLY from what's encoded on-prem; the stream
 * URL arrives proxy-signed (exp/sig) so the un-allowlisted minio host is
 * trusted on the master-playlist request.
 */
export const aeApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/ae/episodes`),
  getStream: (animeId: string, episode: number, quality?: string, server?: string) =>
    apiClient.get(`/anime/${animeId}/ae/stream`, {
      params: { episode, ...(quality && { quality }), ...(server && { server }) },
    }),
}

/**
 * workstream raw-jp / Phase 02 — aggregated subtitle sources.
 * `byLang` filters to a CSV of ISO 639-1 codes. `all` returns every track.
 */
export const subtitlesApi = {
  byLang: (animeId: string, episode: number, langs: string[]) =>
    apiClient.get(`/anime/${animeId}/subtitles`, {
      params: { episode, lang: langs.join(',') },
    }),
  all: (animeId: string, episode: number) =>
    apiClient.get(`/anime/${animeId}/subtitles/all`, {
      params: { episode },
    }),
}

export const hanimeApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/hanime/episodes`),
  getStream: (animeId: string, slug: string) =>
    apiClient.get(`/anime/${animeId}/hanime/stream`, {
      params: { slug }
    }),
}

export const anime18Api = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/anime18/episodes`),
  getStream: (animeId: string, episodeSlug: string) =>
    apiClient.get(`/anime/${animeId}/anime18/stream`, {
      params: { ep: episodeSlug }
    }),
}

export const themesApi = {
  list: (params?: { year?: number; season?: string; type?: string; sort?: string }) =>
    apiClient.get('/themes', { params }),
  get: (id: string) => apiClient.get(`/themes/${id}`),
  rate: (id: string, score: number) => apiClient.post(`/themes/${id}/rate`, { score }),
  unrate: (id: string) => apiClient.delete(`/themes/${id}/rate`),
  myRatings: (params?: { year?: number; season?: string }) =>
    apiClient.get('/themes/my-ratings', { params }),
}

export const adminThemesApi = {
  sync: () => apiClient.post('/themes/admin/sync'),
  syncStatus: () => apiClient.get('/themes/admin/sync/status'),
}

export const statusApi = {
  getStatus: () => apiClient.get('/status'),
}

export default apiClient
