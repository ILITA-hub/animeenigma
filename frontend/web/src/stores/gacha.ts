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

// ── Wallet persistence (page-fetch optimization 2026-06-11) ─────────────────
// The navbar balance chip used to fire GET /api/gacha/wallet on every hard
// page load. The wallet is now persisted to localStorage and rendered
// instantly; the network refresh only fires when the cached copy is stale
// (WALLET_STALE_MS), on tab-return, or shortly after an action that earns
// crystals (see GACHA_CREDIT_EVENT). Mutations (pull, daily claim) keep
// updating the balance from their own responses — the primary "server
// message" channel.

const WALLET_STORAGE_KEY = 'gacha:wallet'
const WALLET_STALE_MS = 5 * 60 * 1000

/**
 * Window event the API layer dispatches after an action that triggers an
 * async server-side credit (mark-episode-watched → player service credits
 * «Энигмы» fire-and-forget). The store reacts with a delayed forced refresh
 * so the chip reflects the earned crystals without polling.
 */
export const GACHA_CREDIT_EVENT = 'gacha:maybe-credited'
/** Credit producer is async (queue + 3s HTTP timeout) — give it a beat. */
const CREDIT_REFRESH_DELAY_MS = 2_500

function loadPersistedWallet(): { wallet: GachaWallet; ts: number } | null {
  if (typeof localStorage === 'undefined') return null
  try {
    const raw = localStorage.getItem(WALLET_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as { wallet?: GachaWallet; ts?: number }
    if (!parsed || typeof parsed !== 'object' || !parsed.wallet) return null
    return { wallet: parsed.wallet, ts: typeof parsed.ts === 'number' ? parsed.ts : 0 }
  } catch {
    return null
  }
}

function persistWallet(wallet: GachaWallet | null, ts: number): void {
  if (typeof localStorage === 'undefined') return
  try {
    if (!wallet) localStorage.removeItem(WALLET_STORAGE_KEY)
    else localStorage.setItem(WALLET_STORAGE_KEY, JSON.stringify({ wallet, ts }))
  } catch {
    /* storage full / disabled — degrade to per-load fetches */
  }
}

export const useGachaStore = defineStore('gacha', () => {
  // ── State ─────────────────────────────────────────────────────────────────
  const persisted = loadPersistedWallet()
  const wallet = ref<GachaWallet | null>(persisted?.wallet ?? null)
  const walletFetchedAt = ref<number>(persisted?.ts ?? 0)
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

  /** Central wallet setter: updates state + the localStorage mirror. */
  function setWallet(next: GachaWallet | null): void {
    wallet.value = next
    walletFetchedAt.value = next ? Date.now() : 0
    persistWallet(next, walletFetchedAt.value)
  }

  /**
   * Fetch or create the wallet. `ifStale: true` (navbar chip path) skips the
   * network while the persisted copy is younger than WALLET_STALE_MS — the
   * cached balance renders instantly and mutations keep it current.
   */
  async function refreshWallet(opts?: { ifStale?: boolean }): Promise<void> {
    if (opts?.ifStale && wallet.value && Date.now() - walletFetchedAt.value < WALLET_STALE_MS) {
      return
    }
    loadingWallet.value = true
    walletError.value = null
    try {
      const res = await gachaApi.getWallet()
      setWallet(unwrap<GachaWallet>(res))
    } catch (err: unknown) {
      walletError.value = extractMessage(err)
    } finally {
      loadingWallet.value = false
    }
  }

  // ── Background balance freshness (navbar chip lifecycle) ──────────────────
  let watchAttached = false
  let creditTimer: ReturnType<typeof setTimeout> | null = null

  function onVisibilityChange(): void {
    if (typeof document === 'undefined' || document.visibilityState === 'hidden') return
    void refreshWallet({ ifStale: true })
  }

  function onMaybeCredited(): void {
    // Debounce bursts (binge-marking episodes) into one refresh.
    if (creditTimer) clearTimeout(creditTimer)
    creditTimer = setTimeout(() => {
      creditTimer = null
      void refreshWallet()
    }, CREDIT_REFRESH_DELAY_MS)
  }

  /**
   * Idempotent: attach the tab-return + credit-event listeners that keep the
   * navbar balance chip fresh without per-page fetches. Called by Navbar when
   * the gacha surface is visible for an authenticated user.
   */
  function startBalanceWatch(): void {
    if (watchAttached || typeof window === 'undefined') return
    watchAttached = true
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener(GACHA_CREDIT_EVENT, onMaybeCredited)
  }

  /** Detach listeners + drop the persisted wallet (logout path). */
  function stopBalanceWatch(): void {
    if (creditTimer) {
      clearTimeout(creditTimer)
      creditTimer = null
    }
    if (!watchAttached || typeof window === 'undefined') return
    watchAttached = false
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener(GACHA_CREDIT_EVENT, onMaybeCredited)
  }

  /** Clear wallet state + persistence (logout). */
  function clearWallet(): void {
    setWallet(null)
    stopBalanceWatch()
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
        setWallet(data.wallet)
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
        setWallet({ ...wallet.value, balance: data.balance })
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
    startBalanceWatch,
    stopBalanceWatch,
    clearWallet,
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
