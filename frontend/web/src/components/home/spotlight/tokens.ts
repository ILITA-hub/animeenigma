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

export const cardTokens: Record<SpotlightCardType, CardToken> = {
  anime_of_day:          { accent: 'cyan',   kickerKey: 'spotlight.animeOfDay.title',          icon: 'sparkles' },
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
