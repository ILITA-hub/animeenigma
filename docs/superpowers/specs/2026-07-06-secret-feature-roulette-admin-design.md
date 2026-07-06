# Secret Feature Roulette — Admin Management Page

**Date:** 2026-07-06
**Status:** Approved-direction (owner requested; recommended option chosen while owner away)

## Goal

Add an admin page to manage the footer "Secret Feature" roulette. It lists **all
features** in the pool, shows the **roulette state (enabled/disabled)** — a master
switch plus a per-feature toggle — and a **direct link** to each feature. The page
is explicitly a seed for a future **role-based access management** surface, so the
per-feature granularity here is deliberate (per-feature → per-role later).

## Chosen approach (Option A: backend-persisted, global + per-feature)

Toggles take effect **live for all users** and are **fail-open**: if the frontend
cannot read the config, the roulette behaves exactly as today.

### Source of truth
- **Feature roster + client eligibility** stays in the frontend
  `utils/secretFeatures.ts` (`SECRET_FEATURES`). It already owns the pool and the
  client-side `eligible()` predicates (auth/PWA/wall gates). We only enrich each
  entry with a `labelKey` for display.
- **Admin enable-state** is stored in the backend (catalog) as a tiny key→bool
  store. The backend does NOT duplicate the feature roster — it stores only
  explicit on/off overrides. Absent key = enabled (default true).

### Backend (catalog) — additive, low-risk
- `domain.SecretFeatureFlag { Key string PK; Enabled bool; UpdatedAt }`. A
  reserved key `__roulette__` holds the master switch.
- Repo: `GetAll(ctx) map[string]bool`, `Set(ctx, key, enabled)` (upsert).
- Service: `GetConfig` → `{ rouletteEnabled, features: map[key]bool }` (explicit
  overrides only); `PublicState` → `{ rouletteEnabled, disabledKeys []string }`;
  `SetRoulette`, `SetFeature`. Master/feature default = enabled.
- Handler + routes:
  - Admin (under existing `/admin` group — no gateway change):
    - `GET  /admin/secret-features` → `{ rouletteEnabled, features }`
    - `PUT  /admin/secret-features/roulette` `{ enabled }`
    - `PUT  /admin/secret-features/feature/{key}` `{ enabled }`
  - Public read (roulette enforcement): `GET /secret-features/state` →
    `{ rouletteEnabled, disabledKeys }`.
- `AutoMigrate(&domain.SecretFeatureFlag{})`.

### Gateway — one public route
Add `r.HandleFunc("/secret-features/*", proxyHandler.ProxyToCatalog)` (public)
alongside the collections passthrough. Admin `/api/admin/*` is already a
catch-all to catalog.

### Frontend
- `utils/secretFeatures.ts`: add `labelKey` per entry; add module-level admin
  state (`applySecretFeatureAdminState`) that `pickSecretFeature` consults —
  filtering out admin-disabled keys. **Null state = fail-open** (behaves as today).
- `App.vue`: on mount, fetch `/secret-features/state`, apply it, and gate the
  footer roulette button with a reactive `rouletteEnabled` ref (hidden when the
  master switch is off). Fetch failure = fail-open (button stays, no filter).
- `api/client.ts`: `secretFeaturesApi.getState()` (public) + `adminApi`
  `listSecretFeatures` / `setSecretRoulette` / `setSecretFeature`.
- `views/admin/AdminSecretFeatures.vue`: master roulette Switch + a table of the
  code-defined roster (label · direct link · per-feature Switch). Toggle → PUT →
  update local state. Mirrors `AdminCollections.vue` conventions.
- Router: `/admin/secret-features` (`requiresAuth` + `requiresAdmin`).
- `AdminDashboard.vue`: add a 7th tool card (bump the spec's hardcoded count).
- i18n `admin.secretFeatures.*` + `admin.dashboard.secretFeaturesDesc` in
  en/ru/ja (locale-parity test enforced).

### Testing
- Backend: `repo/secret_feature_test.go` (SQLite, testify) — set/get/getall +
  default-true resolution.
- Frontend: extend `secretFeatures.spec.ts` (admin-disabled key excluded;
  roulette-disabled reflected); new `AdminSecretFeatures.spec.ts` (loads on
  mount, renders rows, toggle calls the setter); bump `AdminDashboard.spec.ts`
  6→7.

## Non-goals (future RBAC rework)
Roles, permissions, per-role visibility, audit log. This page persists simple
on/off flags; the role model replaces/extends it later.
