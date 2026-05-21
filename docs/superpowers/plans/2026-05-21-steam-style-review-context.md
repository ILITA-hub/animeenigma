# Steam-style review context — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show live episode progress + watch status on every review card so users can spot drive-by reviewers at a glance, Steam-style.

**Architecture:** Expose existing `anime_list.status` and `anime_list.episodes` columns through the `reviewResponse` wire shape — no new DB columns, no migration, no snapshot drift. Frontend renders a compact metadata line inline with the date: `May 20 · 📺 3/12 · Watching`. Edge cases (`plan_to_watch`, `episodes == 0`) get a `⚠️` flag and amber tint.

**Tech Stack:** Go 1.21+ (services/player), GORM, chi router, sqlite-in-memory for handler tests · Vue 3 + TypeScript (frontend/web), vue-i18n, Tailwind · existing `watchlist.*` i18n keys reused for status labels.

**Spec:** `docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md`

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `services/player/internal/handler/review.go` | Modify | Extend `reviewResponse` struct + `toReviewResponse` projection with `status` and `episodes` fields; add TODO link-back comments |
| `services/player/internal/handler/review_shape_test.go` | Modify | Promote `status` + `episodes` from `forbiddenLeakKeys` to `allowedReviewKeys`; add positive assertions that they appear in projected responses |
| `frontend/web/src/views/Anime.vue` | Modify | Extend `Review` TS interface (lines 1063–1071) with `episodes`, `status`, `anime?`; add `formatReviewStats()` helper; replace the single-date `<p>` (line 755) with the inline date+stats line |
| `frontend/web/src/locales/en.json` | Modify | Add `anime.reviewStats.watched` / `watchedOpen` / `noProgress` keys |
| `frontend/web/src/locales/ru.json` | Modify | Same keys, RU |
| `frontend/web/src/locales/ja.json` | Modify | Same keys, JA |

After all tasks complete, the mandatory `/animeenigma-after-update` skill runs the redeploy + changelog + commit/push cycle (per CLAUDE.md).

---

## Task 1: Update backend test to assert the new 9-field shape (TDD red)

**Files:**
- Modify: `services/player/internal/handler/review_shape_test.go`

The current test asserts a 7-scalar-plus-`anime` wire shape and lists `status` + `episodes` in `forbiddenLeakKeys`. We move both from forbidden to allowed, and add a positive assertion that they actually appear in the projected response (not just "are permitted").

- [ ] **Step 1: Update `allowedReviewKeys` and `forbiddenLeakKeys` and rename the three test functions**

Edit `services/player/internal/handler/review_shape_test.go`:

Replace the doc-comment block at the top (lines 3–8) with:

```go
// Tests for Phase 1 (workstream: social) plan 02 — SOCIAL-NF-01 contract:
// the review endpoints' JSON wire shape must be EXACTLY the 9 canonical
// scalars + optional `anime` preload, even though the underlying
// `AnimeListEntry` row has many more fields (notes, tags, mal_id,
// is_rewatching, priority, started_at, completed_at, updated_at). The
// handler-local `reviewResponse` projection struct is what enforces this;
// these tests assert the projection is in place.
//
// 2026-05-21: `status` + `episodes` promoted from forbidden to allowed as
// part of the Steam-style review-context feature. See
// docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md.
```

Replace `allowedReviewKeys` (lines 115–124) with:

```go
var allowedReviewKeys = map[string]bool{
	"id":          true,
	"user_id":     true,
	"anime_id":    true,
	"username":    true,
	"score":       true,
	"review_text": true,
	"created_at":  true,
	"status":      true, // Steam-style review-context (2026-05-21)
	"episodes":    true, // Steam-style review-context (2026-05-21)
	"anime":       true,
}
```

Replace `forbiddenLeakKeys` (lines 129–132) with:

```go
var forbiddenLeakKeys = []string{
	"notes", "tags", "mal_id",
	"is_rewatching", "priority", "started_at", "completed_at", "updated_at",
}
```

Replace `assertReviewShape` (lines 150–160) with a version that also asserts `status` + `episodes` are PRESENT:

```go
// assertReviewShape asserts a single JSON object has only the canonical
// review keys (any subset of allowedReviewKeys), none of the forbidden
// leak keys, AND the two Steam-style context fields (status, episodes)
// are present in the body — proves the projection actually populates them.
func assertReviewShape(t *testing.T, obj map[string]json.RawMessage) {
	t.Helper()
	for k := range obj {
		assert.Truef(t, allowedReviewKeys[k],
			"unexpected key %q in review response — projection must strip AnimeListEntry-only fields", k)
	}
	for _, k := range forbiddenLeakKeys {
		_, present := obj[k]
		assert.Falsef(t, present, "forbidden key %q leaked into review response", k)
	}
	_, hasStatus := obj["status"]
	assert.Truef(t, hasStatus, "review response must include `status` for Steam-style context")
	_, hasEpisodes := obj["episodes"]
	assert.Truef(t, hasEpisodes, "review response must include `episodes` for Steam-style context")
}
```

Rename the three test functions (lines 165, 194, 216) from `…_ShapeIsExactly7Fields` to `…_ShapeIsCanonical` so the name doesn't lie about the field count once we add more in the future:

```go
func TestReviewHandler_GetAnimeReviews_ShapeIsCanonical(t *testing.T) {
```

```go
func TestReviewHandler_CreateOrUpdateReview_ShapeIsCanonical(t *testing.T) {
```

```go
func TestReviewHandler_GetUserReview_ShapeIsCanonical(t *testing.T) {
```

- [ ] **Step 2: Run the tests and verify they FAIL**

Run:

```bash
cd /data/animeenigma && go test ./services/player/internal/handler/ -run TestReviewHandler -v 2>&1 | tail -40
```

Expected output (3 failures): each test should fail with messages like:
```
--- FAIL: TestReviewHandler_GetAnimeReviews_ShapeIsCanonical
    review_shape_test.go: review response must include `status` for Steam-style context
    review_shape_test.go: review response must include `episodes` for Steam-style context
```

This confirms the test is correctly red — the handler's current 7-field projection doesn't yet emit `status` / `episodes`.

- [ ] **Step 3: Commit the failing test (red phase)**

```bash
cd /data/animeenigma
git add services/player/internal/handler/review_shape_test.go
git commit -m "$(cat <<'EOF'
test(player/review): assert Steam-style status+episodes in wire shape

Promote `status` and `episodes` from forbiddenLeakKeys to allowedReviewKeys
and add positive assertions that the handler projection populates them.
Tests are RED until the handler change in the next commit.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 2: Extend `reviewResponse` wire shape (TDD green)

**Files:**
- Modify: `services/player/internal/handler/review.go`

- [ ] **Step 1: Add `Status` and `Episodes` fields to the struct + projection**

In `services/player/internal/handler/review.go`, replace the `reviewResponse` struct (lines 22–31) with:

```go
type reviewResponse struct {
	ID         string            `json:"id"`
	UserID     string            `json:"user_id"`
	AnimeID    string            `json:"anime_id"`
	Username   string            `json:"username"`
	Score      int               `json:"score"`
	ReviewText string            `json:"review_text"`
	CreatedAt  time.Time         `json:"created_at"`
	// Status and Episodes — Steam-style review context (2026-05-21). Live
	// values from anime_list.status / anime_list.episodes, NOT snapshotted.
	// If the reviewer keeps watching after publishing, these numbers update.
	//
	// TODO(rewatch): surface rewatch context on review cards. AnimeListEntry
	// has is_rewatching (bool) and WatchProgress has watch_count (1 = first
	// watch, 2+ = rewatch). Future enhancement should render "🔁 On rewatch"
	// as a 4th segment. See
	// docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md.
	//
	// TODO(passive-watcher): fix the false-negative ⚠️ for users who watch
	// without updating their list. Replace `episodes` source with
	// max(anime_list.episodes, COUNT DISTINCT episode_number in
	// watch_history WHERE completed=true) — adds a subquery per render.
	// Same spec link as above.
	Status   string            `json:"status"`
	Episodes int               `json:"episodes"`
	Anime    *domain.AnimeInfo `json:"anime,omitempty"`
}
```

Replace `toReviewResponse` (lines 35–46) with:

```go
func toReviewResponse(e *domain.AnimeListEntry) reviewResponse {
	return reviewResponse{
		ID:         e.ID,
		UserID:     e.UserID,
		AnimeID:    e.AnimeID,
		Username:   e.Username,
		Score:      e.Score,
		ReviewText: e.ReviewText,
		CreatedAt:  e.CreatedAt,
		Status:     e.Status,
		Episodes:   e.Episodes,
		Anime:      e.Anime,
	}
}
```

- [ ] **Step 2: Run the tests and verify they PASS**

Run:

```bash
cd /data/animeenigma && go test ./services/player/internal/handler/ -run TestReviewHandler -v 2>&1 | tail -20
```

Expected: 3 PASS lines:
```
--- PASS: TestReviewHandler_GetAnimeReviews_ShapeIsCanonical
--- PASS: TestReviewHandler_CreateOrUpdateReview_ShapeIsCanonical
--- PASS: TestReviewHandler_GetUserReview_ShapeIsCanonical
PASS
ok  	github.com/ILITA-hub/animeenigma/services/player/internal/handler	...
```

- [ ] **Step 3: Run the full player-service test suite to confirm no regressions**

Run:

```bash
cd /data/animeenigma && go test ./services/player/... 2>&1 | tail -10
```

Expected: all packages PASS. If `services/player/internal/service/review_test.go` or `services/player/internal/repo/list_review_test.go` fail because they hard-coded the old field set, update them in the SAME commit — but on inspection of those files they assert business behavior, not the wire shape, so they should still pass unchanged.

- [ ] **Step 4: Commit (green phase)**

```bash
cd /data/animeenigma
git add services/player/internal/handler/review.go
git commit -m "$(cat <<'EOF'
feat(player/review): expose live status + episodes on review wire shape

Steam-style review context — show what the reviewer was up to when they
hit publish. No new columns, no snapshot: live values from the same
anime_list row that absorbs the review.

Includes TODO comments referencing the design spec for two future
enhancements (rewatch badge, watch_history fallback for passive
watchers) so the next implementer has the full context.

Spec: docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 3: Add i18n strings for the review-stats badge

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

We need 3 new keys under `anime.reviewStats`. Status labels reuse existing `watchlist.*` keys; we don't duplicate them. We do `en` first so the TS lookup in Task 4 has something to read against during development.

- [ ] **Step 1: Find an existing `anime.*` block to insert under**

Run:

```bash
cd /data/animeenigma && grep -n '"reviewsCount"\|"writeReview"\|"editReview"' frontend/web/src/locales/en.json
```

Expected: a small cluster of keys under `"anime": { ... }`. Insert the new `reviewStats` block adjacent to those.

- [ ] **Step 2: Add `anime.reviewStats` block to `frontend/web/src/locales/en.json`**

Find the existing `"writeReview"` key and add a new `reviewStats` object immediately AFTER it (still inside the `"anime": {}` block):

```json
"reviewStats": {
  "watched": "📺 {watched}/{total} · {status}",
  "watchedOpen": "📺 {watched} eps · {status}",
  "noProgress": "📺 0/{total} · {status} ⚠️",
  "noProgressOpen": "📺 0 eps · {status} ⚠️",
  "planToWatchFlag": "📺 {watched}/{total} · {status} ⚠️",
  "planToWatchOpenFlag": "📺 {watched} eps · {status} ⚠️"
},
```

- [ ] **Step 3: Add the same block to `frontend/web/src/locales/ru.json`**

In `frontend/web/src/locales/ru.json`, immediately after the matching `writeReview` (or equivalent) key inside the `"anime"` block:

```json
"reviewStats": {
  "watched": "📺 {watched}/{total} · {status}",
  "watchedOpen": "📺 {watched} сер. · {status}",
  "noProgress": "📺 0/{total} · {status} ⚠️",
  "noProgressOpen": "📺 0 сер. · {status} ⚠️",
  "planToWatchFlag": "📺 {watched}/{total} · {status} ⚠️",
  "planToWatchOpenFlag": "📺 {watched} сер. · {status} ⚠️"
},
```

- [ ] **Step 4: Add the same block to `frontend/web/src/locales/ja.json`**

```json
"reviewStats": {
  "watched": "📺 {watched}/{total}話 · {status}",
  "watchedOpen": "📺 {watched}話 · {status}",
  "noProgress": "📺 0/{total}話 · {status} ⚠️",
  "noProgressOpen": "📺 0話 · {status} ⚠️",
  "planToWatchFlag": "📺 {watched}/{total}話 · {status} ⚠️",
  "planToWatchOpenFlag": "📺 {watched}話 · {status} ⚠️"
},
```

- [ ] **Step 5: Validate JSON parses (catches missing commas etc.)**

Run:

```bash
cd /data/animeenigma && for f in frontend/web/src/locales/{en,ru,ja}.json; do echo "== $f =="; python3 -c "import json; json.load(open('$f')); print('OK')"; done
```

Expected: three `OK` lines. Fix any `JSONDecodeError` before continuing.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "$(cat <<'EOF'
i18n(anime/reviews): add reviewStats strings for episode+status badge

EN/RU/JA strings for the inline Steam-style review-context line. Status
labels reuse existing watchlist.* keys; these strings cover the four
rendering modes (closed series, open series, no-progress flag,
plan-to-watch flag).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 4: Frontend — extend `Review` interface and add the badge to the review card

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

Two changes in the same file: type extension (lines ~1063–1071) and template change at the review card (lines ~755).

- [ ] **Step 1: Extend the `Review` TypeScript interface**

In `frontend/web/src/views/Anime.vue`, replace the `Review` interface (lines 1063–1071) with:

```ts
interface Review {
  id: string
  user_id: string
  anime_id: string
  username: string
  score: number
  review_text: string
  created_at: string
  // Steam-style review context (2026-05-21). Live values from anime_list
  // row — NOT snapshotted at review time. `anime` carries episodes_count
  // for the "watched / total" rendering; backend preloads it.
  status?: string
  episodes?: number
  anime?: {
    episodes_count?: number
  }
}
```

- [ ] **Step 2: Add a `formatReviewStats` helper function in `<script setup>`**

Locate the existing `formatDate` helper (around line 1470) and add `formatReviewStats` immediately after it:

```ts
const formatReviewStats = (review: Review): string => {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  const total = review.anime?.episodes_count ?? 0

  // Map raw status enum -> existing watchlist.* i18n keys.
  const statusKeyMap: Record<string, string> = {
    watching: 'watchlist.watching',
    completed: 'watchlist.completed',
    on_hold: 'watchlist.onHold',
    dropped: 'watchlist.dropped',
    plan_to_watch: 'watchlist.planToWatch',
  }
  const statusLabel = t(statusKeyMap[status] || statusKeyMap.watching)

  // Pick template variant: closed (total known) vs open (total unknown);
  // flagged (plan_to_watch OR episodes==0) vs normal.
  const flagged = status === 'plan_to_watch' || episodes === 0
  const open = total === 0

  let key: string
  if (flagged && status === 'plan_to_watch' && open) {
    key = 'anime.reviewStats.planToWatchOpenFlag'
  } else if (flagged && status === 'plan_to_watch') {
    key = 'anime.reviewStats.planToWatchFlag'
  } else if (flagged && open) {
    key = 'anime.reviewStats.noProgressOpen'
  } else if (flagged) {
    key = 'anime.reviewStats.noProgress'
  } else if (open) {
    key = 'anime.reviewStats.watchedOpen'
  } else {
    key = 'anime.reviewStats.watched'
  }

  return t(key, { watched: episodes, total, status: statusLabel })
}

const isReviewFlagged = (review: Review): boolean => {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  return status === 'plan_to_watch' || episodes === 0
}
```

The `t` symbol is already imported via the `useI18n()` destructure near line 1094 (`const { t, locale } = useI18n()`).

- [ ] **Step 3: Replace the date `<p>` in the review card with the date + stats line**

In `frontend/web/src/views/Anime.vue`, locate the review card date line (line 755):

```vue
<p class="text-white/60 text-sm">{{ formatDate(review.created_at) }}</p>
```

Replace it with:

```vue
<p class="text-white/60 text-sm">
  {{ formatDate(review.created_at) }}
  <template v-if="review.status">
    <span class="text-white/30 mx-1">·</span>
    <span :class="isReviewFlagged(review) ? 'text-amber-400' : 'text-white/60'">{{ formatReviewStats(review) }}</span>
  </template>
</p>
```

The `<template v-if="review.status">` guard means old reviews loaded from a stale cache (no `status` field) render the date alone, no badge — graceful degradation.

- [ ] **Step 4: Type-check the frontend**

Run:

```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit 2>&1 | tail -20
```

Expected: no errors. (If there are pre-existing errors unrelated to this change, ignore them — but confirm `Anime.vue` itself reports clean.)

- [ ] **Step 5: ESLint the file**

Run:

```bash
cd /data/animeenigma/frontend/web && bunx eslint src/views/Anime.vue 2>&1 | tail -10
```

Expected: clean (or only pre-existing warnings). Fix any new warnings the change introduced.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/views/Anime.vue
git commit -m "$(cat <<'EOF'
feat(anime/reviews): inline episode+status badge on each review card

Steam-style "X hrs on record" equivalent — each review card now shows
'May 20, 2026 · 📺 3/12 · Watching' so users can spot drive-by reviewers
at a glance. Plan-to-watch and episodes=0 cases get an amber ⚠️ flag.

Status labels reuse existing watchlist.* i18n keys; new strings live
under anime.reviewStats.*. The `review.status` template guard makes
stale-cache reviews degrade to just the date with no badge.

Spec: docs/superpowers/specs/2026-05-21-steam-style-review-context-design.md

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 5: Mandatory after-update — redeploy, changelog, push

The project's CLAUDE.md mandates `/animeenigma-after-update` after any implementation. It handles:

1. Lint + build the affected code
2. `make redeploy-player` (backend changed) + `make redeploy-web` (frontend changed)
3. Health checks
4. Update `frontend/web/public/changelog.json` with a user-facing entry in the project's enthusiastic + emoji tone
5. Commit the changelog and push everything to remote

- [ ] **Step 1: Invoke the after-update skill**

Run the skill:

```
/animeenigma-after-update
```

Provide it this hint when it asks for a summary:

> Steam-style review context: every review card now shows the reviewer's
> current episode progress (e.g. "📺 3/12") and watch status (Watching /
> Completed / Dropped / On Hold / Plan to Watch) inline with the date.
> Reviews from users who haven't started watching (or set status to "plan
> to watch") get an amber ⚠️ flag so it's obvious who's reviewing without
> watching.

Expected end state: player + web services redeployed and healthy, changelog updated, everything pushed to `origin/main`.

- [ ] **Step 2: Manual smoke verification**

Open the site at https://animeenigma.ru in a browser, navigate to an anime that has reviews, scroll to the reviews tab, and confirm at least one review card shows the new badge in the expected format. If reviewer state is `watching` with episodes > 0, badge should be white/grey. If `plan_to_watch` or `episodes == 0`, badge should be amber with a ⚠️.

If you don't have a reviewer fixture for both states handy, log in as `ui_audit_bot` (per CLAUDE.md test-user section) and write one review at `plan_to_watch` and one at `watching · ep 3` to verify both visual states render correctly.

---

## Acceptance checklist

After all tasks complete, the feature is shipped when:

- [ ] Backend wire shape includes `status` and `episodes` (verify: `curl -s https://animeenigma.ru/api/anime/<uuid>/reviews | jq '.data[0] | keys'`)
- [ ] `go test ./services/player/...` passes
- [ ] `bunx tsc --noEmit` in `frontend/web/` is clean for `Anime.vue`
- [ ] Browser shows the badge inline with the date on the anime reviews tab
- [ ] Amber ⚠️ renders for `plan_to_watch` reviewers and zero-episode reviewers
- [ ] White/grey badge renders for legitimate reviewers
- [ ] Old reviews loaded from stale cache (no `status` field) degrade gracefully to date-only
- [ ] Player + web services are healthy after redeploy (`make health`)
- [ ] Changelog has a user-facing entry
- [ ] Everything pushed to `origin/main`

## Future enhancements (do NOT do in this plan)

These are documented as code TODOs and in the spec — surface them when they come up but don't bundle them into this implementation:

1. **Rewatch badge** — render `is_rewatching` / `watch_count` as a 4th segment with positive treatment ("🔁 On rewatch")
2. **watch_history fallback** — fix the passive-watcher false-negative
3. **Per-card filter** — "Hide reviews from users with < N episodes watched" toggle above the reviews list
