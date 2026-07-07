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
	// Height is the source video stream's height in pixels, as reported by
	// ffprobe. The transcode argv never applies `-vf scale`, so the encoded
	// output height always matches the probed source height. Zero when
	// ffprobe failed, its output didn't parse, or no video stream was
	// reported — callers must treat 0 as "unknown", not "0p".
	Height int
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

// ffprobeOutput is the minimal subset of ffprobe's JSON we read. Streams is
// populated by the SAME `-show_streams` flag probe() already passes to
// ffprobe (previously requested but unparsed) — reading it to find the video
// stream's height costs no extra subprocess.
type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

// ffprobeStream is the minimal subset of an ffprobe stream record needed to
// find the source video's height (CodecType == "video").
type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	Height    int    `json:"height"`
}

// TranscodeOpts carries optional per-call knobs. The zero value reproduces
// the historical behavior (ffmpeg's default stream selection), so every
// existing caller — the encoder worker's RAW/JP autocache path in
// particular — keeps its original-audio semantics untouched.
type TranscodeOpts struct {
	// AudioLang, when non-empty, is the ISO-639 language of the audio track
	// to burn into the output (e.g. "eng"). It is used by the admin
	// batch-ingest DUB path: source releases are dual-audio (JP + EN) with
	// Japanese usually first, and ffmpeg's default selection ("most channels,
	// else first") would pick Japanese. When a matching track exists we map it
	// explicitly; when none matches we fall back to ffmpeg's default (so a
	// single-language source still encodes rather than failing).
	AudioLang string
}

// probedAudioStream is the slice of an ffprobe audio-stream record we read to
// resolve a language → `-map 0:a:N` ordinal.
type probedAudioStream struct {
	Tags struct {
		Language string `json:"language"`
	} `json:"tags"`
}

type ffprobeAudioStreams struct {
	Streams []probedAudioStream `json:"streams"`
}

// langMatches reports whether an ffprobe language tag denotes the same
// language the caller asked for, tolerating the 2-letter / 3-letter / English
// spellings common in release metadata (en/eng/english, ja/jpn/japanese, …).
func langMatches(tag, want string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	want = strings.ToLower(strings.TrimSpace(want))
	if tag == "" || want == "" {
		return false
	}
	if tag == want {
		return true
	}
	norm := func(s string) string {
		switch s {
		case "en", "eng", "english":
			return "eng"
		case "ja", "jp", "jpn", "japanese":
			return "jpn"
		case "ru", "rus", "russian":
			return "rus"
		}
		return s
	}
	return norm(tag) == norm(want)
}

// selectAudioOrdinal returns the 0-based AUDIO-stream ordinal (the N in
// `-map 0:a:N`) of the first audio stream whose language tag matches want.
// The input slice MUST be the audio streams in ffprobe order. Returns
// (0, false) when nothing matches — the caller then omits `-map` and lets
// ffmpeg pick its default audio.
func selectAudioOrdinal(streams []probedAudioStream, want string) (int, bool) {
	for i, s := range streams {
		if langMatches(s.Tags.Language, want) {
			return i, true
		}
	}
	return 0, false
}

// ScopedTempDir creates a fresh per-call scratch directory for an ffmpeg
// pass: it first ensures base exists (MkdirAll, best-effort — so a
// configured-but-not-yet-created tmpdir doesn't fail every call), then
// MkdirTemp's a prefix-named subdirectory inside it. Shared by Transcode
// ("encode-"), Storyboard ("storyboard-"), and the storyboard backfill
// worker ("sb-backfill-", which passes its own configured base rather than
// a Transcoder's Config.Tmpdir).
func ScopedTempDir(base, prefix string) (string, error) {
	if base != "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return "", fmt.Errorf("mkdir tmpdir: %w", err)
		}
	}
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return "", fmt.Errorf("mkdir %s tmpdir: %w", strings.TrimSuffix(prefix, "-"), err)
	}
	return dir, nil
}

// runFfmpeg runs an ffmpeg subprocess with the plumbing shared by Transcode
// and Storyboard: exec.CommandContext, a bounded 2KB stderr ring-buffer,
// cmd.Start, a best-effort nice-priority reduction (NEVER fails the call
// over priority), and cmd.Wait. label identifies the caller in the returned
// error text — Transcode passes "ffmpeg" (keeping its error strings
// byte-identical to before this extraction); Storyboard passes "ffmpeg
// storyboard".
func (t *Transcoder) runFfmpeg(ctx context.Context, args []string, label string) error {
	cmd := exec.CommandContext(ctx, t.cfg.BinaryPath, args...)
	ring := newRingBuffer(2048)
	cmd.Stderr = ring
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s start failed: %s", label, err)
	}
	// Best-effort: run at low scheduling priority so it yields to interactive
	// work. NEVER fail the call over priority.
	if t.cfg.Nice > 0 {
		if err := syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, t.cfg.Nice); err != nil && t.log != nil {
			t.log.Debugw("setpriority(ffmpeg) failed; continuing at default priority",
				"label", label, "pid", cmd.Process.Pid, "nice", t.cfg.Nice, "error", err)
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %s\nstderr tail:\n%s", label, err, ring.String())
	}
	return nil
}

// probe runs ffprobe and returns (durationSec, bitrateKbps, height). height
// is the first video stream's pixel height, or 0 when none was reported. On
// parse failure all three come back as zero; the caller substitutes the
// default bitrate cap and treats height as unknown. Errors from exec.Run are
// NOT fatal — probe is best-effort metadata.
func (t *Transcoder) probe(ctx context.Context, sourcePath string) (int, int, int) {
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
		return 0, 0, 0
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		if t.log != nil {
			t.log.Warnw("ffprobe output parse failed",
				"source", sourcePath, "error", err)
		}
		return 0, 0, 0
	}
	durFloat, _ := strconv.ParseFloat(parsed.Format.Duration, 64)
	brBps, _ := strconv.ParseInt(parsed.Format.BitRate, 10, 64)
	height := 0
	for _, s := range parsed.Streams {
		if s.CodecType == "video" && s.Height > 0 {
			height = s.Height
			break
		}
	}
	return int(durFloat), int(brBps / 1000), height
}

// audioOrdinalForLang runs ffprobe over the audio streams only and returns the
// 0-based audio ordinal (the N in `-map 0:a:N`) of the first track whose
// language tag matches lang. Returns (0, false) on any ffprobe/parse error or
// when no track matches — the caller then omits `-map` and lets ffmpeg choose
// its default audio, so a probe failure degrades gracefully rather than
// dropping audio.
func (t *Transcoder) audioOrdinalForLang(ctx context.Context, sourcePath, lang string) (int, bool) {
	cmd := exec.CommandContext(ctx, t.cfg.FfprobePath,
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream_tags=language",
		"-print_format", "json",
		sourcePath,
	)
	out, err := cmd.Output()
	if err != nil {
		if t.log != nil {
			t.log.Warnw("ffprobe audio streams failed; using ffmpeg default audio",
				"source", sourcePath, "error", err)
		}
		return 0, false
	}
	var parsed ffprobeAudioStreams
	if err := json.Unmarshal(out, &parsed); err != nil {
		if t.log != nil {
			t.log.Warnw("ffprobe audio streams parse failed; using ffmpeg default audio",
				"source", sourcePath, "error", err)
		}
		return 0, false
	}
	return selectAudioOrdinal(parsed.Streams, lang)
}

// Transcode is the public entry. Creates a per-call temp dir, runs
// ffprobe, computes the chosen bitrate, runs ffmpeg, returns the
// Result on success. It preserves ffmpeg's default audio selection
// (the RAW/JP autocache path relies on this original-audio behavior).
//
// On non-zero ffmpeg exit, returns an error containing the stderr tail
// and DOES NOT clean up the temp dir (the encoder worker writes both
// signals to library_jobs.error_text). On success the temp dir is
// also retained — the caller (encoder worker) is responsible for
// os.RemoveAll(filepath.Dir(result.PlaylistPath)) AFTER the MinIO
// upload completes.
func (t *Transcoder) Transcode(ctx context.Context, sourcePath string) (*Result, error) {
	return t.TranscodeWithOpts(ctx, sourcePath, TranscodeOpts{})
}

// TranscodeWithOpts is Transcode plus optional per-call knobs (opts). With the
// zero-value opts it is byte-for-byte the old Transcode. When opts.AudioLang is
// set it maps the matching audio track explicitly (the admin DUB batch-ingest
// path); a non-matching source falls back to ffmpeg's default audio.
func (t *Transcoder) TranscodeWithOpts(ctx context.Context, sourcePath string, opts TranscodeOpts) (*Result, error) {
	tmp, err := ScopedTempDir(t.cfg.Tmpdir, "encode-")
	if err != nil {
		return nil, err
	}

	durationSec, sourceKbps, height := t.probe(ctx, sourcePath)

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
	}
	// Explicit audio-track selection for the DUB ingest path. When a track in
	// the requested language exists, map first video + that audio; otherwise
	// leave the stream map unset so ffmpeg's default selection still produces
	// output (a single-language source, or one lacking the requested dub).
	if opts.AudioLang != "" {
		if ord, ok := t.audioOrdinalForLang(ctx, sourcePath, opts.AudioLang); ok {
			args = append(args, "-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", ord))
		} else if t.log != nil {
			t.log.Warnw("no audio track matched requested language; using ffmpeg default",
				"lang", opts.AudioLang, "source", sourcePath)
		}
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "veryfast",
	)
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
	if err := t.runFfmpeg(ctx, args, "ffmpeg"); err != nil {
		return nil, err
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
		Height:       height,
	}, nil
}
