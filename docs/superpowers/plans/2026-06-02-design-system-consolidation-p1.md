# Design System Consolidation — P1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate AnimeEnigma's three overlapping token vocabularies into one layered, shadcn-vue-anchored source of truth in `main.css`, plus a canonical `DESIGN-SYSTEM.md` reference — with **zero rendered change** to the 96 current components.

**Architecture:** Three-tier token model. Tier 1 (existing primitives) is untouched. Tier 2 (shadcn-vue semantic slots) + Tier 3 (brand extension) are **added** to `:root` as raw vars, then exposed to Tailwind v4 via `@theme inline` so utilities like `bg-primary`, `bg-destructive`, `border-border` generate. Old scattered tokens (`--ink*`, `--pink`, `--f-*`, `--accent*`) become deprecated aliases pointing at canonical tokens. Hand-rolled `.btn-*`/`.glass-*` classes are re-pointed to canonical tokens (identical values → identical render).

**Tech Stack:** Tailwind v4 (CSS-first config, `@theme` / `@theme inline`), Vue 3, Vite, Vitest, bun.

**Reference spec:** `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`

**One deferred decision (important):** shadcn's `--accent` means "subtle hover surface," but `--accent` is already brand-cyan in 12 components (18 `var(--accent)` usages). Flipping its meaning would break them. So **P1 keeps `--accent`/`--accent-soft`/`--accent-line`/`--accent-glow` as back-compat aliases of the new `--brand-cyan`**, and the shadcn hover-surface `--accent` meaning is introduced in **P2** after those usages are repointed to `var(--brand-cyan)`. P1 does **not** define a shadcn `--accent` slot.

---

## File Structure

- `frontend/web/src/styles/main.css` — **modified.** Add Tier 2 + Tier 3 raw vars to the existing `:root`; add one `@theme inline` block; convert duplicate tokens to aliases; re-point `.btn-*`/`.glass-*` to canonical tokens. Cascade footguns preserved verbatim.
- `frontend/web/src/styles/DESIGN-SYSTEM.md` — **created.** The canonical day-to-day reference (token tables, usage rules, component inventory, cascade rules). Co-located with `main.css` so it's discoverable.
- `frontend/web/src/styles/__tests__/design-tokens.spec.ts` — **created.** A Vitest guard that reads `main.css` source and asserts canonical tokens + alias wiring exist (drift tripwire).
- `frontend/web/src/styles/__tests__/built-css-utilities.spec.ts` — **created.** A Vitest guard that runs the production CSS build and asserts the new utilities (`.bg-primary`, `.text-destructive`, `.border-border`, …) are emitted.

---

## Task 1: Canonical `DESIGN-SYSTEM.md` reference

**Files:**
- Create: `frontend/web/src/styles/DESIGN-SYSTEM.md`

- [ ] **Step 1: Write the reference doc**

Create `frontend/web/src/styles/DESIGN-SYSTEM.md` with this exact content:

````markdown
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
````

- [ ] **Step 2: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/DESIGN-SYSTEM.md
git commit -m "docs(design): canonical DESIGN-SYSTEM.md reference (P1)"
```

---

## Task 2: Add Tier 2 + Tier 3 raw vars to `:root`

**Files:**
- Modify: `frontend/web/src/styles/main.css` (the `:root` block that currently starts `--spotlight-fade-ms: 400ms;` and contains `--surface-2`, `--ink`, `--accent`, etc.)

- [ ] **Step 1: Read the file first**

Run: `sed -n '1,40p;380,460p' frontend/web/src/styles/main.css` to locate the `@theme` block (≈ line 16) and the `:root` block (≈ line 395, contains `--spotlight-fade-ms`, `--surface-2`, `--ink`, `--accent`, `--r-*`, `--f-*`).

- [ ] **Step 2: Insert the canonical token block inside `:root`**

Inside the existing `:root { … }` block, immediately AFTER the `--violet: #a78bfa;` line (keep all existing lines), insert:

```css
  /* ============================================================
     CANONICAL TOKENS (P1 consolidation — 2026-06-02)
     Tier 2 = shadcn-vue semantic slots. Tier 3 = brand extension.
     Components consume THESE, not the Tier-1 primitives above.
     Ref: ./DESIGN-SYSTEM.md
     ============================================================ */

  /* Tier 2 — shadcn-vue semantic slots */
  --background: var(--color-base);
  --foreground: #ffffff;
  --card: var(--color-surface);
  --card-foreground: #ffffff;
  --popover: var(--elevated);
  --popover-foreground: #ffffff;
  --primary: var(--color-cyan-500);
  --primary-foreground: var(--color-base);
  --secondary: var(--elevated);
  --secondary-foreground: #ffffff;
  --muted: var(--surface-2);
  --muted-foreground: rgba(255, 255, 255, 0.56);
  --border: rgba(255, 255, 255, 0.10);
  --input: rgba(255, 255, 255, 0.12);
  --ring: var(--color-cyan-400);
  /* NOTE: shadcn --accent (hover surface) is deferred to P2 — see DESIGN-SYSTEM.md.
     --accent stays brand-cyan below for back-compat. */

  /* Tier 3 — brand extension (Neon Tokyo) */
  --brand-cyan: var(--color-cyan-400);
  --brand-pink: var(--color-pink-500);
  --brand-pink-foreground: #ffffff;
  --brand-violet: var(--violet);
  --success-foreground: var(--color-base);
  --warning-foreground: var(--color-base);
  --info: #5ab8ff;
  --info-foreground: var(--color-base);
  --destructive: #ff4d4d;
  --destructive-foreground: #ffffff;
  /* soft (chip/badge) backgrounds @ ~14% */
  --primary-soft: rgba(0, 184, 230, 0.14);
  --brand-pink-soft: rgba(255, 45, 124, 0.14);
  --success-soft: rgba(0, 255, 157, 0.14);
  --warning-soft: rgba(255, 214, 0, 0.14);
  --info-soft: rgba(90, 184, 255, 0.14);
  --destructive-soft: rgba(255, 77, 77, 0.14);
```

- [ ] **Step 3: Verify dev build still compiles**

Run: `cd frontend/web && bun run build 2>&1 | tail -5`
Expected: build succeeds (`✓ built in …`), no CSS parse errors.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/main.css
git commit -m "feat(design): add Tier 2 + Tier 3 canonical tokens to :root (P1)"
```

---

## Task 3: Wire canonical tokens into Tailwind via `@theme inline`

**Files:**
- Modify: `frontend/web/src/styles/main.css`

- [ ] **Step 1: Write the failing build-output test**

Create `frontend/web/src/styles/__tests__/built-css-utilities.spec.ts`:

```ts
import { describe, it, expect, beforeAll } from 'vitest'
import { execSync } from 'node:child_process'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

// Builds the app once, then asserts the new semantic utilities are emitted.
const DIST = join(__dirname, '../../../dist/assets')

describe('canonical token utilities are generated', () => {
  let css = ''
  beforeAll(() => {
    execSync('bun run build', { cwd: join(__dirname, '../../..'), stdio: 'ignore' })
    const cssFile = readdirSync(DIST).find((f) => f.endsWith('.css'))
    if (!cssFile) throw new Error('no built CSS found in dist/assets')
    css = readFileSync(join(DIST, cssFile), 'utf8')
  }, 120_000)

  it.each([
    'bg-primary', 'text-primary-foreground', 'bg-secondary',
    'bg-background', 'text-foreground', 'bg-card', 'bg-popover',
    'bg-muted', 'text-muted-foreground', 'border-border',
    'bg-destructive', 'bg-success-soft', 'bg-brand-pink', 'ring-ring',
  ])('emits .%s', (util) => {
    // Tailwind escapes nothing for these simple names; class selector must exist.
    expect(css).toContain(`.${util}`)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd frontend/web && bunx vitest run src/styles/__tests__/built-css-utilities.spec.ts`
Expected: FAIL — utilities like `.bg-primary` / `.bg-destructive` not yet emitted (no `@theme inline` mapping).

- [ ] **Step 3: Add the `@theme inline` block**

In `frontend/web/src/styles/main.css`, immediately AFTER the closing `}` of the existing `@theme { … }` block (the one defining `--color-base`/`--color-cyan-*`, ends before `@keyframes float`), insert:

```css
/* Expose canonical :root tokens to Tailwind v4 so utilities generate
   (bg-primary, text-foreground, border-border, …). `inline` resolves the
   referenced :root var at use-site. Success/warning color utilities already
   come from the @theme block above; here we add their foreground/soft
   companions plus all new slots. Radius is intentionally NOT mapped here —
   that would clobber Tailwind's default rounded-* scale. */
@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-popover: var(--popover);
  --color-popover-foreground: var(--popover-foreground);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-primary-soft: var(--primary-soft);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);

  --color-brand-cyan: var(--brand-cyan);
  --color-brand-pink: var(--brand-pink);
  --color-brand-pink-foreground: var(--brand-pink-foreground);
  --color-brand-pink-soft: var(--brand-pink-soft);
  --color-brand-violet: var(--brand-violet);

  --color-success-foreground: var(--success-foreground);
  --color-success-soft: var(--success-soft);
  --color-warning-foreground: var(--warning-foreground);
  --color-warning-soft: var(--warning-soft);
  --color-info: var(--info);
  --color-info-foreground: var(--info-foreground);
  --color-info-soft: var(--info-soft);
  --color-destructive: var(--destructive);
  --color-destructive-foreground: var(--destructive-foreground);
  --color-destructive-soft: var(--destructive-soft);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/styles/__tests__/built-css-utilities.spec.ts`
Expected: PASS — all listed utilities found.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/main.css frontend/web/src/styles/__tests__/built-css-utilities.spec.ts
git commit -m "feat(design): wire canonical tokens via @theme inline + build-output guard (P1)"
```

---

## Task 4: Convert duplicate tokens to deprecated aliases

**Files:**
- Modify: `frontend/web/src/styles/main.css` (the `:root` block)

- [ ] **Step 1: Write the failing alias-wiring test**

Create `frontend/web/src/styles/__tests__/design-tokens.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const css = readFileSync(join(__dirname, '../main.css'), 'utf8')

describe('canonical tokens declared', () => {
  it.each(['--background', '--foreground', '--primary', '--primary-foreground',
    '--secondary', '--muted', '--muted-foreground', '--border', '--input', '--ring',
    '--brand-cyan', '--brand-pink', '--info', '--destructive', '--success-soft'])(
    'declares %s', (t) => {
      expect(css).toMatch(new RegExp(`\\${t}\\s*:`))
    })
})

describe('deprecated tokens are aliased to canonical ones (no divergent literals)', () => {
  it('--pink aliases --brand-pink', () => {
    expect(css).toMatch(/--pink:\s*var\(--brand-pink\)/)
  })
  it('--ink aliases --foreground', () => {
    expect(css).toMatch(/--ink:\s*var\(--foreground\)/)
  })
  it('--accent stays brand-cyan for P1 back-compat', () => {
    expect(css).toMatch(/--accent:\s*var\(--brand-cyan\)/)
  })
  it('--f-display aliases --font-display', () => {
    expect(css).toMatch(/--f-display:\s*var\(--font-display\)/)
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd frontend/web && bunx vitest run src/styles/__tests__/design-tokens.spec.ts`
Expected: FAIL — `--pink`/`--ink`/`--accent`/`--f-display` still hold literal values, not `var(...)` aliases.

- [ ] **Step 3: Edit the existing duplicate token lines into aliases**

In the `:root` block, replace these existing lines (left) with aliases (right). The COMPUTED VALUE is identical, so rendering does not change:

```css
/* BEFORE  ->  AFTER */
--ink: #ffffff;                         -> --ink: var(--foreground); /* deprecated → --foreground */
--ink-2: rgba(255, 255, 255, 0.78);     -> --ink-2: rgba(255, 255, 255, 0.78); /* deprecated → text-foreground/80; kept (no exact slot) */
--ink-3: rgba(255, 255, 255, 0.56);     -> --ink-3: var(--muted-foreground); /* deprecated → --muted-foreground */
--ink-4: rgba(255, 255, 255, 0.36);     -> --ink-4: rgba(255, 255, 255, 0.36); /* deprecated; kept (no exact slot) */
--accent: #00d4ff;                      -> --accent: var(--brand-cyan); /* deprecated → --brand-cyan (P2 flips to shadcn hover surface) */
--accent-soft: rgba(0, 212, 255, 0.14); -> --accent-soft: var(--primary-soft); /* deprecated → --primary-soft */
--pink: #ff2d7c;                        -> --pink: var(--brand-pink); /* deprecated → --brand-pink */
--pink-soft: rgba(255, 45, 124, 0.14);  -> --pink-soft: var(--brand-pink-soft); /* deprecated → --brand-pink-soft */
--f-display: var(--font-display);       -> (already an alias — leave as-is)
--f-ui: var(--font-sans);               -> (already an alias — leave as-is)
--f-mono: var(--font-mono);             -> (already an alias — leave as-is)
--f-jp: var(--font-jp);                 -> (already an alias — leave as-is)
```

Leave `--surface-2`, `--elevated`, `--line`, `--line-strong`, `--accent-line`, `--accent-glow`, `--violet`, and `--r-*` exactly as they are (they have no exact canonical equivalent, or are referenced by the canonical layer itself).

> The `--f-*` lines are already `var(...)` aliases in the current file — Step 1's test for `--f-display` will already pass; that's fine. The behavior-changing edits are `--ink`, `--ink-3`, `--accent`, `--accent-soft`, `--pink`, `--pink-soft`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/styles/__tests__/design-tokens.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/main.css frontend/web/src/styles/__tests__/design-tokens.spec.ts
git commit -m "refactor(design): alias deprecated tokens to canonical slots (P1)"
```

---

## Task 5: Re-point hand-rolled `.btn-*` / `.glass-*` classes to canonical tokens

**Files:**
- Modify: `frontend/web/src/styles/main.css` (the `.btn-primary`, `.btn-secondary`, `.glass*` rules near the middle of the file)

- [ ] **Step 1: Re-point the button + focus rules**

Edit these rules to reference canonical tokens. Values are identical, so render is unchanged:

```css
/* .btn-primary */
background-color: var(--color-cyan-500);  -> background-color: var(--primary);
color: var(--color-base);                 -> color: var(--primary-foreground);
/* .btn-primary:hover */
background-color: var(--color-cyan-400);  -> background-color: var(--brand-cyan);
box-shadow: var(--shadow-glow-cyan);      -> (leave — shadow token unchanged)

/* .btn-secondary */
background-color: var(--color-pink-500);  -> background-color: var(--brand-pink);
color: white;                             -> color: var(--brand-pink-foreground);
/* .btn-secondary:hover */
background-color: var(--color-pink-400);  -> (leave — no exact token; pink-400 hover stays primitive)

/* .btn:focus-visible and .btn-primary:focus-visible */
box-shadow: 0 0 0 2px var(--color-cyan-400);  -> box-shadow: 0 0 0 2px var(--ring);
```

Leave `.glass`, `.glass-elevated`, `.glass-card`, `.glass-nav`, `.glass-mobile-nav` as-is for P1 (their `rgba(255,255,255,…)` literals have no exact canonical token and re-pointing risks subtle diffs; they migrate in P4). Leave `:focus-visible` global rule as-is OR optionally re-point its `var(--color-cyan-400)` → `var(--ring)` (same value).

- [ ] **Step 2: Build and confirm no errors**

Run: `cd frontend/web && bun run build 2>&1 | tail -3`
Expected: build succeeds.

- [ ] **Step 3: Run the full styles test suite**

Run: `cd frontend/web && bunx vitest run src/styles/`
Expected: PASS (both guard specs).

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/styles/main.css
git commit -m "refactor(design): re-point .btn-* classes to canonical tokens (P1)"
```

---

## Task 6: Full verification gate (type-check, full test run, in-browser smoke)

**Files:** none modified (verification only).

- [ ] **Step 1: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors (CSS-only changes; tests are typed).

- [ ] **Step 2: Full frontend test suite**

Run: `cd frontend/web && bunx vitest run`
Expected: PASS — no regressions in existing specs (spotlight, locales, etc.).

- [ ] **Step 3: Deploy the web build locally**

Run: `cd /data/animeenigma && make redeploy-web 2>&1 | tail -15`
Expected: rebuild + restart succeed. (If `i18n-lint` flakes with phantom missing keys, retry — it's a known race per project memory.)

- [ ] **Step 4: In-browser visual smoke test (mandatory — jsdom cannot catch cascade bugs)**

Open the running site and verify **no rendered diff** vs. before on these surfaces:
- Home (spotlight carousel rotates; cards, CTAs, dots, hover arrows intact)
- Browse (genre filter popup, pagination, cards)
- A watch/player view (one of the 5 players loads; buttons styled)
- A modal/dialog (e.g. genre filter or report button) — glass elevation intact
- Mobile width (≤767px): `PersonalPickCard`'s "+ N more" button shows on mobile only (the `md:hidden` cascade footgun still works)
- Primary (cyan) and secondary (pink) buttons render with correct fills, glows, and focus rings

Use the browser tools (`tabs_context_mcp` → `navigate` → `computer` screenshots) against the local deploy. Capture before/after only if a diff is suspected.

- [ ] **Step 5: Confirm and report**

If all surfaces match: P1 is complete. Report the test output + which surfaces were visually confirmed. Do NOT run `/animeenigma-after-update` yet — wait for explicit "ship it" (per project memory).

---

## Self-Review (completed during authoring)

- **Spec coverage:** §3 token tiers → Tasks 2–3; §4 type/spacing/radius/elevation → documented in Task 1 doc (no token changes needed beyond fonts already aliased); §5 main.css plan (add/alias/preserve-cascade/re-point) → Tasks 2,4,5; §6 component layer → documented in Task 1 inventory (build deferred to P2 per scope); §7 governance → Task 1 doc; §8 roadmap → spec only (this plan IS P1). Covered.
- **Cascade footguns:** Plan never moves `.cta-*` out of `@layer components` and never touches `.spotlight-frame`/`.shuffle-deck`; new blocks are additive. Task 6 Step 4 explicitly re-verifies the `md:hidden` footgun.
- **`--accent` collision:** handled explicitly (kept as brand-cyan alias; shadcn flip deferred to P2). No task introduces a conflicting `--accent`.
- **Placeholder scan:** no TBD/TODO; all CSS and test code is literal.
- **Type consistency:** token names identical across Tasks 1–5 and both test files (`--primary`, `--primary-foreground`, `--brand-pink`, `--destructive`, `--success-soft`, …).
- **Zero-breakage claim:** every re-point in Tasks 4–5 maps to an identical computed value; divergent literals (`--ink-2`, `--ink-4`, glass rgba) are explicitly left untouched.
