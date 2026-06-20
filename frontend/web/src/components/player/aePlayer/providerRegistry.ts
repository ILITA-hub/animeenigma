// frontend/web/src/components/player/aePlayer/providerRegistry.ts
import type { ProviderDef } from '@/types/aePlayer'

// Identity hues are the design-system "brand-exempt" hues (NOT the lint-forbidden
// palette): cyan/orange/pink/rose. Keep hex here (allowlisted in DS task).
export const PROVIDER_REGISTRY: ProviderDef[] = [
  // First-party — self-hosted library (MinIO HLS). Episodes/stream resolve
  // straight from on-prem storage; the in-player provider shows no episodes
  // until a title has been encoded into the library. Broad audio/lang
  // coverage so it stays selectable as a first-party source under any filter.
  { id: 'ae', name: 'AnimeEnigma', hue: '#00d4ff', group: 'first-party',
    audios: ['sub', 'dub'], langs: ['en', 'ru', 'ja'], content: ['common'], scraper: false,
    blurb: 'First-party self-hosted library (MinIO HLS). Only titles encoded on-prem.' },

  // EN scraper providers (live health from backend).
  { id: 'allanime',   name: 'AllAnime',   hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — direct MP4 / robust HLS, broad coverage.' },
  { id: 'animefever', name: 'AnimeFever', hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — degraded: ad-substitution on our IP class.' },
  { id: 'gogoanime',  name: 'Gogoanime',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — megaplay HLS, ~78% popular coverage.' },
  // Miruro is DUB-only: its upstream stopped serving sub streams, so it must not
  // be offered/auto-selected for SUB (original-Japanese-audio) playback (2026-06-19).
  { id: 'miruro',     name: 'Miruro',     hue: '#00d4ff', group: 'en', audios: ['dub'],        langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — DUB only (upstream dropped sub).' },
  { id: 'nineanime',  name: '9anime',     hue: '#00d4ff', group: 'en', audios: ['sub'],        langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — MP4-only last resort.' },
  { id: 'animepahe',  name: 'AnimePahe',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — Kwik via stealth-Chromium sidecar.' },

  // RU.
  { id: 'kodik',   name: 'Kodik',  hue: '#22d3ee', group: 'ru', audios: ['dub', 'sub'], langs: ['ru'], content: ['common'], scraper: false,
    blurb: 'RU HLS — Russian dub & sub teams.' },
  { id: 'animelib', name: 'AniLib', hue: '#ff8a3d', group: 'ru', audios: ['sub'], langs: ['ru'], content: ['common'], scraper: false,
    blurb: 'RU direct MP4 — currently unavailable.',
    staticDisabled: { reason: 'Unavailable', description: 'AniLib direct streams are not currently working' } },

  // 18+ (adult group). 18anime is a scraper provider but in the adult orchestrator.
  { id: 'hanime',  name: 'Hanime', hue: '#ff4d8d', group: 'adult', audios: ['dub'], langs: ['ru'], content: ['hentai'], scraper: false,
    blurb: '18+ only — Russian dub.' },
  { id: '18anime', name: '18anime', hue: '#fb7185', group: 'adult', audios: ['sub', 'dub'], langs: ['en'], content: ['hentai'], scraper: true,
    blurb: '18+ only — EN sub/dub scraper.' },

  // JP raw.
  { id: 'raw', name: 'Raw', hue: '#fb7185', group: 'raw', audios: ['sub'], langs: ['ja'], content: ['common'], scraper: false,
    blurb: 'Original Japanese audio — pair with browsable JP subs.' },
]

export const providerById = (id: string): ProviderDef | undefined =>
  PROVIDER_REGISTRY.find(p => p.id === id)

// Hand-ranked default-selection order, best-first. The smart default walks
// this list and picks the first provider whose row is `active` (and, for
// availability-gated providers like first-party `ae`, that actually has a
// local copy). Brand-exempt: order is reliability/quality judgement, not the
// registry array order. Tune as real telemetry lands (Stage 2).
export const CURATED_TIER: string[] = [
  'ae',         // first-party self-hosted — preferred WHEN the title is in the library
  'allanime',   // direct-MP4 / robust HLS
  'gogoanime',  // megaplay, ~78% popular coverage
  'miruro',
  'animepahe',
  'animefever',
  'nineanime',
  'kodik',      // RU
  'raw',        // JP
  '18anime',    // adult
  'animelib',
  'hanime',
]
