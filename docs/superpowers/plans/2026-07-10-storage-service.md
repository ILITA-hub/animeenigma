# Storage Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Single internal storage service (`services/storage`, :8099) that owns MinIO + external S3 placement for all user content; library/upscaler write through it; ae provider gains a per-episode Local/Cloud server choice; ~24GB of autocache content migrates to S3.

**Spec:** `docs/superpowers/specs/2026-07-10-storage-service-design.md` — read it first; it is the authority on policy/API shapes.

**Architecture:** Stateless control-plane service handing out presigned PUT/GET URLs and doing server-side move/delete/copy across two minio-go clients (`minio:9000` bucket `raw-library`; `s3.firstvds.ru` bucket `raw-library`, SSL). Consumers use `libs/storageclient`. Owning services record the returned `storage` id (`minio`|`s3`) in their domain rows. Reads keep the existing player→hls-proxy path; `libs/videoutils` presign seam becomes multi-storage.

**Tech Stack:** Go (chi-style routers as in sibling services), minio-go v7, GORM + hand SQL migration, Vue 3 + TS frontend.

**Metrics (per .planning/CONVENTIONS.md):** UXΔ = +2 (Better) · CDI = 0.05 * 34 · MVQ = Kraken 88%/85%

## Global Constraints

- Storage ids are exactly `minio` and `s3` (wire + DB + combo.server). Content classes: `library-auto`, `library-manual`, `upscaled`.
- Policy defaults: `library-auto`→`s3`, `library-manual`→`minio` (admin may override per job), `upscaled`→`s3`.
- Player server labels: `Local` (minio), `Cloud` (s3). Default = Local when both copies exist.
- Both buckets are named `raw-library`; key layout identical on both backends (`autocache.RawPrefix` unchanged).
- Port 8099, `/internal/storage/*` only — NO gateway route. Expose `/metrics` like sibling services.
- Go conventions per CLAUDE.md (`libs/errors`, `libs/logger`, service layout). Frontend: bun, DS tokens only, i18n en/ru/ja parity.
- New libs module gotcha (from memory): update `go.work`, importer `go.mod`, and EVERY go.work-covered Dockerfile (`COPY libs/storageclient/go.mod ...` line), then `go work sync`.
- Never run `gofmt -w`/`make fmt` (smart-quote landmine); commit with explicit pathspecs; push after each commit.
- Secrets: S3 creds already exist in `docker/.env` as `S3_ACCESS_KEY`/`S3_SECRET_KEY`; compose interpolates — never hardcode.

---

### Task 1: `services/storage` — the service itself

**Files:**
- Create: `services/storage/go.mod`, `services/storage/Dockerfile` (copy pattern from `services/watch-together/`)
- Create: `services/storage/cmd/storage-api/main.go`
- Create: `services/storage/internal/config/config.go`
- Create: `services/storage/internal/domain/storage.go`
- Create: `services/storage/internal/service/placement.go` + `placement_test.go`
- Create: `services/storage/internal/service/backends.go`
- Create: `services/storage/internal/handler/storage.go` + `storage_test.go`
- Create: `services/storage/internal/transport/router.go`
- Modify: `go.work` (add `services/storage`), `docker/docker-compose.yml` (service block), `deploy/kustomize/` only if sibling services have entries there (mirror watch-together; skip if none)

**Interfaces (Produces — later tasks rely on these exact shapes):**

```go
// domain/storage.go
const (
    BackendMinio = "minio"
    BackendS3    = "s3"
)
const (
    ClassLibraryAuto   = "library-auto"
    ClassLibraryManual = "library-manual"
    ClassUpscaled      = "upscaled"
)

type IngestURLsRequest struct {
    Class    string   `json:"class"`
    Prefix   string   `json:"prefix"`   // trailing slash, bucket-relative
    Files    []string `json:"files"`    // basenames
    Override string   `json:"override"` // "", "minio", "s3" — only honored for library-manual
}
type PutURL struct {
    Name string `json:"name"`
    URL  string `json:"put_url"`
}
type IngestURLsResponse struct {
    Storage   string   `json:"storage"`
    URLs      []PutURL `json:"urls"`
    ExpiresIn int      `json:"expires_in"` // seconds
}
type MoveRequest struct{ Storage, FromPrefix, ToPrefix string } // json: storage, from_prefix, to_prefix
type DeletePrefixRequest struct{ Storage, Prefix string }       // json: storage, prefix
type CopyPrefixRequest struct{ FromStorage, ToStorage, Prefix string } // json: from_storage, to_storage, prefix
type Object struct {
    Key  string `json:"key"` // bucket-relative
    Size int64  `json:"size"`
}
```

HTTP API (all JSON):
- `POST /internal/storage/ingest-urls` → `IngestURLsResponse` (presigned PUTs, 1h expiry; segments/playlist ordering is the caller's job)
- `POST /internal/storage/download-urls` body `{storage, prefix}` → `{urls:[{name,get_url}]}` (presigned GETs for every object under prefix; `name` = key relative to prefix)
- `POST /internal/storage/move` → `{moved:N}` (server-side CopyObject+Remove within one backend)
- `POST /internal/storage/copy` → `{copied:N, bytes:N}` (cross-backend: GET stream from source client → PutObject to target; used by migration)
- `DELETE /internal/storage/prefix` body `DeletePrefixRequest` → `{deleted:N}`
- `GET /internal/storage/list?storage=&prefix=` → `{objects:[Object]}`
- `GET /internal/storage/base-urls` → `{"minio":"http://minio:9000/raw-library","s3":"https://s3.firstvds.ru/raw-library"}` (scheme from UseSSL)
- `GET /internal/storage/health` → 200 `{minio:"up"|"down", s3:"up"|"down"}` (BucketExists probe; 200 even if one down — callers decide)
- `GET /metrics`

**Placement logic (`service/placement.go`) — write test first:**

```go
func (p *Placement) Resolve(class, override string) (string, error) {
    switch class {
    case domain.ClassLibraryAuto, domain.ClassUpscaled, domain.ClassLibraryManual:
    default:
        return "", errors.InvalidArgument("unknown content class: " + class)
    }
    if override != "" {
        if class != domain.ClassLibraryManual {
            return "", errors.InvalidArgument("override only allowed for library-manual")
        }
        if override != domain.BackendMinio && override != domain.BackendS3 {
            return "", errors.InvalidArgument("unknown storage override: " + override)
        }
        return override, nil
    }
    return p.defaults[class], nil // from config; spec defaults above
}
```

**Config env (with defaults):** `STORAGE_PORT=8099`; `STORAGE_MINIO_ENDPOINT=minio:9000`, `STORAGE_MINIO_ACCESS_KEY=minioadmin`, `STORAGE_MINIO_SECRET_KEY=minioadmin`, `STORAGE_MINIO_BUCKET=raw-library`, `STORAGE_MINIO_USE_SSL=false`; `STORAGE_S3_ENDPOINT`, `STORAGE_S3_ACCESS_KEY`, `STORAGE_S3_SECRET_KEY`, `STORAGE_S3_BUCKET=raw-library`, `STORAGE_S3_USE_SSL=true`; `STORAGE_CLASS_LIBRARY_AUTO=s3`, `STORAGE_CLASS_LIBRARY_MANUAL=minio`, `STORAGE_CLASS_UPSCALED=s3`. If `STORAGE_S3_ENDPOINT` is empty the s3 backend is absent: placement resolving to `s3` falls back to `minio` with a warn log (keeps dev environments working).

**Backends (`service/backends.go`):** wrap two `*minio.Client` keyed by storage id (map `map[string]*backend{client,bucket,endpoint,useSSL}`). Presign PUT: `client.PresignedPutObject(ctx, bucket, prefix+name, time.Hour)`. List: `client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})`. Move: for each listed object `CopyObject` to new key then `RemoveObject`. Copy (cross-backend): `src.GetObject` → `dst.PutObject(ctx, bucket, key, reader, obj.Size, minio.PutObjectOptions{ContentType: srcStat.ContentType})`. On startup, `MakeBucket` each backend if `BucketExists` is false (ignore "already exists" races) — mirrors `services/library/internal/minio/writer.go:122-140`.

**Handler tests:** construct handler with a fake `Backends` interface (define `type Backends interface` in handler for exactly this); table-test ingest-urls (auto→s3, manual default→minio, manual override s3→s3, bad class 400, override on auto 400) and health JSON shape. No real MinIO in unit tests.

**Compose block (mirror watch-together's shape):** service name `storage`, `container_name: animeenigma-storage`, build `services/storage`, `ports: ["127.0.0.1:8099:8099"]` (debug only), env from the table above with `STORAGE_S3_ENDPOINT: ${S3_ENDPOINT}`, `STORAGE_S3_ACCESS_KEY: ${S3_ACCESS_KEY}`, `STORAGE_S3_SECRET_KEY: ${S3_SECRET_KEY}`, `depends_on: minio: condition: service_healthy`, healthcheck curl `http://localhost:8099/internal/storage/health`.

**Steps:**
- [ ] Write `placement_test.go` (5 cases above); run `cd services/storage && go test ./internal/service/` — FAIL (package empty)
- [ ] Implement config/domain/placement; test PASSES
- [ ] Write `handler/storage_test.go` against fake Backends (ingest-urls cases + health); FAIL, then implement handler+router+backends+main; PASS
- [ ] `go build ./...` in services/storage; add go.work entry; `go work sync`
- [ ] Add compose block; `make redeploy-storage` (add Makefile target only if `redeploy-%` isn't already pattern-based — check `Makefile` first, it likely is); `curl -s localhost:8099/internal/storage/health` → both `"up"`
- [ ] Verify a real presigned PUT roundtrip by hand: request ingest-urls (class library-manual, prefix `_smoke/1/`, files `["a.txt"]`), `curl -T` a file to the returned URL, `GET /internal/storage/list?storage=minio&prefix=_smoke/` shows it, then `DELETE /internal/storage/prefix` and confirm `{deleted:1}`
- [ ] Commit `services/storage go.work docker/docker-compose.yml Makefile` (pathspec) `feat(storage): internal storage service — S3/MinIO placement authority`; push

### Task 2: `libs/storageclient`

**Files:**
- Create: `libs/storageclient/go.mod` (module `github.com/ILITA-hub/animeenigma/libs/storageclient`), `client.go`, `client_test.go`
- Modify: `go.work`; EVERY go.work-covered service Dockerfile gets `COPY libs/storageclient/go.mod libs/storageclient/go.mod` beside the existing libs COPY lines (grep for `libs/videoutils/go.mod` to find them all); `go work sync`

**Interfaces (Produces):**

```go
package storageclient

func New(baseURL string) *Client // http.Client timeout 30s

type IngestResult struct {
    Storage string
    URLs    []PutURL // {Name, URL}
}
func (c *Client) IngestURLs(ctx context.Context, class, prefix string, files []string, override string) (*IngestResult, error)
// UploadFiles = IngestURLs + concurrent PUTs (errgroup, SetLimit(concurrency)) of every
// non-playlist file, then playlist.m3u8 last on the calling goroutine — port the ordering +
// contentTypeFor map verbatim from services/library/internal/minio/writer.go:199-269.
// PUT sets Content-Type (unsigned header — S3 accepts it) and Content-Length.
func (c *Client) UploadFiles(ctx context.Context, class, override, prefix string, filePaths []string, concurrency int) (storage string, err error)
func (c *Client) Move(ctx context.Context, storage, fromPrefix, toPrefix string) error
func (c *Client) CopyPrefix(ctx context.Context, fromStorage, toStorage, prefix string) (copied int, bytes int64, err error)
func (c *Client) DeletePrefix(ctx context.Context, storage, prefix string) (int, error)
func (c *Client) List(ctx context.Context, storage, prefix string) ([]Object, error) // Object{Key string; Size int64}
func (c *Client) BaseURLs(ctx context.Context) (map[string]string, error) // cached 5 min
func (c *Client) URLFor(ctx context.Context, storage, path string) (string, error) // BaseURLs()[storage] + "/" + path
func (c *Client) DownloadPrefix(ctx context.Context, storage, prefix, destDir string) error // download-urls + GET each to destDir/<name>
```

**Steps:**
- [ ] `client_test.go` with `httptest.Server` faking the storage API: UploadFiles uploads segments before playlist (record PUT order), URLFor caching (one base-urls hit for two calls), DeletePrefix count parse. Run — FAIL
- [ ] Implement; PASS (`cd libs/storageclient && go test ./...`)
- [ ] go.work + Dockerfile COPY sweep + `go work sync`; `go build ./...` from repo root still green for all services
- [ ] Commit (pathspec: `libs/storageclient go.work 'services/*/Dockerfile'`) `feat(libs): storageclient — thin client for the storage service`; push

### Task 3: Library DB — `storage` columns + constraint migration

**Files:**
- Create: `services/library/migrations/017_episode_storage.sql` (bump number to next free; check `ls services/library/migrations/`)
- Modify: `services/library/internal/domain/episode.go:48-77` (+`Storage string \`gorm:"type:text;not null;default:minio" json:"storage"\``), `services/library/internal/domain/job.go:69-95` (+`Storage string \`gorm:"type:text;not null;default:''" json:"storage"\`` — requested override, resolved value written back after upload)
- Modify: `services/library/internal/repo/episode.go` — dup-key mapping unchanged (constraint renamed); add `storage` to any explicit column lists; **evictor query gains `WHERE storage = 'minio'`** (find the evictor's candidate query — `services/library/internal/service/` evictor; it must never pick s3 rows)
- Modify: `services/library/internal/handler/jobs.go:170+` (Create) — accept `storage` in the create payload, validate `''|'minio'|'s3'`, persist on the job
- Test: extend the existing repo/handler test files beside those units (follow their current patterns; if a unit has no test file, add minimal ones for: create two rows same (shik,ep) different storage OK, third with dup storage → AlreadyExists; job create rejects `storage:"tape"`)

**Migration SQL (017_episode_storage.sql):**

```sql
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS storage TEXT NOT NULL DEFAULT 'minio';
ALTER TABLE library_jobs     ADD COLUMN IF NOT EXISTS storage TEXT NOT NULL DEFAULT '';
ALTER TABLE library_episodes DROP CONSTRAINT IF EXISTS library_episodes_shikimori_ep_uniq;
ALTER TABLE library_episodes ADD CONSTRAINT library_episodes_shikimori_ep_storage_uniq
    UNIQUE (shikimori_id, episode_number, storage);
```

Follow the run mechanism of `016_*.sql` exactly (the library service applies migrations at startup — verify how before writing).

**Steps:**
- [ ] Write failing repo test (dual-storage rows); run `cd services/library && go test ./internal/repo/` — FAIL
- [ ] Add migration + struct fields + repo/evictor/jobs changes; test PASS; `go build ./...`
- [ ] Commit (pathspec) `feat(library): episode/job storage column, dual-presence unique constraint`; push

### Task 4: Library — replace embedded MinIO writer with storageclient

**Files:**
- Modify: `services/library/internal/config/config.go:254-261` — replace `MinioConfig` usage with `Storage struct { URL string }` (`LIBRARY_STORAGE_URL`, default `http://storage:8099`); keep `LIBRARY_MINIO_UPLOAD_CONCURRENCY` (rename to `LIBRARY_UPLOAD_CONCURRENCY`, keep old env as fallback)
- Delete: `services/library/internal/minio/writer.go` (whole package) — after all callers are moved
- Modify: `services/library/cmd/library-api/main.go:333-402` — construct `storageclient.New(cfg.Storage.URL)`; wire into encoder pool, evictor, storyboard backfill, handlers
- Modify: `services/library/internal/service/encoder_worker.go` — `Uploader` interface (`:52-58`) becomes `Upload(ctx context.Context, class, override, prefix string, files []string) (storage string, err error)`; `processJob:313-323`: class = `domain.ClassLibraryAuto` if `job.Source == autocache` else `domain.ClassLibraryManual` (define local consts or reuse strings — do NOT import the storage service's internal domain; declare the class strings in library's own domain package), override = `job.Storage`; after successful upload write resolved storage back to the job row and set it on the episode row (`:359-397`)
- Modify: `services/library/internal/handler/jobs.go:406-510` (Link) — list `pending/<id>/` via `client.List(ctx, job.Storage, ...)` (job.Storage now holds the RESOLVED id after upload), `client.Move` within that storage, episode row gets that storage
- Modify: `services/library/internal/handler/episodes.go:107,116,192,206` — URLs via `client.URLFor(ctx, ep.Storage, ep.MinioPath+"playlist.m3u8")`; add `"storage"` to episode JSON responses (list + single); single-episode GET accepts optional `?storage=` and when absent prefers `minio` over `s3`
- Modify: evictor + storyboard backfill call sites (`DeletePrefix`/`DownloadPrefix` with `ep.Storage`)
- Modify: `services/library/cmd/library-batchingest/main.go:138-145,224-301` — storageclient instead of writer; add `-storage` flag (default `minio`) passed as override with class `library-manual`; **switch its upload prefix from the legacy `{shik}/{ep}/` to `autocache.RawPrefix`** only if episodes handler already reads both layouts — otherwise keep legacy layout untouched (do not change layout semantics in this task)
- Modify: `services/library/go.mod` (+storageclient require), `services/library/Dockerfile` if it doesn't already COPY the new lib (Task 2 swept it)
- Modify: `docker/docker-compose.yml` library env: add `LIBRARY_STORAGE_URL: http://storage:8099`, add `depends_on: storage: condition: service_healthy`; REMOVE `LIBRARY_MINIO_*` vars

**Interfaces (Consumes):** `storageclient.Client` from Task 2; class consts semantics from Task 1; `Episode.Storage`/`Job.Storage` from Task 3.

**Steps:**
- [ ] Update `Uploader` interface + fake uploader in existing encoder tests (grep `Uploader` in `services/library/internal/service/*_test.go`); tests updated to assert class/override routing (autocache→library-auto, manual→library-manual with job override); FAIL first, then implement
- [ ] Sweep callers; delete `internal/minio`; `go build ./...` + `go test ./...` in services/library — green
- [ ] `make redeploy-library`; `docker logs animeenigma-library --tail 20` clean; create a tiny manual magnet job via admin API? — too heavy; instead run `library-batchingest -dry-run` smoke and `curl library episode list` for an existing title → items now carry `"storage":"minio"` and a working URL
- [ ] Commit `refactor(library): all object I/O through storage service`; push

### Task 5: Catalog — union enumeration + `?server=` + servers list

**Files:**
- Modify: `services/catalog/internal/parser/library/client.go` — `EpisodeResponse`/`EpisodeListItem` + `Storage string \`json:"storage"\``; `GetEpisode(ctx, shikimoriID string, episode int, storage string)` (adds `?storage=` when non-empty)
- Modify: `services/catalog/internal/service/raw_resolver.go:209-340` — `GetLibraryEpisodes` dedupes by episode_number for the episode list but keeps `map[int][]string` of storages; `GetLibraryStream(ctx, anime, episode int, server string)`:
  - validate server ∈ {"", "minio", "s3"}
  - server=="" → prefer `minio` if present else `s3`
  - fetch that copy via `GetEpisode(..., server)`; build signed stream via `newLibraryStream` (`:184`) unchanged
  - when the episode exists in BOTH storages, attach `Servers: []RawServer{{ID:"minio",Label:"Local"},{ID:"s3",Label:"Cloud"}}` to the returned `RawStream` (add `Servers []RawServer` + `type RawServer struct{ ID, Label string }` with json tags `id`,`label`)
- Modify: `services/catalog/internal/handler/ae.go` — `GetAeStream` reads `r.URL.Query().Get("server")` (mirror `scraper.go:116`), passes through; response JSON includes `servers` when non-empty
- Modify: `services/catalog/internal/service/capability/families_firstparty.go` + `raw_resolver.go AeTitleInfo:256` — wherever ae episode COUNTS/flags are computed from `ListEpisodes`, dedupe by episode_number (dual-presence must not double-count; `partial_library` unchanged semantics)
- Test: extend existing raw_resolver tests (grep `raw_resolver_test`): dual-storage episode → servers list present + default resolves minio; s3-only episode → no servers, resolves s3; `server=s3` explicit → s3 URL signed

**Interfaces (Consumes):** library episode API `storage` field + `?storage=` param from Task 4. **(Produces):** ae stream response `servers:[{id,label}]` + `server` query param for Task 7.

**Steps:**
- [ ] Failing resolver tests (fake library HTTP server returning dual rows); implement; PASS; `go build ./...`
- [ ] `make redeploy-catalog`; `curl -s 'localhost:8081/api/anime/<uuid>/ae/stream?episode=1'` on a known library title → 200, URL signed, no `servers` (single copy today)
- [ ] Commit `feat(catalog): ae dual-storage — union episodes, server param, Local/Cloud servers`; push

### Task 6: Streaming — multi-storage presign seam

**Files:**
- Modify: `libs/videoutils/storage.go` — add `type MultiStorage struct{ storages []*Storage }`, `func NewMultiStorage(ss ...*Storage) *MultiStorage`, `func (m *MultiStorage) PresignURL(rawURL string) (string, bool)` (first storage whose `IsOwnHost` matches wins; each `Storage.PresignURL:91-114` already host-gates), `func (m *MultiStorage) Hosts() []string`
- Modify: `services/streaming/internal/config/config.go:77-83` — add second `videoutils.StorageConfig` from env `S3_ENDPOINT`/`S3_ACCESS_KEY`/`S3_SECRET_KEY`/`S3_LIBRARY_BUCKET` (default `raw-library`)/`S3_USE_SSL` (default true); optional — skip when endpoint empty
- Modify: `services/streaming/cmd/streaming-api/main.go:56` + `services/streaming/internal/handler/stream.go:72-79` — build both storages, `proxyCfg.UpstreamSigner = multi.PresignURL`, `FirstPartyHosts` gains both hosts
- Modify: `docker/docker-compose.yml` streaming env — add the `S3_*` vars (`${S3_ENDPOINT}` etc.)
- Test: `libs/videoutils/storage_test.go` (or create) — MultiStorage presigns by host, unknown host → ("",false). NOTE: `Storage.PresignURL` parses `/{bucket}/{object}` from the URL — confirm the parsed bucket (not the configured one) is used in `PresignedGetObject`; if it uses the configured bucket, both are `raw-library` so behavior is identical, but add a test pinning whichever it is.

**Steps:**
- [ ] Failing MultiStorage test (two fake-endpoint storages; assert host routing — construction doesn't dial, `NewStorage:51-66` only builds a client); implement; PASS
- [ ] `go build ./...` streaming; `make redeploy-streaming`; existing ae playback (minio) still works: `curl -s -o /dev/null -w '%{http_code}' 'https://animeenigma.org/api/streaming/hls-proxy?...'` with a freshly-signed ae stream URL from Task 5's curl → 200
- [ ] Commit `feat(streaming): multi-storage upstream presign (minio + external s3)`; push

### Task 7: Frontend — ae server plumbing + admin destination picker

**Files:**
- Modify: `frontend/web/src/api/client.ts:973-980` — `aeApi.getStream(animeId, episode, quality, server?: string)` appends `&server=` when set; `adminLibraryApi.createJob` payload type + `storage`
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts:313-350` — `makeAeAdapter.resolveStream` accepts the combo (align with other adapters' signature), passes `combo.server || undefined`, returns `servers: data.servers ?? []` on the StreamResult
- Modify: `frontend/web/src/types/library.ts` — `Job`/`CreateJobPayload`/episode types + `storage`
- Modify: `frontend/web/src/views/admin/RawLibrary.vue` — destination select on the create-job form (existing DS select pattern in that file/admin views; options `minio`/`s3`, default `minio`), include in `createJob` payload (`:451-461`); storage badge chip on job + episode rows (plain token-bound chip, cyan for Local / indigo for Cloud — both DS-exempt hues)
- Modify: i18n `frontend/web/src/locales/{en,ru,ja}.json` — keys `admin.library.storage.label` ("Storage"/«Хранилище»/「ストレージ」), `admin.library.storage.minio` ("Local (MinIO)"/«Локально (MinIO)»/「ローカル (MinIO)」), `admin.library.storage.s3` ("Cloud (S3)"/«Облако (S3)»/「クラウド (S3)」). Player server labels come from the backend (`Local`/`Cloud`) like every other provider's server names — no player i18n keys.
- No SourcePanel/AePlayer changes needed: `resolvedServers` fills from `stream.servers` (`AePlayer.vue:1632`), Server section renders when non-empty (`SourcePanel.vue:154-180`), combo/WT/failover already generic — verify, don't modify.

**Interfaces (Consumes):** `servers:[{id,label}]` + `server` param from Task 5; job `storage` field from Task 3/4.

**Steps:**
- [ ] Implement; run `/frontend-verify` (DS-lint + i18n parity + real build) — green
- [ ] Chrome smoke is opt-in per DS-NF-06 — skip unless owner asks; instead verify via curl that the built payload requests `server=` (grep dist not needed — vitest optional). Verify types: `bunx tsc --noEmit` (already in frontend-verify)
- [ ] Commit `feat(web): ae Local/Cloud server choice + admin storage destination picker`; push

### Task 8: Migration cmd + run (24GB autocache → S3)

**Files:**
- Create: `services/library/cmd/library-storage-migrate/main.go` — flags `-source autocache` (episode source filter), `-dry-run`, `-limit N`. For each episode row `storage='minio' AND source=$source`: `client.List(minio, prefix)` → `client.CopyPrefix(minio, s3, prefix)` → `client.List(s3, prefix)` and compare count+total bytes → UPDATE row `storage='s3'` (via EpisodeRepo; add `UpdateStorage(id, storage)` method) → `client.DeletePrefix(minio, prefix)`. Any mismatch: log + skip (leave row on minio). Idempotent: rows already `s3` are never selected.
- Modify: `services/library/internal/repo/episode.go` — `UpdateStorage`; `services/library/Dockerfile` — bake the binary beside `library-batchingest` (same pattern)

**Steps:**
- [ ] Build + `-dry-run -limit 2` via `docker compose run --rm --entrypoint /app/library-storage-migrate library -dry-run -limit 2` → prints planned copies, no writes
- [ ] Real run `-limit 1`; verify: catalog ae stream for that title returns s3-host URL, **play it in the real player on prod (actual anime, per project rule)** and confirm segments 200 via hls-proxy; local prefix gone (`mc ls`)
- [ ] Full run (28 remaining, ~24GB, expect ~1h; run with `run_in_background` + Monitor); afterwards `du -sh` minio volume dropped ~24G; spot-check 2 more titles play
- [ ] Commit `feat(library): storage-migrate cmd — autocache content to cloud S3`; push

### Task 9 (Phase 2): Upscaler onto the storage service

**Files:**
- Investigate first (read-only): how UPSCALED-* output is registered/consumed (`services/upscaler/internal/handler/models.go:67`, `model_admin.go:67`, `internal/autocache/layout.go:16`, whether library episode rows or upscaler DB rows record tracks, and how playback URLs for upscaled tracks are built)
- Delete: `services/upscaler/internal/minio/writer.go` (the copy-paste)
- Modify: upscaler upload call sites → `storageclient.UploadFiles(ctx, "upscaled", "", prefix, files, 8)`; record returned storage wherever the track record lives; URL building → `client.URLFor`
- Modify: `services/upscaler/internal/config/config.go:117-120` + `docker/docker-compose.yml` upscaler env: `UPSCALER_STORAGE_URL: http://storage:8099` (keep its staging `MINIO_*` only if the staging/logs bucket genuinely stays — staging is not user content; leave it)
- Existing `UPSCALED-*` prefixes: extend `library-storage-migrate` with `-track upscaled` mode ONLY if the investigation shows library_episodes rows track them; otherwise migrate via a documented one-off `mc mirror` + record update matching whatever store holds the pointer. Decide from evidence, document in the commit message.

**Steps:**
- [ ] Investigate + implement + `go build ./...` + existing upscaler tests green
- [ ] `make redeploy-upscaler`; trigger one upscale job (admin) → output lands on s3 (`aws s3 ls`), plays via player
- [ ] Commit `refactor(upscaler): UPSCALED tracks through storage service → cloud S3`; push

### Task 10: Docs, deploy sweep, changelog, feedback

- [ ] Update `docs/environment-variables.md` (storage service section + library/streaming/upscaler changes) and CLAUDE.md service-ports table (+`storage | 8099 | /metrics | Storage placement authority (MinIO+S3)`) and the aePlayer reference doc's ae row (JP subs table untouched; add a line about Local/Cloud servers) — commit
- [ ] Add prometheus scrape job for storage:8099 if prometheus.yml lists sibling services (REMEMBER: prometheus config changes need container RECREATE, not restart — memory `reference_prometheus_recreate_for_command_flags`)
- [ ] Run `/animeenigma-after-update` (simplify → lint/build → redeploy changed services → health → Russian Trump-mode changelog → commit/push). Note for changelog: user-facing bits are the Local/Cloud server choice + admin storage picker; the rest is infra.
- [ ] `bin/feedback-status 2026-07-09T06-41-45_tNeymik_manual ai_done claude-code`

## Self-Review Notes

- Spec coverage: service+API (T1), client (T2), data model (T3), library refactor incl. batchingest/evictor/Link (T4), catalog union+server (T5), presign seam (T6), FE player+admin+i18n (T7), migration (T8), upscaler Phase 2 (T9), ops/docs/changelog/feedback (T10). Phase 3 explicitly out of scope per spec.
- Type consistency: storage ids/classes pinned in Global Constraints; `servers:[{id,label}]` shape identical in T5 (producer) and T7 (consumer); `Uploader.Upload` new signature stated once in T4 where it changes.
- Known judgment calls delegated with evidence rules (batchingest layout, upscaler track registry) rather than guessed.
