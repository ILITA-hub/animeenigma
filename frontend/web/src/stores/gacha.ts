import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  gachaApi,
  type GachaWallet,
  type BannerView,
  type CollectionView,
  type PullResult,
  type DailyClaimResponse,
} from '@/api/gacha'

export const useGachaStore = defineStore('gacha', () => {
  // ── State ─────────────────────────────────────────────────────────────────
  const wallet = ref<GachaWallet | null>(null)
  const banners = ref<BannerView[]>([])
  const collection = ref<CollectionView | null>(null)

  const loadingWallet = ref(false)
  const loadingBanners = ref(false)
  const loadingCollection = ref(false)
  const loadingPull = ref(false)
  const loadingDaily = ref(false)

  const walletError = ref<string | null>(null)
  const bannersError = ref<string | null>(null)
  const collectionError = ref<string | null>(null)
  const pullError = ref<string | null>(null)

  // ── Computed ──────────────────────────────────────────────────────────────
  const balance = computed(() => wallet.value?.balance ?? 0)
  const dailyStreak = computed(() => wallet.value?.daily_streak ?? 0)

  // ── Helpers ───────────────────────────────────────────────────────────────
  function unwrap<T>(response: { data: unknown }): T {
    const data = (response as { data?: { data?: T; success?: boolean } }).data
    if (data && typeof data === 'object' && 'data' in data) {
      return data.data as T
    }
    return data as T
  }

  // ── Actions ───────────────────────────────────────────────────────────────

  /** Fetch or create the wallet. Auto-called on login + after pulls. */
  async function refreshWallet(): Promise<void> {
    loadingWallet.value = true
    walletError.value = null
    try {
      const res = await gachaApi.getWallet()
      wallet.value = unwrap<GachaWallet>(res)
    } catch (err: unknown) {
      walletError.value = extractMessage(err)
    } finally {
      loadingWallet.value = false
    }
  }

  /** POST /api/gacha/daily — claim the daily reward. */
  async function claimDaily(): Promise<DailyClaimResponse | null> {
    loadingDaily.value = true
    pullError.value = null
    try {
      const res = await gachaApi.claimDaily()
      const data = unwrap<DailyClaimResponse>(res)
      // Update wallet from the returned wallet object
      if (data.wallet) {
        wallet.value = data.wallet
      }
      return data
    } catch (err: unknown) {
      pullError.value = extractMessage(err)
      return null
    } finally {
      loadingDaily.value = false
    }
  }

  /** GET /api/gacha/banners */
  async function fetchBanners(): Promise<void> {
    loadingBanners.value = true
    bannersError.value = null
    try {
      const res = await gachaApi.getBanners()
      banners.value = unwrap<BannerView[]>(res) ?? []
    } catch (err: unknown) {
      bannersError.value = extractMessage(err)
    } finally {
      loadingBanners.value = false
    }
  }

  /** POST /api/gacha/banners/{id}/pull */
  async function pull(bannerId: string, mode: 'x1' | 'x10'): Promise<PullResult | null> {
    loadingPull.value = true
    pullError.value = null
    try {
      const res = await gachaApi.pull(bannerId, mode)
      const data = unwrap<PullResult>(res)
      // Update balance from pull result
      if (wallet.value && data.balance !== undefined) {
        wallet.value = { ...wallet.value, balance: data.balance }
      }
      // Refresh collection so owned marks update
      void fetchCollection()
      return data
    } catch (err: unknown) {
      pullError.value = extractMessage(err)
      return null
    } finally {
      loadingPull.value = false
    }
  }

  /** GET /api/gacha/collection */
  async function fetchCollection(): Promise<void> {
    loadingCollection.value = true
    collectionError.value = null
    try {
      const res = await gachaApi.getCollection()
      collection.value = unwrap<CollectionView>(res)
    } catch (err: unknown) {
      collectionError.value = extractMessage(err)
    } finally {
      loadingCollection.value = false
    }
  }

  return {
    // State
    wallet,
    banners,
    collection,
    loadingWallet,
    loadingBanners,
    loadingCollection,
    loadingPull,
    loadingDaily,
    walletError,
    bannersError,
    collectionError,
    pullError,
    // Computed
    balance,
    dailyStreak,
    // Actions
    refreshWallet,
    claimDaily,
    fetchBanners,
    pull,
    fetchCollection,
  }
})

// ── Error helper ──────────────────────────────────────────────────────────────
function extractMessage(err: unknown): string {
  const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
  return (
    e?.response?.data?.error?.message ??
    e?.response?.data?.message ??
    (err instanceof Error ? err.message : String(err))
  )
}
