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
 * • scraperAdapter  — covers all SCRAPER_IDS (EN + 18anime scraper path)
 *   using scraperApi.getEpisodes / getServers / getStream (resp.data.data envelope).
 * • rawAdapter      — covers 'raw' (AllAnime JP) using rawApi.getEpisodes /
 *   getStream (resp.data?.data ?? resp.data envelope).
 * • anime18Adapter  — covers '18anime' NON-scraper path using anime18Api.getEpisodes /
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
  '18anime', // the 18anime SCRAPER path (separate from the anime18Api path below)
])

// ─── Adapters ────────────────────────────────────────────────────────────────

function makeScraperAdapter(api: typeof scraperApi): ProviderAdapter {
  return {
    async listEpisodes(animeId: string, prefer?: string): Promise<EpisodeOption[]> {
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
      const prefer = combo.provider || undefined
      const episodeId = String(ep.key)

      // 1. Fetch servers for this episode
      const sResp = await api.getServers(animeId, episodeId, prefer)
      const sEnv = sResp.data?.data as ScraperEnvelope | undefined
      const srvs: ScraperServer[] = sEnv?.servers ?? []
      if (srvs.length === 0) {
        throw new NotAvailableError(combo.provider, 'has no servers for this episode')
      }

      // Pick the server matching combo.server, or fall back to the first sub/raw
      const serverId = combo.server && srvs.find((s) => s.id === combo.server)
        ? combo.server
        : (srvs.find((s) => s.type === 'sub') ?? srvs[0]).id

      const selectedSrv = srvs.find((s) => s.id === serverId) ?? srvs[0]
      const category: 'sub' | 'dub' = selectedSrv.type === 'dub' ? 'dub' : 'sub'

      // 2. Fetch stream
      const stResp = await api.getStream(animeId, episodeId, serverId, category, prefer)
      const stEnv = stResp.data?.data as ScraperEnvelope | undefined
      const stream = stEnv?.stream
      if (!stream?.sources?.length) {
        throw new NotAvailableError(combo.provider, 'returned no stream sources')
      }

      const source = stream.sources[0]
      return {
        url: source.url,
        type: source.type === 'mp4' ? 'mp4' : 'hls',
        headers: stream.headers,
        servers: srvs.map((s) => ({ id: s.id, label: s.name })),
      }
    },
  }
}

/**
 * Curried overload used internally — allows passing `prefer` through
 * `listEpisodes` for the scraper adapter without changing the ProviderAdapter
 * interface (the interface only has `animeId`).
 */
function makeScraperAdapterWithPrefer(
  api: typeof scraperApi,
  prefer: string | undefined,
): ProviderAdapter {
  const base = makeScraperAdapter(api)
  return {
    listEpisodes: (animeId) => (base as any).listEpisodes(animeId, prefer),
    resolveStream: base.resolveStream.bind(base),
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
      return {
        url: stream.url,
        type: stream.type ?? 'hls',
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
        throw new NotAvailableError('18anime-direct', 'returned no stream URL')
      }
      return {
        url: data.url,
        type: data.is_hls ? 'hls' : 'mp4',
      }
    },
  }
}

// ─── Kodik proxy helper ───────────────────────────────────────────────────────

function buildProxyUrl(url: string, referer: string): string {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
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
      // Use the first translation to determine episode count
      const first = translations[0]
      return Array.from({ length: first.episodes_count }, (_, i) => {
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
 * - provider === 'kodik'       → kodikAdapter (requires deps.kodikApi)
 * - provider in SCRAPER_IDS   → scraperAdapter (requires deps.scraperApi)
 * - provider === 'raw'         → rawAdapter (requires deps.rawApi)
 * - provider === '18anime-direct' → anime18Adapter (requires deps.anime18Api)
 * - anything else              → NotAvailableError
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
      return makeScraperAdapterWithPrefer(deps.scraperApi, provider)
    }

    if (provider === 'raw') {
      if (!deps.rawApi) {
        throw new NotAvailableError(provider, 'not available (rawApi dep missing)')
      }
      return makeRawAdapter(deps.rawApi)
    }

    if (provider === '18anime-direct') {
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
