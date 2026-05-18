# Phase 4: ffmpeg HLS Transcoder + MinIO Writer - Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** Auto-generated (SPEC pre-written, ambiguity_score 0.20)

<domain>
## Phase Boundary

Workers progress jobs through `encoding` and `uploading`. ffmpeg subprocess wrapper transcodes the downloaded file to H.264 + AAC + HLS (6s segments, VOD playlist). MinIO writer uploads to bucket `raw-library/{shikimori_id-or-pending/job_id}/{episode_number}/`. Auto-detect episode number from source filename via per-uploader regex patterns. On success (with `shikimori_id`), insert `library_episodes` row. Expose `GET /api/library/episodes/{shikimori_id}/{episode}` for Phases 5 and 6 to consume.

**Out of scope:** Admin UI (Phase 5), hybrid resolver (Phase 6), storage cap enforcement, per-anime quality profiles, re-encode on pattern updates.

</domain>

<decisions>
## Implementation Decisions

### Locked from SPEC (`milestones/v0.2-phases/04-ffmpeg-minio-transcoder/04-SPEC.md`)

- **ffmpeg in container:** `apt-get install -y ffmpeg` in library Dockerfile.
- **MinIO client:** `github.com/minio/minio-go/v7` (already used in `services/streaming`).
- **Bucket ACL:** server-side-only (no public read). Streaming proxy in front, mirroring v3.0 self-hosted videos.
- **Segment naming:** `segment_NNN.ts` (zero-padded 3 digits).
- **Default encode workers:** `LIBRARY_ENCODE_WORKERS=2`.
- **Encode tmpdir:** `LIBRARY_ENCODE_TMPDIR=/tmp/encode`. Clean on success, retain on failure.
- **Bitrate cap:** `LIBRARY_ENCODE_MAX_BITRATE_KBPS=5000` default. Cap = min(source bitrate via ffprobe, env cap).
- **Episode detector fallback:** Generic regex `- (\d{1,3})\s*[\(\[]` only when uploader-specific misses. Emits `library_filename_detect_fallback_total{uploader}` counter.
- **No-`shikimori_id` policy:** MinIO upload to `pending/{job_id}/{episode_number}/`. Job lands `done` with no `library_episodes` row. Phase 5 admin UI links + Copies to final path.

### ffmpeg Command (locked)

```
ffmpeg -hide_banner -nostats -y -i {source} \
  -c:v libx264 -preset veryfast \
  -b:v {bv}k -maxrate {bv}k -bufsize {bv*2}k \
  -c:a aac -b:a 128k \
  -hls_time 6 -hls_playlist_type vod \
  -hls_segment_filename {tmp}/segment_%03d.ts \
  {tmp}/playlist.m3u8
```

`bv = min(source bitrate from ffprobe, LIBRARY_ENCODE_MAX_BITRATE_KBPS, default 5000)`.

### Stderr handling (locked)

- Capture stderr to a 2 KB bounded ring buffer.
- On non-zero exit, write the buffer contents as `library_jobs.error_text`.

### Migrations (locked)

- `migrations/002_library_episodes.sql` — uuid PK, shikimori_id, episode_number, job_id (FK), minio_path, duration_sec, size_bytes, created_at. UNIQUE(shikimori_id, episode_number).
- `migrations/003_library_filename_patterns.sql` — uuid PK, uploader, pattern_regex, example. Seed with Ohys-Raws, SubsPlease, Erai-raws, Leopard-Raws, ARC-Raws (each pattern has one capture group around episode number). Idempotent INSERT ON CONFLICT DO NOTHING.

### Episodes Endpoint (locked)

- `GET /api/library/episodes/{shikimori_id}/{episode}` → 200 with `{minio_url, duration_sec, size_bytes}` or 404.
- Admin-gated through the existing gateway prefix (Phase 2 gate).

### Metrics (additions)

- `library_encode_duration_seconds` histogram
- `library_upload_bytes_total` counter
- `library_filename_detect_fallback_total{uploader}` counter
- `library_encode_failures_total{reason}` counter (added during implementation; optional, but useful for alerting)

### MinIO Bucket Bootstrap (locked)

- Create `raw-library` bucket at service start. Idempotent — ignore `BucketAlreadyOwnedByYou`.

### Encoder Worker (locked)

- Separate goroutine type from Phase 3's downloader worker.
- Pool size `LIBRARY_ENCODE_WORKERS` (default 2).
- Claims jobs where `status='encoding'` via the same `Claim()` (FOR UPDATE SKIP LOCKED) → flips to `encoding` (no-op transition for status, just lease).
  - Actually: better to claim from `encoding` and transition to `uploading` after Transcode success; flip to `done` after upload success.
- On Transcode error → `failed` with `error_text` from stderr tail.
- On Upload error → `failed` with `error_text` describing MinIO error.

### Claude's Discretion (autonomous mode)

- Internal helper signatures, package layout details.
- Whether to use a `domain/episode.go` separate from `domain/job.go` (favor separate file).
- Exact concurrent upload pool implementation for MinIO segments (recommend `errgroup` SetLimit 8).
- ffprobe argument list (use minimal `-v error -print_format json -show_format -show_streams`).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/library/` — Phase 1/2/3 in place. Service has DB, torrent client, job queue, worker, disk guard, metrics, admin gate.
- `services/streaming/` — Existing MinIO consumer; reference for client config + bucket helpers.
- `services/library/internal/service/download_worker.go` — Phase 3 worker; encoder worker is analogous but consumes `encoding` jobs.
- `services/library/internal/metrics/` — Extend with three new collectors.
- `services/library/internal/handler/jobs.go` — Reference shape for `GET /episodes/...` handler.
- `services/library/internal/repo/job.go` — Reference GORM repo pattern.
- `services/library/cmd/library-api/main.go` — Wire ffmpeg + minio writer + encoder workers + episodes handler.
- `services/library/Dockerfile` — Extend deps stage with `apt-get install -y ffmpeg`.

### Established Patterns

- Migrations: SQL files + `go:embed` runner in main.go; idempotent.
- Tests: unit via mocks; INTEGRATION=1 for real ffmpeg / real MinIO smoke.
- Metrics: `promauto` + `prometheus.DefaultRegisterer`; collectors live in `internal/metrics/`.
- Worker: `errgroup` with N goroutines; `context.Context` for shutdown.
- Repo: GORM with separate `Create / Get / List / Update*` methods.

### Integration Points

- Router: register `/episodes/{shikimori_id}/{episode}` route under existing admin-gated prefix.
- Compose: existing `library_minio_staging` volume already mounted (Phase 1) — use it for `LIBRARY_ENCODE_TMPDIR`.
- Env: add MinIO + ffmpeg + encode envs to `docker/.env.example`. Reuse the project's MinIO root creds from `docker/.env`.
- MinIO endpoint: `minio:9000` (internal docker DNS).

</code_context>

<specifics>
## Specific Ideas

- SPEC reference at `milestones/v0.2-phases/04-ffmpeg-minio-transcoder/04-SPEC.md` is authoritative.
- Test for full pipeline: queued → downloading → encoding → uploading → done. Best-effort with a small public-domain torrent if env allows; otherwise gate behind `INTEGRATION=1` and use a local source file.
- Verification: `mc ls raw-library/{shikimori_id}/1/` should show playlist.m3u8 + segment_*.ts files.

</specifics>

<deferred>
## Deferred Ideas

- Admin UI (Phase 5) — episode linker dialog when shikimori_id=NULL.
- Hybrid resolver (Phase 6) — catalog calls episodes endpoint then falls back to AllAnime.
- Storage cap / retention policy — v0.2.1 followup.
- Per-anime quality profiles — out of scope; everything is 1080p-cap.
- Re-encode on pattern update — admin manually requeues.

</deferred>
