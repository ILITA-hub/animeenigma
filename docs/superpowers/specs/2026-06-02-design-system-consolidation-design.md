# Design System Consolidation — Neon Tokyo, shadcn-vue–anchored

**Date:** 2026-06-02
**Status:** Design approved — ready for implementation planning
**Scope of this effort:** Define + document the single source of truth, and consolidate `main.css`. The component-wide refactor and shadcn-vue install are sequenced as **next steps** (Phases P2–P6), not done here.

---

## 1. Problem

AnimeEnigma already has a real visual identity — the **"Neon Tokyo"** theme (dark base, cyan + pink dual accent, glassmorphism, glow shadows, self-hosted editorial type). This is **not a redesign**. The problem is that the design language is currently defined in **three overlapping, partly-diverging places**, and consumed inconsistently across 96 components.

### Current state (measured 2026-06-02)

The language lives in three places inside `frontend/web/src/styles/main.css`:

1. **`@theme` block** (Tailwind v4 native tokens) — `--color-base`, `--color-cyan-400/500/600`, `--color-pink-*`, `--color-success`, `--color-warning`, fonts, shadows, animations. Generates utilities like `bg-cyan-500`.
2. **`:root` semantic tokens** (added later) — `--ink/--ink-2/3/4`, `--surface-2`, `--elevated`, `--line/--line-strong`, `--accent/--accent-soft/--accent-line`, `--pink`, `--violet`, `--r-sm…2xl` radii, `--f-display/ui/mono/jp`. These **duplicate and partly diverge** from the `@theme` set (e.g. `--accent: #00d4ff` == `--color-cyan-400`; the radius scale and ink ramp exist only here).
3. **Hand-rolled component CSS classes** — `.btn-primary/.secondary/.ghost`, `.glass*`, `.cta-hero/card/text`, `.card-hover`, etc., plus documented cascade footguns.

Consumption is fragmented:

- 55 `.vue` files use `@theme` utilities (`bg-cyan-500`), 19 use `:root` semantic vars (`var(--ink)`), 17 hardcode hex — many mix all three.
- **Status/semantic colors are sprawled across the raw Tailwind palette** with no defined mapping: `bg-red-500` (×19), `amber-500` (×17), `emerald-500` (×12), plus `green/purple/yellow/blue/slate/gray/sky/teal/orange-*`. Every component picks its own red/amber/green for error/warning/success.
- A `components/ui/` primitive set exists (Button, Card, Badge, Input, Modal, Select, Tabs, …) but many components bypass it and hand-roll with `.glass-card` + raw utilities.
- No shadcn-adjacent tooling present yet: no `reka-ui`/`radix-vue`, no `class-variance-authority`, no `clsx`/`tailwind-merge`, no `cn()`.

### Goal

One **layered token source of truth**, anchored on **shadcn-vue** conventions so the design tokens and the (future) component library speak the same language, while keeping the Neon Tokyo identity intact and breaking **nothing** today.

> **Note on "shadcn":** shadcn/ui is React-only. This is a Vue 3 project, so we anchor on **shadcn-vue** (built on **Reka UI**, formerly Radix Vue) — same philosophy (you *own* the copy-pasted component code, not an npm dependency) and the same CSS-variable token convention.

---

## 2. Decisions (locked)

- **Outcome:** Define + document the system; consolidate `main.css`. Component migration is phased (next steps).
- **Anchor:** shadcn-vue (Reka UI). Adopt its token naming + component conventions; map Neon Tokyo onto its semantic slots.
- **Token architecture:** Three-tier model (Approach A) — primitives → shadcn semantic slots → brand extension.
- **Brand mapping:** **cyan → `--primary`**, **pink → `--brand-pink`** (a brand CTA variant, *not* shadcn's neutral `--secondary`).
- **Zero breakage:** Tiers 2 & 3 are additive; old scattered tokens become deprecated aliases pointing at canonical slots.

---

## 3. Token architecture (three tiers)

All in `main.css` as one layered system. Tier 1 is unchanged; Tiers 2 & 3 are new canonical layers that reference Tier 1.

### Tier 1 — Primitives (raw values, keep existing names, no edits)

The palette of record: `--color-base #08080f`, `--color-surface #11111c`, `--surface-2 #161623`, `--elevated #1c1c2c`, `--color-cyan-400 #00d4ff / -500 #00b8e6 / -600 #009dcc`, `--color-pink-400 #ff4d8d / -500 #ff2d7c / -600 #e6196b`, `--color-success #00ff9d`, `--color-warning #ffd600`, `--violet #a78bfa`, ink ramp (`#fff`, `.78`, `.56`, `.36`), line alphas (`.06`, `.12`), glow shadows.

### Tier 2 — shadcn-vue semantic slots (new; each → a primitive)

| Slot | Value (→ primitive) | Notes |
|---|---|---|
| `--background` | `--color-base` #08080f | page bg (body keeps its radial-gradient overlay) |
| `--foreground` | `#ffffff` | default text |
| `--card` / `--card-foreground` | `--color-surface` / white | resting card surface |
| `--popover` / `--popover-foreground` | `--elevated` / white | menus, dropdowns |
| `--primary` / `--primary-foreground` | `--color-cyan-500` / `--color-base` | **cyan = primary action**; dark text on cyan (matches `.btn-primary`) |
| `--secondary` / `--secondary-foreground` | `--elevated` neutral / white | **shadcn-neutral low-emphasis button**, *not* pink |
| `--muted` / `--muted-foreground` | `--surface-2` / `rgba(255,255,255,.56)` | muted surfaces + low-emphasis text |
| `--accent` / `--accent-foreground` | `rgba(255,255,255,.05)` / white | **shadcn meaning = subtle hover surface**, not brand |
| `--border` | `rgba(255,255,255,.10)` | `--line` (.06) + `--line-strong` (.12) kept as finer-grain extras |
| `--input` | `rgba(255,255,255,.12)` | input borders |
| `--ring` | `--color-cyan-400` #00d4ff | matches current `:focus-visible` |
| `--radius` | `0.75rem` | base; explicit `--r-sm…2xl` scale kept |

### Tier 3 — Brand extension (Neon Tokyo things shadcn doesn't ship)

| Token | Value | Use |
|---|---|---|
| `--brand-cyan` | `--color-cyan-400` #00d4ff | glow/brand cyan |
| `--brand-pink` / `--brand-pink-foreground` | `--color-pink-500` #ff2d7c / white | **the pink CTA** (today's `.btn-secondary`) → a `brand`/`pink` variant |
| `--brand-violet` | #a78bfa | tertiary accent |
| `--success` / `--success-foreground` | #00ff9d / `--color-base` | status: success |
| `--warning` / `--warning-foreground` | #ffd600 / `--color-base` | status: warning |
| `--info` / `--info-foreground` | **#5ab8ff** *(new)* / `--color-base` | status: info (distinct from primary cyan) |
| `--destructive` / `--destructive-foreground` | **#ff4d4d** *(new)* / white | status: error/destructive — replaces ad-hoc `red-500` sprawl |
| `*-soft` variants | source color @ ~14% alpha | badge/chip backgrounds: `--primary-soft`, `--brand-pink-soft`, `--success-soft`, `--warning-soft`, `--info-soft`, `--destructive-soft` |
| `--glow-cyan` / `--glow-pink` / `--glow-sm-cyan` | existing shadows | accent elevation |

**Two values are net-new (approved, tunable):** `--info #5ab8ff`, `--destructive #ff4d4d`.

### Wiring

Tiers 2 & 3 are exposed to Tailwind v4 via `@theme inline`, so `bg-background`, `text-foreground`, `bg-primary`, `text-primary-foreground`, `bg-destructive`, `border-border`, `bg-success-soft`, `ring-ring`, etc. become real utilities. Old scattered `:root` tokens (`--ink*`, `--accent`, `--accent-soft`, `--accent-line`, `--pink`, `--f-*`) become **deprecated aliases** pointing at the canonical slots so the 96 current components run unchanged.

### The one teaching point

shadcn's `--accent` is a **hover surface** (e.g. a hovered menu item background). Your **brand cyan is `--primary` / `--brand-cyan`**. Two different "accents," explicitly separated, so shadcn-vue components behave as upstream expects.

---

## 4. Type, spacing, radius, elevation

**Type** — 5 self-hosted families, canonicalized on the `@theme` `--font-*` set:
- `--font-sans` (Inter + Noto Sans JP fallback) — body/UI
- `--font-display` (Manrope) — headings
- `--font-mono` (JetBrains Mono) — code/stats
- `--font-jp` (Noto Sans JP) — subtitle / JP text

The later `--f-display/ui/mono/jp` aliases collapse into `--font-*`; `--f-*` names stay as deprecated aliases. A size/weight ramp is **documented** (uses Tailwind's `text-xs…5xl`; no new tokens): display headings `font-display font-semibold/bold`; body `font-sans font-normal/medium`. The existing "only `font-medium`/`font-semibold`" rule is captured as a **per-zone exception** for spotlight cards, not a global law.

**Spacing** — keep Tailwind's default 4px scale; no custom spacing tokens. The spec **documents existing conventions** so they're stated once: card padding `p-4 md:p-6 lg:p-8`, touch target 44px (`.touch-target`), section rhythm.

**Radius** — `--radius` base (0.75rem) + explicit `--r-sm 8 / --r-md 12 / --r-lg 16 / --r-xl 22 / --r-2xl 28`, documented with usage: chips→sm, buttons/inputs→md/lg, cards→xl, modals→2xl.

**Elevation** — three documented levels mapping to existing glass utilities: `glass` (flat overlay) → `glass-card` (resting) → `glass-elevated` (raised: dropdowns/modals). Glow shadows are **accent** elevation, not structural.

---

## 5. `main.css` consolidation plan (what changes now)

1. **Add** Tier 2 + Tier 3 token blocks and the `@theme inline` wiring. Additive — no existing token removed.
2. **Alias** the duplicate `:root` semantic tokens (`--ink*`, `--surface-2`, `--accent*`, `--pink`, `--f-*`) to canonical slots, each tagged `/* deprecated — migrate to --foreground / --primary / etc. */`.
3. **Preserve the load-bearing cascade behavior verbatim** (documented footguns — see §7):
   - `.cta-*` family stays inside `@layer components` (so utilities win — keeps `md:hidden` working on `PersonalPickCard`'s mobile button).
   - `.spotlight-frame`, `.shuffle-deck`, `.glass-card` stay **unlayered** (a second `@layer components` block gets tree-shaken by Tailwind v4).
4. **Re-point** hand-rolled component classes (`.btn-*`, `.glass-*`) to reference canonical tokens (`background-color: var(--primary)` instead of `var(--color-cyan-500)`) — same rendered output, now token-driven.

**Net effect:** `main.css` is the single source of truth; current rendering is byte-for-byte unchanged; duplicate vocabularies unified with a clear deprecation trail.

> **Verification gate for the `main.css` change:** a visual smoke test (home, browse, watch, player, a modal, the spotlight carousel) confirming no rendered diff, since main.css affects all 96 components at once. jsdom does not catch cascade bugs — verify in-browser.

---

## 6. Component layer (defined now, built in P2–P6)

- **Foundation:** adopt shadcn-vue's `cn()` (clsx + tailwind-merge, at `src/lib/utils.ts`) and `cva` for variants. Every primitive uses these.
- **Primitive inventory + mapping** — existing `components/ui/` maps onto shadcn-vue ~1:1:

| Current (`components/ui/`) | shadcn-vue target | Variant / API notes |
|---|---|---|
| `Button.vue` (`primary/secondary/ghost/outline`, `sm/md/lg`) | `Button` (cva) | `primary→default`, **`secondary→` new `brand`/`pink` variant (not shadcn `secondary`)**, `ghost→ghost`, `outline→outline`, add `destructive`; sizes `sm/md/lg(/icon)` |
| `Card.vue` | `Card` (+ `CardHeader/Content/Footer`) | token-driven `bg-card` |
| `Badge.vue` | `Badge` | variants → `--primary/--success/--warning/--destructive-soft` |
| `Input.vue` | `Input` | `border-input`, `ring-ring` |
| `Select.vue` | `Select` | Reka UI listbox |
| `Modal.vue` | `Dialog` | Reka UI dialog + `glass-elevated` |
| `Tabs.vue` | `Tabs` | |
| `ContextMenu.vue` | `DropdownMenu` / `ContextMenu` | |
| (hand-rolled today) | `Tooltip`, `Popover`, `Switch`, `Checkbox` | net-new primitives shadcn-vue provides |

Variants are token-driven (`bg-primary`, `bg-destructive`, `data-accent` for the brand-pink path), so primitives inherit Neon Tokyo automatically.

---

## 7. Governance (rules — to be added to project memory / CLAUDE.md in P6)

1. **Use tokens — never hardcode** hex or off-palette Tailwind colors (`red-500`, `amber-500`, `emerald-500`, etc.). Status → `--success / --warning / --info / --destructive` (+ `-soft`).
2. **Reuse before building** — prefer an existing `components/ui/` (shadcn-vue) primitive before creating a new component.
3. **Cascade rules are load-bearing** — `.cta-*` stays layered; `.spotlight-frame` / `.shuffle-deck` / `.glass-card` stay unlayered. Don't "tidy" them. (Ref: `reference_tailwind_v4_css_cascade.md`.)
4. **Smoke-test i18n key paths** in-browser when changing `t(...)` paths (string-typed, not type-checked).
5. **Verify cascade/visual changes in a real browser**, not jsdom.

---

## 8. Next steps — phased migration roadmap

Each phase is independently shippable with zero breakage between them.

| Phase | Deliverable |
|---|---|
| **P1 (this effort)** | Spec + `main.css` consolidation (layered tokens, `@theme inline`, deprecation aliases). |
| **P2** | Install Reka UI + shadcn-vue CLI; scaffold `cn()` / `cva`; bring `Button` + `Card` onto tokens as the working proof. |
| **P3** | Swap the rest of `components/ui/` to shadcn-vue primitives behind the same import paths. |
| **P4** | Migrate high-traffic surfaces (home, browse, watch, player, nav) to tokens-only + primitives. |
| **P5** | Sweep remaining components; add a lint/stylelint rule flagging hardcoded hex + off-palette colors; wire into the deploy gate. |
| **P6** | Add governance rules (§7) to project memory / CLAUDE.md. |

---

## 9. Project metrics (per `.planning/CONVENTIONS.md` — no days/hours)

**This effort (P1):**
- **UXΔ = 0 (Ambiguous)** — by design, zero user-visible change now; value is structural (enables consistent UX later).
- **CDI = 0.04 * 8** — Spread: 1 file changed now (`main.css`) but it is consumed everywhere (high reach); Shift: low (additive, aliased). Effort_Fib: 8.
- **MVQ = Griffin 85% / 80%** — disciplined consolidation with a clear deprecation trail; slop-resistance from the zero-breakage + in-browser verification gate.

**Full roadmap (P1–P6), projected:**
- **UXΔ = +2 (Better)** — consistent status colors, unified primitives, fewer one-off visual bugs.
- **CDI = 0.12 * 55** — Spread: touches most of the 96 components by P5; Shift: moderate (token swaps, primitive adoption). Effort_Fib: 55.
- **MVQ = Phoenix 80% / 85%** — a from-the-ashes unification of three fragmented systems into one.

---

## 10. Out of scope

- Net-new visual identity / rebrand (we build on Neon Tokyo).
- A living in-app `/styleguide` gallery route (considered, deferred — can be a later phase).
- Backend / Go services (frontend design only).
- Changing the radial-gradient body background, glow aesthetic, or font choices.
