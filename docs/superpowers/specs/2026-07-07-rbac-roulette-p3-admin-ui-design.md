# RBAC and Roulette — Phase 3: Policy Admin UI + Standard User Resolver

**Status:** approved (design-prototyping v4, owner-signed 2026-07-07)
**Builds on:** `docs/superpowers/specs/2026-07-06-rbac-and-roulette-design.md` (overall) · P1 (policy-service :8098) + P2 (gateway `FeatureGate`) — both merge-ready on `feat/rbac-and-roulette`.
**Prototype of record:** `.superpowers/brainstorm/policy-admin/content/policy-admin-v4.html`

**Effort metrics:** UXΔ = +3 (Better) — admins gain runtime, per-role + per-user feature control with a live access check. · CDI = 0.05 * 21 (new admin view + shared resolver across 3 services; low spread — additive, no playback-path change). · MVQ = Griffin 88%/82%.

---

## Goal

Ship the `/admin/policy` **Features tab** — the runtime "steering wheel" for the P1/P2 policy engine — and a **single standard user-ID resolver** reused across admin surfaces. After this phase an admin can, at runtime, set each feature's audience (roles + per-user allow/deny), toggle its roulette membership, open the feature to preview it, and see who can access what — without a rebuild.

## Scope

**In scope**
- **Part A — Standard user resolver** (foundational, cross-cutting): one canonical auth endpoint + one shared FE component; `recs` adopts both.
- **Part B — Policy admin UI (P3)**: `AdminPolicy.vue` Features tab (absorbs `AdminSecretFeatures.vue`), composable, feature registry, contextual open-link, live access preview, router + redirect, i18n ×3.

**Out of scope (deferred)**
- **P4** — FE runtime cutover: `useFeatureVisible()` replacing `VITE_*_ADMIN_ONLY`, roulette footer reading `/api/policy/features/mine`, and the Providers-tab facade content. Until P4 the user-facing footer roulette still reads catalog `SecretFeatureFlag`; admin edits here are authoritative for the gateway boundary (P2) but won't change nav/footer visibility yet. **This transient split is expected — document it in the UI.**
- **Provider-policy DB ownership** move out of catalog (existing owner TODO — file as ISS-NNN, untouched here). The Providers tab is a **disabled/"coming in P4" placeholder** only.

---

## Part A — Standard User Resolver

### A1. Backend — canonical auth endpoint

`GET /api/admin/users/resolve?q=<identifier>` — JWT + admin.

- Accepts any single identifier: **UUID** (`id`), **username**, **public_id**, or **telegram_id** (numeric string).
- Resolution cascade (reuses existing `UserRepository` methods): UUID-shaped → `GetByID`; all-digits → `GetByTelegramID`(parsed int64) then fall through; else `GetByUsername` then `GetByPublicID`. First hit wins.
- **200** `{ "id": "<uuid>", "username": "...", "public_id": "...", "telegram_id": 123 }` (`telegram_id` omitempty).
- **404** when no user matches. **400** when `q` is empty/whitespace.
- Handler lives in auth (`services/auth/internal/handler/`), gated by the existing `AuthMiddleware` + `AdminMiddleware`, mounted under a new `/api/admin/users` group in `services/auth/internal/transport/router.go`.

**Gateway:** proxy `GET /api/admin/users/resolve` → auth via `ProxyToAuth`, inside a `JWTValidation + AdminRoleMiddleware` group, registered **before** the catalog `/api/admin/*` catch-all so it isn't swallowed. (`getServiceURL("auth")` already exists.)

### A2. Frontend — `UserResolveInput.vue` (shared, single source of the resolve UX)

`frontend/web/src/components/admin/UserResolveInput.vue`

- Wraps the `Input` primitive. Placeholder (i18n): "username, public_id, Telegram ID или UUID".
- On **Enter** (or explicit button), calls `GET /api/admin/users/resolve?q=`; on 200 emits `resolve` with `{ id, username, public_id, telegram_id }` and clears; on 404 shows an inline destructive message ("«…» не найден"); in-flight spinner (reuse `Spinner`).
- Emits: `@resolve="(user)"`. Props: `mode?: 'chip'|'nav'` (nav = recs picker just needs the id; chip = editor pushes a chip). Keep it dumb — the parent decides what to do with the resolved user.
- No `window.open`, no direct store coupling. lucide icons via **named imports**.

### A3. `recs` adoption (unify on the one resolver)

- `AdminRecsPicker.vue` uses `UserResolveInput` (mode `nav`): resolve the typed identifier to a **UUID client-side**, then `router.push('/admin/recs/{uuid})`. This gives the recs picker Telegram-ID support + inline not-found validation it lacks today. The existing "You"/self quick-action stays.
- `services/recs/internal/handler/admin_recs.go` `resolveUserID`: add `telegram_id = ?` to the `WHERE` (numeric-only) as a defensive fallback so a direct `/admin/recs/{tgid}` deep link still resolves even if it bypasses the picker. Preserve all existing behavior + tests (UUID passthrough, users-table-missing fall-through).

---

## Part B — Policy Admin UI (P3)

### B1. Feature registry (FE) — `frontend/web/src/config/policyFeatures.ts`

Static map `key → { route: string /* same-origin relative */, icon? }` for the open-link and any per-feature affordances. Flags absent from the registry render without an open-link (graceful). Seed: `fanfic→/fanfics`, `gacha→/gacha`, `profile-wall→/profile`, `anidle→/anidle`, `showcase-editor→/profile#showcase`, `themes→/themes`, `status`, `game`, `downloads`, `my-feedback` → their routes. Labels come from the flag's `label` field (BE) — registry holds routes/icons only.

### B2. Composable — `frontend/web/src/composables/useAdminPolicy.ts`

- `list()` → `GET /api/admin/policy/flags` → `{ flags: FeatureFlag[], rouletteEnabled: boolean }`.
- `setFlag(key, { roles, allowUsers, denyUsers, roulette, failSafe, label })` → `PUT /api/admin/policy/flags/{key}`.
- `setRoulette(enabled)` → `PUT /api/admin/policy/roulette`.
- Types mirror the P1 `FeatureFlag` JSON: `{ key, roles, allowUsers, denyUsers, roulette, failSafe, label, updatedAt }`. `allowUsers`/`denyUsers` are **userID (UUID) arrays**.

### B3. `AdminPolicy.vue` — Features tab

`frontend/web/src/views/admin/AdminPolicy.vue` (Tabs: **Фичи** active, **Провайдеры** placeholder).

- **Master roulette switch** (top): reads `rouletteEnabled`, writes `setRoulette`. (The reserved `__roulette__` flag is collapsed into this by the BE and excluded from `flags`.)
- **Per-flag card** for each `flags[]` entry:
  - Header: `label`, `key` + registry `route`, **open-link** (lucide `ExternalLink` + contextual helper B4), **failSafe badge** (`admin`→violet "админы" / `everyone`→success "все"), **roulette** `Switch`.
  - Body — audience editor:
    - **Roles**: three toggle chips `admin` / `user` / `everyone` (multi-select → `roles[]`).
    - **Allow list** / **Deny list**: chips of resolved users (`username #id-prefix`, removable) + a `UserResolveInput` to add. Store `id` in `allowUsers`/`denyUsers`; on add, push `{id, username}` to the chip model.
  - **Persist** per flag via an explicit **Save** button (dirty-state aware) → `setFlag`. Optimistic disable while in flight; toast on success/failure.
- **Display resolution (id→username):** on load, `allowUsers`/`denyUsers` arrive as UUIDs. Resolve each to a username for the chip via the standard resolver (`q=<uuid>` hits the UUID branch). Counts are tiny (0–2/flag day-one); a deleted user renders as the raw id. (A batch endpoint is a future optimization, not required.)

### B4. Contextual open helper — `frontend/web/src/composables/useOpenFeature.ts`

- `isStandalone = matchMedia('(display-mode: standalone)').matches || (navigator as any).standalone === true`.
- Template renders a **plain** `<a :href="route" target="_blank" rel="noopener noreferrer">` (never `window.open`).
- `@click` handler: **if standalone** → `e.preventDefault()` + `router.push(route)` (in-app nav); **else** do nothing (native `target="_blank"` opens a browser tab — a plain click then takes the same path as ⌘/ctrl-click and never launches the installed PWA).
- **Rationale (documented):** `window.open()` with an absolute PWA-scoped URL is captured by the OS/browser and launches the installed app; a relative native anchor is not. Routes MUST be same-origin relative.

### B5. Access preview — "Проверка доступа"

A panel that mirrors the P1/P2 `CanAccess` order **client-side** (guest→deny · deny-list→deny · allow-list→allow · everyone→allow · role→allow · else deny). Admin picks a **target user** via `UserResolveInput` + a role selector (or "anonymous"/"guest"); the panel shows each flag as **виден/скрыт** and which roulette entries would surface. Pure preview — no writes. (Reuses the resolver; genuinely useful for verifying an allow/deny grant.)

### B6. Providers tab — placeholder

Second tab renders a disabled/"P4" empty-state (title + one line: "Управление провайдерами появится в P4"). No API calls. The DB-ownership refactor stays a deferred TODO.

### B7. Router + redirect

- Add `/admin/policy` → `AdminPolicy.vue` with the admin-gated route meta used by other `/admin/*` pages. (Gateway SPA route `/admin/policy` + `/admin/policy/*` → web is already registered in P1.)
- `/admin/secret-features` → **redirect** to `/admin/policy`. Remove `AdminSecretFeatures.vue` from the router (keep or delete the SFC per finishing cleanup; its roulette controls are absorbed here). Update any nav/link that pointed at `/admin/secret-features`.

### B8. i18n — `admin.policy.*` in en / ru / **ja**

All three locales (parity gate). Namespace: tab labels, master switch, role chip labels, allow/deny labels + placeholder, failSafe copy, open-link aria, resolver not-found, access-preview copy, providers placeholder, save/toast strings.

---

## Design System

Neon-Tokyo. Reuse primitives: `Tabs`, `Card`/`glass-card`, `Switch`, `Input`, `Button`, `Badge`, `Spinner`, `Tooltip`. lucide **named imports** (`ExternalLink`, etc.). Semantic tokens only; no hardcoded hex; no `window.open`. Passes `design-system-lint.sh` + `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real `bun run build`, tsc). Provider/brand hues (cyan) allowed per DS carve-out.

## Testing

- **Go:** auth resolver — repo/handler unit tests (each identifier type + 404 + empty). recs — existing `admin_recs` tests stay green; add a telegram_id resolve case.
- **FE:** `UserResolveInput.spec.ts` (resolve success/404/empty), `useAdminPolicy` typing, `AdminPolicy.spec.ts` (renders flags, role toggle, roulette toggle, save calls setFlag, access preview logic), i18n parity spec, `tsc --noEmit`, real build.
- Manual: click "открыть ↗" in browser (new tab, no app launch) vs standalone (in-app).

## Deferred / follow-ups

- **P4:** `useFeatureVisible()` cutover + roulette footer via `/features/mine` + Providers facade content.
- **Provider-policy DB ownership** (ISS-NNN).
- Batch `id→username` resolve endpoint (only if per-id display calls prove chatty).
