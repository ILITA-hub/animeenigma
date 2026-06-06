// frontend/web/src/components/player/unified/providerRegistry.ts
import type { ProviderDef } from '@/types/unifiedPlayer'

// Identity hues are the design-system "brand-exempt" hues (NOT the lint-forbidden
// palette): cyan/orange/pink/rose. Keep hex here (allowlisted in DS task).
export const PROVIDER_REGISTRY: ProviderDef[] = [
  // First-party — WIP, always inactive for now.
  { id: 'ae', name: 'AnimeEnigma', hue: '#00d4ff', group: 'first-party',
    audios: ['sub', 'dub'], langs: ['en', 'ru'], content: ['common'], scraper: false,
    staticDisabled: { reason: 'WIP', description: 'We are working on our own hosting', wip: true } },

  // EN scraper providers (live health from backend).
  { id: 'allanime',   name: 'AllAnime',   hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'animefever', name: 'AnimeFever', hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'gogoanime',  name: 'Gogoanime',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'miruro',     name: 'Miruro',     hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },
  { id: 'nineanime',  name: '9anime',     hue: '#00d4ff', group: 'en', audios: ['sub'],        langs: ['en'], content: ['common'], scraper: true },
  { id: 'animepahe',  name: 'AnimePahe',  hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true },

  // RU.
  { id: 'kodik',   name: 'Kodik',  hue: '#22d3ee', group: 'ru', audios: ['dub', 'sub'], langs: ['ru'], content: ['common'], scraper: false },
  { id: 'animelib', name: 'AniLib', hue: '#ff8a3d', group: 'ru', audios: ['sub'], langs: ['ru'], content: ['common'], scraper: false,
    staticDisabled: { reason: 'Unavailable', description: 'AniLib direct streams are not currently working' } },

  // 18+ (adult group). 18anime is a scraper provider but in the adult orchestrator.
  { id: 'hanime',  name: 'Hanime', hue: '#ff4d8d', group: 'adult', audios: ['dub'], langs: ['ru'], content: ['hentai'], scraper: false,
    staticDisabled: { reason: 'Unavailable', description: 'Hanime streams are not currently working' } },
  { id: '18anime', name: '18anime', hue: '#fb7185', group: 'adult', audios: ['sub', 'dub'], langs: ['en'], content: ['hentai'], scraper: true },

  // JP raw.
  { id: 'raw', name: 'Raw', hue: '#fb7185', group: 'raw', audios: ['sub'], langs: ['ja'], content: ['common'], scraper: false },
]

export const providerById = (id: string): ProviderDef | undefined =>
  PROVIDER_REGISTRY.find(p => p.id === id)
