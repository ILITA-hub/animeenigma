# AnimeEnigma Design System — "Neon Tokyo"

Single source of truth for design tokens, anchored on **shadcn-vue** (Reka UI) conventions.
Implemented in `./main.css`. Full rationale: `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`.

## Token tiers

**Tier 1 — primitives (raw values, never reference these directly in components).**
`--color-base #08080f`, `--color-surface #11111c`, `--surface-2 #161623`, `--elevated #1c1c2c`,
`--color-cyan-400 #00d4ff / -500 #00b8e6 / -600 #009dcc`, `--color-pink-400 #ff4d8d / -500 #ff2d7c / -600 #e6196b`,
`--color-success #00ff9d`, `--color-warning #ffd600`, `--violet #a78bfa`.

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

## Deprecated aliases (migrate away over P2–P5)

`--ink/-2/-3/-4` → `--foreground`/`--muted-foreground` ramp · `--pink` → `--brand-pink` ·
`--f-display/ui/mono/jp` → `--font-*` · `--accent*` → `--brand-cyan` (kept until P2 repoints usages, then `--accent` flips to the shadcn hover surface).

## Component inventory (target shadcn-vue mapping)

`Button`→Button(cva) · `Card`→Card · `Badge`→Badge · `Input`→Input · `Select`→Select ·
`Modal`→Dialog · `Tabs`→Tabs · `ContextMenu`→DropdownMenu · plus new: `Tooltip`, `Popover`, `Switch`, `Checkbox`.
Button variant map: `primary→default`, `secondary→brand` (pink, NOT shadcn secondary), `ghost→ghost`, `outline→outline`, add `destructive`.
