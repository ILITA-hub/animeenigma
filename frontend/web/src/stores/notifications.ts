/**
 * Pinia store for the notifications engine — v1.0 Phase 3.
 *
 * Responsibilities:
 *   - Hold the cached active notification list (unread + read — read rows
 *     stay visible in the dropdown, tinted by the card renderers)
 *   - Drive the 60s polling lifecycle (visibilitychange-paused, single-flight)
 *   - Optimistic mark-read / dismiss / mark-all-read
 *   - Track which notifications have shown a toast this session
 *   - Translate the backend's `watch_url` shape into a vue-router target
 *
 * The store is intentionally framework-agnostic on the click path:
 * `handleClick(notification, push)` accepts the router's `push` function
 * as a callback rather than calling `useRouter()` inside the action
 * (composables don't work in store actions outside setup scope).
 *
 * Workstream: notifications, Phase 3.
 */

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Router, RouteLocationRaw } from 'vue-router'

import {
  listNotifications,
  markRead as apiMarkRead,
  markAllRead as apiMarkAllRead,
  dismiss as apiDismiss,
  click as apiClick,
} from '@/api/notifications'
import type { UserNotification, NewEpisodePayload } from '@/types/notification'

const POLL_INTERVAL_MS = 60_000

/**
 * Minimum gap between "immediate" fetches (start(), tab-visible). Rapid
 * visibility flaps and auth-watcher restarts used to fire several unread
 * fetches within seconds of page load; within this window the cached list is
 * plenty fresh. The 60s ticker and explicit force-refreshes (bell open)
 * bypass it. Page-fetch optimization 2026-06-11.
 */
const MIN_FETCH_GAP_MS = 15_000

/** localStorage key for the persisted toast-shown id set. */
const SHOWN_TOAST_STORAGE_KEY = 'notif:shownToasts'

/**
 * Load the persisted set of toast-shown notification ids.
 *
 * Persisting across reloads is the fix for the toast re-fire bug: without it,
 * the in-memory set resets on every page load, so a still-unread notification
 * pops a fresh toast each time the page is opened. Fails safe to an empty set
 * on SSR / parse error / disabled storage.
 */
function loadShownToastIds(): Set<string> {
  if (typeof localStorage === 'undefined') return new Set()
  try {
    const raw = localStorage.getItem(SHOWN_TOAST_STORAGE_KEY)
    if (!raw) return new Set()
    const arr: unknown = JSON.parse(raw)
    return Array.isArray(arr)
      ? new Set(arr.filter((x): x is string => typeof x === 'string'))
      : new Set()
  } catch {
    return new Set()
  }
}

/** Persist the toast-shown id set. Best-effort; storage errors are non-fatal. */
function persistShownToastIds(ids: Set<string>): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(SHOWN_TOAST_STORAGE_KEY, JSON.stringify([...ids]))
  } catch {
    /* storage full / disabled — suppression degrades to session-only */
  }
}

/** Drop the persisted toast-shown id set (called on logout). */
function clearShownToastIds(): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.removeItem(SHOWN_TOAST_STORAGE_KEY)
  } catch {
    /* non-fatal */
  }
}

/**
 * Read the build-time feature flag. Defaults to TRUE (feature on) when
 * unset; only the literal string `'false'` disables the engine. The flag
 * exists for emergency rollback (see Phase 3 plan §Rollback).
 */
function isFeatureEnabled(): boolean {
  // import.meta.env is replaced at build time, so this is constant per build.
  const raw = (import.meta as ImportMeta & { env: Record<string, string | undefined> })
    .env.VITE_NOTIFICATIONS_ENABLED
  return raw !== 'false'
}

/**
 * Parse the backend's `watch_url` into a vue-router target.
 *
 * Backend ships `/anime/{id}/watch?provider=X&team=Y&episode=N`.
 * The live frontend route is `/anime/:id`, which consumes `?episode=N`
 * (lands the user on the episode) and `?provider=`/`?team=` (aePlayer
 * preselects that source + team on mount — see Anime.vue queryProvider).
 * This helper unwraps the `/watch` suffix and preserves all query params.
 *
 * Defensive: if the URL doesn't match the expected shape, return a raw
 * push target with the original path — vue-router will surface a 404
 * via the catch-all route, which the router alias in router/index.ts
 * also redirects to the canonical form.
 *
 * Exported for unit-testing.
 */
export function translateWatchUrl(url: string): RouteLocationRaw {
  if (!url || typeof url !== 'string') {
    return { path: '/' }
  }

  // Use URL parsing so query handling is correct (multiple values, encoding).
  // Anchor against a dummy base because `watch_url` is relative.
  let parsed: URL
  try {
    parsed = new URL(url, 'http://_anchor')
  } catch {
    return { path: url }
  }

  const path = parsed.pathname || '/'
  const query: Record<string, string> = {}
  parsed.searchParams.forEach((value, key) => {
    query[key] = value
  })

  // Pattern: /anime/{id}/watch  → /anime/{id}
  const match = path.match(/^\/anime\/([^/]+)\/watch\/?$/)
  if (match) {
    return { path: `/anime/${match[1]}`, query }
  }

  // Already the canonical /anime/:id shape (or any other URL — let the
  // router sort it out). Preserve all query params either way.
  return { path, query }
}

export const useNotificationsStore = defineStore('notifications', () => {
  // ---------------------------------------------------------------------------
  // Reactive state
  // ---------------------------------------------------------------------------

  const notifications = ref<UserNotification[]>([])
  const unreadCount = ref<number>(0)
  /** Toast-suppression set. Persisted to localStorage so a page reload does
   *  not re-toast an already-shown, still-unread notification. */
  const shownToastIds = ref<Set<string>>(loadShownToastIds())
  const polling = ref<boolean>(false)
  const lastFetchAt = ref<number>(0)
  const error = ref<string | null>(null)

  // ---------------------------------------------------------------------------
  // Internal (non-reactive) bookkeeping
  // ---------------------------------------------------------------------------

  let intervalId: ReturnType<typeof setInterval> | null = null
  let visListenerAttached = false
  let authListenerAttached = false
  let inFlight: Promise<void> | null = null
  let started = false

  // ---------------------------------------------------------------------------
  // Getters
  // ---------------------------------------------------------------------------

  /**
   * The next notification that should pop a toast: oldest-first traversal
   * of the cached list, skipping anything already shown this session,
   * read, or dismissed.
   *
   * Toast suppression by route (when the user is already on the matching
   * anime view) is enforced in NotificationToast.vue, not here — the
   * store should never need to know about the current route.
   */
  const latestUndismissedToast = computed<UserNotification | null>(() => {
    for (const n of notifications.value) {
      if (shownToastIds.value.has(n.id)) continue
      if (n.read_at) continue
      if (n.dismissed_at) continue
      return n
    }
    return null
  })

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  /**
   * Fetch the active notification list (read + unread) + counts. Read
   * rows are kept so the dropdown can render them tinted instead of
   * making them vanish the moment they're read. Single-flight:
   * concurrent callers (e.g. visibility-change racing the interval
   * tick) wait on the already-running promise rather than firing
   * duplicate requests.
   */
  async function fetchNotifications(opts?: { ifStale?: boolean }): Promise<void> {
    if (!isFeatureEnabled()) return
    if (inFlight) return inFlight
    // Freshness guard for the "immediate" callers (start, tab-visible):
    // a fetch that landed within the gap is recent enough.
    if (opts?.ifStale && Date.now() - lastFetchAt.value < MIN_FETCH_GAP_MS) return

    inFlight = (async () => {
      try {
        const data = await listNotifications('all', 20, 0)
        notifications.value = Array.isArray(data.notifications)
          ? data.notifications
          : []
        unreadCount.value = typeof data.unread_count === 'number'
          ? data.unread_count
          : 0
        lastFetchAt.value = Date.now()
        error.value = null
        // Belt + suspenders: prune shownToastIds to only IDs still
        // present in the live list. Prevents unbounded Set growth
        // across long sessions where many notifications come and go.
        if (shownToastIds.value.size > 0) {
          const liveIds = new Set(notifications.value.map((n) => n.id))
          const next = new Set<string>()
          for (const id of shownToastIds.value) {
            if (liveIds.has(id)) next.add(id)
          }
          shownToastIds.value = next
          persistShownToastIds(next)
        }
      } catch (err) {
        // Silent-fail: a bell that occasionally goes stale is preferable
        // to a noisy console for a routine network hiccup. Only stash
        // the most recent error message for debug surfaces.
        error.value = err instanceof Error ? err.message : String(err)
      } finally {
        inFlight = null
      }
    })()

    return inFlight
  }

  /**
   * Optimistic mark-read: flip the local read_at + decrement count,
   * then fire the API call. Rollback on error.
   */
  async function markRead(id: string): Promise<void> {
    const idx = notifications.value.findIndex((n) => n.id === id)
    if (idx < 0) return
    const prior = notifications.value[idx]
    if (prior.read_at) return // already read — no-op

    const now = new Date().toISOString()
    notifications.value[idx] = { ...prior, read_at: now }
    unreadCount.value = Math.max(0, unreadCount.value - 1)

    try {
      await apiMarkRead(id)
    } catch (err) {
      // Rollback
      notifications.value[idx] = prior
      unreadCount.value = unreadCount.value + 1
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    }
  }

  /**
   * Optimistic dismiss: remove from local list, decrement count if it
   * was unread. Rollback on error.
   */
  async function dismissNotification(id: string): Promise<void> {
    const idx = notifications.value.findIndex((n) => n.id === id)
    if (idx < 0) return
    const prior = notifications.value[idx]
    const wasUnread = !prior.read_at && !prior.dismissed_at

    notifications.value = notifications.value.filter((n) => n.id !== id)
    if (wasUnread) {
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    }

    try {
      await apiDismiss(id)
    } catch (err) {
      // Rollback — splice back at the original position so order is
      // preserved.
      const next = [...notifications.value]
      next.splice(idx, 0, prior)
      notifications.value = next
      if (wasUnread) {
        unreadCount.value = unreadCount.value + 1
      }
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    }
  }

  /**
   * Optimistic mark-all-read: zero badge + stamp read_at on every cached
   * row. Rollback on error.
   */
  async function markAllRead(): Promise<void> {
    const prior = notifications.value
    const priorCount = unreadCount.value
    const now = new Date().toISOString()
    notifications.value = prior.map((n) => (n.read_at ? n : { ...n, read_at: now }))
    unreadCount.value = 0

    try {
      await apiMarkAllRead()
    } catch (err) {
      notifications.value = prior
      unreadCount.value = priorCount
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    }
  }

  /**
   * Fire-and-forget click telemetry + navigation. The frontend MUST NOT
   * wait on the click API to navigate — the click event is pure
   * telemetry; navigation is the user-visible action.
   *
   * Marks the toast as "shown" so the same notification doesn't re-toast
   * after click-through navigation triggers a new fetch.
   */
  function handleClick(notification: UserNotification, router: Router): void {
    shownToastIds.value.add(notification.id)
    // Fire-and-forget click telemetry — never block navigation.
    void apiClick(notification.id).catch(() => {
      /* telemetry failure is non-fatal; navigation has already happened */
    })

    // Also mark the notification as read (optimistic) so the badge drops.
    // Failures rollback inside markRead; we explicitly catch so we don't
    // surface an unhandled rejection from a fire-and-forget call.
    void markRead(notification.id).catch(() => { /* silent — non-fatal */ })

    // Extract the watch URL from the typed payload. For unknown types,
    // the renderer registry handles rendering — but click navigation
    // only makes sense for known types. Suppress navigation on unknown.
    if (notification.type === 'new_episode') {
      const payload = notification.payload as NewEpisodePayload | null
      if (payload && typeof payload.watch_url === 'string' && payload.watch_url) {
        router.push(translateWatchUrl(payload.watch_url))
      }
    }
  }

  // ---------------------------------------------------------------------------
  // Polling lifecycle
  // ---------------------------------------------------------------------------

  function onVisibilityChange(): void {
    if (typeof document === 'undefined') return
    if (document.hidden) {
      // Tab hidden — pause the polling ticker. Listener stays attached
      // so we can resume on regain.
      if (intervalId !== null) {
        clearInterval(intervalId)
        intervalId = null
      }
    } else if (polling.value && intervalId === null) {
      // Tab visible — refresh if stale, then resume cadence. The
      // single-flight guard inside fetchNotifications() prevents the
      // immediate refresh from racing the just-restarted interval; the
      // ifStale guard keeps rapid tab flaps from bursting requests.
      void fetchNotifications({ ifStale: true })
      intervalId = setInterval(() => {
        void fetchNotifications()
      }, POLL_INTERVAL_MS)
    }
  }

  function onAuthExpired(): void {
    // Hard-stop on confirmed 401: clear state + tear down polling. The
    // auth listener in App.vue will also fire when isAuthenticated
    // flips, but the explicit 'auth:expired' event is the authoritative
    // signal for "stop now" — covers the case where the user was logged
    // in but the refresh token expired.
    stop()
  }

  /**
   * Idempotent start: kicks off an immediate fetch + the 60s interval +
   * the visibilitychange listener. No-op if already started, or if the
   * feature is disabled by the env flag.
   */
  function start(): void {
    if (!isFeatureEnabled()) return
    if (started) return

    started = true
    polling.value = true

    if (typeof document !== 'undefined' && !visListenerAttached) {
      document.addEventListener('visibilitychange', onVisibilityChange)
      visListenerAttached = true
    }
    if (typeof window !== 'undefined' && !authListenerAttached) {
      window.addEventListener('auth:expired', onAuthExpired)
      authListenerAttached = true
    }

    // Immediate fetch — don't wait the first 60s. ifStale: a stop()/start()
    // flap (auth watcher re-firing on boot) must not duplicate the fetch.
    void fetchNotifications({ ifStale: true })
    if (typeof document === 'undefined' || !document.hidden) {
      intervalId = setInterval(() => {
        void fetchNotifications()
      }, POLL_INTERVAL_MS)
    }
  }

  /**
   * Stop polling + clear all in-memory state. Idempotent. Listeners are
   * detached so a subsequent `start()` re-attaches them cleanly.
   */
  function stop(): void {
    started = false
    polling.value = false

    if (intervalId !== null) {
      clearInterval(intervalId)
      intervalId = null
    }
    if (visListenerAttached && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibilityChange)
      visListenerAttached = false
    }
    if (authListenerAttached && typeof window !== 'undefined') {
      window.removeEventListener('auth:expired', onAuthExpired)
      authListenerAttached = false
    }

    notifications.value = []
    unreadCount.value = 0
    shownToastIds.value = new Set()
    clearShownToastIds()
    inFlight = null
    error.value = null
    lastFetchAt.value = 0
  }

  /** Mark a toast as shown (persisted) without server state changes. */
  function markToastShown(id: string): void {
    shownToastIds.value.add(id)
    persistShownToastIds(shownToastIds.value)
  }

  return {
    // state
    notifications,
    unreadCount,
    shownToastIds,
    polling,
    lastFetchAt,
    error,
    // getters
    latestUndismissedToast,
    // actions
    fetchNotifications,
    markRead,
    dismiss: dismissNotification,
    markAllRead,
    handleClick,
    start,
    stop,
    markToastShown,
  }
})
