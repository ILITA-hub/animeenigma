# Secret Feature Tab («Секретная фича») Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Anidle/Downloads header links and the Status footer link with one header tab that opens a random eligible hidden/legacy feature per click.

**Architecture:** A pure-frontend registry (`utils/secretFeatures.ts`) lists pool entries with click-time eligibility closures; `Navbar.vue` renders a «Секретная фича» button (desktop + mobile drawer) that `router.push`es the roll. Downloads stays a normal nav link only in the installed PWA; the showcase editor is deep-linked via `/profile?showcase=edit`. No backend, no new routes.

**Tech Stack:** Vue 3 `<script setup>`, vue-router 4, Pinia, Vitest + @vue/test-utils, lucide-vue-next, vue-i18n (en/ru/ja parity gate).

**Spec:** `docs/superpowers/specs/2026-07-06-secret-feature-tab-design.md`

## Global Constraints

- Worktree: all edits in `/data/worktrees/secret-feature` (never the base tree `/data/animeenigma`).
- Frontend commands run with `bun` / `bunx` from `frontend/web/` **inside the worktree**.
- i18n: every new key lands in ALL of `en.json`, `ru.json`, `ja.json` (parity gates redeploy).
- Design system: no off-palette Tailwind colors, no `font-bold`, only existing token classes (`nav-link-nt`, `text-brand-cyan`, `text-white/70`-style utilities already in these files). A PostToolUse hook DS-lints every edit.
- lucide icons: NAMED imports only (`import { Sparkles } from 'lucide-vue-next'`).
- Commits: pathspec form (`git commit -m "…" -- <paths>`), each with the three standing co-author trailers:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>`, `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>`, `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`.
- Routes `/anidle`, `/status`, `/downloads` stay registered and directly reachable — only nav placement changes.
- No `gofmt`/backend involvement; do not touch `nav.anidle`/`nav.downloads` locale keys (downloads is still used; anidle is left as harmless dead key to avoid three-locale churn).

---

### Task 1: `secretFeatures.ts` registry + roll logic

**Files:**
- Create: `frontend/web/src/utils/secretFeatures.ts`
- Test: `frontend/web/src/utils/__tests__/secretFeatures.spec.ts`

**Interfaces:**
- Consumes: `offlineDownloadsEnabled` (`@/offline/flag`), `useStandaloneDisplay()` (`@/pwa/standalone`), `useProfileWallVisible()` (`@/utils/profileWallGate`), `useAuthStore()` (`@/stores/auth`).
- Produces: `interface SecretFeature { key: 'anidle'|'status'|'downloads'|'showcase-editor'; to: RouteLocationRaw; path: string; eligible: () => boolean }`; `SECRET_FEATURES: SecretFeature[]`; `pickSecretFeature(currentPath: string): SecretFeature`; `_resetSecretFeatureForTests(): void`. Task 2 calls `router.push(pickSecretFeature(path).to)`.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/utils/__tests__/secretFeatures.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Controllable gate state — the registry reads these lazily at pick time.
const h = vi.hoisted(() => ({
  standalone: { value: false },
  isAuthenticated: false,
  wallVisible: false,
}))

vi.mock('@/offline/flag', () => ({ offlineDownloadsEnabled: true }))
vi.mock('@/pwa/standalone', () => ({ useStandaloneDisplay: () => h.standalone }))
vi.mock('@/utils/profileWallGate', () => ({
  useProfileWallVisible: () => ({
    get value() {
      return h.wallVisible
    },
  }),
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return h.isAuthenticated
    },
  }),
}))

import { pickSecretFeature, _resetSecretFeatureForTests } from '../secretFeatures'

function rollKeys(n: number, currentPath = '/'): string[] {
  return Array.from({ length: n }, () => pickSecretFeature(currentPath).key)
}

beforeEach(() => {
  _resetSecretFeatureForTests()
  h.standalone.value = false
  h.isAuthenticated = false
  h.wallVisible = false
})

describe('pickSecretFeature', () => {
  it('anonymous browser view → pool is exactly anidle+status+downloads', () => {
    const keys = new Set(rollKeys(200))
    expect(keys).toEqual(new Set(['anidle', 'status', 'downloads']))
  })

  it('installed PWA → downloads leaves the pool (it kept its nav link)', () => {
    h.standalone.value = true
    const keys = new Set(rollKeys(200))
    expect(keys).toEqual(new Set(['anidle', 'status']))
  })

  it('authed + wall gate open → showcase editor joins with the deep-link target', () => {
    h.isAuthenticated = true
    h.wallVisible = true
    const picks = Array.from({ length: 300 }, () => pickSecretFeature('/'))
    const editor = picks.find((p) => p.key === 'showcase-editor')
    expect(editor).toBeDefined()
    expect(editor!.to).toEqual({ path: '/profile', query: { showcase: 'edit' } })
  })

  it('authed but wall gate closed → no showcase editor', () => {
    h.isAuthenticated = true
    const keys = new Set(rollKeys(200))
    expect(keys.has('showcase-editor')).toBe(false)
  })

  it('never repeats the previous pick while alternatives exist', () => {
    const keys = rollKeys(100)
    for (let i = 1; i < keys.length; i++) {
      expect(keys[i]).not.toBe(keys[i - 1])
    }
  })

  it('never rolls the page the user is already on', () => {
    const keys = rollKeys(200, '/anidle')
    expect(keys.includes('anidle')).toBe(false)
  })

  it('degrades gracefully when exclusions empty the alternatives', () => {
    // Standalone + on /status: pool {anidle,status} minus current page = {anidle};
    // the no-repeat filter must not empty it — anidle repeats.
    h.standalone.value = true
    const keys = new Set(rollKeys(50, '/status'))
    expect(keys).toEqual(new Set(['anidle']))
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/utils/__tests__/secretFeatures.spec.ts`
Expected: FAIL — `Cannot find module '../secretFeatures'` (or equivalent resolve error).

- [ ] **Step 3: Write the implementation**

Create `frontend/web/src/utils/secretFeatures.ts`:

```ts
import type { RouteLocationRaw } from 'vue-router'
import { offlineDownloadsEnabled } from '@/offline/flag'
import { useStandaloneDisplay } from '@/pwa/standalone'
import { useProfileWallVisible } from '@/utils/profileWallGate'
import { useAuthStore } from '@/stores/auth'

/**
 * «Секретная фича» pool — hidden/legacy features that left the regular nav
 * (feedback 2026-07-04T07-37-57_tNeymik_manual). The Navbar tab opens a
 * random eligible entry; the routes themselves stay directly reachable.
 */
export interface SecretFeature {
  key: 'anidle' | 'status' | 'downloads' | 'showcase-editor'
  /** Navigation target for router.push. */
  to: RouteLocationRaw
  /** Plain path used to avoid re-rolling the page the user is already on. */
  path: string
  /** Evaluated at click time — Pinia stores and gates are live by then. */
  eligible: () => boolean
}

export const SECRET_FEATURES: SecretFeature[] = [
  { key: 'anidle', to: '/anidle', path: '/anidle', eligible: () => true },
  { key: 'status', to: '/status', path: '/status', eligible: () => true },
  {
    // In the installed PWA downloads keep their normal nav link; only the
    // browser view treats them as a secret.
    key: 'downloads',
    to: '/downloads',
    path: '/downloads',
    eligible: () => offlineDownloadsEnabled && !useStandaloneDisplay().value,
  },
  {
    // /profile redirects to /user/:publicId preserving the query;
    // Profile.vue opens the owner's showcase editor on ?showcase=edit.
    key: 'showcase-editor',
    to: { path: '/profile', query: { showcase: 'edit' } },
    path: '/profile',
    eligible: () => useAuthStore().isAuthenticated && useProfileWallVisible().value,
  },
]

let lastKey: SecretFeature['key'] | null = null

/**
 * Uniform random pick over eligible entries, skipping the current page and
 * the previous pick while alternatives remain. Never empty: anidle and
 * status are unconditional.
 */
export function pickSecretFeature(currentPath: string): SecretFeature {
  let pool = SECRET_FEATURES.filter((f) => f.eligible())
  const away = pool.filter((f) => f.path !== currentPath)
  if (away.length > 0) pool = away
  const fresh = pool.filter((f) => f.key !== lastKey)
  if (fresh.length > 0) pool = fresh
  const pick = pool[Math.floor(Math.random() * pool.length)]
  lastKey = pick.key
  return pick
}

export function _resetSecretFeatureForTests(): void {
  lastKey = null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/utils/__tests__/secretFeatures.spec.ts`
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/worktrees/secret-feature
git add frontend/web/src/utils/secretFeatures.ts frontend/web/src/utils/__tests__/secretFeatures.spec.ts
git commit -m "feat(web): secret-feature registry — random hidden/legacy feature roll

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- frontend/web/src/utils/secretFeatures.ts frontend/web/src/utils/__tests__/secretFeatures.spec.ts
```

---

### Task 2: Navbar tab + nav removals + footer status removal + i18n

**Files:**
- Modify: `frontend/web/src/components/layout/Navbar.vue` (imports ~line 397–410; navLinks ~line 459–468; desktop nav after the gacha link ~line 66–75; drawer after the gacha mobile link ~line 265–277)
- Modify: `frontend/web/src/App.vue:112-115` (footer status link)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (nav block, after `"downloads"`)

**Interfaces:**
- Consumes: `pickSecretFeature(currentPath: string): SecretFeature` from Task 1 (`.to` is the push target); `useStandaloneDisplay(): Ref<boolean>`.
- Produces: `nav.secretFeature` i18n key (used verbatim here only).

No unit spec: Navbar has no existing spec and a mount would need ~12 module mocks for a template-level change — covered by vue-tsc, DS-lint hook, `bun run build`, and the locale-parity gates. Logic risk lives in Task 1's registry, which is fully tested.

- [ ] **Step 1: Add i18n keys (all three locales)**

In `frontend/web/src/locales/en.json` nav block, after `"downloads": "Downloads",`:

```json
    "secretFeature": "Secret feature",
```

In `ru.json` after `"downloads": "Загрузки",`:

```json
    "secretFeature": "Секретная фича",
```

In `ja.json` after `"downloads": "ダウンロード",`:

```json
    "secretFeature": "シークレット機能",
```

- [ ] **Step 2: Navbar script changes**

In `frontend/web/src/components/layout/Navbar.vue`:

a. Extend the lucide import (line ~397):

```ts
import { Search, X, ChevronDown, Menu, Gem, Star, Bell, Sparkles } from 'lucide-vue-next'
```

b. Add two imports next to the existing `offlineDownloadsEnabled` import (line ~409):

```ts
import { offlineDownloadsEnabled } from '@/offline/flag'
import { useStandaloneDisplay } from '@/pwa/standalone'
import { pickSecretFeature } from '@/utils/secretFeatures'
```

c. Replace the whole navLinks block (comment + array + filter, lines ~459–468):

```ts
// Hidden/legacy features (anidle, browser-view downloads, status) left the
// nav for the «Secret feature» roulette — utils/secretFeatures.ts. Downloads
// keeps its link only in the installed PWA, where offline playback lives;
// standalone-ness is reactive, hence computed.
const isStandalone = useStandaloneDisplay()
const navLinks = computed(() => [
  { to: '/', label: 'nav.home' },
  { to: '/browse', label: 'nav.catalog' },
  { to: '/schedule', label: 'nav.schedule' },
  ...(offlineDownloadsEnabled && isStandalone.value
    ? [{ to: '/downloads', label: 'nav.downloads' }]
    : []),
])

function openSecretFeature(): void {
  mobileMenuOpen.value = false
  void router.push(pickSecretFeature(router.currentRoute.value.path).to)
}
```

- [ ] **Step 3: Navbar template — desktop button**

Immediately after the gacha desktop `</router-link>` (inside the `hidden md:flex` div, ~line 75), add:

```html
          <!-- Secret feature roulette — random hidden/legacy feature per click -->
          <button
            type="button"
            class="nav-link-nt inline-flex items-center gap-1.5"
            @click="openSecretFeature"
          >
            <Sparkles class="size-4" aria-hidden="true" />
            {{ $t('nav.secretFeature') }}
          </button>
```

- [ ] **Step 4: Navbar template — mobile drawer row**

Immediately after the gacha mobile `</router-link>` block in the drawer, add:

```html
            <button
              type="button"
              class="flex items-center gap-2 px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors text-left"
              @click="openSecretFeature"
            >
              <Sparkles class="size-4 text-brand-cyan" aria-hidden="true" />
              {{ $t('nav.secretFeature') }}
            </button>
```

(`openSecretFeature` already closes the drawer via `mobileMenuOpen.value = false`.)

- [ ] **Step 5: Remove the footer status link**

In `frontend/web/src/App.vue` (~lines 112–115) delete the bullet + link pair:

```html
        <span class="text-brand-cyan/30 text-sm select-none" aria-hidden="true">&bull;</span>
        <router-link to="/status" class="text-white/60 hover:text-white/80 text-sm transition-colors">
          {{ $t('status.title') }}
        </router-link>
```

(The preceding build-hash block and the following FeedbackButton bullet stay untouched.)

- [ ] **Step 6: Verify types, lint, locales**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx tsc --noEmit && bunx vitest run src/locales/__tests__`
Expected: tsc clean; locale parity specs PASS.

- [ ] **Step 7: Commit**

```bash
cd /data/worktrees/secret-feature
git add frontend/web/src/components/layout/Navbar.vue frontend/web/src/App.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(web): «Секретная фича» nav tab; anidle/downloads/status leave chrome

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- frontend/web/src/components/layout/Navbar.vue frontend/web/src/App.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
```

---

### Task 3: MobileTabBar — downloads tab only in installed PWA

**Files:**
- Modify: `frontend/web/src/components/layout/MobileTabBar.vue:56`
- Test: `frontend/web/src/components/layout/MobileTabBar.spec.ts`

**Interfaces:**
- Consumes: the component's existing `isStandalone = useStandaloneDisplay()` ref and `offlineDownloadsEnabled`.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

In `MobileTabBar.spec.ts`, after the existing active-tab test (`'/downloads'` test ~line 103), add (the harness's `standalone()` helper + `h.refs` already exist; standalone defaults to `true`):

```ts
  it('hides the downloads tab in browser view (not installed PWA)', async () => {
    standalone().value = false
    const w = mountBar()
    await nextTick()
    expect(w.find('[data-test="tab-downloads"]').exists()).toBe(false)
    // other tabs unaffected
    expect(w.find('[data-test="tab-home"]').exists()).toBe(true)
  })
```

Add `nextTick` to the spec's top-level vue import (`import type { Ref } from 'vue'` becomes `import { nextTick } from 'vue'` + keep the type import, or one combined `import { nextTick, type Ref } from 'vue'`). `vue` itself is not vi.mocked in this file, so a top-level value import is safe.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/components/layout/MobileTabBar.spec.ts`
Expected: the new test FAILS (`tab-downloads` still renders); all pre-existing tests PASS. Note: if `standalone().value = false` leaks into later tests, reset it in the existing `beforeEach`.

- [ ] **Step 3: Implement the gate**

In `MobileTabBar.vue` line 56, change:

```ts
  ...(offlineDownloadsEnabled ? [{ key: 'downloads', to: '/downloads', icon: Download, label: 'nav.downloads' }] : []),
```

to:

```ts
  // Browser view hides downloads (secret-feature pool covers it); the
  // installed PWA keeps the tab — offline playback lives there.
  ...(offlineDownloadsEnabled && isStandalone.value
    ? [{ key: 'downloads', to: '/downloads', icon: Download, label: 'nav.downloads' }]
    : []),
```

- [ ] **Step 4: Run the spec file to verify all pass**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/components/layout/MobileTabBar.spec.ts`
Expected: PASS including all pre-existing tests (harness default `standalone = true` keeps them green).

- [ ] **Step 5: Commit**

```bash
cd /data/worktrees/secret-feature
git add frontend/web/src/components/layout/MobileTabBar.vue frontend/web/src/components/layout/MobileTabBar.spec.ts
git commit -m "feat(web): mobile tab bar shows downloads only in installed PWA

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- frontend/web/src/components/layout/MobileTabBar.vue frontend/web/src/components/layout/MobileTabBar.spec.ts
```

---

### Task 4: Showcase-editor deep link — `?showcase=edit`

**Files:**
- Modify: `frontend/web/src/router/index.ts:99-107` (own-profile redirect drops the query today)
- Modify: `frontend/web/src/views/Profile.vue` (add a watcher right after `openShowcaseEditor()`, ~line 1053)
- Test: `frontend/web/src/views/__tests__/Profile.showcase.spec.ts`

**Interfaces:**
- Consumes: existing `openShowcaseEditor()`, `isOwnProfile`, `profileWallVisible`, `route`, `router` locals in Profile.vue; Task 1 emits targets of shape `{ path: '/profile', query: { showcase: 'edit' } }`.
- Produces: `/user/:publicId?showcase=edit` behavior relied on by the secret-feature roll.

- [ ] **Step 1: Write the failing tests**

In `Profile.showcase.spec.ts`, the mount helper hardcodes the path. Add a query-capable variant next to `mountProfile` (~line 155):

```ts
async function mountProfileWithQuery(publicIdParam = 'testuser', query = '') {
  await router.push(`/profile/${publicIdParam}${query}`)
  await router.isReady()
  const w = mount(Profile, { global: { plugins: [i18n, router, createPinia()], stubs: globalStubs } })
  await flushPromises()
  return w
}
```

Then add tests at the end of the describe block (owner setup mirrors the existing `'owner + none → "Add Showcase" button'` test):

```ts
  it('?showcase=edit + owner → editor force-opened, marker query stripped', async () => {
    authUser = { public_id: 'testuser' }
    publicProfile = { showcase_state: 'none' }
    const w = await mountProfileWithQuery('testuser', '?showcase=edit')
    // force-edit reveals the showcase tab exactly like clicking "Add Showcase"
    expect(w.find('[data-tab="showcase"]').exists()).toBe(true)
    expect(router.currentRoute.value.query.showcase).toBeUndefined()
  })

  it('?showcase=edit + gate closed → no editor, marker query still stripped', async () => {
    gateOpen = false
    authUser = { public_id: 'testuser' }
    publicProfile = { showcase_state: 'none' }
    const w = await mountProfileWithQuery('testuser', '?showcase=edit')
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(router.currentRoute.value.query.showcase).toBeUndefined()
  })

  it('?showcase=edit + visitor → ignored (no editor, query untouched for non-owner)', async () => {
    authUser = { public_id: 'someone-else' }
    publicProfile = { showcase_state: 'none' }
    const w = await mountProfileWithQuery('testuser', '?showcase=edit')
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
  })
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/views/__tests__/Profile.showcase.spec.ts`
Expected: the two owner/gate tests FAIL (tab absent / query survives); visitor test may already pass; all pre-existing tests PASS.

- [ ] **Step 3: Implement — Profile.vue watcher**

In `views/Profile.vue`, directly after the `openShowcaseEditor()` function body (~line 1053), add:

```ts
// Secret-feature deep link: /profile?showcase=edit (the own-profile redirect
// preserves the query) opens the owner's showcase editor once, then strips
// the marker so refresh/back don't re-trigger it.
watch(
  () => route.query.showcase === 'edit' && !!isOwnProfile.value,
  (hit) => {
    if (!hit) return
    if (profileWallVisible.value) openShowcaseEditor()
    const { showcase: _showcase, ...rest } = route.query
    void router.replace({ query: rest })
  },
  { immediate: true },
)
```

(`watch`, `route`, `router`, `isOwnProfile`, `profileWallVisible` are all already in scope.)

- [ ] **Step 4: Implement — router redirect keeps the query**

In `router/index.ts` lines 99–107, replace the own-profile `beforeEnter`:

```ts
    beforeEnter: (to, _from, next) => {
      const authStore = useAuthStore()
      if (authStore.user?.public_id) {
        // Keep the query (e.g. ?showcase=edit secret-feature deep link).
        next({ path: `/user/${authStore.user.public_id}`, query: to.query })
      } else {
        next()
      }
    }
```

(Rename `_to` → `to` in the parameter list.)

- [ ] **Step 5: Run the spec file to verify all pass**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run src/views/__tests__/Profile.showcase.spec.ts`
Expected: PASS (all pre-existing + 3 new).

- [ ] **Step 6: Commit**

```bash
cd /data/worktrees/secret-feature
git add frontend/web/src/views/Profile.vue frontend/web/src/router/index.ts frontend/web/src/views/__tests__/Profile.showcase.spec.ts
git commit -m "feat(web): ?showcase=edit deep link opens the owner showcase editor

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- frontend/web/src/views/Profile.vue frontend/web/src/router/index.ts frontend/web/src/views/__tests__/Profile.showcase.spec.ts
```

---

### Task 5: Full verification + ship

**Files:** none new — gates only.

- [ ] **Step 1: Full unit suite + typecheck**

Run: `cd /data/worktrees/secret-feature/frontend/web && bunx vitest run && bunx tsc --noEmit`
Expected: all specs PASS, tsc clean. (vue-tsc false-pass caveat is why the real build in /frontend-verify follows.)

- [ ] **Step 2: /frontend-verify**

Invoke the `frontend-verify` skill (DS-lint, i18n en/ru/ja parity, real `bun run build`, lucide/TS2614 traps) from the worktree. Expected: all gates green; no Chrome smoke (small change, opt-in policy).

- [ ] **Step 3: Ship**

Follow the git workflow: pull-rebase-push worktree commits to `main`, then invoke `animeenigma-after-update` (redeploy `web`, changelog entry in Russian Trump-mode, health checks, push). Then set the feedback report to ai_done:

```bash
cd /data/animeenigma && bin/feedback-status 2026-07-04T07-37-57_tNeymik_manual ai_done claude-code
```

Expected: `make redeploy-web` green, health OK, changelog prepended, report status `ai_done`.
