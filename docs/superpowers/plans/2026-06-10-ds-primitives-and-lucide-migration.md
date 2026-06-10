# DS Primitives (Spinner · Alert · Avatar) + lucide-vue-next Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five token-bound `@/components/ui` primitives (Spinner, LoadingState, Alert, Avatar, AvatarGroup), adopt `lucide-vue-next` as the icon system, and migrate ~180 standard inline SVGs across 65 files to lucide (keeping bespoke glyphs).

**Architecture:** Each component follows the existing `Badge.vue` shape — `cva` variants in a `{name}-variants.ts` sibling, `cn()` class merge, `withDefaults(defineProps<Props>())`, co-located `.spec.ts`. Spinner is pure-CSS (scoped `<style>`); Alert consumes lucide icons. Migration is a mechanical per-file swap guarded by an ESLint barrel-import rule and per-file in-browser smokes.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Tailwind v4 (semantic tokens), `class-variance-authority` 0.7, `tailwind-merge` 3.6, `lucide-vue-next` (new), Vitest, bun/bunx.

**Conventions:**
- Frontend uses **bun** — `bun add`, `bunx vitest`, `bunx tsc`.
- Colors: semantic tokens only in new components (`bg-info-soft`, `text-success`, `border-destructive/30`, `bg-brand-cyan/15`). No raw hex in `.vue` (design-system-lint gate). Spinner scoped CSS uses `var(--brand-cyan)`/`var(--brand-pink)`/`currentColor`.
- DS-NF-06: every rendered change gets an in-browser smoke at desktop + mobile.
- Commit co-authors (append to every commit message):
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Path-scoped `git add` only (repo has unrelated uncommitted files — never `git add -A`).
- Push after each commit (realtime backup).

**Spec:** `docs/superpowers/specs/2026-06-10-ds-primitives-spinner-alert-avatar-design.md`

---

## File Structure

```
frontend/web/src/components/ui/
├─ Spinner.vue            (new)  spinner-variants.ts  Spinner.spec.ts
├─ LoadingState.vue       (new)                       LoadingState.spec.ts
├─ Alert.vue              (new)  alert-variants.ts    Alert.spec.ts
├─ Avatar.vue             (new)  avatar-variants.ts   Avatar.spec.ts
├─ AvatarGroup.vue        (new)                        AvatarGroup.spec.ts
└─ index.ts              (edit)  + 8 exports
frontend/web/
├─ package.json          (edit)  + lucide-vue-next
└─ eslint.config.* / .eslintrc (edit) + no-barrel-import rule
```

**Phases:** Phase 1 dependency+guardrail · Phase 2 the five components (TDD) · Phase 3 the 65-file icon migration (batched).

---

# PHASE 1 — Dependency & Guardrail

### Task 1: Add lucide-vue-next + barrel-import guardrail

**Files:**
- Modify: `frontend/web/package.json`
- Modify: `frontend/web/eslint.config.ts` (or `.eslintrc.cjs` — whichever exists)

- [ ] **Step 1: Install the dependency**

Run:
```bash
cd frontend/web && bun add lucide-vue-next
```
Expected: `package.json` gains `"lucide-vue-next": "^0.x"`, `bun.lock` updated.

- [ ] **Step 2: Verify it imports per-icon (tree-shake sanity)**

Run:
```bash
cd frontend/web && bunx tsc --noEmit -e 2>/dev/null; node -e "import('lucide-vue-next').then(m=>console.log(typeof m.X, typeof m.Info))"
```
Expected: prints `object object` (named icon components resolve).

- [ ] **Step 3: Locate the ESLint config**

Run:
```bash
cd frontend/web && ls eslint.config.* .eslintrc.* 2>/dev/null
```
Expected: one file path (e.g. `eslint.config.ts`). Use it in Step 4.

- [ ] **Step 4: Add a no-barrel-import rule**

Add to the rules block of the located config (adapt syntax to flat vs legacy config):

```js
'no-restricted-imports': ['error', {
  paths: [{
    name: 'lucide-vue-next',
    importNames: ['default'],
    message: 'Import named icons only, e.g. `import { X } from "lucide-vue-next"`. No default/namespace import (defeats tree-shaking).',
  }],
  patterns: [{
    group: ['lucide-vue-next/*'],
    message: 'Import from the package root: `import { X } from "lucide-vue-next"`.',
  }],
}],
```

- [ ] **Step 5: Verify the rule fires**

Create a throwaway file and lint it:
```bash
cd frontend/web && printf "import * as i from 'lucide-vue-next'\nconsole.log(i)\n" > /tmp/_barrel.ts && bunx eslint /tmp/_barrel.ts; rm -f /tmp/_barrel.ts
```
Expected: ESLint reports the no-restricted-imports error (namespace import flagged). (If the namespace form isn't caught by `importNames`, the `patterns` entry plus the existing config still forbids subpath imports; that is sufficient — proceed.)

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/package.json frontend/web/bun.lock frontend/web/eslint.config.* frontend/web/.eslintrc.* 2>/dev/null
git commit -m "build(web): add lucide-vue-next + forbid barrel imports

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

# PHASE 2 — The Five Components (TDD)

### Task 2: Spinner

**Files:**
- Create: `frontend/web/src/components/ui/spinner-variants.ts`
- Create: `frontend/web/src/components/ui/Spinner.vue`
- Test: `frontend/web/src/components/ui/Spinner.spec.ts`

- [ ] **Step 1: Write the variants file**

`spinner-variants.ts`:
```ts
import { cva, type VariantProps } from 'class-variance-authority'

// Dual counter-rotating arc spinner. The rings are drawn with pure-CSS
// pseudo-elements in Spinner.vue; this cva only maps size + tone to the
// marker classes those scoped styles key off of.
export const spinnerVariants = cva('ae-spinner inline-block align-middle', {
  variants: {
    size: {
      xs: 'ae-spinner--xs',
      sm: 'ae-spinner--sm',
      md: 'ae-spinner--md',
      lg: 'ae-spinner--lg',
      xl: 'ae-spinner--xl',
    },
    tone: {
      signature: 'ae-spinner--signature',
      mono: 'ae-spinner--mono',
    },
  },
  defaultVariants: { size: 'md', tone: 'signature' },
})

export type SpinnerVariants = VariantProps<typeof spinnerVariants>
```

- [ ] **Step 2: Write the failing test**

`Spinner.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { spinnerVariants } from './spinner-variants'
import Spinner from './Spinner.vue'

describe('spinnerVariants', () => {
  it('maps size to marker class', () => {
    expect(spinnerVariants({ size: 'sm' })).toContain('ae-spinner--sm')
    expect(spinnerVariants({ size: 'xl' })).toContain('ae-spinner--xl')
  })
  it('maps tone to marker class', () => {
    expect(spinnerVariants({ tone: 'mono' })).toContain('ae-spinner--mono')
  })
  it('defaults to md + signature', () => {
    const c = spinnerVariants({})
    expect(c).toContain('ae-spinner--md')
    expect(c).toContain('ae-spinner--signature')
  })
})

describe('Spinner.vue', () => {
  it('has role=status and a visually-hidden label', () => {
    const w = mount(Spinner, { props: { label: 'Загрузка' } })
    expect(w.attributes('role')).toBe('status')
    const sr = w.find('.sr-only')
    expect(sr.exists()).toBe(true)
    expect(sr.text()).toBe('Загрузка')
  })
  it('applies size + tone classes', () => {
    const w = mount(Spinner, { props: { size: 'lg', tone: 'mono' } })
    expect(w.classes()).toContain('ae-spinner--lg')
    expect(w.classes()).toContain('ae-spinner--mono')
  })
  it('defaults label to "Loading"', () => {
    const w = mount(Spinner)
    expect(w.find('.sr-only').text()).toBe('Loading')
  })
})
```

- [ ] **Step 3: Run test — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/ui/Spinner.spec.ts`
Expected: FAIL — `Spinner.vue` does not exist.

- [ ] **Step 4: Write the component**

`Spinner.vue`:
```vue
<template>
  <span :class="cn(spinnerVariants({ size, tone }), props.class)" role="status">
    <span class="sr-only">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { spinnerVariants, type SpinnerVariants } from './spinner-variants'

interface Props {
  size?: NonNullable<SpinnerVariants['size']>
  tone?: NonNullable<SpinnerVariants['tone']>
  label?: string
  class?: HTMLAttributes['class']
}

withDefaults(defineProps<Props>(), { size: 'md', tone: 'signature', label: 'Loading' })
</script>

<style scoped>
.ae-spinner { position: relative; }
.ae-spinner::before,
.ae-spinner::after {
  content: '';
  position: absolute;
  border-radius: 9999px;
  border-style: solid;
  border-color: transparent;
  box-sizing: border-box;
}
.ae-spinner::before { inset: 0; animation: ae-spin 0.8s linear infinite; }
.ae-spinner::after { animation: ae-spin-rev 1.1s linear infinite; }

/* signature: cyan outer ring + pink inner ring */
.ae-spinner--signature::before { border-top-color: var(--brand-cyan); border-bottom-color: var(--brand-cyan); }
.ae-spinner--signature::after { border-left-color: var(--brand-pink); border-right-color: var(--brand-pink); }

/* mono: both rings follow currentColor (inner dimmed) */
.ae-spinner--mono::before { border-top-color: currentColor; border-bottom-color: currentColor; }
.ae-spinner--mono::after { border-left-color: currentColor; border-right-color: currentColor; opacity: 0.55; }

.ae-spinner--xs { width: 14px; height: 14px; }
.ae-spinner--xs::before { border-width: 2px; }
.ae-spinner--xs::after { inset: 3px; border-width: 2px; }
.ae-spinner--sm { width: 18px; height: 18px; }
.ae-spinner--sm::before { border-width: 2px; }
.ae-spinner--sm::after { inset: 4px; border-width: 2px; }
.ae-spinner--md { width: 24px; height: 24px; }
.ae-spinner--md::before { border-width: 3px; }
.ae-spinner--md::after { inset: 5px; border-width: 3px; }
.ae-spinner--lg { width: 36px; height: 36px; }
.ae-spinner--lg::before { border-width: 3px; }
.ae-spinner--lg::after { inset: 7px; border-width: 3px; }
.ae-spinner--xl { width: 52px; height: 52px; }
.ae-spinner--xl::before { border-width: 4px; }
.ae-spinner--xl::after { inset: 10px; border-width: 4px; }

@keyframes ae-spin { to { transform: rotate(360deg); } }
@keyframes ae-spin-rev { to { transform: rotate(-360deg); } }

@media (prefers-reduced-motion: reduce) {
  .ae-spinner::before, .ae-spinner::after { animation-duration: 2.4s; }
}
</style>
```

- [ ] **Step 5: Run test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/ui/Spinner.spec.ts`
Expected: PASS (6 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/Spinner.vue frontend/web/src/components/ui/spinner-variants.ts frontend/web/src/components/ui/Spinner.spec.ts
git commit -m "feat(ui): Spinner — dual-arc donut (signature/mono tones)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 3: LoadingState

**Files:**
- Create: `frontend/web/src/components/ui/LoadingState.vue`
- Test: `frontend/web/src/components/ui/LoadingState.spec.ts`

- [ ] **Step 1: Write the failing test**

`LoadingState.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import LoadingState from './LoadingState.vue'
import Spinner from './Spinner.vue'

describe('LoadingState.vue', () => {
  it('renders a Spinner', () => {
    const w = mount(LoadingState)
    expect(w.findComponent(Spinner).exists()).toBe(true)
  })
  it('shows the label when provided', () => {
    const w = mount(LoadingState, { props: { label: 'Loading episodes…' } })
    expect(w.text()).toContain('Loading episodes…')
  })
  it('omits the label element when no label', () => {
    const w = mount(LoadingState)
    expect(w.find('[data-testid="loadingstate-label"]').exists()).toBe(false)
  })
  it('forwards size + tone to the Spinner', () => {
    const w = mount(LoadingState, { props: { size: 'md', tone: 'mono' } })
    const sp = w.findComponent(Spinner)
    expect(sp.props('size')).toBe('md')
    expect(sp.props('tone')).toBe('mono')
  })
  it('is centered (flex-col items-center)', () => {
    const w = mount(LoadingState)
    expect(w.classes()).toContain('flex-col')
    expect(w.classes()).toContain('items-center')
  })
})
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/ui/LoadingState.spec.ts`
Expected: FAIL — `LoadingState.vue` does not exist.

- [ ] **Step 3: Write the component**

`LoadingState.vue`:
```vue
<template>
  <div :class="cn('flex flex-col items-center justify-center gap-3 p-8', props.class)" role="status">
    <Spinner :size="size" :tone="tone" :label="label ?? 'Loading'" />
    <span v-if="label" data-testid="loadingstate-label" class="text-sm text-muted-foreground">{{ label }}</span>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import Spinner from './Spinner.vue'
import type { SpinnerVariants } from './spinner-variants'

interface Props {
  label?: string
  size?: NonNullable<SpinnerVariants['size']>
  tone?: NonNullable<SpinnerVariants['tone']>
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { size: 'lg', tone: 'signature' })
</script>
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/ui/LoadingState.spec.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/LoadingState.vue frontend/web/src/components/ui/LoadingState.spec.ts
git commit -m "feat(ui): LoadingState — centered Spinner + optional label

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 4: Alert

**Files:**
- Create: `frontend/web/src/components/ui/alert-variants.ts`
- Create: `frontend/web/src/components/ui/Alert.vue`
- Test: `frontend/web/src/components/ui/Alert.spec.ts`

- [ ] **Step 1: Write the variants file**

`alert-variants.ts`:
```ts
import { cva, type VariantProps } from 'class-variance-authority'

export const alertVariants = cva('flex items-start gap-3 rounded-xl border p-4 text-sm', {
  variants: {
    variant: {
      info: 'bg-info-soft border-info/30',
      success: 'bg-success-soft border-success/30',
      warning: 'bg-warning-soft border-warning/30',
      destructive: 'bg-destructive-soft border-destructive/30',
    },
  },
  defaultVariants: { variant: 'info' },
})

export type AlertVariants = VariantProps<typeof alertVariants>

export type AlertVariant = NonNullable<AlertVariants['variant']>

// Per-variant icon tint (separate from the cva so tailwind-merge keeps bg/border
// and text colors in independent groups).
export const alertIconColor: Record<AlertVariant, string> = {
  info: 'text-info',
  success: 'text-success',
  warning: 'text-warning',
  destructive: 'text-destructive',
}
```

- [ ] **Step 2: Write the failing test**

`Alert.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { alertVariants, alertIconColor } from './alert-variants'
import Alert from './Alert.vue'

describe('alertVariants', () => {
  it('info binds info-soft bg + info border', () => {
    const c = alertVariants({ variant: 'info' })
    expect(c).toContain('bg-info-soft')
    expect(c).toContain('border-info/30')
  })
  it('destructive binds destructive-soft', () => {
    expect(alertVariants({ variant: 'destructive' })).toContain('bg-destructive-soft')
  })
  it('defaults to info', () => {
    expect(alertVariants({})).toContain('bg-info-soft')
  })
  it('icon color map covers all variants', () => {
    expect(alertIconColor.warning).toBe('text-warning')
    expect(alertIconColor.success).toBe('text-success')
  })
})

describe('Alert.vue', () => {
  it('has role=alert and default info classes', () => {
    const w = mount(Alert)
    expect(w.attributes('role')).toBe('alert')
    expect(w.classes()).toContain('bg-info-soft')
  })
  it('renders the title when provided', () => {
    const w = mount(Alert, { props: { title: 'Heads up' } })
    expect(w.text()).toContain('Heads up')
  })
  it('renders default slot body', () => {
    const w = mount(Alert, { slots: { default: 'Body text' } })
    expect(w.text()).toContain('Body text')
  })
  it('shows close button + emits dismiss only when dismissible', () => {
    const off = mount(Alert)
    expect(off.find('button[aria-label]').exists()).toBe(false)
    const on = mount(Alert, { props: { dismissible: true } })
    const btn = on.find('button[aria-label]')
    expect(btn.exists()).toBe(true)
    btn.trigger('click')
    expect(on.emitted('dismiss')).toBeTruthy()
  })
  it('uses #icon slot to override the default icon', () => {
    const w = mount(Alert, { slots: { icon: '<i class="custom-icon" />' } })
    expect(w.find('.custom-icon').exists()).toBe(true)
  })
})
```

- [ ] **Step 3: Run test — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/ui/Alert.spec.ts`
Expected: FAIL — `Alert.vue` does not exist.

- [ ] **Step 4: Write the component**

`Alert.vue`:
```vue
<template>
  <div :class="cn(alertVariants({ variant }), props.class)" role="alert">
    <span :class="cn('mt-0.5 shrink-0', alertIconColor[variant])" aria-hidden="true">
      <slot name="icon">
        <component :is="defaultIcon" class="size-[18px]" />
      </slot>
    </span>

    <div class="min-w-0 flex-1">
      <div v-if="title" class="font-semibold text-foreground">{{ title }}</div>
      <div class="text-muted-foreground [overflow-wrap:anywhere]"><slot /></div>
    </div>

    <button
      v-if="dismissible"
      type="button"
      class="-my-1 -mr-1 ml-auto shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-white/10 hover:text-foreground"
      :aria-label="dismissLabel"
      @click="emit('dismiss')"
    >
      <X class="size-4" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { Info, CircleCheck, TriangleAlert, CircleX, X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { alertVariants, alertIconColor, type AlertVariant } from './alert-variants'

interface Props {
  variant?: AlertVariant
  title?: string
  dismissible?: boolean
  dismissLabel?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'info',
  dismissible: false,
  dismissLabel: 'Dismiss',
})

const emit = defineEmits<{ dismiss: [] }>()

const icons = { info: Info, success: CircleCheck, warning: TriangleAlert, destructive: CircleX }
const defaultIcon = computed(() => icons[props.variant])
</script>
```

- [ ] **Step 5: Run test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/ui/Alert.spec.ts`
Expected: PASS (9 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/Alert.vue frontend/web/src/components/ui/alert-variants.ts frontend/web/src/components/ui/Alert.spec.ts
git commit -m "feat(ui): Alert — 4 status variants, dismissible, lucide icons

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 5: Avatar

**Files:**
- Create: `frontend/web/src/components/ui/avatar-variants.ts`
- Create: `frontend/web/src/components/ui/Avatar.vue`
- Test: `frontend/web/src/components/ui/Avatar.spec.ts`

- [ ] **Step 1: Write the variants file**

`avatar-variants.ts`:
```ts
import { cva, type VariantProps } from 'class-variance-authority'

export const avatarVariants = cva('relative inline-flex shrink-0 align-middle', {
  variants: {
    size: {
      xs: 'size-6 text-[10px]',
      sm: 'size-8 text-xs',
      md: 'size-10 text-sm',
      lg: 'size-12 text-[17px]',
      xl: 'size-16 text-[22px]',
    },
  },
  defaultVariants: { size: 'md' },
})

export type AvatarVariants = VariantProps<typeof avatarVariants>
export type AvatarSize = NonNullable<AvatarVariants['size']>
export type AvatarStatus = 'online' | 'idle' | 'offline'

export const avatarDotSize: Record<AvatarSize, string> = {
  xs: 'size-2.5', sm: 'size-2.5', md: 'size-3', lg: 'size-3.5', xl: 'size-4',
}
export const avatarDotColor: Record<AvatarStatus, string> = {
  online: 'bg-success', idle: 'bg-warning', offline: 'bg-white/30',
}

export function avatarInitials(name?: string): string {
  const n = (name ?? '').trim()
  if (!n) return '?'
  return n.split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p[0]).join('').toUpperCase()
}
```

- [ ] **Step 2: Write the failing test**

`Avatar.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { avatarVariants, avatarInitials } from './avatar-variants'
import Avatar from './Avatar.vue'

describe('avatar helpers', () => {
  it('size maps to size-* class', () => {
    expect(avatarVariants({ size: 'lg' })).toContain('size-12')
  })
  it('initials: two words → 2 letters', () => {
    expect(avatarInitials('Alice Brown')).toBe('AB')
  })
  it('initials: one word → 1 letter', () => {
    expect(avatarInitials('Yuki')).toBe('Y')
  })
  it('initials: empty → ?', () => {
    expect(avatarInitials('')).toBe('?')
    expect(avatarInitials(undefined)).toBe('?')
  })
})

describe('Avatar.vue', () => {
  it('renders <img> when src is set', () => {
    const w = mount(Avatar, { props: { src: 'https://x/y.png', name: 'Al B' } })
    const img = w.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('https://x/y.png')
  })
  it('falls back to initials on image error', async () => {
    const w = mount(Avatar, { props: { src: 'https://x/broken.png', name: 'Al B' } })
    await w.find('img').trigger('error')
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('AB')
  })
  it('renders initials when no src', () => {
    const w = mount(Avatar, { props: { name: 'Static Virtual' } })
    expect(w.text()).toContain('SV')
  })
  it('renders presence dot with the right color', () => {
    const w = mount(Avatar, { props: { name: 'A', status: 'online' } })
    expect(w.find('.bg-success').exists()).toBe(true)
  })
  it('omits the dot when no status', () => {
    const w = mount(Avatar, { props: { name: 'A' } })
    expect(w.find('.bg-success').exists()).toBe(false)
  })
})
```

- [ ] **Step 3: Run test — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/ui/Avatar.spec.ts`
Expected: FAIL — `Avatar.vue` does not exist.

- [ ] **Step 4: Write the component**

`Avatar.vue`:
```vue
<template>
  <span :class="cn(avatarVariants({ size }), props.class)">
    <span class="flex size-full items-center justify-center overflow-hidden rounded-full bg-brand-cyan/15 font-semibold leading-none text-brand-cyan">
      <img
        v-if="src && !errored"
        :src="src"
        :alt="name ?? ''"
        class="size-full object-cover"
        @error="errored = true"
      />
      <template v-else>{{ initials }}</template>
    </span>
    <span
      v-if="status"
      :class="cn('absolute bottom-0 right-0 rounded-full ring-2 ring-background', avatarDotSize[size], avatarDotColor[status])"
    />
  </span>
</template>

<script setup lang="ts">
import { ref, computed, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import {
  avatarVariants, avatarInitials, avatarDotSize, avatarDotColor,
  type AvatarSize, type AvatarStatus,
} from './avatar-variants'

interface Props {
  src?: string
  name?: string
  size?: AvatarSize
  status?: AvatarStatus
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { size: 'md' })
const errored = ref(false)
const initials = computed(() => avatarInitials(props.name))
</script>
```

- [ ] **Step 5: Run test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/ui/Avatar.spec.ts`
Expected: PASS (10 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/Avatar.vue frontend/web/src/components/ui/avatar-variants.ts frontend/web/src/components/ui/Avatar.spec.ts
git commit -m "feat(ui): Avatar — img + initials fallback, sizes, presence dot

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 6: AvatarGroup

**Files:**
- Create: `frontend/web/src/components/ui/AvatarGroup.vue`
- Test: `frontend/web/src/components/ui/AvatarGroup.spec.ts`

- [ ] **Step 1: Write the failing test**

`AvatarGroup.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AvatarGroup from './AvatarGroup.vue'
import Avatar from './Avatar.vue'

const items = [
  { name: 'A B' }, { name: 'C D' }, { name: 'E F' },
  { name: 'G H' }, { name: 'I J' }, { name: 'K L' },
]

describe('AvatarGroup.vue', () => {
  it('caps visible avatars at max and shows +N overflow', () => {
    const w = mount(AvatarGroup, { props: { items, max: 4 } })
    expect(w.findAllComponents(Avatar)).toHaveLength(4)
    expect(w.text()).toContain('+2')
  })
  it('shows no overflow chip when items <= max', () => {
    const w = mount(AvatarGroup, { props: { items: items.slice(0, 3), max: 4 } })
    expect(w.findAllComponents(Avatar)).toHaveLength(3)
    expect(w.text()).not.toContain('+')
  })
  it('forwards size to children', () => {
    const w = mount(AvatarGroup, { props: { items: items.slice(0, 2), size: 'sm' } })
    expect(w.findComponent(Avatar).props('size')).toBe('sm')
  })
})
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/ui/AvatarGroup.spec.ts`
Expected: FAIL — `AvatarGroup.vue` does not exist.

- [ ] **Step 3: Write the component**

`AvatarGroup.vue`:
```vue
<template>
  <div :class="cn('flex items-center', props.class)">
    <Avatar
      v-for="(a, i) in visible"
      :key="i"
      v-bind="a"
      :size="size"
      :class="cn('ring-2 ring-background', i > 0 && '-ml-2.5')"
    />
    <span
      v-if="overflow > 0"
      :class="cn('relative -ml-2.5 inline-flex items-center justify-center rounded-full bg-muted font-mono text-muted-foreground ring-2 ring-background', sizeClass)"
    >+{{ overflow }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import Avatar from './Avatar.vue'
import { avatarVariants, type AvatarSize, type AvatarStatus } from './avatar-variants'

interface AvatarItem { src?: string; name?: string; status?: AvatarStatus }

interface Props {
  items: AvatarItem[]
  max?: number
  size?: AvatarSize
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), { max: 4, size: 'md' })
const visible = computed(() => props.items.slice(0, props.max))
const overflow = computed(() => Math.max(0, props.items.length - props.max))
const sizeClass = computed(() => avatarVariants({ size: props.size }))
</script>
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/ui/AvatarGroup.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/AvatarGroup.vue frontend/web/src/components/ui/AvatarGroup.spec.ts
git commit -m "feat(ui): AvatarGroup — overlap stack with +N overflow

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Task 7: Export from index.ts + full gate

**Files:**
- Modify: `frontend/web/src/components/ui/index.ts`

- [ ] **Step 1: Add the exports**

Append to `index.ts` (keep alphabetical-ish grouping consistent with the file):
```ts
export { default as Spinner } from './Spinner.vue'
export { spinnerVariants, type SpinnerVariants } from './spinner-variants'
export { default as LoadingState } from './LoadingState.vue'
export { default as Alert } from './Alert.vue'
export { alertVariants, type AlertVariants } from './alert-variants'
export { default as Avatar } from './Avatar.vue'
export { avatarVariants, type AvatarVariants } from './avatar-variants'
export { default as AvatarGroup } from './AvatarGroup.vue'
```

- [ ] **Step 2: Run the full component suite + type-check + lint gate**

Run:
```bash
cd frontend/web && bunx vitest run src/components/ui/ && bunx tsc --noEmit && bash scripts/design-system-lint.sh
```
Expected: all specs PASS, `tsc` clean, design-system-lint `ERRORS: 0`.

- [ ] **Step 3: In-browser smoke (DS-NF-06)**

Build/serve the dev frontend, mount the five components on a scratch route (or Storybook-style harness page), and verify at **desktop + mobile** widths: spinner spins (signature cyan+pink; mono in a button), all 4 Alert variants tint correctly with lucide icons + working dismiss, Avatar image→initials fallback + presence dots, AvatarGroup overlap + `+N`.
Expected: visually correct, no console errors.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/ui/index.ts
git commit -m "feat(ui): export Spinner/LoadingState/Alert/Avatar/AvatarGroup

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

# PHASE 3 — lucide Migration of Existing Inline SVGs

> Mechanical swap of ~180 standard inline `<svg>` → lucide components across 65 files, **skipping the keep-list** (spec §8: SpotlightIcon, score ◆, MAL ★ identity marks, provider/brand logos, the dual-arc spinner, decorative one-offs). Batched by area; each batch ends with build + browser smoke + commit. Use the canonical mapping in spec §8.

### Task 8: Build the migration inventory

**Files:**
- Create: `docs/superpowers/plans/lucide-migration-inventory.md` (working checklist — not shipped code)

- [ ] **Step 1: Generate the file inventory**

Run:
```bash
cd frontend/web && grep -rc '<svg' src --include=*.vue | grep -v ':0$' | sort -t: -k2 -rn
```
Expected: ~65 lines `path:count`. Paste into the inventory doc as a checklist.

- [ ] **Step 2: Tag keep-list files**

In the inventory, mark each file `MIGRATE` or `KEEP` (bespoke). Definite `KEEP`: `components/home/spotlight/SpotlightIcon.vue`, any file whose SVGs are the score-diamond/star identity marks or provider logos. When unsure, default `MIGRATE` and re-tag during the batch if no lucide equivalent fits.

- [ ] **Step 3: Commit the inventory**

```bash
cd /data/animeenigma
git add docs/superpowers/plans/lucide-migration-inventory.md
git commit -m "docs(ds): lucide migration inventory + keep-list tags

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git push
```

---

### Worked example (the procedure applied to one real file)

Before — `components/player/KodikAdFreePlayer.vue` close button (illustrative):
```html
<button @click="dismiss" aria-label="Close">
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
  </svg>
</button>
```
After:
```html
<button @click="dismiss" aria-label="Close">
  <X class="size-5" />
</button>
```
```ts
// in <script setup>
import { X } from 'lucide-vue-next'
```
Notes: `w-5 h-5` → `size-5`; `stroke="currentColor"` is lucide's default, so the surrounding `text-*` class drives color (add one if the old SVG hardcoded a stroke class). Keep the existing `aria-label` on the button.

### Tasks 9–13: Migrate by area (repeat the same step pattern per batch)

For **each** batch below, perform these steps:

- [ ] **Step A:** For every `MIGRATE` file in the batch, apply the worked-example procedure to each inline `<svg>` (spec §8 mapping; lucide.dev for the rest; re-tag `KEEP` if no fit).
- [ ] **Step B:** Type-check: `cd frontend/web && bunx tsc --noEmit` → clean.
- [ ] **Step C:** Lint + DS gate: `bunx eslint src/<area>` and `bash scripts/design-system-lint.sh` → no new errors, `ERRORS: 0`.
- [ ] **Step D:** Run any specs touching the batch: `bunx vitest run src/<area>` → PASS.
- [ ] **Step E:** **In-browser smoke at desktop + mobile** for the changed screens (DS-NF-06) → icons correct size/alignment/color, no console errors.
- [ ] **Step F:** Commit the batch:
  ```bash
  git add frontend/web/src/<area>
  git commit -m "refactor(web): migrate <area> inline SVGs to lucide

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
  git push
  ```

**Batches:**
- [ ] **Task 9 — `components/ui` + remaining shared primitives** (e.g. `Checkbox.vue` inline check, `Modal`, `Select`, `DropdownMenu` chevrons). `<area>` = `src/components/ui`.
- [ ] **Task 10 — players** (`components/player/**`: Kodik, KodikAdFree, AnimeLib, Hanime, Anime18, OurEnglish, unified/*). KEEP any provider-logo glyphs. `<area>` = `src/components/player`.
- [ ] **Task 11 — views** (`views/**`: Profile 28, Anime 21, Browse 6, admin/*, etc.). `<area>` = `src/views`.
- [ ] **Task 12 — home + spotlight (non-glyph)** (`components/home/**` EXCEPT `SpotlightIcon.vue` and identity-mark glyphs). `<area>` = `src/components/home`.
- [ ] **Task 13 — layout + remainder** (`components/layout/**`, `components/anime/**`, `components/watch-together/**`, anything left). `<area>` = sweep remaining dirs; confirm with `grep -rc '<svg' src --include=*.vue | grep -v ':0$'` showing only KEEP files remain.

---

### Task 14: Final full-app verification

- [ ] **Step 1: Confirm only keep-list SVGs remain**

Run:
```bash
cd frontend/web && grep -rl '<svg' src --include=*.vue
```
Expected: only keep-list files (SpotlightIcon, identity-mark/provider-logo files, the Spinner is CSS so it won't appear). Anything unexpected → migrate or document as KEEP.

- [ ] **Step 2: Full gate**

Run:
```bash
cd frontend/web && bunx vitest run && bunx tsc --noEmit && bunx eslint src && bash scripts/design-system-lint.sh
```
Expected: all PASS, `tsc` clean, ESLint clean (incl. no barrel imports), design-system-lint `ERRORS: 0`.

- [ ] **Step 3: Production build sanity (tree-shake check)**

Run:
```bash
cd frontend/web && bun run build && ls -la dist/assets/*.js | sort -k5 -n | tail -5
```
Expected: build succeeds; bundle sizes comparable to or smaller than the pre-migration baseline (deduped icons). Note the largest chunks; investigate if any chunk ballooned (sign of an accidental broad import).

- [ ] **Step 4: Run /animeenigma-after-update**

Invoke the `/animeenigma-after-update` skill to redeploy web, update the changelog (Russian Trump-mode), health-check, and push. (This is the project's mandated post-implementation step.)

---

## Self-Review

**Spec coverage:** Spinner (T2), LoadingState (T3), Alert incl. dismissible/#icon/variants/tokens (T4), Avatar incl. fallback/dot (T5), AvatarGroup +N (T6), index exports (T7), lucide adoption + guardrail (T1), full migration + keep-list + procedure + per-file smoke (T8–T14), DS compliance gate (T7/T14). All spec sections mapped.

**Placeholder scan:** No "TBD"/"handle edge cases"/"similar to". Migration tasks 9–13 share one explicit step pattern (A–F) applied to enumerated batches with a worked example — a procedure over a real file list, not a placeholder.

**Type consistency:** `SpinnerVariants`/`AvatarVariants`/`AlertVariants` exported from their variants files and imported where used; `avatarInitials`/`avatarDotSize`/`avatarDotColor`/`alertIconColor` defined in Task 4/5 and consumed in the same components; `AvatarStatus`/`AvatarSize`/`AvatarVariant` names consistent across Avatar, AvatarGroup, Alert.
