# Decoder CPU Soft-Yield Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `library` decoder (ffmpeg HLS transcode) a well-behaved background batch workload — full speed when the host is idle, automatically yielding to interactive services under CPU contention.

**Architecture:** Three additive layers. (1) Cap ffmpeg threads via a new `-threads` arg. (2) Run each ffmpeg child at low scheduling priority via best-effort `syscall.Setpriority` (refactor `cmd.Run()` → `Start`/`Setpriority`/`Wait`). (3) Lower the container's CPU weight via compose `cpu_shares: 256`. All knobs env/compose-tunable; the existing `cpus: '4.0'` / `memory: 4G` hard caps and the `veryfast` preset are unchanged.

**Tech Stack:** Go (`services/library`), ffmpeg/libx264, Docker Compose, Linux CFS (cgroup CPU shares) + `nice`.

## Global Constraints

- Quality unchanged: keep `-c:v libx264 -preset veryfast` and the existing bitrate logic (`bv`). Do NOT alter preset/bitrate.
- Hard caps unchanged: compose `deploy.resources.limits.cpus: '4.0'` and `memory: 4G` stay exactly as-is.
- Priority is **best-effort**: a failed `Setpriority` (or `Nice <= 0`) must log-and-continue, NEVER fail a transcode.
- `-threads` is emitted ONLY when `Threads > 0`; `Threads <= 0` omits the flag (preserves today's auto behavior).
- Defaults: `LIBRARY_ENCODE_THREADS=3`, `LIBRARY_ENCODE_NICE=15`, `cpu_shares=256`, `LIBRARY_ENCODE_WORKERS=2` (existing).
- Env vars follow the existing `${VAR:-default}` compose pattern.
- Build/test on Go: `go build ./...`, `go vet ./...`, `go test ./... -p 1` (the `internal/torrent/client_test.go` fixed-port-42069 flake means concurrent `go test ./...` is unreliable — use `-p 1` or retry; it is pre-existing, unrelated to this work).
- All work in the worktree off `origin/main`; commit path-scoped (`git commit <pathspec>`); never `git add -A`.

---

### Task 1: Cap ffmpeg threads (`Config.Threads` + `-threads` argv)

**Files:**
- Modify: `services/library/internal/ffmpeg/transcoder.go` (Config struct ~line 35; argv builder ~lines 193-205)
- Test: `services/library/internal/ffmpeg/transcoder_test.go`

**Interfaces:**
- Produces: `ffmpeg.Config.Threads int` — when `>0`, the Transcode argv contains `-threads <N>`; when `<=0`, no `-threads` flag. Consumed by Task 3 (config wiring).

- [ ] **Step 1: Write the failing test**

Add to `services/library/internal/ffmpeg/transcoder_test.go` (it already imports `strings`, `os`, `path/filepath`, `context`; reuses `writeScript`, `fakeFfmpegSucceedScript`, `fakeFfprobeScript` and the `argv.txt` capture pattern from `TestTranscode_SuccessPath`):

```go
func TestTranscode_ThreadsFlagEmittedWhenSet(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg.sh")
	ffprobe := filepath.Join(dir, "ffprobe.sh")
	writeScript(t, ffmpeg, fakeFfmpegSucceedScript)
	writeScript(t, ffprobe, fakeFfprobeScript)
	source := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewTranscoder(Config{
		BinaryPath: ffmpeg, FfprobePath: ffprobe, Tmpdir: dir,
		MaxBitrateKbps: 5000, Threads: 3,
	}, nil)
	res, err := tr.Transcode(context.Background(), source)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(argv)), "\n")
	if !hasAdjacent(lines, "-threads", "3") {
		t.Fatalf("argv missing `-threads 3`: %v", lines)
	}
}

func TestTranscode_ThreadsFlagOmittedWhenZero(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg.sh")
	ffprobe := filepath.Join(dir, "ffprobe.sh")
	writeScript(t, ffmpeg, fakeFfmpegSucceedScript)
	writeScript(t, ffprobe, fakeFfprobeScript)
	source := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewTranscoder(Config{
		BinaryPath: ffmpeg, FfprobePath: ffprobe, Tmpdir: dir,
		MaxBitrateKbps: 5000, Threads: 0,
	}, nil)
	res, err := tr.Transcode(context.Background(), source)
	if err != nil {
		t.Fatalf("Transcode: %v", err)
	}
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(res.PlaylistPath), "argv.txt"))
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	for _, l := range strings.Split(string(argv), "\n") {
		if l == "-threads" {
			t.Fatalf("argv should not contain -threads when Threads=0: %s", string(argv))
		}
	}
}

// hasAdjacent reports whether lines contains a, immediately followed by b.
func hasAdjacent(lines []string, a, b string) bool {
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == a && lines[i+1] == b {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/library && go test ./internal/ffmpeg/ -run TestTranscode_ThreadsFlag -v`
Expected: FAIL — `TestTranscode_ThreadsFlagEmittedWhenSet` (argv missing `-threads 3`) and a compile error if `Config{...Threads:...}` is not yet a field. (If it won't compile, that IS the red — proceed to Step 3.)

- [ ] **Step 3: Add `Threads` to Config and emit `-threads` in argv**

In `services/library/internal/ffmpeg/transcoder.go`, extend the Config struct:

```go
type Config struct {
	BinaryPath     string // path to ffmpeg, e.g. /usr/bin/ffmpeg
	FfprobePath    string // path to ffprobe, e.g. /usr/bin/ffprobe
	Tmpdir         string // root scratch dir; per-call subdir is auto-created
	MaxBitrateKbps int    // bitrate cap; default 5000 if <= 0
	Threads        int    // libx264 thread cap; 0 = auto (omit -threads)
	Nice           int    // child scheduling niceness; 0 = don't reprioritize (Task 2)
}
```

Then change the argv builder. Replace the `args := []string{ ... }` literal with a build that conditionally inserts `-threads` right after the preset:

```go
	args := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", sourcePath,
		"-c:v", "libx264", "-preset", "veryfast",
	}
	if t.cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(t.cfg.Threads))
	}
	args = append(args,
		"-b:v", fmt.Sprintf("%dk", bv),
		"-maxrate", fmt.Sprintf("%dk", bv),
		"-bufsize", fmt.Sprintf("%dk", bv*2),
		"-c:a", "aac", "-b:a", "128k",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentTemplate,
		playlistPath,
	)
```

(`strconv` is already imported.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/library && go test ./internal/ffmpeg/ -v`
Expected: PASS — including the existing `TestTranscode_SuccessPath` (it sets `Threads:0`/unset → no `-threads`, asserting only the SPEC-locked flags, so it stays green) and the two new tests.

- [ ] **Step 5: Commit**

```bash
git commit services/library/internal/ffmpeg/transcoder.go services/library/internal/ffmpeg/transcoder_test.go \
  -m "feat(library): cap ffmpeg -threads via Config.Threads (decoder CPU soft-yield L1)"
```

---

### Task 2: Deprioritize ffmpeg (`Config.Nice` + Start/Setpriority/Wait)

**Files:**
- Modify: `services/library/internal/ffmpeg/transcoder.go` (imports; the `cmd.Run()` block ~lines 206-210)
- Test: `services/library/internal/ffmpeg/transcoder_test.go`

**Interfaces:**
- Consumes: `Config.Nice int` (added in Task 1's struct edit).
- Produces: transcode runs the child at `nice(Nice)` when `Nice > 0`, best-effort; success/failure semantics of `Transcode` are unchanged.

- [ ] **Step 1: Write the failing test**

Add to `transcoder_test.go` — proves the Start/Setpriority/Wait refactor preserves the success path when `Nice` is set (the fake script exercises a real child process):

```go
func TestTranscode_SuccessWithNiceSet(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg.sh")
	ffprobe := filepath.Join(dir, "ffprobe.sh")
	writeScript(t, ffmpeg, fakeFfmpegSucceedScript)
	writeScript(t, ffprobe, fakeFfprobeScript)
	source := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := NewTranscoder(Config{
		BinaryPath: ffmpeg, FfprobePath: ffprobe, Tmpdir: dir,
		MaxBitrateKbps: 5000, Nice: 15,
	}, nil)
	res, err := tr.Transcode(context.Background(), source)
	if err != nil {
		t.Fatalf("Transcode with Nice=15 must still succeed: %v", err)
	}
	if len(res.SegmentPaths) == 0 {
		t.Fatalf("expected segments produced")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/library && go test ./internal/ffmpeg/ -run TestTranscode_SuccessWithNiceSet -v`
Expected: PASS already at the wrapper level (Nice field exists, currently ignored). This test is a **guard** that the Step-3 refactor doesn't break execution — run it before AND after Step 3. (If you prefer a strict red: temporarily it cannot fail since Nice is a no-op; its value is catching a regression in Step 3.)

- [ ] **Step 3: Refactor `cmd.Run()` → Start/Setpriority/Wait**

In `transcoder.go`, add `"syscall"` to the import block. Replace:

```go
	cmd := exec.CommandContext(ctx, t.cfg.BinaryPath, args...)
	ring := newRingBuffer(2048)
	cmd.Stderr = ring
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %s\nstderr tail:\n%s", err, ring.String())
	}
```

with:

```go
	cmd := exec.CommandContext(ctx, t.cfg.BinaryPath, args...)
	ring := newRingBuffer(2048)
	cmd.Stderr = ring
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start failed: %s", err)
	}
	// Best-effort: run the transcode at low scheduling priority so it yields
	// to interactive work. NEVER fail the transcode over priority.
	if t.cfg.Nice > 0 {
		if err := syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, t.cfg.Nice); err != nil && t.log != nil {
			t.log.Debugw("setpriority(ffmpeg) failed; continuing at default priority",
				"pid", cmd.Process.Pid, "nice", t.cfg.Nice, "error", err)
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %s\nstderr tail:\n%s", err, ring.String())
	}
```

(`t.log` is the existing `*logger.Logger` field on `Transcoder`; guard for nil as shown — tests pass `nil`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/library && go test ./internal/ffmpeg/ -v`
Expected: PASS — all existing tests (SuccessPath, FailurePath, RingBuffer, ProbeFallback, BitrateFloor) plus `TestTranscode_SuccessWithNiceSet`. The failure-path test still works because `cmd.Wait()` returns the non-zero exit and the stderr ring is intact.

- [ ] **Step 5: Commit**

```bash
git commit services/library/internal/ffmpeg/transcoder.go services/library/internal/ffmpeg/transcoder_test.go \
  -m "feat(library): run ffmpeg at nice(Config.Nice), best-effort (decoder CPU soft-yield L2)"
```

---

### Task 3: Config + main wiring (`LIBRARY_ENCODE_THREADS` / `LIBRARY_ENCODE_NICE`)

**Files:**
- Modify: `services/library/internal/config/config.go` (EncodeConfig struct ~line 56; defaults block ~line 227)
- Modify: `services/library/cmd/library-api/main.go` (`ffmpeg.NewTranscoder(ffmpeg.Config{...})` ~lines 344-348; the "encoder pool started" log ~line 389)

**Interfaces:**
- Consumes: `ffmpeg.Config.Threads`, `ffmpeg.Config.Nice` (Tasks 1-2).
- Produces: `config.EncodeConfig.Threads int`, `config.EncodeConfig.Nice int`, populated from env with defaults 3 and 15.

- [ ] **Step 1: Add fields + defaults to config**

In `services/library/internal/config/config.go`, extend EncodeConfig:

```go
type EncodeConfig struct {
	Workers        int
	Tmpdir         string
	FfmpegBin      string
	FfprobeBin     string
	MaxBitrateKbps int
	Threads        int
	Nice           int
}
```

And in the defaults block (the `Encode: EncodeConfig{ ... }` literal):

```go
		Encode: EncodeConfig{
			Workers:        getEnvInt("LIBRARY_ENCODE_WORKERS", 2),
			Tmpdir:         getEnv("LIBRARY_ENCODE_TMPDIR", "/tmp/encode"),
			FfmpegBin:      getEnv("LIBRARY_FFMPEG_BIN", "/usr/bin/ffmpeg"),
			FfprobeBin:     getEnv("LIBRARY_FFPROBE_BIN", "/usr/bin/ffprobe"),
			MaxBitrateKbps: getEnvInt("LIBRARY_ENCODE_MAX_BITRATE_KBPS", 5000),
			Threads:        getEnvInt("LIBRARY_ENCODE_THREADS", 3),
			Nice:           getEnvInt("LIBRARY_ENCODE_NICE", 15),
		},
```

- [ ] **Step 2: Wire into the transcoder in main**

In `services/library/cmd/library-api/main.go`, extend the `ffmpeg.NewTranscoder(ffmpeg.Config{...})` literal:

```go
	transcoder := ffmpeg.NewTranscoder(ffmpeg.Config{
		BinaryPath:     cfg.Encode.FfmpegBin,
		FfprobePath:    cfg.Encode.FfprobeBin,
		Tmpdir:         cfg.Encode.Tmpdir,
		MaxBitrateKbps: cfg.Encode.MaxBitrateKbps,
		Threads:        cfg.Encode.Threads,
		Nice:           cfg.Encode.Nice,
	}, log)
```

And add the two knobs to the "encoder pool started" structured log so the running config is observable:

```go
		"workers", cfg.Encode.Workers,
		"threads", cfg.Encode.Threads,
		"nice", cfg.Encode.Nice,
		"tmpdir", cfg.Encode.Tmpdir,
		"ffmpeg_bin", cfg.Encode.FfmpegBin,
		"max_bitrate_kbps", cfg.Encode.MaxBitrateKbps,
```

- [ ] **Step 3: Build + vet + full test**

Run: `cd services/library && go build ./... && go vet ./... && go test ./... -p 1`
Expected: build OK, vet clean, all tests PASS. (If `go test ./...` without `-p 1` fails on `internal/torrent` port 42069, that's the known pre-existing flake — re-run with `-p 1`.)

- [ ] **Step 4: Commit**

```bash
git commit services/library/internal/config/config.go services/library/cmd/library-api/main.go \
  -m "feat(library): wire LIBRARY_ENCODE_THREADS/NICE into the transcoder (decoder CPU soft-yield L3a)"
```

---

### Task 4: Compose — `cpu_shares` + env vars

**Files:**
- Modify: `docker/docker-compose.yml` (the `library:` service — `environment:` block and service level)

**Interfaces:**
- Produces: running `animeenigma-library` container with `HostConfig.CpuShares=256` and `LIBRARY_ENCODE_THREADS`/`LIBRARY_ENCODE_NICE` env present. Verified live in Task 5.

- [ ] **Step 1: Add env vars to the library `environment:` block**

In `docker/docker-compose.yml`, under the `library:` service `environment:` block, next to the existing `LIBRARY_ENCODE_*` lines, add:

```yaml
      LIBRARY_ENCODE_THREADS: ${LIBRARY_ENCODE_THREADS:-3}
      LIBRARY_ENCODE_NICE: ${LIBRARY_ENCODE_NICE:-15}
```

- [ ] **Step 2: Add `cpu_shares` to the library service**

In the same `library:` service block (service level, sibling of `deploy:` / `restart:` — NOT inside `deploy:`), add:

```yaml
    cpu_shares: 256
```

Leave the existing `deploy.resources.limits.cpus: '4.0'` and `memory: 4G` exactly as-is.

- [ ] **Step 3: Validate compose config**

Run: `docker compose -f docker/docker-compose.yml config >/dev/null && echo "compose valid"`
Expected: `compose valid` (no schema errors; `cpu_shares` is an accepted top-level service key).

- [ ] **Step 4: Commit**

```bash
git commit docker/docker-compose.yml \
  -m "chore(infra): library cpu_shares 256 + LIBRARY_ENCODE_THREADS/NICE env (decoder CPU soft-yield L3b)"
```

---

### Task 5: Deploy + live verification

**Files:** none (deploy + observe). Do this only after Tasks 1-4 are committed and pushed to `main`.

- [ ] **Step 1: Push to main, then redeploy library from a clean worktree**

```bash
git fetch origin main -q && git rebase origin/main && git push origin HEAD:main
make redeploy-library   # builds from this worktree; redeploy.sh copies docker/.env
```
Expected: build succeeds, `library is running`, `alias-check ... OK`.

- [ ] **Step 2: Verify cpu_shares is enforced**

Run: `docker inspect animeenigma-library --format 'CpuShares={{.HostConfig.CpuShares}} NanoCpus={{.HostConfig.NanoCpus}} Memory={{.HostConfig.Memory}}'`
Expected: `CpuShares=256 NanoCpus=4000000000 Memory=4294967296`.
**Gate:** if `CpuShares` is `0`/`1024`, the top-level `cpu_shares` was ignored alongside `deploy:` (spec §6 risk) — do not call this done; resolve before proceeding (e.g. confirm Compose version honors it, or set the weight via the same path the M494 mem-limit work used).

- [ ] **Step 3: Verify thread cap + niceness during a live transcode**

Wait for / trigger an autocache or admin transcode, then:
```bash
docker exec animeenigma-library ps -eo pid,ni,comm | grep ffmpeg
```
Expected: ffmpeg present at `NI` = 15. Thread bound (optional): `docker exec animeenigma-library sh -c 'ls /proc/$(pgrep -n ffmpeg)/task | wc -l'` is bounded near `THREADS+overhead`, not host-core count.

- [ ] **Step 4: Confirm the pipeline still works**

Run: `docker exec animeenigma-postgres psql -U postgres -d library -c "SELECT count(*) FROM library_episodes WHERE source='autocache';"`
Expected: count holds or increases (a transcode completes → episode lands); no new `library_jobs` failures attributable to this change.

- [ ] **Step 5: Run `/animeenigma-after-update`**

Lints/builds, redeploys, updates the changelog (Russian Trump-mode), commits + pushes. (Library is already deployed in Step 1; after-update reconciles + adds the changelog.)

---

## Self-Review

**Spec coverage:** Layer 1 threads → Task 1; Layer 2 nice → Task 2; Layer 3 cpu_shares → Task 4; config tunables (`LIBRARY_ENCODE_THREADS`/`NICE`) → Task 3 + Task 4 env; verification (argv test, docker inspect CpuShares, ps NI, pipeline intact) → Tasks 1-2 tests + Task 5; the §6 `cpu_shares`-coexistence risk → Task 5 Step 2 gate. All spec sections covered.

**Placeholder scan:** none — every code step shows complete code; commands have expected output.

**Type consistency:** `Config.Threads`/`Config.Nice` (Task 1 struct edit) are used identically in Task 2 (`t.cfg.Nice`), Task 3 (`cfg.Encode.Threads`/`Nice` → `ffmpeg.Config{Threads, Nice}`). `EncodeConfig.Threads`/`Nice` defined Task 3 Step 1, consumed Task 3 Step 2. `hasAdjacent` helper defined once in Task 1. Defaults (3/15/256) consistent across Tasks 3-4 and the spec.
