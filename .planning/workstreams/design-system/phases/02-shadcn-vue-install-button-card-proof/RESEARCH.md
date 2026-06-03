# Phase 2: shadcn-vue Install + Button/Card Proof - Research

**Researched:** 2026-06-02
**Domain:** Frontend component toolchain (shadcn-vue + Reka UI + cva) on Vue 3.4 / Vite 5.4 / Tailwind v4 (CSS-first)
**Confidence:** HIGH (stack versions, cva/Card source, token reconciliation all VERIFIED against npm registry + official shadcn-vue registry; bun-CLI quirk MEDIUM)

## Summary

shadcn-vue is a copy-in component pattern, not a runtime dependency: it installs four small libraries (`reka-ui`, `class-variance-authority`, `clsx`, `tailwind-merge`) and drops `cva`-based `.vue` files into `src/components/ui/`. The components reference exactly the CSS-variable token names Phase 1 already created (`--background`, `--primary`, `--ring`, `--radius`, `--secondary`, `--destructive`, `--border`, `--input`, plus the `@theme inline` `--color-*` mapping). Because Phase 1 already authored the canonical token block and `@theme inline` mapping in `frontend/web/src/styles/main.css`, **we must NOT run the shadcn-vue CLI's token-writing/init step against our CSS** — it would overwrite our hand-tuned Neon-Tokyo tokens with its default neutral oklch palette.

**Primary recommendation:** Do a **manual install** — `bun add` the four libs, hand-write `src/lib/utils.ts` (`cn()`), hand-write `components.json` (so any future `bunx shadcn-vue add X` points at the right aliases without re-initializing tokens), and hand-author `Button.vue` + the `Card.*` set by copying the canonical shadcn-vue `new-york` source and swapping in OUR variant set + glass look. Do NOT run `bunx shadcn-vue init` (it rewrites the CSS token block and trips a known bun package-manager-detection bug). This is the only path that satisfies DS-LIB-02 ("wired to our canonical token names") and the Phase-1 constraint ("do NOT regenerate or overwrite the token block") simultaneously.

This is a zero-new-token, zero-`tailwind.config.js`, additive change. The components bind to utilities (`bg-primary`, `bg-destructive`, `border-input`) that Phase 1's `@theme inline` block already generates, so they render against our existing palette with no CSS edits required.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Component variants (`cva`) | Browser / Client (build-time CSS) | — | Pure Tailwind utility composition; no runtime logic |
| Class conflict merging (`cn()`) | Browser / Client (runtime, tiny) | — | `clsx`+`tailwind-merge` run in component render |
| Token → utility mapping | Build (Tailwind v4 `@theme inline`) | — | Already owned by Phase 1 CSS; this phase only consumes |
| Accessible primitive behavior (`asChild`, focus) | Browser / Client (Reka UI) | — | Reka UI is headless DOM/ARIA layer |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `reka-ui` | `2.9.8` | Headless accessible primitives; provides `<Primitive>` (the `as`/`as-child` polymorphic element shadcn-vue Button wraps) | Official shadcn-vue dependency (Reka UI is the renamed Radix-Vue; shadcn-vue is built on it) `[VERIFIED: registry.npmjs.org/reka-ui/latest]` |
| `class-variance-authority` | `0.7.1` | `cva()` variant→class mapping for Button | The shadcn-vue Button is literally a `cva` definition `[VERIFIED: npm registry]` |
| `clsx` | `2.1.1` | Conditional class concatenation inside `cn()` | Half of the canonical `cn()` `[VERIFIED: npm registry]` |
| `tailwind-merge` | `3.6.0` | De-dupes conflicting Tailwind classes inside `cn()` | Other half of `cn()`; v3.x is the Tailwind-v4-compatible line `[VERIFIED: npm registry]` |

**License check (DS-NF-03):** reka-ui MIT, clsx MIT, tailwind-merge MIT, class-variance-authority **Apache-2.0**. All MIT/BSD/Apache — compatible with the project's MIT license. `[VERIFIED: npm registry latest manifests]`

**Peer-dep friction (Vue 3.4):** reka-ui 2.9.8 declares `peerDependencies.vue: ">= 3.4.0"`. Our `vue@^3.4.21` satisfies it — **no friction, no need to upgrade to 3.5+**. `[VERIFIED: registry.npmjs.org/reka-ui/latest peerDependencies]` The other three have no Vue peer dep.

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| (none) | — | — | DS-NF-03 forbids deps beyond the named four. Do NOT add `tailwind-variants`, `radix-vue` (old name), `@radix-icons/vue`, or an icon lib in this phase. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Manual install | `bunx shadcn-vue@latest init` | CLI rewrites the CSS token block (clobbers Phase 1 tokens — forbidden) and hits a known bun PM-detection bug. Reject. |
| `cva` | `tailwind-variants` | Phase requirements name `cva`; tva is an extra dep (violates DS-NF-03). Reject. |
| Reka `<Primitive>` Button | Keep custom `<component :is>` Button | We want the canonical shadcn structure for later phases; `<Primitive>` gives `as`/`as-child` for free and still forwards `@click`. Adopt Primitive but keep our prop surface. |

**Installation:**
```bash
cd frontend/web
bun add reka-ui class-variance-authority clsx tailwind-merge
```
Pins land as `reka-ui@^2.9.8 class-variance-authority@^0.7.1 clsx@^2.1.1 tailwind-merge@^3.6.0`. (All four are runtime `dependencies`, not `devDependencies` — `cn()` and the components execute at render time.)

**Version verification:** Versions above were confirmed live against `registry.npmjs.org/<pkg>/latest` on 2026-06-02. Re-confirm with `bun pm view <pkg> version` (or check the resolved `bun.lockb`) before pinning if the planner runs later than ~30 days out.

## Token-Name Reconciliation (DS-LIB-02) — the critical decision

shadcn-vue components reference these token names (all of which Phase 1 ALREADY defined in `:root` of `main.css`): `--background --foreground --card --card-foreground --popover --popover-foreground --primary --primary-foreground --secondary --secondary-foreground --muted --muted-foreground --accent --accent-foreground --destructive --border --input --ring --radius`. `[CITED: shadcn-vue.com/docs/theming]`

**Phase-1 status of each (VERIFIED by reading `frontend/web/src/styles/main.css`):**
- Present & canonical: `--background --foreground --card --card-foreground --popover --popover-foreground --primary --primary-foreground --secondary --secondary-foreground --muted --muted-foreground --border --input --ring` plus brand `--destructive(-foreground) --brand-pink(-foreground)`.
- `@theme inline` already maps `--color-background/foreground/card/primary/secondary/muted/border/input/ring/destructive/brand-pink/...` so utilities `bg-primary bg-destructive border-border border-input bg-card text-card-foreground bg-brand-pink` **already generate**. `[VERIFIED: grep of main.css lines 54-87]`
- **Two gaps vs shadcn defaults:**
  1. `--accent` exists but is currently a *brand-cyan alias* (Phase-1 comment: "shadcn `--accent` hover surface deferred to P2"). Shadcn's `ghost`/`outline` variants use `hover:bg-accent hover:text-accent-foreground`. There is **no `--accent-foreground` token** and `--color-accent` / `--color-accent-foreground` are **not** in the `@theme inline` block, so `bg-accent`/`text-accent-foreground` utilities do NOT generate today. **Action: do not use shadcn's stock `hover:bg-accent` classes for `ghost`/`outline`.** Re-express those hovers with our existing utilities (see variant table) to avoid depending on the deferred `--accent` flip. This keeps DS-MIGRATE-05's accent flip cleanly in Phase 5.
  2. `--radius` is **absent** from `:root`. Phase 1 added `--r-sm/md/lg/xl/2xl` instead. The stock shadcn Button uses `rounded-md` (a literal Tailwind radius, not `--radius`), and our existing Button uses `rounded-xl`. **Action: keep literal Tailwind radii (`rounded-xl` etc.) in the Button cva to match today's render; do NOT introduce `--radius` this phase** (avoids a new token, respects "no token block changes").

**Safest reconciliation path (RECOMMENDED):**
1. Hand-write `components.json` with `"tailwind": { "config": "", "css": "src/styles/main.css", "baseColor": "neutral", "cssVariables": true }`. This documents intent and lets a *future* `bunx shadcn-vue add <component>` emit correctly-aliased files — but we **never run `init`**, so the CLI never touches `main.css`.
2. **Do not run the token-writing init.** Hand-author Button + Card by copying canonical source and substituting our variant classes. This guarantees the Phase-1 token block is untouched (literally no edit to the `:root`/`@theme` region).
3. Components bind only to utilities that already generate today. No new `--color-*` entries, no new tokens.

`cssVariables: true` is the correct setting (we theme via CSS variables, not utility-class theming), but note it's a marker for the CLI's future `add` runs — it does not by itself rewrite anything when we skip `init`. `[CITED: shadcn-vue.com/docs/components-json — "For Tailwind CSS v4, leave [config] blank"]`

## Architecture Patterns

### System Architecture Diagram
```
consumer .vue  (Profile.vue, Browse.vue, Anime.vue, ...)
   │  imports { Button, Card } from '@/components/ui'  (barrel index.ts)
   ▼
components/ui/Button.vue ──uses──▶ buttonVariants (cva, in button/index.ts or co-located)
   │                                   │ produces utility class string per {variant,size}
   │  wraps                            ▼
   ▼                              cn(buttonVariants(...), props.class)  [clsx + tailwind-merge]
reka-ui <Primitive :as="button|a">  ──renders──▶ <button>/<a> with merged classes + fallthrough @click
                                                    │
                                                    ▼
                       utilities (bg-primary, bg-brand-pink, bg-destructive, border-input, ...)
                                                    │ resolved at build by
                                                    ▼
              Tailwind v4 @theme inline in main.css ──▶ var(--primary), var(--brand-pink), ... (Phase-1 tokens)
```

### Recommended Project Structure
Keep the existing flat `components/ui/` layout (DS-NF-04 requires existing import paths survive). Do **not** adopt shadcn's nested `ui/button/` folder for the Button if it would change the barrel path — keep `Button.vue` and `Card.vue` resolvable through `@/components/ui`.

```
frontend/web/src/
├── lib/
│   └── utils.ts            # NEW — cn() (clsx + tailwind-merge)
├── components/ui/
│   ├── index.ts            # EXISTING barrel — keep `Button`, `Card` exports; add Card subcomponents
│   ├── Button.vue          # REWRITTEN — Reka Primitive + cva, back-compat props
│   ├── button-variants.ts  # NEW — buttonVariants cva (or co-locate in Button.vue <script>)
│   ├── Card.vue            # REWRITTEN — shadcn root, keeps glass variants + padding/rounded props
│   ├── CardHeader.vue      # NEW (optional this phase)
│   ├── CardTitle.vue       # NEW (optional)
│   ├── CardContent.vue     # NEW (optional)
│   └── CardFooter.vue      # NEW (optional)
└── components.json         # NEW — at frontend/web/ root (sibling to package.json)
```

### Pattern 1: `cn()` utility (DS-LIB-01)
**What:** Canonical clsx + tailwind-merge helper.
**Where:** `frontend/web/src/lib/utils.ts`
**Code (exact — this is the CLI's own template, VERIFIED):**
```typescript
// Source: shadcn-vue CLI lib/utils.ts template (unovue/shadcn-vue)
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
```
**tailwind-merge + our custom tokens — do we need `extendTailwindMerge`?** **No, plain `twMerge` is correct.** Reasoning: all our custom token utilities are *color* utilities — `bg-brand-pink`, `bg-primary`, `bg-destructive`, `bg-success-soft`, `text-card-foreground`, `border-input`. tailwind-merge groups by utility *prefix* (`bg-*`, `text-*`, `border-*`), and treats any `bg-<token>` as the background-color group — so two background colors correctly conflict and the last wins, even for token names it has never seen. `[CITED: github.com/dcastil/tailwind-merge — last-conflicting-utility-wins; same-group bg colors conflict by default]` The documented mismerge risk is only when a custom utility *collides across property groups* (e.g. a custom `text-head-xl` font-size vs a `text-<color>`). We have **no such cross-group custom utilities** in the Button/Card surface, so `extendTailwindMerge` is unnecessary this phase. (If a later phase adds custom non-color utilities that share a prefix with a built-in group, revisit.) `[ASSUMED — based on inventory of our token utilities; verify no custom text-size/spacing utility shares a prefix during plan-check]`

### Pattern 2: Button via cva (DS-LIB-03)
**Canonical shadcn-vue Button structure (VERIFIED from the official registry):**
```vue
<!-- Source: shadcn-vue.com /r registry new-york Button.vue (canonical shape) -->
<script setup lang="ts">
import type { PrimitiveProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import type { ButtonVariants } from "."          // or "./button-variants"
import { Primitive } from "reka-ui"
import { cn } from "@/lib/utils"
import { buttonVariants } from "."
interface Props extends PrimitiveProps {
  variant?: ButtonVariants["variant"]
  size?: ButtonVariants["size"]
  class?: HTMLAttributes["class"]
}
const props = withDefaults(defineProps<Props>(), { as: "button" })
</script>
<template>
  <Primitive :as="as" :as-child="asChild" :class="cn(buttonVariants({ variant, size }), props.class)">
    <slot />
  </Primitive>
</template>
```
Canonical cva base (VERIFIED): `inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0`.

**OUR Button cva — variant→class table (uses OUR token utilities; mirrors today's render):**

`buttonVariants` base (adapted — keep cyan focus ring to match today, keep transition-all):
```
inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0
```
> Note: `focus-visible:ring-ring` resolves to `var(--ring)` = `--color-cyan-400` (Phase-1), i.e. identical to today's hard-coded `ring-cyan-400`. Radius is set **per variant** (not in base) because today's variants differ (`rounded-xl` vs `rounded-lg`).

| Variant | Classes (token-driven, matches current render) | Maps to today's |
|---------|--------------------------------------------------|-----------------|
| `default` | `bg-primary text-primary-foreground rounded-xl hover:bg-brand-cyan hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95` | `primary` (cyan) |
| `brand` | `bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95` | `secondary` (pink CTA) |
| `ghost` | `bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20` | `ghost` (unchanged) |
| `outline` | `bg-transparent hover:bg-white/5 text-brand-cyan rounded-xl border border-cyan-400/50 hover:border-cyan-400` | `outline` (unchanged) |
| `destructive` | `bg-destructive text-destructive-foreground rounded-xl hover:bg-destructive/90 active:scale-95` | NEW (no prior equiv) |

| Size | Classes | Maps to today's |
|------|---------|-----------------|
| `sm` | `px-3 py-1.5 text-sm` | `sm` (unchanged) |
| `md` | `px-6 py-3 text-base` | `md` (unchanged) — keep `md` as the name (not shadcn's `default`) |
| `lg` | `px-8 py-4 text-lg` | `lg` (unchanged) |
| `icon` | `h-10 w-10 p-0` | NEW |

`defaultVariants: { variant: "default", size: "md" }`.

> **`bg-primary` vs `bg-cyan-500` (pixel-identical check):** today's `primary` uses `bg-cyan-500` (= `#00b8e6`) and `--primary` = `var(--color-cyan-500)` = same `#00b8e6`, so `bg-primary` renders identically. Today's `text-base` (the literal Tailwind text-base = ink? — actually today's class is `text-base` which is the **font-size** utility for the cyan primary, the text color comes from default) — **planner must verify**: current `primary` line uses `text-base` which in Tailwind is `font-size: 1rem`, NOT a color. The cyan button's text color is inherited (white). `text-primary-foreground` = `var(--primary-foreground)` = `--color-base` = `#08080f` (near-black). **This is a behavior change** — switching to `text-primary-foreground` would make primary-button text dark, not inherited-white. **Recommendation: to render identically, keep the primary button text as-is (omit `text-primary-foreground`, or set the foreground utility only if the current button actually shows dark text).** Smoke-verify the cyan button's text color in-browser before locking the `default` variant's text class. `[VERIFIED: main.css token values; ASSUMED current rendered text color — must confirm in browser]`

**Preserving the existing API surface (DS-NF-04):** The 9 consumers (`Game.vue`, `Browse.vue`, `Anime.vue`, `Themes.vue`, `ActiveSessionsCard.vue`, `ProfileSetup.vue`, `Profile.vue`) use `variant="primary|secondary|ghost|outline"`, `size="sm"`, and `full-width`, plus `@click` (relies on attr fallthrough). They do **not** use `href`, `loading`, or the `#icon` slot (VERIFIED via grep — the `loading=`/`:loading` hits were unrelated `<img loading=lazy>` / `<Skeleton>`). Two viable approaches:

- **(RECOMMENDED) Extended/back-compat props on the shadcn Button.** Add to the cva variant set the legacy aliases `primary`→same as `default` and `secondary`→same as `brand`, keep `fullWidth?: boolean` (apply `w-full` via `cn`), keep `loading?` + `href?` + `#icon` slot for completeness. This means **zero consumer edits** and zero risk. Migrating `primary`→`default` / `secondary`→`brand` call-sites becomes a later cosmetic phase, not this one.
  - Because we keep `href`/`loading`/`fullWidth`/`#icon`, the cleanest implementation is to keep our own thin template (the `<Primitive :as="href ? 'a' : 'button'">` form) wrapping the cva, rather than the bare canonical Button. Reka `<Primitive>` forwards `@click` and arbitrary attrs to the rendered element, so fallthrough is preserved.
- (Alternative) Pure canonical Button + a codemod renaming `primary→default`, `secondary→brand` across the 9 files. More churn, breaks DS-NF-04's "consumers don't break" spirit. Not recommended for a "proof" phase.

### Pattern 3: Card reconciliation (DS-LIB-04 + DS-NF-04)
**Canonical shadcn-vue Card** is 6 subcomponents (VERIFIED from registry):
- `Card` base: `rounded-lg border bg-card text-card-foreground shadow-sm`
- `CardHeader`: `flex flex-col gap-y-1.5 p-6` · `CardTitle`: `text-2xl font-semibold leading-none tracking-tight` · `CardDescription`: `text-sm text-muted-foreground` · `CardContent`: `p-6 pt-0` · `CardFooter`: `flex items-center p-6 pt-0`
- Each accepts `class?: HTMLAttributes['class']` merged via `cn(base, props.class)`.

**Current `Card.vue`** is a single component with `variant: default|elevated|interactive` (using `glass-card`/`glass-elevated`/`card-hover`), `padding: none|sm|md|lg`, `rounded: md|lg|xl|2xl`, `as`, `href`. **Important finding (VERIFIED via grep): there are currently ZERO `<Card>` consumers in the codebase** — `Card.vue` is exported from the barrel but unused (the app uses bespoke cards like `AnimeCardNew`, `ThemeCard`, spotlight `*Card.vue`). DS-NF-04's "consumers don't break" is therefore trivially satisfied for Card, which de-risks the reconciliation.

**Recommended reconciliation (keep glass look + our prop API, adopt shadcn structure additively):**
1. **Keep `Card.vue`'s existing prop API verbatim** (`variant`/`padding`/`rounded`/`as`/`href`) so the barrel export stays back-compatible. Re-implement its internals with `cn()` instead of the `.join(' ')` computed, mapping `variant: default→glass-card`, `elevated→glass-elevated rounded-2xl`, `interactive→glass-card card-hover cursor-pointer` (unchanged classes → identical render). This makes Card "token-driven shadcn-vue" in mechanism (cva/`cn`, Reka `Primitive` for the `as`/`href` polymorphism) while preserving the Neon-Tokyo glass aesthetic.
2. **Add the shadcn subcomponents** (`CardHeader/CardTitle/CardContent/CardFooter`, optionally `CardDescription`) as NEW files, exported from the barrel, for future phases to compose. They use `bg-card`/`text-card-foreground`/`text-muted-foreground` utilities (all generate today). Since nothing consumes them yet, no render risk.
3. **Do NOT** replace the glass `Card` root's background with shadcn's flat `bg-card border shadow-sm` — that would visibly change the (currently-unused, but principled) glass look. Keep glass as the default variant's surface. This is the "keep our glass variants as the Card's look while adopting the shadcn structure" path the phase goal calls for.

> Decisive call: since Card has no consumers, the planner may legitimately scope Card to "rewrite root with `cn()` + add subcomponents, glass preserved" without a migration step. Button is where the real back-compat work lives.

### Anti-Patterns to Avoid
- **Running `bunx shadcn-vue init`** — rewrites `main.css` tokens (forbidden) + bun PM-detection bug. Hand-install instead.
- **Using `hover:bg-accent` / `text-accent-foreground`** in ghost/outline — those utilities don't generate (no `--color-accent` in `@theme inline`; `--accent` flip is deferred to Phase 5). Use explicit `bg-white/5` etc.
- **Putting new component styles in a 2nd `@layer components` block** — Phase-1 footgun: a plain rule in a second `@layer components` gets tree-shaken by Tailwind v4 (documented at main.css:541-543). Our Button/Card carry their styles as **utility classes via cva**, not as authored CSS, so they're immune — but do NOT "helpfully" extract them into a new `@layer components` block.
- **Introducing `--radius`** or rewriting the token block "to match shadcn defaults" — out of scope; respect Phase-1 tokens.
- **Adding `tailwind.config.js`** — unnecessary; Tailwind v4 + manual install needs none, and the canonical Button/Card use only utilities our `@theme inline` already emits.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Variant→class mapping | A `computed` returning `.join(' ')` (today's pattern) | `cva()` | Type-safe variant union, `defaultVariants`, composability; it IS the shadcn contract (DS-LIB-03) |
| Merging caller `class` with base | Manual string concat | `cn()` (clsx + tailwind-merge) | Resolves Tailwind conflicts so `<Button class="bg-red-500">` actually overrides `bg-primary` |
| Polymorphic `as`/`asChild` element | Custom `<component :is>` everywhere | reka-ui `<Primitive>` | Correct attr/event forwarding + `asChild` slot-merge, accessible |
| Accessible primitive behavior | Hand-rolled ARIA | reka-ui | Headless, tested, WAI-ARIA |

**Key insight:** This phase's entire point is to *replace* the hand-rolled `computed`-class pattern with the cva/`cn`/Primitive trio — proving the toolchain on Button+Card before the wider migration.

## Common Pitfalls

### Pitfall 1: shadcn-vue `init` clobbers Phase-1 tokens
**What goes wrong:** `init` writes its neutral-oklch `:root` + `@theme inline` into the CSS file named in `components.json`, overwriting our Neon-Tokyo tokens.
**Why:** The CLI assumes a greenfield CSS and owns the token block.
**Avoid:** Never run `init`. Hand-write `components.json` + `cn()` + components.
**Warning signs:** A diff touching the `:root` token block or `@theme inline` in `main.css`; `bg-primary` suddenly rendering grey.

### Pitfall 2: bun + shadcn-vue CLI package-manager mis-detection
**What goes wrong:** The shadcn CLI family has reported bugs where it defaults to npm / creates a duplicate lockfile / runs `bun install` unexpectedly even when invoked via `bunx`. `[CITED: github.com/shadcn-ui/ui issues #6593, #9874, #9867]`
**Avoid:** Don't use the CLI for installs. Use plain `bun add reka-ui class-variance-authority clsx tailwind-merge`. (If a future phase *must* use `bunx shadcn-vue add X`, prefer `bunx --bun shadcn-vue@latest add X` and verify no `package-lock.json` appeared.)
**Warning signs:** A new `package-lock.json` next to `bun.lockb`.

### Pitfall 3: `text-primary-foreground` darkens the cyan primary button
**What goes wrong:** Adopting shadcn's `bg-primary text-primary-foreground` makes primary-button text near-black (`--primary-foreground` = `#08080f`), where today's button text is inherited white. Visible regression.
**Avoid:** Match today's text color on the `default` variant; smoke-verify in browser before locking.
**Warning signs:** Cyan buttons showing dark text after deploy.

### Pitfall 4: tailwind-merge dropping a custom utility
**What goes wrong (theoretical):** If a custom utility shares a prefix with a different built-in group, twMerge may merge them wrongly.
**Avoid:** Our Button/Card use only color utilities (no cross-group prefix collisions) → plain `twMerge` is safe. If a later phase adds custom non-color utilities sharing a prefix, add `extendTailwindMerge`.
**Warning signs:** A utility you passed silently disappearing from the rendered `class`.

### Pitfall 5: Reka `<Primitive>` + `asChild` in `<script setup>`
**What goes wrong:** `asChild` expects a single root child; multiple roots or a fragment under `as-child` throws/misrenders.
**Avoid:** For Button we render a real `<button>`/`<a>` (`as`), not `asChild`, so this doesn't bite this phase. If consumers later wrap a `<RouterLink>` via `asChild`, ensure single-root slot. No Vue 3.4-specific SSR quirk (app is SPA/CSR; no SSR).
**Warning signs:** "Primitive expects a single child" runtime warning.

## Code Examples

### `components.json` (exact — hand-write at `frontend/web/components.json`)
```json
{
  "$schema": "https://shadcn-vue.com/schema.json",
  "style": "new-york",
  "typescript": true,
  "tailwind": {
    "config": "",
    "css": "src/styles/main.css",
    "baseColor": "neutral",
    "cssVariables": true
  },
  "aliases": {
    "components": "@/components",
    "composables": "@/composables",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "lib": "@/lib"
  },
  "framework": "vite",
  "iconLibrary": "lucide"
}
```
> `tailwind.config: ""` is the Tailwind-v4 signal (CLI skips writing a config). `[CITED: shadcn-vue.com/docs/components-json]` `framework`/`iconLibrary` are accepted by current CLI versions; `iconLibrary` is informational (we add no icon lib this phase). This file exists to make future `add` runs correct — we do not run `init`.

### `src/lib/utils.ts` (exact)
```typescript
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `radix-vue` | `reka-ui` (renamed) | 2024 | Use `reka-ui`, not `radix-vue`, as shadcn-vue's primitive layer |
| `tailwind.config.js` color tokens | Tailwind v4 `@theme` / `@theme inline` CSS-first | Tailwind v4 (2025) | shadcn-vue CLI leaves `tailwind.config` blank for v4 |
| tailwind-merge v2 (Tailwind v3) | tailwind-merge v3.x (Tailwind v4) | v3 release | Must use the v3 line for Tailwind v4 |
| `computed`+`.join(' ')` variants (our Button today) | `cva` + `cn` | this phase | The migration being proven |

**Deprecated/outdated:**
- `radix-vue` package name — superseded by `reka-ui`.
- Any guidance saying shadcn-vue *requires* `tailwind.config.js` — outdated; v4 path leaves it blank.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DS-LIB-01 | Install reka-ui + cva + clsx + tailwind-merge; add `cn()` at `src/lib/utils.ts` | `bun add` line + exact `cn()` code + versions/licenses verified |
| DS-LIB-02 | Configure shadcn-vue into `components/ui/`, wired to canonical tokens | Manual-install path + exact `components.json`; token reconciliation table; "never run init" |
| DS-LIB-03 | Button cva `default/brand/ghost/outline/destructive`; sizes `sm/md/lg/icon` | Full variant→class table using OUR token utilities; canonical Button.vue shape |
| DS-LIB-04 | Card → shadcn-vue, token-driven | Card reconciliation plan; subcomponent base classes; glass-preserved root |
| DS-NF-03 | No deps beyond named toolchain; licenses MIT/BSD/Apache | Only 4 deps; licenses verified (3 MIT + 1 Apache-2.0) |
| DS-NF-04 | Keep import paths + back-compatible prop surface | Barrel `index.ts` untouched paths; Button legacy-variant aliases; Card has 0 consumers (verified) |

## Verification Architecture (Validation)

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.6 (unit) + Playwright 1.58 (e2e/in-browser) |
| Config | `frontend/web` (vitest via package, playwright config present) |
| Quick run | `cd frontend/web && bunx vitest run src/components/ui/` |
| Type gate | `cd frontend/web && bunx vue-tsc --noEmit` |
| Full suite | `cd frontend/web && bunx vitest run && bunx vue-tsc --noEmit && bun run build` |

### Phase Requirements → Test Map
| Req | Behavior | Test Type | Command | Exists? |
|-----|----------|-----------|---------|---------|
| DS-LIB-01 | `cn()` merges + de-dupes | unit | `bunx vitest run src/lib/utils.spec.ts` | ❌ Wave 0 |
| DS-LIB-03 | Button emits expected class per variant/size; legacy aliases work; `@click`/`href`/`fullWidth` | unit (@vue/test-utils) | `bunx vitest run src/components/ui/Button.spec.ts` | ❌ Wave 0 |
| DS-LIB-04 | Card root keeps glass classes; subcomponents render base classes | unit | `bunx vitest run src/components/ui/Card.spec.ts` | ❌ Wave 0 |
| DS-NF-04 | All 9 Button consumers + barrel still typecheck | type | `bunx vue-tsc --noEmit` | ✅ existing |
| "renders identically" | 5-surface in-browser smoke | manual/e2e | Chrome MCP on home, browse, anime detail, a player, 404; focus Profile (most Button usage) | ✅ procedure |

**"Renders identically" proof:** Drive the standing 5-surface in-browser smoke (home, browse, anime detail, a player page, 404). The Button-dense surface is **Profile.vue** (primary/secondary/ghost/outline + full-width logout) — exercise it explicitly; also `Browse.vue` retry (`outline`), `Anime.vue` retry (`outline`), `ProfileSetup.vue` (`ghost` full-width). Compare against pre-change screenshots. Card has no consumers, so its "identical" claim is vacuous for live surfaces — assert via unit test that the glass classes are unchanged.

### Sampling Rate
- Per task commit: `bunx vitest run src/components/ui/ src/lib/ && bunx vue-tsc --noEmit`
- Per wave merge: full `bunx vitest run` + `bun run build`
- Phase gate: full suite green + 5-surface in-browser smoke before verify-work

### Wave 0 Gaps
- [ ] `src/lib/utils.spec.ts` — `cn()` merge/de-dup (DS-LIB-01)
- [ ] `src/components/ui/Button.spec.ts` — variant/size class assertions + legacy aliases + `@click`/`fullWidth`/`href` (DS-LIB-03, DS-NF-04)
- [ ] `src/components/ui/Card.spec.ts` — glass-class preservation + subcomponent bases (DS-LIB-04)
- No framework install needed (Vitest + @vue/test-utils + Playwright already present).

## Security Domain
Not applicable — pure client-side component refactor, no auth/session/input/crypto surface. `security_enforcement` produces no applicable ASVS category for this phase (V5 input-validation: components render trusted slot content only; no new user-input parsing). No threat-model deltas.

## Environment Availability
| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| bun | installs + build | ✓ (project standard) | per project | — |
| Vue | runtime | ✓ | 3.4.21 | — |
| Tailwind v4 | utility generation | ✓ | 4.1.18 | — |
| npm registry access (`bun add`) | fetch 4 new libs | assumed ✓ | — | offline mirror if blocked |
**Blocking missing:** none identified. The four new libs must be fetchable from the registry at install time.

## Runtime State Inventory
Not a rename/refactor-of-stored-state phase. Code-only:
- Stored data: None.
- Live service config: None.
- OS-registered state: None.
- Secrets/env: None.
- Build artifacts: `bun.lockb` updates (expected); no stale artifacts. Verify no `package-lock.json` appears (Pitfall 2).

## Assumptions Log
| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Plain `twMerge` (no `extendTailwindMerge`) is sufficient because all our token utilities are color-group utilities | `cn()` Pattern 1, Pitfall 4 | A custom utility could be silently dropped; mitigated by Button.spec class assertions |
| A2 | Current cyan `primary` button text renders white (inherited), so `text-primary-foreground` (dark) would be a regression | Pitfall 3, Button table | If today's text is already dark, no change needed — must smoke-verify in browser |
| A3 | `framework`/`iconLibrary` keys are accepted by the current shadcn-vue `components.json` schema | components.json example | Harmless if ignored; only matters for future `add` runs which we may not use |

## Open Questions
1. **Should legacy variant names (`primary`/`secondary`) stay as permanent aliases or be migrated this phase?**
   - Known: 9 consumers use `primary/secondary`; phase goal names the new set `default/brand/...`.
   - Unclear: whether the planner wants consumer call-sites migrated now.
   - Recommendation: keep aliases now (zero-churn, satisfies DS-NF-04); migrate call-sites in a later cosmetic phase.
2. **Does the cyan primary button currently show white or dark text?** (See A2.) Resolve by in-browser inspection of an existing `variant="primary"` button before locking the `default` variant's text class.

## Sources
### Primary (HIGH)
- shadcn-vue.com/docs/theming — token list, `@theme inline` mapping, oklch convention
- shadcn-vue.com/docs/components-json — `tailwind.config: ""` for v4, schema fields
- shadcn-vue.com /r registry (new-york Button, Card) — exact `buttonVariants` cva + Card subcomponent base classes
- shadcn-vue CLI `lib/utils.ts` template — exact `cn()`
- registry.npmjs.org/{reka-ui,class-variance-authority,clsx,tailwind-merge}/latest — versions, licenses, peer deps
- `frontend/web/src/styles/main.css` (Phase-1 tokens, `@theme inline`, glass classes, `@layer` footgun) — direct read
- `frontend/web/src/components/ui/{Button,Card,index}.ts/vue` + 9 consumers — direct read/grep
### Secondary (MEDIUM)
- github.com/dcastil/tailwind-merge (+ discussions #521, #278) — tailwind-merge v3 + Tailwind v4 custom-color behavior
- shadcn-ui/ui issues #6593, #9874, #9867 — bun CLI package-manager mis-detection
- shadcn-vue.com/docs/installation/vite, /docs/cli — install flow, `bunx --bun` form

## Metadata
**Confidence breakdown:**
- Standard stack / versions / licenses: HIGH — verified against npm registry live.
- Button cva + Card structure: HIGH — pulled from official shadcn-vue registry.
- Token reconciliation: HIGH — verified by reading Phase-1 `main.css`.
- tailwind-merge no-extend decision: MEDIUM — reasoned from grouping behavior; guarded by unit tests (A1).
- Primary-button text-color regression risk: MEDIUM — needs in-browser confirm (A2).
- bun-CLI quirk: MEDIUM — React-CLI issues; mitigated by avoiding the CLI entirely.

**Research date:** 2026-06-02
**Valid until:** ~2026-07-02 (fast-moving frontend toolchain — re-verify reka-ui/tailwind-merge versions if later).
