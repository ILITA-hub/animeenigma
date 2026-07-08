# Design-System lint rules (build-enforced)

`frontend/web/scripts/design-system-lint.sh` is a prerequisite of `make lint-frontend` (→ `make lint` / CI) AND `make redeploy-web` (deploy gate). `ERRORS>0 ⇒ exit 1` — **FAILS THE BUILD.** It enforces 8 color/token/typography/spacing/primitive rules over `frontend/web/src/**/*.vue` (excludes `*.spec.*` / `__tests__`; Rules 1 & 4 also scan `*.ts`).

A `PostToolUse` hook (`.claude/hooks/ds-lint-postedit.sh`) runs the gate automatically on every `frontend/web/src/**/*.{vue,ts}` edit, so violations surface at edit time, not just at build/deploy. The script itself is the ultimate source of truth (regexes + `--selftest`); this doc mirrors it for reading.

## The 8 rules

1. **No off-palette Tailwind color classes** — `(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50…975)` (shades incl. `925|950|975`). Migrate to a semantic token (`text-destructive`, `bg-warning`, `text-success`, `text-info`, `text-muted-foreground`, …). **EXEMPT brand/provider identity hues** (deliberately absent from the set, NOT forbidden): `cyan`, `pink`, `orange`, `rose`, `indigo`, `teal`, `lime` (Neon-Tokyo + per-provider accents — Kodik cyan, AniLib orange, Hanime pink, Raw rose). Also scans `*.ts` (cva variant class-strings), **excluding `*-variants.ts`** (canonical semantic-variant defs).
2. **No hardcoded hex outside the allowlist** — any raw `#[0-9a-fA-F]{3,8}` in a `.vue` not listed per `(file,hex)` in `frontend/web/scripts/design-system-allowlist.txt`. `.vue` only — `.ts` hex is intentional brand/provider color data (e.g. `providerRegistry.ts`).
3. **No deprecated-alias `var()`** — `var(--ink|--accent|--pink|--violet|--f-display|--f-ui|--f-mono|--f-jp)`. Migrate to canonical (`--brand-violet`, `--font-display`, `--font-sans`, `--font-mono`, `--font-jp`). (Survivors kept: `--ink-2`, `--ink-4`, `--accent-soft`, `--accent-line`, `--accent-glow`, `--pink-soft`.)
4. **No off-scale font weights** — `font-(bold|extrabold|black|light|thin)`; DS allows only `font-medium` / `font-semibold`. Scans `*.vue`+`*.ts`. (Build-enforced since 2026-06-15.)
5. **No bare native form controls** — `<select>`, `<input type="date|checkbox|radio">`; use `Select`/`DatePicker`/`Checkbox`/`Switch`/`RadioGroup` primitives. Exempts `components/player/` (reka portals break in fullscreen → player pickers stay native) and `type="datetime-local"`. **Per-site escape hatch:** a `bespoke-keep` comment within 6 lines above the control exempts it (justify inline). This is the *only* enforceable slice of "reuse primitives" — native-element controls are greppable; div-composition primitives (Card/Badge/Dialog/Tabs/etc.) stay governance-only below.
6. **No arbitrary spacing values** — `(p|px|py|pt|pr|pb|pl|m|mx|my|mt|mr|mb|ml|gap|gap-x|gap-y|space-x|space-y)-[<n>px|rem|em]` bypass the 4px token scale; use a token (`px-[10px]`→`px-2.5`; `1px`→`-px`). **Sizing props (`w/h/min-*/max-*/size`) OUT OF SCOPE** (no token scale for arbitrary px dims); `calc()`/`var()` allowed. Off-grid odd-pixel survivors (`3/5/7/9/11px` on dense player menus + `Stepper`) allowlisted per-`(file,class)` in `frontend/web/scripts/design-system-spacing-allowlist.txt`. (Added 2026-06-17.)
7. **No raw `rgba()`/`hsl()` color literals in `.vue`** — both comma `rgba(0,0,0,.5)` and space/slash `rgb(0 0 0 / .5)`; bind to an alpha token (`--white-a4/a8/a20/a30`, `--cyan-a08/a20/a40/a60`, `--black-a40/a60/a80`, `--scrim-bg-soft/strong`, or a semantic `*-soft`). `rgb*(var(--…))` exempt; `.vue` only (`.ts` color data stays intentional). Identity/decorative literals (gacha rarity hues, gradient ramps) allowlisted per-`(file,value)` in `design-system-allowlist.txt`. Tokens are a curated snap scale — see `docs/superpowers/specs/2026-06-17-ds-rules-hardening-rgba-inline-style-design.md`. (Added 2026-06-17.)

   > Gotcha: the regex matches NUMERIC-only literals, so an *interpolated* `rgba(0, 0, 0, ${x})` in a `.vue` does NOT match. Cleaner still: put dynamic color math in a `.ts` helper — the rule is `.vue`-only.
8. **No static color inside inline `style`** — `style="…"` / `:style="'…'"` carrying `#hex` / `rgb()` / `hsl()`; use a class or token. Dynamic bindings (`:style="{ width: pct }"`) and px/layout values NOT flagged (color is the DS concern, layout isn't). (Added 2026-06-17.)

## Escape-hatch

Prefer migrating to a token; only when none reproduces the value, add a justified line — `path:hex:reason` / `path:value:reason` in `design-system-allowlist.txt` (Rules 2/7/8) or `path:class:reason` in `design-system-spacing-allowlist.txt` (Rule 6) — **never disable the gate.** Prove the fail-path with `bash frontend/web/scripts/design-system-lint.sh --selftest`. Since 05-04, `--accent` is the shadcn hover surface — use `--brand-cyan` for brand cyan.

## Structural rules (GOVERNANCE-ONLY — human/AI-followed, NOT build-enforced)

A grep can't AST-distinguish these:

- Reuse `@/components/ui` primitives before building new (Button/Card/Badge/Input/Select/Dialog/Tabs/DropdownMenu/Tooltip/Popover/Switch/Checkbox).
- Only `font-medium`/`font-semibold` weights. · Padding scale (card `p-4 md:p-6 lg:p-8`). · `cva` variants for component variation.
