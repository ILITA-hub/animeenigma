# Design System v2.0 — Phase 1: Button Reuse — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Widen the `Button` primitive's API so real-world button shapes fit, collapse the duplicated `.btn-*` CSS into the cva as the single source of truth, then swap clean-fit raw `<button>` elements to `<Button>` — with zero intended visual regression.

**Architecture:** API-first, then mechanical swap. Tasks 1–3 widen and de-duplicate the primitive (TDD on `buttonVariants()` class output). Task 4 reconciles `.btn-*`. Tasks 5–8 swap clean-fit buttons one surface-cluster at a time (each independently committed and visually smoke-tested). Task 9 writes governance. Task 10 is the milestone acceptance gate.

**Tech Stack:** Vue 3 SFC, Tailwind CSS v4 (`@theme` tokens), `class-variance-authority` (cva), `tailwind-merge` via `cn()`, Reka UI `Primitive`, Vitest + `@vue/test-utils`, Chrome MCP for in-browser smoke.

**Spec:** `docs/superpowers/specs/2026-06-03-design-system-v2-reuse-portability-design.md` (§3 = Phase 1).

**Resolved open questions (spec §6):** Q1 → `link` is cyan-default; pink/white links pass a `class` override (no per-color cva growth). Q2 → keep both `soft` (borderless) and `ghost` (bordered). Q3 → one commit per surface cluster.

**Implementation refinement vs spec §3.1:** `radius` is implemented as an optional **Button.vue prop** that appends a `rounded-*` utility (collapsed last-wins by `tailwind-merge`), NOT a cva dimension. This achieves the spec's intent (decoupled radius, no pixel movement for existing call sites) without the `compoundVariants` gymnastics a cva `radius` key would need to preserve `ghost`'s `rounded-lg` default. The baked `rounded-*` in each variant stays as the default.

**Working directory:** `frontend/web/`. All `bunx`/`bun` commands run from there. Git commands run from repo root `/data/animeenigma`.

**Commit/push policy:** Commit after every task. Do NOT `git push` (push happens only via `/animeenigma-after-update`). Never `git add -A` — stage explicit paths only (the working tree carries unrelated changes from other workstreams).

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `frontend/web/src/components/ui/button-variants.ts` | cva: variant/size/base class maps | Modify — add `soft`+`link` variants, `xs`+`icon-sm` sizes, swap raw-rgba shadow → `shadow-glow-*` token, add `link` padding-zero compoundVariant |
| `frontend/web/src/components/ui/Button.vue` | SFC wrapper, props → cva | Modify — add optional `radius` prop, append via `cn()` |
| `frontend/web/src/components/ui/Button.spec.ts` | unit tests on `buttonVariants()` + `Button` | Modify — add cases for new variants/sizes/radius + back-compat regression pins |
| `frontend/web/src/styles/main.css` | global CSS + `.btn-*` helpers | Modify — delete `.btn`, `.btn-primary`, `.btn-secondary`, `.btn-ghost` rules |
| `frontend/web/src/styles/__tests__/design-tokens.spec.ts` | pins token wiring incl. `.btn-*` | Modify — remove the `.btn-*` describe block (lines ~36–53) |
| ~9 `.vue` files using `.btn-*` classes | call sites | Modify — migrate to `<Button>` |
| Surface clusters: `Anime.vue`, `Profile.vue`, players, `Navbar`, misc | raw `<button>` call sites | Modify — swap clean fits to `<Button>` |
| `frontend/web/src/styles/DESIGN-SYSTEM.md` | governance doc | Modify — document new API + bespoke-keep list |
| `/root/.claude/projects/-data-animeenigma/memory/project_design_system_governance.md` | memory | Modify — reflect widened primitive |

---

## Task 1: Widen the cva — new variants, sizes, shadow tokens

**Files:**
- Modify: `frontend/web/src/components/ui/button-variants.ts`
- Test: `frontend/web/src/components/ui/Button.spec.ts`

- [ ] **Step 1: Write the failing tests** — append inside the existing `describe('buttonVariants', …)` block in `Button.spec.ts`:

```ts
  it('soft variant: quiet filled, no glow, no border', () => {
    const c = buttonVariants({ variant: 'soft' })
    expect(c).toContain('bg-white/10')
    expect(c).toContain('hover:bg-white/20')
    expect(c).not.toContain('border')
    expect(c).not.toContain('shadow-glow')
  })

  it('link variant: bare text, brand-cyan, padding zeroed', () => {
    const c = buttonVariants({ variant: 'link' })
    expect(c).toContain('text-cyan-400')
    expect(c).toContain('hover:underline')
    expect(c).toContain('bg-transparent')
    // compoundVariant zeroes the size padding so a link is not box-padded:
    expect(c).toContain('px-0')
  })

  it('new sizes map to expected utilities', () => {
    expect(buttonVariants({ size: 'xs' })).toContain('px-2 py-1 text-xs')
    expect(buttonVariants({ size: 'icon-sm' })).toContain('h-8 w-8 p-0')
  })

  it('default/brand glow uses the shadow-glow token, NOT raw rgba', () => {
    const d = buttonVariants({ variant: 'default' })
    expect(d).toContain('hover:shadow-glow-cyan')
    expect(d).not.toContain('rgba(0,212,255')
    const b = buttonVariants({ variant: 'brand' })
    expect(b).toContain('hover:shadow-glow-pink')
    expect(b).not.toContain('rgba(255,45,124')
  })

  it('legacy primary/secondary aliases still mirror default/brand glow tokens', () => {
    expect(buttonVariants({ variant: 'primary' })).toContain('hover:shadow-glow-cyan')
    expect(buttonVariants({ variant: 'secondary' })).toContain('hover:shadow-glow-pink')
  })
```

- [ ] **Step 2: Run the tests, verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/ui/Button.spec.ts`
Expected: FAIL — new `soft`/`link`/`xs`/`icon-sm` cases fail (cva returns base classes for unknown variant), glow-token cases fail (still raw rgba).

- [ ] **Step 3: Rewrite `button-variants.ts`** to:

```ts
import { cva, type VariantProps } from 'class-variance-authority'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-glow-cyan active:scale-95',
        brand: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-glow-pink active:scale-95',
        ghost: 'bg-white/5 hover:bg-white/10 text-white rounded-lg border border-white/10 hover:border-white/20',
        outline: 'bg-transparent hover:bg-white/5 text-cyan-400 rounded-xl border border-cyan-400/50 hover:border-cyan-400',
        destructive: 'bg-destructive text-destructive-foreground rounded-xl hover:bg-destructive/90 active:scale-95',
        // v2 P1 additions — quiet shapes that absorb hand-rolled inline buttons:
        soft: 'bg-white/10 hover:bg-white/20 text-white rounded-lg',
        link: 'bg-transparent text-cyan-400 hover:text-cyan-300 hover:underline underline-offset-4',
        // DS-NF-04 legacy aliases — mirror default/brand (now token-glow):
        primary: 'bg-primary rounded-xl hover:bg-brand-cyan hover:shadow-glow-cyan active:scale-95',
        secondary: 'bg-brand-pink text-brand-pink-foreground rounded-xl hover:bg-pink-400 hover:shadow-glow-pink active:scale-95',
      },
      size: {
        xs: 'px-2 py-1 text-xs',
        sm: 'px-3 py-1.5 text-sm',
        md: 'px-6 py-3 text-base',
        lg: 'px-8 py-4 text-lg',
        icon: 'h-10 w-10 p-0',
        'icon-sm': 'h-8 w-8 p-0',
      },
    },
    compoundVariants: [
      // A link is text, not a box: zero the size padding and let height be intrinsic.
      { variant: 'link', class: 'px-0 py-0 h-auto' },
    ],
    defaultVariants: { variant: 'default', size: 'md' },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/ui/Button.spec.ts`
Expected: PASS — all existing + new cases green. (The existing `default variant binds … rounded-xl` and `sizes map …` cases must still pass: `rounded-xl` stays baked, old sizes unchanged.)

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: exit 0 (the widened `ButtonVariants` union still narrows; `Button.vue`'s `variant?: NonNullable<ButtonVariants['variant']>` now includes `soft`/`link`).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/button-variants.ts frontend/web/src/components/ui/Button.spec.ts
git commit -m "feat(ui): widen Button cva — add soft/link variants, xs/icon-sm sizes, shadow-glow tokens

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Add the optional `radius` prop to Button.vue

**Files:**
- Modify: `frontend/web/src/components/ui/Button.vue`
- Test: `frontend/web/src/components/ui/Button.spec.ts`

- [ ] **Step 1: Write the failing tests** — append to the `describe('Button.vue back-compat', …)` block:

```ts
  it('radius prop overrides the baked corner via tailwind-merge (last-wins)', () => {
    const round = mount(Button, { props: { radius: 'full' } })
    expect(round.classes()).toContain('rounded-full')
    expect(round.classes()).not.toContain('rounded-xl') // twMerge collapsed the baked default
  })

  it('omitting radius keeps the variant default corner (no pixel movement)', () => {
    const def = mount(Button) // default variant
    expect(def.classes()).toContain('rounded-xl')
    const ghost = mount(Button, { props: { variant: 'ghost' } })
    expect(ghost.classes()).toContain('rounded-lg')
  })
```

- [ ] **Step 2: Run the tests, verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/ui/Button.spec.ts`
Expected: FAIL — `radius` prop not yet defined; `rounded-full` absent.

- [ ] **Step 3: Add the prop to `Button.vue`** — in the `<script setup>` `Props` interface add `radius`, in `withDefaults` leave it undefined, and append the mapped class in the template `:class`:

In the `interface Props` block, add:
```ts
  radius?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
```

Replace the `:class` binding on `<Primitive>` with:
```vue
    :class="cn(buttonVariants({ variant, size }), radius && radiusClass[radius], fullWidth && 'w-full', 'touch-target', props.class)"
```

In `<script setup>`, after the `props` definition, add the lookup:
```ts
const radiusClass = {
  sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl', full: 'rounded-full',
} as const
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/ui/Button.spec.ts`
Expected: PASS — `rounded-full` present when `radius="full"`; defaults unchanged when omitted.

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/Button.vue frontend/web/src/components/ui/Button.spec.ts
git commit -m "feat(ui): add optional radius prop to Button (decoupled corner control)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Build-verify the `shadow-glow-*` utilities resolve

This is a **verification gate**, not a code change (unless the fallback fires). It proves Tailwind v4 emits `shadow-glow-cyan`/`shadow-glow-pink` utilities from the `@theme --shadow-glow-*` tokens introduced into the cva in Task 1.

- [ ] **Step 1: Production build**

Run: `cd frontend/web && bunx vite build`
Expected: exit 0, no "unknown utility" / unresolved-class warnings.

- [ ] **Step 2: Grep the built CSS for the resolved glow shadow**

Run: `cd frontend/web && grep -rl "0 0 30px" dist/assets/*.css`
Expected: at least one match — the `--shadow-glow-cyan` value (`0 0 30px rgba(0,212,255,.3)`) made it into the bundle via the `hover:shadow-glow-cyan` utility.

- [ ] **Step 3: Decision**
  - **If both pass:** no change. Proceed.
  - **If `vite build` errors on `shadow-glow-cyan` (utility not generated):** apply the fallback — in `button-variants.ts`, revert the two glow utilities to a single shared cva fragment to avoid per-variant raw-rgba duplication: define `const GLOW_CYAN = 'hover:shadow-[0_0_30px_rgba(0,212,255,0.3)]'` and `const GLOW_PINK = 'hover:shadow-[0_0_30px_rgba(255,45,124,0.3)]'` at the top of the file and interpolate into the `default`/`primary` and `brand`/`secondary` strings. Update the Task 1 Step 1 glow test to assert the shared fragment is present in default/brand AND primary/secondary (the no-duplication property), then re-run Steps 1–2 of this task. Commit the fallback:
    ```bash
    cd /data/animeenigma
    git add frontend/web/src/components/ui/button-variants.ts frontend/web/src/components/ui/Button.spec.ts
    git commit -m "fix(ui): hoist Button glow to shared cva fragment (Tailwind v4 shadow-glow utility unavailable)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
    ```

---

## Task 4: Reconcile `.btn-*` — migrate call sites, delete CSS

**Files:**
- Modify: ~9 `.vue` call sites (discovered in Step 1)
- Modify: `frontend/web/src/styles/main.css` (delete `.btn` / `.btn-primary` / `.btn-secondary` / `.btn-ghost` rules)
- Modify: `frontend/web/src/styles/__tests__/design-tokens.spec.ts` (remove the `.btn-*` describe block)

- [ ] **Step 1: Enumerate the call sites**

Run:
```bash
cd /data/animeenigma/frontend/web
grep -rnE 'class="[^"]*\bbtn(-primary|-secondary|-ghost)?\b' src --include='*.vue' | grep -v '.spec.'
```
Record every file:line. (Measured baseline: `btn-primary` ×1, `btn-secondary` ×1, `btn-ghost` ×1, bare `.btn` ×6 files. NOTE: `.btn-primary-hero` in `FeaturedCard.vue` is a DIFFERENT class — do NOT touch it.)

- [ ] **Step 2: Migrate each call site** to the primitive. Mapping:
  - `class="btn btn-primary …"` → `<Button variant="default" class="…(remaining layout utils)…">`
  - `class="btn btn-secondary …"` → `<Button variant="brand" …>`
  - `class="btn btn-ghost …"` → `<Button variant="ghost" …>`
  - bare `class="btn …"` with no variant modifier → inspect the element's other classes; if it sets its own bg/color it is a **bespoke keep** (leave raw, record in Task 9 list); otherwise map to the closest variant.

  Ensure each migrated file imports `Button`: `import { Button } from '@/components/ui'`. Move any non-color layout utilities (margins, `w-full`, grid placement) into the `class` prop.

- [ ] **Step 3: Delete the dead CSS** — in `frontend/web/src/styles/main.css`, remove the rule blocks: `.btn` (base, lines ~207–213), `.btn:focus-visible`, `.btn:disabled`, `.btn-primary` (+`:hover`/`:active`), `.btn-secondary` (+`:hover`/`:active`), `.btn-ghost` (+`:hover`). Leave `.btn-primary-hero` (it is not in this file — it's scoped in `FeaturedCard.vue`) and all `.cta-*` / `.glass-*` untouched.

- [ ] **Step 4: Update `design-tokens.spec.ts`** — delete the entire `describe('.btn-* classes reference canonical tokens (value-preserving)', …)` block (lines ~36–53), since those classes no longer exist. The "canonical tokens declared" and "deprecated tokens are aliased" blocks stay.

- [ ] **Step 5: Verify**

Run:
```bash
cd /data/animeenigma/frontend/web
grep -rnE 'class="[^"]*\bbtn(-primary|-secondary|-ghost)?\b' src --include='*.vue' | grep -v '.spec.' | grep -v 'btn-primary-hero'   # expect: no output
bunx vitest run src/styles/__tests__/design-tokens.spec.ts src/components/ui/Button.spec.ts   # expect: PASS
bunx vue-tsc --noEmit   # expect: exit 0
bash scripts/design-system-lint.sh --selftest && cd /data/animeenigma && make lint-design   # expect: SELFTEST PASS + PASS
```

- [ ] **Step 6: In-browser smoke** of any migrated surface (Chrome MCP): load the page(s) touched in Step 2 at desktop (1280px) and mobile (390px); confirm the migrated buttons look identical to before. (If a deploy is needed: `make redeploy-web`.)

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/main.css frontend/web/src/styles/__tests__/design-tokens.spec.ts <migrated .vue files>
git commit -m "refactor(ui): migrate .btn-* call sites to <Button>, delete duplicated CSS

Single source of truth is now the cva. design-tokens.spec .btn-* pins removed.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Tasks 5–8: Surface-cluster swaps (clean fits only)

Tasks 5–8 are the same procedure applied to four clusters, each independently committed and smoke-tested. **No new unit tests** are written per swap — these are value-preserving mechanical refactors gated by the existing test suite, the lint gate, the production build, and an in-browser visual diff. The gate for "done" is **zero visible pixel movement**.

### Per-cluster procedure (apply verbatim in each of Tasks 5–8)

- [ ] **A. Audit** — list every raw `<button>` in the cluster's files:
  `grep -n "<button" <file>` for each file, then read each button's full (possibly multi-line) tag.
- [ ] **B. Classify** each button:
  - **Clean fit** → the rendered box (bg / border / radius / padding / text size / hover) is reproducible by `<Button variant size radius class>` **without moving visible pixels**. Examples: `bg-cyan-500 … rounded-xl px-6 py-3` → `variant="default"`; `text-cyan-400 hover:text-cyan-300` (no bg) → `variant="link"` (+ `class` for a non-cyan tint); `bg-white/10 hover:bg-white/20 rounded-lg` → `variant="soft"`; `px-2 py-1 text-xs` nudge → `size="xs"`.
  - **Bespoke keep** → leave raw; add a one-line reason to the running keep-list for Task 9. Default keeps: anything with custom positioning over video, bespoke animation (`kebab-glow`), carousel arrows, or a shape the widened API still can't model without a diff.
- [ ] **C. Apply** the clean-fit swaps. Add `import { Button } from '@/components/ui'` if missing. Preserve `@click`, `:disabled`, `aria-*`, `data-test`, `type`, `v-if`/`v-for` exactly — move them onto `<Button>` (it forwards native attrs + emits `click` via the `<button>` element). Put layout-only utilities in the `class` prop.
- [ ] **D. Verify**:
  ```bash
  cd /data/animeenigma/frontend/web
  bunx vue-tsc --noEmit          # exit 0
  bunx vitest run                # green (modulo pre-existing AnimeContextMenu.spec.ts:227)
  bunx vite build                # clean
  bash scripts/design-system-lint.sh --selftest && cd /data/animeenigma && make lint-design  # PASS
  ```
- [ ] **E. In-browser smoke** (Chrome MCP, after `make redeploy-web` if needed): load each touched view at 1280px and 390px; confirm the swapped buttons are visually identical and still click/navigate. Capture a screenshot of the heaviest view.
- [ ] **F. Commit** (one commit for the cluster):
  ```bash
  cd /data/animeenigma
  git add <cluster .vue files>
  git commit -m "refactor(ui): swap clean-fit raw buttons to <Button> — <cluster name>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
  ```

### Task 5: `Anime.vue` cluster (heaviest — 29 raw buttons)
Files: `src/views/Anime.vue`. Worked example (representative — verify against the real tag):
```vue
<!-- before -->
<button class="text-sm text-pink-400 hover:text-pink-300" @click="showAll = true">Show all</button>
<!-- after -->
<Button variant="link" size="sm" class="text-pink-400 hover:text-pink-300" @click="showAll = true">Show all</Button>
```
Run the per-cluster procedure A–F.

### Task 6: `Profile.vue` cluster (12 raw buttons)
Files: `src/views/Profile.vue`. Run the per-cluster procedure A–F.

### Task 7: Players cluster
Files: `src/components/player/AnimeLibPlayer.vue` (9), `KodikPlayer.vue` (6), `SubtitleSettingsMenu.vue` (5), `OtherSubsPanel.vue` (4). EXPECT a high bespoke-keep ratio here: transport controls positioned over `<video>`, custom-styled overlay controls, and the seek-nudge segmented controls (these last MAY now fit `variant="soft" size="xs"` — adjudicate each). Run procedure A–F.

### Task 8: Navigation + remaining clusters
Files: `src/components/layout/Navbar.vue` (6), `src/views/Browse.vue` (5), `src/components/layout/FeedbackButton.vue` (4), `src/views/WatchTogetherView.vue` (4), `src/views/admin/RawLibrary.vue` (4), and the long tail of remaining files from the Task-0 inventory (`grep -rln "<button" src --include='*.vue' | grep -v "/ui/" | grep -v spec`). Run procedure A–F. Split into two commits if the tail is large (e.g. `nav` and `admin/misc`).

---

## Task 9: Governance — document the widened API + bespoke-keep list

**Files:**
- Modify: `frontend/web/src/styles/DESIGN-SYSTEM.md`
- Modify: `/root/.claude/projects/-data-animeenigma/memory/project_design_system_governance.md`

- [ ] **Step 1: Update `DESIGN-SYSTEM.md`** — in the "Component inventory" section, replace the Button variant map line with the widened API:
```markdown
Button variants: `default` (primary cyan, glow) · `brand` (pink CTA, glow) · `ghost` (bordered quiet) · `soft` (borderless quiet) · `outline` · `destructive` · legacy `primary`/`secondary` aliases.
Button sizes: `xs` · `sm` · `md` · `lg` · `icon` · `icon-sm`. Optional `radius` prop (`sm|md|lg|xl|full`) overrides the variant's baked corner. `link` variant = bare text link (cyan default; override tint via `class`).
```

- [ ] **Step 2: Add a "Bespoke button keeps" subsection** to `DESIGN-SYSTEM.md` listing every button left raw across Tasks 4–8, each with its one-line reason (from the running keep-list). This supersedes the v1.0 governance-only note that "tiny-icon controls / text-links stay bespoke."

- [ ] **Step 3: Update memory** `project_design_system_governance.md` — note the Button primitive was widened (`soft`/`link`, `xs`/`icon-sm`, `radius`), that `.btn-*` CSS was deleted (cva is the sole source of truth), and that primitive-reuse now has a concrete absorbed-shapes + bespoke-keeps record in `DESIGN-SYSTEM.md`. Add a one-line pointer in `MEMORY.md` if not already covered by the existing design-system entry.

- [ ] **Step 4: Commit**
```bash
cd /data/animeenigma
git add frontend/web/src/styles/DESIGN-SYSTEM.md
git commit -m "docs(design-system): document widened Button API + bespoke-keep list

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
(Memory files live outside the repo — no git add needed for them.)

---

## Task 10: Milestone acceptance gate (spec §3.4)

- [ ] **Step 1: Full verification sweep**
```bash
cd /data/animeenigma/frontend/web
bash scripts/design-system-lint.sh --selftest    # SELFTEST PASS, tree clean
cd /data/animeenigma && make lint-design          # PASS
cd frontend/web
bunx vue-tsc --noEmit                             # exit 0
bunx vitest run                                   # green except pre-existing AnimeContextMenu.spec.ts:227
bunx vite build                                   # clean (proves shadow-glow utilities resolve)
grep -rnE 'class="[^"]*\bbtn(-primary|-secondary|-ghost)?\b' src --include='*.vue' | grep -v '.spec.' | grep -v 'btn-primary-hero'   # NO output (.btn-* fully gone)
```

- [ ] **Step 2: Reuse delta** — record the before/after primitive-adoption numbers:
```bash
cd /data/animeenigma/frontend/web
echo "files using <Button>:"; grep -rln "<Button" src --include='*.vue' | grep -v spec | grep -v '/ui/' | wc -l
echo "files still with raw <button> (excl ui/, spec):"; grep -rln "<button" src --include='*.vue' | grep -v '/ui/' | grep -v spec | wc -l
```
Expected: `<Button>` files up substantially from the baseline 11; raw-button files down from 46 (residual = documented bespoke keeps).

- [ ] **Step 3: Final in-browser smoke** (Chrome MCP, against the deployed site) of the top surfaces — `Anime.vue`, `Profile.vue`, `Navbar` — at 1280px and 390px. Confirm zero visible regression. This is the DS-NF-06 standing-rule gate.

- [ ] **Step 4: Run `/animeenigma-after-update`** — redeploy `web`, append a Russian-Trump-mode changelog entry for the button-reuse work, and push. (This is the project's MUST-USE post-implementation step and the ONLY place `git push` happens.)

---

## Self-Review

**Spec coverage (§3 of the spec):**
- §3.1 widen API (link/soft, xs/icon-sm, radius, shadow tokens) → Tasks 1–2 ✓ (radius refinement documented in header).
- §3.2 reconcile `.btn-*` → Task 4 ✓.
- §3.3 swap clean fits + document keeps → Tasks 5–8 + Task 9 keep-list ✓.
- §3.4 acceptance criteria (1–7) → Task 10 Steps 1–3 + per-cluster D/E ✓ (shadow-glow build gate = Task 3 + Task 10 Step 1).
- §3.5 governance → Task 9 ✓.
- §3.6 out-of-scope (Card/Badge, token hygiene, `.cta-*`, `FeaturedCard.btn-primary-hero`) → explicitly excluded in Tasks 4/9 ✓.

**Placeholder scan:** No TBD/TODO. The per-button swap edits are deliberately produced by the Task 5–8 audit step (the exact 150 edits cannot be enumerated pre-audit without moving pixels blindly); the procedure + worked example + classification rule make each edit unambiguous, and the zero-pixel-movement gate bounds it.

**Type consistency:** `buttonVariants` / `ButtonVariants` / `radiusClass` / prop names (`variant`, `size`, `radius`, `fullWidth`, `href`, `loading`, `class`) are consistent across Tasks 1, 2, and the SFC. New variant names (`soft`, `link`) and sizes (`xs`, `icon-sm`) are used identically in tests and swaps.
