# RBAC and roulette — `policy-service` design

- **Date:** 2026-07-06
- **Status:** Approved design (pre-plan)
- **Origin:** three feedback items —
  - `2026-06-08T10-26-58_NANDIorg_9_feedback` — "Добавить в админ панель настройку видимости функциональностей по ролям" (admin control of feature visibility **by role**).
  - `2026-06-11T00-09-19_tNeymik_telegram` — small fixes (hide/add a button, "as Oronemu did") should work for **one user first**, then be scaled to everyone; *"maybe use a feature-flags mechanism."*
  - `2026-07-04T07-37-57_tNeymik_manual` — the "секретная фича" **roulette** tab (already shipped 2026-07-06; here it becomes a *view* over the flag store).

---

## 1. Problem & goal

Today's dark-ship model is **non-interactive and binary**:

- Each dark-shipped feature has a hand-written gate util reading a **build-time** `VITE_*_ADMIN_ONLY` env var (`fanficGate.ts`, `gachaGate.ts`, `profileWallGate.ts`) — either *admin-only* or *all-authenticated*, nothing in between.
- The gateway mirrors this with **static env bools** (`FanficAdminOnly`, `GachaAdminOnly`, `ProfileWallAdminOnly`) that conditionally add `AdminRoleMiddleware` to a route group.
- Changing who sees a feature requires an **env change + rebuild + redeploy**.
- The one interactive surface — `/admin/secret-features` → catalog `SecretFeatureFlag` — governs **only the roulette pool**, still binary on/off. Its own code comment says it *"foreshadows the future role-based access management model (per-feature → per-role)."*

**Goal:** a runtime, per-user-resolvable **feature-access authority** — `policy-service` — that lets an admin, from the admin panel:

1. Gate a feature by **role** (the 06-08 ask).
2. Grant a feature to **specific users first**, then widen the audience (the 06-11 ask).
3. Govern the **roulette** pool through the same store (the "roulette" half of the name).
4. **Manage video-provider policy** (auto/disabled) from one place (a facade tab).

## 2. Scope

**In scope**
- New microservice **`policy-service` (:8098)** with its own Postgres DB.
- `FeatureFlag` store: `key` → audience (`roles` + `allowUsers` + `denyUsers`) + `roulette` bit + `failSafe`.
- Migration of catalog's `SecretFeatureFlag` (roulette on/off + master) into policy-service.
- **Gateway enforcement** — replace static `*AdminOnly` env bools with a per-request `FeatureGate(key)` middleware evaluating a **cached ruleset** locally from JWT claims (hard security boundary).
- **Per-user FE feed** `GET /api/policy/features/mine` driving SPA visibility + roulette eligibility; the `VITE_*_ADMIN_ONLY` gate utils collapse into one `useFeatureVisible(key)`.
- **Admin UI** `/admin/policy` — **Features** tab (audience editor, absorbs `AdminSecretFeatures.vue`) + **Providers** tab (auto/disabled facade over catalog).

**Out of scope (deferred — see §11 TODO)**
- Moving video-provider policy **data** out of catalog into its own DB owned by policy-service. In this scope the Providers tab is a **management facade** only: it reads catalog's roster and writes policy through a catalog admin endpoint; the self-healing engine, probe loop, daily recovery operator, and `/capabilities` derivation stay in catalog, untouched.
- **New roles** (e.g. `beta`, `moderator`). Kept as `user`/`admin` (+ ephemeral `guest`). Per-user `allowUsers` covers the "beta tester / Oronemu-first" case. Named roles/cohorts can be added later without reshaping the model.
- Percentage / cohort rollout bucketing (nobody asked; YAGNI).

## 3. Current state (verified 2026-07-06)

- **Roles** — `libs/authz/jwt.go`: `RoleUser="user"`, `RoleAdmin="admin"`, `RoleGuest="guest"` (ephemeral, Watch-Together-only). JWT carries `UserID`, `Username`, `Role`.
- **Catalog secret-features** — `domain/secret_feature.go` (`SecretFeatureFlag{Key,Enabled}`, reserved `__roulette__` master, `SecretFeatureDefaultsDisabled=["gacha"]`), `service/secret_feature.go` (fail-open resolver), admin `/api/admin/secret-features`, public fail-open `/api/secret-features/state`.
- **FE roulette** — `utils/secretFeatures.ts`: `SECRET_FEATURES` roster (`anidle,status,themes,game,gacha,downloads,showcase-editor,my-feedback`), client `eligible()` closures, admin `disabledKeys` overlay, `pickSecretFeature()`.
- **FE gates** — `utils/fanficGate.ts`, `gachaGate.ts`, `profileWallGate.ts`: `VITE_*_ADMIN_ONLY !== 'false'` → admin-only else all-authenticated.
- **Gateway gating** — `config.go`: `FanficAdminOnly/GachaAdminOnly/ProfileWallAdminOnly` (env, default true). `transport/router.go`: `if cfg.FanficAdminOnly { r.Use(AdminRoleMiddleware) }` on the feature's route group (JWT + `BlockGuestRoleMiddleware` already present). Admin-tool routes are **always** admin-gated, independent of the dark-ship flag.
- **Provider policy** — catalog `domain/scraper_provider.go`: `ProviderPolicy` = `auto|manual|disabled` (admin/machine intent) ⟂ `ProviderHealth` = `up|recovering|down` (probe-observed); `DerivedState()` collapses to a 5-state dashboard code; feeds the `/capabilities` authority. Driven by `service/providerpolicy/engine.go` (`ApplyVerdict`, 24h demote/promote), probe loop (`/internal/providers/probe-result|probe-plan`), and the daily recovery operator. **No admin UI exists today.**

## 4. Data model

```
FeatureFlag {
  key         string     // stable id: "fanfic","gacha","profile-wall","anidle",...
  audience {
    roles      []string   // subset of {"admin","user","everyone"}; "guest" never granted
    allowUsers []string   // userIDs always granted (overrides role miss)
    denyUsers  []string   // userIDs always blocked (overrides everything)
  }
  roulette    bool        // participates in the «Секретная фича» pool
  failSafe    string      // audience used ONLY when the ruleset can't be resolved
                          //   at cold start: "admin" (sensitive) | "everyone" (cosmetic)
  label       string      // human label for the admin table (or i18n key)
  updatedAt   time.Time
}
```

Reserved master key `__roulette__` survives (global roulette on/off), stored as a flag row but excluded from the feature roster.

### Resolution semantics — `canAccess(userID, role, flag)`

Pure, order-sensitive, evaluated locally (no I/O):

1. `role == "guest"` → **deny** (guests are Watch-Together-only).
2. `userID ∈ denyUsers` → **deny**.
3. `userID ∈ allowUsers` → **allow**.
4. `"everyone" ∈ roles` → **allow**.
5. `role ∈ roles` → **allow**.
6. else → **deny**.

Anonymous (no JWT): only step 4 can allow (an `everyone` flag). This is deterministic and side-effect-free so the gateway and FE feed compute identical verdicts.

## 5. Architecture

### 5.1 `policy-service` (:8098)

- **Own Postgres DB** (`FeatureFlag` table). Standard `DB_*` + `JWT_SECRET` + `REDIS_HOST`. GORM `AutoMigrate`.
  - **GORM gotcha:** like `SecretFeatureFlag`, do **not** put a `default:` tag on `bool`/rich fields written explicitly — GORM omits zero-values that carry a default. Rows are always upserted with explicit values.
- **Admin API** (gateway-proxied, admin-gated at the gateway):
  - `GET /api/admin/policy/flags` — list flags (+ resolved master).
  - `PUT /api/admin/policy/flags/{key}` — set audience / roulette / label (upsert).
  - `PUT /api/admin/policy/roulette` — master switch.
  - `GET /api/admin/policy/providers` · `PUT /api/admin/policy/providers/{name}` — provider facade (proxies catalog; §5.4).
- **Internal ruleset feed** — `GET /internal/policy/ruleset` (Docker-network-only; the gateway does **not** proxy `/internal/*`). Returns the compact all-flags snapshot the gateway caches. Cheap, no per-user data.
- **Per-user FE feed** — `GET /api/policy/features/mine` (gateway-proxied; **optional** JWT). Returns, for the caller: `{ visible: string[], roulette: string[], rouletteEnabled: bool }` — the set of flag keys they can see and the roulette-eligible subset. Anonymous callers get the `everyone`/public resolution. **Fail-open** (see §7).
- **Redis pub/sub** — on any flag write, publish an invalidate on `policy:ruleset:changed` so the gateway refreshes immediately (bounded also by a periodic ~15s refresh).
- **Metrics** — `/metrics` (`http_requests_total`, `http_request_duration_seconds`, …), plus `policy_flag_writes_total` and `policy_ruleset_refresh_total`.

### 5.2 Gateway enforcement (hard boundary)

- New `FeatureGate(key string)` middleware in `services/gateway/internal/transport/`. It reads the **in-memory cached ruleset** (loaded from `policy-service /internal/policy/ruleset`, refreshed ~15s + on pub/sub invalidate) and evaluates `canAccess(claims.UserID, claims.Role, flag)`; on deny → `403`. No per-request network hop.
- **Migration:** replace `if cfg.FanficAdminOnly { r.Use(AdminRoleMiddleware) }` with `r.Use(FeatureGate("fanfic"))` on the fanfic/gacha/profile-wall route groups (JWT + `BlockGuestRoleMiddleware` stay). Admin-tool routes remain **unconditionally** `AdminRoleMiddleware` — they are not feature-flagged.
- **Route→key mapping stays explicit in the router** (one `FeatureGate(...)` line per gated group), not data-driven — clearer, and a new gated feature is a one-line change exactly like today.
- **Cold-start / outage safety:** if the ruleset is not yet loaded, `FeatureGate` falls back to the flag's `failSafe` audience (unknown flag → `admin`, i.e. fail-**closed** for sensitive routes). Once loaded, a transient policy-service outage is invisible — the gateway keeps the last-known-good ruleset (fail-**static**).
- **Config:** `POLICY_SERVICE_URL` (default `http://policy-service:8098`), `POLICY_RULESET_REFRESH` (default `15s`). The old `*AdminOnly` env bools are removed once cutover lands (kept only as the seed source — §6).

### 5.3 Frontend

- **`useFeatureVisible(key)`** composable, backed by a Pinia-cached fetch of `/api/policy/features/mine` (fetched once by `App.vue`, refreshed on login/logout). Returns a reactive boolean. Replaces `fanficGate.ts` / `gachaGate.ts` / `profileWallGate.ts`. **Fail-open** to the build-time default (`VITE_*_ADMIN_ONLY`) so a fetch miss = today's behavior.
- **Roulette** — `secretFeatures.ts` `pickSecretFeature()` intersects `roulette` keys from `/features/mine` with the client-side contextual `eligible()` closures (e.g. `downloadsAppOnly` — PWA-installed vs browser — which can't be expressed server-side). Master switch from `rouletteEnabled`.
- **Admin UI** — new route `/admin/policy` (`AdminPolicy.vue`) with two tabs:
  - **Features** — table of flags; each row edits audience (role checkboxes: admin/user/everyone; `allowUsers`/`denyUsers` as username chips resolved to userIDs), a **roulette** toggle, and the global **roulette master** switch. Absorbs `AdminSecretFeatures.vue`.
  - **Providers** — provider roster with an **auto ⇄ disabled** control (facade → catalog), read-only Health/DerivedState shown for context. A banner links the deferred DB-ownership TODO (§11).
  - Follows the Neon-Tokyo DS (semantic tokens, `Select`/`Switch`/`Checkbox` primitives, `p-4 md:p-6 lg:p-8`); i18n en/ru/ja parity.

### 5.4 Provider-policy facade

- policy-service `GET /api/admin/policy/providers` → calls catalog's internal roster (reuse `/internal/scraper/providers`) and returns `{name, policy, health, derivedState}` per provider.
- `PUT /api/admin/policy/providers/{name}` `{policy:"auto"|"disabled"}` → calls a **catalog admin endpoint** that sets `ProviderPolicy` on the row (add if absent; `manual` remains a machine-set state, not exposed as an admin choice here). Catalog stays the single writer of its own table and re-derives `/capabilities`.
- No change to the self-healing engine, probe loop, or daily operator.

## 6. Migration & day-one safety

- **Seed on first boot** from the current effective defaults so behavior is **identical on day one**:
  - `fanfic`, `gacha`, `profile-wall` → `roles:["admin"]`, `failSafe:"admin"` (mirrors `*_ADMIN_ONLY=true`).
  - roulette pool (`anidle,status,themes,game,downloads,showcase-editor,my-feedback`) → `roles:["everyone"]`, `roulette:true`, `failSafe:"everyone"`; `gacha` roulette-seeded **disabled** (matches `SecretFeatureDefaultsDisabled`).
  - `__roulette__` master → enabled.
- Catalog `SecretFeatureFlag` rows migrate to policy-service `FeatureFlag` (roulette bit + disabled state preserved). Catalog's secret-features endpoints are retired once the FE reads `/features/mine`.
- Everything **fail-open on the FE** and **fail-static on the gateway** — no new hard outage mode versus today.

## 7. Failure model

| Layer | Failure | Behavior |
|-------|---------|----------|
| Gateway | ruleset never loaded (cold start) | per-flag `failSafe`; unknown → `admin` (fail-closed for sensitive) |
| Gateway | policy-service down after load | last-known-good ruleset (fail-static) |
| FE | `/features/mine` fetch fails | fail-open to build-time `VITE_*` defaults (today's behavior) |
| FE | roulette feed missing | roulette master on, nothing disabled (today's fail-open) |

## 8. Testing

- **policy-service** — resolver truth table (`canAccess` all 6 branches incl. deny-over-allow, guest, anon); repo upsert (GORM zero-value gotcha); ruleset + `/mine` handler tests; provider facade with a catalog fake. `sqlite` for unit tests (BeforeCreate UUID hook if IDs are UUID — see fanfic-engine gotcha).
- **gateway** — `FeatureGate` allow/deny/403 across role+allow+deny matrices; cold-start `failSafe`; fail-static on refresh error; pub/sub invalidate.
- **FE** — `useFeatureVisible` fail-open; `pickSecretFeature` intersect logic; `AdminPolicy.vue` spec (audience editor round-trip); i18n en/ru/ja parity; `/frontend-verify`.

## 9. Phasing (for the implementation plan)

- **P1 — policy-service foundation:** service scaffold + DB + `FeatureFlag` + resolver + admin CRUD + `/internal/ruleset` + `/mine`; seed defaults; migrate `SecretFeatureFlag`. Gateway route `/api/policy/*` + `/api/admin/policy/*`.
- **P2 — gateway enforcement:** ruleset cache + refresh/pub-sub + `FeatureGate`; cut fanfic/gacha/profile-wall route groups over; remove `*AdminOnly` bools.
- **P3 — admin Features tab:** `/admin/policy` Features tab; absorb `AdminSecretFeatures.vue`; audience editor.
- **P4 — FE visibility cutover + Providers tab:** `useFeatureVisible` replaces the three gate utils; roulette reads `/mine`; provider facade + Providers tab.

## 10. Non-goals / decisions

- Roulette is a **view over flags**, not a separate store.
- **No new roles** in this scope; per-user `allowUsers` covers progressive rollout.
- Provider policy is a **facade** now; data-ownership migration is deferred (§11).
- Enforcement mapping stays **code-explicit** in the gateway router, not a data-driven route table.

## 11. Deferred TODO

> **TODO (refactor, out of current scope):** Move video-provider policy **data** out of catalog into its **own database owned by `policy-service`**. Today the Providers tab is a facade over catalog's `scraper_providers`; the intent is for policy-service to own the admin-intent dimension (policy: auto/manual/disabled) as the authority, with catalog reading it to derive `/capabilities`. This touches the playback-critical self-healing/probe/capability path, so it is planned as a separate, carefully-sequenced migration. Tracked here and to be filed as an ISS-NNN issue.

## 12. Open questions

- Exact catalog admin endpoint for provider-policy writes (reuse/extend an existing handler vs add `PUT /internal/providers/{name}/policy`) — resolve during P4 planning.
- Whether `/features/mine` should be cached per-user in Redis (short TTL) or resolved on the fly — measure first; resolution is cheap.

## Metrics

- **UXΔ** = `+2 (Better)` — admins gain runtime, per-user, role-scoped rollout control; end users see identical behavior on day one.
- **CDI** = `0.06 * 34` — moderate spread (new service + gateway middleware + FE gate collapse + catalog facade + admin UI), sizable effort.
- **MVQ** = `Griffin 85%/80%`.
