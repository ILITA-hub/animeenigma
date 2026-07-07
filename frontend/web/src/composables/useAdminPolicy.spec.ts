/**
 * Vitest spec for useAdminPolicy() — the RBAC-and-roulette policy admin
 * composable (Task 6). Stubs `@/api/client`'s adminApi and verifies the
 * fetch / unwrap / mutation contract against the policy-service backend
 * (services/policy, proxied at /api/admin/policy/*):
 *   GET /api/admin/policy/flags        -> {success,data:{flags,rouletteEnabled}}
 *   PUT /api/admin/policy/flags/{key}  -> {success,data:{key}}
 *   PUT /api/admin/policy/roulette     -> {success,data:{enabled}}
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({
  adminApi: {
    getPolicyFlags: vi.fn(),
    setPolicyFlag: vi.fn(),
    setPolicyRoulette: vi.fn(),
  },
}))

import { adminApi } from '@/api/client'
import { useAdminPolicy, type FeatureFlag, type FeatureFlagPayload } from './useAdminPolicy'

const getFlagsSpy = adminApi.getPolicyFlags as ReturnType<typeof vi.fn>
const setFlagSpy = adminApi.setPolicyFlag as ReturnType<typeof vi.fn>
const setRouletteSpy = adminApi.setPolicyRoulette as ReturnType<typeof vi.fn>

const sampleFlag: FeatureFlag = {
  key: 'fanfic',
  roles: ['admin'],
  allowUsers: [],
  denyUsers: [],
  roulette: false,
  failSafe: 'admin',
  label: 'Fanfic Engine',
  updatedAt: '2026-07-06T12:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useAdminPolicy', () => {
  it('list() GETs the flags path and returns the parsed {flags,rouletteEnabled} envelope', async () => {
    getFlagsSpy.mockResolvedValue({
      data: { success: true, data: { flags: [sampleFlag], rouletteEnabled: true } },
    })

    const { list } = useAdminPolicy()
    const result = await list()

    expect(getFlagsSpy).toHaveBeenCalledTimes(1)
    expect(result).toEqual({ flags: [sampleFlag], rouletteEnabled: true })
  })

  it('list() also unwraps a bare (non-enveloped) response', async () => {
    getFlagsSpy.mockResolvedValue({
      data: { flags: [sampleFlag], rouletteEnabled: false },
    })

    const { list } = useAdminPolicy()
    const result = await list()

    expect(result).toEqual({ flags: [sampleFlag], rouletteEnabled: false })
  })

  it("setFlag('fanfic', payload) PUTs /admin/policy/flags/fanfic with the body", async () => {
    setFlagSpy.mockResolvedValue({ data: { success: true, data: { key: 'fanfic' } } })

    const payload: FeatureFlagPayload = {
      roles: ['admin'],
      allowUsers: ['user-uuid-1'],
      denyUsers: [],
      roulette: true,
      failSafe: 'everyone',
      label: 'Fanfic Engine',
    }

    const { setFlag } = useAdminPolicy()
    await setFlag('fanfic', payload)

    expect(setFlagSpy).toHaveBeenCalledTimes(1)
    expect(setFlagSpy).toHaveBeenCalledWith('fanfic', payload)
  })

  it('setRoulette(true) PUTs the roulette path with {enabled:true}', async () => {
    setRouletteSpy.mockResolvedValue({ data: { success: true, data: { enabled: true } } })

    const { setRoulette } = useAdminPolicy()
    await setRoulette(true)

    expect(setRouletteSpy).toHaveBeenCalledTimes(1)
    expect(setRouletteSpy).toHaveBeenCalledWith(true)
  })
})
