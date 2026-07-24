/**
 * Gacha «Лудка» API — player + admin endpoints.
 *
 * All responses are wrapped in { success: boolean, data: T } by httputil.OK.
 * Callers unwrap via `response.data?.data ?? response.data`.
 *
 * Image paths: backend returns bare keys (e.g. "cards/<uuid>.webp").
 * Use cardImageUrl() to build the full URL.
 */
import { apiClient } from '@/api/client'

// ─── Shared types ──────────────────────────────────────────────────────────

export type Rarity = 'N' | 'R' | 'SR' | 'SSR'

/** Card — mirrors domain.Card (JSON tags). */
export interface GachaCard {
  id: string
  name: string
  source_title: string
  image_path: string
  /** Optional card-back image key; viewer falls back to the branded default. */
  back_path?: string
  rarity: Rarity
  enabled: boolean
  created_at: string
  updated_at: string
}

/** Group — mirrors domain.Group. */
export interface GachaGroup {
  id: string
  name: string
  created_at: string
  updated_at: string
}

/** Banner — mirrors domain.Banner. */
export interface GachaBanner {
  id: string
  name: string
  description: string
  /** Single banner background image key (slider/spin-page). */
  backdrop_path?: string
  is_standard: boolean
  enabled: boolean
  active_from?: string | null
  active_to?: string | null
  sort_order: number
  created_at: string
  updated_at: string
}

/** Wallet — mirrors domain.Wallet. */
export interface GachaWallet {
  user_id: string
  balance: number
  starter_granted: boolean
  daily_streak: number
  last_daily_at?: string | null
  created_at: string
  updated_at: string
}

// ─── Daily claim ───────────────────────────────────────────────────────────

/** dailyClaimResponse from handler/wallet.go */
export interface DailyClaimResponse {
  claimed: boolean
  amount?: number
  streak?: number
  wallet: GachaWallet
}

// ─── Pull ──────────────────────────────────────────────────────────────────

/** service.PulledCard */
export interface PulledCard {
  card: GachaCard
  new: boolean
  count: number
}

/** service.PullResult */
export interface PullResult {
  cards: PulledCard[]
  balance: number
  pity: number
}

// ─── Player views ──────────────────────────────────────────────────────────

/** service.BannerCardView */
export interface BannerCardView {
  id: string
  name: string
  rarity: Rarity
  image_path: string
  /** Optional card-back image key; viewer falls back to the branded default. */
  back_path?: string
  owned: boolean
}

/** service.BannerView */
export interface BannerView {
  id: string
  name: string
  description: string
  /** Single banner background image key (slider/spin-page). */
  backdrop_path?: string
  is_standard: boolean
  cards: BannerCardView[]
  my_pity: number
  pity_threshold: number
}

/** service.CollectionCardView */
export interface CollectionCardView {
  card: GachaCard
  owned: boolean
  count: number
  first_obtained_at?: string | null
}

/** service.RarityProgress */
export interface RarityProgress {
  owned: number
  total: number
}

/** service.CollectionView */
export interface CollectionView {
  cards: CollectionCardView[]
  progress: Record<Rarity, RarityProgress>
}

// ─── Admin request shapes ──────────────────────────────────────────────────

export interface CreateCardRequest {
  name: string
  source_title?: string
  image_path: string
  /** Optional card-back image key. */
  back_path?: string
  rarity: Rarity
  enabled: boolean
  group_ids?: string[]
}

export interface UpdateCardRequest {
  name: string
  source_title?: string
  image_path: string
  /** Optional card-back image key. */
  back_path?: string
  rarity: Rarity
  enabled: boolean
  group_ids?: string[]
}

export interface CreateBannerRequest {
  name: string
  description?: string
  /** Single banner background image key (slider/spin-page). */
  backdrop_path?: string
  is_standard?: boolean
  enabled?: boolean
  active_from?: string | null
  active_to?: string | null
  sort_order?: number
}

export interface UpdateBannerRequest {
  name?: string
  description?: string
  /** Single banner background image key (slider/spin-page). */
  backdrop_path?: string
  is_standard?: boolean
  enabled?: boolean
  active_from?: string | null
  active_to?: string | null
  sort_order?: number
}

export interface UploadImageResponse {
  image_path: string
  image_url: string
}

/** GetBanner admin response: { banner, card_ids } */
export interface AdminBannerDetail {
  banner: GachaBanner
  card_ids: string[]
}

// ─── Card filter params ────────────────────────────────────────────────────

export interface CardFilterParams {
  rarity?: Rarity
  group_id?: string
  enabled?: boolean
}

// ─── Bulk card operations ──────────────────────────────────────────────────

/** Partial field set for bulk card updates — absent keys are left unchanged. */
export interface BulkCardSet {
  name?: string
  source_title?: string
  rarity?: Rarity
  enabled?: boolean
  /** Card-back image key; empty string resets to the branded default. */
  back_path?: string
}

// ─── Image URL helper ──────────────────────────────────────────────────────

/** Build the full public URL for a card/banner image_path / art_path. */
export function cardImageUrl(path: string | undefined | null): string {
  if (!path) return ''
  // Backend may return either a bare key ("cards/uuid.webp") or the full path
  // already prefixed. Guard against double-prefixing.
  if (path.startsWith('/api/gacha/images/') || path.startsWith('http')) return path
  return `/api/gacha/images/${path}`
}

// ─── Player API ─────────────────────────────────────────────────────────────

export const gachaApi = {
  /** GET /api/gacha/wallet — get or create the caller's wallet. */
  getWallet: () =>
    apiClient.get<{ data: GachaWallet }>('/gacha/wallet'),

  /** POST /api/gacha/daily — claim daily reward. Idempotent per UTC day. */
  claimDaily: () =>
    apiClient.post<{ data: DailyClaimResponse }>('/gacha/daily'),

  /** GET /api/gacha/banners — active banners with pity + owned flags. */
  getBanners: () =>
    apiClient.get<{ data: BannerView[] }>('/gacha/banners'),

  /**
   * POST /api/gacha/banners/{id}/pull
   * Body: { mode: "x1" | "x10" }
   */
  pull: (bannerId: string, mode: 'x1' | 'x10') =>
    apiClient.post<{ data: PullResult }>(`/gacha/banners/${bannerId}/pull`, { mode }),

  /** GET /api/gacha/collection — full album + per-rarity progress. */
  getCollection: () =>
    apiClient.get<{ data: CollectionView }>('/gacha/collection'),
}

// ─── Admin API ─────────────────────────────────────────────────────────────

export const gachaAdminApi = {
  // ── Cards ──
  listCards: (params?: CardFilterParams) =>
    apiClient.get<{ data: GachaCard[] }>('/gacha/admin/cards', { params }),

  getCard: (id: string) =>
    apiClient.get<{ data: GachaCard }>(`/gacha/admin/cards/${id}`),

  createCard: (req: CreateCardRequest) =>
    apiClient.post<{ data: GachaCard }>('/gacha/admin/cards', req),

  updateCard: (id: string, req: UpdateCardRequest) =>
    apiClient.patch<{ data: GachaCard }>(`/gacha/admin/cards/${id}`, req),

  deleteCard: (id: string) =>
    apiClient.delete<{ data: { deleted: boolean } }>(`/gacha/admin/cards/${id}`),

  bulkUpdateCards: (ids: string[], set: BulkCardSet) =>
    apiClient.patch<{ data: { updated: number } }>('/gacha/admin/cards/bulk', { ids, set }),

  bulkDeleteCards: (ids: string[]) =>
    apiClient.post<{ data: { deleted: number } }>('/gacha/admin/cards/bulk-delete', { ids }),

  // ── Groups ──
  listGroups: () =>
    apiClient.get<{ data: GachaGroup[] }>('/gacha/admin/groups'),

  createGroup: (name: string) =>
    apiClient.post<{ data: GachaGroup }>('/gacha/admin/groups', { name }),

  renameGroup: (id: string, name: string) =>
    apiClient.patch<{ data: { updated: boolean } }>(`/gacha/admin/groups/${id}`, { name }),

  deleteGroup: (id: string) =>
    apiClient.delete<{ data: { deleted: boolean } }>(`/gacha/admin/groups/${id}`),

  addCardsToGroup: (groupId: string, cardIds: string[]) =>
    apiClient.post<{ data: { updated: boolean } }>(`/gacha/admin/groups/${groupId}/cards`, { card_ids: cardIds }),

  removeCardFromGroup: (groupId: string, cardId: string) =>
    apiClient.delete<{ data: { deleted: boolean } }>(`/gacha/admin/groups/${groupId}/cards/${cardId}`),

  // ── Banners ──
  listBanners: () =>
    apiClient.get<{ data: GachaBanner[] }>('/gacha/admin/banners'),

  getBanner: (id: string) =>
    apiClient.get<{ data: AdminBannerDetail }>(`/gacha/admin/banners/${id}`),

  createBanner: (req: CreateBannerRequest) =>
    apiClient.post<{ data: GachaBanner }>('/gacha/admin/banners', req),

  updateBanner: (id: string, req: UpdateBannerRequest) =>
    apiClient.patch<{ data: GachaBanner }>(`/gacha/admin/banners/${id}`, req),

  deleteBanner: (id: string) =>
    apiClient.delete<{ data: { deleted: boolean } }>(`/gacha/admin/banners/${id}`),

  setBannerCards: (bannerId: string, cardIds: string[]) =>
    apiClient.put<{ data: { updated: boolean } }>(`/gacha/admin/banners/${bannerId}/cards`, { card_ids: cardIds }),

  addBannerCards: (bannerId: string, cardIds: string[]) =>
    apiClient.post<{ data: { updated: boolean } }>(`/gacha/admin/banners/${bannerId}/cards`, { card_ids: cardIds }),

  addGroupCardsToBanner: (bannerId: string, groupId: string) =>
    apiClient.post<{ data: { updated: boolean } }>(`/gacha/admin/banners/${bannerId}/groups/${groupId}`),

  // ── Upload ──
  /** Upload via file (multipart) */
  uploadFile: (file: File, kind = 'cards') => {
    const form = new FormData()
    form.append('file', file)
    form.append('kind', kind)
    return apiClient.post<{ data: UploadImageResponse }>('/gacha/admin/upload', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  /** Upload via URL (JSON) */
  uploadUrl: (imageUrl: string, kind = 'cards') =>
    apiClient.post<{ data: UploadImageResponse }>('/gacha/admin/upload', { image_url: imageUrl, kind }),
}
