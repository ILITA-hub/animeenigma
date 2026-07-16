import { describe, it, expect } from 'vitest'
import { ratingLabel, sourceLabelKey } from './animeMeta'

describe('ratingLabel', () => {
  it('maps Shikimori rating codes to display badges', () => {
    expect(ratingLabel('r_plus')).toBe('R+')
    expect(ratingLabel('pg_13')).toBe('PG-13')
    expect(ratingLabel('rx')).toBe('Rx')
  })
  it('returns empty string for unknown/empty', () => {
    expect(ratingLabel(undefined)).toBe('')
    expect(ratingLabel('none')).toBe('')
  })
})

describe('sourceLabelKey', () => {
  it('maps known sources to an i18n key suffix', () => {
    expect(sourceLabelKey('manga')).toBe('manga')
    expect(sourceLabelKey('light_novel')).toBe('light_novel')
  })
  it('returns null for empty', () => {
    expect(sourceLabelKey(undefined)).toBeNull()
    expect(sourceLabelKey('')).toBeNull()
  })
})
