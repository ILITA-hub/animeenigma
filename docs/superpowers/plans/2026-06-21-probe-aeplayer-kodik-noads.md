# Probe Coverage: aePlayer + no-ads Kodik — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Extend the analytics playback probe to cover `ae` (3 latest distinct-anime library uploads) and a new `kodik-noads` provider (scraped ad-free HLS), splitting basic `kodik` → `kodik-iframe`.

**Architecture:** Engine gains a `[]ProbeTarget{Provider, AnimeSet, Resolver}` registry (shared Validator). New library `/internal/library/recent-episodes` + catalog `/internal/probe/ae-targets` feed the ae anime-set; new `AeResolver`/`AeAnimeSet`/`KodikNoadsResolver` in analytics. Roster gets a guarded rename+insert migration.

**Tech Stack:** Go (chi, GORM), ClickHouse, Prometheus, Grafana provisioning.

## Global Constraints

- Probe reason codes come ONLY from `libs/streamprobe` (`playable, empty_response, cdn_unreachable, status_403, decode_failed, invalid_video, ...`). Do not invent new reason strings.
- `/internal/*` endpoints are Docker-network-only (gateway never proxies them). No auth middleware needed beyond the existing internal-router group.
- The functional data-source key `"kodik"` (capability families, parsers, FE, watch-together, `has_kodik`) MUST stay untouched. The rename applies ONLY to the `stream_providers` row + the catalog `health_checker` liveness label (`providerKodik`).
- Roster table is `stream_providers` (`domain.ScraperProvider.TableName()`). Migrations are guarded + idempotent, mirroring `scraperprovider.AnimefeverDeclaim`.
- Every target must produce ≥1 verdict per run (synthetic `empty_response` when its anime-set is empty) so it always appears on the dashboard rather than "— not probed".
- Tests: table-driven, fakes only, no live API calls.
- Effort/impact metrics use UXΔ / CDI / MVQ (see `.planning/CONVENTIONS.md`), never time units.

---

### Task 1: Library — recent-episodes internal endpoint

**Files:**
- Modify: `services/library/internal/repo/episode.go`
- Modify: `services/library/internal/handler/` (add handler; match existing handler file style)
- Modify: `services/library/internal/transport/router.go`
- Test: `services/library/internal/repo/episode_test.go` (+ handler test if a handler test file exists)

**Interfaces:**
- Produces: `GET /internal/library/recent-episodes?limit=N` → `{"episodes":[{"shikimori_id":"...","episode_number":3}]}` — newest distinct `shikimori_id` (one newest episode per anime), `ORDER BY created_at DESC`, deduped, capped at N (default 3, max 20).
- Consumes (Task 2): catalog library client calls this.

- [ ] **Step 1: Write the failing repo test** — in `episode_test.go`, insert library_episodes rows across ≥2 shikimori_ids with differing `created_at`; assert `ListRecentDistinct(ctx, 3)` returns one row per distinct shikimori_id, newest-first, len ≤ 3, each row's `episode_number` is that anime's newest. Use the existing testcontainer/sqlite harness pattern in the package.

- [ ] **Step 2: Run it, verify it fails** — `cd services/library && go test ./internal/repo/ -run ListRecentDistinct -v` → FAIL (undefined).

- [ ] **Step 3: Implement `ListRecentDistinct`** — add to `EpisodeRepository`:
```go
// ListRecentDistinct returns the newest episode of each distinct anime, newest
// upload first, capped at limit. Used by the playback probe's ae target set.
func (r *EpisodeRepository) ListRecentDistinct(ctx context.Context, limit int) ([]domain.Episode, error) {
	if limit <= 0 || limit > 20 {
		limit = 3
	}
	var eps []domain.Episode
	// DISTINCT ON (shikimori_id) keeping the newest created_at per anime, then the
	// newest anime overall. Postgres-specific; matches the library's Postgres DB.
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (shikimori_id) *
		FROM library_episodes
		ORDER BY shikimori_id, created_at DESC, episode_number DESC
	`).Scan(&eps).Error
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list recent distinct episodes")
	}
	// Re-sort the distinct set by recency and cap. (DISTINCT ON forces the first
	// ORDER BY key; the global recency sort + cap is applied in Go.)
	sort.Slice(eps, func(i, j int) bool { return eps[i].CreatedAt.After(eps[j].CreatedAt) })
	if len(eps) > limit {
		eps = eps[:limit]
	}
	return eps, nil
}
```
(Add `sort` import. If the package's tests run on sqlite not Postgres, instead implement with a portable two-step: load `ORDER BY created_at DESC`, dedupe by shikimori_id in Go, cap at limit — pick whichever matches the package's existing test DB. Prefer the portable Go-dedupe version if unsure.)

- [ ] **Step 4: Run repo test → PASS**.

- [ ] **Step 5: Add handler + route** — add `GetRecentEpisodes` handler (mirror an existing library handler's struct/ctor/error style) reading `?limit=`, calling `ListRecentDistinct`, responding `{"episodes":[{"shikimori_id","episode_number"}]}`. Register under the internal group in `router.go` next to other `/internal/...` or library routes: `r.Get("/internal/library/recent-episodes", h.GetRecentEpisodes)`. Confirm the route is NOT behind the admin/JWT middleware (internal-only, like the autocache internal endpoints).

- [ ] **Step 6: Build + test** — `cd services/library && go build ./... && go test ./internal/repo/ ./internal/handler/ 2>&1 | tail`.

- [ ] **Step 7: Commit** — `git add services/library/... && git commit -m "feat(library): /internal/library/recent-episodes (newest distinct-anime uploads) for the ae probe"`.

---

### Task 2: Catalog — library client method + `/internal/probe/ae-targets`

**Files:**
- Modify: `services/catalog/internal/parser/library/client.go` (add `RecentEpisodes`)
- Create: `services/catalog/internal/handler/internal_probe.go`
- Modify: `services/catalog/internal/transport/router.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (construct + wire handler)
- Test: `services/catalog/internal/handler/internal_probe_test.go`

**Interfaces:**
- Consumes: Task 1's `GET /internal/library/recent-episodes`; `AnimeRepository.GetByShikimoriID(ctx, shikimoriID) (*domain.Anime, error)`.
- Produces: `GET /internal/probe/ae-targets?limit=N` → `{"targets":[{"uuid":"...","name":"...","episode":3}]}`. Maps each library shikimori_id → catalog anime (UUID + display name). Skips shikimori_ids with no catalog row (logged). `name` prefers `name_ru` else `name_en` else `name`.

- [ ] **Step 1: Add library client method** — in `parser/library/client.go`:
```go
type RecentEpisode struct {
	ShikimoriID   string `json:"shikimori_id"`
	EpisodeNumber int    `json:"episode_number"`
}
type recentEpisodesResponse struct {
	Episodes []RecentEpisode `json:"episodes"`
}

// RecentEpisodes returns the newest distinct-anime library uploads. Empty slice
// (nil error) when the library is unconfigured or returns 404.
func (c *Client) RecentEpisodes(ctx context.Context, limit int) ([]RecentEpisode, error) {
	if c == nil || c.cfg.APIURL == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s/internal/library/recent-episodes?limit=%d", c.cfg.APIURL, limit)
	// ... mirror the existing GetEpisode request/decoval style (2s timeout, 404→nil,nil, 5xx→error)
}
```

- [ ] **Step 2: Write the failing handler test** — `internal_probe_test.go`: fake library client returning 2 recent episodes (one mapped to a catalog anime, one with an unknown shikimori_id) + a fake anime repo. Assert the response contains ONLY the mapped target with correct `{uuid, name, episode}` and the unmapped one is skipped. Run → FAIL.

- [ ] **Step 3: Implement the handler** — `internal_probe.go`:
```go
type AeTarget struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Episode int    `json:"episode"`
}
type aeTargetsResponse struct {
	Targets []AeTarget `json:"targets"`
}

type InternalProbeHandler struct {
	lib  libraryRecentLister      // interface: RecentEpisodes(ctx, limit) ([]library.RecentEpisode, error)
	repo animeByShikimori          // interface: GetByShikimoriID(ctx, id) (*domain.Anime, error)
	log  *logger.Logger
}

func (h *InternalProbeHandler) AeTargets(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), 3, 20)
	recent, err := h.lib.RecentEpisodes(r.Context(), limit)
	if err != nil { httputil.Error(w, err); return }
	out := make([]AeTarget, 0, len(recent))
	for _, re := range recent {
		a, err := h.repo.GetByShikimoriID(r.Context(), re.ShikimoriID)
		if err != nil || a == nil { continue } // not in catalog → skip
		out = append(out, AeTarget{UUID: a.ID, Name: pickName(a), Episode: re.EpisodeNumber})
	}
	httputil.OK(w, aeTargetsResponse{Targets: out})
}
```
Define the two narrow interfaces locally (don't import concrete repo). `pickName` = name_ru→name_en→name. Add `parseLimit` helper or reuse an existing one.

- [ ] **Step 4: Run handler test → PASS**.

- [ ] **Step 5: Wire route + main** — `router.go` internal group: `r.Get("/internal/probe/ae-targets", internalProbeHandler.AeTargets)`. Construct `InternalProbeHandler` in `main.go` with the existing library client + anime repo + logger.

- [ ] **Step 6: Build + test** — `cd services/catalog && go build ./... && go test ./internal/handler/ -run Probe 2>&1 | tail`.

- [ ] **Step 7: Commit**.

---

### Task 3: Analytics probe — `AnimeRef.Episode`, Resolver episode param, `ProbeTarget`, Engine refactor

**Files:**
- Modify: `services/analytics/internal/probe/types.go` (AnimeRef.Episode, SlotLibraryLatest)
- Modify: `services/analytics/internal/probe/resolver.go` (interface + HTTPResolver signature)
- Modify: `services/analytics/internal/probe/engine.go` (ProbeTarget, []ProbeTarget, RunOnce, synthetic-empty verdict)
- Modify: `services/analytics/internal/probe/engine_test.go`, `resolver_test.go` (signature updates + new dispatch/isolation tests)

**Interfaces:**
- Produces: `ProbeTarget{Provider string; AnimeSet AnimeSetResolver; Resolver Resolver}`; `Resolver.Resolve(ctx, animeUUID, animeName string, episode int, slot AnimeSlot, provider string)`; `NewEngine(targets []ProbeTarget, val Validator, rep Reporter, now func() int64, log *logger.Logger)`.

- [ ] **Step 1: types.go** — add `Episode int` to `AnimeRef`; add `SlotLibraryLatest AnimeSlot = "library_latest"`.

- [ ] **Step 2: resolver.go interface + HTTPResolver** — change interface to `Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error)`. In `HTTPResolver.Resolve`, add the `episode int` param and ignore it with a comment (`// episode unused: the scraper set always probes episode 1`).

- [ ] **Step 3: engine.go — ProbeTarget + refactor**. Replace the Engine struct + NewEngine + RunOnce + probeProvider:
```go
type ProbeTarget struct {
	Provider string
	AnimeSet AnimeSetResolver
	Resolver Resolver
}

type Engine struct {
	targets []ProbeTarget
	val     Validator
	rep     Reporter
	now     func() int64
	log     *logger.Logger
}

func NewEngine(targets []ProbeTarget, val Validator, rep Reporter, now func() int64, log *logger.Logger) *Engine {
	return &Engine{targets: targets, val: val, rep: rep, now: now, log: log}
}

func (e *Engine) probeProvider(ctx context.Context, t ProbeTarget, refs []AnimeRef) (verdicts []Verdict) {
	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorw("probe provider panicked", "provider", t.Provider, "panic", r)
			}
			verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageStream, Reason: streamprobe.ReasonCDNUnreachable})
		}
	}()
	for _, ref := range refs {
		streams, stage, rerr := t.Resolver.Resolve(ctx, ref.UUID, ref.Name, ref.Episode, ref.Slot, t.Provider)
		if rerr != nil {
			verdicts = append(verdicts, Verdict{
				Provider: t.Provider, AnimeUUID: ref.UUID, AnimeName: ref.Name, Slot: ref.Slot, Stage: stage,
				Reason: streamprobe.ReasonCDNUnreachable,
			})
			continue
		}
		for _, s := range streams {
			verdicts = append(verdicts, e.val.Validate(ctx, s))
		}
	}
	// Guarantee at least one verdict so the provider always appears on the
	// dashboard (a target whose anime-set is empty would otherwise vanish).
	if len(verdicts) == 0 {
		verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageEpisodes, Reason: streamprobe.ReasonEmptyResponse})
	}
	return verdicts
}

func (e *Engine) RunOnce(ctx context.Context) error {
	var allVerdicts []Verdict
	var provVerdicts []ProviderVerdict
	for _, t := range e.targets {
		refs, _ := t.AnimeSet.Resolve(ctx) // empty refs → synthetic verdict in probeProvider
		verdicts := e.probeProvider(ctx, t, refs)
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, Rollup(t.Provider, verdicts))
	}
	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
```

- [ ] **Step 4: Update existing tests** — in `engine_test.go`: `fakeRes`/`errRes`/`panicRes` Resolve signatures gain `episode int`; build `[]ProbeTarget` instead of `(providers, as, res)`. `TestEngine_RunOnce` builds `NewEngine([]ProbeTarget{{Provider:"gogoanime", AnimeSet: fakeAS{}, Resolver: fakeRes{}}}, fakeVal{}, rep, ...)`. Keep the panic-isolation and resolve-error assertions. In `resolver_test.go`: `r.Resolve(ctx, "uuid1", "Frieren", 0, SlotAnchor, "gogoanime")`.

- [ ] **Step 5: Add new tests** — `TestEngine_PerTargetDispatch`: two targets with different fake resolvers/anime-sets both probed in one run, each provider's verdicts use its own resolver. `TestEngine_EmptyAnimeSet_SyntheticDown`: a target whose AnimeSet returns `nil` yields one `empty_response` verdict and `Rollup → StatusDown`.

- [ ] **Step 6: Build + test** — `cd services/analytics && go build ./... && go test ./internal/probe/ 2>&1 | tail`.

- [ ] **Step 7: Commit**.

---

### Task 4: Analytics probe — `AeResolver` + `AeAnimeSet`

**Files:**
- Create: `services/analytics/internal/probe/ae.go`
- Test: `services/analytics/internal/probe/ae_test.go`

**Interfaces:**
- Consumes: catalog `/internal/probe/ae-targets?limit=N` and `/api/anime/{uuid}/ae/stream?episode=N`; the `Resolver`/`AnimeSetResolver` interfaces from Task 3.
- Produces: `NewAeAnimeSet(catalogBaseURL string, limit int, hc *http.Client) *AeAnimeSet` (impl `AnimeSetResolver`); `NewAeResolver(catalogBaseURL string, hc *http.Client) *AeResolver` (impl `Resolver`).

- [ ] **Step 1: Write failing tests** — `ae_test.go` with httptest servers:
  - `TestAeAnimeSet_Resolve`: server returns `{"targets":[{"uuid":"u1","name":"Frieren","episode":28}]}` → refs `[{UUID:"u1", Name:"Frieren", Episode:28, Slot:SlotLibraryLatest}]`.
  - `TestAeAnimeSet_Empty`: 500 or empty targets → `nil` refs, nil err.
  - `TestAeResolver_HappyPath`: server `/ae/stream` returns `{"url":"http://minio/x.m3u8","exp":"9","sig":"ab"}` → one ResolvedStream{MasterURL, Exp, Sig, AnimeName, Stage:StageStream}, no error.
  - `TestAeResolver_NoStream`: empty `url` / non-200 → stage `StageStream`, error.
  Run → FAIL.

- [ ] **Step 2: Implement `ae.go`**:
```go
type AeAnimeSet struct{ base string; limit int; hc *http.Client }
func NewAeAnimeSet(base string, limit int, hc *http.Client) *AeAnimeSet { /* default client, trim slash, default limit 3 */ }
func (a *AeAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	// GET base + "/internal/probe/ae-targets?limit=" + limit
	// decode {"targets":[{uuid,name,episode}]}; non-200 / error → return nil, nil (graceful)
	// map → []AnimeRef{ {UUID, Name, Episode, Slot: SlotLibraryLatest} }
}

type AeResolver struct{ base string; hc *http.Client }
func NewAeResolver(base string, hc *http.Client) *AeResolver { /* ... */ }
func (r *AeResolver) Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	// GET base + "/api/anime/" + animeUUID + "/ae/stream?episode=" + episode
	// decode {data?:{url,exp,sig}} OR flat {url,exp,sig} (handle both envelope shapes)
	// empty url / non-200 → return nil, StageStream, error
	// return []ResolvedStream{{Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot, Server: "library", MasterURL: url, Exp: exp, Sig: sig, Stage: StageStream}}, StageStream, nil
}
```
Note: `provider` arg will be `"ae"`. `Server:"library"` gives a stable locus label. Handle BOTH the `{data:{...}}` envelope and a flat body (the ae/stream handler uses `httputil.OK` which may wrap in `{success,data}` — decode defensively, mirroring `animeset.go`'s anchor fetch).

- [ ] **Step 3: Run ae_test → PASS**.

- [ ] **Step 4: Build + test** — `go build ./... && go test ./internal/probe/ -run Ae 2>&1 | tail`.

- [ ] **Step 5: Commit**.

---

### Task 5: Analytics probe — `KodikNoadsResolver`

**Files:**
- Create: `services/analytics/internal/probe/kodiknoads.go`
- Test: `services/analytics/internal/probe/kodiknoads_test.go`

**Interfaces:**
- Consumes: catalog `/api/anime/{uuid}/kodik/translations` → `[{id,title,type,episodes_count,pinned}]` and `/api/anime/{uuid}/kodik/stream?episode=1&translation=ID` → `{stream_url, referer, ...}`; the `Resolver` interface.
- Produces: `NewKodikNoadsResolver(catalogBaseURL string, hc *http.Client) *KodikNoadsResolver` (impl `Resolver`).

- [ ] **Step 1: Write failing tests** — `kodiknoads_test.go` with httptest:
  - `TestKodikNoads_HappyPath`: translations returns 2 (one `pinned:true`) → resolver picks the pinned id; stream returns `{"stream_url":"https://cloud.solodcdn.com/m.m3u8","referer":"https://kodik/"}` → ResolvedStream{MasterURL, Referer:"https://kodik/", Stage:StageStream}.
  - `TestKodikNoads_FirstWhenNoPinned`: no pinned → picks first.
  - `TestKodikNoads_NoTranslations`: empty list → `StageServers`, error (so rollup → down/empty).
  Decode the `{success,data}` envelope (catalog uses `httputil.OK`).

- [ ] **Step 2: Implement `kodiknoads.go`**:
```go
func (r *KodikNoadsResolver) Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	// 1) GET /api/anime/{uuid}/kodik/translations → []{id,title,pinned}
	//    decode {data:[...]} envelope. none → return nil, StageServers, errNoTranslations
	//    pick: first Pinned; else translations[0]
	// 2) ep := episode; if ep == 0 { ep = 1 }
	//    GET /api/anime/{uuid}/kodik/stream?episode=ep&translation=<id>
	//    decode {data:{stream_url,referer}}. empty stream_url / non-200 → nil, StageStream, err
	// 3) return []ResolvedStream{{Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot,
	//      Server: "kodik-"+strconv.Itoa(id), MasterURL: stream_url, Referer: referer, Stage: StageStream}}, StageStream, nil
}
```
`provider` arg will be `"kodik-noads"`.

- [ ] **Step 3: Run → PASS**.

- [ ] **Step 4: Build + test** — `go test ./internal/probe/ -run Kodik 2>&1 | tail`.

- [ ] **Step 5: Commit**.

---

### Task 6: Analytics — main.go target registry + config

**Files:**
- Modify: `services/analytics/internal/config/config.go` (default `PROBE_PROVIDERS`)
- Modify: `services/analytics/cmd/analytics-api/main.go` (build `[]ProbeTarget`)

**Interfaces:**
- Consumes: Tasks 3–5 constructors; existing `NewHTTPResolver`/`NewHTTPValidator`/`NewHTTPAnimeSet`.

- [ ] **Step 1: Config default** — change `ProbeProviders` default to `"gogoanime,miruro,allanime,nineanime,animefever,ae,kodik-noads"`. Add a comment that the order is the dashboard tie-break order.

- [ ] **Step 2: Build the target registry in main.go** — replace the single resolver/animeSet wiring with:
```go
spotlight := probe.NewHTTPAnimeSet(cfg.CatalogURL, cfg.ProbeAnchorUUID, nil, rand.New(rand.NewSource(time.Now().UnixNano()))) //nolint:gosec
scraperRes := probe.NewHTTPResolver(cfg.CatalogURL, nil)

// provider name → how to build its target (anime-set + resolver)
build := map[string]func() probe.ProbeTarget{
	"ae":          func() probe.ProbeTarget { return probe.ProbeTarget{Provider: "ae", AnimeSet: probe.NewAeAnimeSet(cfg.CatalogURL, 3, nil), Resolver: probe.NewAeResolver(cfg.CatalogURL, nil)} },
	"kodik-noads": func() probe.ProbeTarget { return probe.ProbeTarget{Provider: "kodik-noads", AnimeSet: spotlight, Resolver: probe.NewKodikNoadsResolver(cfg.CatalogURL, nil)} },
}
var targets []probe.ProbeTarget
for _, name := range strings.Split(cfg.ProbeProviders, ",") {
	name = strings.TrimSpace(name)
	if name == "" { continue }
	if b, ok := build[name]; ok {
		targets = append(targets, b())
		continue
	}
	// default: EN scraper provider (shared spotlight set + scraper resolver)
	targets = append(targets, probe.ProbeTarget{Provider: name, AnimeSet: spotlight, Resolver: scraperRes})
}
engine := probe.NewEngine(targets, validator, probe.NewPromReporter(chStore), func() int64 { return time.Now().Unix() }, log)
```

- [ ] **Step 3: Build** — `cd services/analytics && go build ./... 2>&1 | tail`. (No new unit test; integration verified at deploy.)

- [ ] **Step 4: Commit**.

---

### Task 7: Roster — split kodik, add kodik-noads, health label

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go`
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (wire the migration after seed/backfill)
- Modify: `services/catalog/internal/service/health_checker.go` (`providerKodik = "kodik-iframe"`)
- Test: `services/catalog/internal/service/scraperprovider/*_test.go`

**Interfaces:**
- Produces: `stream_providers` rows `kodik-iframe` (was `kodik`) and `kodik-noads`; both group `ru`, `scraper_operated=false`.

- [ ] **Step 1: seed.go** — in `defaultProviders`, rename the `kodik` entry to `Name: "kodik-iframe"` with `Reason/Description` marking it the iframe embed ("RU iframe embed — playback not probeable (no direct stream)"). Add a new entry `Name: "kodik-noads", Status: StatusEnabled, SupportsSub: true` with Description "Ad-free scraped Kodik HLS (kodikextract) — direct stream, playback-probed." In `intrinsicGroups`, replace `"kodik": "ru"` with `"kodik-iframe": "ru"` and add `"kodik-noads": "ru"`.

- [ ] **Step 2: Write failing migration test** — assert after running the guarded migration on a DB seeded with a legacy `kodik` row: (a) no row named `kodik` remains, (b) a `kodik-iframe` row exists (status preserved), (c) a `kodik-noads` row exists with group `ru`, (d) re-running is idempotent (no error, no dup). Mirror the existing `AnimefeverDeclaim` test harness.

- [ ] **Step 3: Implement migration** — in `migrate.go`, add `SplitKodik(db *gorm.DB) error` mirroring `AnimefeverDeclaim`'s guard pattern:
```go
// SplitKodik renames the legacy single kodik row to kodik-iframe (the un-probeable
// embed) and inserts kodik-noads (the scraped ad-free HLS). Guard key keeps it
// one-shot; idempotent on re-run.
func SplitKodik(db *gorm.DB) error {
	const guard = "split_kodik_2026_06_21"
	// if guard present → return nil
	// UPDATE stream_providers SET name='kodik-iframe' WHERE name='kodik'
	// INSERT kodik-noads if absent (group='ru', status='enabled', scraper_operated=false, description=...)
	// record guard
}
```
Use the same guard-table mechanism `AnimefeverDeclaim` uses. The INSERT must set `group` + `scraper_operated` explicitly (intrinsic). Make the rename a no-op when `kodik` is already absent.

- [ ] **Step 4: Wire in main.go** — call `scraperprovider.SplitKodik(db.DB)` right after the existing `AnimefeverDeclaim` / seed / backfill block (after the table rename + AutoMigrate + SeedDefaults, before `EmitCatalogSideRoster`).

- [ ] **Step 5: health_checker.go** — change `providerKodik = "kodik"` → `providerKodik = "kodik-iframe"` so the liveness gauge label matches the roster row. (Leave `checkKodik` logic unchanged.)

- [ ] **Step 6: Run tests** — `cd services/catalog && go test ./internal/service/scraperprovider/ 2>&1 | tail`.

- [ ] **Step 7: Commit**.

---

### Task 8: Deploy + live verification (no new code unless a gap is found)

**Files:** none expected. If live verification shows the merged dashboard needs a tweak (e.g. an explicit Playability note for `kodik-iframe`), edit `docker/grafana/dashboards/playback-health.json` minimally and restart Grafana.

- [ ] **Step 1** — Deploy from a clean origin/main worktree: `./deploy/scripts/redeploy.sh library catalog analytics` (order: library + catalog first so their internal endpoints exist before analytics probes).
- [ ] **Step 2** — Verify roster: `docker exec animeenigma-postgres psql -U postgres -d animeenigma -tAc "SELECT name,\"group\",status FROM stream_providers WHERE name LIKE 'kodik%'"` → `kodik-iframe`, `kodik-noads` present; no `kodik`.
- [ ] **Step 3** — Verify catalog ae-targets + library recent: `docker exec animeenigma-catalog wget -qO- 'http://localhost:8081/internal/probe/ae-targets?limit=3'` returns `{targets:[...]}`.
- [ ] **Step 4** — Trigger a probe run: `docker exec animeenigma-scheduler wget -qO- --post-data='' http://analytics:8092/internal/probe/run`. Wait for completion.
- [ ] **Step 5** — Verify probe_runs has `ae` + `kodik-noads`: `docker exec animeenigma-clickhouse clickhouse-client -d analytics -q "SELECT provider, anime_name, reason, playable FROM probe_runs WHERE run_ts=(SELECT max(run_ts) FROM probe_runs) AND provider IN ('ae','kodik-noads') ORDER BY provider"`.
- [ ] **Step 6** — Verify `probe_provider_up{provider="ae"}` and `{provider="kodik-noads"}` exist in `curl -s http://localhost:8092/metrics | grep probe_provider_up`.
- [ ] **Step 7** — Confirm the merged dashboard panel shows the two new providers (Postgres roster row + ClickHouse playability join) via `/api/ds/query` as in the prior session. `kodik-iframe` shows "— not probed" (correct).
- [ ] **Step 8** — `make health`; run `/animeenigma-after-update` for changelog + push (user-facing entry: honest radar now covers the self-hosted ae library + ad-free Kodik).
