# Anime Upscaler Service — Implementation Plan (Phase 1: Batch)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the batch upscale path end-to-end — a new admin-only `services/upscaler` orchestrator that segments a library episode, hands segments to a self-sustained dial-home GPU worker, checkpoints/resumes across spot preemption, reassembles + remuxes, and stores the upscaled HLS to MinIO — with a hardened `ext.animeenigma.org` edge, full `upscale_*` telemetry, remote shell, and a GPU-free `model=mock` E2E test.

**Architecture:** Go microservice (`services/upscaler`, port **8096**) mirroring the `services/themes` scaffold. Workers are a separate provider-agnostic container that dials home over one outbound WebSocket (gorilla) for control/logs/metrics/exec + plain authenticated HTTPS for bulk segment data, authenticated with isolated HMAC capability handles. Spec: `docs/superpowers/specs/2026-06-23-anime-upscaler-design.md`.

**Tech Stack:** Go 1.25 (go.work workspace), chi router, GORM + Postgres (AutoMigrate), Redis (log ring-buffer + lease coordination), `github.com/gorilla/websocket v1.5.1`, `github.com/minio/minio-go/v7`, ffmpeg/ffprobe (subprocess), `libs/{logger,database,cache,metrics,videoutils}`, Prometheus + Grafana. Worker image: see **Core Decision CD-2** (Go vs Python) — plan written language-agnostically for the worker, defaulting to **Go** (reuses gorilla/websocket client + single neutral static binary) shelling out to `realesrgan-ncnn-vulkan` + `ffmpeg`.

---

## Core Decisions to Review

These are the judgment calls baked into this plan. Confirm or override before execution — several change task content.

| ID | Decision | Default in this plan | Alternative | Impacts |
|----|----------|----------------------|-------------|---------|
| **CD-1** | Service port | **8096** (8095 is taken by `anidle`) | any other free port | M1, compose, prometheus, gateway |
| **CD-2** | Worker language | **Go** (reuse gorilla/websocket client, single static neutral binary, shells out to `realesrgan-ncnn-vulkan`+`ffmpeg`) | Python/FastAPI (matches `stealth-scraper`, ncnn pip wheel) | M5 entirely |
| **CD-3** | Schema management | **GORM `AutoMigrate`** (recent-service pattern: themes/recs/notifications) | SQL migration files (library pattern) | M1 (spec §10 said "SQL migrations" — corrected here) |
| **CD-4** | Capability auth | isolated **`JOB_CAPABILITY_SECRET`** (fail-closed, no fallback) + `MintJobHandle`/`VerifyJobHandle`, handle `{jobID}:{operation}`, 48h TTL | reuse `STREAM_TOKEN_SECRET` | M2, M4 |
| **CD-5** | Edge ingress | separate **nginx vhost** for `ext.animeenigma.org` → gateway external handler (no JWT, IP rate-limited, `ExternalAPIKeyMiddleware`) behind **Cloudflare orange-cloud + Authenticated Origin Pulls** | Cloudflare Tunnel | M2 |
| **CD-6** | Source acquisition | shared **`library_torrents`** volume + copy `{downloadDir}/{infohash}/*` to upscaler staging at `job.Status=encoding` | new library `/internal` streaming endpoint | M3 |
| **CD-7** | Source-retention | **opportunistic** (trigger while present; re-acquire via library if dropped after 24h seed) | add proactive "pin-for-upscale" retain flag to library | M3 (+ library change if pinned) |
| **CD-8** | Output codec | **H.264 HLS** (matches hls.js stack), CRF/bitrate configurable | also keep a HEVC archival master | M3 finalizer |
| **CD-9** | Edge mTLS | **opt-in** (penciled, not built in Phase 1) | mandatory CF mTLS client certs at handoff | M2, worker handoff |
| **CD-10** | Realtime path | **excluded** from Phase 1 (designed-for in spec §7) | include a thin realtime slice now | scope |
| **CD-11** | Worker image registry | neutral name on **GHCR private** (`ghcr.io/<neutral>/worker`) | self-hosted registry | M5 packaging |

## Global Constraints

- **No direct changes in `/data/animeenigma`** (base tree). All work in the worktree `/data/ae-upscaler` (branch `feat/upscaler-service`). Exception: `.env`/secrets only.
- **Go workspace:** module path `github.com/ILITA-hub/animeenigma/services/upscaler`; add to root `go.work`; `replace` directives for all `libs/*` (relative `../../libs/*`). Build from repo root.
- **Schema source of truth:** `db.AutoMigrate(...)` in `main.go` (CD-3). No SQL migration runner.
- **Port 8096** must be added to: `docker/prometheus/prometheus.yml`, `docker/docker-compose.yml`, `deploy/scripts/redeploy.sh` SERVICE_PORTS array, gateway config (CD-1).
- **Non-root container** (`USER app`), listens on >1024, no root syscalls. Two-stage Dockerfile, build context is repo root (`..`).
- **No `testify/mock`.** Handwritten fakes with `sync.Mutex`/`atomic.*`; table-driven `t.Run`; `t.Helper()`; `t.Cleanup()`; `t.TempDir()`. Integration tests gated `//go:build integration` (line 1) + `INTEGRATION=1`.
- **Fail-closed secrets:** all capability signing/verification disabled (mint→`""`, verify→`false`, worker→401) when `JOB_CAPABILITY_SECRET` unset.
- **Cardinality discipline:** never put `worker_id` on high-frequency counters/histograms — only on the bounded `upscale_workers_connected` gauge + logs/audit. Aggregate by `gpu_model`/`image_version`/`model`/`status`.
- **Neutral worker artifact:** only `SERVER_URL=https://ext.animeenigma.org` known to the worker; no internal hostnames/service names/codenames in image, env, logs, or local console (`connected/leased/processing/idle/error` only).
- **Design-system / changelog:** N/A (no frontend in Phase 1; admin UI is a follow-up). Run `/animeenigma-after-update` at the end.

---

# Milestone 1 — Orchestrator scaffold, domain & persistence

Produces a running `upscaler` service (`/health`, `/metrics`) registered in compose/gateway/prometheus, with the job/segment/worker/model schema and repos. No worker logic yet.

### Task 1: Scaffold the `services/upscaler` Go service

**Files:**
- Create: `services/upscaler/go.mod`, `services/upscaler/cmd/upscaler-api/main.go`
- Create: `services/upscaler/internal/config/config.go`
- Create: `services/upscaler/internal/transport/router.go`
- Create: `services/upscaler/internal/handler/health.go`
- Create: `services/upscaler/Dockerfile`
- Modify: `go.work` (add `./services/upscaler` to the `use (` block)
- Test: `services/upscaler/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config{Server, Database, Redis, Upscaler}` where `Upscaler` holds `LibraryURL string`, `MinIO minio.Config`, `JobCapabilitySecret string`, `SegmentSeconds int` (default 45), `DefaultScale int` (default 2), `RemoteShellEnabled bool` (default true), `StagingDir string` (default `/data/upscale-staging`), `TorrentsDir string` (default `/data/torrents`). `config.Load() *Config`.
- Produces: `transport.NewRouter(deps RouterDeps) http.Handler` exposing `GET /health`, `GET /metrics`.

- [ ] **Step 1: Write the failing config test**

```go
package config

import (
	"os"
	"testing"
)

func TestLoad_DefaultsAndOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("SERVER_PORT", "8096")
	t.Setenv("SEGMENT_SECONDS", "")     // unset → default 45
	cfg := Load()
	if cfg.Server.Port != "8096" {
		t.Fatalf("port = %q, want 8096", cfg.Server.Port)
	}
	if cfg.Upscaler.SegmentSeconds != 45 {
		t.Fatalf("SegmentSeconds = %d, want 45 default", cfg.Upscaler.SegmentSeconds)
	}
	if cfg.Upscaler.DefaultScale != 2 {
		t.Fatalf("DefaultScale = %d, want 2 default", cfg.Upscaler.DefaultScale)
	}
	if !cfg.Upscaler.RemoteShellEnabled {
		t.Fatal("RemoteShellEnabled should default true")
	}
	_ = os.Unsetenv
}
```

- [ ] **Step 2: Run it, verify it fails** — `cd services/upscaler && go test ./internal/config/...` → FAIL (package/symbols missing).

- [ ] **Step 3: Create `go.mod` + add to `go.work`**

Copy `services/themes/go.mod`, change `module github.com/ILITA-hub/animeenigma/services/themes` → `.../services/upscaler`, keep the `require`/`replace` block for `libs/{logger,database,cache,metrics,authz,videoutils,httputil}` + `gorm`, `chi`, `gorilla/websocket v1.5.1`, `minio-go/v7`. Add `./services/upscaler` to the `use (` list in `/data/ae-upscaler/go.work`.

- [ ] **Step 4: Write `internal/config/config.go`**

Mirror `services/themes/internal/config/config.go` (struct + `getEnv`/`getEnvInt`/`getEnvBool` helpers; `log.Fatalw` if `JWT_SECRET` unset). Add the `Upscaler` sub-struct and load:
```go
Upscaler: UpscalerConfig{
	LibraryURL:          getEnv("LIBRARY_URL", "http://library:8089"),
	JobCapabilitySecret: getEnv("JOB_CAPABILITY_SECRET", ""),
	SegmentSeconds:      getEnvInt("SEGMENT_SECONDS", 45),
	DefaultScale:        getEnvInt("DEFAULT_SCALE", 2),
	RemoteShellEnabled:  getEnvBool("REMOTE_SHELL_ENABLED", true),
	StagingDir:          getEnv("UPSCALE_STAGING_DIR", "/data/upscale-staging"),
	TorrentsDir:         getEnv("LIBRARY_TORRENTS_DIR", "/data/torrents"),
	MinIO:               loadMinIO(),  // mirror library minio.Config envs
},
```

- [ ] **Step 5: Write `transport/router.go` + `handler/health.go`**

Mirror `services/themes/internal/transport/router.go`: chi router, middleware `RequestID → metrics.Collector.Middleware → RequestLogger → Recoverer → RealIP`, `GET /health` → `httputil.OK(w, map[string]string{"status":"ok"})`, `GET /metrics` → `metricsCollector.Handler()`. Leave the `/api/upscale/*` group empty for now (filled in M4/admin).

- [ ] **Step 6: Write `cmd/upscaler-api/main.go`**

Mirror `services/themes/cmd/themes-api/main.go`: `logger.Default()` → `config.Load()` → `database.New(cfg.Database)` → `db.AutoMigrate()` (empty for now, models added Task 2) → `cache.New(cfg.Redis)` → `metrics.NewCollector("upscaler")` → `transport.NewRouter(...)` → `http.Server{Addr: ":"+cfg.Server.Port}` → SIGINT/SIGTERM → `srv.Shutdown(30s)`.

- [ ] **Step 7: Write `Dockerfile`** — copy `services/themes/Dockerfile`, swap `themes`→`upscaler`, `EXPOSE 8096`. Add `ffmpeg` to the runtime stage: `RUN apk add --no-cache ca-certificates tzdata ffmpeg` (orchestrator needs ffmpeg for segmenting/finalizing).

- [ ] **Step 8: Run config test, verify pass** — `go test ./internal/config/...` → PASS.

- [ ] **Step 9: Build the binary** — `cd /data/ae-upscaler && go build ./services/upscaler/...` → no errors.

- [ ] **Step 10: Commit**
```bash
git add services/upscaler go.work && git commit -m "feat(upscaler): scaffold service (port 8096) + config + health"
```

---

### Task 2: Domain models + AutoMigrate

**Files:**
- Create: `services/upscaler/internal/domain/job.go`, `segment.go`, `worker.go`, `model.go`
- Modify: `services/upscaler/cmd/upscaler-api/main.go` (AutoMigrate call)
- Test: `services/upscaler/internal/domain/job_test.go`

**Interfaces:**
- Produces: `domain.UpscaleJob`, `domain.UpscaleSegment`, `domain.UpscaleWorker`, `domain.UpscaleModel`; status enums `JobStatus`, `SegmentStatus`; `(JobStatus).IsTerminal() bool`.

Field shapes (GORM tags, `TableName()` pins names):

```go
// job.go
type JobStatus string
const (
	JobQueued     JobStatus = "queued"
	JobSegmenting JobStatus = "segmenting"
	JobUpscaling  JobStatus = "upscaling"
	JobFinalizing JobStatus = "finalizing"
	JobDone       JobStatus = "done"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)
func (s JobStatus) IsTerminal() bool {
	switch s { case JobDone, JobFailed, JobCancelled: return true }
	return false
}
type UpscaleJob struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	ShikimoriID   string     `gorm:"type:text;not null;index;column:shikimori_id" json:"shikimori_id"`
	Episode       int        `gorm:"type:int;not null;column:episode" json:"episode"`
	Model         string     `gorm:"type:text;not null;column:model" json:"model"`
	Scale         int        `gorm:"type:int;not null;default:2;column:scale" json:"scale"`
	Status        JobStatus  `gorm:"type:text;not null;default:queued;index;column:status" json:"status"`
	ProgressPct   int        `gorm:"type:int;not null;default:0;column:progress_pct" json:"progress_pct"`
	SourceCodec   string     `gorm:"type:text;column:source_codec" json:"source_codec,omitempty"`
	SourcePixFmt  string     `gorm:"type:text;column:source_pixfmt" json:"source_pixfmt,omitempty"`
	SourceFPS     string     `gorm:"type:text;column:source_fps" json:"source_fps,omitempty"`
	SegmentCount  int        `gorm:"type:int;not null;default:0;column:segment_count" json:"segment_count"`
	OutputPrefix  string     `gorm:"type:text;column:output_prefix" json:"output_prefix,omitempty"`
	ErrorText     string     `gorm:"type:text;column:error_text" json:"error_text,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
	CompletedAt   *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}
func (UpscaleJob) TableName() string { return "upscale_jobs" }
```

```go
// segment.go — the resume ledger
type SegmentStatus string
const (
	SegPending SegmentStatus = "pending"
	SegLeased  SegmentStatus = "leased"
	SegDone    SegmentStatus = "done"
)
type UpscaleSegment struct {
	JobID          string        `gorm:"type:uuid;primaryKey;column:job_id" json:"job_id"`
	Idx            int           `gorm:"type:int;primaryKey;column:idx" json:"idx"`
	Status         SegmentStatus `gorm:"type:text;not null;default:pending;index;column:status" json:"status"`
	LeaseExpiresAt *time.Time    `gorm:"column:lease_expires_at" json:"lease_expires_at,omitempty"`
	WorkerID       string        `gorm:"type:text;column:worker_id" json:"worker_id,omitempty"`
	InBytes        int64         `gorm:"type:bigint;not null;default:0;column:in_bytes" json:"in_bytes"`
	OutBytes       int64         `gorm:"type:bigint;not null;default:0;column:out_bytes" json:"out_bytes"`
	StartedAt      *time.Time    `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt    *time.Time    `gorm:"column:completed_at" json:"completed_at,omitempty"`
}
func (UpscaleSegment) TableName() string { return "upscale_segments" }
```

```go
// worker.go — fleet registry
type UpscaleWorker struct {
	WorkerID         string     `gorm:"type:text;primaryKey;column:worker_id" json:"worker_id"`
	GPUInfo          string     `gorm:"type:text;column:gpu_info" json:"gpu_info,omitempty"`
	ImageVersion     string     `gorm:"type:text;column:image_version" json:"image_version,omitempty"`
	ModelsAvailable  string     `gorm:"type:text;column:models_available" json:"models_available,omitempty"` // csv
	Status           string     `gorm:"type:text;not null;default:idle;column:status" json:"status"`           // idle|busy|draining|gone
	CurrentJobID     string     `gorm:"type:uuid;column:current_job_id" json:"current_job_id,omitempty"`
	CurrentSegment   int        `gorm:"type:int;column:current_segment" json:"current_segment"`
	SessionExpiresAt *time.Time `gorm:"column:session_expires_at" json:"session_expires_at,omitempty"`
	LastHeartbeatAt  *time.Time `gorm:"column:last_heartbeat_at" json:"last_heartbeat_at,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at"`
}
func (UpscaleWorker) TableName() string { return "upscale_workers" }
```

```go
// model.go — model registry
type UpscaleModel struct {
	Name       string    `gorm:"type:text;primaryKey;column:name" json:"name"`
	Version    string    `gorm:"type:text;primaryKey;column:version" json:"version"`
	Checksum   string    `gorm:"type:text;not null;column:checksum" json:"checksum"`
	ObjectPath string    `gorm:"type:text;not null;column:object_path" json:"object_path"`
	Builtin    bool      `gorm:"type:boolean;not null;default:false;column:builtin" json:"builtin"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}
func (UpscaleModel) TableName() string { return "upscale_models" }
```

- [ ] **Step 1: Write failing test** for `JobStatus.IsTerminal()` (table-driven: queued→false, done/failed/cancelled→true).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Create the four domain files** as above.
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Wire AutoMigrate** in `main.go`: `db.AutoMigrate(&domain.UpscaleJob{}, &domain.UpscaleSegment{}, &domain.UpscaleWorker{}, &domain.UpscaleModel{})`.
- [ ] **Step 6: Build** → `go build ./services/upscaler/...`.
- [ ] **Step 7: Commit** — `feat(upscaler): domain models + AutoMigrate`.

---

### Task 3: Repositories (job, segment, worker, model)

**Files:**
- Create: `services/upscaler/internal/repo/{job,segment,worker,model}.go`
- Test: `services/upscaler/internal/repo/{job_sqlite_test.go, segment_sqlite_test.go, segment_integration_test.go}`

**Interfaces:**
- Produces:
  - `JobRepository`: `Create(ctx, *UpscaleJob) error`, `Get(ctx, id) (*UpscaleJob, error)`, `List(ctx, JobFilter) ([]UpscaleJob, error)`, `UpdateStatus(ctx, id, JobStatus, errText string) error`, `SetProgress(ctx, id, pct int) error`, `SetSourceMeta(ctx, id, codec, pixfmt, fps string, segCount int) error`, `SetOutputPrefix(ctx,id,prefix string) error`.
  - `SegmentRepository`: `BulkInsertPending(ctx, jobID string, n int) error`, `LeaseNext(ctx, jobID, workerID string, ttl time.Duration) (*UpscaleSegment, error)`, `MarkDone(ctx, jobID string, idx int, outBytes int64) error`, `ExpireStale(ctx, now time.Time) (int, error)`, `Counts(ctx, jobID) (pending, leased, done int, err error)`, `ListByJob(ctx, jobID) ([]UpscaleSegment, error)`.
  - `WorkerRepository`: `Upsert(ctx, *UpscaleWorker) error`, `Heartbeat(ctx, workerID string, jobID string, seg int, now time.Time) error`, `MarkGone(ctx, workerID string) error`, `ListConnected(ctx, since time.Time) ([]UpscaleWorker, error)`.
  - `ModelRepository`: `Upsert(ctx, *UpscaleModel) error`, `Get(ctx, name, version string) (*UpscaleModel, error)`, `List(ctx) ([]UpscaleModel, error)`.

Key logic to get right (rest mirrors `services/themes/internal/repo/theme.go` GORM patterns):

- `SegmentRepository.LeaseNext` MUST be atomic under concurrency. Use `clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}` inside a transaction: select the lowest-`idx` row where `status='pending' OR (status='leased' AND lease_expires_at < now)`, set `status='leased', worker_id=?, lease_expires_at=now+ttl, started_at=coalesce(started_at,now)`, return it. Returns `(nil, nil)` when none available (job fully leased/done).
- `ExpireStale` flips `leased` rows with `lease_expires_at < now` back to `pending` (clears `worker_id`) — the spot-resume mechanism. Returns count.
- `Counts` is a single `GROUP BY status` query.

- [ ] **Step 1: Write `segment_sqlite_test.go`** — table-driven over the ledger: `BulkInsertPending(job, 3)` → `Counts` = (3,0,0); `LeaseNext` twice → 2 distinct idx leased, `Counts`=(1,2,0); `MarkDone(idx0)` → done=1; `ExpireStale(now+ttl+1)` → leased→pending. Use an in-memory sqlite DB helper `openTestDB(t)` (mirror `services/library/internal/repo/demand_sqlite_test.go`, including the custom `now()` UDF registration if needed).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement the four repos.** For `LeaseNext`, write the SQLite-compatible query (SQLite ignores `SKIP LOCKED` but the unit test is single-threaded; real concurrency is covered by the integration test). Document the dialect gap in a comment (WR-03 pattern).
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Write `segment_integration_test.go`** (`//go:build integration`) — real Postgres, two goroutines call `LeaseNext` concurrently on a 1-segment job; assert exactly one wins (the other gets `nil`). Per-test DB via `CREATE DATABASE`; re-apply AutoMigrate to prove idempotence.
- [ ] **Step 6: Run integration** — `INTEGRATION=1 go test -tags=integration ./internal/repo/...` → PASS.
- [ ] **Step 7: Commit** — `feat(upscaler): job/segment/worker/model repos + lease ledger`.

---

### Task 4: Register service in compose, gateway, prometheus, redeploy

**Files:**
- Modify: `docker/docker-compose.yml` (new `upscaler:` block + `library_torrents` + new `upscale_staging` volume mounts; gateway `UPSCALER_SERVICE_URL` env + `depends_on`)
- Modify: `services/gateway/internal/config/config.go` (`UpscalerService` URL), `services/gateway/internal/handler/proxy.go` (`ProxyToUpscaler`), `services/gateway/internal/transport/router.go` (`/api/upscale/*` admin group)
- Modify: `docker/prometheus/prometheus.yml` (scrape job `upscaler` → `upscaler:8096`)
- Modify: `deploy/scripts/redeploy.sh` (SERVICE_PORTS: add `upscaler:8096`)

**Interfaces:**
- Produces: gateway route `/api/upscale/*` → `upscaler:8096` (admin-gated); env `UPSCALER_SERVICE_URL=http://upscaler:8096`.

- [ ] **Step 1: Add the compose `upscaler:` block** after `library:` — copy the library block shape: `x-logging: *default-logging`, `build.context: ..`, `dockerfile: services/upscaler/Dockerfile`, `container_name: animeenigma-upscaler`, `mem_limit: 1g` (ffmpeg segment/finalize), `restart: unless-stopped`, env (`SERVER_PORT: 8096`, `DB_*`, `JWT_SECRET`, `REDIS_*`, `LIBRARY_URL`, `MINIO_*`, `JOB_CAPABILITY_SECRET`, `SEGMENT_SECONDS`, `DEFAULT_SCALE`, `REMOTE_SHELL_ENABLED`, `EXT_HMAC_SECRET`, `TRACING_ENABLED: "false"`), `ports: ["127.0.0.1:8096:8096"]`, `volumes: [library_torrents:/data/torrents:ro, upscale_staging:/data/upscale-staging]`, `depends_on: postgres/redis service_healthy`. Add `upscale_staging:` to the top-level `volumes:` map.
- [ ] **Step 2: Gateway registration** — add `UpscalerService: getEnv("UPSCALER_SERVICE_URL", "http://upscaler:8096")` to `ServiceURLs`; add `ProxyToUpscaler(w,r){ h.proxy(w,r,"upscaler") }`; in `router.go` add `r.Route("/upscale", func(r chi.Router){ r.Use(JWTValidationMiddleware); r.Use(AdminRoleMiddleware); r.HandleFunc("/*", proxyHandler.ProxyToUpscaler) })` under `/api`. (Keep `/api/upscale/*` path as-is — no rewrite case needed.)
- [ ] **Step 3: Prometheus** — add `- job_name: 'upscaler'` / `static_configs: targets: ['upscaler:8096']` / `metrics_path: /metrics`.
- [ ] **Step 4: redeploy.sh** — add `upscaler:8096` to the SERVICE_PORTS array used for health verification.
- [ ] **Step 5: Validate compose** — `docker compose -f docker/docker-compose.yml config -q` → no error.
- [ ] **Step 6: Build + boot locally** — `make redeploy-upscaler` (or `docker compose build upscaler && docker compose up -d upscaler postgres redis`); `curl -sf http://localhost:8096/health` → `{"status":"ok"}`; `curl -s http://localhost:8096/metrics | head` → Prometheus output.
- [ ] **Step 7: Commit** — `feat(upscaler): wire into compose/gateway/prometheus/redeploy`.

---

# Milestone 2 — Capability tokens & hardened edge

### Task 5: Isolated HMAC capability handles (`JOB_CAPABILITY_SECRET`)

**Files:**
- Create: `services/upscaler/internal/capability/capability.go`
- Test: `services/upscaler/internal/capability/capability_test.go`

**Interfaces:**
- Produces: `capability.Init(secret string)`; `capability.Enabled() bool`; `capability.MintJobHandle(jobID, operation string, ttl time.Duration) (handle, exp, sig string)`; `capability.VerifyJobHandle(jobID, operation, exp, sig string, now time.Time) bool`.

Mirror `libs/videoutils/provenance.go` exactly: `handle = jobID + ":" + operation`; `sig = hex(HMAC_SHA256(secret, handle + "\n" + exp))[:32]`; `exp = strconv Unix seconds`; `subtle.ConstantTimeCompare`. Fail-closed when `secret == ""` (mint returns `("","","")`, verify returns `false`). `sync.Once`-gated secret load.

- [ ] **Step 1: Write table-driven test** — `TestMintVerify`: init with secret; mint `("job-1","segment-get",48h)`; verify same → true; verify at `exp` boundary; verify expired (now after exp) → false; verify tampered sig → false; verify wrong jobID → false; verify wrong operation (`segment-put`) → false. `TestFailClosedWhenUnset`: `Init("")` → mint returns empty, verify returns false.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement `capability.go`.**
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Init in `main.go`** — `capability.Init(cfg.Upscaler.JobCapabilitySecret)`; log WARN if disabled.
- [ ] **Step 6: Commit** — `feat(upscaler): isolated HMAC job-capability handles (fail-closed)`.

---

### Task 6: Hardened `ext.animeenigma.org` worker edge

**Files:**
- Modify: `services/gateway/internal/config/config.go` (add `ExternalAPIKey` from `EXTERNAL_API_KEY` env)
- Create: `services/gateway/internal/handler/external_api.go` (worker-only proxy)
- Create: `services/gateway/internal/middleware/external_api_key.go` (`ExternalAPIKeyMiddleware`)
- Modify: `services/gateway/internal/transport/router.go` (mount `/worker/*` route group, **outside** JWT, IP-rate-limited)
- Create: `infra/nginx/ext.animeenigma.org.conf` (vhost) + `docs/upscaler-edge-setup.md` (CF + cert runbook)
- Test: `services/gateway/internal/middleware/external_api_key_test.go`, `services/gateway/internal/handler/external_api_test.go`

**Interfaces:**
- Produces: edge routes `/worker/enroll`, `/worker/ws`, `/worker/segments/*`, `/worker/models/*` proxied to `upscaler:8096` with **no JWT**, gated by `ExternalAPIKeyMiddleware` (static `X-API-Key`) + per-IP rate limit; generic error bodies.

- [ ] **Step 1: Write `external_api_key_test.go`** — request without `X-API-Key` → 401 with body `{"error":"unauthorized"}` (no internal detail); with correct key → passes to next; with wrong key → 401. Constant-time compare.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement `ExternalAPIKeyMiddleware`** — reads `X-API-Key`, `subtle.ConstantTimeCompare` vs `cfg.ExternalAPIKey`; fail-closed (if `ExternalAPIKey==""` → reject all). Generic 401.
- [ ] **Step 4: Implement `external_api.go`** — a dedicated handler that proxies `/worker/*` to `upscaler` (reuse `proxyService.Forward(r,"upscaler")` but preserve `Upgrade/Connection` for `/worker/ws` like `ws_proxy.go`; strip `Cookie`). WS path uses the `httputil.ReverseProxy` director pattern from `services/gateway/internal/transport/ws_proxy.go` (FlushInterval -1, no hop-by-hop stripping of Upgrade).
- [ ] **Step 5: Mount in `router.go`** — `r.Route("/worker", func(r chi.Router){ r.Use(perIPRateLimit); r.Use(ExternalAPIKeyMiddleware); r.Handle("/ws", wtStyleWSProxyForUpscaler); r.HandleFunc("/*", externalHandler.Proxy) })` at router root (NOT under `/api`, NOT under any admin/JWT group). Ensure registered before any catch-all.
- [ ] **Step 6: Write `external_api_test.go`** — httptest: `/worker/segments/x` without key → 401; with key → forwarded (assert upstream hit via a stub). Assert no `Set-Cookie`/internal headers leak.
- [ ] **Step 7: Run → PASS.**
- [ ] **Step 8: Write the nginx vhost** `infra/nginx/ext.animeenigma.org.conf`: server_name `ext.animeenigma.org`, TLS, `location /worker/ { proxy_pass http://gateway:8000; proxy_set_header X-Real-IP $remote_addr; proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection $connection_upgrade; }`. No other locations. Document in `docs/upscaler-edge-setup.md`: Cloudflare orange-cloud, **Authenticated Origin Pulls** (origin rejects non-CF), WAF managed rules + rate-limit rules, that nginx must sit between CF and gateway so `X-Real-IP` chain holds, and the opt-in mTLS (CD-9) steps.
- [ ] **Step 9: Commit** — `feat(gateway): hardened ext.animeenigma.org worker edge (API-key, IP-limit, WS proxy)`.

---

# Milestone 3 — Source acquisition, segmenter & finalizer

### Task 7: Source acquisition from the library volume

**Files:**
- Create: `services/upscaler/internal/source/acquire.go`, `services/upscaler/internal/source/probe.go`
- Test: `services/upscaler/internal/source/{acquire_test.go, probe_test.go}`

**Interfaces:**
- Produces: `source.Resolver` interface `Resolve(ctx, job *UpscaleJob) (localPath string, err error)`; `source.Probe(ctx, path string) (ProbeResult, error)` where `ProbeResult{VideoPath, Codec, PixFmt, FPS string, Width, Height int, HasAudio bool, SubTracks []int, FontAttachments int}`.

Logic:
- `Resolve` needs the job's torrent infohash. Phase-1 acquisition (CD-6): the admin trigger supplies the library `job_id`/`infohash`; the resolver looks under `{TorrentsDir}/{infohash}/` (and the known flat `{TorrentsDir}/{name}` fallback — note the library autocache flat-vs-infohash-dir defect) for the largest video file, copies it to `{StagingDir}/{upscaleJobID}/source.<ext>`. If absent (dropped after 24h seed) return a typed `ErrSourceGone` so the handler can surface "re-acquire via library" (CD-7).
- `Probe` runs `ffprobe -v error -print_format json -show_format -show_streams` (mirror `services/library/internal/ffmpeg/transcoder.go probe()`), picks the real video stream (codec_type=video, largest), records codec/pix_fmt/avg_frame_rate/width/height, counts audio + subtitle streams + `attachment` streams (fonts). Soft-fail to zero values is NOT acceptable here (unlike library) — a probe failure must fail the job (we need accurate metadata for remux).

- [ ] **Step 1: Write `probe_test.go`** — fake `ffprobe` shell script (transcoder_test.go pattern) emitting JSON with a 10-bit HEVC video stream + 1 audio + 2 subtitle + 3 attachment streams; assert `ProbeResult{Codec:"hevc", PixFmt:"yuv420p10le", HasAudio:true, SubTracks len 2, FontAttachments 3}`.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `probe.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `acquire_test.go`** — `t.TempDir()` fake torrents dir with `{infohash}/episode.mkv` (largest) + a `.nfo`; assert `Resolve` copies the `.mkv` to staging and returns its path; second case: empty dir → `ErrSourceGone`.
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `acquire.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Commit** — `feat(upscaler): source acquisition + ffprobe detection from library volume`.

---

### Task 8: Lossless video segmenter + audio/subs demux

**Files:**
- Create: `services/upscaler/internal/ffmpeg/segmenter.go`
- Test: `services/upscaler/internal/ffmpeg/segmenter_test.go`

**Interfaces:**
- Produces: `ffmpeg.Segmenter` with `Segment(ctx, srcVideoPath, outDir string, seconds int) ([]string, error)` (returns ordered segment paths `seg_%05d.mkv`) and `DemuxSidecars(ctx, srcPath, outDir string) (Sidecars, error)` where `Sidecars{AudioPath string, SubPaths []string, FontPaths []string, ChaptersPath string}`.

ffmpeg args (from research):
- Segment: `ffmpeg -hide_banner -nostats -y -i {src} -map 0:v:0 -c:v copy -an -sn -f segment -segment_time {seconds} -reset_timestamps 1 -segment_format matroska {outDir}/seg_%05d.mkv` (5-digit padding supports >999 segments; keyframe-aligned by `-segment_time`; matroska container preserves any video codec incl. 10-bit HEVC/AV1 without re-encode).
- Demux audio: `ffmpeg ... -i {src} -map 0:a? -c:a copy {outDir}/audio.mka` (all audio tracks, `?` = optional). Subs: `-map 0:s? -c:s copy {outDir}/subs.mks`. Fonts/attachments: `-dump_attachment:t "" -i {src}` into `{outDir}/fonts/`. Chapters: `ffmpeg -i {src} -f ffmetadata {outDir}/chapters.ini`.

- [ ] **Step 1: Write `segmenter_test.go`** — fake `ffmpeg` script that, given `-f segment`, writes `seg_00000.mkv`, `seg_00001.mkv` into outDir and captures argv to a sidecar file; assert (a) returned slice is the sorted 2 paths, (b) argv contains `-c:v copy`, `-an`, `-sn`, `-segment_time 45`, `-reset_timestamps 1`. Second test: `DemuxSidecars` argv contains `-map 0:a?`, `-c:a copy`.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `segmenter.go`** (mirror transcoder.go's `exec.CommandContext` + bounded stderr ring buffer; glob+sort `seg_*.mkv`). **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** — `feat(upscaler): lossless -c copy segmenter + sidecar demux`.

---

### Task 9: Finalizer (concat + remux + HLS encode) + MinIO writeback

**Files:**
- Create: `services/upscaler/internal/ffmpeg/finalizer.go`
- Create: `services/upscaler/internal/minio/writer.go` (port the library Uploader interface + EnsureBucket + Upload)
- Create: `services/upscaler/internal/autocache/layout.go` (`UpscaledPrefix(shikimoriID string, episode, scaleHeight int) string`)
- Test: `services/upscaler/internal/ffmpeg/finalizer_test.go`, `services/upscaler/internal/minio/writer_test.go`

**Interfaces:**
- Produces: `ffmpeg.Finalizer.Concat(ctx, upscaledSegDir string, sc Sidecars, probe ProbeResult, out string) error`; `minio.Writer` with `EnsureBucket(ctx) error` + `Upload(ctx, prefix string, files []string) (int64, error)` + `URLFor(path) string` (copied verbatim from `services/library/internal/minio/writer.go`, bucket `raw-library`); `autocache.UpscaledPrefix(id, ep, h)` → `aeProvider/{id}/UPSCALED-{h}p/{ep}/`.

Finalizer logic (CD-8, H.264 HLS):
1. concat demuxer over upscaled segments → `concat.txt` listing `file 'seg_00000.mkv'` lines, `ffmpeg -f concat -safe 0 -i concat.txt -c:v copy {tmp}/video.mkv` (segments are already upscaled+encoded by the worker — concat is stream-copy).
2. remux + transcode to HLS: `ffmpeg -i {tmp}/video.mkv -i audio.mka -i subs.mks -map 0:v -map 1:a? -map 2:s? -c:v libx264 -preset slow -crf 18 -pix_fmt {yuv420p|yuv420p10le from probe} -c:a copy -c:s copy -hls_time 6 -hls_playlist_type vod -hls_segment_filename {out}/segment_%03d.ts {out}/playlist.m3u8`. Use the probe FPS to set `-r`/`-fps_mode passthrough` for VFR safety. (Encode params CRF/preset configurable.)

- [ ] **Step 1: Write `writer_test.go`** — port `services/library/internal/minio/writer_test.go` `fakeUploader`; assert `Upload` puts `.ts` segments concurrently then `playlist.m3u8` LAST, content-types correct, returns total bytes.
- [ ] **Step 2: Run → FAIL.** **Step 3: Port `writer.go` + `layout.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `finalizer_test.go`** — fake ffmpeg; assert concat argv has `-f concat -safe 0 -c:v copy`; remux argv has `-c:v libx264 -crf 18 -hls_time 6` and pix_fmt matches probe; produces `playlist.m3u8` + `segment_000.ts`.
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `finalizer.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Commit** — `feat(upscaler): finalizer (concat+remux+H264 HLS) + MinIO writeback`.

---

# Milestone 4 — Dial-home control plane

### Task 10: WS protocol types + enrollment/session

**Files:**
- Create: `services/upscaler/internal/controlplane/protocol.go` (frame envelope + types)
- Create: `services/upscaler/internal/controlplane/enroll.go` (`POST /worker/enroll` handler logic)
- Create: `services/upscaler/internal/controlplane/session.go` (session token mint/verify — reuse `capability`)
- Test: `controlplane/{protocol_test.go, enroll_test.go}`

**Interfaces:**
- Produces: `protocol.Frame{Type string; Seq int; Payload json.RawMessage}` with `Type ∈ {register, command, log, heartbeat, metrics, exec_open, exec_data, exec_close, lease_req, lease_grant}`; typed payload structs `RegisterPayload`, `CommandPayload{Cmd string}`, `HeartbeatPayload{...}`, `MetricsPayload{...}`, `ExecPayload{...}`. `enroll.Handle(...)` exchanges a one-time `ENROLL_TOKEN` for a session credential (capability handle, op=`session`, TTL 12h) + assigns a `worker_id`.

- [ ] **Step 1: Write `protocol_test.go`** — round-trip marshal/unmarshal each frame type; unknown type rejected.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `protocol.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `enroll_test.go`** — valid enroll token → returns session handle + worker_id + upserts `upscale_workers` row; invalid/empty token → 401; reused one-time token → 401 (track consumed tokens in Redis with TTL).
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `enroll.go` + `session.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Commit** — `feat(upscaler): WS protocol envelope + worker enrollment/session`.

---

### Task 11: WS hub (pumps) + lease delivery + heartbeat/liveness + resume sweeper

**Files:**
- Create: `services/upscaler/internal/controlplane/hub.go` (connection registry, readPump/writePump)
- Create: `services/upscaler/internal/controlplane/ws_handler.go` (`/worker/ws` upgrade)
- Create: `services/upscaler/internal/service/leaser.go` (assigns segments, drives job state)
- Create: `services/upscaler/internal/service/sweeper.go` (ExpireStale + liveness → MarkGone)
- Test: `controlplane/hub_test.go`, `service/{leaser_test.go, sweeper_test.go}`

**Interfaces:**
- Consumes: `SegmentRepository`, `JobRepository`, `WorkerRepository`, `protocol`, `minio.Writer`, `capability`.
- Produces: `hub.Hub` with `Register(conn)`, `Unregister(id)`, `Send(workerID, Frame) error`, `Broadcast(Frame)`; `leaser.OnLeaseReq(workerID, jobID) (*UpscaleSegment, handles, error)`; `sweeper.Run(ctx)` (ticker: `ExpireStale(now)` re-leases preempted segments + `WorkerRepository` rows with `last_heartbeat_at < now-Nx` → `MarkGone`).

WS specifics (gorilla, from research): `newWSUpgrader()` with `CheckOrigin`; `readPump` sets `SetReadLimit`, `SetReadDeadline(now+pongWait)`, `SetPongHandler` resetting deadline; `writePump` ticker `pingPeriod=30s`, `pongWait=60s`, write deadline on every write; 2 goroutines/conn; graceful close via `sync.Once` + context. On `lease_req` frame → `leaser.OnLeaseReq` → reply `lease_grant` with segment idx + minted `segment-get`/`segment-put` handles (op-scoped, 48h). On `heartbeat` → `WorkerRepository.Heartbeat` + record metrics. On preempt (conn drop) → `Unregister`; the sweeper re-leases via TTL.

- [ ] **Step 1: Write `sweeper_test.go`** — seed a `leased` segment with expired lease + a worker with stale heartbeat; run one sweep tick; assert segment→`pending`, worker→`gone`. (No WS; pure repo logic.)
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `sweeper.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `leaser_test.go`** — handwritten fake repos; `OnLeaseReq` on a job with 2 pending segs → returns idx0 with valid `segment-get`+`segment-put` handles; second call → idx1; third → `nil` (job→`finalizing` trigger asserted via fake JobRepo capturing `UpdateStatus`).
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `leaser.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Write `hub_test.go`** — real `httptest.Server` + gorilla `Dialer` (rooms `websocket_test.go` pattern, 100–500ms timings): connect, send `lease_req`, assert `lease_grant` received; drop conn, assert `Unregister`; keepalive ping/pong.
- [ ] **Step 10: Run → FAIL.** **Step 11: Implement `hub.go` + `ws_handler.go`.** **Step 12: Run → PASS.**
- [ ] **Step 13: Wire** `sweeper.Run` (goroutine) + hub into `main.go`; mount `/worker/ws` + `/worker/enroll` on the upscaler router (these are reached via the gateway `/worker/*` edge from M2). **Commit** — `feat(upscaler): WS hub + lease delivery + spot-resume sweeper`.

---

### Task 12: Command set + log ring-buffer + admin SSE

**Files:**
- Create: `services/upscaler/internal/controlplane/commands.go` (cancel/drain/shutdown/reconfigure/update/exec enqueue + deliver over WS)
- Create: `services/upscaler/internal/service/logbuffer.go` (per-job Redis ring-buffer, capped)
- Create: `services/upscaler/internal/handler/admin.go` (jobs CRUD, fleet, `GET /api/upscale/jobs/{id}/logs/stream` SSE, `POST /workers/{id}/commands`)
- Test: `controlplane/commands_test.go`, `service/logbuffer_test.go`, `handler/admin_test.go`

**Interfaces:**
- Produces: `commands.Issue(workerID, cmd string, args json.RawMessage) error` (validates against whitelist, sends `command` frame); `logbuffer.Append(jobID, line LogLine)`, `logbuffer.Tail(jobID, n) []LogLine`, `logbuffer.Subscribe(jobID) <-chan LogLine`; admin endpoints `POST /api/upscale/jobs`, `GET /api/upscale/jobs`, `GET /api/upscale/jobs/{id}`, `POST /api/upscale/jobs/{id}/cancel`, `POST /api/upscale/jobs/{id}/retry`, `GET /api/upscale/workers`, `POST /api/upscale/workers/{id}/commands`, `GET /api/upscale/jobs/{id}/logs/stream`.

- [ ] **Step 1: Write `commands_test.go`** — `Issue` with a non-whitelisted cmd → error; whitelisted → frame sent to the right worker (fake hub captures). `exec` honored only when `RemoteShellEnabled` (covered fully in Task 13).
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `commands.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `logbuffer_test.go`** — append > cap → oldest evicted (`Tail` returns last cap); `Subscribe` receives appended lines. (Use a Redis test or an in-memory fake `cache` — mirror existing cache fakes.)
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `logbuffer.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Write `admin_test.go`** — `POST /api/upscale/jobs {shikimori_id, episode, model, scale}` creates a `queued` job (fake service); `GET /jobs/{id}` returns it; SSE endpoint streams a published log line (httptest flush). The metrics `Hijack/Flush` forwarding must be present (research gotcha) so SSE isn't buffered.
- [ ] **Step 10: Run → FAIL.** **Step 11: Implement `admin.go` + mount `/api/upscale/*` group in router.** **Step 12: Run → PASS.**
- [ ] **Step 13: Commit** — `feat(upscaler): commands + log ring-buffer + admin API/SSE`.

---

### Task 13: Remote shell (exec over dial-home)

**Files:**
- Create: `services/upscaler/internal/controlplane/exec.go` (server side: relay admin↔worker exec frames; audit)
- Create: `services/upscaler/internal/handler/exec.go` (`GET /api/upscale/workers/{id}/shell` admin WS that bridges to the worker's exec stream)
- Test: `controlplane/exec_test.go`

**Interfaces:**
- Produces: `exec.Open(workerID, adminID string) (sessionID string, err error)` (refuses when `!RemoteShellEnabled`); relays `exec_data` frames both directions over the worker's WS; `exec_close` on either side ends + writes an audit row/log; idle timeout enforced.

- [ ] **Step 1: Write `exec_test.go`** — `Open` when `RemoteShellEnabled=false` → error (no frame sent). When true → `exec_open` frame sent to worker, audit log line appended (admin id, worker id, ts). A stub worker echoes `exec_data`; assert relay back to the admin channel. Session revoke (worker `MarkGone`) → `exec_close` emitted.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `exec.go` + `handler/exec.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** — `feat(upscaler): admin-initiated remote shell over dial-home (audited, gated)`.

---

# Milestone 5 — Worker container (CD-2 default: Go)

> If CD-2 → Python, replace M5 tasks with the FastAPI/uvicorn equivalent (stealth-scraper Dockerfile shape, `aiohttp` dial-home client, `websockets` lib, same protocol/frames). The protocol (M4) is language-neutral; only this milestone changes.

### Task 14: Worker skeleton + dial-home client (reconnect/backoff)

**Files:**
- Create: `worker/go.mod` (module `github.com/ILITA-hub/animeenigma/worker`, standalone; **not** in the main go.work — it's a separate artifact), `worker/cmd/worker/main.go`, `worker/internal/agent/client.go`, `worker/internal/agent/config.go`
- Create: `worker/Dockerfile` (neutral name; `ffmpeg` + `realesrgan-ncnn-vulkan` + models)
- Test: `worker/internal/agent/client_test.go`

**Interfaces:**
- Produces: `agent.Config{ServerURL, EnrollToken, Mode string}` (env `SERVER_URL`, `ENROLL_TOKEN`, `MODE`); `agent.Client` with `Run(ctx)` — enroll → open WS → readPump/writePump → reconnect with exponential backoff (1s→30s). Neutral local console only.

- [ ] **Step 1: Write `client_test.go`** — httptest server emulating `/worker/enroll` (returns session) + `/worker/ws`; assert client enrolls, connects, sends `register`, and reconnects after a forced close (backoff capped). Assert local stdout contains only neutral tokens (no URL/host).
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `config.go` + `client.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `worker/Dockerfile`** — `FROM golang:1.25 AS build` (static binary) → `FROM debian:bookworm-slim`, `apt-get install -y --no-install-recommends ffmpeg ca-certificates`, fetch `realesrgan-ncnn-vulkan` release + bake `best-quality`+`realtime` models under `/models`, `USER app`, `ENTRYPOINT ["/worker"]`. Neutral image labels. (CD-11 registry.)
- [ ] **Step 6: Commit** — `feat(worker): dial-home agent skeleton + reconnect + neutral Dockerfile`.

---

### Task 15: Lease loop + segment GET/PUT over HTTPS

**Files:**
- Create: `worker/internal/agent/leaseloop.go`, `worker/internal/agent/transfer.go`
- Test: `worker/internal/agent/{leaseloop_test.go, transfer_test.go}`

**Interfaces:**
- Produces: lease loop: send `lease_req` → receive `lease_grant{idx, getURL, putURL}` → `transfer.Download(getURL) → process → transfer.Upload(putURL)` → loop; deletes local files after each upload (process-and-delete). `transfer` attaches `?handle=&exp=&sig=` to `{ServerURL}/worker/segments/{job}/{idx}` and the `X-API-Key`.

- [ ] **Step 1: Write `transfer_test.go`** — httptest verifies GET includes the capability query + `X-API-Key`; PUT streams the file; 401 path retried/aborted appropriately.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `transfer.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `leaseloop_test.go`** — fake server grants 2 segments then `nil`; assert worker processes both (via a stubbed processor) and deletes locals; on grant gap, idles.
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `leaseloop.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Commit** — `feat(worker): lease loop + capability-signed segment transfer + process-and-delete`.

---

### Task 16: Model plugin interface + `mock`/`best-quality`/`realtime`

**Files:**
- Create: `worker/internal/upscale/model.go` (interface + registry), `worker/internal/upscale/mock.go`, `worker/internal/upscale/realesrgan.go`
- Test: `worker/internal/upscale/{mock_test.go, realesrgan_test.go}`

**Interfaces:**
- Produces: `upscale.Model` interface `Name() string`; `Upscale(ctx, framesDir, outDir string, scale int) error`; `upscale.Get(name string) (Model, error)`. `mock` = CPU `ffmpeg scale` (or copy) — instant, no GPU. `best-quality`/`realtime` shell out to `realesrgan-ncnn-vulkan -i framesDir -o outDir -s {scale} -n {modelName}` (research arg shape).

- [ ] **Step 1: Write `mock_test.go`** — `mock.Upscale` over a tiny frame dir produces same count of output frames (uses fake/real ffmpeg `scale`); registry `Get("mock")` returns it; `Get("nope")` errors.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `model.go` + `mock.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Write `realesrgan_test.go`** — fake `realesrgan-ncnn-vulkan` script; assert argv `-s {scale} -n realesr-animevideov3` for `realtime` and the heavy model name for `best-quality`.
- [ ] **Step 6: Run → FAIL.** **Step 7: Implement `realesrgan.go`.** **Step 8: Run → PASS.**
- [ ] **Step 9: Commit** — `feat(worker): pluggable models (mock + realesrgan animevideov3/anime6B)`.

---

### Task 17: Per-segment pipeline (decode → model → encode)

**Files:**
- Create: `worker/internal/upscale/pipeline.go`
- Test: `worker/internal/upscale/pipeline_test.go`

**Interfaces:**
- Produces: `pipeline.Process(ctx, inSegPath, outSegPath string, model upscale.Model, scale int) (Stats, error)` — `ffmpeg` decode seg → PNG/PPM frames; `model.Upscale`; `ffmpeg` re-encode upscaled frames → `outSegPath` (matroska, `libx264 -crf 16` or lossless intermediate — final HLS encode happens server-side in Task 9). `Stats{DecodeFPS, InferenceFPS, EncodeFPS, Frames}`.

- [ ] **Step 1: Write `pipeline_test.go`** — fake ffmpeg (decode emits N ppm; encode consumes them) + `mock` model; assert outSeg created, `Stats.Frames==N`, fps fields populated.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `pipeline.go`.** **Step 4: Run → PASS.**
- [ ] **Step 5: Wire pipeline into `leaseloop` processor.** **Commit** — `feat(worker): decode→model→encode per-segment pipeline + stats`.

---

### Task 18: Worker telemetry, exec handler, command handling

**Files:**
- Create: `worker/internal/agent/telemetry.go` (heartbeat+metrics frames: GPU via `nvidia-smi`/`rocm-smi` parse, host, pipeline fps), `worker/internal/agent/exec.go` (PTY/one-shot), `worker/internal/agent/commands.go` (cancel/drain/shutdown/reconfigure/update)
- Test: `worker/internal/agent/{telemetry_test.go, exec_test.go, commands_test.go}`

**Interfaces:**
- Produces: telemetry frames every N s; `exec` spawns a PTY in-container (`creack/pty` or `os/exec` + allowlist) only on server `exec_open`; command handlers act at safe boundaries.

- [ ] **Step 1–2:** `commands_test.go` — `drain` finishes current seg then idles; `cancel` aborts; `shutdown` exits cleanly. FAIL → implement → PASS.
- [ ] **Step 3–4:** `telemetry_test.go` — fake `nvidia-smi` script; assert metrics frame carries `gpu_util`, `vram_used`, fps. FAIL → implement → PASS.
- [ ] **Step 5–6:** `exec_test.go` — `exec_open` runs an allowlisted command, streams `exec_data` back; refuses unknown when not PTY mode. FAIL → implement → PASS.
- [ ] **Step 7: Commit** — `feat(worker): telemetry + remote-shell + command handling`.

---

# Milestone 6 — Observability & GPU-free E2E

### Task 19: `upscale_*` Prometheus metrics

**Files:**
- Create: `libs/metrics/upscaler.go`
- Modify: `services/upscaler/...` (record at lease/heartbeat/finalize sites)
- Test: `libs/metrics/upscaler_test.go`

**Interfaces:**
- Produces (cardinality-disciplined): `UpscaleWorkersConnected` gauge `{gpu_model,image_version,model}`; counters `UpscaleLeaseExpiredTotal`, `UpscaleCommandTotal{type}`, `UpscaleEnrollTotal{result}`, `UpscaleModelFetchTotal{result}`, `UpscaleEdgeRequestsTotal{path,status}`; histograms `UpscaleSegmentDuration{stage}`, gauges `UpscaleJobProgressRatio`, `UpscaleJobEtaSeconds`, `UpscaleQueueDepth{status}`; worker-reported gauges `UpscaleWorkerGPUUtil`, `UpscaleWorkerVRAMUsedBytes`, `UpscaleDecodeFPS/InferenceFPS/EncodeFPS` (labels `gpu_model`,`image_version`). Helper `RecordWorkerTelemetry(MetricsPayload)`.

- [ ] **Step 1: Write `upscaler_test.go`** — `prometheus.NewRegistry()` + `testutil.ToFloat64`; assert each metric registers and `{type=cancel}` increments independently; assert NO `worker_id` label on counters/histograms.
- [ ] **Step 2: Run → FAIL.** **Step 3: Implement `libs/metrics/upscaler.go`** (mirror `libs/metrics/probe.go` style). **Step 4: Run → PASS.**
- [ ] **Step 5: Record at sites** (sweeper expire, command issue, enroll, finalize, heartbeat). **Step 6: Build.** **Step 7: Commit** — `feat(upscaler): full upscale_* metrics with cardinality discipline`.

---

### Task 20: Grafana dashboard

**Files:**
- Create: `docker/grafana/dashboards/upscaler.json`
- Test: `docker/grafana/dashboards/upscaler_test.go` (or a `bash` JSON-validity + datasource-uid check, mirroring existing dashboard tests if present)

- [ ] **Step 1:** Build `upscaler.json` with rows: Fleet (`upscale_workers_connected`, GPU util/VRAM per `image_version`), Jobs (progress, ETA, queue depth by status, `upscale_lease_expired_total` rate = preemptions), Pipeline (decode/inference/encode fps), Edge (requests/auth-failures/rate-limit). Datasource uid = Prometheus (match existing dashboards).
- [ ] **Step 2:** Validate JSON parses + references the correct datasource uid. **Step 3: Commit** — `feat(upscaler): Grafana fleet+jobs dashboard`. (Note the known base-tree bind-mount gotcha — dashboard renders only after base-tree autosync picks it up post-merge.)

---

### Task 21: GPU-free `model=mock` end-to-end integration test

**Files:**
- Create: `services/upscaler/internal/e2e/mock_e2e_test.go` (`//go:build integration`)
- Create: `services/upscaler/internal/e2e/testdata/tiny.mkv` (a few seconds, generated via ffmpeg in a setup step or committed tiny fixture)

**Interfaces:**
- Consumes: the real orchestrator (in-process: hub + leaser + sweeper + repos on Postgres + fake MinIO) and the real worker agent (in-process, `MODE=batch`, `model=mock`, no GPU).

- [ ] **Step 1:** Write the E2E: boot the upscaler router on `httptest`; start an in-process worker agent pointed at it; `POST /api/upscale/jobs` for `tiny.mkv` (acquire via a temp staging dir); assert progression `queued→segmenting→upscaling→finalizing→done`, MinIO fake received `playlist.m3u8` + `.ts`, **kill the worker mid-job** (close its WS after 1 segment), restart it, assert it resumes and the job completes (no lost/dup segments). Assert a remote-shell round-trip (open exec, run `echo ok`, see output + audit line). Scrape `/metrics`, assert expected `upscale_*` series present.
- [ ] **Step 2: Run** `INTEGRATION=1 go test -tags=integration ./internal/e2e/...` → PASS.
- [ ] **Step 3: Commit** — `test(upscaler): GPU-free model=mock E2E with mid-job spot-resume + exec + metrics`.

---

## Self-Review

Run after the plan is written (done inline below; fixes applied):
1. **Spec coverage** — every spec §4–§12 + D1–D14 maps to a task (see the coverage matrix the review workflow produces). Realtime (§7) intentionally deferred (CD-10).
2. **Placeholder scan** — no "TBD/handle errors/etc."; ffmpeg arg lists, token math, lease SQL, metric names are concrete.
3. **Type consistency** — `MintJobHandle`/`VerifyJobHandle`, `LeaseNext`/`ExpireStale`, `protocol.Frame`, `upscale.Model` names are stable across consuming tasks.
4. **Scope** — Phase 1 only; worker is a separate module/artifact; admin UI deferred.

## Execution Handoff

After review + decision confirmation: **subagent-driven-development** (fresh subagent per task, two-stage review) is recommended given the size; worktree `/data/ae-upscaler` is already prepared. Worktree-isolated tasks that mutate files in parallel should use `isolation: worktree`.
