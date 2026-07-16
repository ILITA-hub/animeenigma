import { describe, it, expect } from 'vitest'
import { classifyConnection, SLOW_SUSTAINED_MS, type ConnectionInputs } from './connectionHealth'

// A healthy, playing baseline; each test overrides just the fields it exercises.
const base: ConnectionInputs = {
  online: true,
  buffering: false,
  hasStarted: true,
  bytesFlowing: true,
  sustained: false,
  hasError: false,
}

describe('classifyConnection', () => {
  it('is ok during healthy playback', () => {
    expect(classifyConnection(base)).toBe('ok')
  })

  it('reports offline whenever navigator is offline — even mid-playback', () => {
    expect(classifyConnection({ ...base, online: false })).toBe('offline')
  })

  it('offline wins over a source error (the offline IS the root cause)', () => {
    expect(classifyConnection({ ...base, online: false, hasError: true })).toBe('offline')
  })

  it('reports slow when buffering is sustained with bytes still flowing', () => {
    expect(classifyConnection({ ...base, buffering: true, sustained: true })).toBe('slow')
  })

  it('is not slow for a brief (non-sustained) buffer hiccup', () => {
    expect(classifyConnection({ ...base, buffering: true, sustained: false })).toBe('ok')
  })

  it('is not slow before the first frame (initial load is not a connection badge)', () => {
    expect(classifyConnection({ ...base, hasStarted: false, buffering: true, sustained: true })).toBe('ok')
  })

  it('is not slow when no bytes are flowing — that is a dead source (failover owns it)', () => {
    expect(classifyConnection({ ...base, buffering: true, sustained: true, bytesFlowing: false })).toBe('ok')
  })

  it('suppresses slow while a source-error overlay is up', () => {
    expect(classifyConnection({ ...base, buffering: true, sustained: true, hasError: true })).toBe('ok')
  })

  it('exposes a sane grace threshold', () => {
    expect(SLOW_SUSTAINED_MS).toBeGreaterThanOrEqual(2000)
  })
})
