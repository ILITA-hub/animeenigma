# Scrub-Preview Overhaul: Library Storyboards + SW Segment Cache — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the scrub-bar hover preview's duplicate-download shadow engine with (Track A) server-generated sprite storyboards for ae/library content and (Track B) a service-worker segment cache that turns the shadow engine's re-downloads into local disk hits for every other provider.

**Architecture:** Track A bolts a second ffmpeg pass onto the existing library transcode (source file is still on disk during the 24h seed window), tiles 160×90 frames into 10×10 JPEG sheets + a `storyboard.vtt` (`url#xywh=` cues), uploads them next to the HLS output in MinIO, and threads a signed storyboard URL through library → catalog `RawStream` → FE `StreamResult` → `ScrubPreview`, which then draws sprite crops with **no shadow engine at all**. Track B adds a passive SW route on `/api/streaming/hls-proxy` segment requests: tee every 200 response into a bounded Cache Storage cache keyed by the upstream `url=` param (signatures stripped), and serve **scrub-marked** requests cache-first. The main player's request path is never served from cache and never blocked.

**Tech Stack:** Go (library, catalog, libs/videoutils), ffmpeg `tile` filter, WebVTT `#xywh` storyboard convention, Workbox `injectManifest` SW (`frontend/web/src/sw.ts`), hls.js `xhrSetup`, Vue 3 + vitest + bun.

**Origin:** feedback report `2026-06-25T04-08-33_claude-code_manual` (owner rejected: watched-frame snapshots, WebCodecs decode, shadow-engine polish). Owner decisions 2026-07-04: storyboards for ae + SW cache for the rest; low quality is *preferred* for previews (bandwidth); reliability = drop/skip on risk; this system has one of the lowest priorities on the platform.

**Metrics:** UXΔ = +3 (Better) · CDI = 0.05 * 21 · MVQ = Griffin 87%/82%

## Global Constraints

- **Lowest-priority system:** storyboard generation/backfill failures must NEVER fail an encode job or an API request; SW cache errors must NEVER break playback. Every new failure path degrades to today's behavior (shadow engine / plain network).
- **Never block a response for a cache write** — SW writes go through `event.waitUntil` after the response is returned (owner directive, verbatim).
- **Drop on risk:** skip SW cache writes when `navigator.storage.estimate()` is unavailable or headroom < 1 GB; skip storyboard work when the library `DiskGuard` disallows.
- **Low quality is preferred** for preview assets: sheets are 160×90 cells, JPEG `-q:v 8`; the shadow engine stays pinned to HLS level 0. Do not add quality-upgrade logic.
- **Main player untouched:** no `xhrSetup`/loader on `useVideoEngine`'s Hls instance; SW never serves the main player from cache (pass-through + background tee only).
- Effort metrics only (UXΔ / CDI / MVQ) — never days/hours (`.planning/CONVENTIONS.md`).
- Frontend: `bun`/`bunx` only; run `/frontend-verify` before finishing FE work. No new i18n strings are introduced (verify none creep in — en/ru/ja parity gates redeploy).
- Go: NEVER run `gofmt -w` / `make fmt` (smart-quote corruption landmine); format only the lines you touch.
- Work only in the worktree `/data/ae-scrub-preview` (base tree is a read-only mirror). Commit with pathspecs (`git add <paths>`), never `git commit -a`. Do not push from subagents — the orchestrator pushes.
- Commit trailer co-authors (every commit):
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Storyboard geometry is locked and shared by BE + FE: **cadence 5 s** (matches `BUCKET_SEC = 5` in `ScrubPreview.vue:76`), **10×10 grid**, **160×90 cells**, sheets named `storyboard_%03d.jpg` (1-based), VTT named `storyboard.vtt`, all under the episode's MinIO prefix `aeProvider/<shikimoriID>/RAW/<ep>/`.

## File Structure

**Track A (backend):**
- Create `services/library/internal/ffmpeg/storyboard.go` (+`storyboard_test.go`) — ffmpeg sprite pass + pure VTT builder.
- Modify `services/library/internal/domain/episode.go` — `HasStoryboard` field; new migration `services/library/migrations/00N_storyboard.sql`.
- Modify `services/library/internal/minio/writer.go` — `UploadStoryboard`.
- Modify `services/library/internal/service/encoder_worker.go` — best-effort storyboard pass after HLS upload.
- Modify `services/library/internal/handler/episodes.go` — `storyboard_url` in episode response.
- Modify `libs/videoutils/proxy.go` — `.vtt` cue-URL rewriting (signs sheet URLs like m3u8 children).
- Modify `services/catalog/internal/service/raw_resolver.go` (+ catalog's library client struct) — `RawStream.Storyboard`.
- Create `services/library/internal/service/storyboard_backfill.go` (+test) — backfill worker for pre-existing episodes.

**Track A (frontend):**
- Create `frontend/web/src/components/player/aePlayer/storyboardVtt.ts` (+`storyboardVtt.spec.ts`) — VTT parser + cue lookup.
- Modify `frontend/web/src/composables/aePlayer/useProviderResolver.ts`, `frontend/web/src/types/aePlayer.ts`, `AePlayer.vue`, `PlayerScrubBar.vue`, `ScrubPreview.vue` — storyboard prop chain + sprite render mode.

**Track B (frontend only):**
- Create `frontend/web/src/pwa/segmentCache.ts` (+`segmentCache.spec.ts`) — key normalization, scrub marker, cache-first/tee handler.
- Modify `frontend/web/src/sw.ts` — register the segment route.
- Modify `frontend/web/src/pwa/registerPwa.ts` — kill-switch also purges `ae-seg-*`.
- Modify `ScrubPreview.vue` — `xhrSetup` scrub marker on the shadow Hls instance.

**Docs:** `docs/aeplayer-reference.md` (canonical player doc) — new preview-sources section.

**Execution order (dependencies + FE serialization):**
- Go dependency chain: 1 → 2 → 3 → 5; Task 4 is independent of all of them; Task 8 needs 1+2.
- FE tasks (6, 7, 9, 10) are STRICTLY SERIAL — only ONE agent may touch `frontend/web` at a time per worktree (bun install/vitest race). Order: 6 → 9 → 7 → 10 (7 also needs Task 5's contract; 10 needs 9).
- Task 11 (docs) last.
- Go tasks and the single active FE task may run in parallel. Suggested schedule: {1, 4, 6} → {2, 9} → {3, 5, 10} → {7, 8} → {11}.

---

### Task 1: ffmpeg storyboard pass + pure VTT builder

**Files:**
- Create: `services/library/internal/ffmpeg/storyboard.go`
- Test: `services/library/internal/ffmpeg/storyboard_test.go`

**Interfaces:**
- Consumes: existing `Transcoder`, `Config` (incl. `.Nice`), `newRingBuffer` from `transcoder.go`.
- Produces: `func (t *Transcoder) Storyboard(ctx context.Context, sourcePath string, durationSec int) (*StoryboardResult, error)`; `type StoryboardResult struct { SheetPaths []string; VTTPath string }`; `func BuildStoryboardVTT(durationSec int) string`; exported consts `StoryboardCadenceSec=5, StoryboardCols=10, StoryboardRows=10, StoryboardTileW=160, StoryboardTileH=90`, `StoryboardVTTName = "storyboard.vtt"`.

- [ ] **Step 1: Write the failing tests** (`storyboard_test.go`, `//go:build unix`, reuse `writeScript` + the argv-sidecar pattern from `transcoder_test.go`)

```go
//go:build unix

package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake ffmpeg: records argv, writes 2 sheet JPEGs next to the output pattern (last arg).
const fakeStoryboardFfmpeg = `#!/bin/sh
` + lastArgPrelude + `
N=$(printf "%03d" 1); echo "jpg" > "$OUTDIR/storyboard_${N}.jpg"
N=$(printf "%03d" 2); echo "jpg" > "$OUTDIR/storyboard_${N}.jpg"
exit 0
`

func TestBuildStoryboardVTT_TwelveSeconds(t *testing.T) {
	got := BuildStoryboardVTT(12)
	if !strings.HasPrefix(got, "WEBVTT\n") {
		t.Fatalf("missing WEBVTT header:\n%s", got)
	}
	// ceil(12/5) = 3 cues; last cue clamps to 12s.
	for _, want := range []string{
		"00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90",
		"00:00:05.000 --> 00:00:10.000\nstoryboard_001.jpg#xywh=160,0,160,90",
		"00:00:10.000 --> 00:00:12.000\nstoryboard_001.jpg#xywh=320,0,160,90",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing cue %q in:\n%s", want, got)
		}
	}
}

func TestBuildStoryboardVTT_SecondSheetAndRowWrap(t *testing.T) {
	// frame index 100 (t=500s) is the first cell of sheet 2; index 10 (t=50s) wraps to row 2.
	got := BuildStoryboardVTT(520)
	if !strings.Contains(got, "00:08:20.000 --> 00:08:25.000\nstoryboard_002.jpg#xywh=0,0,160,90") {
		t.Errorf("frame 100 must land on sheet 2 cell (0,0):\n%s", got)
	}
	if !strings.Contains(got, "00:00:50.000 --> 00:00:55.000\nstoryboard_001.jpg#xywh=0,90,160,90") {
		t.Errorf("frame 10 must wrap to row 2 (y=90):\n%s", got)
	}
}

func TestBuildStoryboardVTT_NonPositiveDuration(t *testing.T) {
	if got := BuildStoryboardVTT(0); got != "" {
		t.Fatalf("want empty VTT for 0 duration, got %q", got)
	}
}

func TestStoryboard_SuccessPath(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fake_ffmpeg.sh")
	writeScript(t, ffmpegBin, fakeStoryboardFfmpeg)
	tr := NewTranscoder(Config{BinaryPath: ffmpegBin, FfprobePath: "/bin/true", Tmpdir: dir}, nil)
	src := filepath.Join(dir, "src.mp4")
	if err := os.WriteFile(src, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tr.Storyboard(context.Background(), src, 900)
	if err != nil {
		t.Fatalf("Storyboard: %v", err)
	}
	if len(res.SheetPaths) != 2 {
		t.Fatalf("SheetPaths len = %d, want 2", len(res.SheetPaths))
	}
	if filepath.Base(res.VTTPath) != StoryboardVTTName {
		t.Fatalf("VTTPath = %q", res.VTTPath)
	}
	vtt, err := os.ReadFile(res.VTTPath)
	if err != nil || !strings.HasPrefix(string(vtt), "WEBVTT") {
		t.Fatalf("VTT file must exist with header: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(filepath.Dir(res.VTTPath), "argv.txt"))
	for _, w := range []string{"fps=1/5", "tile=10x10", "-q:v", "storyboard_%03d.jpg"} {
		if !strings.Contains(string(argv), w) {
			t.Errorf("argv missing %q:\n%s", w, string(argv))
		}
	}
}

func TestStoryboard_FfmpegFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	ffmpegBin := filepath.Join(dir, "fail.sh")
	writeScript(t, ffmpegBin, fakeFfmpegFailScript)
	tr := NewTranscoder(Config{BinaryPath: ffmpegBin, FfprobePath: "/bin/true", Tmpdir: dir}, nil)
	src := filepath.Join(dir, "src.mp4")
	_ = os.WriteFile(src, []byte("s"), 0o644)
	if _, err := tr.Storyboard(context.Background(), src, 900); err == nil {
		t.Fatal("expected error on ffmpeg failure")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /data/ae-scrub-preview/services/library && go test ./internal/ffmpeg/ -run 'Storyboard|BuildStoryboardVTT' -v`
Expected: FAIL — `undefined: BuildStoryboardVTT`, `undefined: StoryboardVTTName`, etc.

- [ ] **Step 3: Implement** (`storyboard.go`)

```go
package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Storyboard geometry — LOCKED, mirrored by the frontend sprite renderer
// (ScrubPreview BUCKET_SEC=5) and BuildStoryboardVTT. Changing any of these
// requires regenerating every stored storyboard.
const (
	StoryboardCadenceSec = 5
	StoryboardCols       = 10
	StoryboardRows       = 10
	StoryboardTileW      = 160
	StoryboardTileH      = 90
	StoryboardVTTName    = "storyboard.vtt"
)

// StoryboardResult is what Storyboard returns on success. The caller owns
// cleanup of the shared temp dir (filepath.Dir(VTTPath)) after upload.
type StoryboardResult struct {
	SheetPaths []string // absolute paths to storyboard_NNN.jpg, sorted ASC
	VTTPath    string   // absolute path to storyboard.vtt in the same dir
}

// Storyboard runs one extra ffmpeg pass over the source: sample 1 frame per
// cadence, letterbox into fixed 160x90 cells, tile 10x10 per JPEG sheet.
// Low JPEG quality is deliberate (preview-only asset, bandwidth-first).
func (t *Transcoder) Storyboard(ctx context.Context, sourcePath string, durationSec int) (*StoryboardResult, error) {
	if durationSec <= 0 {
		durationSec, _ = t.probe(ctx, sourcePath) // backfill callers may not know it
	}
	if durationSec <= 0 {
		return nil, fmt.Errorf("storyboard: unknown duration for %s", sourcePath)
	}
	tmp, err := os.MkdirTemp(t.cfg.Tmpdir, "storyboard-")
	if err != nil {
		return nil, fmt.Errorf("mkdir storyboard tmpdir: %w", err)
	}
	sheetTemplate := filepath.Join(tmp, "storyboard_%03d.jpg")
	vf := fmt.Sprintf(
		"fps=1/%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,tile=%dx%d",
		StoryboardCadenceSec, StoryboardTileW, StoryboardTileH,
		StoryboardTileW, StoryboardTileH, StoryboardCols, StoryboardRows,
	)
	args := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", sourcePath,
		"-vf", vf,
		"-q:v", "8", // low quality preferred for previews (owner decision 2026-07-04)
		sheetTemplate,
	}
	cmd := exec.CommandContext(ctx, t.cfg.BinaryPath, args...)
	ring := newRingBuffer(2048)
	cmd.Stderr = ring
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg storyboard start failed: %s", err)
	}
	if t.cfg.Nice > 0 {
		if err := syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, t.cfg.Nice); err != nil && t.log != nil {
			t.log.Debugw("setpriority(storyboard ffmpeg) failed; continuing",
				"pid", cmd.Process.Pid, "nice", t.cfg.Nice, "error", err)
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg storyboard failed: %s\nstderr tail:\n%s", err, ring.String())
	}
	sheets, err := filepath.Glob(filepath.Join(tmp, "storyboard_*.jpg"))
	if err != nil || len(sheets) == 0 {
		return nil, fmt.Errorf("storyboard produced no sheets (glob err: %v)", err)
	}
	sort.Strings(sheets)
	vttPath := filepath.Join(tmp, StoryboardVTTName)
	if err := os.WriteFile(vttPath, []byte(BuildStoryboardVTT(durationSec)), 0o644); err != nil {
		return nil, fmt.Errorf("write storyboard vtt: %w", err)
	}
	return &StoryboardResult{SheetPaths: sheets, VTTPath: vttPath}, nil
}

// BuildStoryboardVTT emits the WebVTT thumbnail track: one cue per cadence
// bucket, payload "storyboard_NNN.jpg#xywh=x,y,w,h" (relative sheet names —
// the HLS proxy rewrites them to signed proxy URLs like m3u8 children).
func BuildStoryboardVTT(durationSec int) string {
	if durationSec <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("WEBVTT\n")
	perSheet := StoryboardCols * StoryboardRows
	n := (durationSec + StoryboardCadenceSec - 1) / StoryboardCadenceSec
	for i := 0; i < n; i++ {
		start := i * StoryboardCadenceSec
		end := (i + 1) * StoryboardCadenceSec
		if end > durationSec {
			end = durationSec
		}
		sheet := i/perSheet + 1
		cell := i % perSheet
		x := (cell % StoryboardCols) * StoryboardTileW
		y := (cell / StoryboardCols) * StoryboardTileH
		fmt.Fprintf(&b, "\n%s --> %s\nstoryboard_%03d.jpg#xywh=%d,%d,%d,%d\n",
			vttTimestamp(start), vttTimestamp(end), sheet, x, y, StoryboardTileW, StoryboardTileH)
	}
	return b.String()
}

func vttTimestamp(s int) string {
	return fmt.Sprintf("%02d:%02d:%02d.000", s/3600, (s%3600)/60, s%60)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/ae-scrub-preview/services/library && go test ./internal/ffmpeg/ -count=1 -v`
Expected: ALL PASS (including the pre-existing transcoder tests — regression check).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add services/library/internal/ffmpeg/storyboard.go services/library/internal/ffmpeg/storyboard_test.go
git commit -m "feat(library): ffmpeg storyboard pass + WebVTT thumbnail-track builder"
```

---

### Task 2: Episode column, MinIO upload, encoder-worker integration

**Files:**
- Modify: `services/library/internal/domain/episode.go` (Episode struct, ~line 48-62)
- Create: `services/library/migrations/015_storyboard.sql` (verify `015` is next-free with `ls` — `014_library_jobs_transcoding.sql` is the current last) + register it: add a `go:embed` var in `services/library/migrations/migrations.go` and append it to the ordered apply list in `services/library/cmd/library-api/main.go`, exactly mirroring how `014` is registered in both places
- Modify: `services/library/internal/minio/writer.go`
- Modify: `services/library/internal/service/encoder_worker.go` (~lines 300-370)
- Test: extend `services/library/internal/service/encoder_worker_test.go`

**Interfaces:**
- Consumes: `Transcoder.Storyboard` + `StoryboardResult` + `StoryboardVTTName` (Task 1); existing writer upload helpers and worker locals (`job`, `result *ffmpeg.Result`, the resolved MinIO `prefix`, `episodeRepo.Create`).
- Produces: `Episode.HasStoryboard bool` (json `has_storyboard`); `func (w *Writer) UploadStoryboard(ctx context.Context, prefix string, sheetPaths []string, vttPath string) error` (adapt receiver name to the actual type in `writer.go`). Storyboard objects live at `<prefix>storyboard_NNN.jpg` + `<prefix>storyboard.vtt`.

- [ ] **Step 1: Write the failing worker test.** The worker consumes narrow interfaces declared in `encoder_worker.go`: `Transcoder` (`:42`, currently only `Transcode`), `Uploader` (`:48`), `EpisodeStore` (`:36`). The test file's handwritten fakes are `stubTranscoder` (`encoder_worker_test.go:113`), `stubUploader` (`:128`), `stubEpisodeStore` (`:96`); the reference happy-path test is `TestEncoder_HappyPath_WithShikimoriID` (`:334`). Extend the stubs:

```go
// on stubTranscoder — record + configurable failure:
	storyboardErr   error
	storyboardCalls []string // sourcePath per call

func (s *stubTranscoder) Storyboard(ctx context.Context, sourcePath string, durationSec int) (*ffmpeg.StoryboardResult, error) {
	s.storyboardCalls = append(s.storyboardCalls, sourcePath)
	if s.storyboardErr != nil {
		return nil, s.storyboardErr
	}
	dir := os.TempDir() // tests only need paths that exist for RemoveAll
	return &ffmpeg.StoryboardResult{
		SheetPaths: []string{filepath.Join(dir, "storyboard_001.jpg")},
		VTTPath:    filepath.Join(dir, "storyboard.vtt"),
	}, nil
}

// on stubUploader:
	uploadStoryboardErr    error
	uploadStoryboardPrefix string

func (s *stubUploader) UploadStoryboard(ctx context.Context, prefix string, sheetPaths []string, vttPath string) error {
	s.uploadStoryboardPrefix = prefix
	return s.uploadStoryboardErr
}
```

Then two tests, each cloned from `TestEncoder_HappyPath_WithShikimoriID` (same stubs/wiring; only the storyboard knobs differ):

```go
func TestEncoder_StoryboardFailureDoesNotFailJob(t *testing.T) {
	// stubTranscoder{storyboardErr: errors.New("boom")} — everything else as in the happy path.
	// Assert: job reaches the same terminal success status as the happy-path test,
	// and the created episode has HasStoryboard == false.
}

func TestEncoder_StoryboardSuccessSetsFlagAndUploads(t *testing.T) {
	// all stubs succeed. Assert: uploader.uploadStoryboardPrefix equals the HLS
	// upload's prefix (compare with the recorded uploadCall), and the episode
	// passed to stubEpisodeStore has HasStoryboard == true.
}
```

(The two test bodies are the happy-path test verbatim plus those knob changes and assertions — copy it, don't re-derive the wiring.)

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/ae-scrub-preview/services/library && go test ./internal/service/ -run Storyboard -v`
Expected: FAIL — fakes lack the new methods / `HasStoryboard` undefined.

- [ ] **Step 3: Implement domain field + migration**

`domain/episode.go` — add to `Episode` (match the struct's existing tag style):

```go
	// HasStoryboard marks that storyboard_NNN.jpg + storyboard.vtt exist
	// under MinioPath (scrub-preview sprite track).
	HasStoryboard bool `gorm:"not null;default:false" json:"has_storyboard"`
```

Migration file (`services/library/migrations/015_storyboard.sql`):

```sql
ALTER TABLE library_episodes
    ADD COLUMN IF NOT EXISTS has_storyboard BOOLEAN NOT NULL DEFAULT FALSE;
```

Register it in BOTH places (grep `014` to find them): the `go:embed` var in `migrations/migrations.go` and the ordered apply list in `cmd/library-api/main.go`.

- [ ] **Step 4: Implement `UploadStoryboard`** in `minio/writer.go`, mirroring the existing segment-upload helper (same client, bucket, content-type handling):

The existing single-object primitive is `putFile(ctx, prefix, path)` (`writer.go:256`), which derives Content-Type via `contentTypeFor` (`writer.go:187`) — currently mapping only `.m3u8`/`.ts`. First extend `contentTypeFor` with `.jpg → image/jpeg` and `.vtt → text/vtt` (add a small table-driven test next to any existing `contentTypeFor` coverage), then:

```go
// UploadStoryboard puts the sprite sheets + VTT under the episode prefix.
// Order is irrelevant (nothing references the VTT until has_storyboard flips
// true in Postgres).
func (w *Writer) UploadStoryboard(ctx context.Context, prefix string, sheetPaths []string, vttPath string) error {
	for _, p := range sheetPaths {
		if err := w.putFile(ctx, prefix, p); err != nil {
			return fmt.Errorf("upload storyboard sheet %s: %w", filepath.Base(p), err)
		}
	}
	if err := w.putFile(ctx, prefix, vttPath); err != nil {
		return fmt.Errorf("upload storyboard vtt: %w", err)
	}
	return nil
}
```

(Adapt receiver/method names to `writer.go`'s actual types; `putFile` keys the object as `prefix + filepath.Base(path)` — verify at `:256` and adjust if its contract differs.)

- [ ] **Step 5: Wire into `encoder_worker.go`** — immediately after the HLS MinIO upload succeeds and BEFORE `episodeRepo.Create` (~line 329). Best-effort, never fails the job:

```go
	// Storyboard pass (scrub-preview sprites) — strictly best-effort: any
	// failure is logged and the job proceeds without a storyboard. Skipped for
	// pending/ jobs (no ShikimoriID → no episode row exists to flag).
	hasStoryboard := false
	if job.ShikimoriID == "" {
		// nothing — pending/ uploads have no episode row; sprites would be orphans
	} else if sb, sbErr := w.transcoder.Storyboard(ctx, sourcePath, result.DurationSec); sbErr != nil {
		w.log.Warnw("storyboard generation failed; episode ships without preview sprites",
			"job_id", job.ID, "error", sbErr)
	} else {
		if upErr := w.minio.UploadStoryboard(ctx, prefix, sb.SheetPaths, sb.VTTPath); upErr != nil {
			w.log.Warnw("storyboard upload failed", "job_id", job.ID, "error", upErr)
		} else {
			hasStoryboard = true
		}
		_ = os.RemoveAll(filepath.Dir(sb.VTTPath))
	}
```

Then set `HasStoryboard: hasStoryboard` in the `domain.Episode` literal passed to the episode store (~line 329-364). Extend the worker's interfaces in `encoder_worker.go`: add `Storyboard(ctx context.Context, sourcePath string, durationSec int) (*ffmpeg.StoryboardResult, error)` to the `Transcoder` interface (`:42`) and `UploadStoryboard(ctx context.Context, prefix string, sheetPaths []string, vttPath string) error` to the `Uploader` interface (`:48`). Adapt local identifiers (the worker's receiver is `p`, fields `transcoder`, etc. — read the surrounding function first).

- [ ] **Step 6: Run tests**

Run: `cd /data/ae-scrub-preview/services/library && go test ./... -count=1`
Expected: ALL PASS.

- [ ] **Step 7: Commit**

```bash
cd /data/ae-scrub-preview
git add services/library/internal/domain/episode.go services/library/migrations services/library/internal/minio/writer.go services/library/internal/service/encoder_worker.go services/library/internal/service/encoder_worker_test.go
git commit -m "feat(library): generate + upload scrub-preview storyboards at ingest (best-effort)"
```

---

### Task 3: `storyboard_url` in the library episodes API

**Files:**
- Modify: `services/library/internal/handler/episodes.go` (URL build sites ~lines 93, 172)
- Test: the handler's existing test file (same dir)

**Interfaces:**
- Consumes: `Episode.HasStoryboard` (Task 2); the existing public-URL builder used for `minio_url` (`URLFor`, `minio/writer.go:416`).
- Produces: episode JSON gains `"storyboard_url,omitempty"` — absolute upstream URL `<prefix>storyboard.vtt`, only when `HasStoryboard`. Catalog consumes it in Task 5.

- [ ] **Step 1: Failing test** — in the episodes handler test, add: an episode row with `HasStoryboard: true` yields a response where `storyboard_url` = the same base as `minio_url` with `playlist.m3u8` → `storyboard.vtt`; with `HasStoryboard: false` the key is absent.
- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/services/library && go test ./internal/handler/ -v` — expect FAIL.
- [ ] **Step 3: Implement** — there are TWO response structs to extend: `episodeListItem` (`episodes.go:51`) and `episodeResponse` (`episodes.go:62`); add `StoryboardURL string \`json:"storyboard_url,omitempty"\`` to both. The URL builder is the handler's `urlBuilder` field (NOT `w` — that's the `http.ResponseWriter` in these funcs), used at `:93` and `:172`; at each site add:

```go
	if ep.HasStoryboard {
		item.StoryboardURL = h.urlBuilder.URLFor(ep.MinioPath + ffmpeg.StoryboardVTTName)
	}
```

(Adapt the receiver/local names to each site — mirror exactly how `MinIOURL` is populated one line above.)
- [ ] **Step 4: Run** the same test — expect PASS; then `go test ./... -count=1` for the service.
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add services/library/internal/handler
git commit -m "feat(library): expose storyboard_url on episode responses"
```

---

### Task 4: HLS-proxy VTT rewriting (sheet URLs get signed like m3u8 children)

**Files:**
- Modify: `libs/videoutils/proxy.go`
- Test: the existing proxy/rewrite test file in `libs/videoutils/`

**Why:** sprite sheets live on the private MinIO host (and the VTT references them by bare relative name). The FE cannot mint HMAC provenance. The proxy already rewrites m3u8 children into `/api/streaming/hls-proxy?url=...&exp=...&sig=...` (`rewriteM3U8URLs`, `proxy.go:863`; core in `rewriteHLSURL`, `:932-980`). Do the same for `.vtt` cue payloads so each sheet request arrives pre-signed.

**Interfaces:**
- Consumes: `rewriteHLSURL(...)` (reuse — do NOT duplicate the resolve+sign logic).
- Produces: `func rewriteVTTURLs(content, manifestURL, referer, sess string) string`; the proxy handler rewrites response bodies when the upstream `url=` path ends in `.vtt` (same trigger style as the m3u8 branch — find the `rewriteM3U8URLs(` call site and mirror its conditions).

- [ ] **Step 1: Failing test** (mirror the existing m3u8 rewrite tests' setup — signer configured so `exp`/`sig` are emitted):

```go
func TestRewriteVTTURLs_StoryboardCues(t *testing.T) {
	in := "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90\n\n00:00:05.000 --> 00:00:10.000\nstoryboard_001.jpg#xywh=160,0,160,90\n"
	out := rewriteVTTURLs(in, "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard.vtt", "", "")
	if !strings.Contains(out, "/api/streaming/hls-proxy?url="+url.QueryEscape("http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard_001.jpg")) {
		t.Fatalf("cue URL not proxied:\n%s", out)
	}
	if !strings.Contains(out, "#xywh=160,0,160,90") {
		t.Fatalf("xywh fragment must be preserved:\n%s", out)
	}
	if !strings.Contains(out, "&exp=") || !strings.Contains(out, "&sig=") {
		t.Fatalf("sheet URLs must carry provenance:\n%s", out)
	}
	if !strings.Contains(out, "00:00:00.000 --> 00:00:05.000") {
		t.Fatalf("timing lines must be untouched:\n%s", out)
	}
}

func TestRewriteVTTURLs_NonImagePayloadUntouched(t *testing.T) {
	in := "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\nSome subtitle text line\n"
	if out := rewriteVTTURLs(in, "http://x/s.vtt", "", ""); out != in {
		t.Fatalf("subtitle-style payload must pass through unchanged:\n%s", out)
	}
}
```

- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/libs/videoutils && go test ./... -run VTT -v` — expect FAIL (undefined `rewriteVTTURLs`).
- [ ] **Step 3: Implement**

```go
// vttImageCue matches storyboard-style cue payloads: an image path with an
// optional #xywh fragment. Anything else (real subtitles) passes through.
var vttImageCue = regexp.MustCompile(`^[^\s#]+\.(?:jpe?g|png|webp)(?:#.*)?$`)

// rewriteVTTURLs rewrites image cue payloads in a WebVTT thumbnail track to
// proxied+signed URLs, preserving #xywh fragments. Timing lines, headers,
// and non-image payloads are untouched.
func rewriteVTTURLs(content, manifestURL, referer, sess string) string {
	basePath := manifestDirBase(manifestURL) // see below — same derivation as rewriteM3U8URLs
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "WEBVTT") ||
			strings.HasPrefix(trimmed, "NOTE") || strings.Contains(trimmed, "-->") {
			continue
		}
		if !vttImageCue.MatchString(trimmed) {
			continue
		}
		urlPart, frag, hasFrag := strings.Cut(trimmed, "#")
		rewritten := rewriteHLSURL(urlPart, basePath, referer, sess)
		if hasFrag {
			rewritten += "#" + frag
		}
		lines[i] = rewritten
	}
	return strings.Join(lines, "\n")
}
```

**CRITICAL detail:** `rewriteHLSURL`'s second argument is a **directory base** (`scheme://host/dir/`), NOT the manifest URL — relative URLs resolve by plain concatenation `basePath + urlStr` (`proxy.go:953-956`). Passing the full VTT URL would produce `.../storyboard.vttstoryboard_001.jpg`. `rewriteM3U8URLs` derives the base at `proxy.go:876-883` — extract that derivation into a small shared helper (`manifestDirBase(manifestURL string) string`) and use it from BOTH rewriters (behavior-preserving refactor for the m3u8 side; its existing tests are the guard). Match `rewriteHLSURL`'s exact signature at `proxy.go:932` when calling. Then find the branch where the handler calls `rewriteM3U8URLs(...)` and add the analogous `.vtt` branch keyed on the upstream URL path suffix:

```go
	case strings.HasSuffix(strings.ToLower(upstreamPath), ".vtt"):
		body = rewriteVTTURLs(body, upstreamURL, referer, sess)
```

(Adapt to the surrounding code's actual variable names and control flow — if it's an `if`, mirror the `if`.)
- [ ] **Step 4: Run** `cd /data/ae-scrub-preview/libs/videoutils && go test ./... -count=1` — expect ALL PASS (m3u8 tests are the regression guard).
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add libs/videoutils
git commit -m "feat(videoutils): rewrite+sign VTT storyboard cue URLs in the HLS proxy"
```

---

### Task 5: Catalog contract — `RawStream.Storyboard`

**Files:**
- Modify: `services/catalog/internal/service/raw_resolver.go` (RawStream `:116-125`, `newLibraryStream` `:145`, `GetLibraryStream` `:203`)
- Modify: catalog's library HTTP client response struct (grep `MinIOURL` under `services/catalog/` — the struct decoding library's episode response gains `StoryboardURL string \`json:"storyboard_url"\``)
- Test: `raw_resolver`'s existing test file

**Interfaces:**
- Consumes: library's `storyboard_url` (Task 3); `streamsign.Sign` (already used for the playlist URL in `newLibraryStream`).
- Produces (FE consumes in Task 7):

```go
// RawStoryboard points at the episode's WebVTT thumbnail track (signed for
// the HLS proxy, same trust path as the playlist URL).
type RawStoryboard struct {
	URL string `json:"url"`
	Exp string `json:"exp,omitempty"`
	Sig string `json:"sig,omitempty"`
}
```
plus `Storyboard *RawStoryboard \`json:"storyboard,omitempty"\`` on `RawStream`.

- [ ] **Step 1: Failing test** — extend the resolver tests: when the (fake) library client returns `StoryboardURL: "http://minio:9000/raw-library/aeProvider/1/RAW/1/storyboard.vtt"`, `GetLibraryStream` returns a stream whose `Storyboard.URL` equals it and whose `Exp`/`Sig` are non-empty (when the signer is configured in the test the same way the existing playlist exp/sig test does it); when `StoryboardURL` is empty, `Storyboard == nil`.
- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/services/catalog && go test ./internal/service/ -run Library -v` — expect FAIL.
- [ ] **Step 3: Implement** — add the struct + field; extend `newLibraryStream` (and its call site) with the storyboard URL:

```go
	if storyboardURL != "" {
		exp, sig := streamsign.Sign(storyboardURL)
		s.Storyboard = &RawStoryboard{URL: storyboardURL, Exp: exp, Sig: sig}
	}
```

(Match how the existing playlist `Exp`/`Sig` are minted in `newLibraryStream` — same helper, same error/empty handling. Thread `StoryboardURL` from the library client response through `GetLibraryStream`.)
- [ ] **Step 4: Run** `cd /data/ae-scrub-preview/services/catalog && go test ./internal/service/... -count=1` — expect PASS.
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add services/catalog/internal/service services/catalog/internal
git commit -m "feat(catalog): storyboard track on the ae/library stream contract"
```

---

### Task 6: FE storyboard VTT parser

**Files:**
- Create: `frontend/web/src/components/player/aePlayer/storyboardVtt.ts`
- Test: `frontend/web/src/components/player/aePlayer/storyboardVtt.spec.ts`

**Interfaces:**
- Produces:

```ts
export interface StoryboardCue {
  start: number // seconds
  end: number
  url: string // absolute sheet URL (already proxied+signed by the backend)
  x: number
  y: number
  w: number
  h: number
}
export function parseStoryboardVtt(text: string, baseUrl: string): StoryboardCue[]
export function cueAt(cues: StoryboardCue[], t: number): StoryboardCue | null
```

- [ ] **Step 1: Failing spec** (≥5 assertions; bun/vitest):

```ts
import { describe, it, expect } from 'vitest'
import { parseStoryboardVtt, cueAt } from './storyboardVtt'

const BASE = 'https://animeenigma.org/api/streaming/hls-proxy?url=x%2Fstoryboard.vtt'

const SAMPLE = `WEBVTT

00:00:00.000 --> 00:00:05.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=0,0,160,90

00:00:05.000 --> 00:00:10.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=160,0,160,90

01:00:00.000 --> 01:00:05.000
storyboard_002.jpg#xywh=320,90,160,90
`

describe('parseStoryboardVtt', () => {
  it('parses proxied cues with times, url, and xywh', () => {
    const cues = parseStoryboardVtt(SAMPLE, BASE)
    expect(cues).toHaveLength(2)
    expect(cues[0]).toMatchObject({ start: 0, end: 5, x: 0, y: 0, w: 160, h: 90 })
    expect(cues[1].x).toBe(160)
    expect(cues[0].url).toBe('https://animeenigma.org/api/streaming/hls-proxy?url=a&exp=1&sig=b')
  })
  it('SKIPS bare-relative payloads — the backend proxy rewriting cues to signed absolute URLs is the contract; a client-resolved relative sheet URL could never carry url=/sig= and would always 404', () => {
    const cues = parseStoryboardVtt(SAMPLE, BASE)
    expect(cues.every((c) => c.start < 3600)).toBe(true)
  })
  it('skips malformed cues instead of throwing', () => {
    expect(parseStoryboardVtt('WEBVTT\n\ngarbage\nnot-a-timing\n', BASE)).toEqual([])
    expect(parseStoryboardVtt('', BASE)).toEqual([])
  })
})

describe('cueAt', () => {
  const cues = parseStoryboardVtt(SAMPLE, BASE)
  it('finds the covering cue and handles boundaries [start, end)', () => {
    expect(cueAt(cues, 0)?.x).toBe(0)
    expect(cueAt(cues, 4.9)?.x).toBe(0)
    expect(cueAt(cues, 5)?.x).toBe(160)
  })
  it('returns null outside all cues and clamps nothing', () => {
    expect(cueAt(cues, 20)).toBeNull()
    expect(cueAt([], 1)).toBeNull()
  })
})
```

- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/frontend/web && bun install && bunx vitest run src/components/player/aePlayer/storyboardVtt.spec.ts` — expect FAIL (module missing). (`bun install` because a fresh worktree has no `node_modules`.)
- [ ] **Step 3: Implement**

```ts
export interface StoryboardCue {
  start: number
  end: number
  url: string
  x: number
  y: number
  w: number
  h: number
}

const TIMING = /^(\d{2,}):(\d{2}):(\d{2})\.(\d{3})\s+-->\s+(\d{2,}):(\d{2}):(\d{2})\.(\d{3})/
const XYWH = /#xywh=(\d+),(\d+),(\d+),(\d+)\s*$/

function toSec(h: string, m: string, s: string, ms: string): number {
  return Number(h) * 3600 + Number(m) * 60 + Number(s) + Number(ms) / 1000
}

/** Parse a WebVTT thumbnail track (url#xywh cue payloads). Malformed cues are
 *  skipped — a broken storyboard degrades to "no preview", never to a throw. */
export function parseStoryboardVtt(text: string, baseUrl: string): StoryboardCue[] {
  const cues: StoryboardCue[] = []
  const lines = text.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const t = TIMING.exec(lines[i])
    if (!t) continue
    const payload = (lines[i + 1] ?? '').trim()
    const g = XYWH.exec(payload)
    if (!g) continue
    const raw = payload.slice(0, payload.indexOf('#'))
    // Only absolute or root-relative URLs are valid — the proxy rewrites every
    // cue to a signed /api/streaming/hls-proxy URL. A bare-relative name means
    // the rewrite didn't happen; resolving it client-side could never produce
    // a fetchable (signed) sheet URL, so skip the cue.
    if (!/^(?:https?:)?\/\//.test(raw) && !raw.startsWith('/')) continue
    let url: string
    try {
      url = new URL(raw, baseUrl).href
    } catch {
      continue
    }
    cues.push({
      start: toSec(t[1], t[2], t[3], t[4]),
      end: toSec(t[5], t[6], t[7], t[8]),
      url,
      x: Number(g[1]),
      y: Number(g[2]),
      w: Number(g[3]),
      h: Number(g[4]),
    })
  }
  return cues
}

/** Binary search for the cue covering t (cues are emitted sorted). */
export function cueAt(cues: StoryboardCue[], t: number): StoryboardCue | null {
  let lo = 0
  let hi = cues.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const c = cues[mid]
    if (t < c.start) hi = mid - 1
    else if (t >= c.end) lo = mid + 1
    else return c
  }
  return null
}
```

- [ ] **Step 4: Run** the spec — expect PASS. Also `bunx tsc --noEmit` (scoped type check happens in Task 7's verify; here vitest is enough).
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add frontend/web/src/components/player/aePlayer/storyboardVtt.ts frontend/web/src/components/player/aePlayer/storyboardVtt.spec.ts
git commit -m "feat(web): storyboard VTT parser for scrub previews"
```

---

### Task 7: FE prop chain + ScrubPreview storyboard mode

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (`LibraryStream` `:103-110`, `makeAeAdapter().resolveStream` `:336-348`, `buildProxyUrl` `:431-449`)
- Modify: `frontend/web/src/types/aePlayer.ts` (`StreamResult` `:55-66`)
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (`:220-221`)
- Modify: `frontend/web/src/components/player/aePlayer/PlayerScrubBar.vue` (props `:111-112`, forward `:72-76`)
- Modify: `frontend/web/src/components/player/aePlayer/ScrubPreview.vue`
- Test: extend `frontend/web/src/components/player/aePlayer/ScrubPreview.spec.ts`

**Interfaces:**
- Consumes: `RawStream.Storyboard {url,exp,sig}` (Task 5); `parseStoryboardVtt`/`cueAt` (Task 6); existing `buildProxyUrl(url, referer, streamType?, sign?)`.
- Produces: `StreamResult.storyboardUrl?: string` (fully-built proxy URL); ScrubPreview prop `storyboardUrl?: string | null`.

- [ ] **Step 1: Failing spec.** Read `ScrubPreview.spec.ts` first (it already mocks hls.js). Add a `storyboard mode` describe block — mock `global.fetch` to return the SAMPLE VTT from Task 6's spec, mock `Image` with an immediately-firing `onload`, then assert: (1) mounting with `storyboardUrl` set fetches the VTT exactly once; (2) hover renders (component's `hasFrame` becomes true / canvas branch visible) **without** the hls.js mock ever being constructed; (3) when the VTT fetch rejects, the component falls back to the shadow-engine path (hls.js mock IS constructed on hover); (4) `storyboardUrl=null` behaves exactly as before (existing tests stay green).
- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/frontend/web && bunx vitest run src/components/player/aePlayer/ScrubPreview.spec.ts` — expect FAIL (no such prop).
- [ ] **Step 3: Implement the chain, top-down.**

`useProviderResolver.ts` — extend the interface and adapter:

```ts
interface LibraryStream {
  url: string
  type: 'hls' | 'mp4'
  quality?: string
  exp?: string
  sig?: string
  storyboard?: { url: string; exp?: string; sig?: string }
}
```

In `makeAeAdapter().resolveStream`, alongside the existing return:

```ts
      storyboardUrl: stream.storyboard
        ? buildProxyUrl(stream.storyboard.url, '', undefined, {
            exp: stream.storyboard.exp,
            sig: stream.storyboard.sig,
          })
        : undefined,
```

(Match `buildProxyUrl`'s actual signature at `:431` — the `sign` argument shape is whatever the playlist URL already uses there.)

`types/aePlayer.ts` — add to `StreamResult`:

```ts
  /** Proxied WebVTT thumbnail-track URL (library content only). When set, the
   *  scrub preview uses sprite sheets and never starts a shadow engine. */
  storyboardUrl?: string
```

`AePlayer.vue:220-221` — third preview prop:

```html
      :preview-url="currentStream?.url ?? null"
      :preview-type="currentStream?.type ?? null"
      :preview-storyboard-url="currentStream?.storyboardUrl ?? null"
```

`PlayerScrubBar.vue` — prop `previewStoryboardUrl?: string | null` next to `:111-112`, forwarded at `:72-76` as `:storyboard-url="previewStoryboardUrl ?? null"`.

`ScrubPreview.vue` — storyboard mode (sprite crop, zero shadow engine):

```ts
const props = defineProps<{
  timeSec: number
  visible: boolean
  streamUrl: string | null
  streamType: 'hls' | 'mp4' | null
  stillUrl?: string
  /** WebVTT thumbnail track — when set and loadable, sprite mode replaces the shadow engine */
  storyboardUrl?: string | null
}>()
```

```ts
import { parseStoryboardVtt, cueAt, type StoryboardCue } from './storyboardVtt'

let storyCues: StoryboardCue[] | null = null
/** true from mount-with-storyboardUrl until the VTT fetch settles — hovers during
 *  the load must NOT boot the shadow engine (it would race the sprite mode). */
let storyPending = false
const sheetImgs = new Map<string, HTMLImageElement>()

function storyboardActive(): boolean {
  return storyCues !== null && storyCues.length > 0
}

async function loadStoryboard(u: string) {
  storyPending = true
  try {
    const r = await fetch(u)
    if (!r.ok) throw new Error(`storyboard vtt http=${r.status}`)
    const cues = parseStoryboardVtt(await r.text(), u)
    storyCues = cues.length > 0 ? cues : null
  } catch (e) {
    storyCues = null // broken storyboard → shadow-engine fallback, never an error
    slog(`storyboard load failed, falling back to shadow engine: ${String(e)}`)
  } finally {
    storyPending = false
    if (storyboardActive() && props.visible) renderStoryboard(props.timeSec)
  }
}

function renderStoryboard(t: number) {
  const cue = cueAt(storyCues!, t)
  if (!cue) {
    hasFrame.value = false
    return
  }
  let img = sheetImgs.get(cue.url)
  if (!img) {
    img = new Image()
    img.src = cue.url
    img.onload = () => {
      if (props.visible && storyboardActive()) renderStoryboard(props.timeSec)
    }
    img.onerror = () => {
      // Evict so the sheet is retried on a later hover instead of pinning a
      // permanently-broken entry (e.g. evicted MinIO object) as "loading".
      sheetImgs.delete(cue.url)
      hasFrame.value = false
    }
    sheetImgs.set(cue.url, img)
  }
  if (!img.complete || img.naturalWidth === 0) return // draw on onload re-entry
  const ctx = canvasRef.value?.getContext('2d')
  if (ctx) ctx.drawImage(img, cue.x, cue.y, cue.w, cue.h, 0, 0, THUMB_W, THUMB_H)
  hasFrame.value = true
}
```

Gate the two existing watchers:
- In the `[visible, timeSec]` watcher: `if (storyPending) return` and `if (storyboardActive()) { renderStoryboard(t); return }` before `void ensureEngine()`.
- In the `streamUrl` watcher (and its eager-init timer): reset storyboard state (`storyCues = null; storyPending = false; sheetImgs.clear()`), then `if (props.storyboardUrl) { void loadStoryboard(props.storyboardUrl); return }` — the shadow engine is skipped while the load is pending (`storyPending` gate above); if the load fails, `storyCues === null && !storyPending` lets the next hover fall through to `ensureEngine()` naturally. Also watch `() => props.storyboardUrl` with the same reset+load logic (it can arrive after `streamUrl`).
- `destroyEngine()` untouched (storyboard mode never created an engine).
- [ ] **Step 4: Run** `cd /data/ae-scrub-preview/frontend/web && bunx vitest run src/components/player/aePlayer/ && bunx vue-tsc --noEmit` — expect ALL PASS (vue-tsc, not bare tsc — `.vue` types).
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add frontend/web/src/composables/aePlayer/useProviderResolver.ts frontend/web/src/types/aePlayer.ts frontend/web/src/components/player/aePlayer
git commit -m "feat(web): sprite-storyboard scrub previews for library content"
```

---

### Task 8: Storyboard backfill worker (already-ingested episodes)

**Files:**
- Create: `services/library/internal/service/storyboard_backfill.go`
- Test: `services/library/internal/service/storyboard_backfill_test.go`
- Modify: `services/library/internal/config/config.go` (flag + defaults), `services/library/cmd/library-api/main.go` (start the worker), `services/library/internal/repo/` episode repo (two new methods), `services/library/internal/minio/writer.go` (prefix download helper)

**Interfaces:**
- Consumes: `Transcoder.Storyboard` (Task 1 — note: ffmpeg reads a **local** `playlist.m3u8` as its `-i` source; segments are referenced relatively so a downloaded prefix dir is a valid input; it self-probes duration when passed ≤ 0), `UploadStoryboard` (Task 2), the existing disk guard — real signature `Allow(minFreePct int) (allowed bool, freePct int, err error)` (`disk_guard.go:67`), episode repo.
- Produces: background loop; repo methods `ListWithoutStoryboard(ctx, limit int) ([]domain.Episode, error)` and `SetHasStoryboard(ctx, id string) error`; writer method `DownloadPrefix(ctx, prefix, destDir string) error` (downloads `playlist.m3u8` + `segment_*.ts` under prefix); config `StoryboardBackfillEnabled bool` (env `STORYBOARD_BACKFILL_ENABLED`, default `true`), `StoryboardBackfillPauseSec int` (env `STORYBOARD_BACKFILL_PAUSE_SEC`, default `60`).

**Behavior (lowest-priority by construction):** one episode at a time; between episodes sleep `PauseSec`; before each episode check the disk guard — disallowed OR guard error ⇒ sleep and retry later (drop-on-risk); any per-episode error → log warn, mark nothing, continue to the next (an episode that keeps failing is retried on the next full pass; the loop re-queries `ListWithoutStoryboard` each cycle and exits the pass when the returned page yields no successful work). Episode duration: pass `ep.DurationSec` (deref, 0 when nil) — `Storyboard` self-probes when ≤ 0 and errors if still unknown (that error is just the per-episode warn path).

- [ ] **Step 1: Failing tests** — with handwritten fakes (repo/writer/transcoder/guard):

```go
func TestBackfill_ProcessesEpisodeAndSetsFlag(t *testing.T) {
	// repo returns one episode (HasStoryboard=false, DurationSec=1400) then none;
	// assert order: DownloadPrefix(prefix, dir) → Storyboard(localPlaylist, 1400)
	// → UploadStoryboard(prefix,...) → SetHasStoryboard(id); temp dir removed.
}

func TestBackfill_DiskGuardDisallowedSkipsWork(t *testing.T) {
	// guard.Allow() == false → no repo/minio/transcoder calls this cycle.
}

func TestBackfill_EpisodeErrorContinues(t *testing.T) {
	// two episodes; first's Storyboard errors → SetHasStoryboard NOT called for it,
	// second still processed fully.
}
```

- [ ] **Step 2: Run** `go test ./internal/service/ -run Backfill -v` — expect FAIL.
- [ ] **Step 3: Implement** the worker (mirror the structure/logging of an existing library background worker — read `internal/autocache/` for the loop+config style first):

```go
// StoryboardBackfill fills storyboards for episodes ingested before the
// storyboard pass existed. Deliberately slow and yielding: this is one of the
// lowest-priority workloads on the host (owner directive 2026-07-04).
type StoryboardBackfill struct {
	repo   EpisodeRepo   // ListWithoutStoryboard / SetHasStoryboard
	minio  StoryboardStore // DownloadPrefix / UploadStoryboard
	trans  StoryboardMaker // Storyboard(ctx, sourcePath, durationSec)
	guard  DiskAllower     // Allow(minFreePct int) (bool, int, error) — matches disk_guard.go:67
	minFreePct int         // reuse the guard threshold the encoder path already uses
	log    *logger.Logger
	pause  time.Duration
	tmpdir string
}

func (b *StoryboardBackfill) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if ok, _, err := b.guard.Allow(b.minFreePct); err != nil || !ok {
			b.log.Debugw("storyboard backfill: disk guard disallows, waiting", "error", err)
			sleepCtx(ctx, b.pause) // drop on risk — guard error counts as "no"
			continue
		}
		eps, err := b.repo.ListWithoutStoryboard(ctx, 1)
		if err != nil || len(eps) == 0 {
			sleepCtx(ctx, 10*b.pause) // idle: nothing to do (or transient DB error)
			continue
		}
		b.processOne(ctx, eps[0]) // errors are logged inside, never abort the loop
		sleepCtx(ctx, b.pause)
	}
}
```

`processOne`: `os.MkdirTemp(b.tmpdir, "sb-backfill-")` → `defer os.RemoveAll` → `DownloadPrefix(ctx, ep.MinioPath, dir)` → duration := `ep.DurationSec` deref or 0 → `b.trans.Storyboard(ctx, filepath.Join(dir, "playlist.m3u8"), duration)` → `UploadStoryboard(ctx, ep.MinioPath, ...)` → `SetHasStoryboard(ctx, ep.ID)`; each step: on error `log.Warnw(...)` + return. `sleepCtx` = `select { case <-ctx.Done(): case <-time.After(d): }`. `ListWithoutStoryboard` must ORDER BY `created_at ASC` and — to avoid re-hammering a permanently-broken episode every cycle — skip episodes younger than 10 minutes (`created_at < now() - interval '10 minutes'`; ingest-time generation covers fresh ones). Repo methods via GORM on `library_episodes`. Wire in `main.go` behind `cfg.StoryboardBackfillEnabled` as a goroutine, same pattern as the autocache workers.
- [ ] **Step 4: Run** `cd /data/ae-scrub-preview/services/library && go test ./... -count=1` — ALL PASS.
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add services/library
git commit -m "feat(library): storyboard backfill worker for pre-existing episodes"
```

---

### Task 9: SW segment cache (`segmentCache.ts` + sw.ts route)

**Files:**
- Create: `frontend/web/src/pwa/segmentCache.ts`
- Test: `frontend/web/src/pwa/segmentCache.spec.ts`
- Modify: `frontend/web/src/sw.ts`

**Interfaces:**
- Consumes: proxy URL shape — flat `/api/streaming/hls-proxy` endpoint; segment identity = the `url=` query param (absolute upstream URL); `exp`/`sig`/`sess` vary per playlist-rewrite and MUST NOT enter the cache key; `type=mp4` marks progressive MP4 (Range traffic — excluded).
- Produces:

```ts
export const SEG_CACHE = 'ae-seg-v1'
export const SEG_MAX_ENTRIES = 150 // ~2MB/segment → ≤ ~300MB disk
export const SEG_TTL_MS = 3 * 60 * 60 * 1000
export const SCRUB_PARAM = 'aescrub'
export function segmentCacheKey(requestUrl: string): string | null
export function isScrubRequest(requestUrl: string): boolean
export function markScrubUrl(url: string): string
export async function handleSegmentRequest(request: Request, event: FetchEvent): Promise<Response>
```

Task 10 imports `markScrubUrl` in ScrubPreview and purges `ae-seg-*` in the kill-switch.

- [ ] **Step 1: Failing spec** (pure helpers under jsdom; the handler with stubbed `caches`/`fetch`/`navigator.storage` globals via `vi.stubGlobal`):

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  SEG_TTL_MS, segmentCacheKey, isScrubRequest, markScrubUrl, handleSegmentRequest,
} from './segmentCache'

const SEG = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/segment_001.ts') + '&exp=111&sig=aaa&sess=s1'
const SEG_RESIGNED = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/segment_001.ts') + '&exp=222&sig=bbb'
const PLAYLIST = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/playlist.m3u8') + '&exp=1&sig=a'
const MP4 = 'https://animeenigma.org/api/streaming/hls-proxy?url=' +
  encodeURIComponent('https://cdn.example/v/movie.mp4') + '&type=mp4'

describe('segmentCacheKey', () => {
  it('keys segments by upstream url only — resigned URL maps to the same key', () => {
    const k = segmentCacheKey(SEG)
    expect(k).toBeTruthy()
    expect(segmentCacheKey(SEG_RESIGNED)).toBe(k)
  })
  it('works on a dedicated stream origin (VITE_HLS_PROXY_BASE)', () => {
    expect(segmentCacheKey(SEG.replace('animeenigma.org', 'stream.animeenigma.org'))).toBe(segmentCacheKey(SEG))
  })
  it('rejects playlists, mp4-progressive, foreign paths, and garbage', () => {
    expect(segmentCacheKey(PLAYLIST)).toBeNull()
    expect(segmentCacheKey(MP4)).toBeNull()
    expect(segmentCacheKey('https://animeenigma.org/api/anime/x')).toBeNull()
    expect(segmentCacheKey('not a url')).toBeNull()
  })
})

describe('markScrubUrl / isScrubRequest', () => {
  it('appends the marker to hls-proxy urls only, idempotently', () => {
    const m = markScrubUrl(SEG)
    expect(isScrubRequest(m)).toBe(true)
    expect(isScrubRequest(SEG)).toBe(false)
    expect(markScrubUrl(m)).toBe(m)
    expect(markScrubUrl('https://x.example/other')).toBe('https://x.example/other')
  })
  it('marker does not change the cache key', () => {
    expect(segmentCacheKey(markScrubUrl(SEG))).toBe(segmentCacheKey(SEG))
  })
})

describe('handleSegmentRequest', () => {
  let store: Map<string, Response>
  const fakeCache = {
    match: vi.fn(async (k: string) => store.get(k)),
    put: vi.fn(async (k: string, r: Response) => void store.set(k, r)),
    delete: vi.fn(async (k: string) => store.delete(k)),
    keys: vi.fn(async () => [...store.keys()].map((k) => new Request(k))),
  }
  const waits: Promise<unknown>[] = []
  const event = { waitUntil: (p: Promise<unknown>) => waits.push(p) } as unknown as FetchEvent

  beforeEach(() => {
    store = new Map()
    waits.length = 0
    vi.stubGlobal('caches', { open: async () => fakeCache })
    vi.stubGlobal('navigator', { storage: { estimate: async () => ({ usage: 0, quota: 50_000_000_000 }) } })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })))
  })
  afterEach(() => vi.unstubAllGlobals())

  it('non-scrub: returns network response and tees the copy via waitUntil', async () => {
    const resp = await handleSegmentRequest(new Request(SEG), event)
    expect(resp.status).toBe(200)
    await Promise.all(waits)
    expect(fakeCache.put).toHaveBeenCalledTimes(1)
  })
  it('scrub hit: served from cache without fetch', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    ;(fetch as ReturnType<typeof vi.fn>).mockClear()
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG_RESIGNED)), event)
    expect(resp.status).toBe(200)
    expect(fetch).not.toHaveBeenCalled()
  })
  it('scrub miss: falls through to network', async () => {
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG)), event)
    expect(resp.status).toBe(200)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('expired entries are treated as misses', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    const key = segmentCacheKey(SEG)!
    const old = store.get(key)!
    const h = new Headers(old.headers)
    h.set('x-ae-cached-at', String(Date.now() - SEG_TTL_MS - 1))
    store.set(key, new Response(await old.arrayBuffer(), { headers: h }))
    const resp = await handleSegmentRequest(new Request(markScrubUrl(SEG)), event)
    expect(fetch).toHaveBeenCalled()
    expect(resp.status).toBe(200)
  })
  it('cache write failures never affect the returned response', async () => {
    fakeCache.put.mockRejectedValueOnce(new Error('quota'))
    const resp = await handleSegmentRequest(new Request(SEG), event)
    expect(resp.status).toBe(200)
    await Promise.all(waits) // must not reject
  })
  it('ranged requests bypass cache read AND tee', async () => {
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    fakeCache.put.mockClear()
    ;(fetch as ReturnType<typeof vi.fn>).mockClear()
    const ranged = new Request(markScrubUrl(SEG), { headers: { range: 'bytes=0-100' } })
    await handleSegmentRequest(ranged, event)
    expect(fetch).toHaveBeenCalledTimes(1) // no cache hit despite the entry existing
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
  })
  it('skips writes when storage headroom is low or estimate unavailable', async () => {
    vi.stubGlobal('navigator', { storage: { estimate: async () => ({ usage: 0, quota: 500_000_000 }) } })
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
    vi.stubGlobal('navigator', {})
    await handleSegmentRequest(new Request(SEG), event)
    await Promise.all(waits)
    expect(fakeCache.put).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run** `cd /data/ae-scrub-preview/frontend/web && bunx vitest run src/pwa/segmentCache.spec.ts` — expect FAIL.
- [ ] **Step 3: Implement**

```ts
// SW segment cache — passively tees /api/streaming/hls-proxy SEGMENT responses
// into Cache Storage so the scrub-preview shadow engine's re-fetches become
// local disk hits instead of duplicate provider egress.
//
// Reliability contract (owner directive 2026-07-04):
//  - the MAIN player is never served from this cache (pass-through + tee only);
//  - a cache write NEVER blocks or fails a response (waitUntil + swallow);
//  - writes are skipped outright when storage headroom is unknown or < 1GB.

export const SEG_CACHE = 'ae-seg-v1'
export const SEG_MAX_ENTRIES = 150
export const SEG_TTL_MS = 3 * 60 * 60 * 1000
export const SCRUB_PARAM = 'aescrub'
const PROXY_PATH = '/api/streaming/hls-proxy'
const MIN_HEADROOM_BYTES = 1_000_000_000
const CACHED_AT = 'x-ae-cached-at'
const SEG_EXT = /\.(ts|m4s)$/i

/** Cache identity of a proxied segment request: the upstream `url` param only.
 *  exp/sig/sess rotate per playlist-rewrite and must not fragment the cache.
 *  Returns null for anything that is not a cacheable HLS segment request. */
export function segmentCacheKey(requestUrl: string): string | null {
  try {
    const u = new URL(requestUrl)
    if (!u.pathname.endsWith(PROXY_PATH)) return null
    if (u.searchParams.get('type') === 'mp4') return null
    const upstream = u.searchParams.get('url')
    if (!upstream) return null
    if (!SEG_EXT.test(new URL(upstream).pathname)) return null
    return '/__segcache/?u=' + encodeURIComponent(upstream)
  } catch {
    return null
  }
}

export function isScrubRequest(requestUrl: string): boolean {
  try {
    return new URL(requestUrl).searchParams.get(SCRUB_PARAM) === '1'
  } catch {
    return false
  }
}

/** Tag an hls-proxy URL as scrub-preview traffic (cache-first in the SW). */
export function markScrubUrl(url: string): string {
  try {
    const u = new URL(url, self.location?.href ?? 'https://x.invalid')
    if (!u.pathname.endsWith(PROXY_PATH)) return url
    if (u.searchParams.get(SCRUB_PARAM) === '1') return url
    u.searchParams.set(SCRUB_PARAM, '1')
    return u.href
  } catch {
    return url
  }
}

async function readSegment(key: string): Promise<Response | null> {
  const cache = await caches.open(SEG_CACHE)
  const hit = await cache.match(key)
  if (!hit) return null
  const at = Number(hit.headers.get(CACHED_AT) ?? 0)
  if (!at || Date.now() - at > SEG_TTL_MS) {
    void cache.delete(key).catch(() => {})
    return null
  }
  return hit
}

async function writeSegment(key: string, resp: Response): Promise<void> {
  const est = await navigator.storage?.estimate?.().catch(() => undefined)
  if (!est || typeof est.quota !== 'number' || typeof est.usage !== 'number') return // unknown → drop on risk
  if (est.quota - est.usage < MIN_HEADROOM_BYTES) return
  const body = await resp.arrayBuffer()
  const headers = new Headers({
    'Content-Type': resp.headers.get('Content-Type') ?? 'video/mp2t',
    [CACHED_AT]: String(Date.now()),
  })
  const cache = await caches.open(SEG_CACHE)
  await cache.put(key, new Response(body, { status: 200, headers }))
  const keys = await cache.keys()
  // Cache API preserves insertion order → the front of keys() is the oldest.
  for (let i = 0; i < keys.length - SEG_MAX_ENTRIES; i++) {
    await cache.delete(keys[i])
  }
}

/** Concurrent tee ceiling — a seek storm must not pile up N×2MB arrayBuffers
 *  in SW memory (drop-on-risk: skip the tee, never the response). */
let inflightTees = 0
const MAX_INFLIGHT_TEES = 4

/** SW fetch handler for proxied segment requests. Scrub-marked → cache-first;
 *  everything else → transparent network with a background tee. Ranged
 *  requests (EXT-X-BYTERANGE streams share one URL across ranges) bypass the
 *  cache entirely — a cached full 200 would corrupt a ranged read. */
export async function handleSegmentRequest(request: Request, event: FetchEvent): Promise<Response> {
  const key = request.headers.has('range') ? null : segmentCacheKey(request.url)
  if (key && isScrubRequest(request.url)) {
    const hit = await readSegment(key).catch(() => null)
    if (hit) return hit
  }
  const resp = await fetch(request)
  if (key && resp.status === 200 && inflightTees < MAX_INFLIGHT_TEES) {
    inflightTees++
    const copy = resp.clone()
    event.waitUntil(
      writeSegment(key, copy)
        .catch(() => {})
        .finally(() => {
          inflightTees--
        }),
    )
  }
  return resp
}
```

`sw.ts` — register AFTER the offline route (order matters only for readability; matchers are disjoint):

```ts
import { segmentCacheKey, handleSegmentRequest } from './pwa/segmentCache'

// HLS segment tee/cache: scrub-preview traffic is served cache-first; the main
// player is passed through untouched (see segmentCache.ts reliability contract).
registerRoute(
  ({ url }) => segmentCacheKey(url.href) !== null,
  ({ request, event }) => handleSegmentRequest(request, event),
)
```

- [ ] **Step 4: Run** `bunx vitest run src/pwa/ && bunx vue-tsc --noEmit` — ALL PASS. Then a real build (`bun run build`) — the SW bundle must compile (`injectManifest` builds `src/sw.ts`).
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add frontend/web/src/pwa/segmentCache.ts frontend/web/src/pwa/segmentCache.spec.ts frontend/web/src/sw.ts
git commit -m "feat(web): SW segment cache — scrub previews reuse already-downloaded segments"
```

---

### Task 10: Scrub marker on the shadow engine + kill-switch purge

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/ScrubPreview.vue` (hls config, ~line 387-394)
- Modify: `frontend/web/src/pwa/registerPwa.ts` (`unregisterAll`, ~line 111-119)
- Test: extend `ScrubPreview.spec.ts`; extend/create the registerPwa spec if one exists (else assert via a small pure helper)

**Interfaces:**
- Consumes: `markScrubUrl` (Task 9).
- Produces: every shadow-engine HLS request carries `aescrub=1`; kill-switch deletes `ae-seg-*` caches (and continues to preserve `ae-offline-*`).

- [ ] **Step 1: Failing spec** — in `ScrubPreview.spec.ts`, assert the Hls constructor mock receives a config containing a function-valued `xhrSetup`, and that calling that `xhrSetup(fakeXhr, hlsProxySegmentUrl)` invokes `fakeXhr.open('GET', urlContaining('aescrub=1'), true)` while a non-proxy URL leaves the xhr untouched.
- [ ] **Step 2: Run** — expect FAIL.
- [ ] **Step 3: Implement.** In `ScrubPreview.vue`'s `new Hls({...})` config add:

```ts
    // Tag shadow-engine traffic so the SW serves it cache-first (segmentCache.ts).
    // hls.js skips its own open() when xhrSetup already opened the request.
    xhrSetup: (xhr: XMLHttpRequest, url: string) => {
      const marked = markScrubUrl(url)
      if (marked !== url) xhr.open('GET', marked, true)
    },
```

with `import { markScrubUrl } from '@/pwa/segmentCache'`. In `registerPwa.ts` `unregisterAll()` extend the purge filter:

```ts
    const keys = await caches.keys()
    await Promise.all(
      keys
        .filter((k) => k.startsWith('workbox-') || k.startsWith('ae-seg-'))
        .map((k) => caches.delete(k)),
    )
```

(The comment above it about preserving `ae-offline-*` user downloads stays true — extend it to note `ae-seg-*` is disposable.)
- [ ] **Step 4: Run** `bunx vitest run src/components/player/aePlayer/ScrubPreview.spec.ts src/pwa/ && bunx vue-tsc --noEmit` — ALL PASS.
- [ ] **Step 5: Commit**

```bash
cd /data/ae-scrub-preview
git add frontend/web/src/components/player/aePlayer/ScrubPreview.vue frontend/web/src/pwa/registerPwa.ts frontend/web/src/components/player/aePlayer/ScrubPreview.spec.ts
git commit -m "feat(web): scrub-marked shadow-engine requests + ae-seg kill-switch purge"
```

---

### Task 11: Canonical docs update

**Files:**
- Modify: `docs/aeplayer-reference.md`

- [ ] **Step 1: Update the ScrubPreview/preview section** of the canonical player reference (find the existing scrub-preview / hover-preview coverage) to document, code-verified against the merged implementation:
  - the **three preview sources** in priority order: (1) storyboard sprite track (library content; `StreamResult.storyboardUrl` → VTT + `#xywh` crops; no shadow engine), (2) shadow engine + SW segment cache (scrub-marked `aescrub=1` requests served cache-first from `ae-seg-v1`), (3) plain shadow engine (SW absent/killed — today's behavior);
  - the locked storyboard geometry (5 s cadence, 10×10 × 160×90 sheets, `storyboard.vtt`, MinIO prefix layout) and the signing chain (catalog signs the VTT; the proxy rewrites+signs sheet cues like m3u8 children);
  - the SW cache's reliability contract (main player never cache-served, waitUntil-only writes, 1 GB headroom drop-guard, tee-concurrency cap, 150-entry/3 h FIFO bounds, kill-switch purge);
  - the coverage caveats: ranged/extension-less segments bypassed, Safari = tee-only (native HLS path has no `xhrSetup`), and the privacy note (segments — incl. 18+ content — persist to disk ≤ 3 h; kill-switch purges).
- [ ] **Step 2: Verify claims against code** — every file/line/name cited in the new section must exist in the worktree (`grep` each).
- [ ] **Step 3: Commit**

```bash
cd /data/ae-scrub-preview
git add docs/aeplayer-reference.md
git commit -m "docs(player): storyboard + SW segment cache in the aeplayer reference"
```

---

## Verification (end-to-end, after all tasks)

1. `cd /data/ae-scrub-preview/services/library && go test ./... -count=1 -race`
2. `cd /data/ae-scrub-preview/services/catalog && go test ./internal/service/... -count=1`
3. `cd /data/ae-scrub-preview/libs/videoutils && go test ./... -count=1`
4. `cd /data/ae-scrub-preview/frontend/web && bunx vitest run src/components/player/aePlayer/ src/pwa/ && bunx vue-tsc --noEmit && bun run build`
5. Run `/frontend-verify` (DS-lint, i18n parity — expect zero new strings, real build).
6. Live smoke after deploy (needs `.env` copied into the worktree per the deploy-from-worktree rule, or deploy from base after push): open a **library** episode (ae source family) → hover the scrub bar → sprite previews appear instantly with no shadow-engine egress in DevTools Network; open a **scraped** episode (e.g. gogoanime) → hover previews still work; re-hover previously fetched spots → served from SW cache (DevTools: "(ServiceWorker)" in the Size column); verify main playback unaffected with SW enabled and with kill-switch on.
7. `/animeenigma-after-update` (redeploy library+catalog+streaming+web, changelog, commit, push).

## Rollout / risk notes

- **No feature flags:** storyboard mode activates only when `storyboard` appears in the stream payload (new episodes / backfilled ones); SW cache activates only where the PWA SW runs; both degrade to today's behavior otherwise. The kill-switch (`sw-config.json`) remains the SW-side emergency stop.
- **Known non-goals (v1):** MP4-progressive providers (Range) are excluded from the SW cache; ranged HLS segment requests (EXT-X-BYTERANGE) bypass it; extension-less path-style segment CDNs (e.g. okcdn — see the comment at `proxy.go:739-745`) are never keyed, so they silently keep today's behavior; Safari's native-HLS shadow path (`v.src`, no hls.js → no `xhrSetup`) never emits scrub-marked requests, so Safari gets tee-only (no cache-first previews); the shadow engine stays L0-pinned (owner: low quality preferred); no server-side sprite generation for scraped providers (egress).
- **Honest coverage statement:** for multi-level scraped HLS, cache hits cover the shadow engine's own repeats (re-hovers after FIFO eviction of the canvas cache, engine re-inits, episode re-opens within 3 h) — buffered-ahead main-level segments are NOT reused (deliberate: would require level-switching complexity for quality nobody needs in a 192×108 bubble). For single-variant streams (all library content pre-storyboard, some scraped) hits cover everything the main player already downloaded.
- **Privacy note (document in Task 11):** the SW cache persists video segments — including 18+ (hanime) content — to disk for up to 3 h / 150 entries where nothing persisted before; the kill-switch and the FIFO/TTL bounds are the mitigations.
- **Backfill pacing:** worst case one episode ≈ download prefix (LAN MinIO) + one nice'd ffmpeg decode; at 60 s pauses a 500-episode library completes in a few days — acceptable for the lowest-priority tier. `STORYBOARD_BACKFILL_ENABLED=false` is the off switch.
