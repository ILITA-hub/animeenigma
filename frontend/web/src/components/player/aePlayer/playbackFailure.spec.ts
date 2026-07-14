import { describe, it, expect } from 'vitest'
import {
  classifyPlaybackFailure,
  mapErrorKind,
  EPISODE_GAP_REASON,
  type FailureInputs,
} from './playbackFailure'

const base: FailureInputs = {
  reason: 'resolve failed',
  failingProvider: 'gogoanime',
  hackerMode: false,
  roomPinned: false,
  providerAutoSelected: true,
  candidateExists: true,
  attemptsExceeded: false,
  firstParty: false,
}

describe('classifyPlaybackFailure', () => {
  it('does not emit while a candidate exists (failover will recover)', () => {
    expect(classifyPlaybackFailure(base).emit).toBe(false)
  })

  it('emits all_exhausted when the auto chain has no candidate left', () => {
    const d = classifyPlaybackFailure({ ...base, candidateExists: false })
    expect(d).toEqual({ emit: true, tag: 'all_exhausted', exhausted: true })
  })

  it('emits all_exhausted when the switch-attempt cap is hit', () => {
    const d = classifyPlaybackFailure({ ...base, attemptsExceeded: true })
    expect(d).toEqual({ emit: true, tag: 'all_exhausted', exhausted: true })
  })

  it('emits ae_failed when the first-party source fails, even with a candidate', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae', firstParty: true })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
  })

  it('emits ae_failed (exhausted) when ae was the last candidate', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae', firstParty: true, candidateExists: false })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: true })
  })

  it('never emits in hacker mode', () => {
    expect(classifyPlaybackFailure({ ...base, candidateExists: false, hackerMode: true }).emit).toBe(false)
    expect(classifyPlaybackFailure({ ...base, failingProvider: 'ae', firstParty: true, hackerMode: true }).emit).toBe(false)
  })

  it('never emits for the content-gap reason', () => {
    expect(classifyPlaybackFailure({ ...base, candidateExists: false, reason: EPISODE_GAP_REASON }).emit).toBe(false)
    expect(classifyPlaybackFailure({ ...base, failingProvider: 'ae', firstParty: true, reason: EPISODE_GAP_REASON }).emit).toBe(false)
  })

  it('does not emit for a manual non-ae pick failure', () => {
    expect(classifyPlaybackFailure({ ...base, providerAutoSelected: false, candidateExists: false }).emit).toBe(false)
  })

  it('emits ae_failed for a manual ae pick failure', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae', firstParty: true, providerAutoSelected: false })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
  })

  it('tags ae_failed for ANY first-party provider (group, not id)', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae2', firstParty: true, candidateExists: true })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
  })

  it('does not treat a non-firstparty provider named ae-like as first-party', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'aegis', firstParty: false, candidateExists: true })
    expect(d.emit).toBe(false)
  })
})

describe('mapErrorKind', () => {
  it('maps known failure reasons', () => {
    expect(mapErrorKind('silent stall')).toBe('stall_timeout')
    expect(mapErrorKind('playback fatal')).toBe('playback_fatal')
    expect(mapErrorKind('resolve failed')).toBe('stream_error')
    expect(mapErrorKind('anything else')).toBe('stream_error')
  })
})
