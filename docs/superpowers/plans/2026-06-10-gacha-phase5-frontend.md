# Лудка (Gacha) — Phase 5: Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The complete UI: player side (`/gacha` banner list + daily claim, `/gacha/:id` spin screen with pity counter and x1/x10, collection album tab in Profile with black-silhouette unowned cards, navbar «Энигмы» balance + «Лудка» item) and admin side (`/admin/gacha` — cards/groups/banners management + image upload). Everything gated by `VITE_GACHA_ADMIN_ONLY` (default TRUE ⇒ visible to admins only; the bundled release flips it to false). i18n en+ru with parity test.

**Architecture:** Vue 3 + Pinia + vue-router, existing `apiClient` axios instance, `@/components/ui` primitives ONLY (Button/Card/Badge/Input/Select/Dialog/Tabs/Spinner/Alert…), Neon-Tokyo semantic tokens (design-lint is build-ENFORCED), lucide-vue-next NAMED imports. Gate logic: `gachaVisible(authStore) = flagAdminOnly ? authStore.isAdmin : authStore.isAuthenticated` in ONE util consumed by navbar + router guard.

---

## Context for the implementer (READ these files first)

- DS bible: `frontend/web/src/styles/DESIGN-SYSTEM.md`. Lint gate: `frontend/web/scripts/design-system-lint.sh` (off-palette Tailwind colors FAIL the build; exempt brand hues: cyan/pink/orange/rose/indigo/teal/lime). Only `font-medium`/`font-semibold`. `bunx` not npx; `bun` not npm.
- Patterns: `src/views/admin/AdminCollections.vue` + `AdminCollectionEdit.vue` (admin CRUD pages), `src/views/Profile.vue` (Tabs usage — collection tab goes here), `src/components/layout/Navbar.vue` (nav items + auth dropdown), `src/router/index.ts` (`meta:{requiresAuth,requiresAdmin}` + guard), `src/stores/auth.ts` (`isAdmin`), `src/api/client.ts` (axios instance + interceptors), `src/locales/__tests__/watch-together-keys.spec.ts` (en↔ru parity WITHOUT ja — copy this shape for `gacha-keys.spec.ts`).
- Backend API (ALL live on gacha:8093, gateway routes committed): 
  - Player: `GET /api/gacha/wallet`, `POST /api/gacha/daily` → `{claimed,balance,streak,bonus?}`-shaped (check handler), `GET /api/gacha/banners` (active + cards + owned + my_pity + pity_threshold), `POST /api/gacha/banners/{id}/pull` `{mode:"x1"|"x10"}` → `{cards:[{card,new,count}],balance,pity}`, `GET /api/gacha/collection` (album: all enabled cards + owned/count + per-rarity progress).
  - Admin: `/api/gacha/admin/cards|groups|banners` CRUD per `services/gacha/internal/transport/router.go` route list; `POST /api/gacha/admin/upload` (multipart `file`+`kind` OR JSON `{image_url,kind}`) → `{image_path,image_url}`.
  - Images: public `/api/gacha/images/{path}`.
  - **Read the ACTUAL response shapes from `services/gacha/internal/handler/*.go` + `internal/service/pull.go` view structs before writing TS types — do not guess field names.**
- Rarity styling (exempt hues): N → `text-muted-foreground`/neutral; R → teal; SR → indigo; SSR → orange (gold). Badges via existing `Badge` primitive + these accents.
- Silhouettes (spec §6.4): unowned card = same `<img>` with CSS `filter: brightness(0)` + name «???», rarity still visible via group/badge.
- Flag: `const GACHA_ADMIN_ONLY = import.meta.env.VITE_GACHA_ADMIN_ONLY !== 'false'` (unset ⇒ true ⇒ admin-only). Util `src/utils/gachaGate.ts` exporting `useGachaVisible()`.
- Dirty-tree rules: frontend tree is clean except 2 deleted font files (`public/fonts/noto-sans-jp-*.woff2`) — parallel work, do NOT stage them. Path-scoped commits, no `-A`, trailers per previous phases.

## Tasks

### Task A1 — API layer + store + gate util + i18n
- `src/api/gacha.ts`: typed functions for every endpoint above (player + admin). TS interfaces mirror Go JSON exactly.
- `src/stores/gacha.ts` (Pinia): wallet/balance state, `refreshWallet`, `claimDaily`, `banners`, `pull(bannerId,mode)`, `collection`; loading/error states.
- `src/utils/gachaGate.ts`: flag + `useGachaVisible()`.
- i18n: `gacha.*` namespace in `en.json` + `ru.json` (nav item «Лудка», balance tooltip, banner list, spin screen: cost/pity/«До гаранта SSR: X/90»/крутить ×1/×10, results NEW/×N, daily claim + streak, collection: прогресс/«???», admin: все CRUD-лейблы). Parity spec `src/locales/__tests__/gacha-keys.spec.ts` (en↔ru, no ja — mirror watch-together spec).
- Vitest: store spec (pull updates balance+collection refresh; daily claim states) with mocked api. 
- Commit: `feat(web): gacha API layer, store, visibility gate, i18n namespace`.

### Task A2 — Player UI
- Navbar: «Энигмы» balance chip (icon + number; lucide `Sparkles` or `Gem`, named import) → links to `/gacha`; «Лудка» nav item. BOTH rendered only when `useGachaVisible()`. Balance auto-refreshes on login + after pulls (store-driven).
- `src/views/Gacha.vue` (`/gacha`): daily-claim card (button + streak display, disabled after claim with «уже получено сегодня»), list of active banners (art, name, description, СТАНДАРТНЫЙ badge for is_standard) → click to `/gacha/:id`. Guest/non-gated → router guard redirects (below).
- `src/views/GachaBanner.vue` (`/gacha/:id`): banner art header, balance, pity progress («До гаранта SSR: {n}/{threshold}» + progress bar), Крутить ×1 (100) / ×10 (900) buttons (disabled when insufficient — show needed amount), card pool grid (owned ✓ badge), result Dialog: pulled cards grid with rarity-colored frames, NEW badge, ×N dupe badge; close → pool/balance/pity refreshed.
- Profile collection tab: new Tab «Коллекция» in `Profile.vue` tabs → `src/components/profile/GachaCollection.vue`: per-rarity sections SSR→N with progress «3 / 12», grid of cards; unowned = `brightness(0)` silhouette + «???»; owned = image + name + ×N badge. Tab itself visible only via `useGachaVisible()`.
- Heights/padding per DS (cards `p-4 md:p-6 lg:p-8` where card-like), only semantic tokens + exempt hues, `target="_blank" rel="noopener noreferrer"` n/a (no external links).
- Vitest: GachaCollection silhouette rendering (owned vs not — ≥5 asserts), GachaBanner pull-button disabled state.
- Commit: `feat(web): gacha player UI — banner list, spin screen, collection album, navbar balance`.

### Task A3 — Admin UI
- `src/views/admin/AdminGacha.vue` (`/admin/gacha`) with internal `Tabs`: **Карточки** (table: image thumb, name, source, rarity badge, enabled switch, groups; filters rarity/enabled/group; create/edit Dialog: name, source_title, rarity Select, enabled Switch, groups multiselect, image — file input OR URL input → calls admin upload → preview), **Группы** (list + create/rename/delete + view cards in group, add cards picker), **Баннеры** (list: name, standard/enabled, window, sort; create/edit Dialog: fields + card set editor with «добавить группу» Select that calls AddGroupCards + per-card add/remove via SetCards/AddCards).
- Reuse admin page scaffolding/patterns from AdminCollections.vue. Confirm-before-delete via existing Dialog pattern.
- Router: `/admin/gacha` route `meta:{requiresAuth:true,requiresAdmin:true}`; add card link on AdminDashboard.vue if it has a tools list (check; skip if structure differs).
- Vitest: AdminGacha cards-tab spec (rows render, dialog opens, ≥5 asserts).
- Commit: `feat(web): gacha admin UI — cards/groups/banners management + upload`.

### Task A4 — Router + guards
- Routes: `/gacha` (`requiresAuth`), `/gacha/:id` (`requiresAuth`), `/admin/gacha` (admin). Player routes ALSO need the gate: extend the existing global `beforeEach` — if route `meta.gachaGated` and `!gachaVisible` → redirect home. Mark `/gacha*` with `meta:{gachaGated:true}`.
- Commit: `feat(web): gacha routes + visibility guard`.

(Tasks A1→A4 sequential, ONE implementer subagent — shared files: router, i18n, Navbar.)

### Task B (CONTROLLER) — verify, deploy, live admin smoke
1. `cd frontend/web && bunx vitest run src/ --reporter=basic` (gacha specs + parity + existing suite untouched), `bunx tsc --noEmit`, `bash scripts/design-system-lint.sh`, `make i18n-lint` if exists.
2. Verify gateway working tree CLEAN → `make redeploy-gateway` (activates ALL dormant /api/gacha routes — everything is admin-gated/dark-shipped, safe).
3. `make redeploy-web` (runs lint gates; VITE_GACHA_ADMIN_ONLY unset ⇒ admin-only build).
4. Live smoke: `make health` green; `/api/gacha/wallet` via gateway: no token 401, ui_audit_bot (non-admin) 403 — the dark-ship gate LIVE check deferred from Phase 1. Site root loads; navbar shows NO Лудка for anon (curl the HTML / check bundle).
5. In-browser DS-NF-06 smoke (desktop+mobile) — the USER (admin) clicks through: создать карту с картинкой по URL → группа → баннер → включить → крутка ×1/×10 → коллекция. Report what to test.
6. Push; update memory. **Changelog: DEFERRED to the bundled release** (feature is invisible to users; advertising it now would defeat the dark-ship — explicit deviation from after-update step 4, changelog entry ships when flags flip).

## Self-Review
- Spec §6 fully covered: navbar balance ✅, banner list ✅, spin screen (pity/буттons/results NEW/×N) ✅, collection silhouettes brightness(0)+«???»+progress ✅ (§6.4 album = all enabled cards — matches backend CollectionView), daily claim ✅, guest blocked ✅ (requiresAuth + gate), §7 admin (cards w/ groups multiselect + filters, groups CRUD, banners w/ group-bulk-add, upload file-or-URL) ✅, §12 dark-ship `VITE_GACHA_ADMIN_ONLY` default-true ✅.
- DS compliance is build-enforced; rarity hues from the exempt list; primitives only.
- i18n parity test guards en↔ru; ja intentionally excluded (watch-together precedent).
- Risk: response-shape drift — mitigated by "read Go handlers first" instruction; reviewer re-checks field names.
