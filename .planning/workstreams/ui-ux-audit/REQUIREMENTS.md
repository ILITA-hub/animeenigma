# Requirements: AnimeEnigma `ui-ux-audit` workstream

**Milestone:** v0.1 UX Reassessment Remediation
**Defined:** 2026-05-13
**Core value:** Close every open finding from the 2026-04-07 / 04-17 / 04-20 / 05-12 audits while keeping AnimeEnigma's competitive strengths (RU/EN/JA i18n, multi-source video, rec_click instrumentation, responsive design) and extending them with the Tier E strategic gaps surfaced by the competitive benchmark (continue-watching row, per-card progress, multi-axis filters, schedule view).
**Source documents:**
- `docs/issues/ui-audit-2026-05-12.md` (master report + tiered plan)
- `docs/issues/ui-audit-2026-05-12/ranked-findings-with-metrics.md` (priority order, UXΔ/CDI/MVQ)
- `docs/issues/ui-audit-2026-05-12/competitive-benchmark.md` (Tier E rationale)

## v0.1 Requirements

Each requirement is a phase-level outcome. UA-NNN codes are the original audit finding identifiers; multiple UA-NNN can roll up into one UX-NN.

### Tier A — Catastrophic (Phase 1)

- [ ] **UX-01** — Grafana anonymous Admin access on the public internet disabled; access logs audited for last 30 days. Closes **UA-115**.
- [ ] **UX-02** — Profile API-key copy button exposes accessible name in all three locales. Closes **UA-065**.

### Tier B — Quick wins (Phase 2)

- [ ] **UX-03** — i18n leak batch closed: Navbar hamburger labels, locale-switcher button, "Failed to fetch anime" error all routed through `$t()`. Closes **UA-043 / UA-073 / UA-050 / UA-080**.
- [ ] **UX-04** — Dynamic `<title>` populated with anime name on `/anime/:id` and with username on `/user/:public_id`. Closes **UA-051 / UA-068**.
- [ ] **UX-05** — Aria-label batch: Home `/schedule` icon link, Auth `<h1>` sr-only, QR canvas `role="img"` + label, Navbar search-close button, AdminRecs recompute toast. Closes **UA-042 / UA-070 / UA-071 / UA-081 / UA-099**.
- [ ] **UX-06** — Tier-A-adjacent quick wins: drawer Schedule entry, RecItem `alt=""`, URL hint in import placeholders, drawer Schedule entry redundancy. Closes **UA-055 / UA-059 / UA-067 / UA-085**.

### Bug fixes (Phase 3)

- [ ] **UX-07** — Resume state machine no longer renders two contradictory banners. `lastWatched` capped at `totalEpisodes`; single banner driven by `kind` switch. Closes **UA-110**.
- [ ] **UX-08** — `ui_audit_bot` seed script mirrors `watch_history` rows into `watch_progress` so resume banners render on seeded data. Closes **UA-111**.
- [ ] **UX-09** — Pinned-rec reason line localized via `pin_reason_key` (RU/EN/JA dictionaries seeded). Closes **UA-057**.

### Tier C — Major sweeps

- [ ] **UX-10** — `text-white/40` global contrast sweep across 9 surfaces (`/40` → `/60` where text carries meaning). Closes **UA-052 / UA-066 / UA-072 / UA-074 / UA-076 / UA-077 / UA-086 / UA-121**. (Phase 4)
- [ ] **UX-11** — Browse view: genre-placeholder contrast, GenreFilterPopup `aria-haspopup` + `aria-expanded`, sr-only h2 to fix heading order. Closes **UA-046 / UA-047 / UA-048**. (Phase 4)
- [ ] **UX-12** — Reusable `<ButtonGroup>` component (`role="group"` + `aria-pressed`) introduced; five surface migrations (Anime RU/EN switch, provider chips, Themes type-filter, Game answer-options, Navbar mobile-lang toggle). Closes **UA-062 / UA-063 / UA-075 / UA-078 / UA-082**. Adds **UA-069** Profile tab `aria-controls` as a bonus. (Phase 5)
- [ ] **UX-13** — Navbar mobile drawer a11y: `role="dialog"`, `aria-modal`, focus trap, ESC handler, `aria-expanded` state on hamburger. Closes **UA-053 / UA-054 / UA-083 / UA-084 / UA-112 / UA-045**. (Phase 6)
- [ ] **UX-14** — `Input.vue` passes `$attrs` to inner input so aria-* attributes propagate through; consumers audited. Closes **UA-044**. Also fixes **UA-058** RecItem h3 element. (Phase 7)

### Continue-Watching (Phoenix new feature)

- [ ] **UX-15** — Logged-in Home view renders a Continue-Watching row above existing rows when `watch_progress` has uncompleted episodes. Empty/loading states defined. Closes **UA-061**, addresses **Tier E #1**. (Phase 8)

### Card-data exposure (Phase 9)

- [ ] **UX-16** — Per-card progress badge (`Х серия` / `1-X из Y+`) rendered on RecItem, Browse cards, Search cards for in-progress titles. Tier E #2.
- [ ] **UX-17** — "Latest episodes" row links directly to the new episode (player at episode N), not just the anime detail page. Tier E #16.
- [ ] **UX-18** — Sub/Dub indicator badge on episode cards/rows sourced from existing Kodik translation data. Tier E #17.

### Recommendations polish (Phase 10)

- [ ] **UX-19** — "Because you watched X" reasoning chip on every personalized rec card; pipes through existing `rec_click` instrumentation. Tier E #10. Also addresses **UA-060** top_contributor visibility verification.
- [ ] **UX-20** — Numbered Top-10 visual treatment (giant-numeral-behind-poster) on the "Топ аниме" row. Tier E #14.

### Catalog browse + Detail expansion (Phase 11)

- [ ] **UX-21** — Sort dropdown on `/browse` exposing 5 axes (popularity / rating / year / recently-updated / A-Z). Tier E #6.
- [ ] **UX-22** — Detail-page Quick-Navigation anchor menu (sticky TOC: poster → описание → серии → похожее → комментарии). Tier E #7.
- [ ] **UX-23** — Theater mode toggle on player views (collapses navbar/sidebar, max-width player without entering fullscreen). Tier E #8.
- [ ] **UX-24** — System-status banner on Home (renders only when there's an active incident sourced from AUTO-NNN). Tier E #15.

### AdminRecs SPA (Phase 12)

- [ ] **UX-25** — AdminRecs table gains caption, aria-label, keyboard handlers, `aria-expanded` on expandable rows. Closes **UA-094 / UA-095 / UA-093**.
- [ ] **UX-26** — AdminRecsPicker focus management + loading indicator; admin error mapping (401/500/timeout) → friendly i18n keys; empty-state help text; mobile horizontal-scroll affordance. Closes **UA-090 / UA-091 / UA-092 / UA-096 / UA-097 / UA-098 / UA-100 / UA-101**.

### Optimistic UI (Phase 13)

- [ ] **UX-27** — Watchlist actions (status pill flip, score change, list add/remove) feel instant: optimistic UI flips state before API confirms, rolls back on error with toast. Tier E #9.

### Marketing-surface polish (Phase 14)

- [x] **UX-28** — Soft social proof: follower count on detail page derived from `anime_list` rows with status='watching'. Tier E #18.
- [x] **UX-29** — Search-scope clarity: placeholder text disambiguates ("Поиск: название или жанр"). Tier E #19.
- [x] **UX-30** — FAQ accordion on a public marketing surface (`/о-сервисе` or similar) with curated content. Tier E #20.

### Multi-axis catalog filter (Phase 15 — Dragon)

- [ ] **UX-31** — `/browse` filter UI rebuilt as a persistent sidebar exposing genre + format + status + year + **provider/audio-source (Kodik/AnimeLib/HiAnime/Consumet)** + sort. URL-state-persisted. Tier E #3 — uniquely AnimeEnigma's competitive moat.

### Broadcast schedule (Phase 16 — Phoenix)

- [ ] **UX-32** — New `/schedule` route + Home "На этой неделе" row showing today/tomorrow airing episodes by hour. Sourced from Shikimori `nextEpisodeAt`. Tier E #4.

### Editorial collections (Phase 17 — Dragon)

- [ ] **UX-33** — Admin-curated `Подборки` (Collections) tool + Home row distinct from algorithmic recs. New DB schema (`collections` + `collection_items`). Tier E #5.

### Skip-Intro detection (Phase 18 — Griffin)

- [ ] **UX-34** — Skip-Intro CTA on HiAnime/Consumet players using aniskip.com timestamps. Pairs with Phase 16 single-player work in root milestone. Tier E #13.

### Grafana dashboard rebuild (Phase 19 — Kraken)

- [ ] **UX-35** — Grafana dashboards renamed for consistent prefix discipline; empty rows removed; row numbering normalized; panel-type appropriateness verified (Time-series vs Stat); time-range defaults standardized. Closes **UA-116 / UA-117 / UA-118 / UA-119 / UA-120**. Tier E #12.

### Polish batch (Phase 20 — Tier D)

- [ ] **UX-36** — All remaining severity-1 cosmetic findings closed in one polish PR: **UA-058 / UA-060 / UA-069 / UA-085 / UA-116** (any Grafana items not fully done in Phase 19) and any new minor surfaces discovered during prior phases. Pair with a design-review checkpoint before merge.

## Non-functional

- [ ] **UX-NF-01** — Every phase's PR triggers `make redeploy-<service>` for any service it touches and updates `frontend/web/public/changelog.json` via the existing `animeenigma-after-update` hook. No phase completes without redeploy + verification of the affected surface.
- [ ] **UX-NF-02** — Every UI change ships with RU/EN/JA locale entries (no untranslated strings reach production). CI lint catches missing keys.
- [ ] **UX-NF-03** — axe-core re-run after each phase confirms no regression on any previously-clean view (Profile, Themes, Schedule, Game maintain zero violations).
- [ ] **UX-NF-04** — Tier-E strategic phases (15, 16, 17, 18) each include a follow-up mini-audit (`docs/issues/ui-audit-followup-{phase}.md`) verifying the new surface meets the same axe + Nielsen bar.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Single-player abstraction (E11) | Already in flight as Phase 16/18 in root v3.0 milestone. This workstream coordinates with it (Phase 18 Skip-Intro depends on consolidated player) but does not duplicate the work. |
| New OAuth providers | Auth (`/auth`) audit only covered the existing Telegram path; no new providers in scope. |
| New video providers | Kodik/AnimeLib/HiAnime/Consumet are the four players this workstream styles; new providers are root-milestone work. |
| Backend rec engine changes | The `rec_click` instrumentation and pin_source/top_contributor data come from `services/player/internal/recs.go`; this workstream only renders them. |
| Notifications engine | See project memory `project_notifications_engine.md` — separate workstream / phase candidate. |
| Crunchyroll geo-block workaround | Crunchyroll's geo-block stays out of scope; competitive benchmark uses public UX case studies for that column. |
