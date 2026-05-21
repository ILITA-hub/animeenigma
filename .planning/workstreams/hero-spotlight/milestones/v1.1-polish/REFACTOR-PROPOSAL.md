---
milestone: v1.1-polish
workstream: hero-spotlight
status: proposal-pending-review
created: 2026-05-21
parent_uat: ../v1.0-phases/03-dynamic-cards-migration/03-UAT.md
direction: cinematic-backdrops-distinct-templates
---

# Hero Spotlight v1.1 — Visual Refactor Proposal

> **Why this milestone exists.** Phase 03 of v1.0 shipped a functional 9-card
> spotlight, but UAT (2026-05-21) flagged every card as visually below bar:
> *"EACH card looks poor — refactor them one by one."* This proposal sketches
> a card-by-card refactor that gives each card type its own visual identity
> while keeping the carousel chrome and backend resolvers untouched.

> **Scope note.** The original v1.1 candidates from the v1.0 audit
> (slide-order personalization, opt-outs, editorial card, WebSocket
> now_watching, flag cleanup) are deferred to v1.2 to keep this polish
> milestone laser-focused.

---

## Cross-cutting design direction

### 1. Backdrop layer (NEW — adds visual depth)

A new `<SpotlightBackdrop>` component renders behind each card:

- **Anime cards** (AnimeOfDay, RandomTail, NotTimeYet, ContinueWatchingNew):
  blurred + tinted poster (`filter: blur(40px) saturate(1.2)`), `opacity 0.4`,
  with a vignette gradient that fades poster → card-bg from 60% across.
  → Each anime card now *feels like that anime* without sacrificing text legibility.
- **News/stats cards** (LatestNews, TelegramNews, PlatformStats): branded
  gradient mesh (cyan→purple→pink for news; cyan→teal for stats; telegram-blue
  variants for telegram news). No backdrop imagery; gradient + subtle noise.
- **Multi-item cards** (PersonalPick, NowWatching): use the first item's
  blurred poster (PersonalPick) or a generic "live" cyan/green animated mesh
  (NowWatching).

Implementation: shared `<SpotlightBackdrop :variant="..." :poster-url="...">`
that emits a single positioned `<div class="absolute inset-0">` consumed via
`<slot name="backdrop">` in each card. Pure CSS — no JS, no extra requests
beyond posters cards already load.

### 2. Card-type theming via a small token map

```ts
// frontend/web/src/components/home/spotlight/tokens.ts
export const cardTokens = {
  anime_of_day:        { accent: 'cyan',   kicker: 'Аниме дня' },
  random_tail:         { accent: 'purple', kicker: 'Случайная находка' },
  personal_pick:       { accent: 'cyan',   kicker: 'В тренде / Для вас' },
  telegram_news:       { accent: 'sky',    kicker: 'Из нашего Telegram', icon: 'telegram' },
  latest_news:         { accent: 'amber',  kicker: 'Что нового',          icon: 'sparkles' },
  platform_stats:      { accent: 'teal',   kicker: 'Платформа за неделю', icon: 'chart' },
  now_watching:        { accent: 'green',  kicker: 'Сейчас смотрят',      icon: 'pulse' },
  not_time_yet:        { accent: 'amber',  kicker: 'Не пришло ли время?', icon: 'clock' },
  continue_watching_new:{ accent: 'purple',kicker: 'Новая серия!',        icon: 'play' },
}
```

Each accent maps to a Tailwind utility set: `text-{accent}-300`,
`from-{accent}-500/20`, etc. — kept in one place so card files stay terse.

### 3. CTA hierarchy

Today: every card uses identical `btn btn-primary text-sm md:text-base`.
After: 3 sizes — `cta-hero` (oversized, anime cards), `cta-card` (default),
`cta-text` (text-only secondary). All defined in `main.css`, no per-card
overrides.

### 4. Carousel chrome (separate from cards)

- **Dots**: replace 6 grey dots with labeled pill indicators — each shows the
  card-type icon. On hover/focus reveal a tooltip with the card kicker.
  Active dot uses the card's accent color.
- **Chevrons**: increase tap target to `min-h-[44px]`, add backdrop-blur halo.
- **Transition lock** (fixes the blank-card bug surfaced during inspection):
  add an `isTransitioning` ref that blocks `next()`/`prev()`/`goTo()` while
  Vue's `<transition mode="out-in">` is between `leave` and `enter`. Pair
  with a CSS variable for the transition duration so the lock window matches
  the actual fade time. Restores a clean experience under rapid clicks.

---

## Phasing — 10 atomic phases, executable one-by-one

| # | Phase | Touches | Why this order |
|---|---|---|---|
| 1 | **Foundation** — tokens, `<SpotlightBackdrop>`, CTA classes, transition lock, carousel chrome | shared components only; no card edits | Unblocks every other phase. Ship first, get the transition bug fix to production immediately. |
| 2 | **AnimeOfDayCard** refactor | `cards/AnimeOfDayCard.vue` + spec | Highest-frequency anime card; sets the cinematic-backdrop pattern. |
| 3 | **RandomTailCard** refactor | `cards/RandomTailCard.vue` + spec | Differentiate from AnimeOfDay (purple "discovery" accent, shuffle iconography). |
| 4 | **PersonalPickCard** refactor | `cards/PersonalPickCard.vue` + spec | Fixes the cut-off-title bug; redesigns the 1/2/3-poster grid. |
| 5 | **NowWatchingCard** refactor | `cards/NowWatchingCard.vue` + spec | Larger poster thumbs; pulsing accent; social/live feel. |
| 6 | **TelegramNewsCard** refactor | `cards/TelegramNewsCard.vue` + spec | Telegram blue branding; channel attribution; richer post layout. |
| 7 | **LatestNewsCard** refactor | `cards/LatestNewsCard.vue` + spec | Type-based icons (feat/fix/perf); proper title field on backend (or smarter split). |
| 8 | **PlatformStatsCard** refactor | `cards/PlatformStatsCard.vue` + spec | Fill the N=1 dead space with sparkline + comparator copy. |
| 9 | **NotTimeYetCard** refactor | `cards/NotTimeYetCard.vue` + spec | Distinct "you-bookmarked-this" vibe (amber clock accent) vs AnimeOfDay. |
| 10 | **ContinueWatchingNewCard** refactor | `cards/ContinueWatchingNewCard.vue` + spec | Resume CTA jumps direct to `ep N` watch route; "new episode" badge becomes a hero ribbon. |

Each phase is self-contained (touches 1-3 files), test-driven (`.spec.ts`
updated with new visual assertions), and end-to-end verifiable
(`bunx playwright test spotlight-full` regression run after each card).

---

## Per-card detail sheets

Each block below states: **current visual problem → target visual → concrete diffs**.

### Phase 01 · Foundation

**Why first.** Every other phase consumes the new tokens, the backdrop
component, and the transition lock. Also ships the blank-card transition
bug fix that surfaced during UAT inspection.

**Deliverables**
- `frontend/web/src/components/home/spotlight/tokens.ts` — card-type token map.
- `frontend/web/src/components/home/spotlight/SpotlightBackdrop.vue` —
  shared backdrop renderer (variants: `poster-blur`, `gradient-mesh`).
- `frontend/web/src/components/home/spotlight/SpotlightIcon.vue` — 9-icon
  sprite (telegram, sparkles, chart, pulse, clock, play, etc.) so each card
  can reference `<SpotlightIcon name="..." />` without an icon-library dep.
- `frontend/web/src/styles/main.css` — add `.cta-hero`, `.cta-card`,
  `.cta-text` button classes; replace the existing `spotlight-fade` CSS with
  a variable-driven duration (`--spotlight-fade-ms: 400ms`).
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` —
  add `isTransitioning` ref + `@before-leave`/`@after-enter` listeners on
  the `<transition>` that gate `next()`/`prev()`/`goTo()`.
- `CarouselControls.vue` — labeled-pill dots with hover tooltips + larger
  chevron tap targets.

**Verification gates**
- Transition lock: 10 rapid `<right-arrow>` keypresses produce 1 settled
  card per ~500ms (not 10 mid-fades). E2E test under
  `frontend/web/e2e/spotlight-transition-lock.spec.ts`.
- Tokens lookup: 9 entries in `cardTokens` map (one per card type) —
  enforced by a Vitest test that iterates the `SpotlightCard` union.
- A11y: tooltips reach AA contrast, dots have `aria-current` + tooltip text.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.04 * 13 · MVQ = Griffin 88%/85%`

---

### Phase 02 · AnimeOfDayCard

**Current problems**
- Disabled "Add to list" CTA renders as visual noise (`opacity-50 cursor-not-allowed`).
- Small fixed-width poster (lg:w-44 = 176px) → poster looks postage-stamp on a 1250px-wide hero.
- Generic dark card with no anime-specific personality.

**Target visual**
- Full-bleed blurred poster backdrop (via `<SpotlightBackdrop variant="poster-blur">`).
- Foreground anime poster scales up to `lg:w-56` (224px) and gets a subtle
  drop-shadow + cyan-glow border on hover.
- "АНИМЕ ДНЯ" kicker uses `text-cyan-300/90 text-[10px] uppercase tracking-[0.18em]` over the backdrop.
- Genre tags get color-coded backgrounds (action=red, comedy=yellow, etc. — map in tokens.ts).
- Score badge moves from poster-overlay to a meta-row pill so the poster art is unobstructed.
- Remove the disabled "Add to list" CTA entirely (until wired); single oversized cyan "Смотреть →" `cta-hero` button.

**Spec assertions added** (`AnimeOfDayCard.spec.ts`)
- `<SpotlightBackdrop>` is rendered with `variant="poster-blur"` and the anime's `poster_url`.
- No element matches `[aria-disabled="true"]` (the dead CTA is gone).
- Genre tags use background classes from `cardTokens.anime_of_day.genreMap`.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Phoenix 82%/80%`

---

### Phase 03 · RandomTailCard

**Current problems**
- Visually indistinguishable from AnimeOfDayCard (same poster-left/text-right layout).
- "СЛУЧАЙНАЯ НАХОДКА" kicker uses faint `text-cyan-300/80` — too quiet for what should be a discovery card.
- No iconography or motion suggesting "shuffle / surprise".

**Target visual**
- Backdrop uses `variant="poster-blur"` BUT with a `from-purple-500/30` gradient overlay (purple = discovery, not cyan).
- Add a small `<SpotlightIcon name="shuffle" class="w-4 h-4 text-purple-300">` next to the kicker.
- Kicker promoted to `text-purple-200 text-xs uppercase tracking-[0.2em] font-semibold` for stronger presence.
- Tagline: "Откройте что-то новое" → swap to a randomly-rotated set of 4 taglines from i18n (so two RandomTail cards in a session don't read identically).
- Reuse `cta-hero` button class (purple variant via `data-accent="purple"`).
- Optional: a 2-second "shuffle deck" animation on mount (5 mini-cards fanning to a single poster) — gated behind `prefers-reduced-motion`.

**Spec assertions**
- `cardTokens.random_tail.accent === 'purple'` applied to kicker + CTA.
- `SpotlightIcon` with `name="shuffle"` is in the DOM.
- Reduced-motion: shuffle animation skipped (mount-time check).

**Metrics**: `UXΔ = +2 (Better) · CDI = 0.03 * 5 · MVQ = Sprite 78%/82%`

---

### Phase 04 · PersonalPickCard

**Current problems**
- 3 huge full-bleed posters with `flex-1` → posters eat all height, titles below get clipped.
- Tiny "В ТРЕНДЕ" kicker — easily missed.
- Mobile shows 1 poster + tiny "+ 2 ещё →" link in a corner; feels orphaned.
- No reason copy per item ("because you watched X") — just the generic title.

**Target visual**
- Two-zone layout: featured pick (left, 60% width) + 2 secondary picks (right column, 40% width, stacked).
- Featured: backdrop = featured's blurred poster, foreground poster ~280×400, large title + reason chip (e.g. "Похоже на Steins;Gate").
- Secondary picks: small 96×144 cards in a vertical stack, each with title + a one-line reason.
- Mobile (<md): featured pick fills viewport, "+ 2 ещё →" becomes a full-width footer button (not a tiny link).
- Logged-in title ("Для вас, ui_audit_bot") vs anon title ("В тренде на этой неделе") — use the username when present.

**Spec assertions**
- Featured-pick container present + `aria-label` references its anime title.
- Secondary picks render exactly `data.items.length - 1` rows.
- Mobile (<768px) hides secondary picks but renders a full-width "+ N more" CTA.
- Username appears in logged-in title when `data.source === 'personal' && data.username`.

**Metrics**: `UXΔ = +4 (Better) · CDI = 0.05 * 13 · MVQ = Kraken 88%/85%`

---

### Phase 05 · NowWatchingCard

**Current problems**
- 32×44px poster thumb — too small to recognize the anime.
- 3 rows look identical (just username + episode number swap) → low scannability.
- No avatars for the user, no "you might know X" thread.

**Target visual**
- Each row becomes a richer card-strip: 56×84 poster (3.5x larger), gradient-tinted by anime's dominant color (precomputed at fetch or via canvas).
- Username uses a small color-hashed avatar circle (deterministic — `bg-{hash(username) % palette}-500`).
- "LIVE" badge becomes a pulsing micro-element next to avatar (not text on the right).
- Background: NO blurred poster (because there are up to 3 anime, none dominant) — instead a subtle animated cyan→green mesh gradient suggesting "real-time activity".
- Hover row: lift effect + accent border.

**Spec assertions**
- Avatar circle present with deterministic color class for given username.
- Poster `w-14 h-21` (56×84) — assert via class.
- Each row is a `router-link` to `/anime/{anime_id}`.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 80%/75%`

---

### Phase 06 · TelegramNewsCard

**Current problems**
- 3 plain text excerpts in equal-width boxes — no Telegram visual identity.
- No post imagery (Telegram posts often have media — we discard it).
- "Открыть пост →" link is the same cyan as everything else.

**Target visual**
- Backdrop: `variant="gradient-mesh"` with telegram-blue (`#229ED9`) + dark navy.
- Header gets the Telegram logo (inline SVG, mono) + channel name + subscriber count if available from existing `news:telegram` cache structure.
- Post layout: optional 1:1 thumbnail (left) + text excerpt (right) per post; falls back to text-only if no media.
- "Открыть пост →" uses telegram-blue accent and an external-link icon.
- 1/2/3-post grid uses the same `AdaptiveSlice` rule we already enforce on backend.

**Backend touch (small)**
- `services/catalog/internal/service/spotlight/cards/telegram_news.go`: pass through any image URL field already in `news:telegram` cache (zero new fetches).

**Spec assertions**
- Telegram SVG mark rendered with `aria-label="Telegram"`.
- External link uses `target="_blank" rel="noopener noreferrer"` (T-03-18 pin held).
- Post thumb visible when `post.image_url` present; falls back cleanly otherwise.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 84%/86%`

---

### Phase 07 · LatestNewsCard

**Current problems**
- Sentence-splitter regex produces fragile titles (`^(.+?[.!?——–:])` matches dates, URLs, etc.).
- 3 text-only entries with no visual distinction between feature / bugfix / perf-improvement.
- "Прочитать →" anchor scrolls to `#changelog` — but most users don't know that section exists below.

**Target visual**
- Each entry gets a type-icon (sparkles / wrench / lightning-bolt / etc.) based on the entry's `type` field (already in the Phase-1 resolver schema).
- Type pill on the right ("Новая фича", "Исправление", "Улучшение") with type-coded accent color.
- Date shown as relative ("2 дня назад") instead of raw ISO.
- "Открыть журнал →" link gets clearer scroll affordance (chevron-down + scroll-snap target).

**Backend touch (none required)**
- The Phase-1 resolver already emits `{date, type, message}` per entry.
- Optional follow-up: split `message` into `title` + `body` server-side so the frontend regex can go away.

**Spec assertions**
- `<SpotlightIcon>` matches the `entry.type` for each entry (mapped via `cardTokens.latest_news.iconByType`).
- Relative date uses `Intl.RelativeTimeFormat` with `localeStr`.
- Type pill has accent class drawn from `cardTokens.latest_news.accentByType`.

**Metrics**: `UXΔ = +2 (Better) · CDI = 0.03 * 5 · MVQ = Sprite 75%/80%`

---

### Phase 08 · PlatformStatsCard

**Current problems**
- N=1 metric renders as a single tile with ~800px of empty space on either side.
- No comparator copy (e.g. "+15% vs prior week").
- No iconography for the metric type.

**Target visual**
- Layout: hero stat (left, oversized 8xl number, accent color) + supporting micro-grid of related stats (right, 2×2 grid: total animes, active users, top genre, top anime).
- Sparkline below the hero number: 7-day mini-chart of the metric value (deterministic — derived from existing daily counters; no new endpoint).
- Delta chip: green ↑ / red ↓ vs prior period, computed in component.
- Card backdrop: `variant="gradient-mesh"` with teal→cyan, with a faint chart-grid SVG pattern overlay.

**Backend touch (small)**
- `cards/platform_stats.go`: extend response to include `previous_value` for delta computation, and `series: [n,n,n,n,n,n,n]` for the sparkline. Reuse existing daily-aggregation; emit through the same envelope.

**Spec assertions**
- Sparkline SVG element present with `data-points` attribute matching `series`.
- Delta chip uses `text-green` when `value > previous_value`, `text-red` otherwise.
- Micro-grid renders exactly 4 supporting stats when present.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.05 * 8 · MVQ = Kraken 80%/75%`

---

### Phase 09 · NotTimeYetCard

**Current problems**
- Layout identical to AnimeOfDayCard.
- "Не пришло ли время?" subtitle is the only differentiator and it's tiny grey text.
- CTA points to `/anime/{id}` — doesn't surface the user's existing list status.

**Target visual**
- Backdrop: poster-blur with `amber/30` overlay (warm = nostalgic / "remember this?").
- Header: large clock icon + "Не пришло ли время?" promoted to `text-amber-200 text-base font-semibold`.
- Subtitle moved to a status pill: "В планах" (yellow) or "Отложено" (gray-blue), color-coded.
- CTA: "Начать просмотр →" → goes directly to `/anime/{id}/watch` (not detail page) since the user is already aware of the anime.
- Add a small "Last added: 2 weeks ago" timestamp so the user feels the reminder context.

**Backend touch (small)**
- `cards/not_time_yet.go`: include `added_at` from the user's list row (already in the SELECT, just needs to flow to the Data type).

**Spec assertions**
- Status pill renders "В планах" for `data.status === 'planned'`, "Отложено" for `'postponed'`.
- CTA href ends in `/watch`.
- Relative `added_at` ("2 недели назад") rendered when provided.

**Metrics**: `UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 82%/80%`

---

### Phase 10 · ContinueWatchingNewCard

**Current problems**
- Layout identical to AnimeOfDayCard + a tiny "New episode N!" purple badge.
- CTA goes to `/anime/{id}` — user has to click again to find the episode.
- No emphasis on which episode is new vs which they've watched.

**Target visual**
- Backdrop: poster-blur with `purple/30` overlay (purple = new content).
- Hero ribbon across the top of the poster: "🎬 НОВАЯ СЕРИЯ {n}!" (replaces the small corner badge).
- Side meta layout shows a 2-row stack: "Вы посмотрели до серии {last_watched_episode}" (subdued) + "Новая серия {new_episode_number}" (large + accent).
- CTA: "Смотреть серию {n} →" → href `/anime/{id}/watch?episode={new_episode_number}`.
- Optional: thumbnail of the new episode's first frame (if available) as a secondary visual element.

**Spec assertions**
- Hero ribbon renders with `data.new_episode_number` interpolated.
- CTA href contains both `/watch` AND `episode={n}`.
- Two episode-number lines render (last watched + new), accent classes correct.

**Metrics**: `UXΔ = +4 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 86%/82%`

---

## Open questions before plan files get written

1. **Backdrop opacity** — 0.4 is a starting point; want me to A/B `0.3 / 0.4 / 0.5`
   live before locking it in? (We can prototype during Phase 01.)
2. **Telegram thumbnail availability** — Phase 06 needs `image_url` in the
   `news:telegram` cache. Worth a 5-min spike to confirm what fields the
   existing Telegram pipeline writes before committing the backend touch.
3. **PlatformStats sparkline** — would 7 daily samples be enough, or do we
   want 14 for a steadier trendline? (Frontend doesn't care; the resolver
   choice affects cache TTL.)
4. **Mobile breakpoint feel** — proposals above describe desktop. Each phase
   plan will spell out the `<md` rebreakdown explicitly. Do you want me to
   prioritize mobile-first or desktop-first per card?
5. **Reduced-motion** — the shuffle-deck animation on RandomTail is the only
   added motion. Everything else relies on opacity fades that already honor
   `prefers-reduced-motion`. OK?

---

## Next step

Once you sign off on this proposal (or redirect any of the cards), I'll
convert it into:

- `milestones/v1.1-polish/ROADMAP.md` (10 phases)
- `milestones/v1.1-polish/REQUIREMENTS.md` (~25-30 reqs across cards)
- 10 `phases/{NN}-…/{NN}-PLAN.md` files (one per phase, plan-checker-ready)

Then `/gsd-execute-phase 01 --ws hero-spotlight` ships Phase 01 (foundation
+ transition bug fix) and we proceed card-by-card from there.
