import { reactive } from 'vue'

/**
 * sourceFallbackDebug — shared record of source auto-fallback DECISIONS.
 *
 * When a video source fails (a saved provider returns "not available", a stream
 * dies mid-playback, …) the player can auto-switch to the next-best provider.
 * That convenience hides what the resolver actually decided — which is exactly
 * what you want to inspect while tuning the smart-source ranking.
 *
 * In HACKER MODE the player does NOT auto-switch. Instead every fallback it
 * WOULD have performed is recorded here as an "intent" (`acted = false`), so the
 * behavior can be verified manually before it's trusted to act on its own. With
 * hacker mode off the same records are written with `acted = true` the moment a
 * switch is performed, giving a uniform ledger either way.
 *
 * UnifiedPlayer.vue writes; DebugHud.vue renders the SOURCE FALLBACK section.
 * Every intent is also mirrored to the console (prefix `[SourceFallback]`) so it
 * is checkable even when the HUD is hidden.
 */

export interface FallbackIntent {
  /** seconds since the channel epoch (current title), for ordering/display */
  at: number
  /** provider that failed / was unavailable */
  from: string
  /** provider the player would switch to; null = no eligible candidate */
  to: string | null
  /** why the fallback triggered, e.g. 'saved source unavailable' */
  reason: string
  /** false = logged-only (hacker mode); true = the switch was performed */
  acted: boolean
}

interface SourceFallbackDebugState {
  /** timestamped ring buffer, newest last */
  intents: FallbackIntent[]
}

const INTENTS_MAX = 20

export const sourceFallbackDebug: SourceFallbackDebugState = reactive({ intents: [] })

let epoch = typeof performance !== 'undefined' ? performance.now() : 0

/**
 * Record one source-fallback decision (an intent in hacker mode, an executed
 * switch otherwise). Always pushed to the ring buffer AND mirrored to console.
 */
export function recordFallbackIntent(intent: Omit<FallbackIntent, 'at'>): void {
  const t = typeof performance !== 'undefined' ? performance.now() : 0
  const rec: FallbackIntent = { at: (t - epoch) / 1000, ...intent }
  sourceFallbackDebug.intents.push(rec)
  if (sourceFallbackDebug.intents.length > INTENTS_MAX) sourceFallbackDebug.intents.shift()
  // eslint-disable-next-line no-console
  console.info(
    `[SourceFallback] ${intent.acted ? 'SWITCH' : 'INTENT (hacker — not switching)'}: ` +
      `${intent.from} → ${intent.to ?? '(no candidate)'} · ${intent.reason}`,
  )
}

/** Reset the ledger on a new title — a fresh anime gets a clean epoch. */
export function resetFallbackIntents(): void {
  epoch = typeof performance !== 'undefined' ? performance.now() : 0
  sourceFallbackDebug.intents = []
}
