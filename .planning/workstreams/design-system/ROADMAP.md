# Roadmap: AnimeEnigma `design-system` workstream

**Workstream:** design-system (parallel to root `v3.x` scraper work and other workstreams `watch-together`, `notifications`, `raw-jp`, `social`, `ui-ux-audit`, `hero-spotlight`)
**Active milestone:** v1.0 Design System Consolidation — Phase 1 shipped 2026-06-02; Phases 2–6 planned.
**Phase numbering:** Workstream-local — `v1.0` phases live at `phases/01-*`..`phases/06-*`.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`
**Requirements:** `REQUIREMENTS.md`

## Milestones

- ⏳ **v1.0 Design System Consolidation** — 6 phases. One layered, shadcn-vue-anchored token system + the app migrated onto it, with a lint gate so it stays consolidated. Phase 1 (foundation) complete; Phases 2–6 remain.
- ⏳ **v1.1 Living styleguide route** — Conditional. An in-app `/styleguide` gallery rendering every token + primitive live. Deferred; needs its own brainstorm.
- ⏳ **v1.2 Multi-theme** — Conditional. Light theme / per-user themes, enabled by the token model. Deferred.

## v1.0 phases (planning)

| Phase | Title | Status |
|-------|-------|--------|
| 1 | Token Foundation + Reference | ✅ Complete (2026-06-02 — 6 commits, listed individually below [the `ba8e4e83`→`d2baa16d` span is non-contiguous: 2 unrelated parallel-session commits are interleaved, so don't `git diff` the range]; tsc clean, 686 vitest pass, 5-surface in-browser smoke, opus final review *approve-with-minors* fixed. **Already pushed to `origin/main`** via a parallel session's push. See plan `docs/superpowers/plans/2026-06-02-design-system-consolidation-p1.md`) |
| 2 | shadcn-vue Install + Button/Card Proof | ⏳ Planned (1 plan) |
| 3 | Primitive Set Swap | ⏳ Planned |
| 4 | High-Traffic Surface Migration | ⏳ Planned |
| 5 | Tail Sweep + Lint Enforcement | ⏳ Planned |
| 6 | Governance into Memory | ⏳ Planned |

## Goal (v1.0)

One token vocabulary, shadcn-vue-anchored, that both the design tokens (`main.css`) and the component library (`components/ui/`) speak — with the 97-component app migrated onto it and a build-failing lint gate that keeps it consolidated. Neon Tokyo identity preserved throughout; zero rendered regression between phases.

**UXΔ = +2 (Better)** | **CDI = 0.12 * 55** | **MVQ = Phoenix 80%/85%**

## Phasing rationale

Six phases, each independently shippable and reversible, deliberately ordered to keep blast radius bounded — important because parallel Claude sessions land commits on `main` concurrently, so a single 44-file color sweep would be an unreviewable merge-conflict magnet.

- **Phase 1 (Foundation)** built the single source of truth additively — new tokens + `@theme inline` + deprecated aliases + the reference doc — changing nothing visually. It made the rest *possible* without touching a single component's rendering.
- **Phase 2 (Install + Proof)** is the smallest reversible bridgehead: add the shadcn-vue toolchain and convert just Button + Card. Validates the whole approach on 2 components before committing to 95 more.
- **Phase 3 (Primitive Swap)** rebuilds the `ui/` library behind unchanged import paths — isolating "build the library" from "use the library," so consumers don't move yet.
- **Phase 4 (High-Traffic Migration)** delivers the most user-visible consistency first (Home/Browse/Watch/nav/detail) and proves the tokens + primitives on real screens.
- **Phase 5 (Tail + Enforce)** sweeps the remaining components and then locks the door with a lint gate — the step that makes consolidation *stay*.
- **Phase 6 (Governance)** writes the rules into memory + CLAUDE.md so every future session follows them by default.

The `--accent` semantic flip (DS-MIGRATE-05) is intentionally late: it can only happen once every `var(--accent)` brand usage has been repointed (Phase 4–5), otherwise it would silently shift 12 components' accent color.

## Standing visual-regression smoke set

Because the whole milestone's premise is "zero rendered change," every phase that touches CSS/components MUST in-browser-smoke the **same 5 surfaces** established in Phase 1, at desktop + mobile widths, and confirm no rendered diff (jsdom can't catch cascade bugs — DS-NF-06):

1. **Home** — spotlight carousel (stats joke card + RandomTail purple `cta-hero` — the `@layer components` cascade-footgun zone), Онгоинги/Топ rails
2. **Browse / catalog** — filter sidebar, star badges, cards, pagination
3. **Anime detail** — cyan `.btn-primary` ("Смотреть"), status badge, schedule pill, language pills + green OurEnglish button
4. **A watch/player surface** — one of the 5 players loads + styled controls
5. **404** — muted styling + the "На главную" button

Phases 2–5 reference this set in their success criteria as "smoke the standing 5-surface set."

## Phases

### Phase 1: Token Foundation + Reference  ✅ COMPLETE (2026-06-02)

**Goal:** Consolidate the three overlapping token vocabularies into one layered, shadcn-vue-anchored source of truth in `main.css`, plus a canonical `DESIGN-SYSTEM.md`, with zero rendered change.
**Requirements:** DS-FOUND-01..07, DS-NF-01, DS-NF-02
**Delivered:** Tier 2 + Tier 3 tokens in `:root`; `@theme inline` wiring; value-preserving aliases; `.btn-*` re-points; `DESIGN-SYSTEM.md`; two Vitest guards (built-CSS utilities + token/alias/btn assertions). Commits `ba8e4e83` (doc), `2e028562` (tokens), `76d12a62` (@theme inline + guard), `a9a6a76a` (aliases), `9d2bd6fe` (.btn-* re-point), `d2baa16d` (review polish). Verified: tsc clean, 686 vitest pass, deployed + 5-surface in-browser smoke (home stats card, RandomTail purple `cta-hero`, catalog, anime detail cyan `.btn-primary`, 404), opus final review *approve-with-minors* (minors fixed).
**Plan:** `docs/superpowers/plans/2026-06-02-design-system-consolidation-p1.md`

### Phase 2: shadcn-vue Install + Button/Card Proof

**Goal:** Add the shadcn-vue toolchain and prove it end-to-end on two primitives. End-state: `Button` and `Card` are token-driven shadcn-vue components (`cva` variants, `cn()` merging), rendering identically to today on every existing usage, with the cyan→`default` / pink→`brand` / `destructive` variants working.
**Depends on:** Phase 1 (tokens must exist for the components to bind to).
**Requirements:** DS-LIB-01, DS-LIB-02, DS-LIB-03, DS-LIB-04, DS-NF-03, DS-NF-04
**Touches:**
- `frontend/web/package.json` (add `reka-ui`, `class-variance-authority`, `clsx`, `tailwind-merge`)
- `frontend/web/src/lib/utils.ts` (new — `cn()`)
- `frontend/web/components.json` (new — shadcn-vue config) or equivalent
- `frontend/web/src/components/ui/Button.vue`, `Card.vue` (rewrite, same import path)
- co-located `.spec.ts` for each
**Plans:** 1 plan
- [ ] `02-01-PLAN.md` — install toolchain + cn(); Button → cva/cn (back-compat aliases) + components.json; A2 text-color in-browser gate; Card → cn() + glass preserved + subcomponents; full verification gate
**Success criteria:**
1. `bun install` + `bun run build` clean; no new lint/tsc errors.
2. Every existing `<Button>`/`Card` usage renders identically — smoke the standing 5-surface set.
3. Button exposes `default`(cyan)/`brand`(pink)/`ghost`/`outline`/`destructive` variants + `sm/md/lg/icon` sizes, all token-driven.
4. Vitest covers variant→class mapping for both.
5. DS-NF-03 attested: no deps beyond the named toolchain; licenses compatible.

### Phase 3: Primitive Set Swap

**Goal:** Bring the shadcn-vue-equivalent `components/ui/` primitives onto shadcn-vue behind unchanged import paths, and add the four hand-rolled-today primitives. End-state: `@/components/ui` exports shadcn-vue-based Badge, Input, Select, Dialog (was Modal), Tabs, DropdownMenu (was ContextMenu), Tooltip, Popover, Switch, Checkbox — all token-driven. The 6 app-specific composites (`ButtonGroup`, `GenreFilterPopup`, `PaginationBar`, `SearchAutocomplete`, `Skeleton`, `Toaster`) are explicitly OUT of this swap (DS-LIB-07) — they're not shadcn-vue primitives; they get token-migrated in Phases 4–5 like any other component.
**Depends on:** Phase 2 (`cn()`/`cva` + components.json in place).
**Requirements:** DS-LIB-05, DS-LIB-06, DS-LIB-07, DS-NF-04
**Touches:** `frontend/web/src/components/ui/*` (rewrites + new primitives), `index.ts` barrel, co-located specs.
**Success criteria:**
1. Each swapped primitive renders identically on its current consumers (in-browser smoke per primitive's busiest surface).
2. Import paths + prop surfaces unchanged (or a documented codemod applied across consumers in the same phase).
3. New primitives (Tooltip/Popover/Switch/Checkbox) have at least one real consumer wired or a Vitest mount test.
4. The 6 composites are confirmed untouched-but-still-rendering (no accidental swap).
5. Full vitest + tsc green.

### Phase 4: High-Traffic Surface Migration

**Goal:** Migrate the heaviest-used surfaces to tokens-only + `ui/` primitives — Home, Browse, Watch/player, nav, anime detail. End-state: those surfaces contain zero off-palette colors and zero hardcoded hex; their buttons/cards/badges use the primitives.
**Depends on:** Phase 3 (primitives available).
**Requirements:** DS-MIGRATE-01, DS-MIGRATE-02 (partial), DS-MIGRATE-03 (partial), DS-MIGRATE-04 (partial), DS-MIGRATE-06 (partial)
**Touches:** `views/Home.vue`, `views/Browse.vue`, `views/Watch*.vue`, the 5 player components, `components/layout/*` nav, anime-detail components + their children.
**Success criteria:**
1. Grep shows zero off-palette color classes + zero `#hex` in the migrated files.
2. In-browser smoke confirms no rendered regression — the standing 5-surface set, at desktop + mobile widths.
3. Status colors on these surfaces come from `--success/-warning/-info/-destructive`.
4. Full vitest + tsc green; e2e (spotlight, player) specs still pass.

### Phase 5: Tail Sweep + Lint Enforcement

**Goal:** Migrate the remaining components, complete the alias retirement, flip `--accent`, then lock the door with a build-failing lint gate.
**Depends on:** Phase 4 (high-traffic surfaces already clean lowers risk).
**Requirements:** DS-MIGRATE-02 (complete), DS-MIGRATE-03 (complete), DS-MIGRATE-04 (complete), DS-MIGRATE-05, DS-MIGRATE-06 (complete), DS-GOV-01, DS-GOV-02
**Touches:** remaining `components/**/*.vue`, `main.css` (delete retired aliases, flip `--accent`), a new lint config (stylelint or custom node check) + its `scripts/` entry + the `redeploy-web`/CI gate.
**Success criteria:**
1. Repo-wide grep: zero off-palette color classes, zero hardcoded hex (outside the documented allowlist), zero `var(--ink|--accent|--pink)` brand usages.
2. `--accent` now resolves to the shadcn hover surface; the temporary brand-cyan alias deleted; no visual regression (in-browser smoke).
3. The lint rule fails the build on a deliberately-introduced `bg-red-500` test case, and passes on the clean tree; wired into the deploy gate.
4. Allowlist/escape-hatch documented.
5. Full vitest + tsc + e2e green.

### Phase 6: Governance into Memory

**Goal:** Write the design-system rules where every future session will see them.
**Depends on:** Phase 5 (rules describe the now-enforced reality).
**Requirements:** DS-GOV-03, DS-NF-05, DS-NF-06
**Touches:** project memory (`/root/.claude/projects/-data-animeenigma/memory/` + `MEMORY.md` pointer), `CLAUDE.md` (a Design System section pointing at `DESIGN-SYSTEM.md` + the lint gate).
**Success criteria:**
1. A memory entry captures: use tokens / never hardcode / reuse `ui/` primitives before building new / verify visual changes in-browser, with a pointer to `DESIGN-SYSTEM.md`.
2. `CLAUDE.md` has a Design System subsection.
3. The governance text matches the enforced lint rule (no rule documented that isn't enforced, and vice-versa).

## Next

```
/gsd-execute-phase 02 --ws design-system        # execute Phase 2 (1 plan, 5 tasks, 2 checkpoints)
```
