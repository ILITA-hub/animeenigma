# RBAC and Roulette — Phase 4: Feature-Visibility Cutover — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `/api/policy/features/mine` authoritative on the frontend — nav, dark-ship routes, profile-wall tab, and the footer roulette pool all read the per-user feed instead of `VITE_*_ADMIN_ONLY` env flags and client-side `SECRET_FEATURES` eligibility.

**Architecture:** A boot-loaded feature-visibility store → a `useFeatureVisible(key)` composable → the three gate utils delegate to it, the router guard consults it, and the footer roulette reads `mine.roulette`. Vue 3 / Pinia / Neon-Tokyo.

**Tech Stack:** Vue 3 `<script setup>`, Pinia, Vitest, vue-i18n; the shared `apiClient`.

## Global Constraints

- **Spec of record:** `docs/superpowers/specs/2026-07-07-rbac-roulette-p4-feature-visible-cutover-design.md`.
- **Day-one parity (acceptance bar):** on a freshly-seeded backend, a normal user and an admin must see EXACTLY what they see today — nav items, /fanfics & /gacha access, profile showcase tab, footer roulette pool contents — unchanged. The P1 seed reproduces current behavior (fanfic/gacha/profile-wall→admin; anidle/status/themes/game/downloads/showcase-editor/my-feedback→everyone+roulette; gacha roulette OFF; master ON).
- **Fail-open (D1):** `useFeatureVisible(key)` = feed-loaded ? `mine.visible.has(key)` : per-key failSafe fallback — dark-ship keys `fanfic`/`gacha`/`profile-wall` → `authStore.isAdmin`; all other keys → `true`. Remove the `VITE_*_ADMIN_ONLY` env reads entirely.
- **`/features/mine` shape:** `{ rouletteEnabled: boolean, visible: string[], roulette: string[] }` (envelope `{success,data}`). Anonymous callers get everyone-flags only. Fail-open server-side.
- **Security note:** FE visibility is cosmetic — the gateway `FeatureGate` (P2) is the real boundary. Do not treat FE gating as security.
- **DS:** primitives + semantic tokens; lucide named imports; DS-lint 0; `font-medium`/`font-semibold` only.
- **i18n:** any new string in en/ru/ja (parity gate).
- **Commit co-authors** on every commit: Claude Code / 0neymik0 / NANDIorg.
- Worktree only; never edit the base tree. Do NOT push/deploy (owner-gated).

## File Structure

- `frontend/web/src/stores/featureVisibility.ts` — Pinia store: `load()`, `visible: Set<string>`, `roulette: string[]`, `rouletteEnabled: boolean`, `loaded: boolean`, `ready: Promise<void>`.
- `frontend/web/src/composables/useFeatureVisible.ts` — `useFeatureVisible(key): ComputedRef<boolean>` (D1).
- `frontend/web/src/api/client.ts` — add `getFeaturesMine()`; later remove orphaned secret-features methods.
- `frontend/web/src/utils/{fanfic,gacha,profileWall}Gate.ts` — delegate to `useFeatureVisible`; drop `*_ADMIN_ONLY` const exports.
- `frontend/web/src/router/index.ts` — guard consults the store; drop the `GACHA_ADMIN_ONLY`/`FANFIC_ADMIN_ONLY` imports.
- `frontend/web/src/utils/secretFeatures.ts` — footer pool from `mine.roulette` + `rouletteEnabled`.
- `frontend/web/src/App.vue` — boot `featureVisibility.load()` instead of `secretFeaturesApi.getState()`.
- `frontend/web/src/views/admin/AdminPolicy.vue` — relabel Providers tab placeholder ("P5").

---

## Task 1: Feature-visibility store + `useFeatureVisible` + api method

**Files:**
- Create: `frontend/web/src/stores/featureVisibility.ts`, `frontend/web/src/composables/useFeatureVisible.ts`
- Modify: `frontend/web/src/api/client.ts` (add `getFeaturesMine`)
- Test: `frontend/web/src/composables/useFeatureVisible.spec.ts`, `frontend/web/src/stores/featureVisibility.spec.ts`

**Interfaces:**
- Produces:
  - `useFeatureVisibilityStore()` (Pinia) with state `{ visible: Set<string>, roulette: string[], rouletteEnabled: boolean, loaded: boolean }`, action `load(): Promise<void>` (GET `/api/policy/features/mine`, unwrap `{success,data}`, populate state, resolve `ready`; on error leave `loaded=false` and resolve `ready` so awaiters proceed to fallback), and a `ready: Promise<void>`.
  - `useFeatureVisible(key: string): ComputedRef<boolean>` — D1 fallback via a `const DARKSHIP_FALLBACK_ADMIN = new Set(['fanfic','gacha','profile-wall'])`.
  - `apiClient` `adminApi`-sibling `getFeaturesMine()` returning `{rouletteEnabled, visible, roulette}` (reuse the envelope-unwrap convention).

- [ ] **Step 1: Write failing tests**

`useFeatureVisible.spec.ts`:
- feed loaded, `visible` has `fanfic` → `useFeatureVisible('fanfic').value === true`; not in `visible` → false (for a non-darkship public key too).
- feed NOT loaded: `fanfic`/`gacha`/`profile-wall` → equals `authStore.isAdmin` (test both admin and non-admin via a mocked auth store); a public key (e.g. `anidle`) → true.
`featureVisibility.spec.ts`:
- `load()` GETs `/api/policy/features/mine`, populates `visible`(Set)/`roulette`/`rouletteEnabled`, sets `loaded=true`; on a rejected request leaves `loaded=false` and still resolves `ready`.

(Mock the api client + auth store the way sibling specs do — grep an existing store/composable spec.)

- [ ] **Step 2: Run, verify fail**

Run: `cd frontend/web && bunx vitest run src/composables/useFeatureVisible.spec.ts src/stores/featureVisibility.spec.ts`
Expected: FAIL (missing modules).

- [ ] **Step 3: Implement**

- `api/client.ts`: `getFeaturesMine: () => apiClient.get<{data:{rouletteEnabled:boolean;visible:string[];roulette:string[]}}>('/policy/features/mine').then(unwrap)` — match the existing method style.
- `stores/featureVisibility.ts`: Pinia store per the interface; `load()` guards against double-fetch; `ready` is a promise resolved after the first `load()` settles (success OR failure).
- `composables/useFeatureVisible.ts`: `computed(() => store.loaded ? store.visible.has(key) : (DARKSHIP_FALLBACK_ADMIN.has(key) ? authStore.isAdmin : true))`.

- [ ] **Step 4: Run tests + tsc + DS-lint**

Run: `cd frontend/web && bunx vitest run src/composables/useFeatureVisible.spec.ts src/stores/featureVisibility.spec.ts && bunx tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: PASS, DS-lint 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/stores/featureVisibility.ts frontend/web/src/composables/useFeatureVisible.ts frontend/web/src/composables/useFeatureVisible.spec.ts frontend/web/src/stores/featureVisibility.spec.ts frontend/web/src/api/client.ts
git commit -m "feat(web): feature-visibility store + useFeatureVisible (reads /features/mine)"
```

---

## Task 2: Gate-util cutover + router guard

**Files:**
- Modify: `frontend/web/src/utils/fanficGate.ts`, `gachaGate.ts`, `profileWallGate.ts`
- Modify: `frontend/web/src/router/index.ts`
- Test: extend/adjust the gate specs if present; a router-guard spec.

**Interfaces:**
- Consumes: `useFeatureVisible` (Task 1).
- Produces: `useFanficVisible`/`useGachaVisible`/`useProfileWallVisible` now delegate to `useFeatureVisible('fanfic'|'gacha'|'profile-wall')`; the `*_ADMIN_ONLY` const exports are removed; the router `beforeEach` gates `meta.gachaGated`/`meta.fanficGated` (and the profile-wall tab, wherever gated) via the store instead of the consts.

- [ ] **Step 1: Write/adjust failing tests**

- Gate specs: `useFanficVisible()` returns what `useFeatureVisible('fanfic')` returns (feed-loaded true/false; fallback admin). Mirror for gacha/profile-wall.
- Router guard: navigating to `/gacha` (meta.gachaGated) when the store says gacha NOT visible → redirected (to home or 404, matching current behavior — READ the current guard to preserve its redirect target); when visible → allowed. The guard must `await featureVisibility.ready` before deciding.

- [ ] **Step 2: Run, verify fail**

Run: `cd frontend/web && bunx vitest run src/utils/ src/router` (adjust paths to the actual specs)
Expected: FAIL.

- [ ] **Step 3: Implement**

- Rewrite each gate util: keep the `use*Visible()` export, body = `return useFeatureVisible('<key>')`. DELETE the `export const *_ADMIN_ONLY` lines and the `import.meta.env.VITE_*` reads.
- `router/index.ts`: remove the `import { GACHA_ADMIN_ONLY } ...` / `FANFIC_ADMIN_ONLY` imports. Find the `beforeEach` (or per-route `beforeEnter`) that currently reads those consts for `meta.gachaGated`/`meta.fanficGated` (grep `gachaGated`/`fanficGated`). Rewrite it to: `await useFeatureVisibilityStore().ready`, then check `useFeatureVisible('gacha'|'fanfic').value` (or read the store's `visible` set directly in the guard — a guard isn't a setup context, so use the store instance + the D1 fallback logic; extract the fallback into a shared helper `isFeatureVisible(key, store, authStore)` used by both the composable and the guard to avoid duplication). Preserve the exact current redirect behavior for a blocked route.

- [ ] **Step 4: Run tests + tsc + DS-lint**

Run: `cd frontend/web && bunx vitest run src/utils src/router && bunx tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: PASS, DS-lint 0. Grep to confirm NO remaining `VITE_FANFIC_ADMIN_ONLY|VITE_GACHA_ADMIN_ONLY|VITE_PROFILE_WALL_ADMIN_ONLY|_ADMIN_ONLY` references in `src/` (except comments/spec).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/fanficGate.ts frontend/web/src/utils/gachaGate.ts frontend/web/src/utils/profileWallGate.ts frontend/web/src/router/index.ts frontend/web/src/utils frontend/web/src/router
git commit -m "feat(web): dark-ship gates + router guard read policy feed (drop VITE_*_ADMIN_ONLY)"
```

---

## Task 3: Footer roulette cutover

**Files:**
- Modify: `frontend/web/src/utils/secretFeatures.ts`, `frontend/web/src/App.vue`
- Test: `frontend/web/src/utils/secretFeatures.spec.ts` (adjust)

**Interfaces:**
- Consumes: the feature-visibility store (Task 1).
- Produces: `pickSecretFeature(currentPath)` picks from the roster filtered by `store.roulette` (server-resolved membership); `isRouletteEnabled()` returns `store.rouletteEnabled`; `applySecretFeatureAdminState` + client `eligible()` pool logic removed. App.vue boots `featureVisibility.load()` instead of `secretFeaturesApi.getState()`.

- [ ] **Step 1: Write/adjust failing tests**

`secretFeatures.spec.ts`:
- with the store's `roulette = ['anidle','themes']` and `rouletteEnabled=true`, `pickSecretFeature('/x')` only ever returns entries whose `key ∈ {anidle,themes}`; excludes the current path + last pick as before.
- `isRouletteEnabled()` reflects `store.rouletteEnabled` (false → footer button hidden path).
(Use the store with a seeded state; reset between tests.)

- [ ] **Step 2: Run, verify fail**

Run: `cd frontend/web && bunx vitest run src/utils/secretFeatures.spec.ts`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `secretFeatures.ts`: keep `SECRET_FEATURES[]` as the key→`to`/`labelKey` registry (drop the `eligible` field, or ignore it). `pickSecretFeature`: `pool = SECRET_FEATURES.filter(f => store.roulette.includes(f.key))` then the existing current-path/lastKey exclusion. `isRouletteEnabled()` → `store.rouletteEnabled`. Remove `applySecretFeatureAdminState`, `adminDisabled`, `rouletteMasterEnabled` module state, and the per-key `eligible()` client logic. Keep `_resetSecretFeatureForTests` (reset `lastKey`). Read the store via `useFeatureVisibilityStore()` inside the functions (they run at click time / boot, after Pinia is active).
- `App.vue`: replace the `secretFeaturesApi.getState()` + `applySecretFeatureAdminState(...)` block (the `onMounted` fetch ~line 227-236) with `await useFeatureVisibilityStore().load()`. Remove now-unused imports (`applySecretFeatureAdminState`, `secretFeaturesApi` if unused elsewhere).

- [ ] **Step 4: Run tests + tsc + DS-lint**

Run: `cd frontend/web && bunx vitest run src/utils/secretFeatures.spec.ts && bunx tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: PASS, DS-lint 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/secretFeatures.ts frontend/web/src/App.vue frontend/web/src/utils/secretFeatures.spec.ts
git commit -m "feat(web): footer roulette pool from policy /features/mine (retire secret-features state)"
```

---

## Task 4: Cleanup + Providers relabel + full parity gate

**Files:**
- Modify: `frontend/web/src/api/client.ts` (remove orphaned secret-features methods), `frontend/web/src/views/admin/AdminPolicy.vue` (Providers placeholder copy), locales if the placeholder copy changes.
- Test: a day-one parity spec.

**Interfaces:**
- Consumes: everything above.
- Produces: no remaining references to `secretFeaturesApi`/`/api/secret-features/*` or the orphaned `adminApi` secret-features methods (P3 left them for P4); Providers tab says "P5".

- [ ] **Step 1: Write the parity test**

`frontend/web/src/composables/featureVisibility.parity.spec.ts`: seed the store with the day-one-equivalent `mine` (matching the CORRECTED seed in `services/policy/internal/domain/feature_flag.go` after the Task-3 parity fix) for three identities and assert visibility matches today's:
- **admin** `mine = { rouletteEnabled:true, visible:[all 10 keys], roulette:[anidle,status,themes,game,downloads,fanfic,showcase-editor,my-feedback] (8) }` → `useFeatureVisible('fanfic'|'gacha'|'profile-wall')` all true; footer pool = those 8. (gacha NOT in roulette — seed roulette:false.)
- **user** `mine = { rouletteEnabled:true, visible:[anidle,status,themes,game,downloads,my-feedback], roulette:[same 6] }` → fanfic/gacha/profile-wall false; footer pool = those 6. (showcase-editor + fanfic are admin-only, absent.)
- **anon** `mine = { rouletteEnabled:true, visible:[anidle,status,themes,game,downloads], roulette:[same 5] }` → fanfic/gacha/profile-wall false; footer pool = those 5. (my-feedback + showcase-editor + fanfic absent — the parity fix.)

- [ ] **Step 2: Run, verify fail (or pass if logic already correct)**

Run: `cd frontend/web && bunx vitest run src/composables/featureVisibility.parity.spec.ts`
If it passes immediately (logic from T1–T3 already correct), that's fine — it's a regression lock. If it fails, fix the underlying logic (not the test).

- [ ] **Step 3: Cleanup**

- `api/client.ts`: grep for `secretFeaturesApi` and the orphaned `adminApi` secret-features methods (`getSecretFeatures*`/`setSecret*`/roulette/feature) — remove them AND their now-unused types, but ONLY after grepping `src/` to confirm nothing still imports them (App.vue + secretFeatures.ts were the users; both cut over in T3). If anything else still references them, STOP and report.
- `AdminPolicy.vue`: change the Providers tab placeholder copy from "coming in P4" to "coming in P5" (or a neutral "coming soon"); update the i18n value in en/ru/ja if it names P4.
- Grep `src/` for any dangling `VITE_*_ADMIN_ONLY`, `applySecretFeatureAdminState`, `/api/secret-features` references — none should remain in code.

- [ ] **Step 4: Full FE gate**

Run:
```
cd frontend/web
bunx vitest run
bash scripts/design-system-lint.sh
bunx tsc --noEmit
bun run build
```
Expected: full suite PASS (baseline: the pre-existing unrelated `SubtitleSettingsMenu.spec.ts` failure may persist — confirm it's unchanged vs origin/main, everything else green), DS-lint 0, tsc clean, build 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/api/client.ts frontend/web/src/views/admin/AdminPolicy.vue frontend/web/src/locales frontend/web/src/composables/featureVisibility.parity.spec.ts
git commit -m "chore(web): remove orphaned secret-features client; Providers tab → P5; day-one parity test"
```

---

## Self-Review notes (plan author)

- Coverage: store/composable → T1; gates + router → T2; footer + App boot → T3; cleanup + parity + gate → T4. Spec D1/D2/D3 all covered (D1 fallback in T1, parity in T4, feed-timing/guard-await in T1/T2).
- Shared fallback logic (`isFeatureVisible(key, store, authStore)`) is extracted in T2 so the composable and the router guard don't diverge — call it out to the reviewer.
- Day-one parity (D2) is the acceptance bar — T4's parity spec is the regression lock; a manual check on a seeded backend happens at deploy.
- Verify during execution: the exact router guard shape (`meta.gachaGated`/`fanficGated`), the profile-wall tab gate location in `Profile.vue`, and that `secretFeaturesApi`/orphaned methods have no other consumers before deletion.
- After Task 4: `/frontend-verify`, then the whole-branch review, then owner-gated integrate/deploy.
