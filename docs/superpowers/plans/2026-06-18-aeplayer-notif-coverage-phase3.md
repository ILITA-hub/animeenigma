# aePlayer Notification Coverage — Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `kodik` and `animelib` aePlayer watchers (who persist an empty `translation_id`) get new-episode notifications, via anime-level (any-team) latest-episode resolution — closing the last gap (excluding `hanime`).

**Architecture:** New pure-logic helpers + thin client methods give an any-team latest-episode count: `kodik.LatestEpisodeAnyTranslation(shikimoriID)` (max episode-count across ALL translations) and `animelib.LatestEpisodeAnyTeam(ctx, animelibID)` (max episode number across the full list). Catalog's `LatestAvailable` routes `kodik`/`animelib` combos with an **empty** `translation_id` to these (legacy translation-specific path unchanged for non-empty id). The internal handler drops the `translation_id`-required check; `hotcombos` admits these players with an empty id.

**Tech Stack:** Go (catalog), `go test`.

**Spec:** `docs/superpowers/specs/2026-06-18-aeplayer-notification-coverage-design.md` (Phase 3). Builds on Phases 1–2.

## Global Constraints

- **Work in a clean `origin/main` git worktree**, NOT the shared `/data/animeenigma` tree (stale/dirty). Controller provides the path. Commit there; controller pushes.
- **Commits:** path-scoped (`git commit <pathspec>`), never `git add -A`/bare. `git show --stat HEAD` after each. Do NOT push.
- **Co-authors on every commit:**
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Testing reality:** the kodik client's existing tests hit the LIVE Kodik API (`NewClient()` + real shikimori ids); animelib has no client test. Do NOT add live-API tests. Test the **pure helpers** (count/max) hermetically with synthetic structs — that is where the logic risk is. The thin HTTP wrappers are covered by the existing live tests + the Task 5 runtime smoke.
- **Error contract:** "no episodes" → a `NotFound`-class error whose message contains `no episode`/`not found`/`no episodes returned` (existing `isNotFoundLike` → `CodeNotFound` → detector silent-skip). Client/transport failures → other message → `CodeInternal`/`CodeUnavailable` → retried.
- **Anime-level semantics:** any-team latest is team- AND watch_type-agnostic (the count is "is there a newer episode of this anime on this source at all"). Deep-link `provider=kodik`/`animelib` (empty team) already pins the source in aePlayer (shipped earlier).
- **No schema change** (empty `translation_id` collapses to one snapshot row per `(anime, player, watch_type, language)`).

---

### Task 1: kodik — any-team latest-episode helper

**Files:**
- Modify: `services/catalog/internal/parser/kodik/latest_episode.go` (add `resultEpisodeCount` + `LatestEpisodeAnyTranslation`; refactor `LatestEpisodeForTranslation` to use the helper)
- Test: `services/catalog/internal/parser/kodik/latest_episode_test.go` (create — pure-helper tests, NO network)

**Interfaces:**
- Produces: `(*kodik.Client).LatestEpisodeAnyTranslation(shikimoriID string) (int, error)`; pure `resultEpisodeCount(r SearchResult) int`; pure `maxAnyTeamEpisode(results []SearchResult) int`.

`SearchResult` (confirmed) has: `LastEpisode int`, `EpisodesCount int`, `Seasons map[string]*Season`, `Type string`, `Translation *Translation`. The current `LatestEpisodeForTranslation` count precedence is: `LastEpisode` → else `EpisodesCount` → else sum of `len(season.Episodes)` over `Seasons` → else `1` if `Type=="anime"`.

- [ ] **Step 1: Write the failing pure-helper test**

Create `services/catalog/internal/parser/kodik/latest_episode_test.go`:
```go
package kodik

import "testing"

func TestResultEpisodeCount_Precedence(t *testing.T) {
	if got := resultEpisodeCount(SearchResult{LastEpisode: 12, EpisodesCount: 5}); got != 12 {
		t.Errorf("LastEpisode wins: got %d, want 12", got)
	}
	if got := resultEpisodeCount(SearchResult{EpisodesCount: 5}); got != 5 {
		t.Errorf("EpisodesCount fallback: got %d, want 5", got)
	}
	if got := resultEpisodeCount(SearchResult{Type: "anime"}); got != 1 {
		t.Errorf("anime min-1 fallback: got %d, want 1", got)
	}
	if got := resultEpisodeCount(SearchResult{}); got != 0 {
		t.Errorf("empty non-anime: got %d, want 0", got)
	}
}

func TestMaxAnyTeamEpisode(t *testing.T) {
	results := []SearchResult{
		{Translation: &Translation{ID: 1}, LastEpisode: 7},
		{Translation: &Translation{ID: 2}, LastEpisode: 12}, // a different team is further ahead
		{Translation: &Translation{ID: 3}, EpisodesCount: 9},
	}
	if got := maxAnyTeamEpisode(results); got != 12 {
		t.Errorf("maxAnyTeamEpisode = %d, want 12", got)
	}
	if got := maxAnyTeamEpisode(nil); got != 0 {
		t.Errorf("maxAnyTeamEpisode(nil) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run — verify it fails**

Run from the worktree: `cd services/catalog && go test ./internal/parser/kodik/ -run 'TestResultEpisodeCount|TestMaxAnyTeamEpisode' 2>&1 | tail -12`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Add the helpers + the public method; refactor the existing method**

In `services/catalog/internal/parser/kodik/latest_episode.go`, add:
```go
// resultEpisodeCount returns the episode count a single search result implies,
// using Kodik's field precedence: last_episode → episodes_count → summed
// season episodes → 1 for a bare anime entry.
func resultEpisodeCount(r SearchResult) int {
	count := r.LastEpisode
	if count == 0 {
		count = r.EpisodesCount
	}
	if count == 0 && r.Seasons != nil {
		for _, season := range r.Seasons {
			if season != nil && season.Episodes != nil {
				count += len(season.Episodes)
			}
		}
	}
	if count == 0 && r.Type == "anime" {
		count = 1
	}
	return count
}

// maxAnyTeamEpisode returns the highest episode count across ALL translations
// (team-agnostic) — the anime-level "latest episode" for notification detection.
func maxAnyTeamEpisode(results []SearchResult) int {
	best := 0
	for _, r := range results {
		if c := resultEpisodeCount(r); c > best {
			best = c
		}
	}
	return best
}

// LatestEpisodeAnyTranslation returns the latest episode available across ANY
// translation for the anime (used by the notifications detector for aePlayer
// kodik combos, which carry no specific translation_id). Returns 0 + nil when
// the anime has no kodik results (caller maps that to NotFound/skip).
func (c *Client) LatestEpisodeAnyTranslation(shikimoriID string) (int, error) {
	results, err := c.SearchByShikimoriID(shikimoriID)
	if err != nil {
		return 0, fmt.Errorf("kodik: search by shikimori_id %q: %w", shikimoriID, err)
	}
	return maxAnyTeamEpisode(results), nil
}
```
Then refactor the count block inside `LatestEpisodeForTranslation` (the `count := r.LastEpisode … if count == 0 && r.Type == "anime" { count = 1 }` sequence) to a single `count := resultEpisodeCount(r)` — behavior-identical, covered by its existing live test.

- [ ] **Step 4: Run the pure tests — verify they pass; build**

Run: `cd services/catalog && go test ./internal/parser/kodik/ -run 'TestResultEpisodeCount|TestMaxAnyTeamEpisode' -v 2>&1 | tail -12 && go build ./internal/parser/kodik/`
Expected: PASS; build OK. (Do NOT run the whole kodik package — it makes live API calls.)

- [ ] **Step 5: Commit**

```bash
git commit services/catalog/internal/parser/kodik/latest_episode.go services/catalog/internal/parser/kodik/latest_episode_test.go \
  -m "feat(kodik): LatestEpisodeAnyTranslation (team-agnostic max) + extract resultEpisodeCount

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: animelib — any-team latest-episode helper

**Files:**
- Modify: `services/catalog/internal/parser/animelib/latest_episode.go` (add `maxAnimeLibEpisode` + `LatestEpisodeAnyTeam`)
- Test: `services/catalog/internal/parser/animelib/latest_episode_test.go` (create — pure-helper test, NO network)

**Interfaces:**
- Produces: `(*animelib.Client).LatestEpisodeAnyTeam(ctx context.Context, animelibID int) (int, error)`; pure `maxAnimeLibEpisode(eps []Episode) int`.

`animelib.Episode` has `Number string` (confirmed). `GetEpisodes(animeID int) ([]Episode, error)` returns the full team-agnostic episode list.

- [ ] **Step 1: Write the failing pure-helper test**

Create `services/catalog/internal/parser/animelib/latest_episode_test.go`:
```go
package animelib

import "testing"

func TestMaxAnimeLibEpisode(t *testing.T) {
	eps := []Episode{{Number: "1"}, {Number: "12"}, {Number: "7"}}
	if got := maxAnimeLibEpisode(eps); got != 12 {
		t.Errorf("maxAnimeLibEpisode = %d, want 12", got)
	}
	// Non-numeric / fractional numbers are skipped, not fatal.
	if got := maxAnimeLibEpisode([]Episode{{Number: "3"}, {Number: "3.5"}, {Number: "abc"}}); got != 3 {
		t.Errorf("maxAnimeLibEpisode (mixed) = %d, want 3", got)
	}
	if got := maxAnimeLibEpisode(nil); got != 0 {
		t.Errorf("maxAnimeLibEpisode(nil) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run — verify it fails**

Run: `cd services/catalog && go test ./internal/parser/animelib/ -run TestMaxAnimeLibEpisode 2>&1 | tail -12`
Expected: FAIL — helper undefined. (If the animelib package has live-API tests that fail offline, scope strictly with `-run TestMaxAnimeLibEpisode`.)

- [ ] **Step 3: Add the helper + the public method**

In `services/catalog/internal/parser/animelib/latest_episode.go` (add `"context"` and `"strconv"` imports if absent):
```go
// maxAnimeLibEpisode returns the highest integer episode number in the list.
// Non-numeric/fractional Number strings are skipped (AnimeLib uses string
// numbers; specials like "3.5" don't advance the "latest episode" count).
func maxAnimeLibEpisode(eps []Episode) int {
	best := 0
	for _, e := range eps {
		n, err := strconv.Atoi(e.Number)
		if err != nil {
			continue
		}
		if n > best {
			best = n
		}
	}
	return best
}

// LatestEpisodeAnyTeam returns the latest episode number across the full
// (team-agnostic) episode list for an AnimeLib anime id. Used by the
// notifications detector for aePlayer animelib combos (no specific team).
// Returns 0 + nil when the list is empty (caller maps to NotFound/skip).
func (c *Client) LatestEpisodeAnyTeam(ctx context.Context, animelibID int) (int, error) {
	eps, err := c.GetEpisodes(animelibID)
	if err != nil {
		return 0, fmt.Errorf("animelib: episodes for id %d: %w", animelibID, err)
	}
	return maxAnimeLibEpisode(eps), nil
}
```
> If `GetEpisodes` is not context-aware, keep the `ctx` param on `LatestEpisodeAnyTeam` (the catalog caller passes one) but it may be unused — add `_ = ctx` only if the linter complains; otherwise leave it for forward-compat. Verify `fmt` is imported.

- [ ] **Step 4: Run the pure test — verify it passes; build**

Run: `cd services/catalog && go test ./internal/parser/animelib/ -run TestMaxAnimeLibEpisode -v 2>&1 | tail -10 && go build ./internal/parser/animelib/`
Expected: PASS; build OK.

- [ ] **Step 5: Commit**

```bash
git commit services/catalog/internal/parser/animelib/latest_episode.go services/catalog/internal/parser/animelib/latest_episode_test.go \
  -m "feat(animelib): LatestEpisodeAnyTeam (team-agnostic max episode number)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: Catalog — route empty-id kodik/animelib to any-team; handler accepts them

**Files:**
- Modify: `services/catalog/internal/service/episodes_lookup.go` (`LatestAvailable` dispatch ~100-140; add two service methods)
- Modify: `services/catalog/internal/handler/internal_episodes.go` (drop the `legacy && translationID == ""` 400, ~92-94)
- Test: `services/catalog/internal/handler/internal_episodes_test.go` (extend)

**Interfaces:**
- Consumes: `LatestEpisodeAnyTranslation` (Task 1), `LatestEpisodeAnyTeam` (Task 2), existing `s.catalogService.ResolveAnimeLibID`, `s.animeRepo.GetByShikimoriID`.

- [ ] **Step 1: Update the dispatch in `LatestAvailable`**

Remove the early guard (it currently rejects empty-id kodik/animelib):
```go
	animeLevel := isAnimeLevelPlayer(player)
	if !animeLevel && translationID == "" {
		return EpisodesLookupResult{}, apperrors.InvalidInput("translation_id required")
	}
```
→ replace with:
```go
	animeLevel := isAnimeLevelPlayer(player)
```
(An unknown player still falls through to the switch `default` → InvalidInput.)

In the `switch {`, add two cases BEFORE the existing `case player == "kodik":` / `case player == "animelib":`:
```go
	case player == "kodik" && translationID == "":
		latest, translationTitle, err = s.latestKodikAnyTeam(shikimoriID)
	case player == "animelib" && translationID == "":
		latest, translationTitle, err = s.latestAnimeLibAnyTeam(ctx, shikimoriID)
```
(The existing `case player == "kodik":` / `case player == "animelib":` now only run when `translationID != ""` — the legacy translation-specific path — unchanged.)

- [ ] **Step 2: Add the two service methods**

In `episodes_lookup.go` (near `lookupAnimeLib`):
```go
// latestKodikAnyTeam resolves the anime-level latest episode for an aePlayer
// kodik combo (empty translation_id) — the max across any translation.
func (s *EpisodesLookupService) latestKodikAnyTeam(shikimoriID string) (int, string, error) {
	n, err := s.kodikClient.LatestEpisodeAnyTranslation(shikimoriID)
	if err != nil {
		return 0, "", err
	}
	if n == 0 {
		return 0, "", apperrors.NotFound("no kodik episodes for anime")
	}
	return n, "", nil
}

// latestAnimeLibAnyTeam resolves the anime-level latest episode for an aePlayer
// animelib combo (empty translation_id) — the max across the full episode list.
func (s *EpisodesLookupService) latestAnimeLibAnyTeam(ctx context.Context, shikimoriID string) (int, string, error) {
	anime, err := s.animeRepo.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", fmt.Errorf("anime lookup by shikimori_id %q: %w", shikimoriID, err)
	}
	if anime == nil {
		return 0, "", apperrors.NotFound("anime not found")
	}
	animelibID, err := s.catalogService.ResolveAnimeLibID(ctx, anime)
	if err != nil {
		return 0, "", fmt.Errorf("animelib id resolve: %w", err)
	}
	n, err := s.animelibClient.LatestEpisodeAnyTeam(ctx, animelibID)
	if err != nil {
		return 0, "", err
	}
	if n == 0 {
		return 0, "", apperrors.NotFound("no animelib episodes for anime")
	}
	return n, "", nil
}
```

- [ ] **Step 3: Drop the handler's `translation_id` requirement for kodik/animelib**

In `services/catalog/internal/handler/internal_episodes.go`, remove the block (~92-94):
```go
	if legacy && translationID == "" {
		httputil.BadRequest(w, "translation_id is required")
		return
	}
```
Keep the `animeLevel`/`legacy` bools and the `if !animeLevel && !legacy { 400 }` unknown-player check. (kodik/animelib are now accepted with OR without `translation_id`: empty → any-team, present → legacy.) Update the package doc comment line that says `translation_id` is required for kodik/animelib.

- [ ] **Step 4: Update the failing handler test**

In `internal_episodes_test.go`, the Phase 1 test asserts `kodik` no-id → 400. Change that expectation to **200** and add `animelib` no-id → 200. Keep `hanime` → 400. Concretely, update `TestInternalEpisodes_AnimeLevelPlayersNoTranslationID` (or add a sibling):
```go
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=kodik"); c != 200 {
		t.Errorf("kodik no-id = %d, want 200 (Phase 3 any-team)", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=animelib"); c != 200 {
		t.Errorf("animelib no-id = %d, want 200 (Phase 3 any-team)", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=hanime&translation_id=x"); c != 400 {
		t.Errorf("hanime = %d, want 400", c)
	}
```

- [ ] **Step 5: Run handler tests; build + full service/handler tests**

Run:
```bash
cd services/catalog && go test ./internal/handler/ -run TestInternalEpisodes -v 2>&1 | tail -15
go build ./... && go test ./internal/handler/ ./internal/service/ 2>&1 | tail -10
```
Expected: handler test PASS; build OK; packages PASS.

- [ ] **Step 6: Commit**

```bash
git commit services/catalog/internal/service/episodes_lookup.go services/catalog/internal/handler/internal_episodes.go services/catalog/internal/handler/internal_episodes_test.go \
  -m "feat(catalog): route empty-id kodik/animelib combos to any-team latest-episode

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 4: Notifications — `hotcombos` admits empty-id kodik/animelib

**Files:**
- Modify: `services/notifications/internal/job/hotcombos.go` (WHERE clause + doc comment)
- Test: `services/notifications/internal/job/hotcombos_test.go` (extend)

- [ ] **Step 1: Update the failing test**

In `hotcombos_test.go`, the Phase 1 `TestHotCombos_AdmitsAnimeLevelPlayers` seeds a `kodik` row WITH a `translation_id` (legacy, admitted) and asserts `hanime`-empty excluded. Add empty-id `kodik`/`animelib` rows and assert they are now admitted; keep `hanime`-empty excluded. Add to the existing seeds:
```go
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-ke','666','ongoing'),('a-ale','777','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-ke','watching'),('u1','a-ale','watching')`)
	seedWatch(t, db, "u1", "a-ke", "kodik", "ru", "sub", "", 4)     // aePlayer kodik, empty id → admitted
	seedWatch(t, db, "u1", "a-ale", "animelib", "ru", "sub", "", 2) // aePlayer animelib, empty id → admitted
```
and extend the post-`Collect` assertions: count empty-id kodik/animelib as admitted (the result already keys by player, but the legacy kodik with id and the empty-id kodik share `player="kodik"` — so assert presence by a richer key). Simplest robust assertion: collect `(player, translation_id)` pairs and assert `{kodik,""}` and `{animelib,""}` are present, and `{hanime,""}` is absent:
```go
	seen := map[string]bool{}
	for _, c := range combos {
		seen[c.Player+"|"+c.TranslationID] = true
	}
	if !seen["kodik|"] {
		t.Errorf("empty-id kodik combo not admitted")
	}
	if !seen["animelib|"] {
		t.Errorf("empty-id animelib combo not admitted")
	}
	if seen["hanime|"] {
		t.Errorf("empty-id hanime combo must NOT be admitted")
	}
```
> Confirm `domain.Combo` exposes `TranslationID` (it does — used by the detector). Keep the existing english/ae/raw assertions.

- [ ] **Step 2: Run — verify it fails**

Run from the worktree: `cd services/notifications && go test ./internal/job/ -run TestHotCombos_AdmitsAnimeLevel 2>&1 | tail -15`
Expected: FAIL — empty-id kodik/animelib dropped (not in the IN list yet).

- [ ] **Step 3: Update the filter**

In `hotcombos.go`, extend the player IN-list:
```go
		  AND (wh.translation_id != '' OR wh.player IN ('english', 'ae', 'raw', 'kodik', 'animelib'))
```
Update the doc comment: anime-level players now also include `kodik`/`animelib` (resolved any-team when `translation_id` is empty); only `hanime`/`18anime` with an empty id stay excluded.

- [ ] **Step 4: Run — verify it passes; full job tests**

Run: `cd services/notifications && go test ./internal/job/ -run TestHotCombos_AdmitsAnimeLevel -v 2>&1 | tail -12 && go test ./internal/job/ 2>&1 | tail -5`
Expected: PASS; package PASS.

- [ ] **Step 5: Commit**

```bash
git commit services/notifications/internal/job/hotcombos.go services/notifications/internal/job/hotcombos_test.go \
  -m "feat(notifications): admit empty-id kodik/animelib combos (any-team) into detection

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 5: Verify + deploy + changelog

**Files:** `frontend/web/changelog.full.json` + `frontend/web/public/changelog.json` (changelog only).

- [ ] **Step 1: Targeted + package suites**

Run (avoid live-API kodik/animelib packages except the scoped pure tests):
```bash
cd services/catalog && go build ./... && go test ./internal/service/ ./internal/handler/ 2>&1 | tail -8
go test ./internal/parser/kodik/ -run 'TestResultEpisodeCount|TestMaxAnyTeamEpisode' 2>&1 | tail -4
go test ./internal/parser/animelib/ -run TestMaxAnimeLibEpisode 2>&1 | tail -4
cd ../notifications && go test ./... 2>&1 | tail -8
```
Expected: no FAIL.

- [ ] **Step 2: Changelog (Russian Trump-mode)**

Read `frontend/web/changelog.full.json`; merge into the top `2026-06-18` group a `feature` entry, e.g.:
`"🔔 KODIK И ANILIB В НАШЕМ ПЛЕЕРЕ — ТЕПЕРЬ ТОЖЕ С УВЕДОМЛЕНИЯМИ! Смотришь русскую озвучку через плеер AnimeEnigma — узнаёшь о новой серии СРАЗУ, любая команда озвучки. Закрыли ПОСЛЕДНЮЮ дыру: теперь ВСЕ источники в нашем плеере шлют уведомления. ЖОСКО. Никто другой так не делает!"`
Then `cd frontend/web && node scripts/changelog-trim.mjs`. Commit BOTH files (path-scoped, co-authors).

- [ ] **Step 3: Deploy (controller, from the clean worktree)**

Symlink node_modules (`ln -sfn /data/animeenigma/frontend/web/node_modules <wt>/frontend/web/node_modules`) for the redeploy-web vue-tsc gate; copy `docker/.env`. Then `make redeploy-catalog`, `make redeploy-notifications`, `make redeploy-web`; `make health` (all ✓). (Scraper unchanged this phase.)

- [ ] **Step 4: Runtime verification**

```bash
# kodik/animelib empty-id now RESOLVE (no longer 400). 200 if the anime has episodes
# on that source, 404 if not — NOT 400/500.
curl -s -o /dev/null -w "kodik    no-id -> %{http_code}\n" "http://localhost:8081/internal/anime/57466/episodes?player=kodik&watch_type=sub&language=ru"
curl -s -o /dev/null -w "animelib no-id -> %{http_code}\n" "http://localhost:8081/internal/anime/57466/episodes?player=animelib&watch_type=sub&language=ru"
# legacy kodik WITH id still works (regression):
curl -s -o /dev/null -w "kodik    w/ id -> %{http_code}\n" "http://localhost:8081/internal/anime/57466/episodes?player=kodik&translation_id=609&watch_type=sub&language=ru"
```
Expected: empty-id kodik/animelib → 200 or 404 (NOT 400); legacy-with-id → 200/404 as before.

## Notes / scope

- This completes the milestone: `english`(sub+dub)/`ae`/`raw`/`kodik`/`animelib` all covered. `hanime`/`18anime` remain excluded (no episode-list capability, 18+).
- A user who watched the SAME anime on both the legacy KodikPlayer (numeric id) AND aePlayer-kodik (empty id) has two distinct snapshot rows → could receive two notifications for one episode. Rare; acceptable; documented.
- any-team kodik/animelib is watch_type-agnostic (the count is "a new episode exists on this source"), consistent with the english-sub anime-level behavior.
- Optional cleanup carried from earlier phases (do if convenient, not required): `maxEpisodeNum`/`max==0` sentinel can't represent an EP0; orchestrator normalize loop runs on the nil-safe error path.
