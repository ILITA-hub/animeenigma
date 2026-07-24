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
  deleteNotification as apiDelete,
  click as apiClick,
} from '@/api/notifications'
import type { UserNotification, NewEpisodePayload } from '@/types/notification'

const POLL_INTERVAL_MS = 60_000

/** Page size for the "older notifications" history modal (backend caps at 100). */
const HISTORY_PAGE_SIZE = 30

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

  // History modal ("view older notifications") — offset-paged over the same
  // /notifications endpoint. Kept in the store (not the modal component) so
  // markRead/dismiss/markAllRead can keep both cached lists consistent: the
  // first page of history overlaps the dropdown's 20 rows as separate objects.
  const historyOpen = ref<boolean>(false)
  const historyItems = ref<UserNotification[]>([])
  const historyTotal = ref<number>(0)
  const historyLoading = ref<boolean>(false)
  const historyError = ref<string | null>(null)

  // ---------------------------------------------------------------------------
  // Internal (non-reactive) bookkeeping
  // ---------------------------------------------------------------------------

  let intervalId: ReturnType<typeof setInterval> | null = null
  let visListenerAttached = false
  let authListenerAttached = false
  let inFlight: Promise<void> | null = null
  let started = false
  /** Bumped on every openHistory() so a page response that lands after a
   *  reopen (or logout) can't append stale rows into the fresh session. */
  let historyEpoch = 0

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

  /** More history pages exist beyond what's loaded. */
  const hasMoreHistory = computed<boolean>(
    () => historyItems.value.length < historyTotal.value,
  )

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

  /** Blank the history session; a stale in-flight page no longer owns any
   *  of this state (the epoch bump orphans it). */
  function resetHistoryState(): void {
    historyEpoch += 1
    historyItems.value = []
    historyTotal.value = 0
    historyLoading.value = false
    historyError.value = null
  }

  /** Locate a notification in both cached lists (dropdown + history) —
   *  the same id lives in each as a separate object. */
  function locate(id: string) {
    const idx = notifications.value.findIndex((n) => n.id === id)
    const histIdx = historyItems.value.findIndex((n) => n.id === id)
    return {
      idx,
      histIdx,
      priorMain: idx >= 0 ? notifications.value[idx] : null,
      priorHist: histIdx >= 0 ? historyItems.value[histIdx] : null,
    }
  }

  /**
   * Open the history modal and load its first page. Always restarts from a
   * blank list — reopening must surface anything that arrived since.
   */
  function openHistory(): void {
    if (!isFeatureEnabled()) return
    resetHistoryState()
    historyOpen.value = true
    void fetchMoreHistory()
  }

  function closeHistory(): void {
    historyOpen.value = false
  }

  /**
   * Append the next history page (offset = rows already loaded). Deduped by
   * id because new notifications arriving mid-scroll shift offset pages.
   */
  async function fetchMoreHistory(): Promise<void> {
    if (!isFeatureEnabled()) return
    if (historyLoading.value) return
    if (historyItems.value.length > 0 && !hasMoreHistory.value) return

    const epoch = historyEpoch
    historyLoading.value = true
    try {
      const data = await listNotifications('history', HISTORY_PAGE_SIZE, historyItems.value.length)
      if (epoch !== historyEpoch) return
      const page = Array.isArray(data.notifications) ? data.notifications : []
      const seen = new Set(historyItems.value.map((n) => n.id))
      const fresh = page.filter((n) => !seen.has(n.id))
      historyItems.value = [...historyItems.value, ...fresh]
      historyTotal.value =
        fresh.length === 0
          ? // Empty page (the end) or all-duplicates (offset drift): stop
            // paging rather than spin on identical requests.
            historyItems.value.length
          : typeof data.total === 'number'
            ? data.total
            : historyItems.value.length
      if (typeof data.unread_count === 'number') {
        unreadCount.value = data.unread_count
      }
      historyError.value = null
    } catch (err) {
      if (epoch !== historyEpoch) return
      historyError.value = err instanceof Error ? err.message : String(err)
    } finally {
      if (epoch === historyEpoch) historyLoading.value = false
    }
  }

  /**
   * Optimistic mark-read: flip the local read_at + decrement count,
   * then fire the API call. Rollback on error. Flips the row in BOTH the
   * dropdown list and the history list (separate objects for the same id).
   */
  async function markRead(id: string): Promise<void> {
    const { idx, histIdx, priorMain, priorHist } = locate(id)
    const prior = priorMain ?? priorHist
    if (!prior) return
    if (prior.read_at) return // already read — no-op
    // Dismissed rows are outside the unread count (server excludes them);
    // flipping one here would wrongly decrement the badge.
    if (prior.dismissed_at) return

    const now = new Date().toISOString()
    if (priorMain) notifications.value[idx] = { ...priorMain, read_at: now }
    if (priorHist) historyItems.value[histIdx] = { ...priorHist, read_at: now }
    unreadCount.value = Math.max(0, unreadCount.value - 1)

    try {
      await apiMarkRead(id)
    } catch (err) {
      // Rollback
      if (priorMain) notifications.value[idx] = priorMain
      if (priorHist) historyItems.value[histIdx] = priorHist
      unreadCount.value = unreadCount.value + 1
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    }
  }

  /**
   * Optimistic dismiss: remove from the dropdown list, stamp dismissed_at
   * on the history copy (history keeps dismissed rows, rendered dimmed),
   * decrement the badge if it was unread. Rollback on error.
   */
  async function dismissNotification(id: string): Promise<void> {
    const { idx, histIdx, priorMain, priorHist } = locate(id)
    const prior = priorMain ?? priorHist
    if (!prior) return
    if (prior.dismissed_at) return // already dismissed — no-op
    const wasUnread = !prior.read_at

    if (priorMain) {
      notifications.value = notifications.value.filter((n) => n.id !== id)
    }
    if (priorHist) {
      historyItems.value[histIdx] = {
        ...priorHist,
        dismissed_at: new Date().toISOString(),
      }
    }
    if (wasUnread) {
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    }

    try {
      await apiDismiss(id)
    } catch (err) {
      // Rollback — splice the dropdown row back at its original position
      // so order is preserved; restore the un-stamped history copy.
      if (priorMain) {
        const next = [...notifications.value]
        next.splice(idx, 0, priorMain)
        notifications.value = next
      }
      if (priorHist) {
        historyItems.value[histIdx] = priorHist
      }
      if (wasUnread) {
        unreadCount.value = unreadCount.value + 1
      }
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    }
  }

  /**
   * Optimistic delete ("bin" in the history modal): remove the row from BOTH
   * cached lists, shrink the history total (deleted rows leave the history
   * scope server-side, unlike dismissed rows), and drop the badge if the row
   * was still counting toward it. Rollback on error.
   */
  async function deleteNotification(id: string): Promise<void> {
    const { idx, histIdx, priorMain, priorHist } = locate(id)
    const prior = priorMain ?? priorHist
    if (!prior) return
    if (prior.deleted_at) return // already deleted — no-op
    // A row feeds the badge only while unread AND not dismissed; deleting one
    // in that state must drop the count, deleting any other must not.
    const wasCountingUnread = !prior.read_at && !prior.dismissed_at

    if (priorMain) {
      notifications.value = notifications.value.filter((n) => n.id !== id)
    }
    if (priorHist) {
      historyItems.value = historyItems.value.filter((n) => n.id !== id)
      historyTotal.value = Math.max(0, historyTotal.value - 1)
    }
    if (wasCountingUnread) {
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    }

    try {
      await apiDelete(id)
    } catch (err) {
      // Rollback — splice both rows back at their original positions so order
      // (and the history offset denominator) is preserved.
      if (priorMain) {
        const next = [...notifications.value]
        next.splice(idx, 0, priorMain)
        notifications.value = next
      }
      if (priorHist) {
        const next = [...historyItems.value]
        next.splice(histIdx, 0, priorHist)
        historyItems.value = next
        historyTotal.value = historyTotal.value + 1
      }
      if (wasCountingUnread) {
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
    const priorHist = historyItems.value
    const priorCount = unreadCount.value
    const now = new Date().toISOString()
    notifications.value = prior.map((n) => (n.read_at ? n : { ...n, read_at: now }))
    // Dismissed history rows stay untouched — the server's mark-all-read
    // only stamps active rows.
    historyItems.value = priorHist.map((n) =>
      n.read_at || n.dismissed_at ? n : { ...n, read_at: now },
    )
    unreadCount.value = 0

    try {
      await apiMarkAllRead()
    } catch (err) {
      notifications.value = prior
      historyItems.value = priorHist
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

    resetHistoryState()
    historyOpen.value = false
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
    historyOpen,
    historyItems,
    historyTotal,
    historyLoading,
    historyError,
    // getters
    latestUndismissedToast,
    hasMoreHistory,
    // actions
    fetchNotifications,
    markRead,
    dismiss: dismissNotification,
    delete: deleteNotification,
    markAllRead,
    handleClick,
    start,
    stop,
    markToastShown,
    openHistory,
    closeHistory,
    fetchMoreHistory,
  }
})
