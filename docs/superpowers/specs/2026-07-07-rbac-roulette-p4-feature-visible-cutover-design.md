# RBAC and Roulette — Phase 4: Feature-Visibility Cutover — Design

**Status:** draft for owner approval (2026-07-07)
**Builds on:** P1 (policy-service), P2 (gateway `FeatureGate`), P3 (`/admin/policy` admin UI) — all merge-ready on `feat/rbac-and-roulette`.

**Effort metrics:** UXΔ = +2 (Better) — admin flag edits finally change what users see (nav/footer/routes), completing the runtime model. · CDI = 0.06 * 13 (touches shared boot + nav + router + footer; low spread, but user-facing). · MVQ = Griffin 86%/84%.

## Goal

Make the P1–P3 policy engine authoritative on the **frontend**: nav items, dark-ship routes, the profile-wall tab, and the footer roulette pool all read the per-user `/api/policy/features/mine` feed instead of build-time `VITE_*_ADMIN_ONLY` env flags and the client-side `SECRET_FEATURES` eligibility. After P4, an admin flipping a flag in `/admin/policy` changes what that user sees on next load — end to end.

## Scope

**In scope (P4):**
- A `useFeatureVisible(key)` composable + a small store fed once at app boot from `GET /api/policy/features/mine`.
- Retire `VITE_FANFIC_ADMIN_ONLY` / `VITE_GACHA_ADMIN_ONLY` / `VITE_PROFILE_WALL_ADMIN_ONLY` reads; reimplement `useFanficVisible`/`useGachaVisible`/`useProfileWallVisible` as thin delegates to `useFeatureVisible('fanfic'|'gacha'|'profile-wall')`, or replace their call sites.
- Router guards for dark-ship routes consult the store (await feed readiness) instead of the build-time consts.
- Footer roulette (`utils/secretFeatures.ts` + App.vue): pool = `mine.roulette[]` (keys, server-resolved), gated by `mine.rouletteEnabled`; map keys → routes via a registry (reuse/extend `config/policyFeatures.ts`). Retire the `/api/secret-features/state` fetch + `applySecretFeatureAdminState`.
- Remove the orphaned `adminApi` secret-features methods left by P3.

**Deferred to P5:**
- **Providers facade tab** in `AdminPolicy.vue` (auto/disabled per provider). Needs its own grounding of the scraper `/admin/scraper/*` provider-policy admin API; provider-policy DB-ownership is already a separate deferred TODO. The Providers tab stays a placeholder EmptyState (relabel "coming soon"/"P5").

## Key design decisions (need owner confirmation)

### D1 — Fail-open behavior when the feed is unavailable
The `/features/mine` feed is fail-open server-side, but the FE must still decide what to show during the pre-load window or a total policy outage. **Decision:** `useFeatureVisible(key)` returns `mine.visible.includes(key)` when the feed has loaded; otherwise a per-key **failSafe fallback** — the three dark-ship keys (`fanfic`, `gacha`, `profile-wall`) fall back to `authStore.isAdmin` (reproducing today's admin-only default), all other keys fall back to visible. This preserves today's exact behavior even if policy is down, and removes the `VITE_*` env dependency entirely. (Security is unaffected regardless — the gateway `FeatureGate` is the real boundary; FE visibility is cosmetic.)

### D2 — Day-one parity is the acceptance bar
The P1 seed reproduces current behavior (fanfic/gacha/profile-wall → admin; anidle/status/themes/game/downloads/showcase-editor/my-feedback → everyone + roulette; gacha roulette OFF; master ON). After the cutover, a normal user and an admin must see **exactly** what they see today. This is the primary regression check (manual + tests): nav items, /fanfics & /gacha route access, profile showcase tab, and the footer roulette pool contents must be unchanged for both roles on a freshly-seeded backend.

### D3 — Feed fetch timing / router guards
The feed is fetched once at app init (replacing the existing `/api/secret-features/state` boot fetch in App.vue) into a store exposing a `ready` promise. Route guards for dark-ship pages `await` `ready` (with the D1 failSafe fallback if it never resolves) before allowing/redirecting. Nav/tab visibility is reactive on the store.

## Architecture

- `stores/featureVisibility.ts` (or a composable-backed module): `load()` (GET `/api/policy/features/mine`, once), reactive `visible: Set<string>`, `roulette: string[]`, `rouletteEnabled: boolean`, `loaded: boolean`, `ready: Promise<void>`.
- `composables/useFeatureVisible.ts`: `useFeatureVisible(key): ComputedRef<boolean>` per D1.
- `utils/{fanfic,gacha,profileWall}Gate.ts`: internals delegate to `useFeatureVisible`; drop the `*_ADMIN_ONLY` const exports (update the router import).
- `utils/secretFeatures.ts`: roulette pool built from `mine.roulette[]` + registry routes; drop `SECRET_FEATURES[].eligible()` client eligibility + `applySecretFeatureAdminState` (or keep the roster only for key→route/label metadata and let the server decide membership).
- `App.vue`: swap the secret-features boot fetch for the feature-visibility `load()`.
- `api/client.ts`: add `getFeaturesMine()`; remove the orphaned secret-features admin methods.

## Testing
- Vitest: `useFeatureVisible` (feed-loaded includes/excludes; fail-open per-key fallback for dark-ship vs public); the roulette footer builds its pool from `mine.roulette` + `rouletteEnabled`; router guard redirects a non-visible dark-ship route and allows a visible one; **day-one parity** fixture (seed-equivalent `mine` for admin vs user reproduces current nav/route/pool visibility).
- i18n en/ru/ja parity; DS-lint 0; tsc; real build.
- Manual: freshly-seeded backend — verify a normal user sees no fanfic/gacha nav, admin does; footer roulette pool identical to today for both.

## Out of scope / follow-ups
- P5: Providers facade tab (+ its provider-policy admin API grounding).
- Backend catalog `/api/secret-features/*` removal (once the FE no longer calls it) — a later cleanup; P4 just stops calling it.
- Provider-policy DB-ownership move (existing TODO).
