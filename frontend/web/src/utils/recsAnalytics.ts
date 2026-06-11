import { apiClient } from '@/api/client'

// Phase 14 (REC-EVAL-01): rec_click + rec_watched emit pipeline.
//
// Click→watched correlation is purely client-side: emitRecClick stores a
// small FIFO buffer in localStorage. When the player marks an episode watched
// (auto or manual), it calls emitRecWatchedIfRecent which looks up the most
// recent click for that anime within the last 7 days (ISS-026: 1h was too
// narrow — missed most real watch sessions). If a match exists the event is
// emitted and the click is removed so each click converts at most once
// (fire-once, ISS-026). No match → no emit (strict correlation per spec §11.5;
// session-based attribution deferred to v2.1).
//
// Telemetry is best-effort: API failures are swallowed so a click or an
// auto-mark is never blocked by a network blip on the events endpoint.

const STORAGE_KEY = 'recentRecClicks'
const MAX_ENTRIES = 50
const TTL_MS = 7 * 24 * 60 * 60 * 1000 // 7 days — ISS-026: 1h missed most real watch sessions

export type PinSource = 'local' | 'shikimori_similar' | 'score_5_fallback'

export interface RecClickPayload {
  event_type: 'rec_click'
  anime_id: string
  signal_id: string // 's6_pin' for pinned items, otherwise top_contributor
  pinned: boolean
  pin_source?: PinSource
  pin_seed_anime_id?: string
  source_route?: string
  rank?: number
}

export interface RecWatchedPayload {
  event_type: 'rec_watched'
  anime_id: string
  signal_id: string
  pinned: boolean
  pin_source?: string
  pin_seed_anime_id?: string
  source_route?: string
  rank?: number
}

interface StoredClick {
  anime_id: string
  signal_id: string
  pinned: boolean
  pin_source?: string
  pin_seed_anime_id?: string
  rank?: number
  timestamp: number // ms epoch
}

function readStore(): StoredClick[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as StoredClick[]
    if (!Array.isArray(parsed)) return []
    const now = Date.now()
    return parsed.filter((c) => typeof c?.timestamp === 'number' && now - c.timestamp < TTL_MS)
  } catch {
    return []
  }
}

function writeStore(entries: StoredClick[]): void {
  try {
    // Keep only the last MAX_ENTRIES entries (FIFO bounded buffer).
    const trimmed = entries.slice(-MAX_ENTRIES)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed))
  } catch {
    // localStorage quota or disabled — telemetry is best-effort.
  }
}

/**
 * findRecentClick returns the most recent click for the given anime_id within
 * the TTL window (7 days, ISS-026), or null if none.
 */
export function findRecentClick(animeId: string): StoredClick | null {
  const store = readStore()
  // Most recent first — the FIFO buffer is appended chronologically, so
  // iterate from the tail.
  for (let i = store.length - 1; i >= 0; i--) {
    if (store[i].anime_id === animeId) return store[i]
  }
  return null
}

/**
 * emitRecClick fires POST /api/events/rec with the click payload AND
 * stores the click in localStorage for later correlation. Failures are
 * swallowed — telemetry must never break a click.
 */
export async function emitRecClick(payload: RecClickPayload): Promise<void> {
  const store = readStore()
  store.push({
    anime_id: payload.anime_id,
    signal_id: payload.signal_id,
    pinned: payload.pinned,
    pin_source: payload.pin_source,
    pin_seed_anime_id: payload.pin_seed_anime_id,
    rank: payload.rank,
    timestamp: Date.now(),
  })
  writeStore(store)
  try {
    await apiClient.post('/events/rec', payload)
  } catch {
    // Best-effort — already logged the click locally.
  }
}

/**
 * emitRecWatched fires POST /api/events/rec with the watched payload. Caller
 * is responsible for finding the matching click (via findRecentClick) and
 * threading the signal_id through.
 */
export async function emitRecWatched(payload: RecWatchedPayload): Promise<void> {
  try {
    await apiClient.post('/events/rec', payload)
  } catch {
    // Best-effort.
  }
}

/**
 * removeClick deletes all stored clicks for an anime — called after a
 * successful rec_watched emit so each click converts at most once (ISS-026).
 */
function removeClick(animeId: string): void {
  writeStore(readStore().filter((c) => c.anime_id !== animeId))
}

/**
 * emitRecWatchedIfRecent is the one call players make on mark-watched
 * (auto or manual): looks up the most recent rec click for this anime
 * within the TTL window (7 days, ISS-026), emits rec_watched with the
 * originating signal_id, and removes the click (fire-once). No click → no-op.
 */
export async function emitRecWatchedIfRecent(animeId: string, sourceRoute: string): Promise<void> {
  const recent = findRecentClick(animeId)
  if (!recent) return
  removeClick(animeId)
  await emitRecWatched({
    event_type: 'rec_watched',
    anime_id: animeId,
    signal_id: recent.signal_id,
    pinned: recent.pinned,
    pin_source: recent.pin_source,
    pin_seed_anime_id: recent.pin_seed_anime_id,
    source_route: sourceRoute,
    rank: recent.rank,
  })
}
