// Pure decision logic for the aePlayer "connection problem" corner indicator.
// Framework-free so it is unit-testable without mounting the player.
//
// Owner TODO (feedback 2026-07-13T02-57-05_tNeymik_manual): surface a badge when
// a stall is a *real user-side* connection problem — NOT a dead provider. Dead
// sources (no bytes at all) are already owned by the failover watchdog
// (`useSourceFailover.ts`, which only advances when `fragLoadedCount === 0`) and
// the error overlay. This classifier therefore only fires when the transport is
// alive but not keeping up, plus the certain `navigator.onLine === false` case.

export type ConnectionState = 'ok' | 'slow' | 'offline'

/** How long playback must stay continuously buffering — after the first frame,
 *  with bytes still flowing — before we call it a slow *connection* rather than
 *  a normal in-buffer hiccup (a seek into an unbuffered region resolves fast).
 *  The composable owns the timer that turns this into the `sustained` flag. */
export const SLOW_SUSTAINED_MS = 4000

export interface ConnectionInputs {
  /** navigator.onLine — false is a certain, user-side "no connection". */
  online: boolean
  /** The player is currently showing its buffering state (`showBuffering`). */
  buffering: boolean
  /** First frame has played. Excludes initial-resolve buffering (loading), which
   *  the BufferingOverlay + failover chain already own. */
  hasStarted: boolean
  /** Fragments have loaded (`fragLoadedCount > 0`): bytes ARE flowing, so a stall
   *  here is slow transport, not a dead source. */
  bytesFlowing: boolean
  /** `buffering` has persisted past SLOW_SUSTAINED_MS (owned by the composable). */
  sustained: boolean
  /** A terminal source-error overlay is up — it owns the surface, so suppress the
   *  "slow" badge rather than blame the connection for a dead stream. */
  hasError: boolean
}

/** Classify the current connection health for the corner indicator. */
export function classifyConnection(i: ConnectionInputs): ConnectionState {
  // Offline is certain and the most useful thing to say — it wins even over an
  // error overlay (a dead stream while offline IS the offline problem).
  if (!i.online) return 'offline'
  // A terminal error overlay owns the surface; don't second-guess it as "slow".
  if (i.hasError) return 'ok'
  // Alive but not keeping up: buffering past the grace window, after the first
  // frame, with bytes still trickling in.
  if (i.hasStarted && i.buffering && i.bytesFlowing && i.sustained) return 'slow'
  return 'ok'
}
