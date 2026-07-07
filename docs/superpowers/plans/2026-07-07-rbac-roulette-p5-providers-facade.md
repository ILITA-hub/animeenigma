# RBAC and Roulette — Phase 5: Providers Facade + P4 Cleanups — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Fill the `/admin/policy` Providers tab (list providers + policy/health; admin flips auto↔disabled — a facade over catalog's `stream_providers`), plus the P4 dead-code cleanups.

**Architecture:** Catalog owns provider policy; add admin read/write endpoints there (facade), wire the FE tab, then sweep P4 dead code. Go (chi/GORM) + Vue 3.

## Global Constraints

- **Spec of record:** `docs/superpowers/specs/2026-07-07-rbac-roulette-p5-providers-facade-design.md`.
- **Facade only:** write to catalog's existing `ScraperProvider.Policy`; do NOT touch the self-heal engine (`service/providerpolicy/engine.go`), the probe pipeline, or `/capabilities` derivation. The DB-ownership move stays deferred.
- **Admin policy levers = `auto` | `disabled`** only. `disabled` is the hard lock (immune to self-heal); `auto` re-enters machine-managed rotation. Reject `manual` from the admin write (400).
- **`SetPolicy` sets `Policy` + `PolicySince = now`; leaves `Health`/`HealthSince` untouched** (health is probe-owned).
- **DS:** primitives (`Switch`/`SegmentedControl`/`Badge`/`ConfirmDialog`/`Card`/`EmptyState`); lucide named imports; semantic tokens (provider/brand hues allowed); DS-lint 0.
- **i18n** en/ru/ja parity. **Commit co-authors** (Claude Code / 0neymik0 / NANDIorg). Worktree only; do NOT push.
- **B1 backend deletion is grep-gated:** remove dead catalog secret-features ONLY after confirming zero live consumers; keep catalog build+tests green.

## File Structure

- `services/catalog/internal/handler/admin_scraper_providers.go` — new admin handler (List + SetPolicy).
- `services/catalog/internal/transport/router.go` — mount the two routes in the `/api/admin` group.
- `services/catalog/internal/domain/scraper_provider.go` — (read-only; reuse `DerivedState()`/`StateCode()`/wire fields).
- `frontend/web/src/composables/useAdminProviders.ts`, `frontend/web/src/api/client.ts` — FE composable + adminApi methods.
- `frontend/web/src/views/admin/AdminPolicy.vue` — fill the `#providers` tab.
- `frontend/web/src/locales/{en,ru,ja}.json` — `admin.policy.providers.*`.
- (B1) delete: catalog `secret_feature.{handler,service,repo}.go` + `SecretFeatureFlag` domain + routes; gateway `/secret-features/*` proxy.
- (B2/B3) `frontend/web/src/utils/secretFeatures.ts`, `Navbar.vue`, `Profile.vue`, `FanficsView.vue`, `api/fanfic.ts`, `router/index.ts`.

---

## Task 1: Catalog — admin scraper-providers endpoints

**Files:**
- Create: `services/catalog/internal/handler/admin_scraper_providers.go`
- Modify: `services/catalog/internal/transport/router.go` (mount routes; construct handler in `cmd/catalog-api/main.go`)
- Test: `services/catalog/internal/handler/admin_scraper_providers_test.go`

**Interfaces:**
- Consumes: the `ScraperProvider` GORM model + repo (read how `internal_scraper_providers.go` `List` queries + `toWire` maps; `DerivedState()`/`StateCode()` on the domain type).
- Produces: `GET /api/admin/scraper-providers` → `{providers:[wire]}` (wire includes a new `derived_state` string); `PUT /api/admin/scraper-providers/{name}/policy` body `{policy}` → updated wire.

- [ ] **Step 1: Write failing tests**

Handler tests (sqlite `:memory:` or the catalog test harness — read a sibling `*_handler_test.go` / `internal_scraper_providers_test.go` for the fixture style):
- `List` returns seeded providers each with `policy`/`health`/`derived_state` populated (assert `derived_state` matches `DerivedState()` for a known `(policy,health)` pair, e.g. auto+up → the UP code).
- `SetPolicy` name=X policy=disabled → 200, row's `Policy==disabled`, `PolicySince` advanced, `Health` UNCHANGED; policy=auto → 200 Policy==auto.
- `SetPolicy` unknown name → 404; policy="manual" → 400; policy="bogus" → 400.

- [ ] **Step 2: Run, verify fail**

Run: `cd services/catalog && go test ./internal/handler/ -run ScraperProviders -v`
Expected: FAIL (handler missing).

- [ ] **Step 3: Implement**

- `admin_scraper_providers.go`: `AdminScraperProvidersHandler{ db, log }`. `List` — reuse/duplicate the `internal_scraper_providers.go` query + wire mapping; extend the wire struct with `DerivedState string \`json:"derived_state"\`` set from `provider.DerivedState().StateCode()` (or whatever the exact accessor is — read the domain). `SetPolicy` — parse `{policy}`, validate ∈ {auto,disabled} (`domain.PolicyAuto`/`PolicyDisabled`) else `httputil.BadRequest`; `GetByName` (or `Where("name=?")`) → 404 (`errors.NotFound`) if absent; set `Policy` + `PolicySince=time.Now()`, save; return the updated wire. Follow `secret_feature.go`'s handler idiom.
- Router: in the catalog `/api/admin` group, `r.Get("/scraper-providers", h.List)` and `r.Put("/scraper-providers/{name}/policy", h.SetPolicy)`. Construct the handler in `main.go` with the shared `*gorm.DB` + logger.

- [ ] **Step 4: Run tests**

Run: `cd services/catalog && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog
git commit -m "feat(catalog): admin scraper-provider policy endpoints (list + set auto/disabled)"
```

---

## Task 2: FE — Providers tab

**Files:**
- Create: `frontend/web/src/composables/useAdminProviders.ts`
- Modify: `frontend/web/src/api/client.ts`, `frontend/web/src/views/admin/AdminPolicy.vue`, `frontend/web/src/locales/{en,ru,ja}.json`
- Test: `frontend/web/src/composables/useAdminProviders.spec.ts`, extend `AdminPolicy.spec.ts`

**Interfaces:**
- Consumes: `adminApi` (Task-1 endpoints), primitives (`Switch`/`SegmentedControl`/`Badge`/`ConfirmDialog`/`EmptyState`).
- Produces: functional Providers tab.

- [ ] **Step 1: Write failing tests**

- `useAdminProviders.spec.ts`: `list()` GETs `/admin/scraper-providers` → parsed providers; `setPolicy('gogoanime','disabled')` PUTs `/admin/scraper-providers/gogoanime/policy` `{policy:'disabled'}`. Mock the api client.
- `AdminPolicy.spec.ts` (extend): the Providers tab renders one row per provider with a status pill from `derived_state`; toggling a provider OFF opens the confirm dialog and, on confirm, calls `setPolicy(name,'disabled')`; toggling ON calls `setPolicy(name,'auto')` with no confirm.

- [ ] **Step 2: Run, verify fail**

Run: `cd frontend/web && bunx vitest run src/composables/useAdminProviders.spec.ts src/views/admin/AdminPolicy.spec.ts`
Expected: FAIL.

- [ ] **Step 3: Implement**

- `api/client.ts`: `listScraperProviders()` + `setScraperProviderPolicy(name, policy)` on `adminApi` (mirror `getPolicyFlags`/`setPolicyFlag`), typed `ScraperProviderWire { name, group, engine, policy, health, derived_state, reason, description, ... }`.
- `useAdminProviders.ts`: `{ list, setPolicy }` mirroring `useAdminPolicy.ts`.
- `AdminPolicy.vue` `#providers`: replace `EmptyState` with a loaded list. Each row: name/group/engine, a `Badge` status pill mapping `derived_state` → label+tone (UP→success, Recovering/Degraded→warning, Down→destructive, Disabled→muted), the `reason` if present, and a `Switch` (on = auto, off = disabled). Off-toggle → `ConfirmDialog` (disabling drops the provider from playback failover) → `setPolicy(name,'disabled')`; on-toggle → `setPolicy(name,'auto')`. Optimistic + toast; refresh row from the returned wire. Load on tab mount (or on view mount).
- i18n `admin.policy.providers.*` (tab title, column labels, pill states, enable/disable, confirm copy, toasts) in en/ru/ja. Repurpose the existing `providersPlaceholder.*` keys.

- [ ] **Step 4: Run tests + gates**

Run: `cd frontend/web && bunx vitest run src/composables/useAdminProviders.spec.ts src/views/admin/AdminPolicy.spec.ts src/locales && bash scripts/design-system-lint.sh && bunx tsc --noEmit`
Expected: PASS, DS-lint 0, parity green.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useAdminProviders.ts frontend/web/src/composables/useAdminProviders.spec.ts frontend/web/src/api/client.ts frontend/web/src/views/admin/AdminPolicy.vue frontend/web/src/views/admin/AdminPolicy.spec.ts frontend/web/src/locales
git commit -m "feat(admin-web): Providers tab — list + auto/disabled facade over catalog policy"
```

---

## Task 3: Backend cleanup — remove dead catalog secret-features + gateway proxy

**Files:**
- Delete: catalog `internal/handler/secret_feature.go`, `internal/service/.../secret_feature*.go`, `internal/repo/.../secret_feature*.go`, the `SecretFeatureFlag` domain type + its migration/seed, and their tests.
- Modify: `services/catalog/internal/transport/router.go` (drop `/api/admin/secret-features*` + `/api/secret-features/state` routes), `services/catalog/cmd/catalog-api/main.go` (drop handler construction), `services/gateway/internal/transport/router.go` (drop the `/secret-features/*` proxy route).

**Interfaces:** none produced; pure removal.

- [ ] **Step 1: Grep-gate the deletion**

Run: `grep -rn "secret-features\|secret_feature\|SecretFeature" services/ frontend/web/src --include=*.go --include=*.ts --include=*.vue | grep -iv "_test\|//\|policy\|feature_flag"`
Read the results. Confirm the ONLY remaining references are the catalog secret-features files themselves + the gateway proxy route (the FE cut over in P4). If any LIVE consumer (a non-test import, a runtime call) remains, STOP and report it — do not delete.

- [ ] **Step 2: Remove**

Delete the catalog secret-features handler/service/repo/domain + tests; drop the routes in catalog router + the handler wiring in `main.go`; drop the gateway `/secret-features/*` proxy route. Also drop any `SecretFeatureFlag` `AutoMigrate`/seed registration.

- [ ] **Step 3: Build + test**

Run: `cd services/catalog && go build ./... && go test ./... -count=1` and `cd services/gateway && go build ./... && go test ./... -count=1`
Expected: both build clean, all tests PASS (no dangling references to the removed symbols).

- [ ] **Step 4: Commit**

```bash
git add services/catalog services/gateway
git commit -m "chore(catalog,gateway): remove dead secret-features backend (FE cut over to policy in P4)"
```

---

## Task 4: FE cleanup — dead code, stale comments, cold-load guard timeout

**Files:**
- Modify: `frontend/web/src/utils/secretFeatures.ts` (drop `isRouletteEnabled`), `Navbar.vue`, `Profile.vue`, `FanficsView.vue`, `api/fanfic.ts` (comments), `router/index.ts` (guard timeout).
- Test: extend the router-guard spec.

- [ ] **Step 1: Write the failing test**

Router-guard cold-load timeout: with the feature-visibility store's `ready` NEVER resolving (simulate a hung feed), navigating to a `gachaGated` route must still resolve within the short timeout using the D1 fallback (admin allowed, non-admin redirected home) rather than hanging. Assert the navigation completes (doesn't await indefinitely). Mirror `feature-visibility-gate.spec.ts`.

- [ ] **Step 2: Run, verify fail**

Run: `cd frontend/web && bunx vitest run src/router`
Expected: FAIL (guard awaits `ready` unbounded).

- [ ] **Step 3: Implement**

- `router/index.ts`: in the async guard, replace the bare `await featureVisibility.ready` with a race against a ~2.5s timeout: `await Promise.race([featureVisibility.ready, new Promise(r => setTimeout(r, 2500))])`. Then decide via `resolveVisible(key, {loaded: fv.loaded, visible: fv.visible}, authStore.isAdmin)` as before (on timeout `loaded` is still false → D1 failSafe fallback). The store keeps loading in the background; nav visibility self-corrects reactively. Keep `load()` defensive call.
- `utils/secretFeatures.ts`: delete `isRouletteEnabled()` (grep-confirm no non-test consumer — App.vue reads `store.rouletteEnabled` directly). Update/remove its test.
- Comments: in `Navbar.vue`, `Profile.vue`, `FanficsView.vue`, `api/fanfic.ts`, replace `VITE_*_ADMIN_ONLY`-naming comments with an accurate note ("visibility resolved at runtime via the policy feed / useFeatureVisible").

- [ ] **Step 4: Full FE gate**

Run:
```
cd frontend/web
bunx vitest run
bash scripts/design-system-lint.sh
bunx tsc --noEmit
bun run build
```
Expected: full suite PASS (bar the pre-existing unrelated `SubtitleSettingsMenu` failure — confirm unchanged), DS-lint 0, tsc clean, build 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src
git commit -m "chore(web): drop dead isRouletteEnabled; refresh stale gate comments; bound guard cold-load"
```

---

## Self-Review notes (plan author)

- Coverage: A1→T1, A2→T2, B1→T3, B2/B3→T4.
- Verify during execution: the exact `providerWire`/`toWire` + `DerivedState()`/`StateCode()` accessors (read `internal_scraper_providers.go` + `scraper_provider.go`); the catalog `/api/admin` group's middleware; that `/api/admin/scraper-providers` doesn't collide with the gateway `/admin/scraper/*` scraper-proxy prefix (research says it falls through to catalog — confirm with a gateway routing test or by reading the registration order).
- T3 is deletion-risky — the grep-gate in Step 1 is mandatory; if a live consumer remains, defer T3 and report.
- After Task 4: `/frontend-verify`, whole-branch review (P5 range), then owner-gated integrate/deploy of P1–P5.
