---
id: LIB-ffmpeg-minio
title: ffmpeg HLS transcoder + MinIO writer + episode-number detection
workstream: raw-jp
milestone: v0.2
phase: 04
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.20
mode: --auto
---

# Phase 04 (workstream `raw-jp`, milestone v0.2): ffmpeg HLS Transcoder + MinIO Writer — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.2 Self-Hosted Library
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** LIB-07, LIB-08
**Depends on:** Phase 3 (job queue + worker loop already exists; this phase extends the worker to handle `encoding` + `uploading` states)
**Mode:** `--auto`

## Goal

Workers progress jobs through `encoding` and `uploading`. ffmpeg subprocess wrapper transcodes the downloaded file to H.264 + AAC + HLS (6s segments, VOD playlist). MinIO writer uploads playlist + segments to bucket `raw-library/{shikimori_id}/{episode_number}/`. Auto-detect episode number from the source filename via per-uploader regex patterns. On successful encode + upload, insert a `library_episodes` row.

## Background

**Today, three things are true and need to change:**

1. **Jobs stall at `encoding` after Phase 3.** Phase 3 downloads bytes but stops at the `encoding` state. We need a second worker pool that picks up `encoding` jobs and produces HLS.

2. **ffmpeg is the default tool for HLS segmentation.** It's stable, ubiquitous, and shells out cleanly from Go via `os/exec`. The user's explicit choice in the design doc.

3. **Episode-number auto-detection is required because admin queues raw torrents.** Most uploader filenames embed the episode number in a predictable position (`[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4`). A small regex catalogue covers ~95% of cases; admins manually link the rest via the Phase 5 UI.

**The implementation:**
- `internal/ffmpeg/transcoder.go` — `os/exec` wrapper with structured Cmd construction + stderr capture.
- `internal/minio/writer.go` — `minio-go/v7` client wrapping `PutObject` for playlist + segments.
- `internal/parser/filename/detector.go` — regex catalogue keyed by uploader name.
- `internal/service/encoder_worker.go` — new worker pool consuming `encoding` jobs.
- `migrations/002_library_episodes.sql` + `003_library_filename_patterns.sql`.
- `internal/handler/episodes.go` — read-only endpoint for the hybrid resolver (Phase 6) to query.

## Requirements

### LIB-07: ffmpeg HLS transcoder

- **Current:** No transcoder.
- **Target:**
  - `internal/ffmpeg/transcoder.go` with:
    ```go
    type Transcoder struct { /* binary path + tmpdir + logger */ }
    func NewTranscoder(cfg Config) *Transcoder
    type Config struct { BinaryPath string; Tmpdir string; MaxBitrateKbps int }
    type Result struct { PlaylistPath string; SegmentPaths []string; DurationSec int; SizeBytes int64 }
    func (t *Transcoder) Transcode(ctx context.Context, sourcePath string) (*Result, error)
    ```
  - Cmd: `ffmpeg -hide_banner -nostats -y -i {source} -c:v libx264 -preset veryfast -b:v {bv}k -maxrate {bv}k -bufsize {bv*2}k -c:a aac -b:a 128k -hls_time 6 -hls_playlist_type vod -hls_segment_filename {tmp}/segment_%03d.ts {tmp}/playlist.m3u8`. `{bv}` = min(source bitrate from ffprobe, `MaxBitrateKbps`, default 5000).
  - Pre-flight `ffprobe` call to extract duration + source bitrate.
  - Stderr captured to a bounded ring buffer (2 KB); on non-zero exit, returned as `error_text`.
  - On success, returns the list of produced files + total duration.
- **Acceptance:** Unit test with a fake ffmpeg binary (a shell script that fakes exit 0 / fakes exit 1) verifies command construction + error paths. Integration test (`INTEGRATION=1`) transcodes a 30s public-domain MP4 → asserts a valid HLS playlist + ≥5 segments exist.

### LIB-08: MinIO writer + episode-number detection + db schema

- **Current:** No `internal/minio/` writer. No `library_episodes` table.
- **Target:**
  - Migration `002_library_episodes.sql`:
    ```sql
    CREATE TABLE library_episodes (
      id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      shikimori_id    TEXT NOT NULL,
      episode_number  INT NOT NULL,
      job_id          UUID REFERENCES library_jobs(id),
      minio_path      TEXT NOT NULL,
      duration_sec    INT,
      size_bytes      BIGINT,
      created_at      TIMESTAMPTZ DEFAULT now(),
      UNIQUE(shikimori_id, episode_number)
    );
    ```
  - Migration `003_library_filename_patterns.sql`:
    ```sql
    CREATE TABLE library_filename_patterns (
      id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      uploader        TEXT NOT NULL,
      pattern_regex   TEXT NOT NULL,
      example         TEXT,
      created_at      TIMESTAMPTZ DEFAULT now()
    );
    ```
    Seeded at service start (idempotent INSERT ON CONFLICT DO NOTHING) with patterns for: Ohys-Raws, SubsPlease, Erai-raws, Leopard-Raws, ARC-Raws. Each pattern has one capture group around the episode number.
  - `internal/parser/filename/detector.go` — `DetectEpisode(filename, uploader string) (int, ok bool)`. Tries uploader-specific pattern first, then a generic fallback (`- (\d{1,3})\s*[\(\[]` — matches `Title - 01 (` and `Title - 01 [`).
  - `internal/minio/writer.go` — `Upload(ctx, prefix string, files []string) error`. Concurrent segment uploads via worker pool (default 8). Uploads playlist last (so streaming clients never see a playlist referring to a not-yet-uploaded segment). MinIO bucket `raw-library` bootstrapped at service start (idempotent `MakeBucket` ignoring `BucketAlreadyOwnedByYou`).
  - `internal/repo/episode.go` — `Create(ctx, episode) error`, `GetByShikimoriEpisode(ctx, shikimoriID, episodeNumber) (*Episode, error)`, `List(ctx, shikimoriID) ([]Episode, error)`.
  - `internal/service/encoder_worker.go` — extends the Phase 3 worker pool with a second goroutine type that claims `status='encoding'` jobs:
    1. Resolve the downloaded source path from the torrent client + job ID.
    2. Detect episode number via filename detector.
    3. If the job has no `shikimori_id`, leave `shikimori_id` NULL on the episode and continue (admin will link in Phase 5).
    4. `Transcoder.Transcode(source)` → playlist + segments.
    5. `UpdateStatus(uploading)`.
    6. `Writer.Upload("/{shikimori_id-or-pending}/{episode_number}/", files)`.
    7. `episodeRepo.Create({shikimori_id, episode_number, job_id, minio_path, duration_sec, size_bytes})` — when `shikimori_id` known.
    8. `UpdateStatus(done)`.
  - `internal/handler/episodes.go` — `GET /api/library/episodes/{shikimori_id}/{episode}` returns `{minio_url, duration_sec, size_bytes}` or 404. Used by the hybrid resolver (Phase 6) and the admin UI (Phase 5).
- **Acceptance:**
  1. A queued job for a known small public-domain torrent completes `queued → downloading → encoding → uploading → done`.
  2. MinIO bucket `raw-library` has `<shikimori_id>/1/playlist.m3u8` + ≥5 segments.
  3. `curl` against the MinIO HLS playlist URL plays in `mpv` (smoke).
  4. Job without `shikimori_id` finishes `done` with no `library_episodes` row inserted; admin sees it in Phase 5's pending-link panel.
  5. ffmpeg failure → job `failed` with `error_text` populated from stderr tail (last 2 KB).
  6. `library_encode_duration_seconds` histogram populated.
  7. Unit tests cover episode-number regex extraction across all five seeded uploader patterns (table-driven).

## Acceptance Criteria

1. Migrations `002_library_episodes.sql` and `003_library_filename_patterns.sql` apply idempotently.
2. `internal/ffmpeg/{transcoder.go,transcoder_test.go}` + `internal/minio/{writer.go,writer_test.go}` exist with passing tests.
3. `internal/parser/filename/detector.go` + tests cover Ohys-Raws / SubsPlease / Erai-raws / Leopard-Raws / ARC-Raws variants.
4. Encoder worker claims `encoding` jobs and progresses them through `encoding → uploading → done` (or `failed`).
5. MinIO bucket `raw-library` exists at first start; smoke shows segments + playlist deposited at the expected path.
6. `GET /api/library/episodes/{shikimori_id}/{episode}` returns 200 with `minio_url` for a populated episode; 404 otherwise.
7. ffmpeg failure path populates `error_text` and surfaces via `GET /api/library/jobs/{id}`.

## Auto-selected implementation decisions

- **ffmpeg binary inside container:** Bake into the library Dockerfile (`RUN apt-get install -y ffmpeg`). Yes ~80 MB image bloat; cleaner than a sidecar.
- **MinIO client:** `github.com/minio/minio-go/v7` — already used elsewhere in the project (`services/streaming/`).
- **Bucket ACL:** Server-side-only (no public read). The streaming service proxies the MinIO URLs to the frontend, same pattern as v3.0 self-hosted videos.
- **Segment naming:** `segment_NNN.ts` (zero-padded 3 digits) — matches the design doc and ffmpeg's `%03d`.
- **Concurrent encode workers:** `LIBRARY_ENCODE_WORKERS` default 2 (CPU-bound; bigger value risks thrash).
- **Encode tmpdir:** `LIBRARY_ENCODE_TMPDIR` default `/tmp/encode`. Cleaned up on success (delete the playlist + segments after upload); retained on failure (debugging).
- **Episode-number detector — generic fallback:** Only fires when uploader-specific pattern misses; logs a `library_filename_detect_fallback_total{uploader}` counter so we can spot uploaders we should add a specific pattern for.
- **`shikimori_id` resolution for jobs queued without one:** Episode row NOT inserted. The job lands `done` with `shikimori_id=NULL`. The MinIO upload PATH includes a literal `pending/{job_id}/` prefix so the data is recoverable. Phase 5's admin UI moves the files when the admin links the job (this involves a MinIO server-side `CopyObject` to the proper path; outside this phase's scope — Phase 5 ships the linker).
- **Bitrate cap:** 5 Mbps default; tunable via `LIBRARY_ENCODE_MAX_BITRATE_KBPS`. Most 1080p anime sources are 4-6 Mbps; 5 Mbps preserves quality without bloating storage.

## Touches

- **New:** `services/library/internal/ffmpeg/{transcoder.go,transcoder_test.go}`
- **New:** `services/library/internal/minio/{writer.go,writer_test.go}`
- **New:** `services/library/internal/parser/filename/{detector.go,detector_test.go}`
- **New:** `services/library/internal/repo/episode.go`
- **New:** `services/library/internal/service/encoder_worker.go`
- **New:** `services/library/internal/handler/episodes.go`
- **New:** `services/library/migrations/002_library_episodes.sql`
- **New:** `services/library/migrations/003_library_filename_patterns.sql`
- **Extend:** `services/library/internal/domain/` — `Episode`, `FilenamePattern` types
- **Extend:** `services/library/internal/transport/router.go` (register `/episodes/...` route)
- **Extend:** `services/library/internal/config/config.go` (ffmpeg + minio + encode envs)
- **Extend:** `services/library/cmd/library-api/main.go` (wire writer + encoder workers + handler)
- **Extend:** `services/library/Dockerfile` (`RUN apt-get install -y ffmpeg`)
- **Extend:** `services/library/internal/metrics/library_metrics.go` (add `library_encode_duration_seconds`, `library_upload_bytes_total`, `library_filename_detect_fallback_total`)
- **Extend:** `services/library/go.mod` (`github.com/minio/minio-go/v7`)
- **Extend:** `docker/.env.example` (new `LIBRARY_ENCODE_*` + `LIBRARY_MINIO_*` envs)

## Out of Scope (for this phase)

- Admin UI (Phase 5).
- Hybrid resolver (Phase 6).
- Cleanup of old MinIO segments / storage cap enforcement (v0.2.1 followup).
- Per-anime quality profiles (everything is 1080p-cap, default preset).
- Re-encode on filename-detector pattern catalogue updates (admin can manually requeue).

## Citations to design doc

- Architecture → "ffmpeg/  # HLS-segment transcoder" + "minio/  # bucket writer".
- Persistent storage → `library_episodes` schema + MinIO layout.
- Tech-choices → "Encoding: ffmpeg subprocess: H.264 + AAC + HLS segmenter (6s segments, VOD playlist)".
- Error-handling → "ffmpeg non-zero exit" + "MinIO PUT failure" rows.
