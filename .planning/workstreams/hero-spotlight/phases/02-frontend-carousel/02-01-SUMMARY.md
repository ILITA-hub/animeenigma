---
phase: 02-frontend-carousel
plan: 01
subsystem: frontend
workstream: hero-spotlight
tags:
  - frontend
  - vue3
  - typescript
  - hero-spotlight
  - scaffolding
requirements:
  - HSB-FE-02
  - HSB-FE-09
provides:
  - "frontend/web/src/types/spotlight.ts → SpotlightCard discriminated union"
  - "frontend/web/src/composables/useSpotlight.ts → useSpotlight() composable"
  - "frontend/web/.env.example VITE_HERO_SPOTLIGHT_ENABLED feature flag documentation"
  - "@axe-core/playwright@^4.11.3 dev-dep (unblocks Plan 02-06 axe gate)"
requires:
  - "GET /api/home/spotlight (Phase 1 hero-spotlight workstream — already shipped)"
  - "frontend/web/src/api/client.ts apiClient (shared axios instance)"
affects:
  - "frontend/web/package.json (added 1 devDep)"
  - "frontend/web/bun.lock (regenerated)"
  - "frontend/web/.env (local-only; gitignored)"
tech-stack:
  added:
    - "@axe-core/playwright@4.11.3 (devDep)"
  patterns:
    - "Discriminated-union TypeScript types (`type` literal discriminator on each SpotlightCard variant)"
    - "Composable + apiClient.get() pattern mirroring useContinueWatching.ts"
    - "Silent-self-hide error handling (one console.warn, empty cards array)"
    - "Defensive envelope unwrap: (res.data?.data ?? res.data)"
    - "VITE_* feature flag with !=='false' default-true semantics"
key-files:
  created:
    - "frontend/web/src/types/spotlight.ts"
    - "frontend/web/src/composables/useSpotlight.ts"
    - "frontend/web/src/composables/useSpotlight.spec.ts"
  modified:
    - "frontend/web/.env.example"
    - "frontend/web/.env"
    - "frontend/web/package.json"
    - "frontend/web/bun.lock"
decisions:
  - "Types mirror LIVE backend payload exactly (snake_case, no envelope id, ChangelogEntry uses date/type/message NOT id/title/summary)"
  - "Did NOT forward-declare Phase 3 card variants — keeps cardFor() exhaustiveness honest"
  - "Composable performs single onMounted fetch only; auth-watcher + 30s poll deferred to Phase 3"
  - "Used bun add -d (project mandate) — bun.lock is text format, no bun.lockb in bun 1.3"
metrics:
  ux_delta: "+1 (Better)"
  cdi: "0.01 * 3"
  mvq: "Sprite 90%/85%"
  duration: "00:08:00 (≈8 minutes wall-clock)"
  tasks_completed: 3
  files_touched: 7
  completed: 2026-05-21T03:41:42Z
---

# Phase 2 Plan 01: Frontend Carousel Scaffolding Summary

> Type foundation, data composable, feature-flag env var, and a11y test dependency for the HeroSpotlightBlock carousel — invisible foundation that unblocks Plans 02-02 through 02-06.

## One-liner

Discriminated-union `SpotlightCard` type + `useSpotlight()` fetch composable + `VITE_HERO_SPOTLIGHT_ENABLED` flag + `@axe-core/playwright@4.11.3` devDep, all aligned to the live Phase 1 `/api/home/spotlight` runtime payload.

## Tasks Executed

| Task | Name | Type | Commit | Files |
|------|------|------|--------|-------|
| 1 | SpotlightCard discriminated union | auto | `dbdd15e` | `frontend/web/src/types/spotlight.ts` |
| 2 | useSpotlight composable + Vitest spec | auto (tdd) | `5e6fa33` | `frontend/web/src/composables/useSpotlight.ts`, `frontend/web/src/composables/useSpotlight.spec.ts` |
| 3 | Env flag + @axe-core/playwright install | auto | `c84ad7c` | `frontend/web/.env.example`, `frontend/web/.env`, `frontend/web/package.json`, `frontend/web/bun.lock` |

## Verification Output

All success-criteria commands exit 0:

```text
=== 1. tsc ===
tsc exit: 0

=== 2. eslint (new files only) ===
eslint exit: 0

=== 3. vitest ===
 RUN  v4.1.6 /data/animeenigma/frontend/web
 Test Files  1 passed (1)
      Tests  5 passed (5)
   Duration  772ms
vitest exit: 0

=== 4. axe-core dep ===
├── @axe-core/playwright@4.11.3

=== 5. env flags ===
1   (.env.example contains VITE_HERO_SPOTLIGHT_ENABLED)
1   (.env contains VITE_HERO_SPOTLIGHT_ENABLED)
```

Vitest cases — all 5 pass:

1. `fetches on mount and populates cards when API returns {cards, generated_at}`
2. `unwraps wrapped envelope {success, data:{cards, generated_at}}`
3. `returns empty cards on 5xx and emits one console.warn`
4. `returns empty cards when response body cards field is null`
5. `exposes refresh() that re-runs the fetch`

## Deviations from Plan

### Backend payload discrepancies discovered (Rule 1 — fix bug in plan spec)

The Plan instructed to curl-verify the live Phase 1 endpoint and fall back to RESEARCH.md Pattern 5 if not reachable. The endpoint WAS reachable, and the live payload diverged from the design doc in three ways. The type file was authored to the live shape (with TODO comments documenting each delta) so cards render correctly from day 1:

**1. [Rule 1 — Type/Schema] Cards have no envelope-level `id` field**

- **Live payload:** `{type, data}` — no `id`.
- **Design doc & RESEARCH.md Pattern 5 claim:** `{id, type, data}`.
- **Fix:** Removed `id: string` from each variant of the `SpotlightCard` union. Documented in the file header that Vue keying must fall back to `${type}:${index}` (matches RESEARCH.md Pitfall 10).
- **Files modified:** `frontend/web/src/types/spotlight.ts`
- **Commit:** `dbdd15e`

**2. [Rule 1 — Type/Schema] LatestNewsCard entries use `{date, type, message}`, not `{id, date, title, summary}`**

- **Live payload:** Each entry is `{date: "YYYY-MM-DD", type: "feature|fix|…", message: string}` — matches `frontend/web/public/changelog.json` exactly.
- **Design doc & Plan spec claim:** `{id, date, title, summary}`.
- **Fix:** `ChangelogEntry` interface aligned to live shape. Added inline TODO comment so Plan 02-05's `LatestNewsCard` author knows to render `message` as body (and treat its first sentence as a title fallback).
- **Files modified:** `frontend/web/src/types/spotlight.ts`
- **Commit:** `dbdd15e`

**3. [Rule 2 — Type completeness] SpotlightAnime extended to mirror the full catalog row**

- **Live payload's `anime` object** ships every catalog column (description, year, season, status, kind, rating, material_source, episodes_count, episodes_aired, episode_duration, score, shikimori_id, mal_id, has_* flags, hidden, aired_on, created_at, updated_at).
- **Plan spec asked for only:** `id`, `name`, `name_ru`, `name_jp`, `poster_url`, `score?`, `episodes?`, `genres?`.
- **Fix:** Declared all the extra fields as optional so card components have full type-safety and `name_en` is accessible (Plan 02-04's `AnimeOfDayCard` needs it for the English title locale). Plan-asked `episodes?` field is NOT in live payload — backend uses `episodes_count`/`episodes_aired`. Kept `episodes` in spec but added the backend names; card components should prefer `episodes_count`.
- **Files modified:** `frontend/web/src/types/spotlight.ts`
- **Commit:** `dbdd15e`

**4. [Rule 2 — Forward-compat] `PlatformMetric.key` is an open string union**

- **Live payload's metrics** in the snapshot showed only `[{key: "anime_added_7d", value: 3}]`. Backend may emit additional keys without coordinating a frontend bump.
- **Fix:** Typed `key` as the literal union of known keys `| string` so unknown keys still type-check; cards will fall back to rendering the raw key as label text.
- **Files modified:** `frontend/web/src/types/spotlight.ts`
- **Commit:** `dbdd15e`

### Spec file type-cast adjustment (Rule 1)

Initial Vitest spec used `wrapper.vm as unknown as ReturnType<typeof useSpotlight>` which made `vm.cards` typed as `Ref<SpotlightCard[]>`, causing TS7053 errors when accessing `.length` and `[0]`. Vue test-utils auto-unwraps refs on `wrapper.vm`, so the correct typing is `HarnessVm` with unwrapped fields (`cards: SpotlightCard[]`).

- **Found during:** Task 2 verification (tsc errors after GREEN phase)
- **Fix:** Introduced explicit `HarnessVm` interface (`cards: SpotlightCard[]`, `loading: boolean`, `error: Error | null`, `refresh: () => Promise<void>`) for the cast target. Also typed the harness component's `setup()` return as explicit Refs so vue-tsc tracks the unwrapping correctly.
- **Files modified:** `frontend/web/src/composables/useSpotlight.spec.ts`
- **Commit:** `5e6fa33` (rolled into the GREEN-phase commit; type errors were caught before commit)

### bun.lockb → bun.lock (Rule 3)

Plan's `<files>` listed `frontend/web/bun.lockb`. bun 1.3+ emits the text-based `bun.lock` (no `bun.lockb`). The text lock is what was updated and committed.

- **No file changed in plan needed.** Just a name discrepancy in the plan frontmatter — the project's actual lockfile is `bun.lock`.

## Authentication Gates

None encountered. `GET /api/home/spotlight` is public; composable tests stub `apiClient.get`; install via `bun add` ran cleanly.

## Threat Surface Notes

Reviewed against the plan's `<threat_model>`:

- **T-2-01 (Tampering — flag bypass):** Accepted as designed. `VITE_HERO_SPOTLIGHT_ENABLED` baked at build time. The .env.example comment explicitly documents that the backend `SPOTLIGHT_ENABLED=false` is the real off switch and this flag is a UI kill switch.
- **T-2-02 (DoS — 5xx):** Mitigation in place. `useSpotlight()` catches all throws, sets `cards.value = []`, emits one warn. Tested by spec case 3.
- **T-2-03 (Information disclosure — interceptor bypass):** Mitigation in place. Composable goes through `apiClient` (verified by acceptance grep: no `import axios`, no `fetch(`).

No new surface introduced — composable consumes an existing public endpoint via the existing client.

## Known Stubs

None. Every type field maps to a real backend column or is explicitly forward-compatible. The composable's `loading`, `error`, `refresh` refs all wire to real behavior (no placeholder no-ops).

## Follow-up Work (NOT in scope for this plan)

These belong to subsequent Phase 2 plans — flagged here for the planner:

1. **Plan 02-03 (HeroSpotlightBlock.vue):** When Vue-keying the active slide, use `${card.type}:${index}` since the envelope has no `id`. Cite `02-01-SUMMARY.md` deviation #1 in the comment.
2. **Plan 02-05 (LatestNewsCard.vue):** Render the changelog entry's `message` as body. Title-line extraction: split on the first sentence-ending punctuation or use the first 60 chars + ellipsis. Cite `02-01-SUMMARY.md` deviation #2.
3. **Plan 02-04 (AnimeOfDayCard.vue):** For episode counts use `episodes_count` (live payload field), not the deprecated `episodes` field from RESEARCH.md.
4. **Plan 02-06 (Playwright a11y gate):** `@axe-core/playwright@4.11.3` is now in devDependencies — import as `import AxeBuilder from '@axe-core/playwright'`.
5. **Optional backend follow-up:** If `latest_news` consumers ever need structured titles, add a `title?: string` field to the backend changelog entries (additive — won't break this frontend).

## Self-Check: PASSED

Files verified to exist on disk:

- FOUND: `frontend/web/src/types/spotlight.ts`
- FOUND: `frontend/web/src/composables/useSpotlight.ts`
- FOUND: `frontend/web/src/composables/useSpotlight.spec.ts`
- FOUND: `frontend/web/.env.example` (modified — VITE_HERO_SPOTLIGHT_ENABLED appended)
- FOUND: `frontend/web/.env` (modified — VITE_HERO_SPOTLIGHT_ENABLED appended; gitignored)
- FOUND: `frontend/web/package.json` (modified — @axe-core/playwright in devDeps)
- FOUND: `frontend/web/bun.lock` (regenerated)

Commits verified in git log:

- FOUND: `dbdd15e` — feat(02-01): add SpotlightCard discriminated union type
- FOUND: `5e6fa33` — feat(02-01): add useSpotlight composable + Vitest spec
- FOUND: `c84ad7c` — chore(02-01): add VITE_HERO_SPOTLIGHT_ENABLED + @axe-core/playwright dep
