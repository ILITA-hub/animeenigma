// Package ffmpeg wraps the ffmpeg subprocess used by the upscaler service
// to split source video into lossless keyframe-aligned segments (for
// per-segment upscaling) and to demux the audio/subs/fonts/chapters
// sidecars that bypass upscaling and are retained for the final remux.
//
// Design mirrors services/library/internal/ffmpeg/transcoder.go:
//   - exec.CommandContext — no shell interpolation.
//   - Bounded 2 KB stderr ring buffer — error messages are safe to log.
//   - ffmpeg binary path is injectable so tests use fake shell scripts.
package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

// ---------------------------------------------------------------------------
// Ring buffer (bounded stderr capture)
// ---------------------------------------------------------------------------

// ringBuffer is a fixed-capacity byte sink used for stderr capture.
// Writes past the capacity discard the OLDEST bytes so String() always
// returns the most recent `cap` bytes.
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newRingBuffer(cap int) *ringBuffer {
	if cap <= 0 {
		cap = 2048
	}
	return &ringBuffer{cap: cap}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.cap {
		r.buf = append([]byte(nil), r.buf[len(r.buf)-r.cap:]...)
	}
	return len(p), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Sidecars holds the paths produced by DemuxSidecars.
// Fields are empty/nil when the source has no such track/attachment.
type Sidecars struct {
	AudioPath    string   // {outDir}/audio.mka — all audio tracks muxed; "" if source has no audio
	SubPaths     []string // {outDir}/subs.mks when non-empty (subtitle tracks)
	FontPaths    []string // files inside {outDir}/fonts/ (attachment dumps)
	ChaptersPath string   // {outDir}/chapters.ini; "" if empty file
}

// Segmenter wraps ffmpeg for the two upscaler-pipeline operations:
//   - Segment: split the video-only stream into lossless matroska chunks.
//   - DemuxSidecars: extract audio, subs, fonts, chapters to separate files.
//
// Safe for concurrent use — each call is stateless (no shared output dirs).
type Segmenter struct {
	bin string // path to ffmpeg binary
}

// NewSegmenter returns a Segmenter that shells out to the given binary.
// Pass "ffmpeg" to use the first binary on $PATH, or an absolute path
// (or a fake script) for tests.
func NewSegmenter(ffmpegBin string) *Segmenter {
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	return &Segmenter{bin: ffmpegBin}
}

// Segment splits the video stream from srcVideoPath into keyframe-aligned
// lossless matroska segments inside outDir.
//
// Args (verbatim from SPEC):
//
//	ffmpeg -hide_banner -nostats -y -i {src}
//	       -map 0:v:0 -c:v copy -an -sn
//	       -f segment -segment_time {seconds} -reset_timestamps 1
//	       -segment_format matroska
//	       {outDir}/seg_%05d.mkv
//
// Returns the segment paths sorted lexically (lexical == numeric for
// zero-padded filenames).
func (s *Segmenter) Segment(ctx context.Context, srcVideoPath, outDir string, seconds int) ([]string, error) {
	pattern := filepath.Join(outDir, "seg_%05d.mkv")

	args := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", srcVideoPath,
		"-map", "0:v:0",
		"-c:v", "copy",
		"-an",
		"-sn",
		"-f", "segment",
		"-segment_time", strconv.Itoa(seconds),
		"-reset_timestamps", "1",
		"-segment_format", "matroska",
		pattern,
	}

	if err := s.run(ctx, "", args); err != nil {
		return nil, fmt.Errorf("segment: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(outDir, "seg_*.mkv"))
	if err != nil {
		return nil, fmt.Errorf("glob segments: %w", err)
	}
	sort.Strings(matches)
	return matches, nil
}

// DemuxSidecars extracts the audio, subtitle, font-attachment, and chapter
// streams from srcPath into outDir without re-encoding anything.
//
// Four separate ffmpeg invocations (as specified):
//  1. Audio:   -map 0:a? -c:a copy {outDir}/audio.mka
//  2. Subs:    -map 0:s? -c:s copy {outDir}/subs.mks
//  3. Fonts:   -dump_attachment:t "" -i {src}  (cwd = {outDir}/fonts/)
//  4. Chapters: -f ffmetadata {outDir}/chapters.ini
//
// Empty/zero-size outputs leave the corresponding fields empty — no error.
func (s *Segmenter) DemuxSidecars(ctx context.Context, srcPath, outDir string) (Sidecars, error) {
	var sc Sidecars

	// 1. Audio ----------------------------------------------------------------
	audioPath := filepath.Join(outDir, "audio.mka")
	audioArgs := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", srcPath,
		"-map", "0:a?",
		"-c:a", "copy",
		audioPath,
	}
	if err := s.run(ctx, "", audioArgs); err != nil {
		return sc, fmt.Errorf("demux audio: %w", err)
	}
	if nonEmpty(audioPath) {
		sc.AudioPath = audioPath
	}

	// 2. Subtitles ------------------------------------------------------------
	subsPath := filepath.Join(outDir, "subs.mks")
	subsArgs := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", srcPath,
		"-map", "0:s?",
		"-c:s", "copy",
		subsPath,
	}
	if err := s.run(ctx, "", subsArgs); err != nil {
		return sc, fmt.Errorf("demux subs: %w", err)
	}
	if nonEmpty(subsPath) {
		sc.SubPaths = []string{subsPath}
	}

	// 3. Fonts / attachments --------------------------------------------------
	// ffmpeg -dump_attachment:t "" writes attachment files to cwd.
	fontsDir := filepath.Join(outDir, "fonts")
	if err := os.MkdirAll(fontsDir, 0o755); err != nil {
		return sc, fmt.Errorf("mkdir fonts: %w", err)
	}
	fontArgs := []string{
		"-hide_banner", "-nostats",
		"-dump_attachment:t", "",
		"-i", srcPath,
	}
	// Ignore exit code from dump_attachment: ffmpeg exits non-zero when
	// there are no attachments, but that's not an error for us.
	_ = s.run(ctx, fontsDir, fontArgs)

	fontPaths, err := collectFonts(fontsDir)
	if err != nil {
		return sc, fmt.Errorf("collect fonts: %w", err)
	}
	sc.FontPaths = fontPaths

	// 4. Chapters -------------------------------------------------------------
	chapPath := filepath.Join(outDir, "chapters.ini")
	chapArgs := []string{
		"-hide_banner", "-nostats", "-y",
		"-i", srcPath,
		"-f", "ffmetadata",
		chapPath,
	}
	if err := s.run(ctx, "", chapArgs); err != nil {
		return sc, fmt.Errorf("demux chapters: %w", err)
	}
	if nonEmpty(chapPath) {
		sc.ChaptersPath = chapPath
	}

	return sc, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// run executes the ffmpeg binary with args, capturing stderr to a ring
// buffer. dir is the working directory for the child process; "" keeps
// the caller's cwd.
func (s *Segmenter) run(ctx context.Context, dir string, args []string) error {
	cmd := exec.CommandContext(ctx, s.bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	ring := newRingBuffer(2048)
	cmd.Stderr = ring
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %s\nstderr tail:\n%s", err, ring.String())
	}
	return nil
}

// nonEmpty returns true if the file at path exists and has size > 0.
func nonEmpty(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// collectFonts globs all files in dir (non-recursive) and returns their
// absolute paths. Returns nil, nil when the directory is empty.
func collectFonts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}
