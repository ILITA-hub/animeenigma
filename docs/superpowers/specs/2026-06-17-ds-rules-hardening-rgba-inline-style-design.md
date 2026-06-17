# Design — Harden DS lint (Rules 6 & 7) + curated rgba/hsl migration

**Date:** 2026-06-17
**Status:** Approved (brainstorming) — pending implementation plan
**Owner decisions (locked):** full-migrate · curated snap scale · value-based token naming (`--white-a8`/`--cyan-a20`) · include gacha in this pass

## Metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = 0 (Ambiguous)** — no intended user-visible change; curated snapping introduces only sub-perceptual alpha shifts (≤ ~0.04 delta), and a few intended ones are documented below.
- **CDI = 0.06 * 21** — Spread: touches ~40 `.vue` files + `main.css` + the lint script (wide). Shift: low per-file (mechanical literal→token swaps). Effort_Fib 21.
- **MVQ = Griffin 88%/82%** — disciplined, rule-bounded consolidation with a provable fail-path; slop-resistance high because the gate enforces itself going forward.

## Problem

`design-system-lint.sh` enforces 5 rules but has two gaps:

1. **Rule 2 only catches `#hex`.** Raw `rgba()/hsl()` color literals slip through. Today the tree carries **277 rgba/hsl literals** across `.vue` files — ~30 distinct white-overlay opacities, ~21 brand-cyan opacities, black scrims, dark-bg scrims, and provider/rarity identity hues.
2. **No class-based rule can see inline `style`.** `style="…"` / `:style="'…'"` string literals dodge every rule. **74 inline-style usages** exist; a subset hardcode color.

Goal: close both gaps with build-enforced rules, and migrate existing literals to a **small curated token set** (not one-token-per-opacity).

## Data (audit performed 2026-06-17)

| Cluster | Occurrences | Distinct values | Existing exact-token coverage |
|---|---|---|---|
| White `rgba(255,255,255,a)` | ~124 | ~30 (.01→.85) | .06→`--line`, .10→`--border`, .12→`--line-strong`/`--input`, .36→`--ink-4`, .56→`--muted-foreground`, .78→`--ink-2` |
| Brand-cyan `rgba(0,212,255,a)` | ~52 | ~21 (.05→.95) | .14→`--accent-soft`, .28→`--accent-line`, glow→`--shadow-glow-cyan`/`-sm-cyan` |
| Black scrim `rgba(0,0,0,a)` | ~26 | ~14 (.25→.82) | none |
| Dark-bg scrim `rgba(8,8,15,a)` | ~12 | ~12 (.1→.96) | none |
| Provider/rarity hues (teal/orange/indigo/violet/pink rgb) | ~30 | identity colors, mostly gacha | `--violet`/`--brand-violet` (#a78bfa) only |
| Opaque dark gradient stops `rgb(11,11,24)`… | ~8 | decorative ramps | none |
| `rgba(var(--player-accent-rgb), a)` | 8 | already token-based | exempt by var() rule |

Distribution by context (of 277): **~253 inside `<style>` blocks** (take `var(--token)` cleanly), **13 in class arbitrary-values** (`shadow-[…rgba…]`, can use Tailwind `white/8` alpha utilities or `var()`), **11 in inline `style`/`:style` attrs**.

## Rule 6 — no raw rgba/hsl literals in `.vue`

- **Match:** both comma form `rgba(0,212,255,.4)` and modern space/slash form `rgb(34 211 238 / .4)` (the latter already exists in `Anime.vue` and the draft proposal's regex would miss it). Covers `rgb`, `rgba`, `hsl`, `hsla`.
- **Scope:** `*.vue` only (mirrors Rule 2; `.ts` color data such as `providerRegistry.ts` stays intentional). Excludes `*.spec.*` / `__tests__`.
- **Exemptions:**
  - `rgb*(var(--…))` / `hsl*(var(--…))` — already token-based.
  - Allowlist lines in `design-system-allowlist.txt` using the existing `path:value:reason` format, extended so `value` may be an rgba/hsl literal (normalized whitespace-insensitively before compare).
- **Severity:** ERROR (fails build). New per-rule counter `RULE6_ERRORS`.

## Rule 7 — no static color inside inline `style`

- **Match:** a `#hex`, `rgb(`, or `hsl(` appearing **inside** a `style="…"` attribute or a `:style="'…literal…'"` string literal.
- **Explicitly NOT flagged:** dynamic object/array bindings (`:style="{ width: pct + 'px' }"`), `px`/`%`/`transform`/layout values. *Rationale:* color is the DS concern; layout values are legitimately dynamic. (This is the correction to the originally-proposed `style=.*(#|rgb|px)`, which would have flagged every dynamic width/transform in the app.)
- **Scope/exemptions:** same `.vue`-only, spec/test-excluded, allowlist-aware.
- **Severity:** ERROR. Counter `RULE7_ERRORS`.
- After migration, the 11 inline-style color literals are gone; Rule 7 primarily guards the future.

## Token additions — `frontend/web/src/styles/main.css`

Naming: **value-based** (`--<color>-a<alpha×100>`), reusing existing tokens first. ~13 new tokens.

### White overlay scale (new)
```
--white-a4:  rgba(255, 255, 255, 0.04);
--white-a8:  rgba(255, 255, 255, 0.08);
--white-a20: rgba(255, 255, 255, 0.20);
--white-a30: rgba(255, 255, 255, 0.30);
```
Snap table (→ = maps to):
- .01, .025, .03, .04, .05 → `--white-a4`
- .07, .08 → `--white-a8`   ·   .06 → `--line` (border use) / `--white-a8` is NOT used for .06
- .10 → `--border`   ·   .11, .12, .14 → `--line-strong`
- .16, .18, .20, .22, .25 → `--white-a20`
- .28, .30, .35 → `--white-a30`
- .36, .40 → `--ink-4`   ·   .45, .55, .56, .60 → `--muted-foreground`   ·   .65, .70, .78, .85 → `--ink-2`

Intended (documented) larger shifts: .45→.56 and .65→.78 move ~0.11–0.13; these are sparse (1–2 sites each, mostly text/icon tints) and visually acceptable. Any site where the reviewer judges the shift wrong falls back to the nearest new token or an allowlist entry.

### Brand-cyan alpha scale (new)
```
--cyan-a08: rgba(0, 212, 255, 0.08);
--cyan-a20: rgba(0, 212, 255, 0.20);
--cyan-a40: rgba(0, 212, 255, 0.40);
--cyan-a60: rgba(0, 212, 255, 0.60);
```
Snap: .05–.10 → `--cyan-a08` · .12–.16 → `--accent-soft` (.14) · .18–.22 → `--cyan-a20` · .25–.30 → `--accent-line` (.28) · .35–.50 → `--cyan-a40` · .60 → `--cyan-a60` · .80–.95 → `--brand-cyan` (solid).

### Black scrim scale (new)
```
--black-a40: rgba(0, 0, 0, 0.40);
--black-a60: rgba(0, 0, 0, 0.60);
--black-a80: rgba(0, 0, 0, 0.80);
```
Snap: .25–.45 → a40 · .50–.66 → a60 · .70–.82 → a80. (Box-shadow uses keep their geometry; only the color term swaps to the token.)

### Dark-bg scrim (new)
```
--scrim-bg-soft:   rgba(8, 8, 15, 0.40);
--scrim-bg-strong: rgba(8, 8, 15, 0.85);
```
Snap: .1–.4 → soft · .65–.96 → strong. (`rgba(8,8,15,0)` transparent gradient endpoints → keep as `transparent` keyword, no token.)

## Allowlist additions — `design-system-allowlist.txt`

Documented `path:value:reason` lines (NOT tokenized — identity/decorative data, analogous to `providerRegistry.ts`):

- **Gacha rarity-tier gradient triplets** — `rgb(45,212,191)` teal, `rgb(251,146,60)` orange, `rgb(129,140,248)` indigo, `rgb(167,139,250)` violet, `rgb(255,77,141)`/`rgb(255,45,124)` pink — in `CardViewer3D.vue`, `DropsModal.vue`, `GemCeremony.vue`, `PullSummary.vue`, `SpinDock.vue` etc. Reason: per-rarity identity palette (Common/Rare/Epic/Legendary/Mythic), data-driven gradient stops.
- **One-off opaque dark gradient stops** — `rgb(11,11,24)`, `rgb(13,13,28)`, `rgb(16,16,28)`, `rgb(19,19,38)`, `rgb(22,22,35)`, `rgb(26,26,46)`, `rgb(10,10,15)`, `rgb(2,2,8)` family — spotlight/gacha decorative backdrop ramps. Reason: bespoke gradient ramps, not part of the surface token ladder.

Gacha's **white/cyan/black** literals ARE migrated to the new tokens this pass (only rarity identity hues are allowlisted), per the "include gacha now" decision.

## Migration plan (high level — detailed in the implementation plan)

1. Add the ~13 new tokens to `main.css` `:root`.
2. Extend `design-system-lint.sh`: Rule 6, Rule 7, counters, Summary lines, `--selftest` cases; teach the allowlist matcher to accept rgba/hsl literals (whitespace-normalized).
3. Add allowlist lines for gacha rarity hues + dark gradient stops.
4. Migrate literals → tokens/Tailwind-alpha, cluster by cluster (white → cyan → black → dark-bg → inline-style → class-arbitrary). ~40 files.
5. Update CLAUDE.md "Lint gate" section (5 rules → 7) and `DESIGN-SYSTEM.md` token tier list.

## Verification

- `bash frontend/web/scripts/design-system-lint.sh --selftest` — proves Rule 6 & Rule 7 fail-paths (inject scratch `rgba(1,2,3,.5)` and `style="color:#abc"`, assert detection, assert clean tree passes).
- `bash frontend/web/scripts/design-system-lint.sh` exits 0 on the migrated tree.
- `cd frontend/web && bunx tsc --noEmit` and `bunx vitest run` green.
- **Cascade caveat:** jsdom/vitest cannot catch Tailwind-v4 unlayered-class cascade bugs, and this pass touches `<style>` blocks + `main.css` tokens. Offer an **opt-in** Chrome smoke at the end (per `feedback_chrome_smoke_opt_in`), focused on player overlays, gacha (admin), home rows, and modals.
- Ship via `/animeenigma-after-update` (redeploy-web runs the lint gate as a hard prereq).

## Risks & mitigations

- **Visual regression from snapping** — mitigated by ≤~0.04 deltas for the common cases; the few larger documented shifts are sparse and reviewable; opt-in Chrome smoke as backstop.
- **Tailwind cascade (unlayered custom classes beat utilities)** — only relevant if a migration moves a value from `<style>` to a utility; prefer `var()` in-place in `<style>` blocks to avoid introducing new cascade surfaces.
- **Allowlist drift** — every allowlist line carries a reason; gacha hues are the only broad exemption and are bounded to identity colors.
- **Shared-tree concurrency** — path-scoped commits only (`git commit <pathspec>`), `git show --stat HEAD` after each, push after each (per memory: realtime-backup + bare-commit hazard).

## Out of scope

- `hsl()`/`rgb()` in `.ts` files (intentional brand/provider data).
- Rarity-tier tokenization (`--rarity-*`) — deferred; allowlist for now.
- Migrating `<style>`-block geometry/spacing (only color literals are in scope).
