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
	if t.cfg.Tmpdir != "" {
		if err := os.MkdirAll(t.cfg.Tmpdir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir tmpdir: %w", err)
		}
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
