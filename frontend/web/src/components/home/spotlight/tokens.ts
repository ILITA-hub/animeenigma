/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-02);
 * DS-alignment 2026-06-10 (spec: 2026-06-10-spotlight-ds-alignment-design.md).
 *
 * Per-card design tokens consumed by every card SFC + CarouselDots.
 *
 *   accent     — BRAND-TRIAD hue (A-1, user-approved): cards are
 *                differentiated by kicker ICON, not by a per-card rainbow.
 *                  cyan   = content core   (featured, personal_pick, platform_stats)
 *                  pink   = live/personal  (now_watching, continue_watching_new)
 *                  violet = meta/service   (random_tail, latest_news,
 *                                           telegram_news, not_time_yet)
 *   kickerKey  — i18n key for the card's small uppercase kicker (e.g.
 *                "spotlight.featured.title"). The same key doubles as the
 *                accessible label for the labeled-pill dot indicator.
 *   icon       — name of the inline-SVG icon rendered by SpotlightIcon.vue.
 *
 * The `cardTokens` map is keyed by `SpotlightCard['type']` — a Vitest parity
 * test in tokens.spec.ts iterates that union and asserts a matching entry,
 * so adding a 10th card variant to the union without bumping this map will
 * trip the test.
 */

import type { SpotlightCard } from '@/types/spotlight'

export type SpotlightAccent = 'cyan' | 'pink' | 'violet'

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
  | 'star'

export interface CardToken {
  accent: SpotlightAccent
  kickerKey: string
  icon: SpotlightIconName
}

/**
 * Per-changelog-entry-type lookup for LatestNewsCard. The card resolver
 * passes through whatever `type` string the backend's changelog.json
 * source carries; the conventional-commit-style keys below match the
 * v3.1 changelog conventions (feat/fix/perf/docs) AND the longer-form
 * synonyms ("feature"/"improvement") that exist in the historical
 * changelog.json payload.
 *
 * Unknown types fall back to the `sparkles` icon (no type pill rendered).
 */
export interface LatestNewsTypeBadge {
  i18nKey: string
  accent: string // Tailwind class string — pinned literal for jit safelist
}

export interface LatestNewsTokenExtensions {
  iconByType: Record<string, SpotlightIconName>
  labelByType: Record<string, LatestNewsTypeBadge>
}

// CardToken for `latest_news` is widened with the type-badge lookup
// tables. Other card-type tokens stay on the base `CardToken` shape.
export type LatestNewsCardToken = CardToken & LatestNewsTokenExtensions

// Derive the discriminator union from the SpotlightCard discriminated union
// so a future change to types/spotlight.ts is statically caught.
export type SpotlightCardType = SpotlightCard['type']

// Cyan pill for feat-family entries — brand cyan = "new functionality".
const FEAT_BADGE: LatestNewsTypeBadge = {
  i18nKey: 'spotlight.latestNews.typeFeat',
  accent: 'bg-cyan-500/20 text-cyan-400',
}
// Fix-family entries — semantic success token.
const FIX_BADGE: LatestNewsTypeBadge = {
  i18nKey: 'spotlight.latestNews.typeFix',
  accent: 'bg-success/15 text-success',
}
// Perf-family entries — semantic warning token.
const PERF_BADGE: LatestNewsTypeBadge = {
  i18nKey: 'spotlight.latestNews.typePerf',
  accent: 'bg-warning/15 text-warning',
}

// Map shape preserves the parity guarantee: every variant in SpotlightCard
// still resolves to a token with {accent, kickerKey, icon}; `latest_news`
// is upcast to the wider LatestNewsCardToken (Phase 07) so callers can read
// iconByType / labelByType without extra type assertions.
// (The 12-hue `featured.genreColors` map was removed in the DS alignment —
// genre tags render as neutral overlay badges now, per locked rule D-1.)
type CardTokenMap = Record<SpotlightCardType, CardToken> & {
  latest_news: LatestNewsCardToken
}

export const cardTokens: CardTokenMap = {
  featured:              { accent: 'cyan',   kickerKey: 'spotlight.featured.title',            icon: 'sparkles' },
  random_tail:           { accent: 'violet', kickerKey: 'spotlight.randomTail.title',          icon: 'shuffle'  },
  personal_pick:         { accent: 'cyan',   kickerKey: 'spotlight.personalPick.title',        icon: 'sparkles' },
  telegram_news:         { accent: 'violet', kickerKey: 'spotlight.telegramNews.title',        icon: 'telegram' },
  latest_news: {
    accent: 'violet',
    kickerKey: 'spotlight.latestNews.title',
    icon: 'sparkles',
    // Conventional-commit keys (feat/fix/perf/docs) + the long-form
    // synonyms that ship in the live changelog.json today. Without the
    // synonyms the type pill would silently disappear in production.
    iconByType: {
      feat:           'sparkles',
      feature:        'sparkles',
      fix:            'wrench',
      perf:           'lightning',
      improvement:    'lightning',
      infrastructure: 'wrench',
      docs:           'sparkles',
    },
    labelByType: {
      feat:           FEAT_BADGE,
      feature:        FEAT_BADGE,
      fix:            FIX_BADGE,
      perf:           PERF_BADGE,
      improvement:    PERF_BADGE,
      infrastructure: FIX_BADGE,
    },
  },
  platform_stats:        { accent: 'cyan',   kickerKey: 'spotlight.platformStats.title',       icon: 'chart'    },
  now_watching:          { accent: 'pink',   kickerKey: 'spotlight.nowWatching.title',         icon: 'pulse'    },
  not_time_yet:          { accent: 'violet', kickerKey: 'spotlight.notTimeYet.title',          icon: 'clock'    },
  continue_watching_new: { accent: 'pink',   kickerKey: 'spotlight.continueWatchingNew.title', icon: 'play'     },
  curated:               { accent: 'cyan',   kickerKey: 'spotlight.curated.title',             icon: 'star'     },
  daily_fanfic:          { accent: 'pink',   kickerKey: 'spotlight.dailyFanfic.title',         icon: 'sparkles' },
  upcoming_for_you:      { accent: 'cyan',   kickerKey: 'spotlight.upcomingForYou.title',      icon: 'clock'    },
}


// Active menu-pill classes per accent (v4 A-1 lock, 2026-06-11) — the
// expanded icon+label pill in CarouselDots. Tinted surface + accent ring
// + matching glow; pinned literals (Tailwind JIT scans verbatim strings).
export const accentMenuPill: Record<SpotlightAccent, string> = {
  cyan:   'bg-cyan-500/20 border-cyan-400/50 text-cyan-400 shadow-[0_0_14px_rgba(0,212,255,0.25)]',
  pink:   'bg-pink-500/20 border-pink-400/50 text-pink-400 shadow-[0_0_14px_rgba(255,45,124,0.25)]',
  violet: 'bg-brand-violet/20 border-brand-violet/50 text-brand-violet shadow-[0_0_14px_rgba(167,139,250,0.25)]',
}

// Kicker text color per accent — single source for SpotlightCardShell and
// any card that renders accent-colored text outside the shell. Pinned
// literals (Tailwind JIT scans verbatim strings).
export const accentText: Record<SpotlightAccent, string> = {
  cyan:   'text-cyan-400',
  pink:   'text-pink-400',
  violet: 'text-brand-violet',
}
