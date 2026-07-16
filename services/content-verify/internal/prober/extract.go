package prober

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// ExtractFragment pulls one ~30s fragment at seek: mono 16k wav (whisper)
// + 5 frames (1/6 fps) for the hardsub band scan, in a single ffmpeg run.
// input is either a local .m3u8 (HLS) or a proxied http URL (mp4).
func ExtractFragment(ctx context.Context, ffmpegPath, input string, seek float64, durSec, idx int, dir string) (wav string, err error) {
	wav = filepath.Join(dir, fmt.Sprintf("frag_%d.wav", idx))
	frames := filepath.Join(dir, "frames")
	args := []string{
		"-allowed_extensions", "ALL",
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-ss", fmt.Sprintf("%.1f", seek),
		"-i", input,
		"-t", fmt.Sprintf("%d", durSec),
		"-vn", "-ac", "1", "-ar", "16000", "-y", wav,
		"-vf", "fps=1/6", "-q:v", "2", "-y", filepath.Join(frames, fmt.Sprintf("f_%d_%%02d.png", idx)),
		"-loglevel", "error",
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 2048}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg fragment %d: %w\nstderr tail:\n%s", idx, err, stderr.String())
	}
	return wav, nil
}

type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if remaining := l.n - l.w.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		l.w.Write(p)
	}
	return len(p), nil
}
