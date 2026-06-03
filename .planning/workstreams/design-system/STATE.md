---
gsd_state_version: 1.0
milestone: v3.1
milestone_name: Scraper Self-Healing
current_phase: 05
current_plan: 5
status: ready_to_plan
stopped_at: Phase 05 Plan 05 complete (build-failing design-system lint gate + adjudicated allowlist, wired into make lint-frontend + redeploy-web; full-suite green [DS-GOV-01, DS-GOV-02]) — Phase 05 complete (5/5)
last_updated: "2026-06-03T05:00:00.000Z"
last_activity: 2026-06-03
progress:
  total_phases: 14
  completed_phases: 10
  total_plans: 53
  completed_plans: 46
  percent: 71
---

# Project State

## Current Position

Phase: 05 (tail-sweep-lint-enforcement) — COMPLETE (5/5 plans)
Plan: 5 of 5 (complete)
**Status:** Ready to plan
**Current Phase:** 6
**Last Activity:** 2026-06-03
**Last Activity Description:** Phase 05 Plan 05 complete (build-failing design-system lint gate + adjudicated allowlist, wired into make lint-frontend + redeploy-web; --selftest fail-path; DESIGN-SYSTEM.md docs; SC#5 full-suite green [DS-GOV-01, DS-GOV-02])

## Progress

**Phases Complete:** 3 / 6
**Current Plan:** Not started

## Performance Metrics

| Phase | Plan | Duration | Tasks | Files |
|-------|------|----------|-------|-------|
| 04 | 01 | ~14 min | 3 | 5 |
| Phase 04 P02 | ~4 min | 2 tasks | 4 files |
| Phase 04-high-traffic-surface-migration P03 | 1 session | 2 tasks | 1 files |
| Phase 04 P04 | ~10 min | 3 tasks | 6 files |
| 05 | 01 | ~12 min | 2 | 22 |
| 05 | 02 | ~10 min | 2 | 8 |
| Phase 05 P03 | ~14 min | 3 tasks | 11 files |
| 05 | 04 | ~6 min | 2 | 1 |
| 05 | 05 | ~20 min | 2 | 5 |

## Session Continuity

**Stopped At:** Phase 05 Plan 05 complete (design-system lint gate [DS-GOV-01, DS-GOV-02]) — Phase 05 COMPLETE (5/5)
**Resume File:** None

## Notes

- Phase 1 artifacts live in the repo, not in this workstream's `phases/` dir (it was built before the workstream existed). Plan: `docs/superpowers/plans/2026-06-02-design-system-consolidation-p1.md`. Spec: `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`.
- Phase 1 commits (6, `ba8e4e83`..`d2baa16d` — non-contiguous) are on `main` and **already pushed to `origin/main`** (verified 2026-06-02; a parallel session's push swept them up). The workstream-seed commit is the only local-unpushed design-system commit at creation time.
- `--accent` semantic flip is deferred to Phase 5 (DS-MIGRATE-05) — do not flip earlier.
- Plan 04-01 decisions: `#ffd700` star-gold → `var(--warning)` (#ffd600, 1-unit hue delta per research A3); status-pill opacity modifiers (`/80`,`/90`) kept on the base semantic token (no `-soft` matches the alpha); Badge primitive swap on pills deferred (DS-MIGRATE-06 partial — a structural swap shifts pixels); Browse.vue verified grep-clean (no edit); `bg-cyan-500/80` left as-is (cyan = brand-primary, outside off-palette regex); `--accent-line`/`--accent-soft` literal aliases left untouched.
- Plan 04-01 deferred (pre-existing, NOT this plan): `src/analytics/__tests__/index.spec.ts` TS2307 (missing analytics barrel, analytics workstream); `AnimeContextMenu.spec.ts:227` reference-prop fail (Reka DropdownMenu anchored-mode, Phase 3). See phase `deferred-items.md`.
- Plan 04-04 decisions: player-accent hex kept verbatim (#06b6d4 Kodik cyan, #f97316 AnimeLib orange, #ec4899 Hanime pink) + SubtitleOverlay render defaults (#ffffff, #ffcccc) — NOVEL, near-but-not-equal brand tokens; snapping shifts hue (Pitfall 2); allowlisted, no main.css token (deferred Phase 5). `bg-zinc-900/95` → `bg-popover/95` (elevated menu surface, not card). Opacity modifiers (/20,/30,/50,/70,/80,/90) kept on base semantic tokens (no `-soft` matches the alpha). Player chrome kept structurally intact — NO Button primitive swap (DS-MIGRATE-06: no analog). RawPlayer.vue re-grepped clean (no edit needed).
- Plan 04-04: DS-MIGRATE-02/03/06 advance to "Phase-4 partial complete" on the player surfaces; full completion + `--accent` flip remain Phase 5 (DS-MIGRATE-05). `requirements.mark-complete` returned not_found for these IDs (partial-requirement tracking) — intentionally NOT force-marked at Phase 4.
- Plan 05-01 (commits `87a4afec`, `058eb4a6`): tail-swept 16 home SFCs (11 spotlight + 5 rails) → semantic tokens. NowWatching avatar palette mapped hue-preservingly (red/amber/emerald/sky/violet→destructive/warning/success/info/brand-violet); cyan/pink/orange + `fuchsia-500` kept as brand/provider. `var(--token,#fallback)` repointed by name (fallback stays). Literal aliases kept: `--ink-2/-4`, `--accent-line/-glow`, `--violet`. 6 co-located spotlight specs realigned to migrated class strings (Rule 1). NOVEL hex for Wave-3 allowlist seed: FeaturedCard `#001218`; PlatformStatsCard `#0d2030`/`#050a12`; NowWatching `#0a0e1a` (ring); CollectionsRow `#0e7490`/`#6b21a8` (cover gradient); ContinueWatchingRow `#001218` (play icon). See 05-01-SUMMARY.md "Novel hex" table.
- Plan 05-02 (commits `e9080e23`, `2a9cdec9`): tail-swept 4 admin views (AdminCollectionEdit/Collections/Recs/RawLibrary) + 4 standalone views (Game/Profile/StatusPage/Themes) → semantic tokens. Highest-hit tail files (Profile ≈22, StatusPage ≈13, AdminRecs ≈12, RawLibrary ≈10). Status mapping: green/emerald→success, red→destructive, amber/yellow→warning, blue→info, purple→brand-violet; /80,/40,/30,/20,/10 opacity kept on base token. **Judgment call:** AdminRecs `topContributorClass` is a categorical signal-source palette (s1-s6) — blue/emerald/yellow/purple slots migrated hue-preservingly, but s4 orange-500 (not in off-palette regex) + s6_pin cyan-500 (brand) left verbatim; `reasonBadgeClass` by contrast is genuine status→migrated. Profile `text-pink-400` error + cyan border/ring left as brand. **ZERO novel hex** (no Wave-3 allowlist additions). No co-located specs for the 8 views → no test realignment needed. See 05-02-SUMMARY.md.
- Plan 05-03 (commits `9b820ab3`, `a09c915b`, `b61b0c4f`): tail-swept 11 SFCs — 3 ui composites (GenreFilterPopup/Select/Toaster) + App + LastUpdates + themes (RatingStars/ThemeCard) + BrowseSidebar + ActiveSessionsCard + RoomSidebar + ActivityFeed. Elevated poppers `slate-900/95`→`popover/95`; non-elevated aside `slate-900/40`→`card/40`. LastUpdates changelog `color:` hex → `var(--color-success)`/`var(--color-warning)`/`var(--info)`/`var(--muted-foreground)`; rgba badge BACKGROUNDS left verbatim (value-preserving — `-soft` alpha 0.14≠0.2). **NOVEL hex (Wave-3 allowlist):** LastUpdates `#0e7490` (news-cover gradient start — same novel cyan hue 05-01 kept for CollectionsRow); thumb `#4c1d95`→`var(--brand-violet)`. **Judgment call:** BrowseSidebar AnimeLib `text-orange-500 focus:ring-orange-500` KEPT (provider-identity decorative hue, mirrors `--player-accent #f97316`; orange not in off-palette regex) while english `emerald-500`→`success` and kodik cyan brand left. ThemeCard star `yellow-400`→`warning`; ED badge `purple`→`brand-violet`. **ActivityFeed via STASH-ISOLATION:** `git stash push -- ActivityFeed.vue` → repoint `var(--accent)`×2→`var(--brand-cyan)` + `var(--ink)`→`var(--foreground)` + `var(--ink-3)`→`var(--muted-foreground)` (literal `--ink-2/-4` left) → commit color-only (`b61b0c4f`, 9 ins/9 del, NO analytics hunks) → `git stash pop` clean auto-merge (analytics 63-ins preserved uncommitted, stash list empty). **REPO-WIDE off-palette + repointable-alias greps now ZERO survivors across `src/**/*.vue`** → Wave-2 `--accent` flip-gate prerequisite satisfied. See 05-03-SUMMARY.md.
- Plan 05-05 (commits `4701f8bc`, `e969c9ec`): the build-failing color/token lint gate that locks the migration shut [DS-GOV-01, DS-GOV-02]. `scripts/design-system-lint.sh` (mirrors i18n-lint.sh) enforces EXACTLY 3 rules over `src/**/*.vue` (excl specs/__tests__): (1) off-palette classes — Phase-4 verbatim regex, brand-exempt (cyan|pink|orange|rose|indigo|teal|lime ABSENT, else clean tree fails); (2) non-allowlisted hex (per-(file,hex)); (3) deprecated `var(--ink|--accent|--pink)` (excl `--ink-2/-4`, `--accent-soft/-line/-glow`, `--pink-soft`). `scripts/design-system-allowlist.txt` = 33 justified `path:hex:reason` entries (seeded from 05-01/05-03 novel-hex + Phase-4 player/chrome). `--selftest` proves the fail-path (inject `bg-red-500` → DETECT → trap-clean → clean-tree PASS, no scratch left). Wired into `make lint-design` → `lint-frontend` (CI/all) AND `redeploy-web` prereqs (deploy gate) + `.PHONY`. **ADJUDICATED out-of-scope hex (not blanket-allowlisted):** Auth.vue Telegram blue `#54a9eb`/`#4a96d2` → justified provider-brand allowlist; Auth.vue QR `#000000`/`#ffffff` → functional allowlist; Collections.vue gradient `#0e7490`/`#6b21a8` → adjudicated KEEP (identical to CollectionsRow's kept novel gradient; darker/desaturated, no token within tolerance) — NOT migrated. DESIGN-SYSTEM.md "Lint gate (enforced)" docs the 3 rules + brand-exemption + allowlist format + adjudication + selftest, matching the enforced rules exactly. **SC#5 (owned by this plan): `vue-tsc --noEmit` exit 0; `vitest run` FULL = 831 pass / 1 fail (sole fail = documented pre-existing `AnimeContextMenu.spec.ts:227`); home/spotlight 223/223 pass.** **[Rule 1 deviation]** fixed stale `design-tokens.spec.ts:27` (05-04 flipped `--accent`→`var(--elevated)` but left the test asserting `var(--brand-cyan)`) — realigned to the committed behavior. spotlight e2e canary requires a live `webServer:5173`, not runnable headless here → documented (no rendered/route/component change ⇒ no plausible e2e regression). Pre-existing uncommitted changes (ActivityFeed analytics, scraper, changelog, etc.) NOT swept into either commit. See 05-05-SUMMARY.md. **Phase 05 COMPLETE (5/5).**
- Plan 05-04 (commit `7127275f`): the milestone's ONE intentional rendered change — flipped `main.css :root --accent` from `var(--brand-cyan)` to `var(--elevated)` (#1c1c2c neutral shadcn hover surface, consistent with `--popover`/`--secondary`) and deleted the temp brand-cyan alias annotation + the "deferred to P2 / stays brand-cyan" NOTE block. Hard precondition re-asserted: zero brand `var(--accent)` in `src/**/*.vue`. `--brand-cyan`, `--accent-soft/-line/-glow` untouched; no rule relayered (Tailwind v4 cascade footgun). Staged ONLY main.css (1 ins / 3 del). vue-tsc exit 0, vite build exit 0. human-verify checkpoint AUTO-APPROVED (autonomous); in-browser 5-surface smoke deferred → persisted as `05-04-HUMAN-UAT.md` for live confirmation (DS-NF-06: jsdom/build can't catch a cascade regression). Satisfies the Wave-3 lint-gate rule (c) prerequisite (any `var(--accent)` brand usage is now a violation). See 05-04-SUMMARY.md.
