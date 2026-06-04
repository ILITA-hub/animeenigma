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

## Type / spacing / radius / elevation

- Fonts: `--font-sans` (Inter+Noto JP) body, `--font-display` (Manrope) headings, `--font-mono` (JetBrains) code, `--font-jp` (Noto JP) subtitles.
- Spacing: Tailwind default 4px scale. Card padding `p-4 md:p-6 lg:p-8`. Touch target 44px (`.touch-target`).
- Radius: chips→sm, buttons/inputs→md/lg, cards→xl, modals→2xl.
- Elevation: `glass` (flat) → `glass-card` (resting) → `glass-elevated` (raised). Glows are accent elevation, not structural.

## Lint gate (enforced) — DS-GOV-01 / DS-GOV-02

A build-failing custom bash gate, `frontend/web/scripts/design-system-lint.sh` (mirrors
`scripts/i18n-lint.sh`), locks the color/token migration shut. It runs on the SAME path CI/deploy
already use: it is a prerequisite of **`make lint-frontend`** (→ `make lint` / `all` / CI) AND of
**`make redeploy-web`** (the deploy gate), via the `make lint-design` sub-target. No new dependency.

**It enforces EXACTLY these 3 rules** over `frontend/web/src/**/*.vue` (excluding `*.spec.*` /
`__tests__`). Each violation is an ERROR; `ERRORS>0` ⇒ `exit 1`.

1. **Off-palette Tailwind color classes** — `(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50…900)`.
   Migrate to a semantic token (`text-destructive`, `bg-warning`, `text-success`, `text-info`, `text-brand-violet`, `text-muted-foreground`, …).
2. **Hardcoded hex outside the allowlist** — any raw `#[0-9a-fA-F]{3,8}` in a `.vue` (CSS `<style>` hex or Tailwind arbitrary `*-[#…]`) that is NOT listed in `scripts/design-system-allowlist.txt`. Allowance is per-`(file,hex)`: a hex is allowed only in the file that lists it.
3. **Deprecated brand-alias `var()` usages** — `var(--ink)` / `var(--accent)` / `var(--pink)` (after the Wave-2 flip, brand `--accent` usage is itself a violation). EXCLUDES the literal-alias survivors `--ink-2`, `--ink-4`, `--accent-soft`, `--accent-line`, `--accent-glow`, `--pink-soft`.

**Brand-exemption rationale (why some hues are NOT "off-palette").** `cyan` and `pink` are the
Neon-Tokyo BRAND primitives, and `orange` / `rose` (plus `indigo` / `teal` / `lime`) are per-provider
identity hues (Kodik cyan, AniLib orange, Hanime pink, Raw rose). They are deliberately ABSENT from the
Rule-1 palette set — including them would (correctly) fail the clean tree where Anime.vue, the players,
and the Navbar legitimately use them. Provider/brand hex go on the allowlist instead, with a reason.

**Allowlist / escape-hatch** — `frontend/web/scripts/design-system-allowlist.txt`, one
`path:hex:reason` line per justified exception (`#`-prefixed comments allowed). To add an exception:
**prefer migrating to a token first**; only allowlist when no token reproduces the value within
tolerance (provider-brand identity, subtitle render default, cover/avatar gradient stop, near-base ink,
SVG `<stop>`, functional non-theme color like a QR-code module color, or a canonical-fine
`var(--token,#fallback)` fallback). Every line MUST carry a real reason — never a blanket wildcard.

**Adjudication rule (out-of-scope hex).** Any hex discovered in a file NOT touched by the migration
MUST be ADJUDICATED — decide migrate-to-token vs justified-allowlist and record the reason — never
blanket-allowlisted to force the gate green. (Examples on record: `Auth.vue`'s `#54a9eb`/`#4a96d2` →
justified Telegram provider-brand allowlist; `Collections.vue`'s `#0e7490`/`#6b21a8` cover gradient →
adjudicated keep, no exact token pair.)

**Prove the fail-path** — the gate ships a `--selftest` that injects a scratch `bg-red-500` file,
asserts the gate DETECTS it (would `exit 1`), removes it (trap-guarded), and asserts the clean tree
PASSES — leaving the tree exactly as before:

```bash
bash frontend/web/scripts/design-system-lint.sh --selftest   # → SELFTEST PASS, tree clean
make lint-design                                              # → PASS on the clean tree
```

> Scope is intentionally narrow (color/token discipline only). Structural conventions
> (use-the-primitives, font-weight scale, padding scale, `cva` variants) are Phase-6 governance
> documentation, NOT build-enforced — a grep cannot reliably distinguish a hand-rolled button from a
> primitive without AST analysis. The docs above match the enforced rules exactly (no
> documented-but-unenforced rule).

## Deprecated aliases (migrate away over P2–P5)

`--ink/-2/-3/-4` → `--foreground`/`--muted-foreground` ramp · `--pink` → `--brand-pink` ·
`--f-display/ui/mono/jp` → `--font-*` · `--accent*` → `--brand-cyan` (kept until P2 repoints usages, then `--accent` flips to the shadcn hover surface).

## Component inventory (target shadcn-vue mapping)

`Button`→Button(cva) · `Card`→Card · `Badge`→Badge · `Input`→Input · `Select`→Select ·
`Modal`→Dialog · `Tabs`→Tabs · `ContextMenu`→DropdownMenu · plus new: `Tooltip`, `Popover`, `Switch`, `Checkbox`.

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
