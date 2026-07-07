/**
 * Vitest spec for useAdminProviders() — RBAC-and-roulette P5 Task 2. Mirrors
 * useAdminPolicy.spec.ts: stubs `@/api/client`'s adminApi and verifies the
 * fetch / unwrap / mutation contract against the Task-1 catalog endpoints:
 *   GET /api/admin/scraper-providers            -> {success,data:{providers}}
 *   PUT /api/admin/scraper-providers/{name}/policy -> {success,data:<wire>}
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({
  adminApi: {
    listScraperProviders: vi.fn(),
    setScraperProviderPolicy: vi.fn(),
  },
}))

import { adminApi } from '@/api/client'
import { useAdminProviders, type ScraperProviderWire } from './useAdminProviders'

const listSpy = adminApi.listScraperProviders as ReturnType<typeof vi.fn>
const setPolicySpy = adminApi.setScraperProviderPolicy as ReturnType<typeof vi.fn>

const gogoanime: ScraperProviderWire = {
  name: 'gogoanime',
  status: 'enabled',
  policy: 'auto',
  health: 'up',
  health_since: '2026-07-01T00:00:00Z',
  policy_since: '2026-07-01T00:00:00Z',
  last_probed_at: '2026-07-07T00:00:00Z',
  group: 'en',
  reason: '',
  description: 'GogoAnime',
  scraper_operated: true,
  supports_sub: true,
  supports_dub: true,
  supports_raw: false,
  sub_delivery: 'embedded',
  quality_ceiling: '1080p',
  preference_weight: 10,
  engine: 'browser',
  base_url: 'https://gogoanime.example',
  last_tick_metrics: '',
  updated_at: '2026-07-07T00:00:00Z',
  derived_state: 'UP',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useAdminProviders', () => {
  it('list() GETs the scraper-providers path and returns the parsed providers array', async () => {
    listSpy.mockResolvedValue({
      data: { success: true, data: { providers: [gogoanime] } },
    })

    const { list } = useAdminProviders()
    const result = await list()

    expect(listSpy).toHaveBeenCalledTimes(1)
    expect(result).toEqual([gogoanime])
  })

  it('list() also unwraps a bare (non-enveloped) response', async () => {
    listSpy.mockResolvedValue({
      data: { providers: [gogoanime] },
    })

    const { list } = useAdminProviders()
    const result = await list()

    expect(result).toEqual([gogoanime])
  })

  it("setPolicy('gogoanime','disabled') PUTs the policy path with {policy:'disabled'} and returns the updated wire", async () => {
    const disabled: ScraperProviderWire = { ...gogoanime, policy: 'disabled', derived_state: 'Disabled' }
    setPolicySpy.mockResolvedValue({ data: { success: true, data: disabled } })

    const { setPolicy } = useAdminProviders()
    const result = await setPolicy('gogoanime', 'disabled')

    expect(setPolicySpy).toHaveBeenCalledTimes(1)
    expect(setPolicySpy).toHaveBeenCalledWith('gogoanime', 'disabled')
    expect(result).toEqual(disabled)
  })

  it("setPolicy('gogoanime','auto') calls through with 'auto' and unwraps a bare response", async () => {
    const auto: ScraperProviderWire = { ...gogoanime, policy: 'auto', derived_state: 'UP' }
    setPolicySpy.mockResolvedValue({ data: auto })

    const { setPolicy } = useAdminProviders()
    const result = await setPolicy('gogoanime', 'auto')

    expect(setPolicySpy).toHaveBeenCalledWith('gogoanime', 'auto')
    expect(result).toEqual(auto)
  })
})
