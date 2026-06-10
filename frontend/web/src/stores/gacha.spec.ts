import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useGachaStore } from './gacha'
import type { GachaWallet, PullResult, DailyClaimResponse } from '@/api/gacha'

// ── Mock the api module ───────────────────────────────────────────────────────
// vi.mock is hoisted, so factories must use vi.fn() without referencing module-
// level vars that haven't been initialised yet.

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return {
    ...actual,
    gachaApi: {
      getWallet: vi.fn(),
      claimDaily: vi.fn(),
      getBanners: vi.fn(),
      pull: vi.fn(),
      getCollection: vi.fn(),
    },
  }
})

// ── Test fixtures ─────────────────────────────────────────────────────────────

function makeWallet(override?: Partial<GachaWallet>): GachaWallet {
  return {
    user_id: 'user-1',
    balance: 500,
    starter_granted: true,
    daily_streak: 3,
    last_daily_at: '2026-06-09T10:00:00Z',
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-09T10:00:00Z',
    ...override,
  }
}

function makePullResult(): PullResult {
  return {
    cards: [
      {
        card: {
          id: 'card-1',
          name: 'Test Card',
          source_title: 'Test Anime',
          image_path: 'cards/test.webp',
          rarity: 'SR',
          enabled: true,
          created_at: '2026-06-01T00:00:00Z',
          updated_at: '2026-06-01T00:00:00Z',
        },
        new: true,
        count: 1,
      },
    ],
    balance: 400,
    pity: 1,
  }
}

// Build a standard httputil envelope { data: { success, data } }
function envelope<T>(data: T) {
  return { data: { success: true, data } }
}

describe('gacha store', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // Set up default mock return values after clearing
    const { gachaApi } = await import('@/api/gacha')
    vi.mocked(gachaApi.getWallet).mockResolvedValue(envelope(makeWallet()))
    vi.mocked(gachaApi.claimDaily).mockResolvedValue(
      envelope({
        claimed: true,
        amount: 50,
        streak: 4,
        wallet: makeWallet({ balance: 550, daily_streak: 4 }),
      } as DailyClaimResponse),
    )
    vi.mocked(gachaApi.getBanners).mockResolvedValue(envelope([]))
    vi.mocked(gachaApi.pull).mockResolvedValue(envelope(makePullResult()))
    vi.mocked(gachaApi.getCollection).mockResolvedValue(
      envelope({
        cards: [],
        progress: { N: { owned: 0, total: 0 }, R: { owned: 0, total: 0 }, SR: { owned: 0, total: 0 }, SSR: { owned: 0, total: 0 } },
      }),
    )
  })

  it('balance computed returns 0 when wallet is null', () => {
    const store = useGachaStore()
    expect(store.wallet).toBeNull()
    expect(store.balance).toBe(0)
  })

  it('refreshWallet populates wallet state and balance', async () => {
    const store = useGachaStore()
    await store.refreshWallet()
    expect(store.wallet).not.toBeNull()
    expect(store.balance).toBe(500)
    expect(store.dailyStreak).toBe(3)
  })

  it('pull updates balance from PullResult', async () => {
    const store = useGachaStore()
    await store.refreshWallet()
    expect(store.balance).toBe(500)

    const result = await store.pull('banner-1', 'x1')
    expect(result).not.toBeNull()
    expect(result!.cards).toHaveLength(1)
    expect(result!.cards[0].new).toBe(true)
    // Balance updated from pull result (400)
    expect(store.balance).toBe(400)
  })

  it('pull returns null and sets pullError on API failure', async () => {
    const { gachaApi } = await import('@/api/gacha')
    vi.mocked(gachaApi.pull).mockRejectedValueOnce(new Error('Insufficient funds'))

    const store = useGachaStore()
    const result = await store.pull('banner-1', 'x1')
    expect(result).toBeNull()
    expect(store.pullError).toContain('Insufficient funds')
  })

  it('claimDaily updates wallet balance and streak from daily response', async () => {
    const store = useGachaStore()
    await store.refreshWallet()
    expect(store.balance).toBe(500)

    const res = await store.claimDaily()
    expect(res).not.toBeNull()
    expect(res!.claimed).toBe(true)
    expect(res!.streak).toBe(4)
    expect(store.wallet?.balance).toBe(550)
    expect(store.wallet?.daily_streak).toBe(4)
  })

  it('claimDaily with already-claimed (claimed=false) still resolves without error', async () => {
    const { gachaApi } = await import('@/api/gacha')
    vi.mocked(gachaApi.claimDaily).mockResolvedValueOnce(
      envelope({ claimed: false, wallet: makeWallet() } as DailyClaimResponse),
    )

    const store = useGachaStore()
    const res = await store.claimDaily()
    expect(res).not.toBeNull()
    expect(res!.claimed).toBe(false)
    expect(store.pullError).toBeNull()
  })
})
