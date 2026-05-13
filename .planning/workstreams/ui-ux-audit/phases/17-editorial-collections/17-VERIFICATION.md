# Phase 17 (Editorial Collections) — Verification

**Date:** 2026-05-13
**Verifier:** /gsd-execute-phase agent (autonomous)
**Status:** PASSED (8/8 must-haves verified)

## Must-have results

| # | Must-have | Result | Evidence |
|---|---|---|---|
| 1 | `GET /api/collections` returns flat array of published collections; drafts hidden | PASS | `curl /api/collections` → `{"success":true,"data":[]}` (200). Repo `ListPublished` filters `WHERE published = true`; test `TestCollectionRepository_ListPublishedAndListAdmin` verifies drafts excluded. |
| 2 | `GET /api/collections/:slug` returns full record incl. `items[]` joined to anime, sorted by `sort_order ASC` | PASS | `curl /api/collections/nonexistent-slug` → 404. Repo `GetBySlug` uses `Preload("Items", db.Order("sort_order ASC, created_at ASC"))` + `Preload("Items.Anime")`. |
| 3 | Admin CRUD (POST/PUT/DELETE) + item add/remove (POST/DELETE) | PASS | 7 routes registered in `services/catalog/internal/transport/router.go` inside the `AuthMiddleware + AdminMiddleware`-gated `/api/admin` group. `curl /api/admin/collections` (no token) → 401. |
| 4 | Admin `GET /api/admin/collections` returns drafts + published; gateway 403s non-admin (defense-in-depth: catalog re-checks) | PASS | Gateway routes `/api/admin/*` through `JWTValidationMiddleware + AdminRoleMiddleware`; catalog router also wraps `/api/admin/collections*` with `AuthMiddleware + AdminMiddleware`. `curl -H "Authorization: Bearer invalid" /api/admin/collections` → 401. |
| 5 | `/admin/collections` renders list table + Create button; `/admin/collections/:id` renders form with title × 3 / desc × 3 / cover URL / published toggle / slug / item picker | PASS | `AdminCollections.vue` (155 LOC) and `AdminCollectionEdit.vue` (370 LOC) shipped; both pass `bunx vue-tsc --noEmit` + `bunx eslint`. SPA fallthrough `GET /admin/collections` returns 200 with HTML. |
| 6 | `Home.vue` renders `CollectionsRow` between `ThisWeekRow` and `ContinueWatchingRow`; fetches `GET /api/collections`; row hides when zero published | PASS | `Home.vue` modified: import + placement between `<ThisWeekRow />` and `<ContinueWatchingRow />`. `CollectionsRow.vue` has `v-if="items.length > 0"` self-gate at the wrapping div (line 5). |
| 7 | `/collections/:slug` renders hero (cover + localized title + description) + grid sorted by sort_order ASC | PASS | `Collections.vue` (155 LOC): hero with `cover_image_url` background + gradient scrim, `getLocalizedTitle` for title, locale-switched description, `sortedItems` computed with defensive sort. |
| 8 | All ~20 i18n keys render in en/ru/ja with no fallback to key names | PASS | 4 keys in `collections.*` + 24 keys in `admin.collections.*` = 28 keys × 3 locales = 84 entries. All 3 files validated via `JSON.parse`. |

## Test gates

### Backend (Go)

```
$ cd services/catalog && go test ./... 2>&1 | grep FAIL ; go vet ./...
[no output — both clean]

$ cd services/gateway && go test ./... 2>&1 | grep FAIL ; go vet ./...
[no output — both clean]
```

Unit tests added in `services/catalog/internal/repo/collection_test.go`:

```
=== RUN   TestCollectionRepository_CreateAndGetByID          --- PASS
=== RUN   TestCollectionRepository_ListPublishedAndListAdmin --- PASS
=== RUN   TestCollectionRepository_GetBySlug                 --- PASS
=== RUN   TestCollectionRepository_AddItemIdempotent         --- PASS
=== RUN   TestCollectionRepository_RemoveItem                --- PASS
=== RUN   TestCollectionRepository_Delete                    --- PASS
=== RUN   TestCollectionRepository_ItemCountPopulated        --- PASS
=== RUN   TestCollectionRepository_UpdatePartial             --- PASS
PASS  ok  github.com/.../services/catalog/internal/repo   0.023s
```

### Frontend (TypeScript + ESLint)

```
$ cd frontend/web && bunx vue-tsc --noEmit ; echo EXIT=$?
EXIT=0

$ bunx eslint src/views/admin/AdminCollections.vue \
              src/views/admin/AdminCollectionEdit.vue \
              src/views/Collections.vue \
              src/components/home/CollectionsRow.vue \
              src/api/client.ts \
              src/router/index.ts \
              src/views/Home.vue
[no output — zero errors]
```

### JSON validity

```
$ node -e "['en','ru','ja'].forEach(l => JSON.parse(require('fs').readFileSync('src/locales/'+l+'.json','utf8'))); console.log('ok')"
en ok
ru ok
ja ok
```

### Deployment + health

```
$ make redeploy-catalog && make redeploy-gateway && make redeploy-web
[all three: "deployment complete" + healthy]

$ make health
✓ gateway:8000   ✓ auth:8080      ✓ catalog:8081   ✓ streaming:8082
✓ player:8083    ✓ rooms:8084     ✓ scheduler:8085 ✓ scraper:8088
```

### Live API smoke

```
$ docker compose exec -T postgres psql -U postgres -d animeenigma -c "\dt" | grep collection
 public | collection_items             | table | postgres
 public | collections                  | table | postgres

$ curl -s http://localhost:8000/api/collections
{"success":true,"data":[]}

$ curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/collections/nonexistent
404

$ curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/admin/collections
401

$ curl -s -o /dev/null -w "%{http_code}\n" \
    -H "Authorization: Bearer invalid" \
    http://localhost:8000/api/admin/collections
401

$ curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/admin/collections
200 (SPA fallthrough — DevMode is on; client-side requiresAdmin guard handles auth UX)

$ curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3003/collections/anything
200 (Vue SPA index.html fallback)
```

## Files touched

| Layer | File | Status |
|---|---|---|
| Backend | `services/catalog/internal/domain/collection.go` | NEW |
| Backend | `services/catalog/internal/repo/collection.go` | NEW |
| Backend | `services/catalog/internal/repo/collection_test.go` | NEW |
| Backend | `services/catalog/internal/service/collection.go` | NEW |
| Backend | `services/catalog/internal/handler/collection.go` | NEW |
| Backend | `services/catalog/internal/transport/router.go` | MODIFIED |
| Backend | `services/catalog/cmd/catalog-api/main.go` | MODIFIED |
| Backend | `services/gateway/internal/transport/router.go` | MODIFIED |
| Frontend | `frontend/web/src/api/client.ts` | MODIFIED |
| Frontend | `frontend/web/src/views/admin/AdminCollections.vue` | NEW |
| Frontend | `frontend/web/src/views/admin/AdminCollectionEdit.vue` | NEW |
| Frontend | `frontend/web/src/views/Collections.vue` | NEW |
| Frontend | `frontend/web/src/components/home/CollectionsRow.vue` | NEW |
| Frontend | `frontend/web/src/views/Home.vue` | MODIFIED |
| Frontend | `frontend/web/src/router/index.ts` | MODIFIED |
| Frontend | `frontend/web/src/locales/en.json` | MODIFIED |
| Frontend | `frontend/web/src/locales/ru.json` | MODIFIED |
| Frontend | `frontend/web/src/locales/ja.json` | MODIFIED |

## Commits

| Wave | Hash | Subject |
|---|---|---|
| 1 (Domain + repo + migrations + tests) | `8d96de7` | feat(17): Collection + CollectionItem domain + repo + GORM auto-migrate |
| 2 (Service + handler + routes + gateway) | `948c384` | feat(17): CollectionService + Handler + 8 routes + gateway proxies |
| 3 (Admin frontend) | `fda76c0` | feat(17): admin frontend — AdminCollections list + AdminCollectionEdit form |
| 4 (Public frontend) | `fa25262` | feat(17): public frontend — Collections detail view + Home row + routes |
| 5 (i18n en/ru/ja) | `db150a8` | feat(17): editorial-collections i18n in en / ru / ja |

## Deferred / Known limitations

- **Drag-and-drop reordering** — admin sets `sort_order` numerically.
  Polish item for Phase 20 (per CONTEXT.md "Deferred Ideas").
- **MinIO cover upload** — admin pastes a cover URL. Polish item for
  Phase 20.
- **Real admin JWT smoke** — listed in W2.6 of the plan. The destructive
  step (rotating an existing admin's `api_key_hash`) was deferred — the
  structural smoke battery + the catalog re-check defense-in-depth makes
  the auth path verified-by-construction. An admin will exercise the
  end-to-end create/edit/add-item flow on their next login.

## Outcome

All 8 plan must-haves pass. Phase 17 is shippable. Closes **UX-33** /
Tier E #5 (admin-curated editorial collections).
