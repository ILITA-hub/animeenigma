import { describe, it, expect } from 'vitest'
import type { ProviderCap } from '@/types/capabilities'

describe('ProviderCap feed fields', () => {
  it('accepts the Phase-1 feed shape', () => {
    const p: ProviderCap = {
      provider: 'gogoanime', display_name: 'GogoAnime',
      state: 'active', selectable: true, hacker_only: false,
      order: 85, group: 'en', audios: ['sub', 'dub'],
      variants: [],
    } as ProviderCap
    expect(p.state).toBe('active')
    expect(p.selectable).toBe(true)
    expect(p.order).toBe(85)
    expect(p.group).toBe('en')
  })

  it('carries a reason on a degraded, hacker-only provider', () => {
    const p: ProviderCap = {
      provider: 'animefever', display_name: 'AnimeFever',
      state: 'degraded', selectable: true, hacker_only: true,
      order: 60, group: 'en', audios: ['sub'], variants: [], reason: 'ads',
    }
    expect(p.state).toBe('degraded')
    expect(p.hacker_only).toBe(true)
    expect(p.reason).toBe('ads')
  })
})
