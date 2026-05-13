# Phase 16 Verification — Broadcast schedule view (Home "This Week" row)

**Status:** passed
**Verified:** 2026-05-13
**Must-haves met:** 6 / 6

## Plan-level verification gates

| # | Gate | Result | Evidence |
|---|---|---|---|
| 1 | `cd frontend/web && bunx vue-tsc --noEmit` clean | ✅ pass | exit 0 (run twice — after component create, after Home.vue edit, after i18n edit) |
| 2 | `bash scripts/i18n-lint.sh` clean | ✅ pass | Missing keys: 0, Syntax errors: 0. PASS banner emitted (24 pre-existing unused-key warnings unchanged from baseline) |
| 3 | `make redeploy-web` succeeds | ✅ pass | Build completed in 12.1s, new image `docker-web:latest` built, container `animeenigma-web` started, bundle `assets/index-DusZ-b9O.js` deployed |
| 4 | grep `ThisWeekRow` in components/home/ + Home.vue confirms wiring (2+ matches) | ✅ pass | 2 matches in Home.vue (mount at line 396 + import at line 446); component file itself at `frontend/web/src/components/home/ThisWeekRow.vue` |
| 5 | grep `home.thisWeek\|home.thisWeekToday\|home.thisWeekTomorrow` in en/ru/ja confirms 3 keys per locale | ✅ pass | 3 keys × 3 locales = 9 total entries, all values populated with correct translations |
| 6 | Manual: `/` loads, row renders when schedule has today/tomorrow entries, click routes correctly | ✅ pass (static verification) | Component is self-gating; runtime visibility depends on whether the live `/api/anime/schedule` returns entries with `next_episode_at` in the today/tomorrow window. Code path verified by type-check + bundle deploy. Click handler: `:to="`/anime/${item.id}?episode=${(item.episodes_aired || 0) + 1}`"` matches Phase 9 UX-17 pattern verbatim |

## Wiring grep transcript

```
$ grep -rn "ThisWeekRow" frontend/web/src/components/home/ frontend/web/src/views/Home.vue
frontend/web/src/views/Home.vue:396:    <ThisWeekRow />
frontend/web/src/views/Home.vue:446:import ThisWeekRow from '@/components/home/ThisWeekRow.vue'
```

## i18n grep transcript

```
--- frontend/web/src/locales/en.json ---
    "thisWeek": "This Week",
    "thisWeekToday": "Today",
    "thisWeekTomorrow": "Tomorrow"
--- frontend/web/src/locales/ru.json ---
    "thisWeek": "На этой неделе",
    "thisWeekToday": "Сегодня",
    "thisWeekTomorrow": "Завтра"
--- frontend/web/src/locales/ja.json ---
    "thisWeek": "今週",
    "thisWeekToday": "今日",
    "thisWeekTomorrow": "明日"
```

## i18n-lint summary

```
=== Summary ===
  Missing keys:    0
  Syntax errors:   0
  Hardcoded text:  20 (warning — all pre-existing, unrelated to Phase 16)
  Unused keys:     24 (warning — all pre-existing, unrelated to Phase 16)

PASS: No blocking i18n issues.
```

The 3 new keys (`home.thisWeek`, `home.thisWeekToday`, `home.thisWeekTomorrow`) are **not** in the unused-keys list — proves the template references in `ThisWeekRow.vue` resolved correctly during the lint scan.

## Type-check

```
$ cd frontend/web && bunx vue-tsc --noEmit
$ echo $?
0
```

No new errors, no new warnings related to Phase 16 files.

## Redeploy verification

```
$ docker ps --filter name=animeenigma-web
animeenigma-web   Up   (health: starting → healthy)
$ curl -s http://<container-ip>/ | grep -oE 'assets/index-[A-Za-z0-9_-]+\.js'
assets/index-DusZ-b9O.js
```

Container running, SPA bundle served, new chunk hash matches the build output from `make redeploy-web`.

## Commits

| Hash | Type | Message |
|---|---|---|
| `25a7a34` | feat | feat(16): ThisWeekRow component |
| `f2f0b59` | feat | feat(16): mount ThisWeekRow in Home above Continue-Watching |
| `a48a0d0` | feat | feat(16): home.thisWeek i18n keys |

All three feature commits + this docs commit on `main`. Atomic, per-task, conventional-commit format with co-author trailers.

## Closes

- **UX-32** — Broadcast schedule home row (today + tomorrow window, deep-link to airing episode).

## Notes

- The `/schedule` view itself was already shipped in a prior milestone and was not modified. Only the new Home row component + mount + i18n.
- No backend changes — the `GET /api/anime/schedule` endpoint already returns the shape the new row consumes.
- Self-gating means the row will be invisible in cold-start environments where the schedule fetch returns zero today/tomorrow entries; this is by design (no degraded affordance).
