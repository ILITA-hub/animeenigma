---
workstream: hero-spotlight
milestone: v1.1-polish
total: 30
prefix: HSB-V11
---

# v1.1-polish REQUIREMENTS

## Cross-cutting (Phase 01)

| ID | Requirement | Verification |
|---|---|---|
| HSB-V11-CC-01 | `SpotlightBackdrop.vue` exists with two variants: `poster-blur` (blurred + tinted poster) and `gradient-mesh` (branded gradient). | Vitest snapshot per variant. |
| HSB-V11-CC-02 | `tokens.ts` exports a `cardTokens` object with exactly 9 entries (one per `SpotlightCard.type`). | Type-iteration test enforces parity. |
| HSB-V11-CC-03 | `SpotlightIcon.vue` renders 9 named icons (telegram, sparkles, chart, pulse, clock, play, shuffle, wrench, lightning) via inline `<svg>`. | DOM presence test per icon name. |
| HSB-V11-CC-04 | `main.css` defines `.cta-hero`, `.cta-card`, `.cta-text` classes with documented hierarchy. | Visual diff against UI-SPEC; no card overrides spec contract. |
| HSB-V11-CC-05 | Carousel `<transition>` is wrapped with `isTransitioning` lock — `next`/`prev`/`goTo` no-op while transitioning. | E2E: 10× rapid ArrowRight produces ≤ `ceil(time/duration)` settled cards, never opacity:0 mid-fade. |
| HSB-V11-CC-06 | `CarouselControls.vue` renders labeled-pill dots with card-type icon + tooltip on hover. Active dot uses card's accent color. | Axe-core: tooltips meet AA contrast; `aria-current="true"` on active dot. |

## Card-specific

### AnimeOfDayCard (Phase 02)

| ID | Requirement |
|---|---|
| HSB-V11-AOD-01 | Backdrop renders via `<SpotlightBackdrop variant="poster-blur" :poster-url="data.anime.poster_url">`. |
| HSB-V11-AOD-02 | Disabled "Add to list" button is removed; only one `cta-hero` CTA ("Смотреть →") remains. |
| HSB-V11-AOD-03 | Foreground poster widens to `lg:w-56` (224px); gets `shadow-2xl shadow-cyan-500/20` hover. |
| HSB-V11-AOD-04 | Genre tags use color-coded backgrounds via `cardTokens.anime_of_day.genreMap[genreId]` (fallback `bg-white/10`). |

### RandomTailCard (Phase 03)

| ID | Requirement |
|---|---|
| HSB-V11-RT-01 | Backdrop is `poster-blur` with `purple/30` gradient overlay (distinct from AnimeOfDay's cyan). |
| HSB-V11-RT-02 | Header renders `<SpotlightIcon name="shuffle">` + kicker at `text-purple-200 text-xs uppercase tracking-[0.2em]`. |
| HSB-V11-RT-03 | Tagline randomly selected from 4 candidates in i18n (`spotlight.randomTail.taglines[]`); re-randomized on each mount. |
| HSB-V11-RT-04 | Shuffle-deck mount animation runs ONLY when `!reducedMotion`. |

### PersonalPickCard (Phase 04)

| ID | Requirement |
|---|---|
| HSB-V11-PP-01 | Layout: featured pick (60% width) + 2 secondary picks (40% width, stacked) on desktop. |
| HSB-V11-PP-02 | Featured pick titles no longer truncate (parent grid height adjusted). |
| HSB-V11-PP-03 | Username in title when `data.source === 'personal'` (e.g. "Для вас, ui_audit_bot"). |
| HSB-V11-PP-04 | Mobile (<md): featured-pick full width + full-width "+ N more →" footer button (not corner link). |

### NowWatchingCard (Phase 05)

| ID | Requirement |
|---|---|
| HSB-V11-NW-01 | Each row poster: `w-14 h-21` (56×84) — 3.5× larger than current 32×44. |
| HSB-V11-NW-02 | Avatar circle deterministic from `hash(username) % palette` of 8 colors. |
| HSB-V11-NW-03 | Backdrop = `gradient-mesh` with `cyan→green` animated mesh suggesting "live activity". |
| HSB-V11-NW-04 | LIVE indicator becomes a pulsing micro-element next to avatar (not text on the right). |

### TelegramNewsCard (Phase 06)

| ID | Requirement |
|---|---|
| HSB-V11-TG-01 | Backdrop = `gradient-mesh` with `from-[#229ED9]/30` (Telegram blue). |
| HSB-V11-TG-02 | Header renders Telegram logo SVG with `aria-label="Telegram"`. |
| HSB-V11-TG-03 | When `post.image_url` is present, render a 1:1 thumbnail next to the excerpt. Falls back to text-only otherwise. |
| HSB-V11-TG-04 | `services/catalog/.../telegram_news.go` passes through the image URL from the existing `news:telegram` cache without new fetches. |

### LatestNewsCard (Phase 07)

| ID | Requirement |
|---|---|
| HSB-V11-LN-01 | Each entry renders `<SpotlightIcon>` whose name = `cardTokens.latest_news.iconByType[entry.type]`. |
| HSB-V11-LN-02 | Type pill ("Новая фича" / "Исправление" / "Улучшение") with type-coded accent class. |
| HSB-V11-LN-03 | Date rendered via `Intl.RelativeTimeFormat` (e.g. "2 дня назад"); fallback to absolute ISO if formatter throws. |
| HSB-V11-LN-04 | Sentence-splitter regex removed; title-vs-body derived from a new optional `title` field, fallback to first 60 chars + ellipsis. |

### PlatformStatsCard (Phase 08)

| ID | Requirement |
|---|---|
| HSB-V11-PS-01 | Layout: hero stat (left, `text-8xl`) + 2×2 micro-grid (right) with up to 4 supporting metrics. |
| HSB-V11-PS-02 | Sparkline SVG renders `series[7]` daily values; `data-points` attribute matches series. |
| HSB-V11-PS-03 | Delta chip computed from `value vs previous_value`; green ↑ when positive, red ↓ when negative, gray "—" when zero. |
| HSB-V11-PS-04 | `services/catalog/.../platform_stats.go` extends response: `previous_value: int` + `series: [n,n,n,n,n,n,n]`. |

### NotTimeYetCard (Phase 09)

| ID | Requirement |
|---|---|
| HSB-V11-NT-01 | Backdrop = `poster-blur` with `amber/30` overlay (warm/nostalgic). |
| HSB-V11-NT-02 | Status pill: "В планах" (yellow) when `status === 'planned'`, "Отложено" (gray-blue) when `'postponed'`. |
| HSB-V11-NT-03 | CTA href is `/anime/{id}/watch` (direct-to-watch, not detail page). |
| HSB-V11-NT-04 | Relative `added_at` ("2 недели назад") rendered. `services/catalog/.../not_time_yet.go` passes `added_at` from existing SELECT. |

### ContinueWatchingNewCard (Phase 10)

| ID | Requirement |
|---|---|
| HSB-V11-CWN-01 | Hero ribbon spans poster top: "🎬 НОВАЯ СЕРИЯ {n}!" with `data.new_episode_number` interpolated. |
| HSB-V11-CWN-02 | Two-row episode meta: "Вы посмотрели до серии {last}" (subdued) + "Новая серия {new}" (purple accent, large). |
| HSB-V11-CWN-03 | CTA href is `/anime/{id}/watch?episode={new_episode_number}` (deep-link, not detail page). |
