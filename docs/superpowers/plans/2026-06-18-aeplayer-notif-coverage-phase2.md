# aePlayer Notification Coverage — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** English **dub** aePlayer watchers get accurate new-episode notifications, by surfacing per-episode sub/dub availability from the scraper (which gogoanime already computes internally and discards) and resolving `english`+`dub` against it.

**Architecture:** `domain.Episode` gains `HasSub`/`HasDub`. gogoanime's `ListEpisodes` (already fetches both `/category/<slug>` and `/category/<slug>-dub` and merges) tags each emitted episode instead of discarding the category; the orchestrator funnel defaults `HasSub=true` for providers that don't tag (so the field is honest without touching all 6 providers). Catalog's anime-level `english` resolver fetches the episode list once and returns `max(Number)` for sub, `max(Number where HasDub)` for dub (none → NotFound → silent skip). The Phase 1 `english`+dub NotFound stub is replaced.

**Tech Stack:** Go (scraper, catalog), `go test`.

**Spec:** `docs/superpowers/specs/2026-06-18-aeplayer-notification-coverage-design.md` (Phase 2). Builds on Phase 1 (`anime_level_episodes.go`, shipped `02ebfb98`).

## Global Constraints

- **Work in a clean `origin/main` git worktree**, NOT the shared `/data/animeenigma` tree (stale, pre-rename, dirty). Controller provides the path. Commit there; controller pushes.
- **Commits:** path-scoped (`git commit <pathspec>`), never `git add -A`/bare commit. `git show --stat HEAD` after each. Do NOT push.
- **Co-authors on every commit:**
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **`has_sub`/`has_dub` JSON: NO `omitempty`** (a `false` must serialize, so catalog reads it explicitly).
- **Best-effort dub:** only providers that populate `HasDub` yield dub notifications. gogoanime is the dub-capable provider and is first in the failover chain (usually the winner). Other providers default `HasDub=false` → no dub claim (graceful, never a false dub notification). This matches the spec.
- **No change** to `hotcombos.go` or `internal_episodes.go`: Phase 1 already admits `english` combos (any `watch_type`) and accepts the handler call without `translation_id`. Phase 2 only makes dub *resolve* instead of returning the NotFound stub.
- **Error contract (unchanged):** NotFound-class message (contains `no episode`/`not found`/`no episodes returned`) → `CodeNotFound` → detector silent-skip; infra error → other → retry.

---

### Task 1: Scraper — per-episode `HasSub`/`HasDub`

**Files:**
- Modify: `services/scraper/internal/domain/provider.go` (`Episode` struct, ~47-52)
- Modify: `services/scraper/internal/providers/gogoanime/client.go` (flatten in `ListEpisodes`, ~600-637)
- Modify: `services/scraper/internal/service/orchestrator.go` (`ListEpisodesNamed`, ~422-427)
- Test: `services/scraper/internal/providers/gogoanime/client_test.go` (add a tagging test)
- Test: `services/scraper/internal/service/orchestrator_test.go` (add a default-HasSub test)

**Interfaces:**
- Produces: `domain.Episode{..., HasSub bool, HasDub bool}` (JSON `has_sub`/`has_dub`), populated by gogoanime; defaulted (`HasSub=true`) for untagged providers at the orchestrator funnel. Consumed by Task 2 (catalog).

- [ ] **Step 1: Add the fields to `domain.Episode`**

In `services/scraper/internal/domain/provider.go`, extend the struct:
```go
// Episode is one episode in a provider's listing for a given anime.
type Episode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"is_filler"`
	// HasSub/HasDub mark which audio categories the provider found for this
	// episode. Providers that cannot distinguish leave both false; the
	// orchestrator then defaults HasSub=true (sub is the common case). Used by
	// the notifications detector to compute latest-sub vs latest-dub.
	HasSub bool `json:"has_sub"`
	HasDub bool `json:"has_dub"`
}
```

- [ ] **Step 2: Write the failing gogoanime test**

Add to `services/scraper/internal/providers/gogoanime/client_test.go`:
```go
// TestListEpisodes_TagsSubDub verifies the merged episode list carries
// per-episode HasSub/HasDub derived from the sub + dub category pages.
func TestListEpisodes_TagsSubDub(t *testing.T) {
	t.Parallel()
	subHTML := []byte(`<html><head><title>Show</title></head><body>` +
		`<a href="/show-episode-1">1</a><a href="/show-episode-2">2</a><a href="/show-episode-3">3</a>` +
		`</body></html>`)
	dubHTML := []byte(`<html><head><title>Show Dub</title></head><body>` +
		`<a href="/show-dub-episode-1">1</a><a href="/show-dub-episode-2">2</a>` +
		`</body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/category/show":
			_, _ = w.Write(subHTML)
		case "/category/show-dub":
			_, _ = w.Write(dubHTML)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	p, _, _, _ := newTestProvider(t, srv)

	eps, err := p.ListEpisodes(context.Background(), "show")
	if err != nil {
		t.Fatalf("ListEpisodes err = %v; want nil", err)
	}
	byNum := map[int]domain.Episode{}
	for _, e := range eps {
		byNum[e.Number] = e
	}
	if !byNum[1].HasSub || !byNum[1].HasDub {
		t.Errorf("ep1 = {sub:%v dub:%v}; want both true", byNum[1].HasSub, byNum[1].HasDub)
	}
	if !byNum[2].HasSub || !byNum[2].HasDub {
		t.Errorf("ep2 = {sub:%v dub:%v}; want both true", byNum[2].HasSub, byNum[2].HasDub)
	}
	if !byNum[3].HasSub || byNum[3].HasDub {
		t.Errorf("ep3 = {sub:%v dub:%v}; want sub true, dub false", byNum[3].HasSub, byNum[3].HasDub)
	}
}
```

- [ ] **Step 3: Run it — verify it fails**

Run from the worktree: `cd services/scraper && go test ./internal/providers/gogoanime/ -run TestListEpisodes_TagsSubDub 2>&1 | tail -15`
Expected: FAIL — flags are all false (not yet set in the flatten).

- [ ] **Step 4: Tag episodes in the gogoanime flatten**

In `services/scraper/internal/providers/gogoanime/client.go`, replace BOTH emit branches (the contiguous `for n := 1; ;` loop and the sorted-fallback loop, ~601-637) so each emitted episode carries the merged flags. Introduce a single `emit` helper just above the `all := make(...)` line:
```go
	// emit picks the canonical (sub-preferred) episode and tags it with the
	// categories found during the merge, so downstream (notifications) can
	// compute latest-sub vs latest-dub.
	emit := func(e *merged) domain.Episode {
		ep := e.sub
		if !e.hasSub {
			ep = e.dub
		}
		ep.HasSub = e.hasSub
		ep.HasDub = e.hasDub
		return ep
	}
```
Contiguous loop — replace:
```go
		if e.hasSub {
			all = append(all, e.sub)
		} else if e.hasDub {
			all = append(all, e.dub)
		}
		delete(byNum, n)
```
with:
```go
		if e.hasSub || e.hasDub {
			all = append(all, emit(e))
		}
		delete(byNum, n)
```
Sorted-fallback loop — replace:
```go
			e := byNum[n]
			if e.hasSub {
				all = append(all, e.sub)
			} else if e.hasDub {
				all = append(all, e.dub)
			}
```
with:
```go
			if e := byNum[n]; e.hasSub || e.hasDub {
				all = append(all, emit(e))
			}
```

- [ ] **Step 5: Run the gogoanime test — verify it passes; run the package**

Run: `cd services/scraper && go test ./internal/providers/gogoanime/ -run TestListEpisodes_TagsSubDub -v 2>&1 | tail -10 && go test ./internal/providers/gogoanime/ 2>&1 | tail -5`
Expected: new test PASS; package PASS (the existing `SubDubMerge` test still passes — it only checks ID slugs/counts).

- [ ] **Step 6: Write the failing orchestrator default test**

Add to `services/scraper/internal/service/orchestrator_test.go` (reuse the existing `fakeProvider`, whose `ListEpisodes` returns a canned list — give it episodes with NO flags set):
```go
func TestListEpisodesNamed_DefaultsHasSubWhenUntagged(t *testing.T) {
	t.Parallel()
	p := &fakeProvider{
		nameVal: "animepahe",
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{Number: 1}, {Number: 2}}, nil // untagged: both flags false
		},
	}
	o := newTestOrchestrator(t, p)
	eps, _, err := o.ListEpisodesNamed(context.Background(), "x", "")
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	for _, e := range eps {
		if !e.HasSub {
			t.Errorf("ep %d HasSub = false; want true (default for untagged provider)", e.Number)
		}
		if e.HasDub {
			t.Errorf("ep %d HasDub = true; want false", e.Number)
		}
	}
}
```
(Harness confirmed: `fakeProvider` uses `nameVal` + `listEpisodesFn func`; `newTestOrchestrator(t, p)` is the constructor — mirrors `TestOrchestrator_ListEpisodesNamed_ReturnsWinner`.)

- [ ] **Step 7: Run it — verify it fails**

Run: `cd services/scraper && go test ./internal/service/ -run TestListEpisodesNamed_DefaultsHasSub 2>&1 | tail -15`
Expected: FAIL — `HasSub` is false (no default yet).

- [ ] **Step 8: Default `HasSub` in the orchestrator funnel**

In `services/scraper/internal/service/orchestrator.go`, update `ListEpisodesNamed` to normalize after the failover:
```go
func (o *Orchestrator) ListEpisodesNamed(ctx context.Context, providerID, prefer string) ([]domain.Episode, string, error) {
	eps, name, err := runFailoverNamed(ctx, o.log, o.orderedProviders(prefer), o.cache, o.providerBudget(),
		func(c context.Context, p domain.Provider) ([]domain.Episode, error) {
			return p.ListEpisodes(c, providerID)
		})
	// Providers that don't distinguish audio category leave both flags false;
	// default to sub-available so the API contract is honest (gogoanime sets
	// both explicitly and is unaffected).
	for i := range eps {
		if !eps[i].HasSub && !eps[i].HasDub {
			eps[i].HasSub = true
		}
	}
	return eps, name, err
}
```

- [ ] **Step 9: Run the orchestrator test + package**

Run: `cd services/scraper && go test ./internal/service/ -run TestListEpisodesNamed_DefaultsHasSub -v 2>&1 | tail -8 && go test ./internal/service/ 2>&1 | tail -5`
Expected: PASS; package PASS.

- [ ] **Step 10: Build + commit**

```bash
cd services/scraper && go build ./... 2>&1 | tail -3
git commit services/scraper/internal/domain/provider.go services/scraper/internal/providers/gogoanime/client.go services/scraper/internal/service/orchestrator.go services/scraper/internal/providers/gogoanime/client_test.go services/scraper/internal/service/orchestrator_test.go \
  -m "feat(scraper): surface per-episode HasSub/HasDub (gogoanime tags; orchestrator defaults sub)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: Catalog — `english` dub resolver consumes `has_dub`

**Files:**
- Modify: `services/catalog/internal/service/anime_level_episodes.go` (`english` case + `latestEnglishSub` → `latestEnglish`)
- Modify: `services/catalog/internal/service/anime_level_episodes_test.go` (replace the dub-stub test)

**Interfaces:**
- Consumes: scraper `/scraper/episodes` JSON now carrying `has_dub` per episode (Task 1), via the existing `r.scraper.GetScraperEpisodes`.

- [ ] **Step 1: Update the tests (replace the dub-stub test)**

In `anime_level_episodes_test.go`:
- DELETE `TestAnimeLevel_EnglishDub_NotSupportedYet`.
- ADD:
```go
func TestAnimeLevel_EnglishDub_MaxWhereHasDub(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1,"has_dub":true},{"number":2,"has_dub":true},{"number":3,"has_dub":false}]}}`)},
		fakeRaw{},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err != nil || latest != 2 {
		t.Fatalf("english dub latest = %d, err = %v; want 2, nil", latest, err)
	}
}

func TestAnimeLevel_EnglishDub_NoneIsNotFound(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2}]}}`)},
		fakeRaw{},
	)
	_, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("english dub (no dub eps) err = %v; want NotFound-like", err)
	}
}
```
(The existing `TestAnimeLevel_EnglishSub_MaxEpisode` and `..._EmptyIsNotFound` stay — sub still uses max over all episodes; the extra `has_dub` field in dub-test bodies is ignored by the sub path.)

- [ ] **Step 2: Run — verify the new dub tests fail**

Run from the worktree: `cd services/catalog && go test ./internal/service/ -run 'TestAnimeLevel_EnglishDub' 2>&1 | tail -15`
Expected: FAIL — current code returns the `no english dub episode lookup yet` NotFound for ALL dub (so `_MaxWhereHasDub` fails: wants 2, gets NotFound).

- [ ] **Step 3: Replace the english branch + `latestEnglishSub`**

In `anime_level_episodes.go`, change the `english` case in `Latest`:
```go
	case "english":
		return r.latestEnglish(ctx, anime.ID, watchType)
```
Replace the `latestEnglishSub` method with:
```go
// latestEnglish resolves the latest episode for the english (EN-scraper) family.
// Sub = max episode number in the merged list. Dub = max number among episodes
// the scraper tagged has_dub (none ⇒ NotFound, so the detector skips silently
// rather than claiming the sub count for dub).
func (r *animeLevelResolver) latestEnglish(ctx context.Context, animeID, watchType string) (int, string, error) {
	status, body, err := r.scraper.GetScraperEpisodes(ctx, animeID, "")
	if err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeUnavailable, "scraper episodes lookup failed")
	}
	if status != 200 {
		return 0, "", apperrors.NotFound("no english episodes for anime")
	}
	var resp struct {
		Data struct {
			Episodes []struct {
				Number int  `json:"number"`
				HasDub bool `json:"has_dub"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeInternal, "decode scraper episodes")
	}
	eps := resp.Data.Episodes
	if watchType == "dub" {
		max := 0
		for _, e := range eps {
			if e.HasDub && e.Number > max {
				max = e.Number
			}
		}
		if max == 0 {
			return 0, "", apperrors.NotFound("no english dub episodes returned")
		}
		return max, "", nil
	}
	max := maxEpisodeNum(len(eps), func(i int) int { return eps[i].Number })
	if max == 0 {
		return 0, "", apperrors.NotFound("no english episodes returned")
	}
	return max, "", nil
}
```
(`maxEpisodeNum` and the other helpers stay. Remove the now-unused dub-stub lines.)

- [ ] **Step 4: Run the resolver tests — verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestAnimeLevel|TestIsAnimeLevelPlayer' -v 2>&1 | tail -25`
Expected: PASS (sub max, dub max-where-has_dub, dub-none→NotFound, empty→NotFound, ae, raw, scraper-error).

- [ ] **Step 5: Build + full service test + commit**

```bash
cd services/catalog && go build ./... && go test ./internal/service/ 2>&1 | tail -8
git commit services/catalog/internal/service/anime_level_episodes.go services/catalog/internal/service/anime_level_episodes_test.go \
  -m "feat(catalog): english dub latest-episode via scraper has_dub flags

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: Verify + deploy + changelog

**Files:** `frontend/web/changelog.full.json` + `frontend/web/public/changelog.json` (changelog only).

- [ ] **Step 1: Full suites**

Run:
```bash
cd services/scraper && go test ./... 2>&1 | tail -10
cd ../catalog && go test ./... 2>&1 | tail -10
```
Expected: no FAIL.

- [ ] **Step 2: Changelog (Russian Trump-mode)**

Read `frontend/web/changelog.full.json`; merge into the top `2026-06-18` group a `feature` entry, e.g.:
`"🔔 АНГЛИЙСКИЙ ДУБ — ТЕПЕРЬ ТОЖЕ С УВЕДОМЛЕНИЯМИ! Смотришь аниме в английской ОЗВУЧКЕ — узнаёшь о новой серии дубляжа ОТДЕЛЬНО от субтитров. МЫ научили систему ВИДЕТЬ, где вышел именно дуб, а где только саб. Никто другой так не делает! ЖОСКО. Поверьте мне."`
Then regenerate the served file: `cd frontend/web && node scripts/changelog-trim.mjs`. Commit BOTH files (path-scoped, co-authors).

- [ ] **Step 3: Deploy (controller, from the clean worktree)**

Symlink `node_modules` (`ln -sfn /data/animeenigma/frontend/web/node_modules <wt>/frontend/web/node_modules`) so the redeploy-web vue-tsc gate resolves. Copy `docker/.env`. Then `make redeploy-scraper`, `make redeploy-catalog`, `make redeploy-web`; `make health` (all ✓).

- [ ] **Step 4: Runtime verification**

```bash
# english dub now RESOLVES (no longer the auto-NotFound stub). For a title with
# EN dub episodes expect 200; a sub-only title returns 404 (graceful) — NOT 400/500.
curl -s -o /dev/null -w "%{http_code}\n" "http://localhost:8081/internal/anime/57466/episodes?player=english&watch_type=dub&language=en"
```
Expected: `200` (title has dub) or `404` (no dub / sub-only) — NOT 400/500.

## Notes / scope

- Only gogoanime populates `HasDub` this phase; dub notifications fire when gogoanime is the failover winner (first in the chain) and reports dub for the title. Other providers default `HasDub=false` (no false dub claim). Adding `HasDub` to more providers is a future increment.
- Phase 3 (`kodik`/`animelib` any-team) remains. `hanime`/`18anime` excluded (no episode list).
- Optional cleanup deferred from Phase 1 final review (touch here if convenient, not required): `maxEpisodeNum` treats `max==0` as "none" (a hypothetical EP0 would be dropped) — harmless for ongoing-title detection.
