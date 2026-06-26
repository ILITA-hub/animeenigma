package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
)

// Finalizer performs the two-pass concat+remux pipeline that turns a directory
// of upscaled video segments (produced by the upscaler worker) into a complete
// H.264 HLS package under outDir.
//
// Pass 1 — concat demuxer:
//
//	ffmpeg -f concat -safe 0 -i {concat.txt} -c:v copy {tmp}/video.mkv
//
// Segments are already encoded by the upscaling worker; concat is stream-copy
// so no quality loss occurs and the operation is fast.
//
// Pass 2 — remux + HLS transcode:
//
//	ffmpeg -i {tmp}/video.mkv [-i audio.mka] [-i subs.mks]
//	       -map 0:v [-map 1:a?] [-map 2:s?]
//	       -c:v libx264 -preset slow -crf 18 -pix_fmt {fmt}
//	       -c:a copy -c:s copy
//	       -fps_mode passthrough
//	       -hls_time 6 -hls_playlist_type vod
//	       -hls_segment_filename {out}/segment_%03d.ts
//	       {out}/playlist.m3u8
//
// pix_fmt is selected from the source probe: 10-bit sources (pix_fmt containing
// "10") → yuv420p10le; otherwise → yuv420p. This preserves HDR / 10-bit depth
// without downconverting.
//
// Audio and subtitle sidecars: if a sidecar path is empty (""), the corresponding
// -i/-map pair is omitted entirely from the remux command — ffmpeg does not
// receive an empty -i argument.
type Finalizer struct {
	bin    string // path to ffmpeg binary (injectable for tests)
	workFn func(out string) (string, func(), error)
}

// NewFinalizer returns a Finalizer that shells out to ffmpegBin.
// Pass "ffmpeg" to use the first binary on $PATH.
func NewFinalizer(ffmpegBin string) *Finalizer {
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	return &Finalizer{bin: ffmpegBin, workFn: defaultWorkDir}
}

// defaultWorkDir creates a temporary directory under the OS temp area.
// The cleanup function removes it. Production use.
func defaultWorkDir(_ string) (string, func(), error) {
	d, err := os.MkdirTemp("", "upscaler-concat-*")
	if err != nil {
		return "", nil, err
	}
	return d, func() { _ = os.RemoveAll(d) }, nil
}

// withWorkDir returns a copy of f that uses dir as the scratch area
// and a no-op cleanup. For tests that need to inspect intermediate files.
func (f *Finalizer) withWorkDir(dir string) *Finalizer {
	cp := *f
	cp.workFn = func(string) (string, func(), error) {
		return dir, func() {}, nil
	}
	return &cp
}

// Concat is the main entry point called by the orchestrator.
//
// Steps:
//  1. Glob upscaledSegDir for *.mkv, sort lexically, write concat.txt.
//  2. Run ffmpeg concat demuxer → {tmp}/video.mkv (stream-copy).
//  3. Run ffmpeg remux+HLS transcode → {out}/playlist.m3u8 + segments.
//
// out must be an absolute or relative path; Concat creates it via os.MkdirAll.
func (f *Finalizer) Concat(ctx context.Context, upscaledSegDir string, sc Sidecars, probe source.ProbeResult, out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}

	// --- Step 1: build concat.txt listing sorted segments -----------------
	segs, err := sortedMKVSegments(upscaledSegDir)
	if err != nil {
		return fmt.Errorf("list segments: %w", err)
	}
	if len(segs) == 0 {
		return fmt.Errorf("no .mkv segments found in %s", upscaledSegDir)
	}

	tmp, cleanup, err := f.workFn(out)
	if err != nil {
		return fmt.Errorf("work dir: %w", err)
	}
	defer cleanup()

	concatTxt := filepath.Join(tmp, "concat.txt")
	if err := writeConcatFile(concatTxt, segs); err != nil {
		return fmt.Errorf("write concat.txt: %w", err)
	}

	// --- Step 2: concat demuxer → tmp/video.mkv --------------------------
	videoMKV := filepath.Join(tmp, "video.mkv")
	concatArgs := []string{
		"-hide_banner", "-nostats", "-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatTxt,
		"-c:v", "copy",
		videoMKV,
	}
	if err := f.run(ctx, "", concatArgs); err != nil {
		return fmt.Errorf("concat step: %w", err)
	}

	// --- Step 3: remux + HLS transcode -----------------------------------
	pf := pixFmt(probe.PixFmt)

	segPattern := filepath.Join(out, "segment_%03d.ts")
	playlist := filepath.Join(out, "playlist.m3u8")

	// Build argv dynamically: inputs + maps depend on which sidecars exist.
	//
	// Input slots:
	//   0:v = video.mkv (always)
	//   1:a = audio.mka (when AudioPath != "")
	//   2:s = subs.mks  (when len(SubPaths) > 0)
	//
	// The map indices are positional based on the -i args actually present.
	var remuxArgs []string
	remuxArgs = append(remuxArgs, "-hide_banner", "-nostats", "-y")
	remuxArgs = append(remuxArgs, "-i", videoMKV)

	audioIdx := -1
	subsIdx := -1
	nextInput := 1

	if sc.AudioPath != "" {
		remuxArgs = append(remuxArgs, "-i", sc.AudioPath)
		audioIdx = nextInput
		nextInput++
	}
	if len(sc.SubPaths) > 0 {
		remuxArgs = append(remuxArgs, "-i", sc.SubPaths[0])
		subsIdx = nextInput
		nextInput++
	}

	// Maps.
	remuxArgs = append(remuxArgs, "-map", "0:v")
	if audioIdx >= 0 {
		remuxArgs = append(remuxArgs, "-map", fmt.Sprintf("%d:a?", audioIdx))
	}
	if subsIdx >= 0 {
		remuxArgs = append(remuxArgs, "-map", fmt.Sprintf("%d:s?", subsIdx))
	}

	// Video encode.
	remuxArgs = append(remuxArgs,
		"-c:v", "libx264",
		"-preset", "slow",
		"-crf", "18",
		"-pix_fmt", pf,
	)

	// Audio + subtitle codec passthrough (if present).
	if audioIdx >= 0 {
		remuxArgs = append(remuxArgs, "-c:a", "copy")
	}
	if subsIdx >= 0 {
		remuxArgs = append(remuxArgs, "-c:s", "copy")
	}

	// VFR safety: passthrough fps mode so the frame timestamps from the
	// upscaled segments are preserved as-is without re-timing.
	remuxArgs = append(remuxArgs, "-fps_mode", "passthrough")

	// HLS muxer.
	remuxArgs = append(remuxArgs,
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segPattern,
		playlist,
	)

	if err := f.run(ctx, "", remuxArgs); err != nil {
		return fmt.Errorf("remux step: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sortedMKVSegments globs *.mkv files in dir and returns them sorted.
func sortedMKVSegments(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.mkv"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// writeConcatFile writes a ffmpeg concat demuxer input file listing each segment.
// Format:
//
//	file '/absolute/path/to/seg_00000.mkv'
func writeConcatFile(path string, segs []string) error {
	var sb strings.Builder
	for _, s := range segs {
		fmt.Fprintf(&sb, "file '%s'\n", s)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// pixFmt selects the ffmpeg pix_fmt string from the probe's PixFmt.
// 10-bit sources (any pix_fmt containing "10") → yuv420p10le.
// All other sources → yuv420p (standard 8-bit).
//
// We never downconvert 10-bit to 8-bit; instead we keep the 10-bit pipeline
// so the upscaled output retains the source bit depth.
func pixFmt(probePF string) string {
	if strings.Contains(probePF, "10") {
		return "yuv420p10le"
	}
	return "yuv420p"
}

// run executes the ffmpeg binary with args, capturing stderr to a ring buffer.
// dir is the working directory; "" keeps the caller's cwd.
func (f *Finalizer) run(ctx context.Context, dir string, args []string) error {
	// Reuse the ring-buffer + exec pattern from Segmenter.run.
	seg := &Segmenter{bin: f.bin}
	return seg.run(ctx, dir, args)
}
