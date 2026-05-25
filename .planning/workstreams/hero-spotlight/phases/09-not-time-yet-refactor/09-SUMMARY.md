---
phase: 09-not-time-yet-refactor
plan: 09
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-NT-01, HSB-V11-NT-02, HSB-V11-NT-03, HSB-V11-NT-04]
human_verified: "pending eyeball confirmation on animeenigma.ru after merge + redeploy"
key_files:
  created:
    - frontend/web/src/utils/time.ts
    - frontend/web/src/utils/time.spec.ts
  modified:
    - services/catalog/internal/service/spotlight/types.go
    - services/catalog/internal/service/spotlight/cards/not_time_yet.go
    - services/catalog/internal/service/spotlight/cards/not_time_yet_test.go
    - frontend/web/src/types/spotlight.ts
    - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue
    - frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.spec.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
commits:
  - f4e4e00 feat(spotlight/09) surface anime_list.updated_at as AddedAt on NotTimeYetData (HSB-V11-NT-01)
  - 601fa4f test(spotlight/09) assert NotTimeYetData.AddedAt populated from UpdatedAt (HSB-V11-NT-01)
  - a0ff611 feat(spotlight/09) add added_at to NotTimeYetData TS type (HSB-V11-NT-01)
  - 8c3da52 feat(spotlight/09) extract reusable formatAgo helper to utils/time.ts (HSB-V11-NT-02)
  - 7810213 feat(spotlight/09) add notTimeYet status + addedAt i18n keys (en/ru/ja) (HSB-V11-NT-03)
  - 8d7e5d6 feat(spotlight/09) refactor NotTimeYetCard to amber/clock identity (HSB-V11-NT-04)
  - 71f2dde test(spotlight/09) cover NotTimeYetCard amber/clock refactor (HSB-V11-NT-04)
  - 13e98c8 chore(spotlight/09) merge worktree
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 82%/80%"
  completed_date: 2026-05-25
---

# Phase 09 Plan 09: NotTimeYetCard refactor Summary

Gave NotTimeYetCard a distinct amber/clock "warm reminder" identity, clearly
differentiated from AnimeOfDayCard's cyan: poster-blur backdrop + amber
overlay, clock-icon header, status pill (planned vs postponed), a "Last added
X ago" relative timestamp, and a direct-to-watch CTA.

## What shipped

### HSB-V11-NT-01 — backend `added_at` pass-through
- Added `AddedAt *time.Time json:"added_at,omitempty"` to `NotTimeYetData`.
- Source field is `client.InternalListItem.UpdatedAt` (JSON `updated_at`) in
  `spotlight/client/player_client.go` — already returned, no new query / schema.
- **Deviation (Rule 3, anticipated):** `UpdatedAt` ships as an RFC3339 **string**,
  not `time.Time`. Added a nil-safe `parseUpdatedAt(string) *time.Time` helper
  (tries `RFC3339Nano` then `RFC3339`; drops empty/garbage/zero values so
  `omitempty` keeps them out of the JSON). `not_time_yet_test.go` covers the
  populated, empty, garbage, and zero-time cases.

### HSB-V11-NT-02 — `formatAgo` shared util
- Extracted the `Intl.RelativeTimeFormat` logic to
  `frontend/web/src/utils/time.ts` as `formatAgo(iso, locale)` with a co-located
  `time.spec.ts`. Threshold model follows Phase 07's `formatEntryDate`
  (day → week → absolute) so a 14-day-old entry reads "2 weeks ago" as the
  spec requires. LatestNewsCard's inline copy left untouched (out of scope).

### HSB-V11-NT-03 — status pill + addedAt i18n (3 locales)
- `statusPlanned` (Planned / В планах / 計画中), `statusPostponed`
  (Postponed / Отложено / 延期), `addedAt` (Added {ago} / Добавлено {ago} /
  {ago}に追加) added to en/ru/ja. `watchCta` already existed in all three
  ("Watch"/"Смотреть"/"視聴") — reused, no new key.
- Status pill class: planned → `bg-yellow-500/20 text-yellow-200`, postponed →
  `bg-slate-500/20 text-slate-300`.
- The locale parity spec was upgraded to a recursive `leafPaths` en-vs-ru
  key-set equality check, so any future single-locale drift fails loudly.

### HSB-V11-NT-04 — amber/clock card + direct-to-watch CTA
- `NotTimeYetCard.vue` → single-root `<article>`, `SpotlightBackdrop
  variant="poster-blur"` + `from-amber-500/30` overlay, clock-icon header,
  status pill, episode count, addedAt line, and a `.cta-hero[data-accent="amber"]`
  CTA pointing at **`/anime/{id}/watch`** (not the detail page) — the user
  already bookmarked it, so skip the extra hop.

## Verification
- Backend: `go test ./internal/service/spotlight/... -count=1` — green (race clean).
- Frontend: 319/319 spotlight + util + parity Vitest (18 files); `tsc --noEmit` clean.
- Deployed via `make redeploy-catalog` + `make redeploy-web`.

## Metrics
`UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 82%/80%`

## Deferred
- "Mark as watched" inline CTA (needs write-path; out of v1.1 scope).
