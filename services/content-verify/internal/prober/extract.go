package prober

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractFragment pulls one ~30s fragment at seek: mono 16k wav (whisper)
// + 5 frames (1/6 fps) for the hardsub band scan, in a single ffmpeg run.
// input is either a local .m3u8 (HLS) or a proxied http URL (mp4).
func ExtractFragment(ctx context.Context, ffmpegPath, input string, seek float64, durSec, idx int, dir string) (wav string, err error) {
	wav = filepath.Join(dir, fmt.Sprintf("frag_%d.wav", idx))
	frames := filepath.Join(dir, "frames")
	var args []string
	if strings.HasSuffix(input, ".m3u8") {
		// allowed_extensions is a private option of the hls demuxer: our
		// proxied segment URLs end in a query string rather than a plain
		// .ts/.m4s extension, so the demuxer's default safelist would
		// reject them. Scoped to m3u8 inputs ONLY — passing it against
		// the generic (mp4) demuxer breaks input opening entirely
		// ("Option allowed_extensions not found", exit 8).
		args = append(args, "-allowed_extensions", "ALL")
	}
	args = append(args,
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		// -ss/-t as INPUT options (before -i) bound how much of the input
		// ffmpeg reads at all — both outputs below (wav AND png frames)
		// inherit that bound. Putting -t after -i would only bound the
		// wav's OUTPUT duration, leaving the png output decoding from
		// seek to end-of-input (~180 frames / minutes on a 24min episode
		// instead of ~5 frames / durSec seconds): blows the probe budget
		// and starves the frames/5 hardsub threshold of its expected
		// frame count.
		"-ss", fmt.Sprintf("%.1f", seek),
		"-t", fmt.Sprintf("%d", durSec),
		"-i", input,
		"-vn", "-ac", "1", "-ar", "16000", "-y", wav,
		"-vf", "fps=1/6", "-y", filepath.Join(frames, fmt.Sprintf("f_%d_%%02d.png", idx)),
		"-loglevel", "error",
	)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 2048}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg fragment %d: %w\nstderr tail:\n%s", idx, err, stderr.String())
	}
	return wav, nil
}

// limitedWriter is a true tail ring: it always retains the LAST n bytes
// written, not the first. ffmpeg's fatal error line is at the end of
// stderr, so a head-truncating buffer would keep only harmless startup
// banner noise and throw away the one line that explains the failure.
type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	l.w.Write(p)
	if l.w.Len() > l.n {
		tail := append([]byte(nil), l.w.Bytes()[l.w.Len()-l.n:]...)
		l.w.Reset()
		l.w.Write(tail)
	}
	return len(p), nil
}
