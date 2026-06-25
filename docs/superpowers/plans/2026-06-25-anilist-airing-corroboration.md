# AniList Airing Corroboration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** During catalog calendar sync, corroborate each ongoing anime's Shikimori next-episode date against AniList's broadcaster schedule and adopt AniList's date when it is strictly later, fixing stale dates caused by Shikimori ignoring broadcast hiatuses (e.g. Re:Zero S4 ep12: Shikimori July 1 vs reality Aug 12).

**Architecture:** Add an `AniListAiringByMALID` method to the existing `libs/idmapping` client (one GraphQL call, reusing its IPv4 + egress-wrapped transport). In `catalog.SyncCalendar`, after the calendar is deduped into the `seen` map, run a best-effort reconciliation pass that mutates each entry's next-episode time to AniList's when strictly later and records the winning source. Both existing write paths already read the reconciled value from `seen`, so they pick up the correction automatically; a new `next_episode_source` column plus a pure `defendAniListNextEpisode` guard keep the nightly Shikimori batch refresh from clobbering corrections.

**Tech Stack:** Go 1.x, GORM/PostgreSQL (auto-migrate), AniList public GraphQL (`https://graphql.anilist.co`), Prometheus (`promauto`), standard-library `net/http` + `net/http/httptest` for tests.

## Global Constraints

- **No new Go module / no new dependencies.** `AniListAiringByMALID` is a method on the *existing* `libs/idmapping` module and uses only imports it already has (`bytes`, `context`, `encoding/json`, `fmt`, `io`, `net/http`, `strconv`, `time`). Do **not** touch `go.work`, importer `go.mod` files, or Dockerfiles.
- **No `testify`/`mock`.** Handwritten fakes and stubs only (project convention; see `guesspool_test.go`).
- **Later-wins, exact "any later".** AniList is adopted only when its `airingAt` is *strictly after* Shikimori's date, treating a nil time as "earliest". No tolerance margin.
- **Fail-safe.** Any AniList error / missing mapping / nil airing time leaves the Shikimori value untouched. Calendar sync correctness is never coupled to AniList availability.
- **No Redis cache.** Daily job over dozens of anime; caching is out of scope (YAGNI).
- **Worktree workflow.** All work happens in a worktree off fresh `origin/main` (never edit the `/data/animeenigma` base tree). Commit with the three required co-authors:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Effort metrics (no days/hours):** UXΔ = +2 (Better); CDI = 0.04 * 8; MVQ = Griffin 85%/80%.

---

## File Structure

| File | Responsibility | Task |
|------|----------------|------|
| `libs/idmapping/client.go` | New `AniListAiring` type + `AniListAiringByMALID` method + shared `postAniListGraphQL` helper; refactor `resolveAniList` onto the helper | 1 |
| `libs/idmapping/client_test.go` | Stub-server tests for `AniListAiringByMALID` | 1 |
| `services/catalog/internal/domain/anime.go` | New `NextEpisodeSource` column | 2 |
| `libs/metrics/sync.go` | New `NextEpisodeSourceTotal` counter | 2 |
| `services/catalog/internal/service/calendar_anilist.go` | Source consts, `laterWins` (T2); `AniListAiringFetcher` interface + `reconcileCalendarWithAniList` (T3); `defendAniListNextEpisode` (T4) | 2,3,4 |
| `services/catalog/internal/service/calendar_anilist_test.go` | `laterWins` (T2), reconcile-with-fake (T3), defend-guard (T4) tests | 2,3,4 |
| `services/catalog/internal/service/catalog.go` | Struct field + constructor wiring; `calendarInfo.source`; `SyncCalendar` hook; source propagation in both write paths; `refreshStaleAnime` guard call | 3,4 |

---

## Task 1: AniList airing lookup in `libs/idmapping`

**Files:**
- Modify: `libs/idmapping/client.go`
- Test: `libs/idmapping/client_test.go`

**Interfaces:**
- Consumes: nothing (leaf module).
- Produces:
  - `type AniListAiring struct { AniListID int; Status string; NextEpisode int; NextAiringAt *time.Time }`
  - `func (c *Client) AniListAiringByMALID(ctx context.Context, malID string) (*AniListAiring, error)` — `(result, nil)` when AniList has a Media for the id (`NextAiringAt` is nil when nothing is scheduled); `(nil, nil)` when AniList knows no Media; `(nil, err)` on transport/GraphQL error.

- [ ] **Step 1: Write the failing tests**

Append to `libs/idmapping/client_test.go` (reuses the existing `newTestClient(armServerURL, aniListServerURL string)` helper; ARM URL is unused here so pass an empty-handler server):

```go
func TestAniListAiringByMALID_Scheduled(t *testing.T) {
	const body = `{"data":{"Media":{"id":52991,"status":"RELEASING","nextAiringEpisode":{"episode":12,"airingAt":1786579200}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := newTestClient("", srv.URL)
	got, err := c.AniListAiringByMALID(context.Background(), "52991")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected result, got nil")
	}
	if got.AniListID != 52991 || got.Status != "RELEASING" || got.NextEpisode != 12 {
		t.Errorf("unexpected fields: %+v", got)
	}
	if got.NextAiringAt == nil || got.NextAiringAt.Unix() != 1786579200 {
		t.Errorf("airingAt: want unix 1786579200, got %v", got.NextAiringAt)
	}
	if got.NextAiringAt.Location() != time.UTC {
		t.Errorf("airingAt should be UTC, got %v", got.NextAiringAt.Location())
	}
}

func TestAniListAiringByMALID_NoUpcomingEpisode(t *testing.T) {
	const body = `{"data":{"Media":{"id":1,"status":"FINISHED","nextAiringEpisode":null}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got, err := newTestClient("", srv.URL).AniListAiringByMALID(context.Background(), "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Status != "FINISHED" || got.NextAiringAt != nil {
		t.Errorf("want Media with nil NextAiringAt, got %+v", got)
	}
}

func TestAniListAiringByMALID_NoMedia(t *testing.T) {
	const body = `{"data":{"Media":null}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got, err := newTestClient("", srv.URL).AniListAiringByMALID(context.Background(), "999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for unknown id, got %+v", got)
	}
}

func TestAniListAiringByMALID_GraphQLError(t *testing.T) {
	const body = `{"errors":[{"message":"Too Many Requests"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got, err := newTestClient("", srv.URL).AniListAiringByMALID(context.Background(), "5")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Errorf("want nil result on error, got %+v", got)
	}
}

func TestAniListAiringByMALID_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	defer srv.Close()

	if _, err := newTestClient("", srv.URL).AniListAiringByMALID(context.Background(), "5"); err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}
```

Add `"time"` to the test file's import block if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd libs/idmapping && go test ./... -run TestAniListAiringByMALID -v`
Expected: FAIL — `c.AniListAiringByMALID undefined (type *Client has no field or method AniListAiringByMALID)`.

- [ ] **Step 3: Add the shared POST helper and refactor `resolveAniList` onto it**

In `libs/idmapping/client.go`, add this method (place it just above `resolveAniList`):

```go
// postAniListGraphQL sends a GraphQL query body to AniList and returns the raw
// response bytes. It owns the per-request timeout (aniListTimeout), JSON headers,
// body-size limit, and non-200 handling — so resolveAniList and
// AniListAiringByMALID speak to AniList through exactly one code path.
func (c *Client) postAniListGraphQL(ctx context.Context, body string) ([]byte, error) {
	// Per-request budget derived from the caller's ctx (WR-01); see resolveARM.
	ctx, cancel := context.WithTimeout(ctx, aniListTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.aniListBaseURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("AniList build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AniList request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if rerr != nil {
		return nil, fmt.Errorf("AniList read body: %w", rerr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AniList HTTP %d: %s", resp.StatusCode, truncate(string(respBytes), 200))
	}
	return respBytes, nil
}
```

Then replace the body of `resolveAniList` from the `ctx, cancel := context.WithTimeout(...)` line (currently line 299) through the `respBytes, rerr := io.ReadAll(...)` / non-200 block (currently through line 330) with a single call to the helper. The refactored `resolveAniList` reads:

```go
func (c *Client) resolveAniList(ctx context.Context, malID string) (*MappingResult, error) {
	intID, perr := strconv.Atoi(malID)
	if perr != nil {
		return nil, fmt.Errorf("AniList: invalid MAL id %q: %w", malID, perr)
	}

	body := fmt.Sprintf(
		`{"query":"query($mal:Int){Media(idMal:$mal,type:ANIME){id idMal}}","variables":{"mal":%d}}`,
		intID,
	)

	respBytes, err := c.postAniListGraphQL(ctx, body)
	if err != nil {
		return nil, err
	}

	var parsed aniListGraphQLResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("AniList decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("AniList GraphQL error: %s", parsed.Errors[0].Message)
	}
	if parsed.Data.Media == nil || parsed.Data.Media.ID == 0 {
		return nil, nil // No mapping known to AniList.
	}

	aniListID := parsed.Data.Media.ID
	malIDEcho := parsed.Data.Media.IDMAL
	return &MappingResult{
		AniList: &aniListID,
		MAL:     &malIDEcho,
	}, nil
}
```

- [ ] **Step 4: Add the airing type, response shape, and method**

In `libs/idmapping/client.go`, add the public type near `MappingResult` (after line 59):

```go
// AniListAiring is the broadcaster airing schedule AniList exposes for a Media.
// Unlike Shikimori's naive "last + 1 week" estimate, AniList's nextAiringEpisode
// reflects the real broadcast schedule and models hiatuses. NextAiringAt is nil
// when AniList has no upcoming episode scheduled (e.g. FINISHED series).
type AniListAiring struct {
	AniListID    int        // AniList Media.id
	Status       string     // RELEASING | FINISHED | NOT_YET_RELEASED | CANCELLED | HIATUS
	NextEpisode  int        // nextAiringEpisode.episode; 0 when none scheduled
	NextAiringAt *time.Time // nextAiringEpisode.airingAt (unix seconds → UTC); nil when none
}
```

Add the private response shape next to `aniListGraphQLResponse` (after line 282):

```go
// aniListAiringResponse mirrors the JSON shape returned by AniList for the
// airing-schedule Media query.
type aniListAiringResponse struct {
	Data struct {
		Media *struct {
			ID                int    `json:"id"`
			Status            string `json:"status"`
			NextAiringEpisode *struct {
				Episode  int   `json:"episode"`
				AiringAt int64 `json:"airingAt"`
			} `json:"nextAiringEpisode"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}
```

Add the method (place it after `resolveAniList`):

```go
// AniListAiringByMALID queries AniList for the broadcaster airing schedule by
// MAL/Shikimori id (Shikimori IDs equal MAL IDs). Returns:
//   - (result, nil) — AniList found a Media (NextAiringAt is nil if nothing is scheduled)
//   - (nil, nil)    — AniList knows no Media with this MAL id
//   - (nil, error)  — transport / JSON / GraphQL error
//
// AniList's Media query supports idMal:Int directly and returns
// nextAiringEpisode in the same call. No auth required for public reads.
func (c *Client) AniListAiringByMALID(ctx context.Context, malID string) (*AniListAiring, error) {
	intID, perr := strconv.Atoi(malID)
	if perr != nil {
		return nil, fmt.Errorf("AniList airing: invalid MAL id %q: %w", malID, perr)
	}

	body := fmt.Sprintf(
		`{"query":"query($mal:Int){Media(idMal:$mal,type:ANIME){id status nextAiringEpisode{episode airingAt}}}","variables":{"mal":%d}}`,
		intID,
	)

	respBytes, err := c.postAniListGraphQL(ctx, body)
	if err != nil {
		return nil, err
	}

	var parsed aniListAiringResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("AniList airing decode: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("AniList airing GraphQL error: %s", parsed.Errors[0].Message)
	}
	m := parsed.Data.Media
	if m == nil {
		return nil, nil // No Media known to AniList for this id.
	}

	out := &AniListAiring{AniListID: m.ID, Status: m.Status}
	if m.NextAiringEpisode != nil {
		out.NextEpisode = m.NextAiringEpisode.Episode
		t := time.Unix(m.NextAiringEpisode.AiringAt, 0).UTC()
		out.NextAiringAt = &t
	}
	return out, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd libs/idmapping && go test ./... -v`
Expected: PASS — the five new `TestAniListAiringByMALID_*` tests plus all pre-existing tests (the `resolveAniList` refactor must not regress them).

- [ ] **Step 6: Commit**

```bash
git add libs/idmapping/client.go libs/idmapping/client_test.go
git commit -m "feat(idmapping): AniListAiringByMALID — broadcaster airing schedule lookup

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Storage column, metric, and the `laterWins` rule

**Files:**
- Modify: `services/catalog/internal/domain/anime.go:80` (add field after `NextEpisodeAt`)
- Modify: `libs/metrics/sync.go`
- Create: `services/catalog/internal/service/calendar_anilist.go`
- Test: `services/catalog/internal/service/calendar_anilist_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `domain.Anime.NextEpisodeSource string` (GORM column `next_episode_source`, default `'shikimori'`)
  - `metrics.NextEpisodeSourceTotal *prometheus.CounterVec` (label `source`)
  - package-level consts `sourceShikimori = "shikimori"`, `sourceAniList = "anilist"` (package `service`)
  - `func laterWins(shikimori, anilist *time.Time) (chosen *time.Time, fromAniList bool)`

- [ ] **Step 1: Add the domain column**

In `services/catalog/internal/domain/anime.go`, insert directly after the `NextEpisodeAt` field (line 80):

```go
	// NextEpisodeSource records which provider supplied NextEpisodeAt:
	// "shikimori" (calendar default) or "anilist" (corroborated override when
	// AniList's broadcaster schedule is strictly later). Used by the batch
	// refresh guard to avoid clobbering an AniList correction. Auto-migrated.
	NextEpisodeSource string `gorm:"size:16;default:'shikimori';column:next_episode_source" json:"next_episode_source,omitempty"`
```

- [ ] **Step 2: Add the metric**

In `libs/metrics/sync.go`, add inside the `var ( ... )` block (after `SyncJobEntriesTotal`, before the closing `)`):

```go
	// NextEpisodeSourceTotal counts calendar anime whose next-episode date was
	// resolved from each source during calendar sync: "shikimori" (kept) vs
	// "anilist" (corroborated override). Lets operators see how often AniList
	// corrects a stale Shikimori date.
	NextEpisodeSourceTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "catalog_next_episode_source_total",
			Help: "Calendar anime next-episode date resolutions by source",
		},
		[]string{"source"},
	)
```

- [ ] **Step 3: Write the failing `laterWins` test**

Create `services/catalog/internal/service/calendar_anilist_test.go`:

```go
package service

import (
	"testing"
	"time"
)

func TestLaterWins(t *testing.T) {
	early := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		shikimori    *time.Time
		anilist      *time.Time
		wantChosen   *time.Time
		wantFromAni  bool
	}{
		{"anilist later wins", &early, &late, &late, true},
		{"anilist earlier loses", &late, &early, &late, false},
		{"equal keeps shikimori", &early, &early, &early, false},
		{"anilist nil keeps shikimori", &early, nil, &early, false},
		{"both nil stays nil", nil, nil, nil, false},
		{"shikimori nil adopts anilist", nil, &late, &late, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chosen, fromAni := laterWins(tc.shikimori, tc.anilist)
			if fromAni != tc.wantFromAni {
				t.Errorf("fromAniList: want %v, got %v", tc.wantFromAni, fromAni)
			}
			switch {
			case tc.wantChosen == nil && chosen != nil:
				t.Errorf("chosen: want nil, got %v", chosen)
			case tc.wantChosen != nil && (chosen == nil || !chosen.Equal(*tc.wantChosen)):
				t.Errorf("chosen: want %v, got %v", tc.wantChosen, chosen)
			}
		})
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestLaterWins -v`
Expected: FAIL — `undefined: laterWins`.

- [ ] **Step 5: Create the rule + consts**

Create `services/catalog/internal/service/calendar_anilist.go`:

```go
package service

import "time"

// Next-episode date provenance values stored in domain.Anime.NextEpisodeSource.
const (
	sourceShikimori = "shikimori"
	sourceAniList   = "anilist"
)

// laterWins picks the later of the Shikimori and AniList next-episode times,
// treating nil as "earliest". It returns the chosen time and whether AniList
// won. AniList is adopted only when it is strictly after Shikimori's date (or
// when Shikimori has no date at all): Shikimori's failure mode is reporting a
// date that is too EARLY (it ignores broadcast hiatuses), so only a later
// AniList date carries new information.
func laterWins(shikimori, anilist *time.Time) (chosen *time.Time, fromAniList bool) {
	if anilist == nil {
		return shikimori, false
	}
	if shikimori == nil || anilist.After(*shikimori) {
		return anilist, true
	}
	return shikimori, false
}
```

- [ ] **Step 6: Run the test + build to verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run TestLaterWins -v && go build ./... && cd ../../libs/metrics && go build ./...`
Expected: PASS for `TestLaterWins`; both `go build` succeed (domain column + metric compile).

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/domain/anime.go libs/metrics/sync.go \
        services/catalog/internal/service/calendar_anilist.go \
        services/catalog/internal/service/calendar_anilist_test.go
git commit -m "feat(catalog): next_episode_source column, metric, and later-wins rule

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Calendar reconciliation pass

**Files:**
- Modify: `services/catalog/internal/service/calendar_anilist.go` (add interface + reconcile method)
- Modify: `services/catalog/internal/service/catalog.go` (struct field, constructor, `calendarInfo.source`, `SyncCalendar` hook, both write-path propagations)
- Test: `services/catalog/internal/service/calendar_anilist_test.go` (reconcile-with-fake)

**Interfaces:**
- Consumes: `idmapping.AniListAiringByMALID` (Task 1); `laterWins`, `sourceShikimori`, `sourceAniList` (Task 2); `metrics.NextEpisodeSourceTotal` (Task 2).
- Produces:
  - `type AniListAiringFetcher interface { AniListAiringByMALID(ctx context.Context, malID string) (*idmapping.AniListAiring, error) }`
  - `func (s *CatalogService) reconcileCalendarWithAniList(ctx context.Context, seen map[string]*calendarInfo)` — mutates `seen` in place; never errors.
  - `calendarInfo.source string` field.

- [ ] **Step 1: Write the failing reconcile test**

Append to `services/catalog/internal/service/calendar_anilist_test.go` (add imports `context`, `errors`, and the two project packages shown):

```go
type fakeAiringFetcher struct {
	byID map[string]*idmapping.AniListAiring
	err  map[string]error
}

func (f *fakeAiringFetcher) AniListAiringByMALID(_ context.Context, malID string) (*idmapping.AniListAiring, error) {
	if e, ok := f.err[malID]; ok {
		return nil, e
	}
	return f.byID[malID], nil
}

func TestReconcileCalendarWithAniList(t *testing.T) {
	shiki := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	aniLater := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	aniEarlier := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	seen := map[string]*calendarInfo{
		"100": {shikimoriID: "100", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist later → override
		"200": {shikimoriID: "200", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist earlier → keep
		"300": {shikimoriID: "300", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist nil airing → keep
		"400": {shikimoriID: "400", nextEpisodeAt: &shiki, source: sourceShikimori}, // fetch error → keep
		"500": {shikimoriID: "500", nextEpisodeAt: nil, source: sourceShikimori},    // shiki nil, anilist set → adopt
	}
	fake := &fakeAiringFetcher{
		byID: map[string]*idmapping.AniListAiring{
			"100": {AniListID: 1, Status: "RELEASING", NextEpisode: 12, NextAiringAt: &aniLater},
			"200": {AniListID: 2, Status: "RELEASING", NextEpisode: 5, NextAiringAt: &aniEarlier},
			"300": {AniListID: 3, Status: "FINISHED", NextEpisode: 0, NextAiringAt: nil},
			"500": {AniListID: 5, Status: "RELEASING", NextEpisode: 1, NextAiringAt: &aniLater},
		},
		err: map[string]error{"400": errors.New("anilist down")},
	}

	s := &CatalogService{aniListAiring: fake, log: logger.Default()} // aniListReconcilePacing defaults to 0 → no sleeps
	s.reconcileCalendarWithAniList(context.Background(), seen)

	assert := func(id string, wantAt *time.Time, wantSrc string) {
		t.Helper()
		got := seen[id]
		if wantAt == nil && got.nextEpisodeAt != nil {
			t.Errorf("%s: want nil date, got %v", id, got.nextEpisodeAt)
		}
		if wantAt != nil && (got.nextEpisodeAt == nil || !got.nextEpisodeAt.Equal(*wantAt)) {
			t.Errorf("%s: want date %v, got %v", id, wantAt, got.nextEpisodeAt)
		}
		if got.source != wantSrc {
			t.Errorf("%s: want source %q, got %q", id, wantSrc, got.source)
		}
	}
	assert("100", &aniLater, sourceAniList)
	assert("200", &shiki, sourceShikimori)
	assert("300", &shiki, sourceShikimori)
	assert("400", &shiki, sourceShikimori)
	assert("500", &aniLater, sourceAniList)
}
```

Add to the test file's import block:
```go
	"context"
	"errors"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestReconcileCalendarWithAniList -v`
Expected: FAIL — `unknown field 'source' in struct literal of type calendarInfo`, `s.aniListAiring undefined`, and `s.reconcileCalendarWithAniList undefined`.

- [ ] **Step 3: Add the `source` field to `calendarInfo` and initialize it**

In `services/catalog/internal/service/catalog.go`, change the `calendarInfo` struct (line 918) to:

```go
// calendarInfo holds the deduplicated next-episode air time for a calendar anime.
type calendarInfo struct {
	shikimoriID   string
	nextEpisodeAt *time.Time
	source        string // sourceShikimori (default) or sourceAniList after reconciliation
}
```

In `dedupeCalendarEntries` (line 932), set the default source when constructing the record:

```go
		info := &calendarInfo{shikimoriID: id, source: sourceShikimori}
```

- [ ] **Step 4: Add the interface, struct field, pacing field, and constructor wiring**

In `services/catalog/internal/service/calendar_anilist.go`, update the imports and add the interface + reconcile method. The full file now reads:

```go
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// Next-episode date provenance values stored in domain.Anime.NextEpisodeSource.
const (
	sourceShikimori = "shikimori"
	sourceAniList   = "anilist"
)

// AniListAiringFetcher is the slice of *idmapping.Client the calendar reconciler
// needs. Declared as an interface so tests supply a handwritten fake.
type AniListAiringFetcher interface {
	AniListAiringByMALID(ctx context.Context, malID string) (*idmapping.AniListAiring, error)
}

// laterWins picks the later of the Shikimori and AniList next-episode times,
// treating nil as "earliest". It returns the chosen time and whether AniList
// won. AniList is adopted only when it is strictly after Shikimori's date (or
// when Shikimori has no date at all): Shikimori's failure mode is reporting a
// date that is too EARLY (it ignores broadcast hiatuses), so only a later
// AniList date carries new information.
func laterWins(shikimori, anilist *time.Time) (chosen *time.Time, fromAniList bool) {
	if anilist == nil {
		return shikimori, false
	}
	if shikimori == nil || anilist.After(*shikimori) {
		return anilist, true
	}
	return shikimori, false
}

// reconcileCalendarWithAniList corroborates each calendar anime's Shikimori
// next-episode time against AniList's broadcaster schedule, adopting AniList's
// date only when it is strictly later (later-wins). It mutates seen in place
// (nextEpisodeAt + source) and never returns an error — any AniList failure
// leaves the Shikimori value untouched. Calls are paced (~2 req/s) and abort
// promptly on context cancellation.
func (s *CatalogService) reconcileCalendarWithAniList(ctx context.Context, seen map[string]*calendarInfo) {
	if s.aniListAiring == nil {
		return
	}
	first := true
	for _, info := range seen {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !first && s.aniListReconcilePacing > 0 {
			time.Sleep(s.aniListReconcilePacing)
		}
		first = false

		airing, err := s.aniListAiring.AniListAiringByMALID(ctx, info.shikimoriID)
		if err != nil {
			s.log.Debugw("anilist airing lookup failed, keeping shikimori date",
				"shikimori_id", info.shikimoriID, "error", err)
			info.source = sourceShikimori
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceShikimori).Inc()
			continue
		}

		var aniListAt *time.Time
		if airing != nil {
			aniListAt = airing.NextAiringAt
		}
		chosen, fromAniList := laterWins(info.nextEpisodeAt, aniListAt)
		info.nextEpisodeAt = chosen
		if fromAniList {
			info.source = sourceAniList
			s.log.Infow("adopted anilist next-episode date (later than shikimori)",
				"shikimori_id", info.shikimoriID,
				"anilist_episode", airing.NextEpisode,
				"next_episode_at", chosen)
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceAniList).Inc()
		} else {
			info.source = sourceShikimori
			metrics.NextEpisodeSourceTotal.WithLabelValues(sourceShikimori).Inc()
		}
	}
}
```

In `services/catalog/internal/service/catalog.go`, add two fields to the `CatalogService` struct (after `idMappingClient *idmapping.Client`, line 41):

```go
	// aniListAiring resolves AniList's broadcaster airing schedule for the
	// calendar reconciler. Defaults to the same idmapping client; an interface
	// so tests inject a fake.
	aniListAiring AniListAiringFetcher
	// aniListReconcilePacing throttles per-anime AniList calls during calendar
	// reconciliation (~2 req/s in prod; 0 in tests to skip sleeps).
	aniListReconcilePacing time.Duration
```

In `NewCatalogService`, build the idmapping client once and share it. Replace the line `idMappingClient:  idmapping.NewClient(idMapOpts...),` (line 152) and add the two new fields. The relevant slice of the returned struct literal becomes:

```go
	idMapClient := idmapping.NewClient(idMapOpts...)
	return &CatalogService{
		animeRepo:        animeRepo,
		genreRepo:        genreRepo,
		videoRepo:        videoRepo,
		shikimoriClient:  shikimoriClient,
		aniboomClient:    aniboom.NewClient(),
		kodikClient:      kodikClient,
		jikanClient:      jikan.NewClient(),
		jimakuClient:     jimakuClient,
		animelibClient:   animelibClient,
		hanimeClient:     hanimeClient,
		idMappingClient:  idMapClient,
		aniListAiring:    idMapClient,
		aniListReconcilePacing: 500 * time.Millisecond,
		scraperClient:    scraper.NewClient(scraperAPIURL, scraperTimeout),
		cache:            cache,
		log:              log,
		kodikExtractWrap: egressWrap,
	}
```

> Note: `idMapClient` is already egress-wrapped via `idMapOpts` (built from `EgressTransportWrap` above), so reusing it needs **no** `main.go` change — the catalog service already owns the right client.

- [ ] **Step 5: Run the reconcile test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestReconcileCalendarWithAniList|TestLaterWins' -v`
Expected: PASS.

- [ ] **Step 6: Hook reconciliation into `SyncCalendar` and propagate the source**

In `services/catalog/internal/service/catalog.go`, in `SyncCalendar`, immediately after `seen := dedupeCalendarEntries(calendar)` (line 880) add:

```go
	// Corroborate Shikimori's naive next-episode dates against AniList's
	// broadcaster schedule (later-wins). Best-effort: failures keep Shikimori.
	s.reconcileCalendarWithAniList(ctx, seen)
```

In `importMissingCalendarAnime`, replace the override block (lines 1003-1006):

```go
			// Override next_episode_at from reconciled calendar data (more accurate)
			if info, ok := seen[anime.ShikimoriID]; ok && info.nextEpisodeAt != nil {
				anime.NextEpisodeAt = info.nextEpisodeAt
				anime.NextEpisodeSource = info.source
			}
```

In `updateExistingCalendarEpisodes`, replace the assignment (line 1041) so the source is written alongside the date:

```go
		existing.NextEpisodeAt = info.nextEpisodeAt
		existing.NextEpisodeSource = info.source
```

- [ ] **Step 7: Run the full service test suite + build**

Run: `cd services/catalog && go build ./... && go test ./internal/service/ -count=1`
Expected: build succeeds; all service tests pass (no regressions in calendar dedupe / partition tests, plus the new ones).

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/service/calendar_anilist.go \
        services/catalog/internal/service/calendar_anilist_test.go \
        services/catalog/internal/service/catalog.go
git commit -m "feat(catalog): corroborate calendar next-episode dates with AniList

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Batch-refresh clobber guard

**Files:**
- Modify: `services/catalog/internal/service/calendar_anilist.go` (add `defendAniListNextEpisode`)
- Modify: `services/catalog/internal/service/catalog.go:665-683` (`refreshStaleAnime` calls the guard)
- Test: `services/catalog/internal/service/calendar_anilist_test.go` (defend-guard)

**Interfaces:**
- Consumes: `sourceShikimori`, `sourceAniList` (Task 2); `domain.Anime` (Task 2 column).
- Produces: `func defendAniListNextEpisode(fresh, existing *domain.Anime)` — mutates `fresh` in place.

- [ ] **Step 1: Write the failing guard test**

Append to `services/catalog/internal/service/calendar_anilist_test.go` (add `"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"` to imports):

```go
func TestDefendAniListNextEpisode(t *testing.T) {
	ani := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	shikiEarlier := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	shikiLater := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("defends anilist date against earlier shikimori refresh", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiEarlier}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(ani) || fresh.NextEpisodeSource != sourceAniList {
			t.Errorf("want defended anilist date, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("shikimori even-later date wins and source reverts", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiLater}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(shikiLater) || fresh.NextEpisodeSource != sourceShikimori {
			t.Errorf("want shikimori-later win, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("non-anilist existing is not defended; default source set", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiEarlier}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceShikimori}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(shikiEarlier) || fresh.NextEpisodeSource != sourceShikimori {
			t.Errorf("want shikimori kept, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("defends when shikimori refresh has no date", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: nil}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(ani) || fresh.NextEpisodeSource != sourceAniList {
			t.Errorf("want defended anilist date when fresh nil, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestDefendAniListNextEpisode -v`
Expected: FAIL — `undefined: defendAniListNextEpisode`.

- [ ] **Step 3: Add the guard helper**

Append to `services/catalog/internal/service/calendar_anilist.go` (add `"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"` to its import block):

```go
// defendAniListNextEpisode preserves an AniList-corroborated next-episode date on
// `fresh` (the Shikimori-rebuilt row from the nightly batch refresh) when the
// stored `existing` row was AniList-sourced and still holds the later date.
// Same later-wins rule as the calendar reconciler: Shikimori only wins if it now
// reports an even-later date (the show resumed and slipped further), in which
// case the source reverts to shikimori. Always stamps a provenance value on
// `fresh` so the full-row Update never writes an empty source.
func defendAniListNextEpisode(fresh, existing *domain.Anime) {
	if fresh.NextEpisodeSource == "" {
		fresh.NextEpisodeSource = sourceShikimori
	}
	if existing.NextEpisodeSource == sourceAniList && existing.NextEpisodeAt != nil &&
		(fresh.NextEpisodeAt == nil || existing.NextEpisodeAt.After(*fresh.NextEpisodeAt)) {
		fresh.NextEpisodeAt = existing.NextEpisodeAt
		fresh.NextEpisodeSource = existing.NextEpisodeSource
	}
}
```

- [ ] **Step 4: Call the guard from `refreshStaleAnime`**

In `services/catalog/internal/service/catalog.go`, in `refreshStaleAnime`, immediately after `fresh.CreatedAt = existing.CreatedAt` (line 669) add:

```go

	// Defend an AniList-corroborated next-episode date against this Shikimori
	// batch refresh, which would otherwise clobber the correction.
	defendAniListNextEpisode(fresh, existing)
```

- [ ] **Step 5: Run the guard test + full build/test**

Run: `cd services/catalog && go test ./internal/service/ -run TestDefendAniListNextEpisode -v && go build ./... && go test ./internal/service/ -count=1`
Expected: PASS for the guard test; build succeeds; full service suite green.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/service/calendar_anilist.go \
        services/catalog/internal/service/calendar_anilist_test.go \
        services/catalog/internal/service/catalog.go
git commit -m "feat(catalog): guard AniList next-episode dates from batch-refresh clobber

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Final Verification

After all four tasks:

- [ ] `cd libs/idmapping && go test ./... -count=1` — idmapping green.
- [ ] `cd libs/metrics && go build ./...` — metric compiles.
- [ ] `cd services/catalog && go build ./... && go test ./internal/service/ -count=1` — catalog green.
- [ ] `go vet ./...` from `services/catalog` — no vet issues.
- [ ] Deploy + document via `/animeenigma-after-update` (lints, redeploys catalog, changelog in Russian Trump-mode, push). The catalog column auto-migrates on startup; verify `make logs-catalog` shows a clean boot and the next nightly calendar sync logs `adopted anilist next-episode date` for at least one ongoing hiatus title (Re:Zero S4 is the canonical check: `shikimori_id` 52991-family → ep12 should land on Aug 12, not July 1).

---

## Self-Review

**Spec coverage:**
- Spec §3 (architecture / single GraphQL call, reuse IPv4+egress transport) → Task 1 + Task 3 Step 4 note (no main.go change).
- Spec §4 (`AniListAiringByMALID` + type + shared helper, refactor `resolveAniList`) → Task 1.
- Spec §5 (later-wins, exact "any later", ongoing/calendar-scoped, both write paths) → Task 2 (`laterWins`) + Task 3 (reconcile at the `seen` layer feeding both write paths). **Refinement vs spec:** reconciliation runs once over the deduped `seen` map rather than being duplicated into each write path — strictly cleaner (one AniList call per anime) and same observable behavior; calendar-scope is inherent because `seen` is built only inside `SyncCalendar`.
- Spec §6 (`next_episode_source` column + `refreshStaleAnime` guard) → Task 2 (column) + Task 4 (guard, extracted as the pure `defendAniListNextEpisode` so it is unit-testable without a DB).
- Spec §7 (fail-safe, sequential pacing ≤~2 req/s, no Redis, metric + override log) → Task 3 (`reconcileCalendarWithAniList`) + Task 2 (metric).
- Spec §8 (idmapping stub tests; reconcile/guard fakes) → Task 1 + Task 3 + Task 4 tests.
- Spec §9 (effort metrics) → Global Constraints.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output.

**Type consistency:** `AniListAiring{AniListID,Status,NextEpisode,NextAiringAt}` is identical across Task 1 (definition), Task 3 (fake + reconcile use). `sourceShikimori`/`sourceAniList` consts defined once (Task 2), used in Tasks 3-4. `laterWins(shikimori, anilist *time.Time) (*time.Time, bool)` signature matches its callers. `calendarInfo.source` added in Task 3 Step 3 before first use. `aniListAiring AniListAiringFetcher` field + `aniListReconcilePacing` added in Task 3 Step 4, referenced by the reconcile method in the same step and the test in Step 1.
