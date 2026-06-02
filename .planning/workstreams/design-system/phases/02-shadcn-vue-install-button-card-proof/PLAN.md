---
phase: 02-shadcn-vue-install-button-card-proof
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - frontend/web/package.json
  - frontend/web/bun.lockb
  - frontend/web/components.json
  - frontend/web/src/lib/utils.ts
  - frontend/web/src/lib/utils.spec.ts
  - frontend/web/src/components/ui/button-variants.ts
  - frontend/web/src/components/ui/Button.vue
  - frontend/web/src/components/ui/Button.spec.ts
  - frontend/web/src/components/ui/Card.vue
  - frontend/web/src/components/ui/CardHeader.vue
  - frontend/web/src/components/ui/CardTitle.vue
  - frontend/web/src/components/ui/CardContent.vue
  - frontend/web/src/components/ui/CardFooter.vue
  - frontend/web/src/components/ui/Card.spec.ts
  - frontend/web/src/components/ui/index.ts
autonomous: false
requirements: [DS-LIB-01, DS-LIB-02, DS-LIB-03, DS-LIB-04, DS-NF-03, DS-NF-04]

must_haves:
  truths:
    - "Existing cyan/pink/ghost/outline buttons across the app render pixel-identically to today (no color, radius, glow, or text-color change)."
    - "The 9 Button consumers compile and work with ZERO source edits (legacy variant names primary/secondary still accepted)."
    - "Button exposes default(cyan)/brand(pink)/ghost/outline/destructive variants and sm/md/lg/icon sizes via cva."
    - "Card root still renders the Neon-Tokyo glass look; Card subcomponents (Header/Title/Content/Footer) exist and render token-driven base classes."
    - "cn() merges class lists and de-dupes conflicting Tailwind utilities (last wins)."
    - "Only reka-ui + class-variance-authority + clsx + tailwind-merge were added; no package-lock.json appeared; bun.lockb updated."
  artifacts:
    - path: "frontend/web/src/lib/utils.ts"
      provides: "cn() helper (clsx + tailwind-merge)"
      exports: ["cn"]
    - path: "frontend/web/src/components/ui/button-variants.ts"
      provides: "buttonVariants cva + ButtonVariants type"
      exports: ["buttonVariants", "ButtonVariants"]
    - path: "frontend/web/src/components/ui/Button.vue"
      provides: "shadcn-vue Button (cva + cn + Reka Primitive) with back-compat props"
      contains: "buttonVariants"
    - path: "frontend/web/src/components/ui/Card.vue"
      provides: "glass Card root rebuilt with cn(), same prop API"
      contains: "glass-card"
    - path: "frontend/web/components.json"
      provides: "shadcn-vue config (so future `add` runs alias correctly); never used by init"
      contains: "src/styles/main.css"
  key_links:
    - from: "frontend/web/src/components/ui/Button.vue"
      to: "frontend/web/src/components/ui/button-variants.ts"
      via: "import { buttonVariants }"
      pattern: "buttonVariants"
    - from: "frontend/web/src/components/ui/Button.vue"
      to: "frontend/web/src/lib/utils.ts"
      via: "import { cn }"
      pattern: "from ['\"]@/lib/utils['\"]"
    - from: "frontend/web/src/components/ui/index.ts"
      to: "frontend/web/src/components/ui/Card.vue"
      via: "barrel re-export (unchanged path)"
      pattern: "export .* Card"
---

<objective>
Add the shadcn-vue toolchain (Reka UI + cva + clsx + tailwind-merge + a `cn()` helper) and convert `Button` and `Card` to token-driven shadcn-vue components, **rendering identically to today on every existing usage**.

End-state:
- `Button` is a `cva` + `cn()` + Reka `<Primitive>` component with variants `default`(cyan) / `brand`(pink) / `ghost` / `outline` / `destructive` and sizes `sm` / `md` / `lg` / `icon`. Legacy variant names `primary`→`default` and `secondary`→`brand` stay as aliases so the 9 existing consumers need ZERO edits. `href`, `loading`, `full-width`, and the `#icon` slot are preserved.
- `Card` keeps its exact prop API + glass look, rebuilt with `cn()`; new `CardHeader/CardTitle/CardContent/CardFooter` subcomponents are added (token-driven, no consumers yet).

Purpose: This is the smallest reversible bridgehead for the v1.0 Design System milestone — it proves the whole shadcn-vue/cva/`cn` toolchain on 2 primitives before migrating ~95 more.

Output: 4 new npm deps, `cn()`, `components.json`, rewritten `Button.vue` + `Card.vue`, 4 Card subcomponents, 3 co-located spec files, updated barrel.

Component-wide migration is OUT of scope (Phases 3–5). The `--accent` semantic flip is OUT of scope (Phase 5). DO NOT touch `main.css` at all.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/design-system/ROADMAP.md
@.planning/workstreams/design-system/REQUIREMENTS.md
@.planning/workstreams/design-system/phases/02-shadcn-vue-install-button-card-proof/RESEARCH.md
@frontend/web/src/styles/DESIGN-SYSTEM.md

<interfaces>
<!-- Contracts the executor needs. Extracted from the codebase 2026-06-02. No exploration required. -->

CURRENT Button.vue (frontend/web/src/components/ui/Button.vue) — the thing we are replacing.
Today's variant class strings (MUST be matched pixel-for-pixel by the new cva):
  base:    inline-flex items-center justify-center font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 disabled:opacity-50 disabled:cursor-not-allowed
  primary: bg-cyan-500 hover:bg-cyan-400 text-base rounded-xl hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95
  secondary: bg-pink-500 hover:bg-pink-400 text-white rounded-xl hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95
  ghost:   bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20
  outline: bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400
  sizes:   sm: px-3 py-1.5 text-sm | md: px-6 py-3 text-base | lg: px-8 py-4 text-lg
  fullWidth → appends w-full
  Template wraps `<component :is="href ? 'a' : 'button'">` with `:disabled="disabled || loading"`, `class="touch-target"`, a loading spinner `<span>`, an `#icon` slot `<span>`, then `<slot/>`.
  Props: variant primary|secondary|ghost|outline (default primary), size sm|md|lg (default md), type, href, disabled, loading, fullWidth.

  >>> A2 REGRESSION NOTE: today's `primary` line contains `text-base` which is the Tailwind FONT-SIZE utility (1rem), NOT a text color. The cyan button's text color is INHERITED (white in this app). The shadcn-default `text-primary-foreground` resolves to var(--primary-foreground) = #08080f (near-black) and would VISIBLY DARKEN cyan buttons. Task 3 verifies the live rendered text color before locking the `default` variant's text class. DO NOT add `text-primary-foreground` to `default` until Task 3 confirms it.

CURRENT Card.vue (frontend/web/src/components/ui/Card.vue) — 0 real consumers (only the barrel exports it; AnimeCardNew/ThemeCard/spotlight cards are bespoke, not <Card>).
  Props: as (default 'div'), href, variant default|elevated|interactive (default default), padding none|sm|md|lg (default md), rounded md|lg|xl|2xl (default 2xl).
  variants: default→glass-card | elevated→glass-elevated rounded-2xl | interactive→glass-card card-hover cursor-pointer
  paddings: none→'' | sm→p-3 | md→p-4 | lg→p-6
  roundings (applied ONLY when variant==='default'): md→rounded-md | lg→rounded-lg | xl→rounded-xl | 2xl→rounded-2xl
  base: 'block'

CURRENT barrel (frontend/web/src/components/ui/index.ts) — keep every existing export path:
  exports Badge, Button, ButtonGroup, Card, GenreFilterPopup, Input, Modal, PaginationBar, SearchAutocomplete, Select, Skeleton, Tabs, + SelectOption type.

TOKEN FACTS (verified in frontend/web/src/styles/main.css — DO NOT EDIT main.css):
  --primary = var(--color-cyan-500) = #00b8e6  → `bg-primary` === today's `bg-cyan-500`. IDENTICAL.
  --primary-foreground = var(--color-base) = #08080f (DARK — see A2 note).
  --brand-pink = var(--color-pink-500) = #ff2d7c → `bg-brand-pink` === today's `bg-pink-500`. IDENTICAL.
  --brand-pink-foreground = #ffffff → `text-brand-pink-foreground` === today's `text-white`. IDENTICAL.
  --ring = var(--color-cyan-400) = #00d4ff → `ring-ring` === today's `ring-cyan-400`. IDENTICAL.
  --brand-cyan = var(--color-cyan-400) = #00d4ff → `hover:bg-brand-cyan` === today's `hover:bg-cyan-400`. IDENTICAL.
  @theme inline GENERATES: bg-primary, bg-brand-pink, text-brand-pink-foreground, ring-ring, bg-destructive, text-destructive-foreground, bg-brand-cyan, bg-card, text-card-foreground, text-muted-foreground.
  @theme inline does NOT generate: bg-accent, text-accent-foreground (--color-accent absent). So ghost/outline hovers MUST use literal utilities (bg-white/5 etc.), never hover:bg-accent.
  There is NO --radius utility — use literal Tailwind radii (rounded-xl / rounded-lg).

CANONICAL shadcn-vue Button shape (reka-ui Primitive) — adapt, don't copy verbatim (we keep href/loading/icon):
  import { Primitive } from 'reka-ui'  // provides polymorphic `as` + `as-child`, forwards @click + attrs
  <Primitive :as="..." :class="cn(buttonVariants({ variant, size }), ...)"> ... </Primitive>
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Install the 4 deps, write cn(), TDD the cn() helper</name>
  <files>frontend/web/package.json, frontend/web/bun.lockb, frontend/web/src/lib/utils.ts, frontend/web/src/lib/utils.spec.ts</files>
  <behavior>
    cn() (frontend/web/src/lib/utils.ts):
    - Test 1: cn('a', 'b') === 'a b' (concatenates).
    - Test 2: cn('px-2', false && 'hidden', 'py-1') === 'px-2 py-1' (clsx drops falsy).
    - Test 3: cn('bg-red-500', 'bg-blue-500') === 'bg-blue-500' (tailwind-merge: last conflicting wins).
    - Test 4: cn('bg-primary', 'bg-brand-pink') === 'bg-brand-pink' (custom color tokens still conflict in the bg group — guards Assumption A1).
    - Test 5: cn(['p-2', 'p-4']) === 'p-4' (accepts array input, last wins).
  </behavior>
  <action>
Implements DS-LIB-01 + DS-NF-03 (deps + cn).

Step 1 — install deps via bun (NEVER the shadcn CLI; NEVER `shadcn-vue init`). From `frontend/web/`:
  `bun add reka-ui@2.9.8 class-variance-authority@0.7.1 clsx@2.1.1 tailwind-merge@3.6.0`
These are runtime dependencies (cn + components execute at render time), so they belong in `dependencies`, not `devDependencies` — `bun add` (no `-d`) places them there. Expected: package.json gains the 4 entries; bun.lockb updates; NO `package-lock.json` is created (if one appears, delete it and re-run — Pitfall 2).

Step 2 — write the cn() helper at frontend/web/src/lib/utils.ts (exact canonical template):
  import { type ClassValue, clsx } from 'clsx'
  import { twMerge } from 'tailwind-merge'

  export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs))
  }
Plain `twMerge` (NO `extendTailwindMerge`) is correct — all our token utilities are color-group utilities, so they conflict/merge correctly by prefix (verified by Test 4). Do not add extendTailwindMerge this phase.

Step 3 — write the failing tests first at frontend/web/src/lib/utils.spec.ts covering the 5 behaviors above (import { cn } from './utils'). Run, see RED before the helper exists / GREEN after.

Step 4 — verify NO stray lockfile and the 4 versions resolved as pinned.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && test ! -f package-lock.json && grep -q '"reka-ui"' package.json && grep -q '"class-variance-authority"' package.json && grep -q '"clsx"' package.json && grep -q '"tailwind-merge"' package.json && bunx vitest run src/lib/utils.spec.ts</automated>
  </verify>
  <done>4 deps in package.json `dependencies`; no package-lock.json; cn() exists; all 5 cn() tests pass. Commit: `feat(02-design-system): install shadcn-vue toolchain + cn() helper` (COMMIT only, do NOT push; 3 co-authors).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: components.json + buttonVariants cva + Button.vue rewrite (back-compat) + variant tests</name>
  <files>frontend/web/components.json, frontend/web/src/components/ui/button-variants.ts, frontend/web/src/components/ui/Button.vue, frontend/web/src/components/ui/Button.spec.ts, frontend/web/src/components/ui/index.ts</files>
  <behavior>
    buttonVariants (frontend/web/src/components/ui/button-variants.ts) + Button.vue:
    - Test 1: buttonVariants({ variant: 'default' }) contains 'bg-primary' and 'rounded-xl' and 'hover:bg-brand-cyan'.
    - Test 2: buttonVariants({ variant: 'brand' }) contains 'bg-brand-pink' and 'text-brand-pink-foreground'.
    - Test 3: buttonVariants({ variant: 'ghost' }) contains 'bg-white/5' and 'hover:bg-white/10' (NOT 'bg-accent').
    - Test 4: buttonVariants({ variant: 'outline' }) contains 'text-cyan-400' and 'border-cyan-400/50'.
    - Test 5: buttonVariants({ variant: 'destructive' }) contains 'bg-destructive' and 'text-destructive-foreground'.
    - Test 6: base class string contains 'ring-ring' (not 'ring-cyan-400') and 'focus-visible:ring-2'.
    - Test 7 (sizes): sm→'px-3 py-1.5 text-sm', md→'px-6 py-3 text-base', lg→'px-8 py-4 text-lg', icon→'h-10 w-10 p-0'.
    - Test 8 (LEGACY ALIAS, DS-NF-04): mount Button with variant="primary" → rendered class contains 'bg-primary' (identical to default). variant="secondary" → contains 'bg-brand-pink' (identical to brand).
    - Test 9 (mount): default render is a <button>; with href set it renders an <a> with that href.
    - Test 10 (mount): @click fires (attr/event fallthrough via Primitive); full-width adds 'w-full'; loading shows the spinner span + sets disabled; #icon slot renders when not loading.
  </behavior>
  <action>
Implements DS-LIB-02 (components.json) + DS-LIB-03 (Button cva) + DS-NF-04 (back-compat aliases).

Step 1 — components.json at frontend/web/components.json (sibling to package.json). This is documentation for FUTURE `bunx shadcn-vue add X` runs; we never run `init`, so it never rewrites main.css. Exact:
  {
    "$schema": "https://shadcn-vue.com/schema.json",
    "style": "new-york",
    "typescript": true,
    "tailwind": { "config": "", "css": "src/styles/main.css", "baseColor": "neutral", "cssVariables": true },
    "aliases": { "components": "@/components", "composables": "@/composables", "utils": "@/lib/utils", "ui": "@/components/ui", "lib": "@/lib" },
    "framework": "vite",
    "iconLibrary": "lucide"
  }
`tailwind.config: ""` is the Tailwind-v4 signal. DO NOT run any CLI against this file.

Step 2 — buttonVariants cva at frontend/web/src/components/ui/button-variants.ts:
  import { cva, type VariantProps } from 'class-variance-authority'
  export const buttonVariants = cva(
    'inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
    {
      variants: {
        variant: {
          default: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95',
          brand: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95',
          ghost: 'bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20',
          outline: 'bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400',
          destructive: 'bg-destructive text-destructive-foreground rounded-xl hover:bg-destructive/90 active:scale-95',
          // DS-NF-04 legacy aliases — duplicate the default/brand class strings so old call-sites are unchanged:
          primary: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-[0_0_30px_rgba(0,212,255,0.3)] active:scale-95',
          secondary: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-[0_0_30px_rgba(255,45,124,0.3)] active:scale-95',
        },
        size: {
          sm: 'px-3 py-1.5 text-sm',
          md: 'px-6 py-3 text-base',
          lg: 'px-8 py-4 text-lg',
          icon: 'h-10 w-10 p-0',
        },
      },
      defaultVariants: { variant: 'default', size: 'md' },
    },
  )
  export type ButtonVariants = VariantProps<typeof buttonVariants>

  NOTES:
  - `default`/`primary` deliberately OMIT any text-* color utility right now — today's cyan button inherits white text via `text-base` (font-size). Task 3 confirms the live color and, only if needed, edits this one line. DO NOT add text-primary-foreground here.
  - `brand`/`secondary` text === today's `text-white` via `text-brand-pink-foreground` (#ffffff). Keep `hover:bg-pink-400` (literal) to match today exactly — `--color-pink-400` has no semantic alias.
  - base uses `ring-ring` (resolves to --ring = cyan-400, identical to old hard-coded ring-cyan-400). Added shadcn niceties `gap-2 whitespace-nowrap [&_svg]:*` and `disabled:pointer-events-none`; these are additive and do not change the existing render (no svg children in current consumers, gap only matters with multiple flex children).

Step 3 — rewrite Button.vue (frontend/web/src/components/ui/Button.vue). Keep the full back-compat surface. Use reka-ui Primitive for `as` polymorphism so @click/attrs fall through:
  - <script setup lang="ts">: import { computed } from 'vue'; import { Primitive } from 'reka-ui'; import { cn } from '@/lib/utils'; import { buttonVariants, type ButtonVariants } from './button-variants'.
  - Props interface: variant?: NonNullable<ButtonVariants['variant']> (so it accepts default|brand|ghost|outline|destructive|primary|secondary), size?: NonNullable<ButtonVariants['size']>, type?: 'button'|'submit'|'reset', href?: string, disabled?: boolean, loading?: boolean, fullWidth?: boolean, class?: HTMLAttributes['class'].
  - withDefaults: variant 'default', size 'md', type 'button', disabled/loading/fullWidth false.
  - Template: <Primitive :as="href ? 'a' : 'button'" :href="href" :type="href ? undefined : type" :disabled="disabled || loading" :class="cn(buttonVariants({ variant, size }), fullWidth && 'w-full', 'touch-target', props.class)"> ... </Primitive>
  - Inside: keep the loading spinner span (v-if="loading", animate-spin mr-2 + the svg), the #icon slot span (v-if="$slots.icon && !loading", mr-2), then <slot/>. Copy these verbatim from the current Button.vue so the loading/icon markup is byte-identical.
  - IMPORTANT: pass `touch-target` THROUGH cn() (not as a static class attr) so tailwind-merge doesn't drop it and consumer `class` still wins on conflicts.

Step 4 — barrel: index.ts already exports Button from './Button.vue'; leave that line unchanged. (Card subcomponent exports are added in Task 4.) Optionally also `export { buttonVariants, type ButtonVariants } from './button-variants'` for downstream phases — add it; it's additive.

Step 5 — write Button.spec.ts (frontend/web/src/components/ui/Button.spec.ts) covering all 10 behaviors. Use @vue/test-utils `mount`. For class assertions on buttonVariants, import buttonVariants directly. For mount tests, assert `wrapper.classes()` / `wrapper.find('a').exists()` / `wrapper.emitted()` / spinner span presence. RED first (Button not yet rewritten), GREEN after.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx vitest run src/components/ui/Button.spec.ts src/lib/utils.spec.ts && bunx vue-tsc --noEmit</automated>
  </verify>
  <done>components.json exists; buttonVariants has all 5 variants + 2 legacy aliases + 4 sizes; Button.vue uses cn()+Primitive+buttonVariants and preserves href/loading/full-width/#icon; all Button.spec tests pass; vue-tsc clean (proving the 9 consumers still typecheck — DS-NF-04). Commit: `feat(02-design-system): Button → cva + cn (shadcn-vue), back-compat aliases + components.json` (COMMIT only; 3 co-authors).</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 3: A2 in-browser verify — lock the `default`/`primary` button TEXT COLOR (regression gate)</name>
  <what-built>
Button has been rewritten to cva/cn. The `default`/`primary` (cyan) variant currently has NO explicit text color — matching today's behavior where `text-base` is a font-size and text color is inherited (expected: white). This task confirms the LIVE rendered text color of a real cyan button so we don't silently regress it to dark (`text-primary-foreground` = #08080f).

Automation done before this checkpoint: `make redeploy-web` (Claude runs it), then Claude inspects the rendered color in-browser via Chrome MCP / DevTools and reports the computed `color` of an existing `variant="primary"` button (e.g. Profile.vue API-key "Regenerate" button, or any cyan Button). Claude must:
  1. `cd /data/animeenigma && make redeploy-web` (after Task 2 commit).
  2. Log in as `ui_audit_bot` (per CLAUDE.md UI-audit auth flow) and open a surface with a cyan `variant="primary"` Button (Profile settings → API Key card, or anime detail "Смотреть"/cyan CTA).
  3. Read the computed `color` style of that button element in-browser and compare against the pre-change reference (today's = inherited white, ~rgb(255,255,255)).
  4. State the finding explicitly.
  </what-built>
  <how-to-verify>
1. Confirm Claude's in-browser finding: is the cyan button's rendered TEXT color the SAME as before the change (white)?
   - If SAME (white) → the `default`/`primary` variant is correct AS-IS (no text-* utility). No edit. Proceed.
   - If the new build shows DARK text on cyan buttons (regression) → Claude must FIX by NOT adding `text-primary-foreground`; instead add the inherited/white text utility that matches today (e.g. `text-white`) to BOTH `default` and `primary` lines in button-variants.ts, redeploy, re-verify, then commit the fix.
2. Visually compare a cyan button side-by-side with memory/screenshot of the old one: same fill (#00b8e6 cyan), same `rounded-xl`, same hover glow, same text color.
3. Confirm pink (`secondary`/`brand`) buttons (Profile logout / revoke) still render pink with white text.
  </how-to-verify>
  <resume-signal>Type "approved" if the cyan + pink buttons render identically to today, or describe the color/radius diff you see so Claude can fix button-variants.ts.</resume-signal>
</task>

<task type="auto" tdd="true">
  <name>Task 4: Card.vue rewrite (glass preserved) + Card subcomponents + barrel + tests</name>
  <files>frontend/web/src/components/ui/Card.vue, frontend/web/src/components/ui/CardHeader.vue, frontend/web/src/components/ui/CardTitle.vue, frontend/web/src/components/ui/CardContent.vue, frontend/web/src/components/ui/CardFooter.vue, frontend/web/src/components/ui/Card.spec.ts, frontend/web/src/components/ui/index.ts</files>
  <behavior>
    Card.vue (root, glass preserved, same prop API):
    - Test 1: default mount → classes include 'glass-card' and 'rounded-2xl' (default rounded) and 'p-4' (default md padding) and 'block'.
    - Test 2: variant="elevated" → includes 'glass-elevated' and 'rounded-2xl', does NOT apply the rounded prop map.
    - Test 3: variant="interactive" → includes 'glass-card' 'card-hover' 'cursor-pointer'.
    - Test 4: padding="lg" → 'p-6'; padding="none" → no p-* utility.
    - Test 5: rounded="xl" with default variant → 'rounded-xl' (not rounded-2xl).
    - Test 6: href set → renders an <a> with that href; otherwise <div> (or `as` value).
    CardHeader/CardTitle/CardContent/CardFooter:
    - Test 7: CardHeader root classes include 'flex flex-col gap-y-1.5 p-6'.
    - Test 8: CardTitle classes include 'font-semibold' (use font-semibold, NOT text-2xl by default — honor DESIGN-SYSTEM weight rule; tracking-tight allowed).
    - Test 9: CardContent classes include 'p-6 pt-0'.
    - Test 10: CardFooter classes include 'flex items-center p-6 pt-0'.
    - Test 11: each subcomponent merges a caller `class` prop via cn() (e.g. CardContent class="mt-2" → 'mt-2' present, p-6 still present).
  </behavior>
  <action>
Implements DS-LIB-04 (Card → shadcn-vue, token-driven) + keeps DS-NF-04 (0 Card consumers, but barrel path + prop API preserved).

Step 1 — rewrite Card.vue (frontend/web/src/components/ui/Card.vue). Keep the EXACT prop API (as/href/variant/padding/rounded with the same defaults: as 'div', variant 'default', padding 'md', rounded '2xl'). Replace the `.join(' ')` computed with `cn()` and render via reka-ui Primitive for the as/href polymorphism:
  - script: import { computed } from 'vue'; import { Primitive } from 'reka-ui'; import { cn } from '@/lib/utils'; add `class?: HTMLAttributes['class']` prop.
  - variant map (UNCHANGED classes → identical render): default→'glass-card', elevated→'glass-elevated rounded-2xl', interactive→'glass-card card-hover cursor-pointer'.
  - padding map: none→'', sm→'p-3', md→'p-4', lg→'p-6'.
  - rounding map (applied ONLY when variant==='default', same as today): md→'rounded-md', lg→'rounded-lg', xl→'rounded-xl', '2xl'→'rounded-2xl'.
  - class = cn('block', variants[variant], paddings[padding], variant === 'default' ? roundings[rounded] : '', props.class).
  - Template: <Primitive :as="href ? 'a' : as" :href="href" :class="class"> <slot/> </Primitive>.
  DO NOT replace the glass root with shadcn's flat `bg-card border shadow-sm` — that would change the look. Glass stays (phase goal: keep glass variants as the Card's look while adopting shadcn structure).

Step 2 — add 4 subcomponents (token-driven, use utilities that already generate). Each is a thin <script setup> with a `class?: HTMLAttributes['class']` prop, rendering a plain element with `:class="cn(BASE, props.class)"` + <slot/>:
  - CardHeader.vue: <div>, BASE 'flex flex-col gap-y-1.5 p-6'.
  - CardTitle.vue: <h3>, BASE 'font-semibold leading-none tracking-tight text-card-foreground' (use font-semibold per DESIGN-SYSTEM weight rule; do NOT use text-2xl as a forced size — leave size to consumers; text-card-foreground generates today).
  - CardContent.vue: <div>, BASE 'p-6 pt-0'.
  - CardFooter.vue: <div>, BASE 'flex items-center p-6 pt-0'.
  No consumers wire these yet — they exist for Phase 3+ composition. No render risk.

Step 3 — barrel index.ts: keep the existing `export { default as Card } from './Card.vue'` line. ADD:
  export { default as CardHeader } from './CardHeader.vue'
  export { default as CardTitle } from './CardTitle.vue'
  export { default as CardContent } from './CardContent.vue'
  export { default as CardFooter } from './CardFooter.vue'
  (Additive — does not change any existing import path.)

Step 4 — write Card.spec.ts covering all 11 behaviors with @vue/test-utils mount. RED first, GREEN after.
  </action>
  <verify>
    <automated>cd /data/animeenigma/frontend/web && bunx vitest run src/components/ui/Card.spec.ts && bunx vue-tsc --noEmit</automated>
  </verify>
  <done>Card root rebuilt with cn(), glass + prop API preserved; 4 subcomponents exist + exported from barrel; all 11 Card.spec tests pass; vue-tsc clean. Commit: `feat(02-design-system): Card → cn() + glass preserved, add Card subcomponents` (COMMIT only; 3 co-authors).</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 5: Full verification gate — tsc, full vitest, build, redeploy, standing 5-surface in-browser smoke</name>
  <what-built>
All of Phase 2 is implemented and committed. This is the zero-rendered-change acceptance gate (jsdom can't catch cascade bugs — DS-NF-06).

Automation Claude runs BEFORE presenting this checkpoint:
  1. `cd /data/animeenigma/frontend/web && bunx vue-tsc --noEmit` → clean.
  2. `cd /data/animeenigma/frontend/web && bunx vitest run` → full suite green (the Phase-1 686 + the new utils/Button/Card specs).
  3. `cd /data/animeenigma/frontend/web && bun run build` → clean production build (catches vite/tsc build-time issues + confirms no stray lockfile broke install).
  4. `cd /data/animeenigma && make redeploy-web` (note: `redeploy-web` is gated by the flaky `scripts/i18n-lint.sh` — if it reports phantom missing keys, retry the deploy; trust the vitest locale-parity tests).
  5. Drive the STANDING 5-SURFACE in-browser smoke at desktop + mobile widths (Chrome MCP, logged in as `ui_audit_bot`), confirming NO rendered diff vs today:
     a. Home — spotlight carousel + Онгоинги/Топ rails.
     b. Browse/catalog — the `outline` retry Button (Browse.vue:80), filter sidebar, cards, pagination.
     c. Anime detail — cyan CTA + the `outline` retry Button (Anime.vue:945), badges, language pills.
     d. A player surface — one of the 5 players loads + styled controls.
     e. 404 — muted styling + "На главную" button.
     PLUS the Button-dense surface **Profile.vue** (primary/secondary API-key buttons + secondary full-width logout + ghost buttons) and **ProfileSetup.vue** (ghost full-width) — exercise these explicitly since they use the most Button variants.
  Claude reports pass/fail per surface with the specific buttons checked.
  </what-built>
  <how-to-verify>
1. Confirm Claude reports: vue-tsc clean, full vitest green, `bun run build` clean, `make redeploy-web` succeeded.
2. Spot-check the 5 standing surfaces + Profile + ProfileSetup: do all buttons/cards look EXACTLY as before (color, radius, glow, hover, text color, full-width)? Any diff = regression, Claude must fix before this gate passes.
3. Confirm no `package-lock.json` exists and only the 4 named deps were added (DS-NF-03 attestation).
  </how-to-verify>
  <resume-signal>Type "approved" if all gates pass and the 5 surfaces + Profile/ProfileSetup render identically; otherwise describe the failing gate or the visual diff.</resume-signal>
</task>

</tasks>

<threat_model>
Per RESEARCH.md "Security Domain": this is a pure client-side component refactor with no auth/session/input/crypto surface. Components render trusted slot content only; no new user-input parsing, no network, no storage. `security_enforcement` yields no applicable ASVS L1 category for this phase. No trust boundaries are crossed and no threat-model deltas are introduced.

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-02-01 | Tampering | new npm deps (reka-ui/cva/clsx/tailwind-merge) | accept | Versions pinned + licenses verified MIT/Apache (DS-NF-03); bun.lockb committed; no `package-lock.json` (Pitfall 2 guard in Task 1 verify). Low-value, well-known libs. |
| T-02-02 | Information disclosure | `cn()` passes caller `class` through | accept | `cn()` only string-merges Tailwind classes for trusted internal callers; no user input reaches it; tailwind-merge is render-time string ops, no eval. |
</threat_model>

<verification>
- Per-task: `bunx vitest run` on the touched spec(s) + `bunx vue-tsc --noEmit` (Tasks 2 & 4).
- Phase gate (Task 5): `bunx vue-tsc --noEmit` clean, full `bunx vitest run` green, `bun run build` clean, `make redeploy-web` succeeds, standing 5-surface in-browser smoke (+ Profile/ProfileSetup) shows zero rendered diff.
- DS-NF-03 attestation: `test ! -f package-lock.json` and exactly 4 deps added.
- Requirement → test map: DS-LIB-01 → utils.spec.ts; DS-LIB-02 → components.json exists + Button binds tokens; DS-LIB-03 → Button.spec variant/size assertions; DS-LIB-04 → Card.spec; DS-NF-03 → lockfile/dep check; DS-NF-04 → legacy-alias mount tests + vue-tsc clean (9 consumers compile, 0 edits).
</verification>

<success_criteria>
1. `bun add` of exactly 4 deps; `bun run build` clean; no new tsc/lint errors; no `package-lock.json`.
2. Every existing `<Button>`/`<Card>` usage renders identically — standing 5-surface in-browser smoke + Profile/ProfileSetup pass (Task 3 + Task 5).
3. Button exposes `default`(cyan)/`brand`(pink)/`ghost`/`outline`/`destructive` + `sm/md/lg/icon`, all token-driven; legacy `primary`/`secondary` still accepted (DS-NF-04, zero consumer edits).
4. Vitest covers variant→class mapping for Button and Card (+ cn() merge behavior).
5. DS-NF-03 attested: no deps beyond reka-ui + cva + clsx + tailwind-merge; licenses MIT/Apache-compatible.
6. `main.css` UNCHANGED (no token block touched); `--accent` flip NOT done (Phase 5).
</success_criteria>

<output>
After completion, create `.planning/workstreams/design-system/phases/02-shadcn-vue-install-button-card-proof/02-01-SUMMARY.md`.

Effort metrics (per `.planning/CONVENTIONS.md` — no days/hours):
**UXΔ = 0 (Ambiguous)** — zero rendered change is the explicit acceptance bar; user-facing surface is intentionally identical. | **CDI = 0.02 * 8** — tiny spread (2 components + cn + config), low shift (additive, back-compat aliases), Fibonacci effort 8. | **MVQ = Sprite 88%/82%** — small, sharp, self-contained toolchain bridgehead.
</output>
