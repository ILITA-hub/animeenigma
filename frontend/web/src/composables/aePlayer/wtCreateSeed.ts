import type { Combo } from '@/types/aePlayer'
import type { PlayerKind } from '@/api/watch-together'
import { comboToToken } from './comboMapping'

/** The create-room seed AePlayer hands to InviteButton when the aePlayer
 *  surface is the active one. Carries the combo opaquely in `translation_id`
 *  (so every joiner resolves the same stream) and the episode NUMBER as a
 *  string in `episode_id` — the shape AePlayer's room-sync watchers expect. */
export interface WtCreateSeed {
  player: Extract<PlayerKind, 'aeplayer'>
  translation_id: string
  episode_id: string
}

/**
 * Build the create-room seed from AePlayer's live combo + current episode.
 *
 * Returns `null` (no usable seed) when the source isn't resolved yet — an
 * empty `provider` (AePlayer's initial state) or a non-positive episode
 * number. Seeding a room with either would force joiners into a blank
 * re-resolve, so we refuse and let the legacy kodik default stand.
 */
export function wtCreateSeed(combo: Combo, episode: number): WtCreateSeed | null {
  if (!combo.provider) return null
  if (!Number.isFinite(episode) || episode <= 0) return null
  return {
    player: 'aeplayer',
    translation_id: comboToToken({
      provider: combo.provider,
      audio: combo.audio,
      lang: combo.lang,
      team: combo.team,
      server: combo.server,
    }),
    episode_id: String(episode),
  }
}
