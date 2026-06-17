import { describe, it, expect } from 'vitest'
import { rankingToOrder } from './rankingOrder'
import type { SourceRanking } from '@/types/sourceRanking'

const rec = (provider: string, score: number) =>
  ({ provider, score, reached_rate: 0, ok_rate: 0, p95_ms: 0, stall_rate: 0, samples: 0 })

describe('rankingToOrder', () => {
  it('orders fix → perAnime → global, deduped, fix first', () => {
    const r: SourceRanking = {
      fix: 'kodik',
      perAnime: [rec('allanime', 0.9), rec('kodik', 0.5)],
      global: [rec('miruro', 0.8), rec('allanime', 0.7)],
    }
    expect(rankingToOrder(r)).toEqual(['kodik', 'allanime', 'miruro'])
  })

  it('handles empty ranking', () => {
    expect(rankingToOrder({ fix: '', perAnime: [], global: [] })).toEqual([])
  })

  it('skips empty fix', () => {
    const r: SourceRanking = { fix: '', perAnime: [rec('ae', 1)], global: [] }
    expect(rankingToOrder(r)).toEqual(['ae'])
  })

  it('tolerates null/undefined', () => {
    expect(rankingToOrder(null)).toEqual([])
    expect(rankingToOrder(undefined)).toEqual([])
  })
})
