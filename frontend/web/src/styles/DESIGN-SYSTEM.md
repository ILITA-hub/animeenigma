# AnimeEnigma Design System — "Neon Tokyo"

Single source of truth for design tokens, anchored on **shadcn-vue** (Reka UI) conventions.
Implemented in `./main.css`. Full rationale: `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`.

## Token tiers

**Tier 1 — primitives (raw values, never reference these directly in components).**
Defined in the `@theme` block: `--color-base #08080f`, `--color-surface #11111c`,
`--color-cyan-400 #00d4ff / -500 #00b8e6 / -600 #009dcc`, `--color-pink-400 #ff4d8d / -500 #ff2d7c / -600 #e6196b`,
`--color-success #00ff9d`, `--color-warning #ffd600`.
Defined as `:root` foundation vars (not in `@theme`): `--surface-2 #161623`, `--elevated #1c1c2c`, `--violet #a78bfa`.

**Tier 2 — shadcn-vue semantic slots (use these in components).**

| Slot | Maps to | Utility examples |
|---|---|---|
| `--background` / `--foreground` | base / white | `bg-background`, `text-foreground` |
| `--card` / `--card-foreground` | surface / white | `bg-card` |
| `--popover` / `--popover-foreground` | elevated / white | `bg-popover` |
| `--primary` / `--primary-foreground` | cyan-500 / base | `bg-primary text-primary-foreground` |
| `--secondary` / `--secondary-foreground` | elevated neutral / white | `bg-secondary` (neutral, NOT pink) |
| `--muted` / `--muted-foreground` | surface-2 / white@56% | `bg-muted text-muted-foreground` |
| `--border` / `--input` / `--ring` | white@10% / white@12% / cyan-400 | `border-border`, `ring-ring` |
| `--radius` (+ `--r-sm…2xl`) | 0.75rem base | (use Tailwind `rounded-*`; `--r-*` for custom) |

> shadcn's `--accent` (a hover surface) is **deferred to P2** — see note below.

**Tier 3 — brand extension (Neon Tokyo).**

| Token | Use | Utility |
|---|---|---|
| `--brand-cyan` | glow/brand cyan | `bg-brand-cyan` |
| `--brand-pink` / `-foreground` | the pink CTA | `bg-brand-pink` |
| `--brand-violet` | tertiary accent | `bg-brand-violet` |
| `--success` / `--warning` / `--info` / `--destructive` (+ `-foreground`, `-soft`) | status colors | `bg-destructive`, `bg-success-soft` |
| `--glow-cyan` / `--glow-pink` / `--glow-sm-cyan` | accent elevation | (custom shadow) |

## Usage rules (governance)

1. **Use tokens — never hardcode** hex or off-palette Tailwind colors (`red-500`, `amber-500`, `emerald-500`…). Status → `--success / --warning / --info / --destructive` (+ `-soft`).
2. **Reuse before building** — prefer an existing `components/ui/` (shadcn-vue) primitive before creating a new component.
3. **Cascade rules are load-bearing** — `.cta-*` stays inside `@layer components`; `.spotlight-frame` / `.shuffle-deck` / `.glass-card` stay UNLAYERED. Don't "tidy" them. (Ref: `reference_tailwind_v4_css_cascade.md`.)
4. **Smoke-test i18n key paths** in-browser when changing `t(...)` paths.
5. **Verify cascade/visual changes in a real browser**, not jsdom.
6. **Score glyphs** — AnimeEnigma scores (site rating, user reviews, list scores, theme votes) render the cyan ◆ via the `ScoreDiamond` primitive (`components/ui/ScoreDiamond.vue`); the amber lucide `Star` is reserved for Shikimori/MAL scores. Never render a site score with a star or an inline diamond SVG.

## Type / spacing / radius / elevation

- Fonts: `--font-sans` (Inter+Noto JP) body, `--font-display` (Manrope) headings, `--font-mono` (JetBrains) code, `--font-jp` (Noto JP) subtitles.
- Spacing: Tailwind default 4px scale. Card padding `p-4 md:p-6 lg:p-8`. Touch target 44px (`.touch-target`).
- Radius: chips→sm, buttons/inputs→md/lg, cards→xl, modals→2xl.
- Elevation: `glass` (flat) → `glass-card` (resting) → `glass-elevated` (raised). Glows are accent elevation, not structural.

## Lint gate (enforced) — DS-GOV-01 / DS-GOV-02 / DS-GOV-03

A build-failing custom bash gate, `frontend/web/scripts/design-system-lint.sh` (mirrors
`scripts/i18n-lint.sh`), locks the color/token migration shut. It runs on the SAME path CI/deploy
already use: it is a prerequisite of **`make lint-frontend`** (→ `make lint` / `all` / CI) AND of
**`make redeploy-web`** (the deploy gate), via the `make lint-design` sub-target. No new dependency.

**It enforces EXACTLY these 6 rules** over `frontend/web/src/**/*.vue` (excluding `*.spec.*` /
`__tests__`; **Rules 1 & 4 also scan `*.ts`**, since class-strings leak via `cva` variant files).
Each violation is an ERROR; `ERRORS>0` ⇒ `exit 1`.

1. **Off-palette Tailwind color classes** — `(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50…975)` (the shade group now includes `925|950|975`).
   Migrate to a semantic token (`text-destructive`, `bg-warning`, `text-success`, `text-info`, `text-brand-violet`, `text-muted-foreground`, …). Also scans `*.ts`, **excluding `*-variants.ts`** (the canonical semantic-variant defs).
2. **Hardcoded hex outside the allowlist** — any raw `#[0-9a-fA-F]{3,8}` in a `.vue` (CSS `<style>` hex or Tailwind arbitrary `*-[#…]`) that is NOT listed in `scripts/design-system-allowlist.txt`. Allowance is per-`(file,hex)`: a hex is allowed only in the file that lists it. `.vue` only — `.ts` hex is intentional brand/provider color data (e.g. `providerRegistry.ts`).
3. **Deprecated-alias `var()` usages** — `var(--ink)` / `var(--accent)` / `var(--pink)` (after the Wave-2 flip, brand `--accent` usage is itself a violation), plus `var(--violet)` → `--brand-violet` and the font aliases `var(--f-display|f-ui|f-mono|f-jp)` → `--font-display`/`--font-sans`/`--font-mono`/`--font-jp` (migrated + locked in slice #2). EXCLUDES the literal-alias survivors `--ink-2`, `--ink-4`, `--accent-soft`, `--accent-line`, `--accent-glow`, `--pink-soft`.
4. **Off-scale font weights** — `font-(bold|extrabold|black|light|thin)`; the DS allows ONLY `font-medium` / `font-semibold`. Scans `*.vue` + `*.ts`. (**Promoted 2026-06-15 from governance-only to build-enforced.**)
5. **Bare `<select>` / `<input type="date">`** — use the `Select` / `DatePicker` primitives. Exempts `components/player/` (Reka portals break in fullscreen, so player pickers stay native) and `type="datetime-local"`.
6. **Arbitrary spacing values** — `(p|px|py|pt|pr|pb|pl|m|mx|my|mt|mr|mb|ml|gap|gap-x|gap-y|space-x|space-y)-[<n>px|rem|em]` dodge the 4px token scale; bind to a token instead (`px-[10px]` → `px-2.5`). On-grid values (2/4/6/8/10/14/18px) migrate 1:1 to tokens (`0.5/1/1.5/2/2.5/3.5/4.5`); `1px` → the bare `-px` modifier. **Sizing props (`w/h/min-*/max-*/size`) are DELIBERATELY OUT OF SCOPE** — no token scale exists for arbitrary pixel dimensions, so `w-[380px]` is fine. `calc()`/`var()` arbitrary values are allowed (computed, not magic numbers). Off-grid sub-pixel survivors (odd `3/5/7/9/11px` on the dense player menus + `Stepper`) are listed per-`(file,class)` in `scripts/design-system-spacing-allowlist.txt`. (DS-GOV-03, added 2026-06-17.)

**Brand-exemption rationale (why some hues are NOT "off-palette").** `cyan` and `pink` are the
Neon-Tokyo BRAND primitives, and `orange` / `rose` (plus `indigo` / `teal` / `lime`) are per-provider
identity hues (Kodik cyan, AniLib orange, Hanime pink, Raw rose). They are deliberately ABSENT from the
Rule-1 palette set — including them would (correctly) fail the clean tree where Anime.vue, the players,
and the Navbar legitimately use them. Provider/brand hex go on the allowlist instead, with a reason.

**Allowlist / escape-hatch (two files).** Hex exceptions → `frontend/web/scripts/design-system-allowlist.txt`
(`path:hex:reason`); arbitrary-spacing exceptions → `frontend/web/scripts/design-system-spacing-allowlist.txt`
(`path:class:reason`). One line per justified exception (`#`-prefixed comments allowed). To add an exception:
**prefer migrating to a token first**; only allowlist when no token reproduces the value within
tolerance — for hex: provider-brand identity, subtitle render default, cover/avatar gradient stop, near-base
ink, SVG `<stop>`, functional non-theme color like a QR-code module color, or a canonical-fine
`var(--token,#fallback)` fallback; for spacing: an off-grid odd-pixel value (`3/5/7/9/11px`) whose
sub-pixel tuning is load-bearing (snapping to the 4px scale would visibly shift a deliberately compact
surface). Every line MUST carry a real reason — never a blanket wildcard.

**Adjudication rule (out-of-scope hex).** Any hex discovered in a file NOT touched by the migration
MUST be ADJUDICATED — decide migrate-to-token vs justified-allowlist and record the reason — never
blanket-allowlisted to force the gate green. (Examples on record: `Auth.vue`'s `#54a9eb`/`#4a96d2` →
justified Telegram provider-brand allowlist; `Collections.vue`'s `#0e7490`/`#6b21a8` cover gradient →
adjudicated keep, no exact token pair.)

**Prove the fail-path** — the gate ships a `--selftest` that injects a scratch file with a
`bg-red-500` (Rule 1) AND a `p-[7px]` (Rule 6), asserts the gate DETECTS both (would `exit 1`),
removes it (trap-guarded), and asserts the clean tree PASSES — leaving the tree exactly as before:

```bash
bash frontend/web/scripts/design-system-lint.sh --selftest   # → SELFTEST PASS, tree clean
make lint-design                                              # → PASS on the clean tree
```

> Scope covers color/token discipline (Rules 1–3), the font-weight scale (Rule 4), native
> form-primitive bypass (Rule 5), and binding spacing to the 4px token scale (Rule 6). The
> **font-weight scale and native `<select>`/`date` checks were promoted out of Phase-6 governance into
> build-enforcement** (Rule 4 on 2026-06-15; Rule 6 added 2026-06-17). What REMAINS governance-only
> (human/AI-followed, NOT build-enforced — a grep cannot reliably distinguish these without AST
> analysis): **reuse-the-primitives** (hand-rolled button vs `<Button>`), the **contextual card-padding
> rhythm** (`p-4 md:p-6 lg:p-8` — Rule 6 only forbids *arbitrary* `p-[…px]`; it does NOT mandate which
> token step a card uses), and **`cva` variants for component variation**. The docs above match the
> enforced rules exactly (no documented-but-unenforced rule).

## Deprecated aliases (migrate away over P2–P5)

`--ink/-2/-3/-4` → `--foreground`/`--muted-foreground` ramp · `--pink` → `--brand-pink` ·
`--f-display/ui/mono/jp` → `--font-display`/`--font-sans`/`--font-mono`/`--font-jp` **(migrated + Rule-3 gated, slice #2)** ·
`--violet` → `--brand-violet` **(migrated + Rule-3 gated, slice #2)** ·
`--accent*` → `--brand-cyan` (the `--accent` brand→shadcn-hover flip already landed).

The deprecated `--f-*`/`--violet` aliases remain *defined* in `main.css` (pointing at their canonical) for any third-party/legacy reference, but are now build-forbidden in `src/**/*.vue` by Rule 3.

## Component inventory (target shadcn-vue mapping)

`Button`→Button(cva) · `Card`→Card · `Badge`→Badge · `Input`→Input · `Select`→Select ·
`Modal`→Dialog · `Tabs`→Tabs · `ContextMenu`→DropdownMenu · plus new: `Tooltip`, `Popover`, `Switch`, `Checkbox`,
`Stepper` (numeric −/value/+ input; props `modelValue`/`step`/`min`/`max`/`suffix`/`label`, rounds to step precision — added 2026-06-11).

## Button API (v2.0 — widened 2026-06-04)

The `Button` primitive (`components/ui/button-variants.ts` + `Button.vue`) is the single
source of truth — the old `.btn-*` CSS classes were deleted (v2.0 Phase 1). Reuse it before
hand-rolling a `<button>`.

- **variants:** `default` (cyan primary — `bg-primary text-primary-foreground`, hover-glow + `active:scale-95`) · `brand` (solid pink CTA) · `destructive` (solid red) · `ghost` (bordered quiet — `bg-white/5 border border-white/10`) · `soft` (BORDERLESS quiet — `bg-white/10`, no glow/scale) · `outline` (cyan text + border) · `link` (bare cyan text, `hover:underline`, padding zeroed via `px-0! py-0!`) · legacy `primary`/`secondary` aliases (back-compat, mirror default/brand).
- **sizes:** `xs` (`px-2 py-1 text-xs`) · `sm` · `md` (default) · `lg` · `icon` (40px) · `icon-sm` (32px).
- **props:** `radius` (`sm|md|lg|xl|full`, overrides the variant's baked corner via tailwind-merge) · `fullWidth` · `loading` (spinner + disabled) · `href` (renders `<a>`) · `disabled` · `type` · `class` (passthrough, merged).
- Glow uses the `--shadow-glow-cyan`/`--shadow-glow-pink` tokens (not raw rgba). Color contrast: `default`/`primary` MUST carry `text-primary-foreground` (near-black) — white-on-cyan fails WCAG.

### Bespoke button keeps (governance — NOT every `<button>` becomes `<Button>`)

The v2.0 swap was **value-preserving with minor normalization, color-family-preserving**. These
shapes legitimately stay raw `<button>` because the opinionated primitive can't model them without
a visible diff — do NOT force-swap them:

1. **Stateful / segmented toggles** — active/inactive conditional `:class` (language tabs, provider chips, quality/speed/source selectors, view toggles, `role="tab"`/`role="radio"`/`role="menuitemradio"` groups). One variant can't express two states.
2. **Translucent colored fills** — `bg-*/10`–`/30` tints (e.g. ghost-destructive delete buttons, `bg-cyan-500/10` chips). The solid variants would over-emphasize them.
3. **Icon-only, zero resting background + bespoke hover-tint** — `ghost`/`soft` add a resting bg/border these don't have.
4. **Overlay / transport controls** — play/pause/seek/PiP/settings positioned over `<video>` (absolute positioning, custom sizing, hover-reveal).
5. **Scoped-CSS buttons** — styling lives in a component `<style>` block (`.icon-btn-nt`, `.play-btn`, `.arrow-*`, `.feed-load-more`, `.update-row`, carousel dots/arrows).
6. **No matching variant** — brand hues with no token variant (`bg-brand-violet`, `bg-warning`).
7. **Whole-component / structural buttons** — `EpisodeCard`, `AnimeKebab`, full-bleed poster click targets, complex flex-layout rows.
8. **Sub-`xs` micro-controls** — e.g. `w-6 h-6` steppers smaller than `icon-sm`.
