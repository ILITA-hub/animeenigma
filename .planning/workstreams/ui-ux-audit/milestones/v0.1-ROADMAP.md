# Roadmap: AnimeEnigma `ui-ux-audit` workstream

**Workstream:** ui-ux-audit (parallel to root v3.0 Universal Anime Scraper + parallel to social workstream)
**Active milestone:** v0.1 UX Reassessment Remediation
**Phase numbering:** Workstream-local — starts at 1, independent of root project and `social` workstream numbering.
**Autonomous mode:** All phases below are designed to run cleanly under `/gsd-autonomous --ws ui-ux-audit`. Dependencies are explicit per phase. Phases 1–7 (Tier A → C + bug fixes) can ship first; Tier E phases (8+) extend the platform.

## Milestones

- 🟢 **v0.1 UX Reassessment Remediation** — Phases 1–20 (planning)

## Phases

### Phase 1: Tier A — Catastrophic fixes (security + a11y)

**Goal:** Eliminate the two catastrophic findings from the 2026-05-12 audit: Grafana anonymous Admin exposure (security) and Profile API-key copy button with no accessible name (a11y). Ship today.

**Depends on:** Nothing.

**Requirements:** UX-01, UX-02

**Success Criteria:**
1. `GET https://animeenigma.ru/admin/grafana/dashboards` returns a redirect to `/login` for unauthenticated requests; `docker-compose.yml` has `GF_AUTH_ANONYMOUS_ENABLED: "false"`; service redeployed (`make redeploy-grafana`).
2. Grafana access logs for the prior 30 days reviewed; findings (if any) appended to `docs/issues/ui-audit-2026-05-12/followup-session.md` UA-115 section.
3. `frontend/web/src/views/Profile.vue` API-key copy button has `:aria-label="$t('profile.apiKey.copy')"` and the three locale files have a `profile.apiKey.copy` entry.
4. axe-core re-run on `/user/ui-audit-bot` Settings tab shows zero `button-name` violations.

**SPEC:** `phases/01-tier-a-catastrophic/01-SPEC.md` (to be produced by `/gsd-spec-phase 1 --ws ui-ux-audit`)
**UI hint:** no (env-var + 1-LOC fix)

---

### Phase 2: Tier B — Quick-wins batch

**Goal:** Close ~13 small findings in one PR (~50 LOC across ~8 files). Restores most of the 2026-04-20 expected delta that didn't ship as Batches G/H/I.

**Depends on:** Phase 1.

**Requirements:** UX-03, UX-04, UX-05, UX-06

**Success Criteria:**
1. No literal `"Open menu"` / `"Close menu"` / `"Failed to fetch anime"` in `frontend/web/src/` (grep returns 0); all routed through `$t()`.
2. `<title>` on `/anime/:id` includes the anime name; `<title>` on `/user/:public_id` includes the username.
3. Schedule icon link, Auth h1, QR canvas, Navbar search-close, AdminRecs recompute button all have accessible names verified by axe.
4. Drawer Schedule entry present; RecItem image `alt=""`; import placeholders mention URL acceptance.

**SPEC:** `phases/02-tier-b-quick-wins/02-SPEC.md`
**UI hint:** yes (touches multiple frontend views)

---

### Phase 3: Bug fixes — resume state machine + seed-data sync + pinned-rec localization

**Goal:** Eliminate the resume-banner contradiction bug, sync seeded watch_history to watch_progress, and route pinned-rec reason lines through i18n.

**Depends on:** Phase 1 (only because Phase 1 is the absolute first ship).

**Requirements:** UX-07, UX-08, UX-09

**Success Criteria:**
1. On any anime where `lastWatched > totalEpisodes`, the page renders exactly one banner (either "finished" or "rewatch") — never both. Probed live on at least one seeded anime.
2. `scripts/seed-ui-audit-user.sh` populates `watch_progress` rows matching each `watch_history` row for `ui_audit_bot`. After re-running the seed, `/api/users/progress/{animeId}` returns non-empty data for the seeded anime.
3. Pinned-rec reason line displays in Russian when locale is RU; structured as `pin_reason_key` lookup with English fallback.

**SPEC:** `phases/03-bug-fixes/03-SPEC.md`
**UI hint:** light (resume banner copy change)

---

### Phase 4: Color-contrast + Browse heading sweep

**Goal:** Replace `text-white/40` with `/60` where text carries meaning (≈9 surfaces); fix Browse genre placeholder contrast + heading-order + GenreFilterPopup ARIA semantics.

**Depends on:** Phase 1.

**Requirements:** UX-10, UX-11

**Success Criteria:**
1. axe-core color-contrast violations drop to zero on Anime detail, Profile-settings, Themes, Schedule, Game, Auth (Telegram-Web summary), Navbar search subtitle.
2. axe-core heading-order on `/browse` returns zero violations.
3. GenreFilterPopup trigger button has `aria-haspopup="listbox"` + `aria-expanded` bound to the open state.

**SPEC:** `phases/04-contrast-and-browse-sweep/04-SPEC.md`
**UI hint:** light (CSS class swaps)

---

### Phase 5: `<ButtonGroup>` unification — 5 ARIA toggle surfaces

**Goal:** Introduce a shared `<ButtonGroup>` component (`role="group"` + `aria-pressed`) and migrate 5 existing surfaces. Single pattern fixes 5 findings.

**Depends on:** Phase 1.

**Requirements:** UX-12 (and the bonus UA-069 Profile tab `aria-controls`)

**Success Criteria:**
1. `frontend/web/src/components/ui/ButtonGroup.vue` exists with `role="group"` + `aria-pressed` semantics, has unit tests.
2. Anime RU/EN switch, Anime provider chips, Themes type-filter, Game answer-options, and Navbar mobile-lang toggle all consume `<ButtonGroup>`.
3. Profile tabs (`Мои списки` / `Настройки`) gain `aria-controls` pointing to the panel `id`.
4. axe-core re-runs on all five surfaces show zero a11y violations for the ARIA-toggle pattern.

**SPEC:** `phases/05-buttongroup-unification/05-SPEC.md`
**UI hint:** yes (visual selected-state styling should be consistent)

---

### Phase 6: Navbar drawer a11y

**Goal:** Make the mobile drawer keyboard- and screen-reader-usable: `role="dialog"`, `aria-modal`, focus trap, ESC handler, `aria-expanded` state.

**Depends on:** Phase 1.

**Requirements:** UX-13

**Success Criteria:**
1. Opening the drawer adds `aria-expanded="true"` to the hamburger button; closing it sets `aria-expanded="false"`.
2. Drawer `<div>` has `role="dialog" aria-modal="true" aria-label="..."`.
3. ESC closes the drawer; focus returns to the hamburger button after close.
4. Tab key cycles only within the drawer when open (focus trap).

**SPEC:** `phases/06-navbar-drawer-a11y/06-SPEC.md`
**UI hint:** light (no new visuals, behavior fixes)

---

### Phase 7: `Input.vue` `$attrs` pass-through + RecItem h3

**Goal:** Unblock downstream aria-* attribute pass-through on every form input by adding `v-bind="$attrs"` to `Input.vue`. Convert RecItem title to `<h3>` to fix heading-order on Home rec rows.

**Depends on:** Phase 1.

**Requirements:** UX-14

**Success Criteria:**
1. `Input.vue` template binds `$attrs` on the inner `<input>`; consumers passing `aria-label="..."` now reach the inner element.
2. RecItem renders title inside `<h3 class="...">` consistent with other anime-card rows on Home.
3. No regression on any existing Input consumer (grep audit completed; consumers explicitly relying on attr-swallowing are noted in PR).

**SPEC:** `phases/07-input-attrs-recitem-h3/07-SPEC.md`
**UI hint:** no

---

### Phase 8: Continue-Watching home row (Phoenix new feature)

**Goal:** New row on Home for logged-in users with uncompleted episodes, sourced from `watch_progress`. The single largest UX delta in the audit.

**Depends on:** Phase 3 (UX-08 seed-data fix so the row populates on test users).

**Requirements:** UX-15

**Success Criteria:**
1. Logged-in user with ≥ 1 row in `watch_progress` where `completed=false` sees a "Продолжить просмотр" row at the top of Home (above "Подобрано для вас").
2. Cards in the row link directly to the anime player on the resume episode.
3. Anonymous users do not see the row.
4. Empty state (user logged in but no in-progress titles): row is hidden, no skeleton flicker.
5. Loading state: skeleton row matches existing rec-row skeleton pattern.
6. axe-core: zero new violations on Home.

**SPEC:** `phases/08-continue-watching-row/08-SPEC.md`
**UI hint:** yes (Phoenix-class new surface)

---

### Phase 9: Per-card progress + Sub/Dub indicators + Episode-granular row

**Goal:** Surface watch-progress, sub/dub, and specific-new-episode data at card granularity. Batches three related Tier-E items because they all touch the same card-render surfaces.

**Depends on:** Phase 8 (uses the same watch_progress pipeline).

**Requirements:** UX-16, UX-17, UX-18

**Success Criteria:**
1. Anime cards on Home, Browse, Search, and Profile-watchlist render a progress badge (`X серия` or `1-X из Y+`) when the user has progress on that anime.
2. "Latest episodes" row links go to `/anime/:id?episode=N&autoplay=1` instead of just `/anime/:id`.
3. Episode cards/rows in the "Latest episodes" surface display Sub/Dub badge from existing Kodik multi-translation data.
4. axe-core: zero new violations; per-card badges have semantic accessible text.

**SPEC:** `phases/09-card-progress-subdub-episodes/09-SPEC.md`
**UI hint:** yes

---

### Phase 10: Recommendations polish — reasoning chip + Top-10 visual ✅ shipped 2026-05-13

**Goal:** Close the personalization-explanation gap with Crunchyroll and add Netflix-iconic Top-10 visual treatment.

**Depends on:** Phase 1 (only the rec API output schema extension).

**Requirements:** UX-19, UX-20 (also addresses UA-060 top_contributor visibility)

**Success Criteria:**
1. Every personalized rec card carries a reasoning chip (`Похоже на Frieren` / `Жанр: Фэнтези` / `Топ-донатор: tNeymik`) localized to the active locale.
2. The "Топ аниме" row uses giant-numeral-behind-poster visual treatment with consistent typography. Rank 1-10 visible without horizontal scroll on desktop.
3. `rec_click` instrumentation continues to fire with reason chip metadata.

**SPEC:** `phases/10-recs-polish-reasoning-top10/10-SPEC.md`
**UI hint:** yes (Top-10 typography is design-load)

---

### Phase 11: Catalog browse + detail polish — sort, Quick-Nav, Theater mode, status banner

**Goal:** Four small Tier-E items batched: sort dropdown on browse, sticky Quick-Navigation menu on Anime detail, Theater mode toggle on player views, system-status banner on Home (active-incident only).

**Depends on:** Phase 1, Phase 4 (heading order fixes).

**Requirements:** UX-21, UX-22, UX-23, UX-24

**Success Criteria:**
1. `/browse?sort=popularity|score|year|recent|alpha` returns correctly ordered results; UI dropdown reflects active sort; URL state persistent.
2. Anime detail page has a sticky TOC (`Описание | Серии | Связанные | Отзывы`) that scrolls to anchors and stays visible while scrolling.
3. Theater-mode toggle on player views collapses navbar + footer + sidebars; player centers at max-width without entering fullscreen.
4. Home shows a dismissible status banner when an `AUTO-NNN` issue is `status: active`; banner hides when none.

**SPEC:** `phases/11-browse-detail-polish/11-SPEC.md`
**UI hint:** yes

---

### Phase 12: AdminRecs SPA quality

**Goal:** Make the admin recs debug surface keyboard- and screen-reader-usable; map error states; add empty-state and loading-state UX.

**Depends on:** Phase 5 (table semantics align with ButtonGroup pattern where applicable).

**Requirements:** UX-25, UX-26

**Success Criteria:**
1. AdminRecs table has `<caption>`, `aria-label`, keyboard-expandable rows (Enter/Space + `aria-expanded`).
2. AdminRecsPicker form has `aria-live` for validation; loading indicator visible during navigation.
3. 401 → "Session expired" friendly message; 500 → "Server error, please retry" with retry button; 429 → rate-limit notice.
4. Empty-state help text rendered when user has no recs.
5. Mobile horizontal-scroll table has `aria-label` and visual scroll affordance (right-edge shadow).
6. Router guard surfaces a toast before redirecting non-admin users home.

**SPEC:** `phases/12-adminrecs-spa-quality/12-SPEC.md`
**UI hint:** yes

---

### Phase 13: Optimistic UI on watchlist actions

**Goal:** Status pill flips, score changes, and list add/remove feel instant. Mitigates the Crunchyroll-class "tired of updating my list" pain point.

**Depends on:** Phase 1.

**Requirements:** UX-27

**Success Criteria:**
1. Clicking a status pill flips the visual state within one frame (< 16ms perceived latency).
2. Backend rejection (e.g. network error, 429) rolls back the visual state and shows a toast with retry CTA.
3. Idempotent re-clicks before the previous API call resolves are debounced or queued correctly (no race-condition write).

**SPEC:** `phases/13-optimistic-watchlist/13-SPEC.md`
**UI hint:** yes (motion feel)

---

### Phase 14: Marketing-surface polish — follower count, search hint, FAQ

**Goal:** Three Tier-E small wins on the marketing/discovery surface.

**Depends on:** Phase 1.

**Requirements:** UX-28, UX-29, UX-30

**Success Criteria:**
1. Anime detail page shows a follower count badge (e.g. "12 смотрят") derived from `anime_list` rows with `status='watching'`. Anti-bot consideration: count cached and not recomputed per request.
2. Search input placeholder reads "Поиск: название или жанр" (or RU/EN/JA equivalent per locale).
3. New FAQ accordion lives at `/о-сервисе` (or similar) with curated content: "Что такое AnimeEnigma?", "Платно ли?", "Что такое OP/ED?", "Как импортировать с MAL/Shikimori?".

**SPEC:** `phases/14-marketing-surface-polish/14-SPEC.md`
**UI hint:** yes (FAQ content is the feature)

---

### Phase 15: Multi-axis catalog filter sidebar (Dragon)

**Goal:** Replace the genre-popup-only `/browse` filter with a persistent sidebar exposing genre + format + status + year + **provider/audio-source** + sort. Provider-as-filter is uniquely AnimeEnigma's competitive moat.

**Depends on:** Phase 11 (sort dropdown lives here; Phase 11 establishes URL-state pattern).

**Requirements:** UX-31

**Success Criteria:**
1. `/browse` renders a left/top sidebar with 6 filter axes; each axis collapsible.
2. Active filters reflected in URL query (`?genre=A,B&format=tv&status=ongoing&year=2024-2026&provider=kodik,hianime&sort=popularity`).
3. Backend `/api/anime/search` accepts all six axes; results paginate correctly.
4. Provider axis values map to Kodik / AnimeLib / HiAnime / Consumet (and EnglishPlayer providers once Phase 16 single-player ships).
5. Empty filter state shows full catalog ordered by `sort_priority DESC, score DESC`.
6. axe-core: zero new violations.

**SPEC:** `phases/15-multi-axis-catalog-filter/15-SPEC.md`
**UI hint:** yes (Dragon — design-load)

---

### Phase 16: Broadcast schedule view (Phoenix)

**Goal:** New `/schedule` route + Home "На этой неделе" row sourced from Shikimori `nextEpisodeAt`. Animevost + FOD pattern.

**Depends on:** Phase 8 (continue-watching pattern), Phase 11 (status-banner infra).

**Requirements:** UX-32

**Success Criteria:**
1. `/schedule` shows a 7-day grid (Mon–Sun) with anime episodes airing each day. Times in user's locale-default timezone.
2. Home row "На этой неделе" surfaces today + tomorrow's airing episodes in horizontal scroll.
3. Empty-state for users with no followed anime: show all currently-airing instead.
4. Backend pulls `nextEpisodeAt` from existing Shikimori metadata; no new external API call.

**SPEC:** `phases/16-broadcast-schedule-view/16-SPEC.md`
**UI hint:** yes

---

### Phase 17: Editorial collections (Dragon)

**Goal:** Admin-curated `Подборки` system: new DB schema, admin tooling, Home row distinct from algorithmic recs.

**Depends on:** Phase 8, Phase 12 (admin UX patterns).

**Requirements:** UX-33

**Success Criteria:**
1. New `collections` table (id, slug, title_ru/en/ja, description, sort_priority, created_by) and `collection_items` (collection_id, anime_id, position).
2. Admin tool at `/admin/collections` for CRUD; non-admins blocked by router guard (consistent with Phase 12 admin UX).
3. Public route `/collections/:slug` renders the curated list with poster grid.
4. Home row "Подборки" cycles through 1-3 featured collections (sort_priority DESC).
5. RU/EN/JA locale entries for all admin-tool strings.

**SPEC:** `phases/17-editorial-collections/17-SPEC.md`
**UI hint:** yes

---

### Phase 18: Skip-Intro detection (Griffin)

**Goal:** Surface a "Пропустить опенинг" CTA on HiAnime/Consumet players using aniskip.com timestamps.

**Depends on:** Root milestone Phase 16 (single-player abstraction) — coordinate, do not duplicate.

**Requirements:** UX-34

**Success Criteria:**
1. HiAnime and Consumet players query aniskip.com on episode mount; if intro range present, render Skip-Intro CTA at the right offset.
2. CTA renders in player overlay; clicking seeks to intro_end timestamp.
3. Cache aniskip responses for 24h to avoid hammering the upstream.
4. If aniskip returns no data for an anime/episode, CTA is hidden (no error UI).
5. Phase coordinates with root-milestone Phase 16; ships only after EnglishPlayer consolidation.

**SPEC:** `phases/18-skip-intro-detection/18-SPEC.md` (write after coordination check with root-milestone owner)
**UI hint:** yes (player overlay)

---

### Phase 19: Grafana dashboard rebuild (Kraken)

**Goal:** Tidy up Grafana dashboard inventory: consistent naming, remove empty rows, normalize row numbering, audit panel types, standardize time-range defaults.

**Depends on:** Phase 1 (anonymous-Admin removed first).

**Requirements:** UX-35

**Success Criteria:**
1. All 7 dashboards renamed to `<Service Domain> — <Aspect>` Title-Case convention.
2. Empty "Service Overview" row on AnimeEnigma Monitoring either populated with 4-6 summary panels OR removed.
3. Row numbering normalized (all rows prefixed `1.` … `6.` or none prefixed).
4. Stat panels reviewed; any time-dependent metric switched to Time-series.
5. Default time range set to 6h on every dashboard.
6. Row links added for cross-dashboard navigation (Grafana 9+ feature).
7. Slop-watch: design review checkpoint before merge (this is Kraken-class, easy to phone in).

**SPEC:** `phases/19-grafana-dashboard-rebuild/19-SPEC.md`
**UI hint:** no (Grafana provisioning JSON)

---

### Phase 20: Tier D — polish batch ✅ shipped 2026-05-13

**Goal:** Mop up every remaining severity-1 cosmetic finding from the audit. Last on purpose — pair with design-review checkpoint to avoid slop.

**Depends on:** All prior phases (so we know what's still actually open).

**Requirements:** UX-36

**Success Criteria:**
1. Every severity-1 UA-NNN finding from `docs/issues/ui-audit-2026-05-12.md` and `followup-session.md` is either closed or explicitly marked Won't Fix with one-line rationale.
2. axe-core re-runs on all probed views show no regressions vs the 2026-04-17 + 2026-04-20 baselines.
3. Design-review checkpoint before merge: at least one screenshot diff per affected view.

**SPEC:** `phases/20-tier-d-polish-batch/20-SPEC.md`
**UI hint:** yes (polish batch)

---

## Cross-phase notes

- **Phase 1 always ships first.** UA-115 is a security catastrophe; everything else waits.
- **Phases 2 through 7 can run in any order after Phase 1.** They're independent fixes in different parts of the frontend.
- **Phase 8 unblocks Phases 9, 16, 17** (all use the watch_progress / row-render infrastructure that Phase 8 establishes).
- **Phase 18 (Skip-Intro)** is the only phase with a cross-workstream dependency. Coordinate with root milestone Phase 16 (single-player abstraction) — don't start until that EnglishPlayer surface stabilizes.
- **Phase 19 (Grafana)** is intentionally late because Kraken-class slop risk is high; do it after the higher-MVQ items so the team has bandwidth for design-review.
- **Phase 20 (polish)** is intentionally last because new minor findings can surface from Phases 8–18.

## Autonomous-run order suggestion

If running `/gsd-autonomous --ws ui-ux-audit` straight through:

```
1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12 → 13 → 14 → 15 → 16 → 17 → (root P16 ships) → 18 → 19 → 20
```

Stop after Phase 7 for the first session if budget is tight — that closes Tier A, all Tier B, all Tier C, and the bug fixes. Tier E (Phases 8+) is then a follow-up cycle.

## Out-of-scope reminders

See `REQUIREMENTS.md > Out of Scope` for the full list. Key boundaries:
- No single-player abstraction in this workstream (that's root Phase 16).
- No new OAuth or video providers.
- No backend rec-engine changes beyond rendering existing data.
- No notifications-engine work (separate phase candidate).
