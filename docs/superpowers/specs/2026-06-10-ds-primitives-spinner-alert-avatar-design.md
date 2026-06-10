# Design Spec — DS Primitives: Spinner · Alert · Avatar

**Date:** 2026-06-10
**Status:** Locked (brainstorm approved via hosted-artifact loop, v1→v3)
**Artifact:** `.superpowers/brainstorm/dscomp-1780983901/content/design-v3.html`
**Scope:** Add five new `@/components/ui` primitives chosen by duplication evidence.

---

## 1. Motivation

Gap analysis of `@/components/ui` against hand-rolled patterns in the codebase surfaced three primitives with the strongest duplication signal:

| Primitive | Evidence (hand-rolled today) |
|---|---|
| **Spinner / LoadingState** | `animate-spin` in **27 files** (every player, FeedbackButton, Game, InviteButton…). No shared component — even `Button` rolls its own. |
| **Alert** | Inline tinted notice boxes in **7+ files**: `SystemStatusBanner.vue` (`role="alert"`), the `v-if="error"` block duplicated across 4 player components (`KodikAdFreePlayer.vue:225`, Kodik/AnimeLib/Hanime), form errors (ProfileSetup, AdminCollectionEdit, FeedbackButton), `NotTimeYetCard`. |
| **Avatar / AvatarGroup** | `rounded-full` + `<img>` + `charAt(0)` fallback in **5 files** (Navbar, Profile, ActivityFeed, MemberList, NowWatchingCard). |

Goal: collapse divergent implementations into token-bound primitives, satisfying the DS governance rule **DS-GOV-02 (reuse before building)**.

**Out of scope (YAGNI):** RadioGroup (use `ButtonGroup`), Table, Command, Breadcrumb, Sheet — no recurring hand-rolled demand. RadioGroup explicitly deferred.

---

## 2. Locked Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Ship **Spinner** atom **+ LoadingState** wrapper | ~10/27 sites are centered block-states (`mx-auto mb-3`); wrapper kills the most duplication. |
| D2 | Spinner shape = **F · dual counter-rotating arcs** (donut, real hole) | Brand signature; rejected A (border-ring), B–E, G–H. |
| D3 | Spinner color = **signature default + mono inline** | `tone` prop. `signature` = cyan outer ⟳ + pink inner ⟲ (block/page/empty loads). `mono` = both rings `currentColor` (inner @55% opacity) for inside buttons / tinted contexts. |
| D4 | **Alert** is kept; distinct from Toast | Toast = transient/floating/event/solid (`useToast.ts:34`, 3000ms auto-dismiss). Alert = persistent/inline/state/soft-tint. Maps onto the locked **inline-vs-overlay** principle. |
| D5 | Alert variants = `info \| success \| warning \| destructive` (default `info`) | First-class status tokens exist for all four. |
| D6 | Alert layout = `title?` prop + `default` slot + `#icon` override slot | Mirrors shadcn Alert/AlertTitle/AlertDescription. |
| D7 | Alert `dismissible?` prop (default `false`), emits `dismiss` | One prop covers static banners + closeable (form errors, report status). |
| D8 | Alert colors = **pure semantic tokens** (`bg-*-soft` / `text-*` / `border-*`) | Lint-clean with zero allowlist entries; opposite of Badge's literal-hue exemption (status colors have first-class tokens). |
| D9 | **Avatar** = img + initials fallback, sizes `xs–xl`, presence dot, AvatarGroup `+N` | WT MemberList shows live presence; member lists/now-watching need overlap stacks. |
| D10 | Adopt **`lucide-vue-next`** as the icon system **and migrate all existing inline SVGs** to it now | Tree-shaken per-icon (~0.5 KB each), self-hosted by construction (bundled, no runtime CDN), dedupes ~180 hand-inlined repeats (×, chevrons, play…). Bespoke glyphs are exempt (see §8 keep-list). The new components consume lucide; the repo-wide sweep ships in the same effort. |

---

## 3. Component Specs

### 3.1 Spinner — `Spinner.vue` + `spinner-variants.ts`

- **Props:**
  - `size?: 'xs' | 'sm' | 'md' | 'lg'` — default `md`. (14 / 18 / 24 / 36 px; `xl` 52 reserved for LoadingState.)
  - `tone?: 'signature' | 'mono'` — default `signature`.
  - `label?: string` — a11y text, default `'Loading'`.
  - `class?: HTMLAttributes['class']`.
- **Structure:** pure-CSS dual ring. Two pseudo-rings (`::before` outer, `::after` inset) with `border-color: transparent` on opposite edges, counter-rotating (`spin .8s` / `spin-rev 1.1s`). Border-width + inner inset scale per size. No SVG, no `reka-ui`.
  - `signature`: outer `border-{top,bottom}: --brand-cyan`, inner `border-{left,right}: --brand-pink`.
  - `mono`: both rings `currentColor`, inner `opacity: .55`.
- **A11y:** `role="status"` on root + `<span class="sr-only">{{ label }}</span>`.
- **Usage guidance:** inside `Button` / inline next to text → pass `tone="mono"`. Block/page/empty-state loads → default `signature` (usually via LoadingState).

### 3.2 LoadingState — `LoadingState.vue`

- **Props:** `label?: string`; `size?` (default `lg`); `tone?` (default `signature`); `class?`.
- **Layout:** `flex flex-col items-center justify-center gap-3 p-8`; spinner over muted (`text-muted-foreground`) label. Label optional.

### 3.3 Alert — `Alert.vue` + `alert-variants.ts`

- **Props:**
  - `variant?: 'info' | 'success' | 'warning' | 'destructive'` — default `info`.
  - `title?: string`.
  - `dismissible?: boolean` — default `false`.
  - `class?: HTMLAttributes['class']`.
- **Slots:** `default` (body) · `#icon` (overrides the per-variant default icon).
- **Emits:** `dismiss` — fired when the × button is clicked. (Parent owns visibility via `v-if`; the component does not self-hide.)
- **Default icons (lucide-vue-next):** info → `Info`, success → `CircleCheck`, warning → `TriangleAlert`, destructive → `CircleX`. Close → `X`. (Per D10 — no longer inline SVG.)
- **Token bindings (cva):**
  - base: `flex gap-3 p-4 rounded-xl border text-sm items-start`
  - `info`: `bg-info-soft border-info/30` · icon `text-info`
  - `success`: `bg-success-soft border-success/30` · icon `text-success`
  - `warning`: `bg-warning-soft border-warning/30` · icon `text-warning`
  - `destructive`: `bg-destructive-soft border-destructive/30` · icon `text-destructive`
  - title: `font-semibold text-foreground`; body: `text-muted-foreground` with `overflow-wrap-anywhere`.
  - dismiss button: `ml-auto` ghost, `text-muted-foreground hover:text-foreground`.

### 3.4 Avatar — `Avatar.vue` + `avatar-variants.ts`

- **Props:**
  - `src?: string`.
  - `name?: string` — drives initials (first letters of up to 2 words, uppercased; `'?'` when absent).
  - `size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'` — default `md` (24 / 32 / 40 / 48 / 64 px).
  - `status?: 'online' | 'idle' | 'offline'` — optional presence dot.
  - `class?: HTMLAttributes['class']`.
- **Behavior:** render `<img>` when `src` present and not errored; on `@error`, swap to initials disc. Initials disc: `bg-brand-cyan/16 text-[#5fdcff] font-semibold`. Wrapper is **not** `overflow-hidden` (dot sits outside the disc); the inner disc clips the image.
- **Presence dot:** absolute bottom-right, `ring-2 ring-background`; `online → bg-success`, `idle → bg-warning`, `offline → bg-muted-foreground/`(`--ink-4`). Dot size scales with avatar size.

### 3.5 AvatarGroup — `AvatarGroup.vue`

- **Props:** `max?: number` — default `4`; `size?` forwarded to children; `class?`.
- **Slot:** `default` containing `<Avatar>` children.
- **Behavior:** overlap via negative margin, each child `ring-2 ring-background`. Render first `max`; fold the remainder into a `+N` disc (`bg-elevated text-muted-foreground font-mono`).

---

## 4. File Layout

```
frontend/web/src/components/ui/
├─ Spinner.vue            (new)   spinner-variants.ts   Spinner.spec.ts
├─ LoadingState.vue       (new)                         LoadingState.spec.ts
├─ Alert.vue              (new)   alert-variants.ts     Alert.spec.ts
├─ Avatar.vue             (new)   avatar-variants.ts    Avatar.spec.ts
├─ AvatarGroup.vue        (new)                         AvatarGroup.spec.ts
└─ index.ts              (edit)   + 6 exports (Spinner, LoadingState, Alert, Avatar, AvatarGroup, + variant types)
```

Each `.vue` mirrors `Badge.vue`'s shape: `cn(variants({…}), props.class)`, `withDefaults(defineProps<Props>())`. No new runtime dependency (icons via existing `lucide-vue-next` if present, else inline SVG; spinner is pure CSS).

---

## 5. Testing Plan

Co-located `.spec.ts` per component, mirroring `Badge.spec.ts` (variant-class assertions + render assertions). Minimum coverage:

- **Spinner:** size→class map; `tone` signature vs mono class; `role="status"` + sr-only label present.
- **LoadingState:** renders Spinner; label slot/prop shown/omitted; centered layout classes.
- **Alert:** each variant binds its `bg-*-soft` + `text-*`; `title` renders when set; `#icon` slot overrides default; `dismissible` renders × and emits `dismiss`; body slot renders.
- **Avatar:** size→class; `src` renders `<img>`; `@error` → initials; `name` → correct initials (2-word, 1-word, empty `?`); `status` → correct dot class.
- **AvatarGroup:** caps at `max`, renders `+N` with correct overflow count.

Verify gates: `bunx vitest run src/components/ui/` · `bunx tsc --noEmit` · `bash frontend/web/scripts/design-system-lint.sh` (must stay ERRORS=0) · in-browser smoke at desktop + mobile (DS-NF-06 — jsdom can't catch Tailwind-v4 cascade bugs).

---

## 6. DS Compliance

- **Lint gate:** all five bind semantic tokens / brand-exempt hues only — no off-palette Tailwind classes, no raw hex → passes `design-system-lint.sh` with zero allowlist additions.
- **Governance:** `cva` variants in `{name}-variants.ts`; `font-medium`/`font-semibold` only; `rounded-*` per scale; `p-4` padding; reuse-before-build satisfied (no existing primitive covers these).
- **Coherence:** Alert canonicalizes the exact tokens (`bg-destructive/*`, `text-destructive`) already hand-rolled across the player error blocks; Spinner/Avatar replace literal duplication. Net coherence ↑.

---

## 8. Icon System — `lucide-vue-next` adoption + migration (D10)

**Why lucide:** per-icon ES modules, tree-shaken by Vite (only imported icons bundle, ~0.3–1 KB each). **Self-hosted by construction** — an npm dep bundled into our JS and served by our nginx; zero runtime CDN/fetch, no icon font, no sprite. Migrating dedupes ~180 hand-inlined repeats (the `×` close glyph alone recurs dozens of times) into shared imports.

**Inventory:** 236 inline `<svg>` across 65 `.vue` files (top: `Profile.vue` 28, `Anime.vue` 21, player components, `SpotlightIcon.vue` 11).

**Keep-list (bespoke — do NOT migrate, no lucide equivalent):**
- The new **Spinner** dual-arc rings (pure CSS).
- `components/home/spotlight/SpotlightIcon.vue` and the spotlight glyph set.
- The AnimeEnigma **score diamond ◆** and MAL **star ★** brand glyphs where used as identity marks.
- Any provider/brand logo SVG (Kodik, AniLib, Hanime, etc.).
- Decorative/illustrative one-off paths with no semantic icon meaning.

**Canonical mapping (high-frequency icons):**

| Intent | lucide | Intent | lucide |
|---|---|---|---|
| close / × | `X` | chevron down/up | `ChevronDown` / `ChevronUp` |
| chevron left/right | `ChevronLeft` / `ChevronRight` | play | `Play` |
| pause | `Pause` | kebab / more-vert | `EllipsisVertical` |
| more-horiz | `Ellipsis` | clock | `Clock` |
| bell | `Bell` | search | `Search` |
| check | `Check` | info | `Info` |
| warning triangle | `TriangleAlert` | error circle | `CircleX` |
| success circle | `CircleCheck` | settings/gear | `Settings` |
| user | `User` | external link | `ExternalLink` |
| trash | `Trash2` | edit/pencil | `Pencil` |
| plus | `Plus` | filter | `Filter` |

Icons not in this table: executor picks the closest lucide name from <https://lucide.dev/icons/> by visual match; if none exists, it stays inline (add to keep-list with a one-line reason).

**Per-file migration procedure (applied to each non-keep file):**
1. For each inline `<svg>`, identify intent → lucide name (table above or lucide.dev).
2. `import { Name } from 'lucide-vue-next'` in `<script setup>`.
3. Replace `<svg …>…</svg>` with `<Name :class="…" />`, carrying size/color classes (`size-4`, `text-muted-foreground`, etc.). lucide renders `stroke="currentColor"` so color follows the class.
4. Preserve a11y: keep any `aria-label`/`aria-hidden`; lucide sets `aria-hidden` by default — add a `<span class="sr-only">` if the SVG conveyed meaning.
5. `bunx vitest run` for that file's spec (if any) + `bunx tsc --noEmit`.
6. **In-browser smoke at desktop + mobile** (DS-NF-06) — verify size/alignment/color.
7. Commit the file (or small batch).

**Guardrail:** add an ESLint `no-restricted-imports` rule forbidding `import * as … from 'lucide-vue-next'` (barrel import would defeat tree-shaking).

**Batching:** migrate by area to keep commits reviewable — (a) `components/ui` + the new components, (b) players, (c) `views/*`, (d) `home/*` + spotlight (non-glyph), (e) `layout` + remainder. Each batch ends with a build + browser smoke + commit.

---

## 7. Scoring (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — consistent loading/feedback/identity surfaces + unified icon system; removes divergent ad-hoc treatments.
- **CDI = 0.06 * 34** — moderate spread (new primitives isolated under `components/ui/`, BUT the icon migration touches ~65 files), low-moderate shift (component APIs are additive/non-breaking; icon swaps are visual-only), Effort_Fib 34 (five components + lucide adoption + 65-file SVG sweep with per-file smokes).
- **MVQ = Griffin 86%/82%** — composed-from-known-parts (Badge conventions + mechanical icon swap), main slop risk is the long migration tail (mitigated by per-file browser smoke + keep-list).
