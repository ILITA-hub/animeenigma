---
phase: 04-ffmpeg-hls-transcoder-minio-writer
plan: 01
type: execute
wave: 1
workstream: raw-jp
milestone: v0.2
depends_on: []
files_modified:
  - services/library/migrations/002_library_episodes.sql
  - services/library/migrations/003_library_filename_patterns.sql
  - services/library/migrations/migrations.go
  - services/library/internal/domain/episode.go
  - services/library/internal/domain/filename_pattern.go
  - services/library/internal/repo/episode.go
  - services/library/internal/repo/episode_test.go
  - services/library/internal/repo/episode_integration_test.go
  - services/library/internal/repo/filename_pattern.go
  - services/library/internal/repo/filename_pattern_test.go
  - services/library/internal/parser/filename/detector.go
  - services/library/internal/parser/filename/detector_test.go
  - services/library/internal/ffmpeg/transcoder.go
  - services/library/internal/ffmpeg/transcoder_test.go
  - services/library/internal/minio/writer.go
  - services/library/internal/minio/writer_test.go
  - services/library/internal/metrics/library_metrics.go
  - services/library/internal/metrics/library_metrics_test.go
  - services/library/internal/service/encoder_worker.go
  - services/library/internal/service/encoder_worker_test.go
  - services/library/internal/handler/episodes.go
  - services/library/internal/handler/episodes_test.go
  - services/library/internal/transport/router.go
  - services/library/internal/config/config.go
  - services/library/cmd/library-api/main.go
  - services/library/Dockerfile
  - services/library/go.mod
  - services/library/go.sum
  - docker/.env.example
  - docker/docker-compose.yml
autonomous: true
requirements:
  - LIB-07
  - LIB-08
user_setup: []

must_haves:
  truths:
    - "Workers claim status='encoding' jobs and progress them through encoding → uploading → done (or failed)."
    - "ffmpeg transcodes the source MP4 into a VOD HLS playlist + 6s segments at H.264/AAC with bitrate capped at min(source, LIBRARY_ENCODE_MAX_BITRATE_KBPS)."
    - "ffprobe pre-flight resolves source duration and bitrate; ffmpeg stderr tail (2 KB) lands in library_jobs.error_text on non-zero exit."
    - "MinIO bucket raw-library is created at startup (idempotent); segments + playlist upload to {shikimori_id|pending/job_id}/{episode_number}/."
    - "library_episodes row inserted on success when shikimori_id is set (with duration_sec + size_bytes); no row inserted when shikimori_id is NULL."
    - "library_filename_patterns is seeded idempotently with Ohys-Raws, SubsPlease, Erai-raws, Leopard-Raws, ARC-Raws."
    - "GET /api/library/episodes/{shikimori_id}/{episode} returns 200 {minio_url, duration_sec, size_bytes} or 404."
    - "ffmpeg binary is available inside the library Docker image."
    - "library_encode_duration_seconds + library_upload_bytes_total + library_filename_detect_fallback_total{uploader} appear on /metrics."
  artifacts:
    - path: "services/library/migrations/002_library_episodes.sql"
      provides: "library_episodes schema (UUID PK, UNIQUE(shikimori_id, episode_number), job_id FK)"
      contains: "CREATE TABLE IF NOT EXISTS library_episodes"
    - path: "services/library/migrations/003_library_filename_patterns.sql"
      provides: "library_filename_patterns table + seeded rows for five uploaders"
      contains: "INSERT INTO library_filename_patterns"
    - path: "services/library/internal/domain/episode.go"
      provides: "Episode GORM model + TableName"
      contains: "type Episode struct"
    - path: "services/library/internal/repo/episode.go"
      provides: "Create / GetByShikimoriEpisode / List on library_episodes"
      contains: "func (r *EpisodeRepository)"
    - path: "services/library/internal/parser/filename/detector.go"
      provides: "DetectEpisode(filename, uploader) (int, ok bool)"
      contains: "func DetectEpisode"
    - path: "services/library/internal/ffmpeg/transcoder.go"
      provides: "Transcoder.Transcode → Result{PlaylistPath, SegmentPaths, DurationSec, SizeBytes}"
      contains: "func (t *Transcoder) Transcode"
    - path: "services/library/internal/minio/writer.go"
      provides: "Writer.Upload + EnsureBucket (idempotent MakeBucket)"
      contains: "func (w *Writer) Upload"
    - path: "services/library/internal/service/encoder_worker.go"
      provides: "EncoderPool claiming status='encoding' jobs"
      contains: "func (p *EncoderPool)"
    - path: "services/library/internal/handler/episodes.go"
      provides: "GET /api/library/episodes/{shikimori_id}/{episode} handler"
      contains: "func (h *EpisodesHandler) Get"
  key_links:
    - from: "services/library/internal/service/download_worker.go (Phase 3)"
      to: "services/library/internal/service/encoder_worker.go"
      via: "UpdateStatus(encoding) → Claim(encoding) handoff via library_jobs.status"
      pattern: "domain.JobStatusEncoding"
    - from: "services/library/internal/service/encoder_worker.go"
      to: "services/library/internal/ffmpeg/transcoder.go"
      via: "Transcode(ctx, sourcePath) call inside processJob"
      pattern: "transcoder.Transcode"
    - from: "services/library/internal/service/encoder_worker.go"
      to: "services/library/internal/minio/writer.go"
      via: "Writer.Upload(prefix, files) after UpdateStatus(uploading)"
      pattern: "writer.Upload"
    - from: "services/library/internal/service/encoder_worker.go"
      to: "services/library/internal/repo/episode.go"
      via: "episodeRepo.Create on success when shikimori_id != ''"
      pattern: "episodeRepo.Create"
    - from: "services/library/internal/handler/episodes.go"
      to: "services/library/internal/repo/episode.go"
      via: "episodeRepo.GetByShikimoriEpisode in the handler"
      pattern: "GetByShikimoriEpisode"
    - from: "services/library/cmd/library-api/main.go"
      to: "services/library/internal/service/encoder_worker.go"
      via: "NewEncoderPool(...).Start(rootCtx) wired alongside the Phase-3 WorkerPool"
      pattern: "NewEncoderPool"
---

<objective>
Extend the library service so workers progress jobs through `encoding` and `uploading` and land HLS segments + playlist in MinIO. Adds: ffmpeg transcoder wrapper (Cmd construction + bounded stderr ring buffer), ffprobe-driven bitrate cap, MinIO writer (concurrent segment upload + idempotent bucket bootstrap), uploader-keyed filename detector with five seeded patterns + generic fallback, two new tables (`library_episodes`, `library_filename_patterns`) with idempotent migrations + seed, encoder worker pool (claims `encoding`, drives `encoding → uploading → done|failed`), read-only episodes endpoint, three new metrics, ffmpeg in the Dockerfile, and full config + .env wiring.

Purpose: Phase 3 left jobs sitting at `status='encoding'`. This phase makes the rest of the pipeline work end-to-end so Phase 5 (admin UI) and Phase 6 (hybrid resolver) have a populated `library_episodes` table to consume.

Output: Workers run encoding + uploading lanes side-by-side with the Phase-3 downloader; a queued job for a known small public-domain torrent completes `queued → downloading → encoding → uploading → done` and lands `raw-library/{shikimori_id}/{episode}/playlist.m3u8` + `segment_NNN.ts` files in MinIO; `GET /api/library/episodes/{shikimori_id}/{episode}` returns 200 with `minio_url`; ffmpeg failure → `failed` with stderr-tail `error_text`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/raw-jp/phases/04-ffmpeg-hls-transcoder-minio-writer/04-CONTEXT.md
@.planning/workstreams/raw-jp/milestones/v0.2-phases/04-ffmpeg-minio-transcoder/04-SPEC.md
@.planning/workstreams/raw-jp/milestones/v0.2-REQUIREMENTS.md
@.planning/workstreams/raw-jp/phases/03-torrent-client-job-queue-metrics/03-SUMMARY.md

@services/library/cmd/library-api/main.go
@services/library/internal/transport/router.go
@services/library/internal/config/config.go
@services/library/internal/domain/job.go
@services/library/internal/repo/job.go
@services/library/internal/service/download_worker.go
@services/library/internal/service/disk_guard.go
@services/library/internal/metrics/library_metrics.go
@services/library/internal/handler/jobs.go
@services/library/migrations/001_library_jobs.sql
@services/library/migrations/migrations.go
@services/library/Dockerfile
@services/library/go.mod

<interfaces>
<!--
Key contracts the executor consumes from Phase 1-3. Use directly — no
codebase exploration needed.
-->

From services/library/internal/domain/job.go (Phase 3):

```go
type JobStatus string
const (
    JobStatusQueued      JobStatus = "queued"
    JobStatusDownloading JobStatus = "downloading"
    JobStatusEncoding    JobStatus = "encoding"   // ← encoder worker claims this
    JobStatusUploading   JobStatus = "uploading"
    JobStatusDone        JobStatus = "done"
    JobStatusFailed      JobStatus = "failed"
    JobStatusCancelled   JobStatus = "cancelled"
)

type Job struct {
    ID           string
    Source       JobSource
    Magnet       string
    Title        string
    Uploader     string
    Quality      string
    SizeBytes    int64
    ShikimoriID  string  // may be ""
    Status       JobStatus
    ProgressPct  int
    ErrorText    string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    CompletedAt  *time.Time
}

func (Job) TableName() string { return "library_jobs" }
```

From services/library/internal/repo/job.go (Phase 3):

```go
// Claim is variadic on statuses — Phase 4 passes JobStatusEncoding.
// IMPORTANT: Claim ALWAYS flips the row to JobStatusDownloading in its
// own transaction. The encoder worker must call UpdateStatus(uploading)
// immediately after Claim to fix the status; for clarity the encoder
// worker calls UpdateStatus(encoding) FIRST so the row never sits at
// 'downloading' while the encoder owns it (visible cosmetic glitch).
//
// Phase 4 alternative (cleaner): introduce a dedicated ClaimForEncode
// method that flips status=encoding (no-op transition, but takes the
// row-lock + updated_at touch). RECOMMENDED.
func (r *JobRepository) Claim(ctx context.Context, statuses ...domain.JobStatus) (*domain.Job, error)
func (r *JobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error)
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, newStatus domain.JobStatus, errorText string) error
```

From services/library/internal/torrent/client.go (Phase 3):

```go
// DownloadHandle.ID() returns the lowercase hex infohash. The encoder
// worker resolves the on-disk source via filepath.Join(cfg.Torrent.DownloadDir, handle.ID(), ...)
// — but the torrent client does NOT expose handles by job ID after
// processJob returns. Phase 4 must store the resolved source path
// onto library_jobs (new column) OR look up the torrent dir by
// scanning {download_dir}/{infohash}/.
//
// SIMPLER PATH (chosen): the encoder worker accepts a SourcePathResolver
// interface and a default implementation that scans
// cfg.Torrent.DownloadDir/{infohash}/ for the largest *.mp4|*.mkv
// file. infohash is derived from the magnet via metainfo.ParseMagnetUri.
// Anacrolix's torrent client writes payloads under DataDir/<infohash>/.
```

From services/library/internal/metrics/library_metrics.go (Phase 3):

```go
type LibraryMetrics struct { /* private fields */ }
func NewLibraryMetrics() *LibraryMetrics
func NewLibraryMetricsWithRegisterer(reg prometheus.Registerer) *LibraryMetrics  // test seam
func (m *LibraryMetrics) IncJobsTotal(status string)
func (m *LibraryMetrics) AddDownloadBytes(n int64)
// Phase 4 adds: ObserveEncodeDuration(seconds), AddUploadBytes(n), IncFilenameDetectFallback(uploader), IncEncodeFailures(reason).
```

From services/streaming MinIO reference (already in the workspace):

```
github.com/minio/minio-go/v7 v7.0.67
```

This is the canonical MinIO Go SDK in this repo. Phase 4 adds it to
services/library/go.mod (require) + relies on go.work for resolution.

ffmpeg command (locked in 04-SPEC.md):
```
ffmpeg -hide_banner -nostats -y -i {source} \
  -c:v libx264 -preset veryfast \
  -b:v {bv}k -maxrate {bv}k -bufsize {bv*2}k \
  -c:a aac -b:a 128k \
  -hls_time 6 -hls_playlist_type vod \
  -hls_segment_filename {tmp}/segment_%03d.ts \
  {tmp}/playlist.m3u8
```
bv = min(source bitrate from ffprobe, LIBRARY_ENCODE_MAX_BITRATE_KBPS, default 5000)

ffprobe pre-flight (locked):
```
ffprobe -v error -print_format json -show_format -show_streams {source}
```
Parse format.duration (seconds, float as string) and format.bit_rate (bps as string). Convert bit_rate / 1000 → kbps.

MinIO path convention (locked):
- With shikimori_id: raw-library/{shikimori_id}/{episode_number}/{playlist.m3u8, segment_NNN.ts}
- Without:           raw-library/pending/{job_id}/{episode_number}/{playlist.m3u8, segment_NNN.ts}

Existing project MinIO root creds (compose default): minioadmin / minioadmin. Endpoint inside docker network: minio:9000.
</interfaces>

</context>

<tasks>

<task type="auto">
  <name>Task 1: Domain types + migrations 002 + 003 + seed + episode/pattern repos + tests</name>
  <files>
    services/library/migrations/002_library_episodes.sql,
    services/library/migrations/003_library_filename_patterns.sql,
    services/library/migrations/migrations.go,
    services/library/internal/domain/episode.go,
    services/library/internal/domain/filename_pattern.go,
    services/library/internal/repo/episode.go,
    services/library/internal/repo/episode_test.go,
    services/library/internal/repo/episode_integration_test.go,
    services/library/internal/repo/filename_pattern.go,
    services/library/internal/repo/filename_pattern_test.go
  </files>
  <action>
Create `migrations/002_library_episodes.sql` per SPEC: `library_episodes` table with UUID PK (`gen_random_uuid()`), `shikimori_id TEXT NOT NULL`, `episode_number INT NOT NULL`, `job_id UUID REFERENCES library_jobs(id)`, `minio_path TEXT NOT NULL`, `duration_sec INT`, `size_bytes BIGINT`, `created_at TIMESTAMPTZ DEFAULT now()`, `UNIQUE(shikimori_id, episode_number)`. Use `CREATE TABLE IF NOT EXISTS` and wrap the UNIQUE constraint in a `DO $$ ... EXCEPTION WHEN duplicate_object` block (mirror the Phase-3 enum pattern) so re-apply is idempotent. Add a `CREATE INDEX IF NOT EXISTS idx_library_episodes_shikimori (shikimori_id, episode_number)` covering the GET lookup path.

Create `migrations/003_library_filename_patterns.sql` per SPEC: `library_filename_patterns(id uuid PK, uploader TEXT NOT NULL, pattern_regex TEXT NOT NULL, example TEXT, created_at TIMESTAMPTZ DEFAULT now())`. Add `CREATE UNIQUE INDEX IF NOT EXISTS idx_library_filename_patterns_uploader ON library_filename_patterns (uploader)` so the seeded `INSERT ... ON CONFLICT (uploader) DO NOTHING` is idempotent.

Seed the table inside the SAME migration file via five `INSERT INTO library_filename_patterns (uploader, pattern_regex, example) VALUES (...) ON CONFLICT (uploader) DO NOTHING` rows:
  - **Ohys-Raws**: `^\[Ohys-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+\(`  e.g. `[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4`
  - **SubsPlease**: `^\[SubsPlease\]\s+.+?\s+-\s+(\d{1,3})\s+\(`  e.g. `[SubsPlease] Frieren - 12 (1080p) [ABCD1234].mkv`
  - **Erai-raws**: `^\[Erai-raws\]\s+.+?\s+-\s+(\d{1,3})\s+\[`  e.g. `[Erai-raws] Spy x Family - 07 [1080p][Multiple Subtitle].mkv`
  - **Leopard-Raws**: `^\[Leopard-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+RAW\s+`  e.g. `[Leopard-Raws] Re-Zero - 03 RAW (BS11 1280x720 x264 AAC).mp4`
  - **ARC-Raws**: `^\[ARC-Raws\]\s+.+?\s+-\s+(\d{1,3})\s*[\[\(]`  e.g. `[ARC-Raws] Made in Abyss - 05 [1080p].mkv`

Each `pattern_regex` value must have exactly one capture group enclosing the episode number. Validate the inserted regexes compile against the example string by running the detector test (Task 2) before committing.

Extend `services/library/migrations/migrations.go`: add two new `//go:embed` directives — `LibraryEpisodesSQL` (embeds `002_library_episodes.sql`) and `LibraryFilenamePatternsSQL` (embeds `003_library_filename_patterns.sql`). Order matters in main.go; `002` MUST apply after `001` because of the `library_jobs(id)` FK.

Create `internal/domain/episode.go`:
```go
type Episode struct {
    ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
    ShikimoriID   string     `gorm:"type:text;not null;column:shikimori_id" json:"shikimori_id"`
    EpisodeNumber int        `gorm:"type:int;not null;column:episode_number" json:"episode_number"`
    JobID         *string    `gorm:"type:uuid;column:job_id" json:"job_id,omitempty"`
    MinioPath     string     `gorm:"type:text;not null;column:minio_path" json:"minio_path"`
    DurationSec   *int       `gorm:"type:int;column:duration_sec" json:"duration_sec,omitempty"`
    SizeBytes     *int64     `gorm:"type:bigint;column:size_bytes" json:"size_bytes,omitempty"`
    CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
}
func (Episode) TableName() string { return "library_episodes" }
```
`JobID`, `DurationSec`, `SizeBytes` are pointers because they're nullable. `MinioPath` is the bucket-relative prefix (`{shikimori_id}/{ep}/`), NOT a presigned URL — the handler computes the URL at request time.

Create `internal/domain/filename_pattern.go` mirroring the SQL schema (uploader, pattern_regex, example, created_at) — used by the detector to load patterns at startup.

Create `internal/repo/episode.go`:
```go
type EpisodeStore interface { /* matches the methods below */ }
type EpisodeRepository struct { db *gorm.DB }
func NewEpisodeRepository(db *gorm.DB) *EpisodeRepository

// Create returns liberrors.InvalidInput on duplicate (shikimori_id, episode_number) by mapping the pq unique_violation.
func (r *EpisodeRepository) Create(ctx context.Context, ep *domain.Episode) error
// GetByShikimoriEpisode returns liberrors.NotFound on no row, otherwise the populated Episode.
func (r *EpisodeRepository) GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int) (*domain.Episode, error)
// List returns all episodes for a given shikimori_id ordered by episode_number ASC.
func (r *EpisodeRepository) List(ctx context.Context, shikimoriID string) ([]domain.Episode, error)
```
Wrap GORM errors with `liberrors.Wrap` mirroring the Phase-3 `JobRepository` style.

Create `internal/repo/filename_pattern.go`:
```go
type FilenamePatternRepository struct { db *gorm.DB }
func NewFilenamePatternRepository(db *gorm.DB) *FilenamePatternRepository
// LoadAll returns every row ordered by uploader ASC — called once at startup by the detector.
func (r *FilenamePatternRepository) LoadAll(ctx context.Context) ([]domain.FilenamePattern, error)
```

Unit tests:
- `episode_test.go`: validate `TableName()` returns `"library_episodes"`, exercise pointer-field JSON marshalling (nil → omitted), table-driven `IsNull` style helpers if added.
- `episode_integration_test.go` (build tag `integration`): per-test database (mirror Phase-3 pattern from `job_integration_test.go`), apply migrations 001+002+003 in order, exercise `Create / GetByShikimoriEpisode / List`, assert that a second `Create` on the same `(shikimori_id, episode_number)` returns a non-nil error containing "duplicate key" (UNIQUE constraint holds). Also assert that re-running migrations 002+003 twice is a no-op (the seed inserts don't duplicate).
- `filename_pattern_test.go` (unit): exercise `LoadAll` against an in-memory sqlite (`gorm.io/driver/sqlite` is already in the workspace via other services — if not, gate the test behind `integration`). Simpler: only run the integration variant; skip unit-level for this thin DAO.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go build ./... && go vet ./... && go test ./internal/domain/... ./internal/repo/... -count=1</automated>
  </verify>
  <done>
    - Both new SQL files apply cleanly when run a second time (idempotent — verified via integration test that calls `db.Exec(LibraryEpisodesSQL)` twice).
    - `INSERT ... ON CONFLICT (uploader) DO NOTHING` keeps `SELECT count(*) FROM library_filename_patterns` at exactly 5 after any number of re-applies.
    - `domain.Episode` round-trips through GORM `Create → First` with all nullable pointer fields populated correctly.
    - `EpisodeRepository.{Create, GetByShikimoriEpisode, List}` returns the documented types and `NotFound` on miss.
    - Five uploader patterns exist in the migration; each pattern's regex contains exactly ONE `(...)` capture group around `\d{1,3}`.
  </done>
</task>

<task type="auto">
  <name>Task 2: Filename detector with five uploader patterns + generic fallback + tests</name>
  <files>
    services/library/internal/parser/filename/detector.go,
    services/library/internal/parser/filename/detector_test.go
  </files>
  <action>
Create `internal/parser/filename/detector.go` exposing `DetectEpisode(filename, uploader string) (int, bool)` per the SPEC contract. The detector loads patterns at construction time (NOT per-call regex compilation) — define:
```go
type Detector struct {
    patterns map[string]*regexp.Regexp  // keyed by uploader (lowercased)
    fallback *regexp.Regexp             // generic
    metrics  FallbackMetric             // optional, may be nil
}
type FallbackMetric interface {
    IncFilenameDetectFallback(uploader string)
}
func NewDetector(loaded []domain.FilenamePattern, m FallbackMetric) (*Detector, error)
```
`NewDetector` precompiles every row's `pattern_regex` via `regexp.MustCompile` — return an error if any regex fails to compile (this surfaces bad seed data at startup, not mid-job). Lowercase the uploader key (`strings.ToLower`) so lookup is case-insensitive. The generic fallback is hard-coded: `regexp.MustCompile(\`- (\d{1,3})\s*[\(\[]\`)` — matches `Title - 01 (` and `Title - 01 [`.

`Detect(filename, uploader)` flow:
1. Look up `patterns[strings.ToLower(uploader)]`. If present, run its regex; if it matches and the first capture group parses as int in `[1, 9999]`, return `(n, true)`.
2. Otherwise (or on miss), run the generic fallback. If it matches and parses, increment `metrics.IncFilenameDetectFallback(uploader)` (with the original uploader label — empty string for unknown) and return `(n, true)`.
3. If neither matches, return `(0, false)`.

Use `strconv.Atoi` on the capture group. Trim whitespace before parsing. Constants for the fallback regex live at package scope so tests can assert against it.

Add a `NewDetectorFromDB(ctx, repo *repo.FilenamePatternRepository, m FallbackMetric)` convenience constructor that calls `repo.LoadAll` and forwards to `NewDetector` — used by `main.go`.

Tests (`detector_test.go`):
- Table-driven test exercising every seeded uploader pattern against its SPEC example string (5 rows) → assert episode 1, 12, 7, 3, 5 respectively (matching the example strings from Task 1).
- Negative cases: filename without an episode number → `(0, false)`.
- Fallback path: filename like `Generic Anime - 04 (1080p).mkv` with uploader=`Unknown` → `(4, true)` + assert a stub `FallbackMetric` was incremented exactly once with label `"Unknown"`.
- Uploader case-insensitivity: pattern keyed under `ohys-raws` matches when caller passes `Ohys-Raws` or `OHYS-RAWS`.
- Constructor error path: a row whose `pattern_regex` doesn't compile → `NewDetector` returns a non-nil error.

Use a small mock `FallbackMetric` struct (`type stubFallback struct{ calls []string }`) to assert counter increments — no real prometheus registry needed.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go test ./internal/parser/filename/... -count=1 -v</automated>
  </verify>
  <done>
    - `DetectEpisode` returns the correct episode number for each of the 5 seeded uploaders' example strings.
    - Fallback path fires only when the uploader-specific lookup misses, and increments `IncFilenameDetectFallback` exactly once with the original (un-lowercased) uploader label.
    - `NewDetector` rejects bad regex rows at construction (early failure, not runtime panic).
    - Generic fallback regex matches both `- 01 (` and `- 01 [` shapes.
  </done>
</task>

<task type="auto">
  <name>Task 3: ffmpeg transcoder wrapper + ffprobe pre-flight + bounded stderr ring + tests</name>
  <files>
    services/library/internal/ffmpeg/transcoder.go,
    services/library/internal/ffmpeg/transcoder_test.go
  </files>
  <action>
Create `internal/ffmpeg/transcoder.go` implementing the SPEC-locked surface:
```go
type Config struct {
    BinaryPath       string  // LIBRARY_FFMPEG_BIN
    FfprobePath      string  // LIBRARY_FFPROBE_BIN
    Tmpdir           string  // LIBRARY_ENCODE_TMPDIR
    MaxBitrateKbps   int     // LIBRARY_ENCODE_MAX_BITRATE_KBPS (default 5000)
}
type Result struct {
    PlaylistPath string
    SegmentPaths []string
    DurationSec  int
    SizeBytes    int64
}
type Transcoder struct { cfg Config; log *logger.Logger }
func NewTranscoder(cfg Config, log *logger.Logger) *Transcoder
func (t *Transcoder) Transcode(ctx context.Context, sourcePath string) (*Result, error)
```

`Transcode` flow:
1. `mkdir -p` a per-call subdirectory under `cfg.Tmpdir` (use `os.MkdirTemp(cfg.Tmpdir, "encode-")` so concurrent encoders never collide).
2. **Pre-flight ffprobe**: `exec.CommandContext(ctx, cfg.FfprobePath, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", sourcePath)`. Capture stdout, parse the JSON into `struct { Format struct { Duration string; BitRate string } }`. Convert `Duration` → `int(seconds)`, `BitRate` → kbps via `strconv.ParseInt / 1000`. On parse failure, log a warning and continue with `MaxBitrateKbps` as the chosen bitrate.
3. Compute `bv = min(sourceKbps, cfg.MaxBitrateKbps)` — but `bv >= 500` minimum (extremely-low-bitrate sources still get re-encoded reasonably).
4. **ffmpeg command** (CONTEXT-locked argv, NO shell interpolation):
   ```go
   args := []string{
     "-hide_banner", "-nostats", "-y",
     "-i", sourcePath,
     "-c:v", "libx264", "-preset", "veryfast",
     "-b:v", fmt.Sprintf("%dk", bv),
     "-maxrate", fmt.Sprintf("%dk", bv),
     "-bufsize", fmt.Sprintf("%dk", bv*2),
     "-c:a", "aac", "-b:a", "128k",
     "-hls_time", "6",
     "-hls_playlist_type", "vod",
     "-hls_segment_filename", filepath.Join(tmp, "segment_%03d.ts"),
     filepath.Join(tmp, "playlist.m3u8"),
   }
   cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)
   ```
5. **Bounded stderr ring buffer** (2 KB): implement a `type ringBuffer struct { buf []byte; cap int }` with `Write(p []byte) (int, error)` that overwrites oldest bytes when full, and a `String()` method returning the in-order tail. Attach `cmd.Stderr = ring`. Implement inline in this file — no external dep — and unit-test the ring's overwrite semantics with a series of small + large writes.
6. `cmd.Run()`. On non-zero exit, return `fmt.Errorf("ffmpeg failed: %s\nstderr tail: %s", err, ring.String())`. **Do NOT** clean up `tmp` on failure — the caller (encoder worker) writes the error to `library_jobs.error_text` and admins can inspect the partial output.
7. On success, enumerate `tmp/segment_*.ts` via `filepath.Glob`, sort, populate `Result.SegmentPaths`. Set `Result.PlaylistPath = tmp + "/playlist.m3u8"`. `Result.DurationSec = sourceDurationSec` (from ffprobe; fall back to summing segment durations if ffprobe missed). Set `Result.SizeBytes` = playlist + segments total via `os.Stat` summation.
8. Caller (encoder worker) is responsible for `os.RemoveAll(tmp)` after a successful upload — `Transcode` does NOT auto-clean on success.

Tests (`transcoder_test.go`):
- **Fake ffmpeg binary**: create a temp directory, write a shell script `fake_ffmpeg.sh` that:
  - `--succeed` mode: parses args, validates the expected flags are present, writes a fake `playlist.m3u8` + 6 fake `segment_NNN.ts` files into the `-hls_segment_filename` directory, exits 0.
  - `--fail` mode: emits a long stderr line and exits 1.
  
  Use `os.WriteFile` with mode `0o755`. Skip the test on Windows (build tag `unix`).
- **Fake ffprobe**: a separate script emitting a JSON blob with known `Format.Duration="1450.5"` and `Format.BitRate="3200000"` → assert chosen `bv = min(3200, MaxBitrateKbps=5000) = 3200`.
- Cmd construction test: capture the args via the fake script writing them to a sidecar file and assert every locked flag is present in order.
- Failure path: assert `Transcode` returns a non-nil error containing the stderr tail. Assert the temp dir is NOT removed (use `os.Stat` post-failure).
- ringBuffer unit tests: writes < cap → preserves; writes > cap → keeps the last `cap` bytes only; multi-write accumulation across overflow.
- Integration test (`//go:build integration`, requires `INTEGRATION=1` env + system ffmpeg/ffprobe): transcode a real 10-second public-domain MP4 (use `https://test-videos.co.uk/bigbuckbunny/mp4-h264` 1MB sample downloaded into a `testdata/` dir on demand, OR generate one with `ffmpeg -f lavfi -i testsrc=duration=10:size=320x240:rate=30 -c:v libx264 testdata/sample.mp4` in TestMain). Assert `Result.SegmentPaths` has ≥ 1 entry and `Result.PlaylistPath` exists.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go test ./internal/ffmpeg/... -count=1 -v</automated>
  </verify>
  <done>
    - `Transcode` constructs the SPEC-locked ffmpeg argv exactly (verified via fake-binary echo-args test).
    - ffprobe pre-flight extracts duration + source bitrate; chosen `bv = min(source, MaxBitrateKbps)` and `>= 500`.
    - Non-zero ffmpeg exit returns an error whose message ends with the last 2 KB of stderr (ring buffer overflow path covered).
    - Success path returns a populated `Result{PlaylistPath, SegmentPaths, DurationSec, SizeBytes}`; temp dir is NOT cleaned by `Transcode` (caller's responsibility).
    - `INTEGRATION=1` test transcodes a real 10s clip end-to-end if system ffmpeg + ffprobe are present.
  </done>
</task>

<task type="auto">
  <name>Task 4: MinIO writer + idempotent bucket bootstrap + concurrent segment upload + tests</name>
  <files>
    services/library/internal/minio/writer.go,
    services/library/internal/minio/writer_test.go,
    services/library/go.mod,
    services/library/go.sum
  </files>
  <action>
Add `github.com/minio/minio-go/v7 v7.0.67` (or newer compatible) to `services/library/go.mod` and run `go mod tidy` to sync `go.sum`. Use the same version as `services/streaming/go.mod` for workspace-level consistency.

Create `internal/minio/writer.go` exposing:
```go
type Config struct {
    Endpoint        string  // e.g. "minio:9000"
    AccessKey       string
    SecretKey       string
    Bucket          string  // default "raw-library"
    UseSSL          bool
    UploadConcurrency int   // default 8
}
type Writer struct {
    cfg    Config
    client *minio.Client
    log    *logger.Logger
}
func New(cfg Config, log *logger.Logger) (*Writer, error)
// EnsureBucket creates the bucket if absent. Idempotent: ignores
// BucketAlreadyOwnedByYou / BucketAlreadyExists ErrorCode strings.
func (w *Writer) EnsureBucket(ctx context.Context) error
// Upload PUTs every file at filePaths to bucket/prefix/{basename}.
// Segments upload concurrently (errgroup.SetLimit(cfg.UploadConcurrency)).
// playlist.m3u8 is detected by basename and uploaded LAST so HLS clients
// never see a playlist referencing an unfinished segment.
// On any segment failure, returns the first error encountered (errgroup
// semantics) and aborts remaining uploads.
// Returns total bytes uploaded so the caller can emit AddUploadBytes.
func (w *Writer) Upload(ctx context.Context, prefix string, filePaths []string) (uploadedBytes int64, err error)
// URLFor returns the public URL the streaming proxy will fetch from.
// Format: "{scheme}://{endpoint}/{bucket}/{path}" where scheme is http or https
// per UseSSL. NOT a presigned URL — bucket ACL is server-side-only and the
// streaming service proxies these URLs.
func (w *Writer) URLFor(path string) string
```

Implementation notes:
- Use `minio-go/v7`'s `minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(...), Secure: cfg.UseSSL})`.
- `EnsureBucket`: call `client.BucketExists(ctx, bucket)`; if false, `client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})`. On error, type-assert `minio.ErrorResponse` and ignore codes `BucketAlreadyOwnedByYou` and `BucketAlreadyExists` (covers the race where two library instances start at the same time).
- `Upload`: sort `filePaths` so segments precede playlist by sorting alphabetically (`segment_001.ts < segment_002.ts < ... < playlist.m3u8`); BUT a simpler explicit approach is to filter the playlist out into a separate variable, upload segments via errgroup, then upload the playlist last on the main goroutine. Use this explicit approach — it's clearer and the test is easier to write.
- Each file: `client.PutObject(ctx, bucket, prefix+filepath.Base(file), reader, size, minio.PutObjectOptions{ContentType: contentTypeFor(file)})`. ContentType: `.m3u8` → `application/vnd.apple.mpegurl`, `.ts` → `video/mp2t`, default `application/octet-stream`.
- `UploadConcurrency <= 0` defaults to 8.

Tests (`writer_test.go`):
- Unit test against a `minio.Client` substitute is hard (the SDK doesn't expose a small interface). Instead:
  1. Implement an `Uploader` interface inside `writer.go` that wraps `client.PutObject + client.BucketExists + client.MakeBucket`. Have `Writer` accept it via an unexported field so tests can swap a fake.
  2. Add `newWriterWithUploader(cfg, uploader, log)` test seam.
  3. Fake `Uploader` records every PutObject call (bucket, object, size, contentType) and a list of bucket creations. Tests assert call order (segments first via concurrent calls — track via a mutex-protected slice — then playlist last on the main goroutine).
- Test: `EnsureBucket` swallows `BucketAlreadyOwnedByYou` by stubbing the fake's `MakeBucket` to return a `minio.ErrorResponse{Code:"BucketAlreadyOwnedByYou"}`.
- Test: Upload with 6 segments + 1 playlist asserts:
  - All 7 PutObject calls fired with correct object keys = `prefix + basename`.
  - Content-Type label is correct per extension.
  - The playlist PutObject is invoked AFTER every segment PutObject completes (use a synchronization barrier in the fake — increment a "segments done" counter; the playlist call blocks reading it).
- Test: Upload aborts on first error — fake returns `errors.New("simulated")` from the third segment; assert `Upload` returns non-nil error and that the playlist PutObject is NEVER called.
- Test: `URLFor("foo/bar.m3u8")` with `UseSSL=false` + `Endpoint="minio:9000"` + `Bucket="raw-library"` returns `http://minio:9000/raw-library/foo/bar.m3u8`.
- Integration test (`//go:build integration`): connect to the live `minio:9000` (or whatever `MINIO_ENDPOINT` resolves to), create + upload + read back via `GetObject`, then delete. Gate behind `INTEGRATION=1`.

After implementation, run `go mod tidy` inside `services/library/` and confirm `go.sum` is consistent. If `go.work.sum` needs entries, run `go work sync` from repo root.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go mod tidy && go build ./... && go vet ./... && go test ./internal/minio/... -count=1 -v</automated>
  </verify>
  <done>
    - `Writer.EnsureBucket` is idempotent across two consecutive calls (no error).
    - `Writer.Upload` uploads playlist LAST (concurrent-segment-then-playlist order verified by the synchronization-barrier test).
    - Content-Type per file is `application/vnd.apple.mpegurl` for `.m3u8` and `video/mp2t` for `.ts`.
    - Early abort: the playlist is never uploaded if any segment upload fails.
    - `URLFor` returns the documented `{scheme}://{endpoint}/{bucket}/{path}` shape.
    - `go.mod` requires `github.com/minio/minio-go/v7` at the same version `services/streaming` uses.
  </done>
</task>

<task type="auto">
  <name>Task 5: Encoder worker + new metrics + cancellation + failure paths + tests</name>
  <files>
    services/library/internal/service/encoder_worker.go,
    services/library/internal/service/encoder_worker_test.go,
    services/library/internal/metrics/library_metrics.go,
    services/library/internal/metrics/library_metrics_test.go
  </files>
  <action>
Extend `internal/metrics/library_metrics.go` with three new collectors and matching methods. Reuse the same `promauto.With(reg)` factory pattern already in place — add fields to the existing `LibraryMetrics` struct, initialize them inside `NewLibraryMetricsWithRegisterer`, do not introduce a second metrics struct.

New collectors (names locked in 04-SPEC + 04-CONTEXT):
- `library_encode_duration_seconds` Histogram — Buckets `prometheus.ExponentialBuckets(10, 2, 8)` (10s → 1280s); Help: "Wall time taken by ffmpeg to transcode one source file."
- `library_upload_bytes_total` Counter — Help: "Total bytes uploaded to MinIO across all completed jobs."
- `library_filename_detect_fallback_total` CounterVec labeled `uploader` — Help: "Generic-fallback regex hits, labeled by the uploader the job claimed (empty = unknown)."
- `library_encode_failures_total` CounterVec labeled `reason` — Help: "Encoder-worker failures, labeled by reason (ffmpeg_error, upload_error, source_missing, episode_detect_failed, episode_insert_failed)." (CONTEXT marked this as optional-but-useful — INCLUDE IT.)

Methods on `LibraryMetrics`:
- `ObserveEncodeDuration(seconds float64)`
- `AddUploadBytes(n int64)` — guard against `<= 0` (mirror `AddDownloadBytes`).
- `IncFilenameDetectFallback(uploader string)` — empty label → `"unknown"` so Prometheus doesn't reject.
- `IncEncodeFailures(reason string)`

Test additions in `library_metrics_test.go`: register against a fresh `prometheus.NewRegistry()`, invoke each new method, then read back with `testutil.ToFloat64` to assert the increment.

---

Create `internal/service/encoder_worker.go` modeled on `download_worker.go` (Phase 3) — same shape, same logger pattern, same `JobStore` interface usage. Differences:
1. Claims `domain.JobStatusEncoding` instead of `JobStatusQueued`.
2. Drives `encoding → uploading → done` (or `failed`).
3. Calls Transcoder + Writer + EpisodeRepo + FilenameDetector instead of the torrent client.

Interfaces (declared local to this package for testability):
```go
type Transcoder interface {
    Transcode(ctx context.Context, sourcePath string) (*ffmpeg.Result, error)
}
type Uploader interface {
    Upload(ctx context.Context, prefix string, filePaths []string) (uploadedBytes int64, err error)
    URLFor(path string) string
}
type EpisodeStore interface {
    Create(ctx context.Context, ep *domain.Episode) error
}
type SourcePathResolver interface {
    // Resolve returns the on-disk path to the largest video file for the given job's torrent.
    // infohash is derived from the job's magnet URI by the caller.
    Resolve(ctx context.Context, job *domain.Job, infohash string) (string, error)
}
type EncodeMetrics interface {
    IncJobsTotal(status string)
    ObserveEncodeDuration(seconds float64)
    AddUploadBytes(n int64)
    IncEncodeFailures(reason string)
}
type EpisodeDetector interface {
    DetectEpisode(filename, uploader string) (int, bool)
}
```
`Transcoder` and `Uploader` interfaces match the concrete types from Tasks 3 + 4 by signature. Production wiring passes the concrete types; tests pass stubs.

```go
type EncoderPool struct {
    workers       int
    jobRepo       JobStore           // reuse Phase-3 JobStore (Claim/UpdateStatus/GetByID)
    episodeRepo   EpisodeStore
    transcoder    Transcoder
    uploader      Uploader
    detector      EpisodeDetector
    resolver      SourcePathResolver
    metrics       EncodeMetrics
    log           *logger.Logger
    pollInterval  time.Duration
    wg            sync.WaitGroup
}
func NewEncoderPool(workers int, jobRepo JobStore, episodeRepo EpisodeStore, transcoder Transcoder, uploader Uploader, detector EpisodeDetector, resolver SourcePathResolver, metrics EncodeMetrics, log *logger.Logger) *EncoderPool
func (p *EncoderPool) Start(ctx context.Context)
func (p *EncoderPool) Stop(timeout time.Duration) error
```

`processJob` flow per claimed job (with `job.Status == JobStatusEncoding` because `Claim` flipped it to `downloading` cosmetically — call `UpdateStatus(encoding, "")` immediately after Claim to fix the row back to `encoding` for the duration of this work):
1. Derive `infohash` from `job.Magnet` via `metainfo.ParseMagnetUri` (the magnet is already validated; ignore-error is acceptable but log on parse failure → `failed("invalid magnet at encode time")`).
2. `source, err := resolver.Resolve(ctx, job, infohash)` — on error → `UpdateStatus(failed, err.Error())` + `metrics.IncEncodeFailures("source_missing")`.
3. `episode, ok := detector.DetectEpisode(filepath.Base(source), job.Uploader)` — on miss → `UpdateStatus(failed, "could not detect episode number from filename: "+source)` + `metrics.IncEncodeFailures("episode_detect_failed")`. (CONTEXT: jobs without `shikimori_id` STILL need an episode number — without one we don't know what to call the MinIO key.)
4. `start := time.Now(); result, err := transcoder.Transcode(ctx, source)` — on error → `UpdateStatus(failed, err.Error())` + `metrics.IncEncodeFailures("ffmpeg_error")`.
5. `metrics.ObserveEncodeDuration(time.Since(start).Seconds())`.
6. `UpdateStatus(uploading, "")` + `metrics.IncJobsTotal("uploading")`.
7. Compute `prefix`: if `job.ShikimoriID != ""` → `fmt.Sprintf("%s/%d/", job.ShikimoriID, episode)`; else → `fmt.Sprintf("pending/%s/%d/", job.ID, episode)`. Trailing slash matters for the MinIO object key.
8. `files := append(result.SegmentPaths, result.PlaylistPath)` (Writer.Upload internally separates playlist).
9. `bytes, err := uploader.Upload(ctx, prefix, files)` — on error → `UpdateStatus(failed, err.Error())` + `metrics.IncEncodeFailures("upload_error")`. Note: do NOT clean up the local temp dir on upload failure — admin can retry.
10. `metrics.AddUploadBytes(bytes)`.
11. If `job.ShikimoriID != ""`: build `domain.Episode{ShikimoriID, EpisodeNumber: episode, JobID: &job.ID, MinioPath: prefix, DurationSec: &result.DurationSec, SizeBytes: &result.SizeBytes}` and `episodeRepo.Create(ctx, &ep)`. On unique-key error (re-upload of the same episode), log a warning and continue — the new files have already replaced the old in MinIO. On any other error → `UpdateStatus(failed, ...)` + `metrics.IncEncodeFailures("episode_insert_failed")` + return (don't continue to `done` — the episode wasn't recorded).
12. Clean up the local temp dir (`os.RemoveAll(filepath.Dir(result.PlaylistPath))`).
13. `UpdateStatus(done, "")` + `metrics.IncJobsTotal("done")`.

Cancellation: at every step boundary, re-read the row via `jobRepo.GetByID(ctx, job.ID)`. If `status == cancelled`, exit cleanly without further state writes (the DELETE handler already wrote the terminal status). Hook this check after Transcode (so ffmpeg's stdout work isn't wasted by a late cancel mid-encode — ffmpeg's `exec.CommandContext` already aborts on ctx cancellation, which is the primary cancel path for in-flight encodes).

`Stop`: cancel the worker context, wait `timeout` for `wg.Done()`. No DB rewrite needed (encoder doesn't hold long-lived in-memory state beyond `exec.Cmd`, which dies with the context). If a worker is mid-encode at SIGTERM, the next process's `ResumeInterruptedDownloads` does NOT cover `encoding` — extend it OR add a sibling method `ResumeInterruptedEncodes` that rewrites `status='encoding' AND updated_at < now() - 1h` → `queued` so genuinely-stuck rows get reclaimed. RECOMMENDED: add `ResumeInterruptedEncodes` to `JobRepository` (extends `repo/job.go`) and call it once at boot from `main.go`.

Add a `SourcePathResolver` default implementation `internal/service/source_resolver.go` (or inline in `encoder_worker.go`):
```go
type DefaultSourceResolver struct {
    downloadDir string
}
func NewDefaultSourceResolver(downloadDir string) *DefaultSourceResolver
// Resolve scans {downloadDir}/{infohash}/ recursively for the largest *.mp4 / *.mkv / *.avi file and returns its path.
// Returns an error if no video file is found.
func (r *DefaultSourceResolver) Resolve(ctx context.Context, job *domain.Job, infohash string) (string, error)
```
Use `filepath.WalkDir`; track the largest file by size; consider extensions case-insensitively. Cap scan depth at 10 to avoid runaway recursion on pathological torrents.

Tests (`encoder_worker_test.go`):
- Stubs for every interface (in-memory `JobStore`, in-memory `EpisodeStore`, fake `Transcoder` returning a fixed `Result`, fake `Uploader` recording calls, fake `EpisodeDetector` returning a fixed episode number, fake `SourcePathResolver` returning a temp file path).
- Happy path: queued job with `ShikimoriID="123"`, `Uploader="Ohys-Raws"` → after one `processJob` invocation, assert:
  - `UpdateStatus(uploading)` called.
  - `Uploader.Upload` called with prefix `"123/1/"` and the 3 fake files.
  - `episodeRepo.Create` called once with the expected Episode struct.
  - `UpdateStatus(done)` called last.
  - `ObserveEncodeDuration` called once.
  - `AddUploadBytes(<expected bytes>)` called.
- No-shikimori path: same job with `ShikimoriID=""` → assert prefix is `pending/{job.ID}/1/`, `episodeRepo.Create` is NEVER called, status still transitions to `done`.
- ffmpeg failure: stub `Transcoder` returns error → assert `UpdateStatus(failed, ...)` called with the error message, `Uploader.Upload` is NEVER called, `IncEncodeFailures("ffmpeg_error")` fired.
- MinIO failure: stub `Uploader` returns error → assert status=failed, `episodeRepo.Create` NEVER called, `IncEncodeFailures("upload_error")` fired.
- Episode detect failure: stub `Detector.DetectEpisode` returns `(0, false)` → assert status=failed, transcoder NEVER called, `IncEncodeFailures("episode_detect_failed")` fired.
- Cancellation: stub `JobStore.GetByID` returns a row with status=cancelled after the first call → assert worker exits without calling `UpdateStatus(done)`.
- Concurrency: launch 3 worker goroutines, seed 2 encoding rows in the fake JobStore, run with a tight ctx timeout → assert both rows reach `done` and the third worker exits gracefully on empty-queue + ctx.Done().

`DefaultSourceResolver` test:
- Create a temp dir tree mimicking `{tmp}/{infohash}/some.mp4` (small) + `{tmp}/{infohash}/sub/large.mkv` (larger) → assert the resolver returns the `.mkv` path.
- Empty dir → assert error.
- Symlink loop → bounded by depth cap, returns error gracefully.
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go build ./... && go vet ./... && go test ./internal/service/... ./internal/metrics/... -count=1 -v</automated>
  </verify>
  <done>
    - Encoder worker drives a claimed `encoding` row through `encoding → uploading → done` on the happy path.
    - All four failure branches (source missing, episode detect, ffmpeg, upload) flip status to `failed` with the correct `error_text` and increment `library_encode_failures_total{reason}`.
    - `library_episodes` row inserted iff `shikimori_id != ""`.
    - MinIO prefix uses `shikimori_id/{episode}/` or `pending/{job_id}/{episode}/` per the lock.
    - `library_encode_duration_seconds` observed exactly once per successful encode.
    - `library_upload_bytes_total` increases by the byte count returned by the uploader.
    - Cancellation observed mid-flight exits without writing `done`.
    - `DefaultSourceResolver` returns the largest video under the infohash dir; missing dir returns an error.
    - `JobRepository.ResumeInterruptedEncodes` added + called from `main.go` boot.
  </done>
</task>

<task type="auto">
  <name>Task 6: Episodes endpoint + router wiring + config + main.go wiring + Dockerfile ffmpeg + .env.example + compose</name>
  <files>
    services/library/internal/handler/episodes.go,
    services/library/internal/handler/episodes_test.go,
    services/library/internal/transport/router.go,
    services/library/internal/config/config.go,
    services/library/cmd/library-api/main.go,
    services/library/Dockerfile,
    docker/.env.example,
    docker/docker-compose.yml
  </files>
  <action>
**A. Episodes handler** — Create `internal/handler/episodes.go`:
```go
type EpisodeStoreReader interface {
    GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int) (*domain.Episode, error)
}
type URLBuilder interface {
    URLFor(path string) string  // returns the MinIO HTTP URL for a bucket-relative path
}
type EpisodesHandler struct {
    episodeRepo EpisodeStoreReader
    urlBuilder  URLBuilder
    log         *logger.Logger
}
func NewEpisodesHandler(episodeRepo EpisodeStoreReader, urlBuilder URLBuilder, log *logger.Logger) *EpisodesHandler
func (h *EpisodesHandler) Get(w http.ResponseWriter, r *http.Request)
```

`Get` flow:
1. Extract `shikimori_id` via `chi.URLParam(r, "shikimori_id")` — required, non-empty.
2. Extract `episode` via `chi.URLParam(r, "episode")` and `strconv.Atoi`. If parse fails or `episode < 1` → 400 `{"error":"INVALID_INPUT","message":"invalid episode"}` via `httputil.WriteError`.
3. `ep, err := episodeRepo.GetByShikimoriEpisode(ctx, shikimoriID, episode)` — on `liberrors.NotFound` → 404. On other error → 500.
4. Build response:
   ```go
   type episodeResponse struct {
       MinioURL    string `json:"minio_url"`
       DurationSec int    `json:"duration_sec,omitempty"`
       SizeBytes   int64  `json:"size_bytes,omitempty"`
   }
   ```
   `MinioURL = urlBuilder.URLFor(ep.MinioPath + "playlist.m3u8")` (recall `MinioPath` ends with `/`).
5. 200 + JSON via `httputil.WriteJSON`.

Tests (`episodes_test.go`): stub `EpisodeStoreReader` + `URLBuilder`. Cases:
- 200 happy path: returns `{minio_url, duration_sec, size_bytes}` with the expected URL.
- 404 when repo returns `liberrors.NotFound`.
- 400 on non-numeric `episode` path param.
- 400 on `episode=0` or negative.
- 500 on internal repo error.

**B. Router** — Extend `internal/transport/router.go`:
- Add `episodesHandler *handler.EpisodesHandler` parameter to `NewRouter`.
- Inside the `r.Route("/api/library", ...)` block, register `r.Get("/episodes/{shikimori_id}/{episode}", episodesHandler.Get)`. Keep it in the same admin-gated prefix; the gateway gates everything except `/health` (Phase 2 lock).
- Existing parameters retained in order; new parameter appended at the end so call-site diff is minimal.

**C. Config** — Extend `internal/config/config.go`:
- New sub-structs:
  ```go
  type EncodeConfig struct {
      Workers        int
      Tmpdir         string
      FfmpegBin      string
      FfprobeBin     string
      MaxBitrateKbps int
  }
  type MinioConfig struct {
      Endpoint           string
      AccessKey          string
      SecretKey          string
      Bucket             string
      UseSSL             bool
      UploadConcurrency  int
  }
  ```
- Add `Encode EncodeConfig` and `Minio MinioConfig` fields to `Config`.
- Env mapping in `Load()`:
  - `LIBRARY_ENCODE_WORKERS` (default 2)
  - `LIBRARY_ENCODE_TMPDIR` (default `/tmp/encode`)
  - `LIBRARY_FFMPEG_BIN` (default `/usr/bin/ffmpeg`)
  - `LIBRARY_FFPROBE_BIN` (default `/usr/bin/ffprobe`)
  - `LIBRARY_ENCODE_MAX_BITRATE_KBPS` (default 5000)
  - `LIBRARY_MINIO_ENDPOINT` (default `minio:9000`)
  - `LIBRARY_MINIO_ACCESS_KEY` (default `minioadmin` — matches compose root user)
  - `LIBRARY_MINIO_SECRET_KEY` (default `minioadmin` — matches compose root password)
  - `LIBRARY_MINIO_BUCKET` (default `raw-library`)
  - `LIBRARY_MINIO_USE_SSL` (bool — env val "true"/"false"; default false)
  - `LIBRARY_MINIO_UPLOAD_CONCURRENCY` (default 8)
- Add a `getEnvBool(key string, defaultVal bool) bool` helper if not already present.
- DO NOT add `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` (the streaming-service env vars). Library reads its own LIBRARY_MINIO_* values so the two services stay independent.

**D. main.go wiring** — Extend `cmd/library-api/main.go`:
1. After applying `migrations.LibraryJobsSQL`, apply `migrations.LibraryEpisodesSQL` then `migrations.LibraryFilenamePatternsSQL` in order — each via `db.DB.Exec(...).Error`, fatal-log on error.
2. After `jobRepo` construction, call `jobRepo.ResumeInterruptedEncodes(rootCtx)` (added in Task 5) alongside `ResumeInterruptedDownloads`.
3. Construct `episodeRepo := repo.NewEpisodeRepository(db.DB)` and `patternRepo := repo.NewFilenamePatternRepository(db.DB)`.
4. Construct the MinIO writer:
   ```go
   writer, err := lminio.New(lminio.Config{
       Endpoint:          cfg.Minio.Endpoint,
       AccessKey:         cfg.Minio.AccessKey,
       SecretKey:         cfg.Minio.SecretKey,
       Bucket:            cfg.Minio.Bucket,
       UseSSL:            cfg.Minio.UseSSL,
       UploadConcurrency: cfg.Minio.UploadConcurrency,
   }, log)
   if err != nil { log.Fatalw("minio client init", "error", err) }
   if err := writer.EnsureBucket(rootCtx); err != nil { log.Fatalw("minio bucket bootstrap", "error", err) }
   log.Infow("minio writer ready", "endpoint", cfg.Minio.Endpoint, "bucket", cfg.Minio.Bucket)
   ```
   (Use `lminio` alias since `services/library/internal/minio` shadows the SDK import — pick the alias used in the writer file too.)
5. Construct the transcoder: `transcoder := ffmpeg.NewTranscoder(ffmpeg.Config{BinaryPath: cfg.Encode.FfmpegBin, FfprobePath: cfg.Encode.FfprobeBin, Tmpdir: cfg.Encode.Tmpdir, MaxBitrateKbps: cfg.Encode.MaxBitrateKbps}, log)`.
6. Load filename patterns + build the detector at startup:
   ```go
   detector, err := filename.NewDetectorFromDB(rootCtx, patternRepo, libMetrics)
   if err != nil { log.Fatalw("filename detector init (bad seed?)", "error", err) }
   log.Infow("filename detector ready", "patterns", len(loadedPatterns))
   ```
7. Construct the source resolver: `resolver := service.NewDefaultSourceResolver(cfg.Torrent.DownloadDir)`.
8. Construct the encoder pool:
   ```go
   encoderPool := service.NewEncoderPool(
       cfg.Encode.Workers,
       jobRepo,
       episodeRepo,
       transcoder,
       writer,
       detector,
       resolver,
       libMetrics,
       log,
   )
   encoderPool.Start(rootCtx)
   log.Infow("encoder pool started", "workers", cfg.Encode.Workers, "tmpdir", cfg.Encode.Tmpdir)
   ```
9. Construct + wire the episodes handler:
   ```go
   episodesHandler := handler.NewEpisodesHandler(episodeRepo, writer, log)
   ```
   Pass `episodesHandler` to `transport.NewRouter(...)` (the new param).
10. In the shutdown path (after the existing `pool.Stop(...)` call), call `encoderPool.Stop(15 * time.Second)`. Order: encoder pool stop FIRST (so no new uploads kick off after MinIO context is torn down), then downloader pool stop, then `srv.Shutdown`.

**E. Dockerfile** — Extend `services/library/Dockerfile`:
- In the runtime stage (`FROM alpine:3.19`), replace the existing `RUN apk add --no-cache ca-certificates tzdata` with `RUN apk add --no-cache ca-certificates tzdata ffmpeg`. Alpine 3.19's `ffmpeg` package includes `ffmpeg` AND `ffprobe` binaries at `/usr/bin/ffmpeg` + `/usr/bin/ffprobe` — exactly the LIBRARY_FFMPEG_BIN / LIBRARY_FFPROBE_BIN defaults. Confirm with `docker run --rm alpine:3.19 sh -c "apk add --no-cache ffmpeg && which ffmpeg ffprobe"` if needed. Image bloat (~80 MB) is acceptable per the SPEC decision.

**F. docker-compose.yml** — Extend the `library:` service block:
- Append to `environment:`:
  ```yaml
  LIBRARY_MINIO_ENDPOINT: minio:9000
  LIBRARY_MINIO_ACCESS_KEY: ${MINIO_ROOT_USER:-minioadmin}
  LIBRARY_MINIO_SECRET_KEY: ${MINIO_ROOT_PASSWORD:-minioadmin}
  LIBRARY_MINIO_BUCKET: ${LIBRARY_MINIO_BUCKET:-raw-library}
  LIBRARY_MINIO_USE_SSL: "false"
  LIBRARY_ENCODE_WORKERS: ${LIBRARY_ENCODE_WORKERS:-2}
  LIBRARY_ENCODE_TMPDIR: /tmp/encode
  LIBRARY_FFMPEG_BIN: /usr/bin/ffmpeg
  LIBRARY_FFPROBE_BIN: /usr/bin/ffprobe
  LIBRARY_ENCODE_MAX_BITRATE_KBPS: ${LIBRARY_ENCODE_MAX_BITRATE_KBPS:-5000}
  ```
- Append to `depends_on:`:
  ```yaml
  minio:
    condition: service_healthy
  ```
- The `library_minio_staging:/tmp/encode` volume already exists (Phase 1) — leave it. The encoder writes per-call sub-dirs under it.

**G. .env.example** — Extend `docker/.env.example` (append a "Phase 04 — ffmpeg + MinIO writer" block under the existing Phase 03 block; mirror the comment style):
```bash
# -----------------------------------------------------------------------------
# Phase 04 — ffmpeg + MinIO writer (workstream raw-jp / v0.2)
# -----------------------------------------------------------------------------
# Encoder worker pool — claims status='encoding' jobs from library_jobs,
# transcodes via ffmpeg, uploads HLS playlist + segments to MinIO.

# Number of encoder worker goroutines.
# LIBRARY_ENCODE_WORKERS=2

# Scratch dir for ffmpeg playlist + segments (per-call subdir auto-created).
# LIBRARY_ENCODE_TMPDIR=/tmp/encode

# Bitrate cap. Chosen = min(source bitrate via ffprobe, this cap, default 5000).
# LIBRARY_ENCODE_MAX_BITRATE_KBPS=5000

# Paths to the ffmpeg + ffprobe binaries baked into the library image.
# LIBRARY_FFMPEG_BIN=/usr/bin/ffmpeg
# LIBRARY_FFPROBE_BIN=/usr/bin/ffprobe

# MinIO connection. Defaults to the compose-root minio service.
# LIBRARY_MINIO_ENDPOINT=minio:9000
# LIBRARY_MINIO_ACCESS_KEY=minioadmin
# LIBRARY_MINIO_SECRET_KEY=minioadmin
# LIBRARY_MINIO_BUCKET=raw-library
# LIBRARY_MINIO_USE_SSL=false
# LIBRARY_MINIO_UPLOAD_CONCURRENCY=8
```

Run `cd services/library && go build ./... && go vet ./...` to confirm wiring compiles. Run `make redeploy-library` after the commit (Task 7 includes the live smoke).
  </action>
  <verify>
    <automated>cd /data/animeenigma && cd services/library && go build ./... && go vet ./... && go test ./internal/handler/... ./internal/config/... -count=1 -v</automated>
  </verify>
  <done>
    - `GET /api/library/episodes/{shikimori_id}/{episode}` registered in the router; handler returns the documented 200 / 400 / 404 / 500 shapes.
    - `cfg.Encode` + `cfg.Minio` sub-configs populated from the 11 new env vars with the documented defaults.
    - main.go applies migrations 002 + 003 after 001; constructs writer + transcoder + detector + resolver + encoder pool; shutdown stops encoder pool before downloader pool.
    - Dockerfile installs ffmpeg in the runtime stage; `docker run animeenigma-library which ffmpeg ffprobe` resolves both.
    - docker-compose.yml `library:` service has the 11 new env vars + `depends_on: minio`.
    - .env.example documents every new env var in a labeled Phase-04 block.
    - `go build ./...` succeeds inside services/library/.
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 7: Live smoke + SUMMARY.md</name>
  <what-built>
    Full end-to-end pipeline: a queued library_jobs row should now flow `queued → downloading → encoding → uploading → done` with HLS files in MinIO bucket `raw-library/` and a `library_episodes` row when `shikimori_id` is set. ffmpeg failure path populates `error_text` from the stderr ring. `GET /api/library/episodes/{shikimori_id}/{episode}` returns the MinIO URL. Three new metrics on `/metrics`.
  </what-built>
  <how-to-verify>
1. **Build + deploy:**
   ```bash
   make redeploy-library
   make health   # expect "✓ library:8089"
   docker exec animeenigma-library which ffmpeg ffprobe
   # → /usr/bin/ffmpeg + /usr/bin/ffprobe
   ```

2. **Migrations applied:**
   ```bash
   docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d library -c "\d library_episodes" -c "\d library_filename_patterns" -c "SELECT uploader FROM library_filename_patterns ORDER BY uploader"
   ```
   Expect both tables to exist; expect exactly 5 uploader rows (Ohys-Raws, SubsPlease, Erai-raws, Leopard-Raws, ARC-Raws). Restart the library container and re-run — count MUST still be 5 (idempotence holds).

3. **MinIO bucket bootstrap:**
   ```bash
   docker exec animeenigma-minio mc alias set local http://localhost:9000 minioadmin minioadmin
   docker exec animeenigma-minio mc ls local/raw-library
   ```
   Expect the bucket to exist (empty is fine).

4. **Episodes endpoint — empty case:** Mint a temp admin API key (Phase 3 smoke pattern) and `curl http://localhost:8000/api/library/episodes/9999/1 -H "Authorization: Bearer ak_..."` → 404.

5. **Synthetic encoding job (no torrent — fastest live test):**
   - Drop a small video into the library's source dir: `docker exec animeenigma-library sh -c "mkdir -p /data/torrents/fakehash && ffmpeg -f lavfi -i testsrc=duration=10:size=320x240:rate=30 -f lavfi -i sine=frequency=440:duration=10 -c:v libx264 -c:a aac -shortest /data/torrents/fakehash/'[Ohys-Raws] LiveSmoke - 01 (320x240).mp4'"`
   - Insert a job row directly at `status='encoding'` with a manually-set infohash matching the dir name:
     ```sql
     INSERT INTO library_jobs (source, magnet, title, uploader, shikimori_id, status)
     VALUES ('manual', 'magnet:?xt=urn:btih:fakehash&dn=smoke', 'live smoke', 'Ohys-Raws', '57466', 'encoding');
     ```
     (Use a real magnet only if you want to also exercise Phase 3 — for this smoke we shortcut into the encoder lane.)
   - Watch the logs: `make logs-library` should show `ffprobe duration=10`, `ffmpeg done`, MinIO upload entries.
   - Verify MinIO contents: `docker exec animeenigma-minio mc ls --recursive local/raw-library/57466/1/` → expect `playlist.m3u8` + several `segment_NNN.ts`.
   - Verify episode row: `psql -d library -c "SELECT shikimori_id, episode_number, minio_path, duration_sec FROM library_episodes WHERE shikimori_id='57466'"` → one row, `duration_sec ~= 10`.
   - Verify endpoint: `curl http://localhost:8000/api/library/episodes/57466/1 -H "Authorization: Bearer ak_..."` → 200 with `{"minio_url":"http://minio:9000/raw-library/57466/1/playlist.m3u8","duration_sec":10,...}`.

6. **No-shikimori path:** Repeat step 5 but with `shikimori_id=NULL` in the INSERT. Verify:
   - MinIO files land under `pending/{job_id}/1/...`.
   - No `library_episodes` row inserted for this job.
   - Job lands `status='done'`.

7. **ffmpeg failure path:** Insert a job pointing at a non-video file (`echo "not a video" > /data/torrents/fakebadhash/badfile.mp4`) → expect `status='failed'` + `error_text` containing ffmpeg's "Invalid data found" tail. Check via `curl /api/library/jobs/{id}`.

8. **Metrics:**
   ```bash
   curl -s http://localhost:8089/metrics | grep -E "^library_(encode_duration|upload_bytes|filename_detect_fallback|encode_failures)"
   ```
   Expect all four collectors present after the smoke runs.

9. **Clean up smoke data:**
   ```sql
   DELETE FROM library_episodes WHERE shikimori_id='57466';
   DELETE FROM library_jobs WHERE title LIKE 'live smoke%';
   ```
   ```bash
   docker exec animeenigma-minio mc rm --recursive --force local/raw-library/57466/
   docker exec animeenigma-minio mc rm --recursive --force local/raw-library/pending/
   docker exec animeenigma-library rm -rf /data/torrents/fakehash /data/torrents/fakebadhash
   ```
   Revoke the temp admin API key (Phase 3 smoke pattern).

10. **Write SUMMARY:** Create `.planning/workstreams/raw-jp/phases/04-ffmpeg-hls-transcoder-minio-writer/04-SUMMARY.md` using `@$HOME/.claude/get-shit-done/templates/summary.md`. Include: every must_haves truth marked OBSERVED; commit hashes for each task; deviations from plan (Rule 1/2/3 categorization); files touched; verification results (unit + INTEGRATION + live smoke output); open items (notably: encoder doesn't yet have its own `ResumeInterruptedEncodes` test against a real DB — covered by the smoke but not the unit suite; and the Phase 5 admin UI is the next gate for `pending/{job_id}/` data to become reachable).
  </how-to-verify>
  <resume-signal>Type "approved" once steps 1–10 pass, or describe what failed and resume planning.</resume-signal>
</task>

</tasks>

<threat_model>

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| gateway → library API | Admin-gated; all `/api/library/*` non-`/health` routes pass JWT + AdminMiddleware at the gateway. Library trusts forwarded identity. |
| library → ffmpeg subprocess | `os/exec` with explicit argv (no shell). Source path comes from the on-disk torrent dir (admin-supplied magnets). |
| library → MinIO | Static V4 credentials over the internal docker network. Bucket ACL is server-side-only; the streaming proxy fronts public access. |
| library → Postgres | Standard parameterized GORM queries. Migration SQL is static + embedded. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-04-01 | Tampering | ffmpeg argv construction (`transcoder.go`) | mitigate | Use `exec.CommandContext(bin, args...)` — never `sh -c`. Every arg passed as a discrete element so no shell metacharacter escapes are possible. SourcePath comes from `filepath.WalkDir` results, NOT user input. |
| T-04-02 | Information Disclosure | MinIO public URLs | mitigate | Bucket ACL is server-side-only (`MakeBucket` with default policy). `URLFor` returns an internal `http://minio:9000/...` URL the gateway must proxy. Phase 6 hybrid resolver consumes this; raw MinIO endpoint is NOT exposed to the public internet (compose binds `127.0.0.1`). |
| T-04-03 | Denial of Service | Unbounded ffmpeg stderr → memory | mitigate | 2 KB bounded ring buffer (`ringBuffer` in `transcoder.go`) caps stderr capture. Long-running ffmpeg bounded by `context.Context` (worker shutdown cancels the cmd). Per-call temp dir bounded by host disk-free guard (Phase 3 LIB-NF-01). |
| T-04-04 | Denial of Service | Encoder worker spawned from cancelled job | mitigate | Re-read row before write (mirror Phase 3 download worker pattern). Cancellation observed post-Transcode + post-Upload boundaries. `exec.CommandContext` ensures ffmpeg dies with worker ctx. |
| T-04-05 | Elevation of Privilege | Episodes endpoint reveals private library | accept | Gateway already gates `/api/library/*` behind AdminMiddleware (Phase 2 lock). Phase 6 hybrid resolver calls this from the catalog service over the internal docker network — both internal. No public exposure. |
| T-04-06 | Spoofing | MinIO upload to wrong key by detect failure | mitigate | Episode-detect failure → `failed` with no upload (encoder worker step 3 returns before Transcode). Job's `Uploader` field is treated as a label only; the detector uses it for lookup but does NOT trust it to bypass validation. |
| T-04-07 | Repudiation | Encoder failures not attributable | mitigate | `library_jobs.error_text` records the stderr tail (2 KB) verbatim; `library_encode_failures_total{reason}` labels every failure; structured logs include `job_id` on every state write. |
| T-04-08 | Tampering | Filename detector regex injection via seed migration | mitigate | Seed SQL is static + version-controlled (`003_library_filename_patterns.sql`). `NewDetector` rejects any regex that fails `regexp.Compile` at startup — bad data fails closed, not open. Generic fallback is hard-coded at compile time. |
| T-04-09 | Information Disclosure | ffprobe / ffmpeg leak source path in stderr | accept | Source paths are container-internal (`/data/torrents/{infohash}/...`). The stderr tail in `error_text` is only readable via admin-gated `/api/library/jobs/{id}` — no public exposure. |
| T-04-10 | Denial of Service | Concurrent upload pool exhausts MinIO connections | mitigate | `UploadConcurrency` default 8 (locked); per-job `errgroup.SetLimit` bounds parallelism. MinIO's default client connection pool is 100+ — well within. |

</threat_model>

<verification>

**Build / vet / unit + integration tests:**
```bash
cd services/library && go build ./... && go vet ./... && go test ./... -count=1
INTEGRATION=1 DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
  go test -tags=integration ./... -count=1
```
All packages: ok. Integration test creates per-test databases, applies migrations 001 + 002 + 003, exercises every repo + detector + writer + transcoder method.

**Live smoke:** Steps 1–9 of Task 7 pass against the running stack (`make redeploy-library` → end-to-end synthetic-encoding job → MinIO files + episodes row + endpoint returns 200 → ffmpeg failure path populates `error_text`).

**Migration idempotence:** Re-applying migrations 002 + 003 against the live `library` database is a no-op (`CREATE TABLE IF NOT EXISTS` + `ON CONFLICT DO NOTHING` for the 5 seed rows).

**Metrics presence:** `curl http://localhost:8089/metrics | grep ^library_encode` returns lines for `library_encode_duration_seconds_bucket`, `library_upload_bytes_total`, `library_filename_detect_fallback_total`, and `library_encode_failures_total`.

</verification>

<success_criteria>

1. Migrations `002_library_episodes.sql` + `003_library_filename_patterns.sql` apply idempotently.
2. `internal/ffmpeg/{transcoder.go,transcoder_test.go}` + `internal/minio/{writer.go,writer_test.go}` exist with passing unit + INTEGRATION tests.
3. `internal/parser/filename/detector.go` covers Ohys-Raws / SubsPlease / Erai-raws / Leopard-Raws / ARC-Raws + generic fallback with table-driven tests.
4. `internal/service/encoder_worker.go` claims `status='encoding'` jobs and drives `encoding → uploading → done|failed`.
5. MinIO bucket `raw-library` exists after first start; live smoke shows segments + playlist at `{shikimori_id}/{episode}/`.
6. `GET /api/library/episodes/{shikimori_id}/{episode}` returns 200 with `minio_url` for a populated episode; 404 otherwise.
7. ffmpeg failure → `library_jobs.status='failed'` + `error_text` populated from the 2 KB stderr tail.
8. ffmpeg + ffprobe present inside the library Docker image at `/usr/bin/ffmpeg` + `/usr/bin/ffprobe`.
9. Job without `shikimori_id` finishes `done` with files at `pending/{job_id}/{episode}/` and NO `library_episodes` row.
10. All three SPEC-required new metrics (`library_encode_duration_seconds`, `library_upload_bytes_total`, `library_filename_detect_fallback_total`) appear on `/metrics`; the optional-but-locked `library_encode_failures_total{reason}` also present.
11. `.env.example` documents every new env var; compose `library:` block wires them.

</success_criteria>

<output>

After Task 7 verification passes, create:
`.planning/workstreams/raw-jp/phases/04-ffmpeg-hls-transcoder-minio-writer/04-SUMMARY.md`

Using the standard summary template. Include:
- Frontmatter with `requirements: [LIB-07, LIB-08]` + commit hashes (one per task).
- Sections: "What was built" (per-task narrative), "Files touched" (split new vs extended), "Verification results" (unit + integration + live smoke transcripts), "Deviations from plan" (Rule 1/2/3 categorized), "Out of scope" (Phase 5 admin UI link flow, Phase 6 hybrid resolver, storage cap retention, per-anime quality profiles), "Open items" (anything carried forward), "Self-Check: PASSED" with a `ls` confirmation of every file in `files_modified`.

Commit message format: `feat(04): <task summary>` per task; final commit: `docs(04): summary + verification`.

</output>
