/**
 * useProviderResolver — Task 6 of the Unified Player plan.
 *
 * Provides a factory `makeResolver(deps)` that returns a unified adapter
 * interface. Each adapter normalises its native episode/stream model into
 * the stable `EpisodeOption` / `StreamResult` shapes so the unified player
 * shell never touches raw provider payloads directly.
 *
 * Wired adapters
 * ──────────────
 * • scraperAdapter  — covers all SCRAPER_IDS (EN scraper chain, NOT 18anime)
 *   using scraperApi.getEpisodes / getServers / getStream (resp.data.data envelope).
 * • rawAdapter      — covers 'raw' (AllAnime JP) using rawApi.getEpisodes /
 *   getStream (resp.data?.data ?? resp.data envelope).
 * • anime18Adapter  — covers '18anime' via the SEPARATE anime18Api backend
 *   (/anime18/* routes), NOT the scraper chain. Uses anime18Api.getEpisodes /
 *   getStream (resp.data?.data || resp.data envelope).
 * • kodikAdapter    — covers 'kodik' (RU ad-free HLS path) using kodikApi.getTranslations /
 *   getStream; stream URLs are wrapped through the HLS proxy for CORS (resp.data?.data ?? resp.data).
 *
 * NOT wired (throw NotAvailableError)
 * ─────────────────────────────────────
 * • 'animelib'  — upstream went Kodik-only (see MEMORY.md); hidden by default.
 * • 'hanime'    — hanimeApi.getStream(animeId, slug) needs the episode slug, not
 *                 a number, and the resolver contract uses a number-keyed EpisodeOption;
 *                 kept as NotAvailableError until a slug-keyed adapter is wired.
 * • 'ae'        — first-party / admin-only path, not yet exposed to this layer.
 */

import { scraperApi, rawApi, anime18Api, kodikApi } from '@/api/client'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult, Combo } from '@/types/unifiedPlayer'

// ─── Error ──────────────────────────────────────────────────────────────────

export class NotAvailableError extends Error {
  constructor(provider: string, reason = 'not available') {
    super(`Provider "${provider}" is ${reason}`)
    this.name = 'NotAvailableError'
  }
}

// ─── Scraper raw types (mirrored from OurEnglishPlayer; not re-exported) ────

interface ScraperEpisode {
  id: string
  number: number
  title?: string
  is_filler?: boolean
}

interface ScraperServer {
  id: string
  name: string
  type?: string // "sub" | "dub" | "raw"
}

interface ScraperSource {
  url: string
  type: string // "hls" | "mp4"
  quality?: string
}

interface ScraperEnvelope {
  episodes?: ScraperEpisode[]
  servers?: ScraperServer[]
  stream?: {
    sources: ScraperSource[]
    tracks?: unknown[]
    headers?: Record<string, string>
  }
  meta?: { tried?: string[]; provider?: string }
}

// ─── Raw-JP types (from types/raw.ts) ───────────────────────────────────────

interface RawEpisodesResponse {
  episodes: { id: string; number: number; title: string }[]
  available: boolean
  source: string
}

interface RawStream {
  url: string
  type: 'hls' | 'mp4'
  quality?: string
}

// ─── Anime18 types (mirrored from Anime18Player) ────────────────────────────

interface Anime18Episode {
  slug: string
  url: string
  number: number
}

interface Anime18Source {
  url: string
  referer?: string
  is_hls: boolean
  quality: string
}

// ─── Kodik types ─────────────────────────────────────────────────────────────

interface KodikTranslation {
  id: number
  title: string
  type: string // 'voice' = dub, otherwise sub
  episodes_count: number
}

interface KodikStream {
  stream_url: string
  referer: string
}

// ─── ProviderAdapter interface ───────────────────────────────────────────────

export interface ProviderAdapter {
  /**
   * Returns the normalised episode list for `animeId`.
   * Throws `NotAvailableError` when the provider is unavailable/unwired.
   */
  listEpisodes(animeId: string): Promise<EpisodeOption[]>

  /**
   * Resolves and returns a `StreamResult` for the given episode + combo.
   * `ep.key` carries the provider-native episode identifier (e.g. scraper's
   * opaque `id`, raw's `id`, anime18's `slug`).
   */
  resolveStream(animeId: string, ep: EpisodeOption, combo: Combo): Promise<StreamResult>
}

// ─── Deps injected by makeResolver / useProviderResolver ────────────────────

export interface ResolverDeps {
  scraperApi?: typeof scraperApi
  rawApi?: typeof rawApi
  anime18Api?: typeof anime18Api
  kodikApi?: typeof kodikApi
}

// ─── Set of provider IDs that route through the scraper microservice ─────────

export const SCRAPER_IDS = new Set<string>([
  'allanime',
  'animefever',
  'gogoanime',
  'miruro',
  'nineanime',
  'animepahe',
  // NOTE: '18anime' is NOT in this set — it routes to the anime18Api backend
  // (/anime18/* routes), which is a separate orchestrator from the EN scraper chain.
])

// ─── Adapters ────────────────────────────────────────────────────────────────

/**
 * Closes over an optional `prefer` provider id so that `listEpisodes` can
 * forward it directly to the scraper API without leaking it through the
 * `ProviderAdapter` interface (which only receives `animeId`).
 */
function makeScraperAdapter(api: typeof scraperApi, prefer?: string): ProviderAdapter {
  return {
    async listEpisodes(animeId: string): Promise<EpisodeOption[]> {
      const resp = await api.getEpisodes(animeId, prefer)
      const env = resp.data?.data as ScraperEnvelope | undefined
      const eps: ScraperEpisode[] = env?.episodes ?? []
      return eps.map((ep) => ({
        key: ep.id,
        label: ep.number,
        number: ep.number,
        ...(ep.is_filler ? { isFiller: true } : {}),
      }))
    },

    async resolveStream(animeId: string, ep: EpisodeOption, combo: Combo): Promise<StreamResult> {
      const resolvedPrefer = combo.provider || prefer
      const episodeId = String(ep.key)

      // 1. Fetch servers for this episode
      const sResp = await api.getServers(animeId, episodeId, resolvedPrefer)
      const sEnv = sResp.data?.data as ScraperEnvelope | undefined
      const srvs: ScraperServer[] = sEnv?.servers ?? []
      if (srvs.length === 0) {
        throw new NotAvailableError(combo.provider || prefer || 'scraper', 'has no servers for this episode')
      }

      // Pick the server matching combo.server, or fall back to the first sub/raw
      const serverId = combo.server && srvs.find((s) => s.id === combo.server)
        ? combo.server
        : (srvs.find((s) => s.type === 'sub') ?? srvs[0]).id

      const selectedSrv = srvs.find((s) => s.id === serverId) ?? srvs[0]
      const category: 'sub' | 'dub' = selectedSrv.type === 'dub' ? 'dub' : 'sub'

      // 2. Fetch stream
      const stResp = await api.getStream(animeId, episodeId, serverId, category, resolvedPrefer)
      const stEnv = stResp.data?.data as ScraperEnvelope | undefined
      const stream = stEnv?.stream
      if (!stream?.sources?.length) {
        throw new NotAvailableError(combo.provider || prefer || 'scraper', 'returned no stream sources')
      }

      const source = stream.sources[0]
      const type: 'hls' | 'mp4' = source.type === 'mp4' ? 'mp4' : 'hls'
      const referer = stream.headers?.Referer || stream.headers?.referer || ''
      return {
        url: buildProxyUrl(source.url, referer, type),
        type,
        headers: stream.headers,
        servers: srvs.map((s) => ({ id: s.id, label: s.name })),
      }
    },
  }
}

function makeRawAdapter(api: typeof rawApi): ProviderAdapter {
  return {
    async listEpisodes(animeId: string): Promise<EpisodeOption[]> {
      const resp = await api.getEpisodes(animeId)
      const data: RawEpisodesResponse = resp.data?.data ?? resp.data
      return (data?.episodes ?? []).map((ep) => ({
        key: ep.id,
        label: ep.number,
        number: ep.number,
      }))
    },

    async resolveStream(animeId: string, ep: EpisodeOption): Promise<StreamResult> {
      // rawApi.getStream takes the episode NUMBER, not the id string
      const resp = await api.getStream(animeId, ep.number)
      const stream: RawStream = resp.data?.data ?? resp.data
      if (!stream?.url) {
        throw new NotAvailableError('raw', 'returned no stream URL')
      }
      const type: 'hls' | 'mp4' = stream.type ?? 'hls'
      // AllAnime's fast4speed.rsvp CDN requires Referer: https://allmanga.to/
      // (mirrors the legacy RawPlayer). The proxy injects it.
      return {
        url: buildProxyUrl(stream.url, 'https://allmanga.to/', type),
        type,
      }
    },
  }
}

function makeAnime18Adapter(api: typeof anime18Api): ProviderAdapter {
  return {
    async listEpisodes(animeId: string): Promise<EpisodeOption[]> {
      const response = await api.getEpisodes(animeId)
      const data: Anime18Episode[] = response.data?.data || response.data || []
      return (Array.isArray(data) ? data : []).map((ep) => ({
        key: ep.slug,     // slug is the native identifier needed by getStream
        label: ep.number,
        number: ep.number,
      }))
    },

    async resolveStream(animeId: string, ep: EpisodeOption): Promise<StreamResult> {
      const slug = String(ep.key)
      const response = await api.getStream(animeId, slug)
      const data: Anime18Source | undefined = response.data?.data || response.data
      if (!data?.url) {
        throw new NotAvailableError('18anime', 'returned no stream URL')
      }
      const type: 'hls' | 'mp4' = data.is_hls ? 'hls' : 'mp4'
      // mp4upload (and other 18anime CDNs) require the source-carried Referer.
      return {
        url: buildProxyUrl(data.url, data.referer ?? '', type),
        type,
      }
    },
  }
}

// ─── Kodik proxy helper ───────────────────────────────────────────────────────

/**
 * Wrap an upstream CDN url through the backend HLS proxy so the proxy can
 * inject the required `Referer` header and handle CORS / range requests.
 *
 * EVERY external stream must go through this — handing a raw CDN url straight
 * to `<video>`/hls.js makes the browser send no Referer, and refer-gated CDNs
 * (allmanga.to's fast4speed.rsvp, mp4upload, kwik, …) then 403 or hang at 0:00.
 *
 * `streamType === 'mp4'` adds the `type=mp4` marker the proxy uses to pick its
 * progressive-MP4 (range-passthrough) code path instead of m3u8 rewriting.
 */
function buildProxyUrl(url: string, referer: string, streamType?: 'hls' | 'mp4'): string {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  if (streamType === 'mp4') params.set('type', 'mp4')
  return `/api/streaming/hls-proxy?${params.toString()}`
}

function makeKodikAdapter(api: typeof kodikApi): ProviderAdapter {
  return {
    async listEpisodes(animeId: string): Promise<EpisodeOption[]> {
      const resp = await api.getTranslations(animeId)
      const translations: KodikTranslation[] = resp.data?.data ?? resp.data ?? []
      if (!Array.isArray(translations) || translations.length === 0) {
        return []
      }
      // Use the MAX across all translations so no episode is silently hidden
      // when teams cover different ranges (e.g. translation A has 12 eps,
      // translation B has 24 eps — using only [0] would hide eps 13-24).
      const maxCount = Math.max(0, ...translations.map((t) => t.episodes_count ?? 0))
      return Array.from({ length: maxCount }, (_, i) => {
        const n = i + 1
        return { key: n, label: n, number: n }
      })
    },

    async resolveStream(animeId: string, ep: EpisodeOption, combo: Combo): Promise<StreamResult> {
      const resp = await api.getTranslations(animeId)
      const translations: KodikTranslation[] = resp.data?.data ?? resp.data ?? []
      if (!Array.isArray(translations) || translations.length === 0) {
        throw new NotAvailableError('kodik', 'has no translations')
      }
      // Pick by team name, fall back to first
      const tr = (combo.team ? translations.find((t) => t.title === combo.team) : undefined)
        ?? translations[0]

      const stResp = await api.getStream(animeId, ep.number, tr.id)
      const stream: KodikStream = stResp.data?.data ?? stResp.data
      if (!stream?.stream_url) {
        throw new NotAvailableError('kodik', 'returned no stream URL')
      }
      return {
        url: buildProxyUrl(stream.stream_url, stream.referer),
        type: 'hls',
      }
    },
  }
}

// ─── Resolver (factory + composable) ─────────────────────────────────────────

export interface ProviderResolver {
  listEpisodes(provider: string, animeId: string): Promise<EpisodeOption[]>
  resolveStream(provider: string, animeId: string, ep: EpisodeOption, combo: Combo): Promise<StreamResult>
}

/**
 * `makeResolver(deps)` — injectable factory for testing.
 *
 * Dispatching rules:
 * - provider === 'kodik'     → kodikAdapter (requires deps.kodikApi)
 * - provider in SCRAPER_IDS → scraperAdapter (requires deps.scraperApi)
 * - provider === 'raw'       → rawAdapter (requires deps.rawApi)
 * - provider === '18anime'   → anime18Adapter via anime18Api (/anime18/* backend,
 *                              NOT the EN scraper chain; requires deps.anime18Api)
 * - anything else            → NotAvailableError
 */
export function makeResolver(deps: ResolverDeps): ProviderResolver {
  const UNAVAILABLE_PROVIDERS = new Set<string>([
    'animelib', // upstream went Kodik-only
    'hanime',   // needs slug-based episode key; deferred
    'ae',       // first-party admin path; not exposed here
  ])

  function getAdapter(provider: string): ProviderAdapter {
    if (UNAVAILABLE_PROVIDERS.has(provider)) {
      throw new NotAvailableError(provider)
    }

    if (provider === 'kodik') {
      if (!deps.kodikApi) {
        throw new NotAvailableError(provider, 'not available (kodikApi dep missing)')
      }
      return makeKodikAdapter(deps.kodikApi)
    }

    if (SCRAPER_IDS.has(provider)) {
      if (!deps.scraperApi) {
        throw new NotAvailableError(provider, 'not available (scraperApi dep missing)')
      }
      return makeScraperAdapter(deps.scraperApi, provider)
    }

    if (provider === 'raw') {
      if (!deps.rawApi) {
        throw new NotAvailableError(provider, 'not available (rawApi dep missing)')
      }
      return makeRawAdapter(deps.rawApi)
    }

    if (provider === '18anime') {
      if (!deps.anime18Api) {
        throw new NotAvailableError(provider, 'not available (anime18Api dep missing)')
      }
      return makeAnime18Adapter(deps.anime18Api)
    }

    throw new NotAvailableError(provider)
  }

  return {
    async listEpisodes(provider: string, animeId: string): Promise<EpisodeOption[]> {
      // getAdapter may throw synchronously; async wrapper converts that into a
      // rejected promise so callers can use `.catch()` / `await … catch` uniformly.
      return getAdapter(provider).listEpisodes(animeId)
    },
    async resolveStream(
      provider: string,
      animeId: string,
      ep: EpisodeOption,
      combo: Combo,
    ): Promise<StreamResult> {
      return getAdapter(provider).resolveStream(animeId, ep, combo)
    },
  }
}

/**
 * `useProviderResolver()` — composable that wires the real clients.
 * Call this inside a Vue setup context.
 */
export function useProviderResolver(): ProviderResolver {
  return makeResolver({ scraperApi, rawApi, anime18Api, kodikApi })
}
