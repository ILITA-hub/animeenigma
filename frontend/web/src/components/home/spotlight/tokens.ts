/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-02).
 *
 * Per-card design tokens consumed by every card SFC + CarouselControls.
 *
 *   accent     — Tailwind color stem used for headers, CTAs, dot indicators.
 *   kickerKey  — i18n key for the card's small uppercase kicker (e.g.
 *                "spotlight.animeOfDay.title"). The same key doubles as the
 *                accessible label for the labeled-pill dot indicator.
 *   icon       — name of the inline-SVG icon rendered by SpotlightIcon.vue.
 *
 * The `cardTokens` map is keyed by `SpotlightCard['type']` — a Vitest parity
 * test in tokens.spec.ts iterates that union and asserts a matching entry,
 * so adding a 10th card variant to the union without bumping this map will
 * trip the test.
 */

import type { SpotlightCard } from '@/types/spotlight'

export type SpotlightAccent = 'cyan' | 'purple' | 'sky' | 'amber' | 'teal' | 'green'

export type SpotlightIconName =
  | 'telegram'
  | 'sparkles'
  | 'chart'
  | 'pulse'
  | 'clock'
  | 'play'
  | 'shuffle'
  | 'wrench'
  | 'lightning'

export interface CardToken {
  accent: SpotlightAccent
  kickerKey: string
  icon: SpotlightIconName
}

// Derive the discriminator union from the SpotlightCard discriminated union
// so a future change to types/spotlight.ts is statically caught.
export type SpotlightCardType = SpotlightCard['type']

/**
 * v1.1-polish Phase 02 (HSB-V11-AOD-04) — per-card extra tokens.
 *
 * `anime_of_day.genreColors` maps Shikimori genre IDs to Tailwind bg+text
 * class pairs so each genre tag renders in a hue that matches its mood.
 * Unmapped IDs fall back to the neutral `bg-white/10 text-gray-300` pair
 * (resolved at the call site via `?? 'bg-white/10 text-gray-300'`).
 *
 * Tailwind 4 needs every utility string to appear verbatim in source so
 * the JIT can scan them — keep these literal, do not template-compose.
 *
 * Shikimori genre IDs (verified via shikimori.one/api/genres):
 *   1  = Action       2  = Adventure   4  = Comedy       7  = Mystery
 *   8  = Drama        10 = Fantasy     14 = Horror       22 = Romance
 *   24 = Sci-Fi       30 = Sports      36 = Slice of Life 37 = Supernatural
 */
export const cardTokens: Record<SpotlightCardType, CardToken> & {
  anime_of_day: CardToken & { genreColors: Record<string, string> }
} = {
  anime_of_day:          {
    accent: 'cyan',
    kickerKey: 'spotlight.animeOfDay.title',
    icon: 'sparkles',
    genreColors: {
      '1':  'bg-red-500/20 text-red-200',         // Action
      '2':  'bg-blue-500/20 text-blue-200',       // Adventure
      '4':  'bg-yellow-500/20 text-yellow-200',   // Comedy
      '7':  'bg-orange-500/20 text-orange-200',   // Mystery
      '8':  'bg-pink-500/20 text-pink-200',       // Drama
      '10': 'bg-purple-500/20 text-purple-200',   // Fantasy
      '14': 'bg-fuchsia-500/20 text-fuchsia-200', // Horror
      '22': 'bg-rose-500/20 text-rose-200',       // Romance
      '24': 'bg-cyan-500/20 text-cyan-200',       // Sci-Fi
      '30': 'bg-indigo-500/20 text-indigo-200',   // Sports
      '36': 'bg-emerald-500/20 text-emerald-200', // Slice of Life
      '37': 'bg-violet-500/20 text-violet-200',   // Supernatural
    },
  },
  random_tail:           { accent: 'purple', kickerKey: 'spotlight.randomTail.title',          icon: 'shuffle'  },
  personal_pick:         { accent: 'cyan',   kickerKey: 'spotlight.personalPick.title',        icon: 'sparkles' },
  telegram_news:         { accent: 'sky',    kickerKey: 'spotlight.telegramNews.title',        icon: 'telegram' },
  latest_news:           { accent: 'amber',  kickerKey: 'spotlight.latestNews.title',          icon: 'sparkles' },
  platform_stats:        { accent: 'teal',   kickerKey: 'spotlight.platformStats.title',       icon: 'chart'    },
  now_watching:          { accent: 'green',  kickerKey: 'spotlight.nowWatching.title',         icon: 'pulse'    },
  not_time_yet:          { accent: 'amber',  kickerKey: 'spotlight.notTimeYet.title',          icon: 'clock'    },
  continue_watching_new: { accent: 'purple', kickerKey: 'spotlight.continueWatchingNew.title', icon: 'play'     },
}

// Tailwind class strings for the active labeled-pill dot, by accent. Kept
// here so CarouselControls.vue can simply read `accentDotBg[token.accent]`
// without inline ternaries (and so the lint rule that pins Tailwind class
// strings to source files keeps working).
export const accentDotBg: Record<SpotlightAccent, string> = {
  cyan:   'bg-cyan-500/30 text-cyan-100',
  purple: 'bg-purple-500/30 text-purple-100',
  sky:    'bg-sky-500/30 text-sky-100',
  amber:  'bg-amber-500/30 text-amber-100',
  teal:   'bg-teal-500/30 text-teal-100',
  green:  'bg-green-500/30 text-green-100',
}
