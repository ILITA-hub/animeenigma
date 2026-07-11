// Pure decision logic for aePlayer terminal playback-failure telemetry.
// Kept framework-free so it is unit-testable without mounting the player.
// See docs/superpowers/specs/2026-07-11-aeplayer-playback-failure-alert-design.md.

export type FailureTag = 'ae_failed' | 'all_exhausted'

/** The `advanceToNextSource` reason for a source simply not carrying the
 *  requested episode (incl. ae's partial library / a not-yet-aired episode).
 *  This is a content/scheduling gap, NOT a playback failure. */
export const EPISODE_GAP_REASON = 'source missing the requested episode'

export interface FailureInputs {
  /** The `advanceToNextSource` reason string. */
  reason: string
  /** The provider being abandoned (`state.combo.value.provider`). */
  failingProvider: string
  hackerMode: boolean
  roomPinned: boolean
  providerAutoSelected: boolean
  /** Whether the auto-failover chain still has an untried candidate. */
  candidateExists: boolean
  /** Whether the per-attempt switch cap has been reached. */
  attemptsExceeded: boolean
}

export interface FailureDecision {
  emit: boolean
  tag?: FailureTag
  exhausted?: boolean
}

/** Decide whether this failure is a terminal, alert-worthy playback failure. */
export function classifyPlaybackFailure(i: FailureInputs): FailureDecision {
  // Owner debugging / content gaps never count.
  if (i.hackerMode) return { emit: false }
  if (i.reason === EPISODE_GAP_REASON) return { emit: false }

  const isAe = i.failingProvider === 'ae'
  // Will the auto chain actually switch to another source?
  const willSwitch =
    i.candidateExists && i.providerAutoSelected && !i.roomPinned && !i.attemptsExceeded
  // The auto chain ran out → the viewer sees the error overlay.
  const exhausted = !willSwitch && i.providerAutoSelected && !i.roomPinned

  if (isAe) return { emit: true, tag: 'ae_failed', exhausted }
  if (exhausted) return { emit: true, tag: 'all_exhausted', exhausted: true }
  return { emit: false }
}

/** Map an `advanceToNextSource` reason to a coarse error_kind. */
export function mapErrorKind(reason: string): string {
  if (reason === 'silent stall') return 'stall_timeout'
  if (reason === 'playback fatal') return 'playback_fatal'
  return 'stream_error'
}
