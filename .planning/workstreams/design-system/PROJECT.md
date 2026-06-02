# Project: AnimeEnigma — `design-system` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** design-system
**Created:** 2026-06-02
**Lifecycle:** Independent of v3.x scraper work; parallel to other workstreams (`watch-together`, `notifications`, `raw-jp`, `social`, `ui-ux-audit`, `hero-spotlight`)
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md` (Approved for planning, 2026-06-02)
**P1 implementation plan:** `/data/animeenigma/docs/superpowers/plans/2026-06-02-design-system-consolidation-p1.md`

## Scope of this workstream

Consolidate AnimeEnigma's frontend design language into **one layered, shadcn-vue-anchored design system** — and then migrate the app onto it.

This is **not a redesign.** The "Neon Tokyo" visual identity (dark base, cyan + pink dual accent, glassmorphism, glow shadows, self-hosted editorial type) stays. The problem is that the language is currently defined in **three overlapping, partly-diverging places** and consumed inconsistently across 97 components: the Tailwind v4 `@theme` block, a later `:root` semantic-token set that duplicates it, and hand-rolled component CSS — with status colors sprawled across the raw Tailwind palette (241 off-palette color usages across 44 files) and only 8 components routing through the `components/ui/` primitive set.

> **One token vocabulary, shadcn-vue-anchored, that the tokens and the component library both speak — so a new contributor (human or agent) has exactly one place to look, and drift becomes mechanically impossible.**

## Core value

A design system is leverage: every future feature inherits consistency for free, status colors stop being a per-component coin-flip, one-off visual bugs drop, and the component library (shadcn-vue) gives accessible, well-tested primitives instead of hand-rolled buttons/dialogs. Anchoring on shadcn-vue means we adopt a large, battle-tested ecosystem's conventions rather than inventing our own.

**UXΔ = +2 (Better)** | **CDI = 0.12 * 55** | **MVQ = Phoenix 80%/85%**

## Key decisions (locked in the design doc)

- **Anchor:** shadcn-vue (Reka UI). shadcn/ui is React-only; shadcn-vue is the faithful Vue 3 port (own the copy-pasted component code; same CSS-variable token convention).
- **Token architecture:** three-tier model — Tier 1 primitives → Tier 2 shadcn-vue semantic slots → Tier 3 brand extension.
- **Brand mapping:** cyan → `--primary`; **pink → `--brand-pink`** (a brand CTA variant, *not* shadcn's neutral `--secondary`).
- **Zero breakage between phases:** every phase is independently shippable; old scattered tokens stay as deprecated aliases until their consumers migrate.
- **`--accent` deferral:** shadcn's `--accent` (hover surface) collides with the existing brand-cyan `--accent` (12 files); the semantic flip is deferred until those usages are repointed (Phase 4–5).

## Out of scope for this workstream

| Excluded | Reason |
|---|---|
| Net-new visual identity / rebrand | We build on Neon Tokyo. No new palette, fonts, or glow aesthetic. |
| Changing the radial-gradient body background or glow look | Identity is preserved, not reworked. |
| Backend / Go services | Frontend design only. |
| A living in-app `/styleguide` gallery route | Considered, deferred — could be a later milestone. |
| Dark/light theme toggle | Single dark theme today; multi-theme is a future milestone (the token model makes it possible later). |

## Active milestone

⏳ **v1.0 Design System Consolidation** — 6 phases. P1 (foundation) shipped 2026-06-02; P2–P6 (shadcn-vue adoption + component migration + enforcement) remain.
