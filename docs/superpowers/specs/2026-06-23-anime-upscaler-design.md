# Anime Upscaler Service — Design

**Date:** 2026-06-23
**Status:** Approved design (pre-plan)
**Author:** brainstorm with project owner
**Spec:** `docs/superpowers/specs/2026-06-23-anime-upscaler-design.md`

## 1. Summary

A new admin-only microservice **`services/upscaler/`** on the permanent server orchestrates AI upscaling of library anime episodes. It splits a source episode into segments, hands them one at a time to a **provider-agnostic GPU worker container** that runs on an external "secure GPU cloud," collects the upscaled segments + logs back, reassembles + remuxes the original audio/subtitles, and stores the result to MinIO.

The worker is a **self-sustained, remote-controllable container handed manually to an untrusted third-party GPU operator**. It has no inbound ports, no provider API coupling, and no persistent secrets — it **dials home** over one authenticated outbound channel to a hardened, Cloudflare-proxied edge (`ext.animeenigma.org`). All control, logs, **metrics, remote shell**, and updates flow through that single channel.

Two delivery paths share one worker image:

- **Batch (build first):** archive a high-fidelity upscale. Spot/interruptible, chunked, checkpoint + resume, stored to MinIO.
- **Realtime (design-for-now, build-later):** upscale 2K while the user watches; GPU emits live HLS, the existing `services/streaming` proxy restreams it to the player.

Target: 720p–1080p → 2K–4K. Default 2× (720→1440 "2K", 1080→2160 "4K"); 4× available per-job.

## 2. Goals / Non-Goals

### Goals
- Upscale library episodes from the **best-available original source** (not the lossy re-encode).
- Run compute on **rented external GPUs** we don't own and can't API into.
- **Full remote control + observability** (logs, progress, commands, model/config updates) through the worker's own dial-home channel.
- Survive **spot preemption** with at most one segment of lost work.
- **Leak nothing internal** in the handed-over artifact.
- Be testable end-to-end **without a GPU** via a mock model.

### Non-Goals (this iteration)
- No automated GPU provisioning / no provider API (RunPod, Vast, etc.) — the container is handed over manually.
- No automated/scheduled triggering — admin triggers jobs manually for now.
- No player UI wiring of the upscaled variant as a selectable quality (leave a clean seam; follow-up).
- No realtime path implementation (architected for, not built).
- No CDN (per project principle — self-hosted).

## 3. Constraints & Decisions (locked during brainstorm)

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | **Source = best-available original torrent file**, `ffprobe`-detected (NOT assumed `.mkv`) | Upscaling the library's lossy `libx264 veryfast` re-encode amplifies compression artifacts. Containers vary (`.mkv`/`.mp4`/folders); only the codec inside matters. |
| D2 | Pipeline is always `ffmpeg decode → raw frames → AI model → ffmpeg re-encode → remux` | AI models never read containers; they see frames only. |
| D3 | **Provider-agnostic, self-sustained worker** that **dials home** | No inbound ports, NAT-friendly, runs on any GPU host, handed to a third party. The only provider-specific concern (provisioning) is removed entirely. |
| D4 | **No provider API.** Container handed manually to the GPU cloud's operator (untrusted) | We never touch their control plane. The dial-home channel is our sole control surface. |
| D5 | **Pre-segment on our side** (lossless `ffmpeg -c copy`, video stream only); worker processes **one segment at a time**, deletes it after upload | Remote storage is tiny (≤1 segment). Segment = checkpoint/resume unit. Audio/subs/fonts never leave our server. |
| D6 | **Two real models + one mock**, swappable plugins: `best-quality` (heavy), `realtime` (fast), `mock` (CPU, instant) | One image, pick per job. Mock enables GPU-free E2E tests. |
| D7 | **New `services/upscaler/` microservice** (port 8095), admin-only | Codebase one-service-per-concern; keeps the remote-worker control plane out of the already-heavy `library`. |
| D8 | Control/logs over **WebSocket (outbound) + long-poll fallback**; bulk segment data over separate authenticated **HTTPS** | Small chatty control vs large throughput data — don't push GBs over WS frames. |
| D9 | Updates: **stable shell + hot-fetched parts.** Models/config/job params hot via dial-home (no redeploy); shell/code via new image + operator redeploy (rare) | We can't ask the operator to redeploy often. No in-place self-patching (fragile, unsafe on an untrusted host). |
| D10 | Edge = **`ext.animeenigma.org`**, Cloudflare orange-cloud, hardened, minimal worker-only surface | Dedicated external ingress isolated from the main app; CF provides TLS/DDoS/WAF/rate-limit. |
| D11 | **No internal info in the artifact.** Env var is `SERVER_URL`; no `SVO_URL`/internal hostnames; neutral local console output | The operator can read the container; it must reveal nothing about our architecture or content. |
| D12 | Output stored to MinIO as a new variant; **store now, serve later** | Matches the concrete ask ("send to this server for storage"); player wiring is a follow-up. |
| D13 | **Remote shell over dial-home** — admin-initiated, server-relayed exec/PTY multiplexed over the worker's outbound channel | We have no SSH/provider access into the box; this is the only way to debug a live worker. Low blast-radius because the worker holds no secrets. Admin-only, audited, scoped to the container, instantly revocable, disableable. |
| D14 | **Full dial-home telemetry** → Prometheus `upscale_*` + Grafana | Worker reports GPU/host/pipeline metrics over the channel; server adds fleet/control-plane metrics. Observability is a first-class requirement, not an afterthought. Label discipline to avoid `worker_id` cardinality blowup. |

## 4. Architecture Overview

```
                       Permanent server (internal)
  ┌──────────────────────────────────────────────────────────────────┐
  │ services/library  ──source file──▶  services/upscaler (:8095)      │
  │  (/data/torrents)                    • job DB (Postgres)           │
  │                                      • segmenter (ffmpeg -c copy)  │
  │  MinIO raw-library  ◀──writeback──   • reassemble + remux          │
  │                                      • control plane + model reg   │
  │  services/streaming ◀────────────────  (realtime restream, later) │
  └───────────────▲──────────────────────────────▲───────────────────┘
                  │ internal (origin)             │ internal
                  │
         ┌────────┴───────────────────────────────────────────────┐
         │  Cloudflare (orange cloud)  ext.animeenigma.org          │
         │  TLS · DDoS · WAF · rate-limit · Authenticated Origin    │
         └────────▲───────────────────────────────────▲────────────┘
                  │ WS (control/logs, outbound)        │ HTTPS (segment data)
                  │                                     │
         ┌────────┴─────────────────────────────────────┴───────────┐
         │  GPU worker container (external cloud, untrusted operator) │
         │  SERVER_URL=https://ext.animeenigma.org  ENROLL_TOKEN=…    │
         │  ffmpeg decode → AI model → encode                        │
         │  models: [best-quality | realtime | mock]                 │
         │  holds ≤1 segment · process-and-delete · no secrets       │
         └───────────────────────────────────────────────────────────┘
```

**Invariant:** the worker is a pure compute drone. No inbound ports, no long-lived secrets, no persistent state, never retains video. The server holds all truth (job state, segments, results, signing keys).

## 5. Components

### 5.1 `services/upscaler/` (orchestrator, permanent server)

Go microservice mirroring the standard layout (`cmd/upscaler-api/main.go`, `internal/{config,domain,handler,service,repo,transport}`). Responsibilities:

- **Source acquisition** — obtain the original file from `library` (shared volume / internal handoff). `ffprobe` selects the real video file and records codec / pixfmt / fps / HDR / VFR.
- **Segmenter** — split the **video stream only** into keyframe-aligned `-c copy` segments (~30–60s, configurable). Lossless and fast. Audio/subs/fonts/chapters parked on the server for final remux.
- **Job + segment ledger** (Postgres) — the resume source of truth.
- **Control plane** — worker registry, command queue, WebSocket/long-poll handler, log ring-buffer, heartbeat/liveness, lease manager.
- **Model registry** — versioned model binaries, checksum-served to workers on demand.
- **Finalizer** — concatenate upscaled segments, remux original audio/subs/fonts/chapters, normalize timestamps (VFR / 10-bit), re-encode to H.264 HLS, write to MinIO + record DB row.
- **Admin API + minimal admin UI panel** — jobs, fleet, live logs, commands, model registry.

### 5.2 Worker container (external GPU)

One versioned Docker image = **stable shell + hot-fetched parts**:

| Layer | Location | Change cadence | Update path |
|-------|----------|----------------|-------------|
| **Shell**: dial-home agent (reconnect/resume/control loop), `ffmpeg`, model runtime (`realesrgan-ncnn-vulkan` → NVIDIA *or* AMD; CUDA/PyTorch addable behind same plugin iface) | baked in image | rarely | new image → operator redeploys (infrequent) |
| **Models** (weights): `best-quality` + `realtime` baked for cold-start; any other fetched from server `model@version`, checksum-verified, cached on ephemeral disk; `mock` is code | image + registry | often | upload to registry + bump pointer → next job, **no redeploy** |
| **Job spec + runtime config** (model, scale, segment len, encode params, knobs) | delivered per-job / per-command | every job | instant, **no redeploy** |

Boot config (operator-supplied env, the entire handoff):

```
SERVER_URL=https://ext.animeenigma.org
ENROLL_TOKEN=<one-time, supplied out-of-band>
MODE=batch            # batch | realtime
```

No secrets baked in. The container is fully self-sustained: nothing but a GPU + outbound internet to `SERVER_URL` is required.

### 5.3 Edge: `ext.animeenigma.org` (Cloudflare orange-cloud)

A separate, minimal vhost exposing **only** the worker API (`/worker/*` control + segment data) — none of the catalog/admin/gateway routes. See §9 (Security).

## 6. Batch pipeline (build first) + spot resume

1. **Trigger** — admin `POST /api/upscale/jobs {shikimori_id, episode, model, scale}`.
2. **Acquire source** — read original from `library`; if dropped (24h seed window elapsed), request re-acquire. `ffprobe` picks the real video file.
3. **Segment** — video stream only → keyframe-aligned `-c copy` segments. Audio/subs/fonts parked.
4. **Lease loop** — worker leases the next `pending` segment, downloads it over HTTPS (signed capability handle), `ffmpeg decode → AI → encode`, uploads the upscaled segment, **deletes its local copy**, repeats. Each upload is a checkpoint landing immediately on the server.
5. **Spot resume** — leases have a TTL + heartbeat. Worker preempted ⇒ its in-flight segment's lease expires ⇒ any worker re-leases it. ≤1 segment lost. ("Update" reuses this exact path.)
6. **Finalize** — concat upscaled segments + remux original audio/subs/fonts/chapters, normalize timestamps, re-encode H.264 HLS (CRF/bitrate configurable).
7. **Store** — MinIO `raw-library` under `{shikimori_id}/{episode}/upscaled-{res}/` + new DB row.

**Scale defaults:** 2× (720→1440 "2K", 1080→2160 "4K"); 4× available per-job.

## 7. Realtime pipeline (design-for-now, build-later)

Same worker image, `MODE=realtime`, fast model, GPU **pinned to one viewing session** (not spot — can't preempt mid-watch). The worker pulls the source progressively, upscales just-in-time into a live HLS playlist; the server's `streaming` proxy restreams it to the player as another HLS source. Latency-bound; one GPU per active viewer. The worker + protocol support it from day one; only the realtime session manager + streaming wiring are deferred.

## 8. Control / logs / metrics / shell / updates protocol

Two deliberately split channels (D8). The WebSocket is a **multiplexed** transport — commands, logs, heartbeat/metrics, and the remote-shell exec stream are distinct frame types over the one outbound connection:

| Channel | Transport | Carries | Why |
|---|---|---|---|
| **Control/logs/metrics/shell** | one persistent **WebSocket** (worker→server, outbound), long-poll fallback (no shell over fallback) | commands ↓, logs ↑, heartbeat+metrics ↕, exec stdin ↓ / stdout+stderr ↑ | small, chatty, low-latency, NAT-friendly, instant push, multiplexed |
| **Data** | authenticated **HTTPS** GET/PUT | segment download / upscaled-segment upload | bulk throughput; never via WS frames |

**Registration** — on boot the worker exchanges `ENROLL_TOKEN` for a short-lived session credential, opens the WS, sends `register {worker_id, gpu_info, image_version, models_available, capabilities}`. Server records it in the fleet registry.

**Command set** (whitelisted; worker only *acts*):
- `cancel` — abort current segment now, release lease.
- `drain` — finish current segment, then idle.
- `shutdown` — drain then exit (operator's box frees up).
- `reconfigure` — change runtime knobs live (log verbosity, heartbeat interval, encode params, concurrency) — no restart.
- `update` — drain + exit so a new image can replace it.
- `exec` — open a remote-shell session (see Remote shell below).

**Heartbeat/metrics** — every few seconds the worker emits a telemetry frame (`fps, ETA, %, segment idx, VRAM, GPU util, model@version`, plus the full set in §12). Also the liveness signal (missed beats ⇒ expire lease ⇒ re-lease; identical to spot kill).

### Remote shell (exec over dial-home) — D13

Because we have no SSH/provider access into the operator's box, the only way to inspect a live worker (a stuck `ffmpeg`, GPU state via `nvidia-smi`, a sample frame) is to relay a shell over the channel the worker already holds open.

- **Server-initiated, admin-authenticated only.** An admin opens a session from the upscaler admin UI → the server sends an `exec` command over the worker's WS → the worker spawns a PTY (or a one-shot command) **inside its own container** and multiplexes stdin/stdout/stderr as exec frames. The worker never opens a shell unsolicited; the operator cannot trigger it.
- **Scoped to the container**, runs as the container's non-root user — container isolation keeps it off the operator's host. Two modes: full interactive **PTY** (default, for debug) or **command-allowlist** (hardening option).
- **Disableable** (`REMOTE_SHELL_ENABLED`, default on) and **instantly revocable** — revoking the worker session kills any live shell.
- **Fully audited** — every session and command logged server-side with admin identity, worker, job, and timestamps.
- **Time/idle-bounded** sessions.
- **Low blast-radius by construction** — the worker holds no secrets, no creds, and only one episode's segments, so even a full shell can't reach the catalog/DB/MinIO or other jobs.
- Not available over the long-poll fallback (PTY needs the live socket); falls back to allowlisted one-shot commands only if WS is unavailable.

**Logs** — worker tees agent + `ffmpeg` + model stdout/stderr, tags each line `{source, level, segment, ts}`, streams over WS. Server ring-buffers per job in Redis (capped), flushes to object storage on completion. Admin live-tails via SSE; history via REST.

**Updates** (D9):
- **Models / config / job params** → hot via dial-home, no redeploy (the common case).
- **Shell/code** → publish new image; server sends `update` at the worker's next idle boundary; operator relaunches the new tag. Rare; chunk+resume means draining loses nothing.
- **No in-place self-patching of the shell.**

## 9. Security

### Untrusted operator + untrusted box
- **No baked secrets.** One-time `ENROLL_TOKEN` → short-lived session credential → per-job scoped tokens minted by the server. The operator never holds anything reusable or broad.
- The box only ever sees **encrypted video segments of one episode** — never the catalog, DB, MinIO creds, signing keys, or other jobs.
- **Signed capability handles** for segment GET/PUT (HMAC, scoped to one job **and one segment idx and one operation**, short-TTL) — follows the `videoutils.SignStreamURL` pattern with an isolated secret. A worker can reach only the exact segment it currently holds a lease for, in the direction granted, until the lease expires — not other segments, other jobs, or after preemption. (The worker-facing URL does carry `{jobID}/{idx}`; the binding above is what prevents abuse. An opaque per-segment ticket that hides even those is a possible Phase-2 hardening.)
- **Process-and-delete**: nothing retained on the operator's disk after a segment returns.
- **Instant revocation is the kill switch** (drop session, refuse leases, kill any live shell) — replaces the "force-stop via API" we don't have.

### Remote shell (D13)
- **Admin-only + server-initiated.** Exec frames are accepted by the worker **only** as a response to an authenticated `exec` command on its established session; never operator-triggerable.
- **Container-scoped, non-root.** Runs inside the worker container; container isolation keeps it off the operator's host.
- **Audited + bounded.** Full server-side audit (admin identity, commands, timing); idle/time limits; killed on session revoke.
- **Disableable** via `REMOTE_SHELL_ENABLED`; optional command-allowlist mode instead of full PTY.
- **Low blast-radius** — the worker has no secrets and only one job's segments, so a shell cannot exfiltrate anything sensitive.

### Edge hardening (`ext.animeenigma.org`)
- **Origin locked to Cloudflare** — Authenticated Origin Pulls (or CF Tunnel); origin refuses non-CF traffic.
- Lean on CF for edge TLS, DDoS, WAF managed rules, rate-limiting.
- **Dedicated HMAC signing key, isolated from internal `JWT_SECRET`.**
- **Minimal surface** — only `/worker/*`; none of the app routes.
- Strict input validation, body-size caps, request timeouts, per-worker + per-IP rate limits at origin (defense in depth).
- **Generic error responses** (no stack/internal detail to the worker; specifics logged server-side).
- **Optional (opt-in):** Cloudflare **mTLS client certs** issued to workers at handoff as a second gate.

### Info-hiding in the artifact (D11)
- Only address known to the worker is `SERVER_URL=https://ext.animeenigma.org`. No internal hostnames/service names/codenames anywhere in image, env, or docs.
- **Neutral local console** — `connected / leased / processing / idle / error` only; rich logs go up the channel, not to the operator's stdout.
- Neutral image name (no internal codenames).

## 10. Data model (Postgres, `services/upscaler`)

- **`upscale_jobs`** — `id`, `shikimori_id`, `episode`, `model`, `scale`, `status` (`queued|segmenting|upscaling|finalizing|done|failed|cancelled`), `progress_pct`, `source_codec`, `source_pixfmt`, `source_fps`, `created_at`, `updated_at`, `completed_at`, `error_text`.
- **`upscale_segments`** — `job_id`, `idx`, `status` (`pending|leased|done`), `lease_expires_at`, `worker_id`, `bytes`, `started_at`, `completed_at`. *(The resume ledger.)*
- **`upscale_workers`** (fleet) — `worker_id`, `gpu_info`, `image_version`, `models_available`, `session_expires_at`, `last_heartbeat_at`, `current_job_id`, `current_segment_idx`, `status`.
- **Model registry** — `name`, `version`, `checksum`, `object_path`, `created_at`.

Schema management follows the **recent-service pattern: GORM `AutoMigrate` in `main.go` is the source of truth** (themes/recs/notifications; library's older SQL-migration runner is not used for new services). Domain structs carry the GORM tags + `TableName()`.

## 11. Gateway / routing

- **Internal admin** — `/api/upscale/*` → `upscaler:8095` (admin-gated, like `/api/library/*`). Jobs, fleet, logs, commands, model registry.
- **External worker** — `ext.animeenigma.org/worker/*` → a dedicated hardened ingress for `upscaler` (WS control + HTTPS segment data). **Not** exposed on the main app domain. `/internal/*` stays Docker-network-only as elsewhere.

Env vars (new):
```
# upscaler service
DB_*, REDIS_HOST, JWT_SECRET            # standard trio
LIBRARY_URL            # default http://library:8089 — source acquisition
MINIO_*                # writeback to raw-library
EXT_HMAC_SECRET        # edge token signing, ISOLATED from JWT_SECRET
SEGMENT_SECONDS        # default 45
DEFAULT_SCALE          # default 2
REMOTE_SHELL_ENABLED   # default true — gate the dial-home exec/PTY capability
```

Worker env (operator-supplied): `SERVER_URL`, `ENROLL_TOKEN`, `MODE` (see §5.2); `REMOTE_SHELL_ENABLED` is server-side policy (the server simply won't issue `exec` when disabled).

## 12. Metrics & observability (D14)

Full dial-home telemetry is a first-class deliverable. The worker reports its own GPU/host/pipeline metrics over the control channel; the server derives fleet/control-plane metrics and exposes everything as Prometheus `upscale_*` at `services/upscaler` `/metrics` (port 8095), following the project `libs/metrics` + `/metrics` pattern. A Grafana dashboard (`upscaler-fleet` + `upscaler-jobs`) visualizes them.

**Worker-reported (over heartbeat/metrics frame):**
- **GPU** — `upscale_worker_gpu_util_ratio`, `upscale_worker_vram_used_bytes` / `_total_bytes`, `upscale_worker_gpu_temp_celsius`, `upscale_worker_gpu_power_watts` (labels: `gpu_model`, `image_version`).
- **Host** — CPU %, RAM used, ephemeral disk free, net up/down throughput.
- **Pipeline (per stage)** — `upscale_decode_fps`, `upscale_inference_fps`, `upscale_encode_fps`, end-to-end `upscale_segment_fps`, `upscale_model_load_seconds`, frames processed.

**Server-derived (per job/segment):**
- Segment timings — download/decode/inference/encode/upload durations + bytes, retries.
- Job — `upscale_job_progress_ratio`, `upscale_job_eta_seconds`, segments done/total/leased/failed, wall-clock, throughput (frames/s, MB/s).
- Queue — jobs `queued|active|done|failed`, segment queue depth.

**Control-plane / fleet (server-side):**
- `upscale_workers_connected` (gauge; labels `gpu_model`, `image_version`, `model`), WS connects / reconnects / long-poll fallbacks.
- Heartbeat latency, missed heartbeats, **`upscale_lease_expired_total`** (= spot preemptions), re-leases.
- `upscale_command_total{type}` (incl. `exec`), exec sessions opened/duration.
- Edge: enrollment attempts, auth failures, rate-limit hits, bytes in/out.
- Model registry: fetches, checksum failures.

**Label discipline:** keep `worker_id` OUT of high-frequency counters/histograms (cardinality); it appears only on the bounded `upscale_workers_connected` fleet gauge and in logs/audit. Use `gpu_model` / `image_version` / `model` / `status` for aggregation.

## 13. Testing

- **Unit (Go, table-driven, fake MinIO like `library` tests):** segmenter boundaries, lease/resume ledger state machine, finalizer concat/remux argument building, token signing/verification, command queue delivery (incl. `exec`), exec-frame multiplexing + audit logging + `REMOTE_SHELL_ENABLED` gate, metrics frame parsing → Prometheus mapping (+ label-cardinality assertions), edge auth (enrollment → session → per-job token), rate-limit + generic-error behavior.
- **Integration / E2E with `model=mock` (no GPU, CI-runnable):** run the *same worker image* with `--gpus` omitted + `model=mock` against a tiny clip; exercise enrollment → dial-home → lease → segment download → "upscale" → upload → reassemble → remux → MinIO writeback → control commands → **remote-shell round-trip (open exec, run a command, assert output + audit row)** → metrics scrape (`/metrics` exposes expected `upscale_*` series) → **kill-mid-job → resume → completion**.
- **Mock the GPU/real models** — never hit a real GPU or rented cloud in tests.
- Real models (`best-quality`, `realtime`) are thin swaps behind the same plugin interface; validated manually on a real GPU during rollout.

## 14. Phasing

- **Phase 1 (build):** batch path end-to-end — admin manual trigger, segmenter, lease/checkpoint/resume, dial-home control plane (WS + long-poll), logs, **remote shell (exec/PTY)**, **full `upscale_*` metrics + Grafana dashboard**, model registry, finalizer + MinIO writeback, hardened `ext.` edge, `mock` model + full E2E test, neutral worker artifact with `best-quality` baked.
- **Phase 2 (later):** realtime restream via `streaming`; player quality-variant wiring; optional provider auto-launcher; optional CF mTLS client certs.

## 15. Open items / risks

- **Source retention coupling** — original dropped after `library`'s 24h seed window. v1: admin triggers while present; copy-to-staging on request; re-acquire if gone. A proactive "pin/retain for upscale" flag on `library` is a possible enhancement.
- **Throughput reality** — per-frame upscaling is slow (24-min ep ≈ 35k frames ⇒ tens of minutes to hours on one GPU). This is the cost driver and the reason for spot + chunking. Communicate ETA prominently in the admin UI.
- **Codec/HDR edge cases** — 10-bit HEVC (`yuv420p10le`), VFR, AV1 sources, batch/folder torrents, external subs/fonts. The `ffprobe`-driven source picker + finalizer must handle these without clipping or A/V desync.
- **HEVC vs H.264 archival** — v1 stores H.264 HLS (matches hls.js stack). A higher-fidelity HEVC archival master is a possible option (browser HLS HEVC support is weak, so it stays archival-only for now).
- **Realtime feasibility** — keeping a fast model at playback fps for 2K is GPU-dependent; validate before committing to Phase 2 scope.
