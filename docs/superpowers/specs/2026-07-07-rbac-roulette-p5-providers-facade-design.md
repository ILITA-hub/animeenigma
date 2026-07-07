# RBAC and Roulette — Phase 5: Providers Facade Tab + P4 Cleanups — Design

**Status:** approved-to-build (owner chose "Build P5 first", 2026-07-07). Grounded by a provider-policy admin-surface research pass.
**Builds on:** P1–P4 (merge-ready on `feat/rbac-and-roulette`).

**Effort metrics:** UXΔ = +1 (Better) — admins get a runtime auto/disabled switch per provider + a dead-code sweep. · CDI = 0.04 * 13 (new catalog admin endpoints + FE tab; additive, no self-heal/playback logic touched). · MVQ = Sprite 84%/86%.

## Goal

Fill the `/admin/policy` **Providers** tab: list every scraper provider with its current policy + health, and let an admin flip a provider between **auto** and **disabled** at runtime (a facade over catalog's existing `stream_providers` authority). Plus the P4 dead-code cleanups.

## Scope

**In scope:**
- **Providers facade** — catalog admin read + write endpoints (facade over `ScraperProvider.Policy`), the FE Providers tab, composable, i18n.
- **P4 cleanups** — remove now-dead catalog secret-features backend + gateway `/secret-features/*` proxy; drop the dead FE `isRouletteEnabled()`; refresh stale `VITE_*_ADMIN_ONLY` comments; bound the router-guard cold-load stall.

**Out of scope (unchanged decisions):**
- **Provider-policy DB-ownership move** into policy-service (the long-standing deferred TODO). The facade writes catalog's existing table; catalog keeps the self-heal engine / probe / `/capabilities` authority. File the DB-ownership move as ISS-NNN (still deferred).
- The self-heal state machine, probe pipeline, `/capabilities` derivation — untouched.

## Part A — Providers facade

### A1. Backend (catalog) — read + write endpoints

Catalog owns `stream_providers` (`services/catalog/internal/domain/scraper_provider.go`). Add an admin handler `services/catalog/internal/handler/admin_scraper_providers.go`:

- **`GET /api/admin/scraper-providers`** → `{ providers: [providerWire] }`. Reuse the existing `providerWire`/`toWire` mapping from `internal_scraper_providers.go` (name, policy, health, derived status, health_since/policy_since, group, reason, description, scraper_operated, supports_*, engine, base_url, last_tick_metrics, updated_at). **Add a `derived_state` field** to the wire (from `ScraperProvider.DerivedState()`/`StateCode()`) so the FE renders the 5-state pill without re-implementing precedence.
- **`PUT /api/admin/scraper-providers/{name}/policy`** body `{ "policy": "auto" | "disabled" }`:
  - 404 if `name` doesn't exist; **400** if `policy` ∉ {auto, disabled} (reject `manual` — it's a machine-set state per the domain's admin/machine split; the admin levers are auto/disabled per the owner's ask).
  - Set `Policy` + `PolicySince = now`; **leave `Health`/`HealthSince` untouched** (health stays probe-owned). Return the updated `providerWire`.
  - Semantics that follow for free from the existing model: `disabled` → provider vanishes from failover + capability feed (hard lock, immune to self-heal until flipped back); `auto` → re-enters machine-managed health-gated rotation.
- **Router:** mount both under catalog's existing `/api/admin` group (`services/catalog/internal/transport/router.go` — already `AuthMiddleware + AdminMiddleware`). No new middleware.
- **Gateway:** none. `/api/admin/scraper-providers*` does not match the more-specific `/admin/scraper/*` (scraper service) prefix, so it falls through the generic `/admin/*` → `ProxyToCatalog` group (same as `/api/admin/collections` etc.).

### A2. Frontend — the Providers tab

- `frontend/web/src/composables/useAdminProviders.ts` (mirror `useAdminPolicy.ts`): `list()` → `GET /api/admin/scraper-providers`; `setPolicy(name, policy)` → the PUT. Types for the provider wire + `derived_state`.
- `adminApi` methods in `api/client.ts`: `listScraperProviders()`, `setScraperProviderPolicy(name, policy)`.
- Replace `AdminPolicy.vue`'s `#providers` `EmptyState` with a provider list: each row shows the provider (display name / `name`, `group`, `engine`), a **status pill** from `derived_state` (UP / Recovering / Degraded / Down / Disabled — semantic tokens, provider/brand hues allowed), the `reason` (if any), and an **auto/disabled control** (a `Switch` "Enabled (auto)" ↔ off = "Disabled", or a two-option `SegmentedControl`). A provider currently in `manual` renders its pill as Degraded and the control reflects "auto" side unset — flipping to auto or disabled is the admin action.
- **Confirm on disable** (disabling removes a provider from playback failover): a `ConfirmDialog` before the `setPolicy(name,'disabled')` call. Enabling (→auto) needs no confirm.
- Optimistic update + toast on success/failure; reload the row from the returned wire.
- i18n `admin.policy.providers.*` in en/ru/ja (tab already exists; the placeholder keys get repurposed/extended).

## Part B — P4 cleanups

- **B1 (backend):** remove the now-dead catalog secret-features surface — `services/catalog/internal/{handler/secret_feature.go, service/.../secret_feature*, repo/.../secret_feature*, domain SecretFeatureFlag}` + its routes (`/api/admin/secret-features*`, `/api/secret-features/state`) + the gateway `/secret-features/*` proxy route. **Only after grepping the whole repo to confirm no live consumer remains** (P4 cut the FE over to policy `/features/mine`; the footer + admin no longer call catalog secret-features). If ANY live consumer remains, stop and report. Catalog build + tests stay green.
- **B2 (frontend):** delete the dead `isRouletteEnabled()` from `utils/secretFeatures.ts` (App.vue reads `store.rouletteEnabled` directly now; grep-confirm no other consumer). Refresh the stale `VITE_*_ADMIN_ONLY` comments in `Navbar.vue`, `Profile.vue`, `FanficsView.vue`, `api/fanfic.ts` to name the policy feed instead.
- **B3 (frontend):** bound the router-guard cold-load stall — a first-navigation deep-link to a gated route while policy is down currently awaits `store.ready` up to the axios 30s timeout before failing open. Race `store.ready` against a short timeout (~2.5s) in the guard; on timeout, decide via the D1 failSafe fallback (`resolveVisible` with `loaded=false`) and let the store finish loading in the background (nav visibility self-corrects reactively). Add a test.

## Testing
- **Go (catalog):** handler unit tests — List returns the wire incl. `derived_state`; SetPolicy sets auto/disabled + PolicySince, 404 unknown name, 400 invalid/`manual` policy, health untouched. Full catalog suite green after the B1 deletion.
- **FE:** `useAdminProviders` typing; Providers-tab renders rows + pills, toggle calls setPolicy, disable shows confirm; router-guard cold-load timeout test; i18n en/ru/ja parity; DS-lint 0; tsc; build.

## Deferred / follow-ups
- Provider-policy DB-ownership move into policy-service (ISS-NNN, still deferred).
- After P5: whole-branch review, then owner-gated integrate/deploy of P1–P5.
