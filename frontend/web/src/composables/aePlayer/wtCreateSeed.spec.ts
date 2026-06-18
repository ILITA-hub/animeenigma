/**
 * Workstream watch-together — Plan A.
 *
 * Unit spec for wtCreateSeed(): the pure helper that turns AePlayer's live
 * source combo + current episode into the create-room seed consumed by
 * InviteButton (player / translation_id / episode_id).
 *
 * The whole point of the helper is that when the aePlayer surface is the
 * active one at room-create time, the POST /rooms payload must be:
 *   { player: 'aeplayer', translation_id: comboToToken(combo), episode_id: String(ep) }
 *
 * so every room member resolves the SAME stream (combo carried opaquely in
 * translation_id, episode number stringified in episode_id).
 */

import { describe, it, expect } from 'vitest'
import { wtCreateSeed } from './wtCreateSeed'
import { comboToToken } from './comboMapping'
import type { Combo } from '@/types/aePlayer'

const combo: Combo = {
  audio: 'dub',
  lang: 'en',
  provider: 'allanime',
  server: 'kiwi',
  team: 'SubsPlease',
}

describe('wtCreateSeed', () => {
  it('builds an aeplayer seed from a resolved combo + episode', () => {
    const seed = wtCreateSeed(combo, 7)
    expect(seed).toEqual({
      player: 'aeplayer',
      translation_id: comboToToken(combo),
      episode_id: '7',
    })
  })

  it('carries every combo field opaquely through translation_id', () => {
    const seed = wtCreateSeed(combo, 1)!
    expect(seed).not.toBeNull()
    // The token round-trips back to the same five fields.
    expect(JSON.parse(seed.translation_id)).toEqual({
      provider: 'allanime',
      audio: 'dub',
      lang: 'en',
      team: 'SubsPlease',
      server: 'kiwi',
    })
  })

  it('stringifies the episode number into episode_id', () => {
    expect(wtCreateSeed(combo, 12)!.episode_id).toBe('12')
    expect(wtCreateSeed(combo, 1)!.episode_id).toBe('1')
  })

  it('coerces a null team to null in the token (no crash)', () => {
    const seed = wtCreateSeed({ ...combo, team: null }, 3)!
    expect(seed).not.toBeNull()
    expect(JSON.parse(seed.translation_id).team).toBeNull()
  })

  it('returns null when the combo has no resolved provider yet', () => {
    // provider==='' is AePlayer's un-resolved initial state; a room seeded
    // with an empty provider would force every joiner into a blank re-resolve,
    // so the helper refuses to build a seed until a real source is picked.
    expect(wtCreateSeed({ ...combo, provider: '' }, 5)).toBeNull()
  })

  it('returns null when the episode is not a positive number', () => {
    expect(wtCreateSeed(combo, 0)).toBeNull()
    expect(wtCreateSeed(combo, NaN)).toBeNull()
    expect(wtCreateSeed(combo, -2)).toBeNull()
  })
})
