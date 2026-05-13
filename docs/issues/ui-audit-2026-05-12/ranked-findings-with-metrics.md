# Ranked open findings — combined across all audits with UXΔ / CDI / MVQ

**Compiled:** 2026-05-12
**Scope:** all OPEN findings as of 2026-05-12, across audits 2026-04-07, 2026-04-17, 2026-04-20, and 2026-05-12 (main + follow-up). Closed/verified-fixed items are excluded. Where multiple findings share a root cause, they're rolled into one batch with one metric set.

## Metric definitions (recap)

- **UXΔ — UX Delta Score** (signed, -5…+5): weighted mix of expected task-time change (40%), error-rate change (30%), and subjective-satisfaction change (30%). Bucket: **Better** (+1…+5), **Worse** (-5…-1), or **Ambiguous** (0 OR signals disagree).
- **CDI — Coherence Disruption Index** (0.0…5.0): `Distribution Spread × Coherence Shift`. Spread = touched_components / total_components (~500 frontend files + 30 services as denominator). Shift = 1 (extends existing pattern) → 5 (contradicts existing pattern).
- **MVQ — Mythic Vibe Quotient**: `Creature + Vibe Match % / Slop Resistance %`. Creature archetypes: **Phoenix** (transformative), **Griffin** (elegant hybrid), **Kraken** (massive scope), **Sprite** (small delightful), **Basilisk** (dangerous complexity), **Dragon** (ambitious centerpiece).

Scoring here is one-auditor judgment, not measured. Treat as **planning signal**, not data. UXΔ would be measurable post-ship by re-running the audit with the affected user segment.

---

## Tier A — Catastrophic (ship today)

### A1 · UA-115 — Grafana anonymous Admin on public internet
- **Severity:** 3 (catastrophic, security)
- **UXΔ:** **+1 (Ambiguous)** — for legitimate users: zero visible change (they were never supposed to see Grafana anonymously). For attackers: 100% access blocked. Sub-signals: task-time 0 (unchanged), error-rate +5 (eliminates exposure), satisfaction 0. Weighted = 1.5. Direction: Better in aggregate but **Ambiguous** because task-time signal is null.
- **CDI:** **0.002** (1 docker-compose env-var change in a system of ~500 files). Distribution Spread 1/500=0.002 × Shift 1 (no pattern change). Trivial.
- **MVQ:** **Phoenix 60%/100%** — rises-from-ashes (insecure→secure transformation) but the *code change* is microscopic, so the mythic match is partial. Slop resistance perfect (impossible to phone in an env-var flip).
- **Why ship first:** any breach window beats every other finding. 1-line env-var change + `make redeploy-grafana` + audit access logs.

### A2 · UA-065 — Profile API-key copy button has no accessible name
- **Severity:** 3 (catastrophic, a11y)
- **UXΔ:** **+2 (Better)** — for screen-reader users: feature goes from unusable to usable. Sub-signals: task-time +3 (the feature was infinite-time, now finite), error +4 (completely lifted), satisfaction +3. Weighted = +3.4. For sighted users: 0. Aggregate skewed by tiny affected population, hence +2 not +3.
- **CDI:** **0.002** — 1 LOC in Profile.vue + 1 locale key.
- **MVQ:** **Sprite 80%/95%** — small delightful polish, exactly what a Sprite does. High slop resistance because the right i18n key name and Russian translation requires intentional craft.

---

## Tier B — Quick wins (single PR, ≤ 50 LOC across ~8 files)

### B1 · i18n leaks batch — UA-043, UA-073, UA-050, UA-080, UA-067 hint copy
- **Findings rolled in:** Navbar "Open menu"/"Close menu" English literals (UA-043/UA-080), Locale switcher "ru" with no aria-label (UA-073), "Failed to fetch anime" English error string (UA-050), Profile-import URL-hint placeholder gap (UA-067).
- **Severity:** mostly 2 (major), 1 (minor)
- **UXΔ:** **+2 (Better)** — non-English-speaking screen-reader users currently get raw English announcements; localization brings them to parity. Sub-signals: task-time +1, error +3, satisfaction +2. Weighted = +2.0.
- **CDI:** **0.01** — 5 files (Navbar.vue, locale files ×3, Profile.vue, useAnime.ts) of ~500 = 0.01 × Shift 1.
- **MVQ:** **Sprite 90%/95%** — tiny, delightful, surgical. Highest slop resistance: each requires picking the right i18n key and writing 3 locale entries.

### B2 · Dynamic page titles — UA-051, UA-068
- **Findings:** Anime detail title still "Детали аниме - AnimeEnigma" (UA-051); Profile title still "Профиль - AnimeEnigma" not including username (UA-068).
- **Severity:** 1 (minor) each, but cumulative SEO + browser-history impact is real.
- **UXΔ:** **+1 (Better)** — task-time 0 in-app, but browser-history disambiguation +2, link-share preview +2, SEO long-term +2 weighted across many sessions.
- **CDI:** **0.012** — 1 composable (e.g. `useDocumentTitle`) + 2 view integrations + locale keys; ~6 files = 0.012.
- **MVQ:** **Sprite 85%/85%** — small polish; some risk of slop if the title pattern is just `${anime.title} - AnimeEnigma` with no thought to length/SEO.

### B3 · Single-LOC `aria-label` fixes — UA-042, UA-070, UA-071, UA-081
- **Findings:** Home `/schedule` icon-only mobile link (UA-042); Auth `<h1>` missing (UA-070); QR canvas no `aria-label` (UA-071); Navbar search-close button (UA-081).
- **Severity:** 2 (major) for QR + Auth h1; 1 (minor) for Home icon + search-close.
- **UXΔ:** **+2 (Better)** — for screen-reader users on the affected views: error rate drops, task-time drops, satisfaction unchanged (sighted users don't notice).
- **CDI:** **0.008** — 4 files of ~500 = 0.008 × Shift 1.
- **MVQ:** **Sprite 80%/90%** — pure polish across multiple surfaces. Slight slop risk only if someone copy-pastes the same generic aria-label.

### B4 · Tier-A-adjacent quick wins — UA-099, UA-055, UA-059, UA-067
- **Findings:** Recompute toast in AdminRecs (UA-099); drawer Schedule entry (UA-055); RecItem alt="" since title adjacent (UA-059); URL hint in import placeholders (UA-067).
- **Severity:** 1 each, except UA-055 is 1 minor.
- **UXΔ:** **+1 (Better)** — modest; mostly clarity polish.
- **CDI:** **0.008** — small footprint.
- **MVQ:** **Sprite 70%/85%** — solid polish work.

### B5 · Continue-Watching home row — Tier E #1
- **Severity:** N/A new feature; closes UA-061 (CW row absent on home for logged-in user).
- **UXΔ:** **+4 (Better)** — task-time -50% for resume flow (Home → directly resume vs Home → Profile → click anime → resume), error-rate 0, satisfaction +5. Weighted = +3.7.
- **CDI:** **0.015** — Home.vue + new composable `useContinueWatching` + locale keys; ~6 files = 0.012 × Shift 2 (new pattern that mirrors existing row infrastructure) = 0.024. Use 0.024.
- **MVQ:** **Phoenix 90%/85%** — genuinely transformative for the resume flow; high slop resistance because the empty/loading states need real thought.

---

## Tier C — Major non-trivial sweeps

### C1 · ARIA toggle/group sweep via `<ButtonGroup>` — UA-062, UA-063, UA-075, UA-078, UA-082
- **Findings:** Anime RU/EN switch (UA-062), Anime provider chips (UA-063), Themes type-filter (UA-075), Game answer-options (UA-078), Navbar mobile-lang toggle (UA-082).
- **Severity:** all 2 (major).
- **UXΔ:** **+3 (Better)** — five interactive surfaces fixed with one pattern. Screen-reader users gain the ability to perceive selected state across all of them. Sub-signals: task-time +1 (faster perception of active state), error-rate +4 (eliminates "I don't know which is selected"), satisfaction +3.
- **CDI:** **0.036** — new shared component (1 file) + 5 consumer refactors + maybe an update to existing chip styles; ~7 files of ~500 = 0.014 × Shift 3 (new but compatible pattern, requires consumers to migrate) = 0.042. Round to 0.04.
- **MVQ:** **Griffin 90%/95%** — elegant hybrid of a11y + reusability. Multiple components combined under one disciplined abstraction. Slop resistance very high because the design has to handle 5 different shapes (RU/EN/provider chips/filter/answer/lang).

### C2 · `text-white/40` contrast global sweep — UA-052, UA-066, UA-072, UA-074, UA-076, UA-077, UA-086, UA-121
- **Findings:** Anime detail residual (UA-052/121), Profile import-help (UA-066), Auth Telegram-Web summary (UA-072), Themes empty-state (UA-074), Schedule hint (UA-076), Game leaderboard rank (UA-077), Navbar search subtitle (UA-086).
- **Severity:** all 2 (major) by axe; cumulative impact on contrast-sensitive users + WCAG AA conformance.
- **UXΔ:** **+2 (Better)** — many surfaces, single mental model. Sub-signals: error-rate +3 (low-vision users can now read these strings), task-time 0, satisfaction +2.
- **CDI:** **0.018** — ~9 files touched with identical `/40` → `/60` swap; Spread 0.018 × Shift 1 (extends existing color discipline). Truly trivial pattern-wise.
- **MVQ:** **Sprite 75%/95%** — micro-polish across many places. Very slop-resistant because each swap requires verifying the text isn't actually decorative.

### C3 · Navbar drawer a11y — UA-053, UA-054, UA-083, UA-084, UA-112
- **Findings:** Drawer role=dialog + aria-modal (UA-053/UA-084); ESC close (UA-054); aria-expanded on hamburger (UA-083/UA-112); focus trap (UA-045 partial).
- **Severity:** all 2 (major).
- **UXΔ:** **+3 (Better)** — keyboard-only and screen-reader users currently cannot meaningfully use the mobile drawer. Sub-signals: error-rate +4, task-time +2, satisfaction +3.
- **CDI:** **0.014** — Navbar.vue + tiny focus-trap composable; ~3 files × Shift 2 (introduces dialog pattern not used elsewhere yet). = 0.012.
- **MVQ:** **Griffin 85%/90%** — elegant combination of multiple a11y patterns (dialog + focus trap + keyboard handling + state attribute). High slop resistance because each piece has subtle correctness requirements.

### C4 · `Input.vue` `v-bind="$attrs"` pass-through — UA-044
- **Severity:** 2 (major) — gates downstream aria-* attribute pass-through everywhere Input is used.
- **UXΔ:** **+2 (Better)** — modest aggregate, but unlocks future ergonomic improvements on every form input. Sub-signals: error-rate +1 today; +3 over the next 6 months as consumers start passing aria-labels.
- **CDI:** **0.008** — 1 component change. Risk of breakage on consumers relying on swallowed attrs; needs grep audit. Shift 2.
- **MVQ:** **Sprite 60%/80%** — small enabler; somewhat under-glamorous (no visible UX change) but unlocks futures.

### C5 · AdminRecs table semantics + keyboard — UA-094, UA-095, UA-093
- **Findings:** Table no caption / aria-label (UA-094); rows no keyboard handlers / tabindex (UA-095); S1-S5 column hardcoded non-i18n (UA-093).
- **Severity:** 2 (major) each.
- **UXΔ:** **+2 (Better)** for the (tiny) admin-user segment; near-zero impact on general users.
- **CDI:** **0.006** — 1 view (AdminRecs.vue) + locale keys. Shift 2.
- **MVQ:** **Griffin 75%/85%** — combines table semantics + keyboard + i18n. Slightly lower vibe match because the user impact is narrow.

### C6 · Browse heading + filter — UA-046, UA-047, UA-048
- **Findings:** Genre placeholder contrast (UA-046); GenreFilterPopup trigger missing `aria-haspopup` (UA-047); Browse heading-order h1→h3 (UA-048).
- **Severity:** 2/2/1.
- **UXΔ:** **+1 (Better)** — three small fixes, modest aggregate.
- **CDI:** **0.008** — 2 files (Browse.vue, GenreFilterPopup.vue).
- **MVQ:** **Sprite 70%/90%** — polish on existing surface; high slop resistance because the heading-order fix requires choosing the right semantic h2 text.

---

## Tier-bug · State-machine + data-sync fixes

### Bug1 · UA-110 — Resume state machine renders two contradictory banners
- **Severity:** 2 (major, UX bug).
- **UXΔ:** **+2 (Better)** — eliminates a confusion vector. Sub-signals: task-time 0 (users still find the resume CTA), error-rate +3 (no longer pick the wrong CTA), satisfaction +3 (less "what is this anime trying to tell me").
- **CDI:** **0.006** — 1 composable (`useResumeStateMachine.ts`) + 1 view (Anime.vue) bind tweak. Shift 1 (cap + switch, no new pattern).
- **MVQ:** **Sprite 80%/90%** — polish on logic. Slop resistance high because the off-by-one math + "use kind as switch not flags" requires real thought.

### Bug2 · UA-111 — `ui_audit_bot` seeded watch_history not mirrored to watch_progress
- **Severity:** 1 (minor, test infrastructure).
- **UXΔ:** **0 (Ambiguous)** — no end-user impact; only fixes test-data correctness so future audits can verify resume rendering. Re-frame as DevX, not UX.
- **CDI:** **0.004** — 1 shell script update (`scripts/seed-ui-audit-user.sh`).
- **MVQ:** **Sprite 60%/75%** — small polish, low ceremony. Some slop risk if the seed script just hardcodes 3 rows without matching the watch_history semantics.

---

## Tier D — Polish (1-pt severity batch)

Group all severity-1 cosmetic / polish findings:

UA-057 (Home pinned-rec English reason line — needs i18n key infrastructure), UA-058 (RecItem h3), UA-060 (top_contributor data verification), UA-069 (Profile tab aria-controls), UA-085 (drawer Schedule entry redundant with UA-055), UA-091 (admin picker aria-live), UA-092 (admin picker loading state), UA-096 (admin error-status mapping 401/500/timeout), UA-097 (admin empty-state help text), UA-098 (admin mobile scroll affordance), UA-101 (401/403 conflation in guard), UA-116 (Grafana empty Service Overview row), UA-117 (Grafana row numbering inconsistency), UA-118 (Grafana dashboard naming inconsistency), UA-119 (Grafana SPA cold-load blank — partly upstream).

- **Severity:** 1 each. As a batch.
- **UXΔ:** **+1 (Better)** aggregate — each item is small but the sum lifts perceived polish meaningfully.
- **CDI:** **0.04** — 12-15 files across the codebase + Grafana dashboard JSON edits. Spread ≈ 0.025 × Shift 1.
- **MVQ:** **Sprite 70%/70%** — true polish work. Slop risk medium because polish is the easiest category to phone in.

---

## Tier E — Strategic / competitive (drawn from competitive benchmark)

Each gets its own metrics. Sort by UXΔ desc.

### E1 · Continue-Watching home row — *covered in B5 above*
- See B5 above; this is intentionally moved to Tier B to ship sooner.

### E2 · Per-card progress badge ("Х серия" / "1-X из Y+")
- **UXΔ:** **+3 (Better)** — scan-affordance lift on every grid view. Task-time +3 (instantly see resume state), error-rate +2, satisfaction +3.
- **CDI:** **0.06** — touches every card-rendering surface (RecItem.vue + browse card + search card + Home rows); ~10 files × Shift 3 (new card-level data pipeline). = 0.06.
- **MVQ:** **Griffin 85%/85%** — combines watch_progress data with rendering on already-existing cards.

### E3 · Multi-axis catalog filter sidebar (with provider-as-filter)
- **UXΔ:** **+4 (Ambiguous)** — huge task-time improvement for power users (+5); modest improvement for casual users (+1); short-term learning curve adds error rate (-1). Direction Ambiguous despite high aggregate.
- **CDI:** **0.18** — Browse.vue rebuild + new filter components + URL state management + backend filter params + locale strings; ~15 files × Shift 4 (replaces an existing limited filter UI with a substantially different pattern). = 0.18.
- **MVQ:** **Dragon 90%/85%** — ambitious, showy, centerpiece. Provider-as-filter is uniquely AnimeEnigma's competitive moat — no other site has it.

### E4 · Broadcast schedule view (`/schedule` + home row)
- **UXΔ:** **+2 (Better)** — adds a discovery surface; modest impact since this is "nice to have" not core flow.
- **CDI:** **0.10** — new view + new home row + Shikimori `nextEpisodeAt` ingestion + locale strings; ~10 files × Shift 3.
- **MVQ:** **Phoenix 75%/80%** — transformative for the seasonal discovery pattern; medium slop resistance since "broadcast schedule" can be cheap or rich depending on effort.

### E5 · Editorial collections (`Подборки`)
- **UXΔ:** **+2 (Better)** — admin-curated content surfaces tend to drive 10-30% more clicks than algorithmic-only.
- **CDI:** **0.10** — new admin curation tool + home row + DB schema (collections table) + locale strings.
- **MVQ:** **Dragon 80%/75%** — ambitious-content-led, but slop-resistant only if the admin tooling is built well; risk of becoming "just another row".

### E6 · Sort dropdown on browse
- **UXΔ:** **+2 (Better)** — cheap parity win with animejoy's 5 axes.
- **CDI:** **0.04** — 1 dropdown + backend sort params; ~4 files × Shift 2.
- **MVQ:** **Sprite 80%/85%** — small but high-value polish.

### E7 · Detail-page Quick-Navigation anchor menu
- **UXΔ:** **+2 (Better)** — long detail page becomes scannable; medium aggregate.
- **CDI:** **0.03** — Anime.vue + 1 new component; ~3 files × Shift 2.
- **MVQ:** **Sprite 75%/80%** — animevost-pattern, well-trodden.

### E8 · Theater mode on player views
- **UXΔ:** **+2 (Better)** — useful for HiAnime/Consumet HLS where fullscreen can be aggressive.
- **CDI:** **0.06** — player views + shared toggle state; ~6 files × Shift 2.
- **MVQ:** **Sprite 70%/75%** — small useful polish; some slop risk because "collapse the nav" can be either subtle or jarring.

### E9 · Optimistic UI on watchlist actions
- **UXΔ:** **+2 (Better)** — eliminates perceived API latency on every status pill click.
- **CDI:** **0.05** — watchlist store + status-pill components + rollback logic; ~5 files × Shift 3 (introduces optimistic pattern across the app for the first time).
- **MVQ:** **Phoenix 80%/85%** — transformative for perceived speed; high slop resistance because the rollback path on API failure has to feel intentional (not a janky flash).

### E10 · "Because you watched X" reasoning chip
- **UXΔ:** **+3 (Better)** — closes one of the biggest personalization gaps vs Crunchyroll; gives users insight into why they're seeing each rec.
- **CDI:** **0.07** — RecItem.vue + rec engine output schema + locale strings; ~7 files × Shift 3 (extends rec engine API).
- **MVQ:** **Griffin 85%/90%** — elegant hybrid of personalization + transparency.

### E11 · Single-player abstraction
- **UXΔ:** **+4 (Better)** long-term — consolidating 4-5 players to 1 reduces split-brain UX. Task-time +3 (consistent controls), error-rate +4 (no per-player surprises), satisfaction +3.
- **CDI:** **0.50** — touches all 5 player components + scraper layer + parser orchestration; ~20 files × Shift 5 (replaces existing pattern with new one). **Highest CDI in the report.**
- **MVQ:** **Phoenix 95%/85%** — the canonical transformation. Already in-flight as Phase 16. High slop resistance because each migration has to preserve feature parity.

### E12 · Grafana dashboard rebuild
- **UXΔ:** **+1 (Better)** for the admin segment only.
- **CDI:** **0.05** — Grafana dashboard JSON edits; deploy via provisioning. ~5 dashboard files × Shift 2.
- **MVQ:** **Kraken 60%/55%** — massive scope (7 dashboards × many panels) but easy to phone in; slop risk high because dashboard work is the most copy-paste-able UX work imaginable.

### E13 · Skip-Intro detection
- **UXΔ:** **+3 (Better)** for HiAnime/Consumet users — saves 90s per episode × 10 episodes/week × thousands of sessions.
- **CDI:** **0.08** — aniskip.com client + player overlay + storage + locale strings; ~8 files × Shift 3.
- **MVQ:** **Griffin 80%/85%** — elegant hybrid of external data + player overlay.

### E14 · Numbered Top-10 rank treatment (Netflix-iconic)
- **UXΔ:** **+1 (Better)** — visual lift on existing content row; modest aggregate.
- **CDI:** **0.02** — RecItem.vue or new TopTenItem.vue + Home.vue integration; ~3 files × Shift 2.
- **MVQ:** **Dragon 75%/65%** — wants to be Netflix-showy but the implementation can be cheap; medium slop risk because the giant-numeral treatment requires real typography decisions.

### E15 · System-status banner on home (when active)
- **UXΔ:** **+1 (Better)** for users during incidents; 0 otherwise.
- **CDI:** **0.03** — new component + AUTO-NNN status integration; ~3 files × Shift 2.
- **MVQ:** **Sprite 70%/80%** — small but well-targeted polish.

### E16 · Episode-level granularity in "Latest episodes" row
- **UXΔ:** **+2 (Better)** — surfaces specific new episodes as direct watch links; reduces clicks-to-play.
- **CDI:** **0.05** — Home.vue + RecItem.vue variants + rec engine response shape; ~5 files × Shift 3.
- **MVQ:** **Sprite 80%/85%** — 9anime pattern, well-defined.

### E17 · Sub/Dub indicator on episode cards
- **UXΔ:** **+2 (Better)** — surfaces existing translation data at card granularity.
- **CDI:** **0.04** — card components + translation data plumbing; ~4 files × Shift 2.
- **MVQ:** **Sprite 75%/85%** — small lift on existing data.

### E18 · Soft-social proof: follower count on detail page
- **UXΔ:** **+1 (Better)** — quiet social proof, no friction.
- **CDI:** **0.03** — Anime.vue + 1 query against `anime_list`; ~3 files × Shift 1.
- **MVQ:** **Sprite 70%/75%** — small polish; slop risk if it just becomes a vanity counter without anti-bot considerations.

### E19 · Search-scope clarity (placeholder text)
- **UXΔ:** **+1 (Better)** — tiny but cheap.
- **CDI:** **0.004** — locale string update only.
- **MVQ:** **Sprite 60%/85%** — pure polish.

### E20 · FAQ accordion on public marketing surface
- **UXΔ:** **+1 (Better)** — reduces support load; modest direct UX impact.
- **CDI:** **0.05** — new component + content + locale; ~5 files × Shift 2.
- **MVQ:** **Sprite 60%/65%** — slop risk medium because FAQ content can be lazy.

---

## Combined ranking (top 25 by overall priority)

Priority score (informal): `severity × 2 + UXΔ - CDI × 5 + MVQ_match/20`. Lower CDI penalty per priority point gained, higher MVQ resistance to slop preferred.

| Rank | Item | Severity | UXΔ | CDI | MVQ | Notes |
|---|---|---|---|---|---|---|
| 1 | A1 · UA-115 Grafana anonymous Admin | 3 | +1 Ambig | 0.002 | Phoenix 60/100 | Security imperative dwarfs UX score |
| 2 | A2 · UA-065 API-key copy button name | 3 | +2 Better | 0.002 | Sprite 80/95 | 1-LOC; ship immediately after A1 |
| 3 | B5 · Continue-Watching home row | new | +4 Better | 0.024 | Phoenix 90/85 | Largest single-feature UX lift |
| 4 | E11 · Single-player abstraction (in flight) | new | +4 Better | 0.50 | Phoenix 95/85 | Highest CDI in the report, but Phase 16 is already going |
| 5 | C1 · ARIA toggle/group ButtonGroup | 2 ×5 | +3 Better | 0.04 | Griffin 90/95 | Single pattern unblocks 5 surfaces |
| 6 | E10 · "Because you watched X" reasoning | new | +3 Better | 0.07 | Griffin 85/90 | Closes biggest personalization gap |
| 7 | C3 · Navbar drawer a11y | 2 ×5 | +3 Better | 0.012 | Griffin 85/90 | Mobile-keyboard-user blocker |
| 8 | E13 · Skip-Intro detection | new | +3 Better | 0.08 | Griffin 80/85 | Competitive parity move |
| 9 | B1 · i18n leaks batch | 2 ×4 | +2 Better | 0.01 | Sprite 90/95 | Cheap; restores RU/EN/JA discipline |
| 10 | B3 · single-LOC aria-labels | 2 ×4 | +2 Better | 0.008 | Sprite 80/90 | Cheap; covers multiple surfaces |
| 11 | Bug1 · UA-110 Resume state machine | 2 | +2 Better | 0.006 | Sprite 80/90 | Eliminates UI contradiction |
| 12 | E2 · Per-card progress badge | new | +3 Better | 0.06 | Griffin 85/85 | Industry-standard, missing here |
| 13 | C2 · text-white/40 contrast sweep | 2 ×8 | +2 Better | 0.018 | Sprite 75/95 | One mental model, many surfaces |
| 14 | E9 · Optimistic UI on watchlist | new | +2 Better | 0.05 | Phoenix 80/85 | Mitigates Crunchyroll-class pain point |
| 15 | B2 · Dynamic page titles | 1 ×2 | +1 Better | 0.012 | Sprite 85/85 | SEO + browser history |
| 16 | C6 · Browse heading + filter | 2 ×3 | +1 Better | 0.008 | Sprite 70/90 | Carry-over from 2026-04-20 |
| 17 | C5 · AdminRecs table semantics | 2 ×3 | +2 Better | 0.006 | Griffin 75/85 | Narrow audience, but admin a11y matters |
| 18 | E3 · Multi-axis catalog filter sidebar | new | +4 Ambig | 0.18 | Dragon 90/85 | Ambitious; provider-as-filter is moat |
| 19 | E6 · Sort dropdown on browse | new | +2 Better | 0.04 | Sprite 80/85 | Cheap parity win |
| 20 | C4 · Input.vue v-bind=$attrs | 2 | +2 Better | 0.008 | Sprite 60/80 | Unlocks downstream futures |
| 21 | E4 · Broadcast schedule view | new | +2 Better | 0.10 | Phoenix 75/80 | Discovery surface |
| 22 | E7 · Detail-page Quick-Nav menu | new | +2 Better | 0.03 | Sprite 75/80 | Long detail pages become scannable |
| 23 | E16 · Episode-level "Latest episodes" | new | +2 Better | 0.05 | Sprite 80/85 | 9anime pattern |
| 24 | E5 · Editorial collections | new | +2 Better | 0.10 | Dragon 80/75 | Admin tooling investment |
| 25 | E17 · Sub/Dub on episode cards | new | +2 Better | 0.04 | Sprite 75/85 | Surfaces existing data |

(Tier D polish batch + remaining E items + Bug2 land below the cutoff.)

## Aggregate observations

- **Phoenix-class** items (transformative): A1, A2, B5, E4, E9, E11 — six total. These are the items that move the platform's character, not just trim its edges. Cluster them in one quarter to deliver a clear narrative ("AnimeEnigma got smarter").
- **Griffin-class** items (elegant hybrids): C1, C3, C5, E2, E10, E13 — six total. These are the disciplined-craft wins; ship them together to raise the a11y/UX bar uniformly.
- **Sprite-class** items dominate counts but each delivers little. Treat them as **two coordinated polish PRs** rather than 20 individual ones.
- **Dragon-class** items (Multi-axis filter, Top-10 treatment, Collections) are scope-heavy and should each be their own initiative — don't bundle.
- **Kraken** appears once (Grafana rebuild). It's the easiest to phone in; assign explicit ownership or skip.
- **Basilisk** doesn't appear among our **fixes** — only among our **problems** (UA-115 itself). That's a good sign: nothing we're about to build is "dangerous complexity we should look at sideways". The one true Basilisk in the system is being fixed by killing its env var.

## Slop watch

Items most at risk of being phoned-in if not given craft attention:
1. **E12 Grafana rebuild** — Kraken-class, copy-paste-friendly. Assign or skip.
2. **D polish batch** — easy to ship surface-level fixes that miss deeper consistency. Pair with a design-review checkpoint.
3. **E14 Numbered Top-10 visual** — Netflix-iconic implies real typography work; if shipped as "just add a number," the Dragon vibe collapses.
4. **E20 FAQ accordion** — content quality is the entire feature; ship only when content is curated, not just because the component exists.

The MVQ doesn't replace user research; it's a craft-vigilance flag. Treat anything with **Slop Resistance < 75%** as needing a design check before merge.
