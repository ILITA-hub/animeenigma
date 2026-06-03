# Design System v2.0 — "Reuse + Portability" — Design

**Date:** 2026-06-03
**Workstream:** `design-system` (extends the v1.0 "Consolidation" milestone)
**Status:** approved (brainstorm) — Phase 1 spec; Phases 2–5 are decomposition stubs
**Predecessor:** `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`

---

## 1. Problem statement

The v1.0 milestone consolidated **colors/tokens** and shipped a build-failing lint gate, but
deliberately descoped the **structural** half: hand-rolled elements were not swapped for
`components/ui` primitives (Phase 6 reclassified primitive-reuse as governance-only). The result:

- **~150 raw `<button>` elements across 46 `.vue` files** (concentrated: `Anime.vue` 29,
  `Profile.vue` 12, plus the players and `Navbar`) vs. only ~11 files using the `<Button>` primitive.
  A change to `Button.vue` propagates to ~11–21 call sites; the other ~150 silently diverge.
- **Two parallel button systems**: `.btn-primary/.btn-secondary/.btn-ghost` (CSS in `main.css`) AND
  `Button.vue` cva variants encode the same values twice. The primitive itself bypasses tokens —
  `default`/`brand` hardcode `hover:shadow-[0_0_30px_rgba(0,212,255,0.3)]` as a raw arbitrary value
  instead of the existing `--shadow-glow-cyan` token.
- **`Card.vue` doesn't consume its own tokens** — it composes `.glass-card` (raw rgba-white), never
  `bg-card`/`border-border`, so the documented `--card`/`--border` mapping is inert.
- **Tokens live only as CSS custom properties** — no machine-readable export. AI design tools
  (Claude Design, Figma, Style Dictionary consumers) ingest W3C DTCG JSON far more reliably than a
  parsed Tailwind `@theme` block. This is the gating blocker for the stated goal of handing the
  system to AI tools for tweaking/further development.
- **A permanent tail of "deprecated" aliases** (`--ink`, `--accent`, `--pink`, `--ink-2/4`,
  `--f-display/ui/mono/jp`) doubles the vocabulary an AI/human must reason about.
- **The `.cta-*` family leaks off-palette colors** (`bg-purple-500`, `bg-amber-500`, …) that the lint
  gate forbids in `.vue` — but the gate scans `**/*.vue` only, so `main.css` escapes Rule 1.
- **No light/dark theming wiring** — single hardcoded dark theme; `dark:` appears once in the whole
  codebase. The token *shape* supports theming; the *wiring* doesn't (shadcn ships dual-theme, which
  AI tools assume).

**Root cause of the button sprawl (the key insight):** buttons get hand-rolled because the `Button`
primitive's API is **too opinionated** to model real shapes — it locks `rounded-xl`, `px-6 py-3`, a
hover-glow, and `active:scale-95`. The v1.0 Phase 7 audit found 9 of 10 high-traffic raw buttons were
*justified* bespoke keeps for exactly this reason. The fix is to **widen the API first, then swap.**

---

## 2. Milestone decomposition (5 phases)

Ordering front-loads component reuse (user priority) while honoring one hard dependency: **do not
export tokens until the vocabulary is clean** (Phase 3 before Phase 4).

| # | Phase | Delivers | Depends on |
|---|-------|----------|------------|
| **1** | **Button reuse** | Widen `Button` API (add `link`, `soft` variants; `xs`/`icon-sm` sizes; `radius` dimension); reconcile `.btn-*` vs cva into one source of truth; primitive references `--shadow-glow-*` tokens; swap clean-fit raw buttons; document bespoke keeps. | existing tokens |
| **2** | **Card/Badge reuse** | `Card.vue` consumes `--card`/`--border`; sweep hand-rolled cards/badges to primitives. | Phase 1 pattern |
| **3** | **Token hygiene** | Drain/freeze deprecated aliases; fill near-base neutral ramp gaps; fix `.cta-*` off-palette leak; **extend lint gate to `main.css`**. | — (cleanup) |
| **4** | **Token JSON export** | W3C DTCG `tokens.json` as source of truth + Style Dictionary pipeline generating `main.css`. **Primary AI-tooling enabler.** | Phase 3 |
| **5** | **Light/dark wiring** | `.dark` / `data-theme` structure so theming is config, not refactor. | Phases 3–4 |

Each phase is a GSD phase under `--ws design-system` with its own spec → plan → execute cycle. This
document fully specifies **Phase 1**; Phases 2–5 are scoped stubs to be detailed when reached.

---

## 3. Phase 1 — Button Reuse (full spec)

### 3.1 Widen the Button API

All additions are **purely additive** — existing variant/size names and their rendered output are
unchanged, so already-migrated call sites do not move a pixel.

**New variants** (added to `button-variants.ts` cva `variant` map):

- **`link`** — bare text link. No background, no border, no vertical padding. Brand-cyan foreground by
  default with a hover color shift. Replaces inline `text-cyan-400 hover:text-cyan-300` (and the pink
  equivalent — see open question Q1 on per-color link tints).
- **`soft`** — quiet filled control. `bg-white/10 hover:bg-white/20`, no border, **no glow, no
  `active:scale`**. Replaces nudge/chip-style inline controls and the `.cta-card` shape.

Existing variants kept as-is: `default`, `brand`, `ghost`, `outline`, `destructive`, plus the legacy
`primary`/`secondary` aliases (DS-NF-04 — zero call-site breakage).

**New sizes** (added to the `size` map):

- **`xs`** — `px-2 py-1 text-xs` (small inline controls, e.g. the seek-nudge buttons).
- **`icon-sm`** — `h-8 w-8 p-0` (small square icon controls; complements existing `icon` = `h-10 w-10`).

Existing sizes kept: `sm`, `md`, `lg`, `icon`.

**New `radius` dimension** (new cva variant key, decoupled from `variant`):

- Values: `sm | md | lg | xl | full`.
- The per-variant `rounded-*` classes are **removed from the variant strings** and expressed as the
  `radius` dimension instead. `defaultVariants.radius` and a `compoundVariants` entry reproduce the
  *current* defaults exactly: `default`/`brand`/`outline`/`destructive` → `xl`, `ghost` → `lg`. Net
  rendered output for existing call sites is identical (regression-pinned by tests in §3.4).

**Token cleanup inside the primitive:**

- Replace the raw `hover:shadow-[0_0_30px_rgba(0,212,255,0.3)]` (in `default`/`primary`) with
  `hover:shadow-glow-cyan`, and the pink equivalent in `brand`/`secondary` with
  `hover:shadow-glow-pink`. These utilities are generated from the `@theme` `--shadow-glow-*` tokens.
  **Build-verify** the utilities resolve (acceptance gate in §3.4); if Tailwind v4 does not emit
  `shadow-glow-*` from `@theme` in this project's config, fall back to keeping the value but hoisting
  it to a single shared cva fragment (no per-variant duplication). The raw-rgba string is eliminated
  either way.
- `active:scale-95` stays on `default`/`brand`/`primary`/`secondary`/`destructive` only (brand
  identity); `ghost`/`outline`/`soft`/`link` have no scale animation.

### 3.2 Reconcile the `.btn-*` duplication

`.btn`, `.btn-primary`, `.btn-secondary`, `.btn-ghost` (and `.btn:focus-visible`, `.btn:disabled`) in
`main.css` duplicate the cva. They are used in only **~9 call sites** (measured: `btn-primary` 1,
`btn-secondary` 1, `btn-ghost` 1, bare `.btn` 6 files).

1. Migrate those call sites to `<Button variant="…">` (`btn-primary→default`, `btn-secondary→brand`,
   `btn-ghost→ghost`).
2. Delete the `.btn*` rules from `main.css`. The cva becomes the single source of truth.

(The `.btn-primary-hero` scoped class in `FeaturedCard.vue` is a *different*, card-local class and is
**out of scope** — left untouched.)

### 3.3 Swap policy — clean fits only

Mechanical audit of every raw `<button>` in `src/**/*.vue` (excluding `ui/`, `*.spec.*`,
`__tests__`). Each button is classified as exactly one of:

- **Clean fit** → swap to `<Button variant size radius>` (+ `class` passthrough for layout-only
  utilities like `w-full`, margins, grid placement). A fit is "clean" when the rendered box
  (bg/border/radius/padding/typography/hover) is reproducible by the widened API **without moving
  visible pixels**.
- **Bespoke keep** → stays a raw `<button>`, recorded with a one-line reason in the governance list
  (§3.5). Expected keeps: player transport controls (icon-on-video with custom positioning), the
  `kebab-glow` animated control, carousel prev/next arrows, and any control whose layout/animation the
  primitive can't model.

The keeps are **re-adjudicated against the new, wider API** — the old v1.0 governance note that
"tiny-icon controls and text-links stay bespoke" partly dissolves now that `link`, `xs`, and
`icon-sm` exist.

Brand-identity guard: because the glow + `active:scale` live only on `default`/`brand`, swapping a
quiet inline button to `soft`/`ghost`/`link` never *introduces* a glow it didn't have.

### 3.4 Verification / acceptance criteria

A Phase 1 plan is "done" only when ALL of:

1. `bash frontend/web/scripts/design-system-lint.sh --selftest` → SELFTEST PASS, and
   `make lint-design` → PASS on the tree.
2. `Button.spec.ts` extended: asserts each new variant (`link`, `soft`), new sizes (`xs`,
   `icon-sm`), and the `radius` dimension render expected classes; **and pins that pre-existing
   variant/size combinations render byte-identical class strings** (regression guard on the
   `radius`-decoupling refactor).
3. `bunx vue-tsc --noEmit` exit 0 (the widened `ButtonVariants` type must still narrow).
4. `bunx vite build` clean — **this is the gate that proves `shadow-glow-*` utilities resolve.**
5. `bunx vitest run` green (modulo the pre-existing `AnimeContextMenu.spec.ts:227` failure logged in
   v1.0 `04-deferred-items.md`).
6. **In-browser visual smoke (desktop + mobile)** on the top swapped surfaces — `Anime.vue`,
   `Profile.vue`, `Navbar` — confirming zero visible regression (DS-NF-06 standing rule; jsdom cannot
   catch Tailwind-v4 cascade bugs).
7. The `.btn*` classes are gone from `main.css` and no `.vue` references them (grep-verified).

### 3.5 Governance updates

- `frontend/web/src/styles/DESIGN-SYSTEM.md` — update the component inventory and the Button
  variant/size map to document `link`/`soft`, `xs`/`icon-sm`, and the `radius` dimension.
- Add/refresh a **"bespoke keep" list** (the buttons that legitimately stay raw, each with a reason),
  superseding the v1.0 governance-only note.
- Memory: update `project_design_system_governance.md` to reflect the widened primitive + that
  primitive-reuse now has a concrete absorbed-shapes list.

### 3.6 Out of scope for Phase 1

- Card/Badge swaps (Phase 2), token hygiene & the `.cta-*` leak (Phase 3), token JSON export
  (Phase 4), light/dark wiring (Phase 5).
- The unlayered/layered CSS cascade footguns (`.spotlight-frame`, `.cta-*`) — untouched here.
- `FeaturedCard.vue`'s scoped `.btn-primary-hero`.

---

## 4. Risks & mitigations

| Risk | Mitigation |
|------|-----------|
| `radius`-decoupling silently shifts an existing button's corner | Regression test pins byte-identical class output for all pre-existing variant×size combos (§3.4.2) before any swap. |
| `shadow-glow-*` utility doesn't generate in Tailwind v4 config | `vite build` gate (§3.4.4) catches it; documented fallback to a single shared cva fragment (§3.1). |
| A "clean fit" judgment moves pixels | In-browser desktop+mobile smoke on top surfaces (§3.4.6); clean-fit-only policy keeps diffs ~zero. |
| Scope creep across 46 files in one plan | Plan splits the swap by surface cluster (Anime / Profile / players / nav / misc) so each is independently reviewable & committable. |
| Breaking a call site that passed `.btn-*` extra classes | `<Button>` already supports `class` passthrough via `cn()`; migrate layout utilities to the `class` prop. |

---

## 5. Project metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — quieter, consistent buttons; no user-visible regression intended (clean-fit
  swaps only).
- **CDI = 0.04 × 21** — wide blast radius (~46 files) but each edit is local, mechanical, and
  value-preserving; Fibonacci effort 21.
- **MVQ = Griffin 88%/82%** — disciplined structural consolidation, low slop risk (API-first, then
  mechanical swap with a visual gate).

---

## 6. Open questions for the plan phase

1. **Per-color `link` tints** — should `link` be cyan-only (callers override pink via `class`), or
   should it take an accent sub-variant (`link` + a color modifier) to natively cover the pink/white
   text-links? Leaning cyan-default + `class` override to avoid combinatorial cva growth.
2. **`soft` vs `ghost` overlap** — confirm both are warranted, or fold the borderless quiet shape into
   a `ghost` sub-variant. Leaning keep-both (ghost has a border; soft does not — distinct shapes).
3. **Commit granularity** — one commit per surface cluster (recommended) vs one per file.
