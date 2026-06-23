# Decoder (library encoder) CPU Soft-Yield — Design Spec

**Date:** 2026-06-23
**Status:** Design approved (brainstorm), pending implementation plan
**Owner service:** `services/library` (ffmpeg transcode / encoder pool) + `docker/docker-compose.yml`

---

## 1. Summary

The `library` service's **decoder** (the ffmpeg HLS transcode step that turns
downloaded RAW torrents into `aeProvider/<mal>/RAW/<ep>/` HLS) can consume its
entire CPU budget at 100% during a transcode and competes for CPU on **equal
footing** with latency-sensitive services (gateway / catalog / streaming /
player). This degrades live-playback p95 whenever the autocache (or admin)
pipeline is draining a backlog.

This change makes the decoder a **well-behaved background batch workload**: it
uses spare CPU at full speed when the host is idle, but **automatically yields**
to interactive services under contention. No quality change, no permanent
throughput loss, no new moving parts.

### Goals
- The decoder must never starve interactive services of CPU under contention.
- Each ffmpeg process must have a bounded, predictable thread footprint.
- Full transcode speed is preserved when the host is otherwise idle.
- All knobs are tunable via env / compose (no code change to retune).

### Non-goals
- Changing video quality (keep `libx264 -preset veryfast` + bitrate logic).
- Changing the hard container caps (`cpus: '4.0'`, `memory: 4G` stay as the
  absolute ceiling).
- Adaptive host-load gating (considered and rejected as over-engineered for v1).
- Touching the autocache planner / download worker (fixed separately 2026-06-23).

---

## 2. Current state (measured 2026-06-23)

| Aspect | Today | Problem |
|--------|-------|---------|
| Encoder concurrency | `LIBRARY_ENCODE_WORKERS=2` → 2 ffmpeg at once | OK, but see threads |
| ffmpeg threads | argv has **no `-threads`** → libx264 auto = host core count (8) | 2 × ~8 = ~16 threads thrashing the container's 4-CPU CFS quota |
| Container CPU cap | `deploy.resources.limits.cpus: '4.0'` → enforced `NanoCpus=4e9` | Hard ceiling holds (4 of 8 cores); good |
| Container memory cap | `memory: 4G` → enforced `Memory=4GiB` | Fine |
| CPU weight | `CpuShares` unset (kernel default 1024) — **equal to every other service** | During transcode, library greedily holds ~half the host at 100% and competes equally with latency-sensitive services |
| Process priority | ffmpeg runs at default niceness (`NI=0`) | Background batch work scheduled equal to the library's own API + (via shares) other containers |

Net: not an unbounded *host* overflow (the 4-CPU cap holds), but within that
budget the decoder is a greedy, equal-priority CPU hog that thrashes (16
threads / 4 cores) and degrades interactive latency.

Runtime facts: container base = Alpine (`/bin/nice` + `/bin/ionice` present);
ffmpeg at `/usr/bin/ffmpeg`; argv composed in
`services/library/internal/ffmpeg/transcoder.go`; encoder pool sized by
`cfg.Encode.Workers` (`services/library/internal/config/config.go`).

---

## 3. Decision (locked during brainstorm)

**Soft-yield** (chosen over "hard lower ceiling" and "adaptive load-gating").
Three independent, additive layers, smallest blast radius first:

### Layer 1 — Cap ffmpeg threads (code)
- Add `Threads int` to `ffmpeg.Config`.
- When `Threads > 0`, emit `-threads <N>` in the argv (place it among the input/
  codec options; a single global `-threads` caps libx264 thread count). When
  `Threads <= 0`, omit it (preserves today's auto behavior — safe default-off
  path for tests / opt-out).
- Wire from new env `LIBRARY_ENCODE_THREADS` (default **3**) via
  `config.EncodeConfig.Threads` → `ffmpeg.Config.Threads`.
- Relationship to keep in mind (documented, not enforced): `WORKERS × THREADS`
  should sit near the 4-CPU cap. Defaults `2 × 3 = 6` (mild, acceptable
  oversubscription — CFS + Layer 2/3 absorb it); owner can set `2 × 2 = 4`
  (exact fit) or `1 × 4 = 4` (one job at a time) without code changes.

### Layer 2 — Deprioritize ffmpeg within the container (code)
- Run each ffmpeg child at a low scheduling priority (default **nice 15**).
- Implementation: Go-native, best-effort. Refactor the transcode from
  `cmd.Run()` to `cmd.Start()` → `syscall.Setpriority(PRIO_PROCESS, cmd.Process.Pid, nice)` → `cmd.Wait()`.
  No external binary dependency. If `Setpriority` errors (or `nice <= 0`), log at
  debug and continue — **never fail the transcode** over priority.
- Wire from new env `LIBRARY_ENCODE_NICE` (default **15**); `0` disables.
- Rationale: keeps the library's own HTTP/health responsive during a transcode,
  and complements Layer 3 (shares act between containers; nice acts within).

### Layer 3 — Lower the container CPU weight (compose)
- Add `cpu_shares: 256` to the `library` service in `docker/docker-compose.yml`
  (vs the implicit 1024 every other service has).
- Effect: under CPU contention the kernel CFS gives `library` ~¼ the weight of a
  default-weight service, so interactive services win the CPU race; when the host
  is idle, library still uses its full 4-CPU cap (shares only bind under
  contention). The existing `cpus: '4.0'` + `memory: 4G` caps are unchanged and
  remain the absolute ceiling.

---

## 4. Config / tunables (no code change to retune)

| Knob | Where | Default | Meaning |
|------|-------|---------|---------|
| `LIBRARY_ENCODE_THREADS` | env (compose) | 3 | `-threads` per ffmpeg; 0 = auto (omit flag) |
| `LIBRARY_ENCODE_NICE` | env (compose) | 15 | child nice level; 0 = don't reprioritize |
| `LIBRARY_ENCODE_WORKERS` | env (compose, existing) | 2 | concurrent transcodes |
| `cpu_shares` | compose | 256 | container CPU weight under contention |
| `cpus` / `memory` | compose (existing) | 4.0 / 4G | absolute hard caps (unchanged) |

All env vars follow the existing `${VAR:-default}` compose pattern.

---

## 5. Verification

- **Unit (transcoder):** argv contains `-threads 3` when `Threads=3`; argv omits
  `-threads` when `Threads=0`. (No live ffmpeg needed — assert on composed argv.)
- **Unit (config):** `LIBRARY_ENCODE_THREADS` / `LIBRARY_ENCODE_NICE` parse with
  the documented defaults.
- **Build/test:** `go build ./...`, `go vet`, `go test ./... -p 1` green (note:
  `internal/torrent/client_test.go` binds anacrolix's fixed port 42069 — flaky
  under concurrent `go test ./...`; use `-p 1` or retry).
- **Post-deploy (live):**
  - `docker inspect animeenigma-library` → `CpuShares=256` (confirm top-level
    `cpu_shares` is honored alongside `deploy.resources`; if not, that is the one
    open risk — see §6).
  - During a transcode: `docker exec animeenigma-library ps -eo pid,ni,comm` shows
    `ffmpeg` at `NI=15`, and `ps -eLf | grep -c ffmpeg`-style thread count is
    bounded (~`THREADS` per process, not host-core count).
  - A transcode still completes and a `library_episodes` row lands (pipeline
    intact).

---

## 6. Risks / open questions

- **`cpu_shares` + `deploy.resources` coexistence.** Compose V2 reads
  `deploy.resources.limits` for `docker compose up`; top-level legacy `cpu_shares`
  maps to `HostConfig.CpuShares` and should coexist. If a given Compose version
  ignores top-level `cpu_shares` when a `deploy:` block is present, fall back to
  setting it via the same mechanism the M494 mem-limit fleet work used, or pin it
  post-create. The post-deploy `docker inspect` check is the gate — do not call
  the change done until `CpuShares=256` is observed.
- **Setpriority race.** The child runs at default nice for the few ms between
  `Start()` and `Setpriority()`. Negligible for minutes-long transcodes; not worth
  a `nice`-wrapper exec (which would re-introduce a binary dependency).
- **Oversubscription at defaults.** `2 workers × 3 threads = 6` threads vs a
  4-CPU cap is intentional (keeps two jobs progressing); Layers 2+3 keep it from
  harming interactive work. Tunable down to an exact fit if desired.

---

## 7. Metrics (project convention — `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — live playback / API stays smooth while the decoder
  drains a backlog; no user-visible regression, modest but real win under load.
- **CDI = 0.04 * 5** — Spread low (transcoder.go, config.go, compose, +tests),
  Shift moderate (CPU scheduling behavior), Effort_Fib 5.
- **MVQ = Griffin 88%/85%** — surgical, well-bounded infra hardening; high
  slop-resistance (cgroup/nice are standard, well-understood levers).
