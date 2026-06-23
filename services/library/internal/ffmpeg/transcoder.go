// Package ffmpeg wraps the ffmpeg + ffprobe subprocesses used by the
// library encoder worker to transcode source files into VOD HLS
// (H.264 + AAC + 6s segments).
//
// The wrapper:
//   - Runs ffprobe first to extract duration + source bitrate.
//   - Composes the SPEC-locked ffmpeg argv (no shell interpolation).
//   - Captures stderr to a bounded 2 KB ring buffer.
//   - On non-zero exit, returns the stderr tail as part of the error.
//   - On success returns the playlist path + segment paths + total
//     bytes + duration so the caller can upload + persist metadata.
//
// The caller is responsible for cleaning the per-call temp dir on
// success; on failure the temp dir is RETAINED for debugging (the
// encoder worker writes the stderr tail to library_jobs.error_text
// so admins can inspect both signals).
package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Config holds the ffmpeg + ffprobe knobs locked by 04-SPEC.
type Config struct {
	BinaryPath     string // path to ffmpeg, e.g. /usr/bin/ffmpeg
	FfprobePath    string // path to ffprobe, e.g. /usr/bin/ffprobe
	Tmpdir         string // root scratch dir; per-call subdir is auto-created
	MaxBitrateKbps int    // bitrate cap; default 5000 if <= 0
	Threads        int    // libx264 thread cap; 0 = auto (omit -threads)
	Nice           int    // child scheduling niceness; 0 = don't reprioritize (Task 2)
}

// Result is what Transcode returns on success.
type Result struct {
	PlaylistPath string   // absolute path to {tmp}/playlist.m3u8
	SegmentPaths []string // absolute paths to {tmp}/segment_NNN.ts, sorted ASC
	DurationSec  int      // from ffprobe
	SizeBytes    int64    // playlist + all segments
}

// Transcoder is the public façade. Safe for concurrent use — each
// call creates its own MkdirTemp directory so concurrent encoders
// can't collide on output paths.
type Transcoder struct {
	cfg Config
	log *logger.Logger
}

// NewTranscoder wires a Transcoder. Defaults applied:
//   - BinaryPath  → /usr/bin/ffmpeg
//   - FfprobePath → /usr/bin/ffprobe
//   - Tmpdir      → os.TempDir() when empty
//   - MaxBitrateKbps → 5000 when <= 0
func NewTranscoder(cfg Config, log *logger.Logger) *Transcoder {
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "/usr/bin/ffmpeg"
	}
	if cfg.FfprobePath == "" {
		cfg.FfprobePath = "/usr/bin/ffprobe"
	}
	if cfg.Tmpdir == "" {
		cfg.Tmpdir = os.TempDir()
	}
	if cfg.MaxBitrateKbps <= 0 {
		cfg.MaxBitrateKbps = 5000
	}
	return &Transcoder{cfg: cfg, log: log}
}

// ringBuffer is a fixed-cap byte sink used for stderr capture. Writes
// past the cap discard the OLDEST bytes (FIFO), so String() always
// returns the most recent `cap` bytes in chronological order.
//
// Implementation: keep a buffer of length 2*cap. New writes append;
// when len > 2*cap, slide back to the last `cap` bytes. This keeps
// allocations bounded and amortizes the slide cost.
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

// Write appends p and trims the head if total length exceeds cap.
func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.cap {
		// Keep the last `cap` bytes.
		r.buf = append([]byte(nil), r.buf[len(r.buf)-r.cap:]...)
	}
	return len(p), nil
}

// String returns the current tail contents.
func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// ffprobeOutput is the minimal subset of ffprobe's JSON we read.
type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// probe runs ffprobe and returns (durationSec, bitrateKbps). On parse
// failure both come back as zero; the caller substitutes the default
// bitrate cap. Errors from exec.Run are NOT fatal — probe is
// best-effort metadata.
func (t *Transcoder) probe(ctx context.Context, sourcePath string) (int, int) {
	cmd := exec.CommandContext(ctx, t.cfg.FfprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		sourcePath,
	)
	out, err := cmd.Output()
	if err != nil {
		if t.log != nil {
			t.log.Warnw("ffprobe failed; falling back to bitrate cap",
				"source", sourcePath, "error", err)
		}
		return 0, 0
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		if t.log != nil {
			t.log.Warnw("ffprobe output parse failed",
				"source", sourcePath, "error", err)
		}
		return 0, 0
	}
	durFloat, _ := strconv.ParseFloat(parsed.Format.Duration, 64)
	brBps, _ := strconv.ParseInt(parsed.Format.BitRate, 10, 64)
	return int(durFloat), int(brBps / 1000)
}

// Transcode is the public entry. Creates a per-call temp dir, runs
// ffprobe, computes the chosen bitrate, runs ffmpeg, returns the
// Result on success.
//
// On non-zero ffmpeg exit, returns an error containing the stderr tail
// and DOES NOT clean up the temp dir (the encoder worker writes both
// signals to library_jobs.error_text). On success the temp dir is
// also retained — the caller (encoder worker) is responsible for
// os.RemoveAll(filepath.Dir(result.PlaylistPath)) AFTER the MinIO
// upload completes.
func (t *Transcoder) Transcode(ctx context.Context, sourcePath string) (*Result, error) {
	if t.cfg.Tmpdir != "" {
		if err := os.MkdirAll(t.cfg.Tmpdir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir tmpdir: %w", err)
		}
	}
	tmp, err := os.MkdirTemp(t.cfg.Tmpdir, "encode-")
	if err != nil {
		return nil, fmt.Errorf("mkdir per-call tmpdir: %w", err)
	}

	durationSec, sourceKbps := t.probe(ctx, sourcePath)

	bv := t.cfg.MaxBitrateKbps
	if sourceKbps > 0 && sourceKbps < bv {
		bv = sourceKbps
	}
	if bv < 500 {
		bv = 500
	}

	playlistPath := filepath.Join(tmp, "playlist.m3u8")
	segmentTemplate := filepath.Join(tmp, "segment_%03d.ts")

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

	// Enumerate produced segments.
	matches, err := filepath.Glob(filepath.Join(tmp, "segment_*.ts"))
	if err != nil {
		return nil, fmt.Errorf("glob segments: %w", err)
	}
	sort.Strings(matches)

	// Sum total bytes (playlist + segments).
	var total int64
	if st, err := os.Stat(playlistPath); err == nil {
		total += st.Size()
	}
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil {
			total += st.Size()
		}
	}

	if strings.TrimSpace(playlistPath) == "" {
		return nil, fmt.Errorf("internal: empty playlist path")
	}

	return &Result{
		PlaylistPath: playlistPath,
		SegmentPaths: matches,
		DurationSec:  durationSec,
		SizeBytes:    total,
	}, nil
}
