# Phase 4: High-Traffic Surface Migration - Research

**Researched:** 2026-06-02
**Domain:** Value-preserving CSS/token migration (Vue 3 SFC + Tailwind v4 + shadcn-vue/Reka tokens)
**Confidence:** HIGH (entire scope is local code, grep-verified in this session)

## Summary

This phase migrates AnimeEnigma's highest-traffic Vue surfaces (Home, Browse, anime
detail = `views/Anime.vue`, the 5 players, nav/layout, anime-card children) off
off-palette Tailwind color classes and hardcoded `#hex` onto the existing canonical
token system and `@/components/ui` primitives. The milestone premise is **"zero rendered
change"** — every migrated surface must render pixel-identically. There is no design to
invent: the token system (`main.css`), the off-palette→semantic mapping (DS-MIGRATE-02),
the button-variant map (Phase 2), and the primitive barrel (Phases 2–3) are all locked.

The entire scope is **local source code**, so the research is an exhaustive,
grep-verified inventory rather than ecosystem discovery. I enumerated every off-palette
class and every hex literal across the 23 in-scope files: **55 off-palette occurrences**
and **36 hex literals** (the latter includes `var(--token, #fallback)` fallbacks and
several legitimately-novel render/brand values that must NOT be blindly snapped). I
identified the cascade footgun zone (`.cta-*` layered vs `.spotlight-frame`/`.glass-card`
unlayered), the per-player `--player-accent` brand hues, and the hand-rolled buttons with
primitive analogs vs the bespoke chrome with none.

**Primary recommendation:** Migrate in four plan buckets (Home+Browse+catalog cards →
nav/layout → anime-detail+children → the 5 players), apply the locked color mapping
per-occurrence, treat the flagged novel-hex set as human-judgment-per-case (add a token,
never inline), repoint deprecated aliases to canonical names WITHOUT flipping `--accent`,
and gate every bucket on the acceptance grep (zero hits) + the standing 5-surface
in-browser smoke at desktop+mobile (jsdom cannot catch cascade regressions).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Color → token mapping (DS-MIGRATE-02), applied per-occurrence with semantic judgment:**
  - `red` → `destructive`
  - `amber`/`yellow` → `warning`
  - `emerald`/`green` → `success`
  - `blue`/`sky` → `info`
  - `purple`/`violet` → `brand-violet`
  - `gray`/`slate`/`zinc` → `muted`/`card`/`border` (judgment per role)
- **Hardcoded hex (DS-MIGRATE-03):** Replace `#hex` in `.vue` with nearest canonical
  token. If legitimately novel (no token within tolerance), **add a new token to
  `main.css`** rather than inlining.
- **Deprecated aliases (DS-MIGRATE-04, partial):** Repoint `var(--ink)`/`var(--accent)`/
  `var(--pink)` brand usages in high-traffic files to canonical names. **The `--accent`
  semantic flip itself stays in Phase 5 — Phase 4 only repoints usages.**
- **Primitive adoption (DS-MIGRATE-06, partial):** Replace hand-rolled buttons/cards/
  badges with `@/components/ui` primitives where an equivalent exists; leave bespoke
  structures with no analog structurally intact (still token-only colors).
- **Button variant map (Phase 2, locked):** `primary→default` (cyan), `secondary→brand`
  (pink, NOT shadcn secondary), `ghost→ghost`, `outline→outline`, plus `destructive`.
  Sizes `sm/md/lg/icon`.

### Claude's Discretion
- Per-occurrence semantic judgment within the fixed mapping (is this gray a `muted`
  background, a `card` surface, or a `border`?).
- Order of surface migration; how to batch into plans/waves.
- When a hex is "novel enough" to warrant a new token vs. snap to an existing one.

### Deferred Ideas (OUT OF SCOPE)
- `--accent` semantic flip + brand-cyan alias deletion → Phase 5 (DS-MIGRATE-05).
  **DO NOT flip `--accent` in this phase.**
- Tail component sweep + lint enforcement gate (DS-GOV-01) → Phase 5.
- Governance into memory/CLAUDE.md → Phase 6.
- The 6 app-specific composites (`ButtonGroup`, `GenreFilterPopup`, `PaginationBar`,
  `SearchAutocomplete`, `Skeleton`, `Toaster`) — out of scope for the primitive *swap*
  (they get token re-pointing only if touched, but no structural replacement).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DS-MIGRATE-01 | High-traffic surfaces use ONLY tokens + `ui/` primitives — no off-palette, no hardcoded hex | Full per-file inventory below; acceptance grep defined in Validation Architecture |
| DS-MIGRATE-02 (partial) | Off-palette → semantic tokens, per-occurrence judgment | 55 occurrences enumerated with exact line/class; mapping locked above |
| DS-MIGRATE-03 (partial) | Hardcoded hex → tokens (or new token if novel) | 36 hex literals enumerated; novel-value allowlist candidates flagged |
| DS-MIGRATE-04 (partial) | Deprecated alias usages repointed to canonical names | `var(--ink/--accent/--pink)` usages enumerated; alias→canonical map verified from main.css |
| DS-MIGRATE-06 (partial) | Hand-rolled buttons/cards/badges → `ui/` primitives where they exist | Primitive-analog table below (NotFound `.btn`, player chrome buttons, status badges) |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Color token resolution | `main.css` (`@theme inline` + `:root`) | — | Single source of truth; surfaces only reference utilities/vars |
| Off-palette → semantic mapping | Vue SFC `<template>`/`<style>` | — | Pure presentation-layer edit; no backend/state involvement |
| Hardcoded hex elimination | Vue SFC `<style scoped>` | `main.css` (new tokens if novel) | Render values live in component CSS; novel ones promote to a token |
| Primitive adoption | `@/components/ui` barrel | consuming SFC | Buttons/cards/badges import from barrel; barrel owns the variant API |
| Cascade ordering | `main.css` `@layer components` vs unlayered | — | Load-bearing; never "tidied" by surface edits (DS-NF-02) |
| Visual-regression verification | Browser (Chrome MCP) | Vitest + vue-tsc (structural only) | jsdom cannot catch cascade/render regressions (DS-NF-06) |

## Standard Stack

No new dependencies (DS-NF-03 — Phase 4 adds ZERO npm deps). The migration consumes
what already exists.

### Core (already installed — consume, do not add)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Vue | 3.x | SFC framework | [VERIFIED: package.json] project standard |
| Tailwind v4 | 4.x | Utility CSS bound to `@theme inline` tokens | [VERIFIED: main.css `@theme inline`] token utilities generated here |
| reka-ui | (Phase 2–3) | shadcn-vue primitive substrate | [VERIFIED: `@/components/ui/index.ts` re-exports `reka-ui` DropdownMenu parts] |
| vue-tsc | (devDep) | Type-check gate (`vue-tsc --noEmit`) | [VERIFIED: package.json `type-check` script] |
| vitest | ^4.1.6 | jsdom mount tests (structural, NOT visual) | [VERIFIED: package.json devDep + vitest.config.ts] |
| @playwright/test | (devDep) | e2e specs (spotlight, player, home, anime) | [VERIFIED: playwright.config.ts + e2e/*.spec.ts] |

**Installation:** none. `cd frontend/web && bun install` only if a checkout is fresh.

### `@/components/ui` barrel (verified exports — the migration target)
[VERIFIED: `frontend/web/src/components/ui/index.ts`, read this session]

`Button` (+ `buttonVariants`), `ButtonGroup`, `Badge` (+ `badgeVariants`), `Card`,
`CardHeader`, `CardTitle`, `CardContent`, `CardFooter`, `Checkbox`, `DropdownMenu`
(+ re-exported `DropdownMenuItem`/`DropdownMenuSeparator`/`DropdownMenuLabel`),
`GenreFilterPopup`, `Input`, `Modal` (alias `Dialog`), `PaginationBar`, `Popover`,
`SearchAutocomplete`, `Select` (+ `SelectOption` type), `Skeleton`, `Switch`, `Tabs`,
`Tooltip`.

## Architecture Patterns

### Migration data flow

```
Off-palette class / hex literal in SFC
        │
        ├─ is it a Tailwind color utility (e.g. text-amber-400)?
        │      └─ YES → map per DS-MIGRATE-02 → semantic utility (text-warning)
        │
        ├─ is it a #hex in <style scoped>?
        │      ├─ matches a canonical token value → replace with var(--token)
        │      ├─ is a var(--alias, #fallback) fallback → repoint alias name (DS-MIGRATE-04)
        │      └─ legitimately novel (no token in tolerance) → ADD token to main.css, reference it
        │
        ├─ is it a hand-rolled button/card/badge?
        │      ├─ has a @/components/ui analog → swap to primitive (DS-MIGRATE-06)
        │      └─ bespoke chrome, no analog → keep structure, token-only colors
        │
        └─ EVERY change → acceptance grep (zero hits) + in-browser smoke (zero pixel diff)
```

### Recommended plan/wave buckets (discretion — recommended grouping)

```
Plan A — Catalog surfaces:  views/Home.vue, views/Browse.vue,
                            components/anime/{AnimeCardNew, AnimeCard, EpisodeCard,
                            GenreChip, AnimeContextMenu, AnimeKebab, AnimeQuickNav}.vue
Plan B — Nav/layout:        components/layout/{Navbar, BrandMark, FeedbackButton}.vue
                            + views/NotFound.vue (404 smoke target; carries hand-rolled .btn)
Plan C — Anime detail:      views/Anime.vue (13 off-palette occurrences — heaviest single file)
Plan D — Players (×5):      components/player/{AnimeLibPlayer, HanimePlayer, KodikPlayer,
                            OurEnglishPlayer, RawPlayer, OtherSubsPanel, ResumePill,
                            SubtitleSettingsMenu, SubtitleOverlay}.vue
```

Rationale: each bucket maps to exactly one of the 5 smoke surfaces (Home/Browse → Plan A;
nav+404 → Plan B; anime detail → Plan C; a player → Plan D), so each plan is
independently shippable (DS-NF-05) and verifiable against its own smoke surface.

### Anti-Patterns to Avoid
- **Tidying the cascade (DS-NF-02):** Do NOT move `.cta-*` out of `@layer components`,
  and do NOT wrap `.spotlight-frame`/`.shuffle-deck`/`.glass-card` in a layer.
  Unlayered custom classes intentionally beat utilities; a second plain `@layer
  components` block gets tree-shaken (see Pitfall 1).
- **Blind hex→token snapping:** Several hex are render values or brand hues that have
  no exact token; snapping to the nearest token changes rendering. See "Novel-hex
  allowlist candidates" — judge per case.
- **Flipping `--accent`:** Explicitly Phase 5. `--accent` currently aliases
  `--brand-cyan`; repoint usages but leave the alias mapping alone.
- **`<component :is>` for primitive dispatch:** keep typed import chains so `vue-tsc`
  narrows props (project convention, MEMORY: vue-tsc false-pass risk).

## Per-File Inventory (grep-verified, 2026-06-02)

> Acceptance regex used for off-palette:
> `(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50|100|200|300|400|500|600|700|800|900)`
> Note: `data-accent="purple"` style attribute values are NOT flagged (no `-NNN` suffix) —
> verified this session. The spotlight `.cta-hero[data-accent="..."]` family is therefore
> NOT caught by the grep and is out of scope (it lives in `components/home/spotlight/`, Phase 5).

### Off-palette color classes — 55 occurrences across 12 files [VERIFIED: grep]

| File | Count | Exact matches (line: class) |
|------|------:|------------------------------|
| `views/Anime.vue` | 13 | 82/628/704/709 `text-amber-400`; 249 `bg-amber-500/20`,`bg-amber-500/30`,`border-amber-500/30`; 442 `bg-emerald-500/20`,`border-emerald-500/50`,`text-emerald-400`; 696/805 `text-purple-400` |
| `views/Home.vue` | 3 | 76/102/126 `text-gray-400` |
| `views/Browse.vue` | 0 | (already clean — scout list was conservative) |
| `views/NotFound.vue` | 0 | (no off-palette classes; carries **hex** instead — see hex table) |
| `components/anime/AnimeCardNew.vue` | 5 | 41 `bg-amber-500/90`; 89 `bg-purple-500/80`; 218 `bg-emerald-500/80`; 219 `bg-amber-500/80`; 220 `bg-red-500/80` |
| `components/anime/AnimeContextMenu.vue` | 1 | 28 `text-amber-400` |
| `components/anime/EpisodeCard.vue` | 1 | 28 `bg-emerald-500` |
| `components/anime/{AnimeCard,AnimeKebab,AnimeQuickNav,GenreChip}.vue` | 0 | (clean of off-palette; AnimeCard carries hex) |
| `components/layout/FeedbackButton.vue` | 6 | 17 `text-green-400`; 34 `bg-purple-500/20`,`border-purple-400/50`; 76 `bg-purple-500/20`,`bg-purple-500/30`,`text-purple-300` |
| `components/layout/{Navbar,BrandMark}.vue` | 0 | (clean of off-palette utilities; carry **hex** — see hex table) |
| `components/player/KodikPlayer.vue` | 19 | 37 `bg-yellow-500/90`; 132 `bg-green-500/20`,`border-green-500/50`,`text-green-400`; 145/169 `bg-blue-500/20`,`border-blue-500/50`,`text-blue-400`; 169 `bg-green-500/20`,`border-green-500/50`; 171 `ring-amber-500/30`; 180/208 `bg-amber-500/20`,`bg-amber-500/30`,`text-amber-400`; 194 `bg-blue-500`,`bg-green-500` |
| `components/player/OtherSubsPanel.vue` | 2 | 13 `text-red-400`; 115 `text-amber-400/80` |
| `components/player/ResumePill.vue` | 2 | 17 `text-emerald-400/70`; 52 `text-amber-400/70` |
| `components/player/AnimeLibPlayer.vue` | 1 | 283 `text-red-400/70` |
| `components/player/OurEnglishPlayer.vue` | 1 | 52 `text-amber-400/80` |
| `components/player/SubtitleSettingsMenu.vue` | 1 | 24 `bg-zinc-900/95` |
| `components/player/{HanimePlayer,RawPlayer,SubtitleOverlay}.vue` | 0 | (clean of off-palette; Hanime/Subtitle carry **hex**) |

**Mapping each occurrence (locked):** `amber/yellow→warning`, `emerald/green→success`,
`blue/sky→info`, `purple/violet→brand-violet`, `red→destructive`, `gray/zinc→muted`
(judge `muted` bg vs `muted-foreground` text per role; `text-gray-400` → `text-muted-foreground`).
Opacity modifiers (`/20`, `/80`) carry over onto the `-soft` variant or token with the
same modifier — verify the soft variant exists before using it.

### Hardcoded hex — 36 literals across 8 files [VERIFIED: grep]

| File | Lines: hex | Classification |
|------|-----------|----------------|
| `views/NotFound.vue` | 29/49 `#ff6b6b`, 58 `#ff5252` (hand-rolled `.btn` red); 36 `#fff`; 42 `#999` | **Snap to token.** `.btn` → `destructive`/`primary` token + Button primitive; `#fff`→`foreground`, `#999`→`muted-foreground` |
| `components/anime/AnimeCard.vue` | 49 `#1a1a1a`, 68 `#111` (surfaces); 104/160 `#ff6b6b` (red); 126 `#ffd700` (star gold); 139 `#fff`; 155/177 `#999`/`#aaa` (muted); 174 `#333` (border) | **Snap to token,** EXCEPT `#ffd700` star-gold = likely `warning` or a small novel star token — judge. NOTE: confirm AnimeCard render path (still barrel-exported but AnimeCardNew is what Browse/Anime use). |
| `components/layout/BrandMark.vue` | 13 `#00d4ff`,`#ff2d7c`; 25 `#08080f`; 38 `#00d4ff` | **ALL are `var(--token, #fallback)` fallbacks** — these are DS-MIGRATE-04 alias usages, not raw hex. Repoint `var(--accent,#00d4ff)`→canonical, `var(--pink,#ff2d7c)`→`--brand-pink`. `var(--color-base,#08080f)` is fine. **Brand-glow value `--accent-glow` is literal (no canonical equivalent) — flag.** |
| `components/layout/Navbar.vue` | 13 hex (550–682): many `#00d4ff`/`#ffffff` are `var(--accent/--ink, #fallback)` fallbacks; 650 `#1a3a4a`→`#0e2030` (**novel teal gradient**); 681 `#00ff9d` is `var(--color-success,#00ff9d)` fallback | Repoint alias fallbacks (DS-MIGRATE-04). **`#1a3a4a→#0e2030` gradient is novel — add a token or keep documented.** |
| `components/player/AnimeLibPlayer.vue` | 869 `#f97316` (`--player-accent` orange) | **Per-player brand hue.** Genuinely novel orange — no token within tolerance. Promote to a player-accent token or document allowlist. |
| `components/player/HanimePlayer.vue` | 479 `#ec4899` (`--player-accent` pink) | Near `--brand-pink` (`#ff2d7c`) but NOT equal — judge: snap to brand-pink (small render shift) or keep as player token. |
| `components/player/KodikPlayer.vue` | 1077 `#06b6d4` (`--player-accent` cyan) | Near `--brand-cyan`/`--primary` (`#00b8e6`) but NOT equal — judge per case. |
| `components/player/SubtitleOverlay.vue` | 202 `#ffffff` (default JP-subtitle text color), 336 `#ffcccc` (subtitle render) | **Legitimately novel render values** (user-overridable subtitle color). DO NOT snap — add to documented novel-value allowlist or token if appropriate. |

**Novel-hex allowlist candidates (human-judgment-per-case, do NOT blind-replace):**
- `SubtitleOverlay.vue` `#ffffff`/`#ffcccc` — subtitle render defaults (CLAUDE.md flags these as legitimately novel).
- Per-player `--player-accent` (`#f97316` orange / `#ec4899` pink / `#06b6d4` cyan) — deliberate per-player identity hues; `#f97316` orange has no token; the pink/cyan are near-but-not-equal to brand tokens.
- `Navbar.vue` `#1a3a4a→#0e2030` teal gradient — novel, no token within tolerance.
- `BrandMark.vue` `--accent-glow` shadow value — literal in main.css (no canonical alias).
- `AnimeCard.vue` `#ffd700` star-gold — judge `warning` vs a small star token (only if AnimeCard is still on a render path).

### Deprecated alias usages (DS-MIGRATE-04, partial) [VERIFIED: grep + main.css]

Alias → canonical resolution (from `main.css:401-410`):
- `--ink` → `--foreground`
- `--ink-3` → `--muted-foreground`
- `--accent` → `--brand-cyan` (**do NOT flip — repoint usages to `--brand-cyan`/`--primary` per role**)
- `--pink` → `--brand-pink`
- `--pink-soft` → `--brand-pink-soft`
- **LITERAL, no canonical equivalent (handle with care, may need a new token):**
  `--ink-2` (`rgba(255,255,255,0.78)`), `--ink-4` (`rgba(255,255,255,0.36)`),
  `--accent-soft`, `--accent-line`, `--accent-glow`.

| File | Lines | Aliases used |
|------|-------|--------------|
| `views/Home.vue` | 313/352 `--ink-3`; 321/365 `--accent-line`; 364 `--accent-soft`; 367 `--accent` | repoint `--ink-3`→`--muted-foreground`, `--accent`→canonical; `--accent-line/-soft` are literal — judge |
| `components/layout/BrandMark.vue` | 13/38 `--accent`,`--pink`; 14 `--accent-glow` | repoint accent/pink; `--accent-glow` literal |
| `components/layout/Navbar.vue` | 550–669: `--accent`,`--ink`,`--ink-2`,`--ink-3`,`--accent-line` (many) | repoint ink/ink-3/accent; `--ink-2`/`--accent-line` literal — judge |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Primary/secondary CTA button | New `.btn` CSS (e.g. NotFound `.btn`) | `@/components/ui` `Button` (`default`/`brand`/`ghost`/`outline`/`destructive`) | Token-driven, focus ring, variant map locked Phase 2 |
| Card surface | `bg-[#1a1a1a]` div | `@/components/ui` `Card` (+ Header/Content/Footer) | `bg-card` token, consistent radius/elevation |
| Status badge/pill | `bg-amber-500/20 text-amber-400` span | `@/components/ui` `Badge` (+ `badgeVariants`) where a variant fits | Centralized status colors |
| New color value | Inline `#hex` | Token in `main.css`, reference via utility/var | Single source of truth; lint gate (Phase 5) will fail inline hex |

**Key insight:** The whole point of the milestone is to stop hand-rolling color/buttons.
But "zero rendered change" overrides "use the primitive" when a swap would shift pixels —
in that case keep the structure, apply token-only colors, and leave the primitive swap to
a later phase. The primitive swap is **partial** this phase (DS-MIGRATE-06 partial).

### Primitive-analog map (DS-MIGRATE-06, partial)
| Surface element | Has analog? | Action |
|-----------------|-------------|--------|
| `views/NotFound.vue` `.btn` "На главную" | YES (Button `default`/`brand`) | Swap to `Button` if pixel-identical; else token-only colors |
| Anime-detail "Смотреть" cyan `.btn-primary` | YES (Button `default`) but it's a layered `.btn-primary` | **Cascade-sensitive** — verify in-browser; may stay `.btn-primary` (already token-repointed Phase 1 DS-FOUND-05) |
| Status pills (schedule/ongoing/language) | PARTIAL (Badge variants) | Token-only color repoint is safer; Badge swap only if a variant matches exactly |
| Player chrome buttons (controls) | NO bespoke analog | Keep structure, token-only colors (`--player-accent` handling per novel-hex rule) |
| Spotlight frame / glass-card / shuffle-deck | NO | Bespoke + unlayered — leave structurally untouched (DS-NF-02) |

## Common Pitfalls

### Pitfall 1: Cascade footgun (DS-NF-02, load-bearing)
**What goes wrong:** Migrating utilities near `.cta-*`, `.spotlight-frame`, `.glass-card`,
`.shuffle-deck` silently shifts the cascade; the surface renders differently despite a
"value-preserving" edit.
**Why it happens:** `.cta-*` is intentionally INSIDE `@layer components` (so Tailwind
utilities can override its `display`); `.spotlight-frame`/`.glass-card`/`.shuffle-deck`
are intentionally UNLAYERED (so they BEAT utilities). [VERIFIED: `main.css:493` `@layer
components{ .cta-hero … }`, `main.css:544` unlayered `.spotlight-frame`]. A second plain
`@layer components` block gets tree-shaken (project memory: `reference_tailwind_v4_css_cascade.md`).
**How to avoid:** Never move these classes between layered/unlayered. When migrating
Home.vue (spotlight zone) or any surface using `.cta-*`/`.glass-card`, smoke in-browser.
**Warning signs:** A `md:hidden` or background utility stops working; spotlight card layout
shifts; jsdom tests pass but the live page looks wrong.

### Pitfall 2: Snapping a novel hex to the nearest token (silent render shift)
**What goes wrong:** `#ec4899` (Hanime player pink) "looks like" `--brand-pink` (`#ff2d7c`)
but is a different hue; snapping changes the player chrome color.
**Why it happens:** The mapping is for off-palette *Tailwind classes*, not for every hex.
Per-player accents and subtitle render values are deliberate.
**How to avoid:** For each hex, check if it equals a canonical token value. If not within
tolerance, add a token (or document in the novel-value allowlist) — never inline, never snap.
**Warning signs:** Player accent color or subtitle color changes after migration.

### Pitfall 3: Flipping `--accent` early
**What goes wrong:** Repointing `var(--accent)` usages AND flipping the `--accent`
definition to the shadcn hover-surface meaning in the same phase breaks brand-cyan rendering.
**Why it happens:** `--accent` currently aliases `--brand-cyan` (`main.css:405`); the flip
is DS-MIGRATE-05, Phase 5.
**How to avoid:** Only repoint *usages* to canonical names; leave the alias definition.
**Warning signs:** Brand-cyan elements turn into a hover-grey surface.

### Pitfall 4: i18n key paths drift unnoticed
**What goes wrong:** A migration edit touches a `t(...)` key path; jsdom/tsc pass but the
live page shows raw key strings.
**Why it happens:** i18n keys are string-typed, not type-checked (project memory:
`feedback_smoke_verify_i18n.md`).
**How to avoid:** This phase is color/primitive-only — DO NOT touch copy/keys. If a primitive
swap moves a label, smoke-verify in-browser.

### Pitfall 5: vue-tsc incremental false-pass
**What goes wrong:** `vue-tsc --noEmit` passes from a stale incremental cache.
**How to avoid:** Delete `*.tsbuildinfo` before the gate (project memory:
`feedback_vuetsc_noemit_false_pass.md`); import shared SFC types from the `@/components/ui`
barrel, not from a `.vue <script setup>`.

## Runtime State Inventory

> N/A — this is a frontend SFC/CSS refactor. No stored data, live-service config,
> OS-registered state, secrets/env vars, or build artifacts carry the migrated strings.
> Verified: scope is `frontend/web/src/**/*.vue` + `main.css` only; no DB keys, no env
> var names, no installed packages reference off-palette class names. The only build
> artifact is the Vite bundle, regenerated by `bun run build` (covered by the gate).

## Validation Architecture

> nyquist_validation: enabled (no `.planning/config.json` override found disabling it).

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest ^4.1.6 (jsdom, structural only) + Playwright (e2e/in-browser) + vue-tsc (type) |
| Config file | `frontend/web/vitest.config.ts`, `frontend/web/playwright.config.ts` |
| Quick run command | `cd frontend/web && bunx vitest run <path>` |
| Full suite command | `cd frontend/web && bunx vitest run && rm -f *.tsbuildinfo && bunx vue-tsc --noEmit` |

**Note:** There is NO `"test"` script in package.json — Phase 3 invoked `bunx vitest run`
directly. Use `bunx`, never `npx`/`npm` (project rule).

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | Exists? |
|--------|----------|-----------|-------------------|---------|
| DS-MIGRATE-01/02 | Zero off-palette classes in migrated files | grep gate (automated) | `grep -rEl '(red\|amber\|yellow\|emerald\|green\|blue\|sky\|purple\|violet\|gray\|slate\|zinc)-(50\|100\|...\|900)' <migrated files>` returns nothing | ✅ (grep) |
| DS-MIGRATE-03 | Zero hardcoded hex (outside documented allowlist) | grep gate (automated) | `grep -rnE '#[0-9a-fA-F]{3,8}\b' <migrated files>` reviewed against allowlist | ✅ (grep) |
| DS-MIGRATE-04 | No `var(--ink/--accent/--pink)` left in migrated files | grep gate | `grep -rnE 'var\(--(ink\|accent\|pink)' <migrated files>` (note literal `--accent-soft/-line/-glow`, `--ink-2/-4` may legitimately remain if no canonical token) | ✅ (grep) |
| DS-MIGRATE-06 | Hand-rolled button/card/badge swapped where analog exists | code review + structural Vitest mount | per-file review; new `.spec.ts` if a primitive is newly adopted | ⚠️ partial — see Wave 0 |
| DS-NF-06 | Zero rendered change | **in-browser smoke (manual, mandatory)** | 5-surface smoke at desktop+mobile via Chrome MCP — NOT automatable in jsdom | ❌ Wave 0 (smoke protocol) |
| DS-NF-05 | Each plan independently shippable | full suite + build green per plan | `bunx vitest run && bunx vue-tsc --noEmit && bun run build` | ✅ |

### Sampling Rate
- **Per task commit:** `bunx vitest run <touched .spec.ts>` + acceptance grep on touched files.
- **Per wave/plan merge:** full Vitest + `rm -f *.tsbuildinfo && bunx vue-tsc --noEmit` +
  `bun run build` + acceptance grep returns zero on all files in that plan + the smoke
  surface for that plan (Plan A→Home/Browse, Plan B→nav/404, Plan C→anime detail, Plan D→a player).
- **Phase gate:** full Vitest green + `vue-tsc --noEmit` green + e2e `spotlight`/`player`/
  `home`/`anime` specs pass + acceptance grep zero across ALL migrated files + the full
  5-surface in-browser smoke at desktop+mobile with zero rendered diff.

### Wave 0 Gaps
- [ ] **In-browser smoke protocol** — there is no automated visual-regression harness; the
  5-surface smoke (Home, Browse, anime detail, a player, 404) at desktop+mobile is manual
  via Chrome MCP. Plan must include it as an explicit verification task per plan (jsdom
  cannot catch cascade regressions — DS-NF-06).
- [ ] **Existing structural specs to keep green:** `AnimeContextMenu.spec.ts`,
  `OtherSubsPanel.spec.ts`, `OurEnglishPlayer.spec.ts`, `SubtitleOverlay.spec.ts`,
  `SubtitleSettingsMenu.spec.ts`, `WatchTogetherView.spec.ts`. Home/Browse/Anime/NotFound/
  Navbar/most players have NO `.spec.ts` — structural coverage is e2e-only for those.
- [ ] **No new spec required** unless a primitive is newly adopted (DS-MIGRATE-06); if
  NotFound's `.btn` becomes `Button`, add a minimal mount spec.
- [ ] **Acceptance grep** is the primary automated gate — wire it into each plan's verify
  step (it becomes the Phase 5 lint rule DS-GOV-01).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| bun / bunx | All frontend commands | ✓ (project standard) | — | none (do NOT use npm/npx) |
| Chrome MCP (`mcp__claude-in-chrome__*`) | In-browser 5-surface smoke (DS-NF-06) | required for verification | — | none — smoke is mandatory, jsdom insufficient |
| Vitest / vue-tsc / Playwright | Structural + type + e2e gates | ✓ (devDeps) | vitest ^4.1.6 | none |

**Missing dependencies with no fallback:** none for code; the in-browser smoke requires a
real browser session — there is no jsdom fallback (that's the whole point of DS-NF-06).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Off-palette Tailwind (`amber-500`) + inline hex | Semantic tokens + `ui/` primitives | Phases 1–3 (2026-06-02) | This phase applies it to high-traffic surfaces |
| Hand-rolled `Button`/`Card`/`Badge` | shadcn-vue (Reka) primitives in `@/components/ui` | Phases 2–3 | Swap targets exist; partial adoption this phase |
| `--ink/--accent/--pink` legacy tokens | Value-preserving aliases → canonical | Phase 1 (DS-FOUND-04) | Repoint usages now; `--accent` flip deferred to Phase 5 |

**Deprecated/outdated:**
- `--accent` as brand-cyan — temporary alias, flips in Phase 5 (do NOT pre-empt).
- `components/ui/ContextMenu.vue` — retired in Phase 3 (DS-LIB-08); `AnimeContextMenu.vue`
  is being replaced by a DropdownMenu kebab (`AnimeKebab`). Confirm migration state when
  touching `AnimeContextMenu.vue` (still has 1 off-palette `text-amber-400`).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `AnimeCard.vue` (old) may not be on an active render path (Browse/Anime use `AnimeCardNew`); only barrel-exported via `components/anime/index.ts` | Per-File Inventory | If still rendered, its 9 hex MUST be migrated; if dead, lower priority. Planner should confirm render path or migrate regardless (it's in-scope). |
| A2 | The 5-surface in-browser smoke is manual (no visual-regression harness exists) | Validation Architecture | If a Playwright screenshot-diff harness exists undiscovered, automation is cheaper. Searched e2e specs — none do pixel-diff. |
| A3 | `#ffd700` (AnimeCard star) maps to `warning` rather than needing a dedicated star token | Novel-hex candidates | If it's a deliberate gold distinct from `--warning` (`#ffd600`), snapping shifts the star color slightly (`#ffd700` vs `#ffd600` — 1 unit, likely fine). |
| A4 | Player `--player-accent` hues are deliberate per-player identity (keep as tokens, not snap) | Pitfall 2 | If they were meant to be brand colors, snapping is correct; flagged for per-case judgment. |

## Open Questions

1. **Is `AnimeCard.vue` (old) still rendered anywhere?**
   - What we know: imported only via `components/anime/index.ts` barrel; Browse/Anime use `AnimeCardNew`.
   - What's unclear: whether any view still imports `AnimeCard` (not `New`).
   - Recommendation: migrate it anyway (it's in-scope and carries 9 hex), but the planner
     can deprioritize if a grep confirms zero live consumers.

2. **Do `-soft` variants exist for every status color with the opacity modifiers used?**
   - What we know: `bg-success-soft`/`bg-destructive-soft` etc. are emitted (DS-FOUND-03).
   - What's unclear: whether `bg-warning/30` style explicit-opacity modifiers should become
     `bg-warning-soft` or keep the modifier on the base token.
   - Recommendation: per-occurrence judgment — prefer the documented `-soft` token when the
     intent is a soft background; keep the modifier only if no `-soft` matches the exact alpha.

3. **Does the anime-detail "Смотреть" `.btn-primary` get swapped to `Button` or stay?**
   - What we know: `.btn-primary` was token-repointed in Phase 1 (DS-FOUND-05) and is cascade-sensitive.
   - Recommendation: keep `.btn-primary` (already token-driven, zero-diff) unless a Button
     swap is verified pixel-identical in-browser; primitive adoption is partial this phase.

## Sources

### Primary (HIGH confidence — read/grepped this session)
- `frontend/web/src/styles/DESIGN-SYSTEM.md` — token tiers, usage rules, deprecated-alias map
- `frontend/web/src/styles/main.css:401-410,478-560` — alias definitions, `@layer components` cascade zone
- `frontend/web/src/components/ui/index.ts` — primitive barrel exports
- `frontend/web/vitest.config.ts`, `frontend/web/package.json` — test/type/build commands
- Grep inventory across all 23 in-scope `.vue` files (off-palette + hex + alias) — this session
- `04-CONTEXT.md`, `04-UI-SPEC.md`, `REQUIREMENTS.md`, `STATE.md` — phase contract + locked decisions

### Secondary (MEDIUM — project memory referenced in CLAUDE.md)
- `reference_tailwind_v4_css_cascade.md` — layered/unlayered cascade footgun
- `feedback_vuetsc_noemit_false_pass.md`, `feedback_smoke_verify_i18n.md` — gate caveats
- `feedback_native_rightclick_dropdown_triggers.md` — DropdownMenu kebab pattern (DS-LIB-08)

## Metadata

**Confidence breakdown:**
- Per-file inventory: HIGH — every count/line grep-verified this session.
- Color mapping: HIGH — locked by DS-MIGRATE-02, copied verbatim.
- Novel-hex flags: MEDIUM — classification is reasoned per-value; final "snap vs token"
  is per-case judgment (intentionally left to executor/human, that's the requirement).
- Validation: HIGH — commands verified against package.json/vitest.config; smoke is
  mandated by DS-NF-06.

**Research date:** 2026-06-02
**Valid until:** ~2026-06-16 (stable; re-grep if other sessions migrate these files first,
since parallel workstream sessions touch frontend — counts can drift).
