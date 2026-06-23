//go:build unix

package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeScript creates a shell script at path with the given contents and
// marks it executable. Mirrors the pattern in
// services/library/internal/ffmpeg/transcoder_test.go.
func writeScript(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("writeScript %s: %v", path, err)
	}
}

// fakeFFprobeHEVC emits a JSON blob that looks like a real 10-bit HEVC file
// with 1 video + 1 audio + 2 subtitle + 3 attachment streams.
const fakeFFprobeHEVC = `#!/bin/sh
cat <<'JSON'
{
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "hevc",
      "pix_fmt": "yuv420p10le",
      "width": 1920,
      "height": 1080,
      "avg_frame_rate": "24000/1001"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "aac"
    },
    {
      "index": 2,
      "codec_type": "subtitle",
      "codec_name": "ass"
    },
    {
      "index": 3,
      "codec_type": "subtitle",
      "codec_name": "ass"
    },
    {
      "index": 4,
      "codec_type": "attachment",
      "codec_name": "ttf"
    },
    {
      "index": 5,
      "codec_type": "attachment",
      "codec_name": "ttf"
    },
    {
      "index": 6,
      "codec_type": "attachment",
      "codec_name": "ttf"
    }
  ],
  "format": {
    "duration": "1410.5",
    "bit_rate": "8000000"
  }
}
JSON
`

// fakeFFprobeNoVideo emits valid JSON with no video stream (only audio).
const fakeFFprobeNoVideo = `#!/bin/sh
cat <<'JSON'
{
  "streams": [
    {
      "index": 0,
      "codec_type": "audio",
      "codec_name": "aac"
    }
  ],
  "format": {
    "duration": "60",
    "bit_rate": "128000"
  }
}
JSON
`

// fakeFFprobeMultiVideo emits two video streams; probe must pick the
// largest by width*height (the 4K stream).
const fakeFFprobeMultiVideo = `#!/bin/sh
cat <<'JSON'
{
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "h264",
      "pix_fmt": "yuv420p",
      "width": 1280,
      "height": 720,
      "avg_frame_rate": "30/1"
    },
    {
      "index": 1,
      "codec_type": "video",
      "codec_name": "hevc",
      "pix_fmt": "yuv420p10le",
      "width": 3840,
      "height": 2160,
      "avg_frame_rate": "24000/1001"
    },
    {
      "index": 2,
      "codec_type": "audio",
      "codec_name": "flac"
    }
  ],
  "format": {
    "duration": "1410.5",
    "bit_rate": "25000000"
  }
}
JSON
`

// fakeFFprobeFails exits 1 to simulate ffprobe execution failure.
const fakeFFprobeFails = `#!/bin/sh
echo "Error: Invalid data" 1>&2
exit 1
`

// fakeFFprobeCoverArtPlusReal emits a cover-art "video" stream
// (disposition.attached_pic=1) AND a real video stream. The cover art is
// declared LARGER (4000x4000) than the real stream (1280x720) on purpose:
// a naive largest-area selector would pick the cover art, so this proves the
// attached_pic=1 skip is load-bearing — the real stream must still win.
const fakeFFprobeCoverArtPlusReal = `#!/bin/sh
cat <<'JSON'
{
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "mjpeg",
      "pix_fmt": "yuvj420p",
      "width": 4000,
      "height": 4000,
      "avg_frame_rate": "90000/3753",
      "disposition": { "attached_pic": 1 }
    },
    {
      "index": 1,
      "codec_type": "video",
      "codec_name": "h264",
      "pix_fmt": "yuv420p",
      "width": 1280,
      "height": 720,
      "avg_frame_rate": "24000/1001",
      "disposition": { "attached_pic": 0 }
    },
    {
      "index": 2,
      "codec_type": "audio",
      "codec_name": "aac"
    }
  ],
  "format": { "duration": "1410.5", "bit_rate": "5000000" }
}
JSON
`

// fakeFFprobeOnlyCoverArt emits a SINGLE video stream which is cover art
// (attached_pic=1) and nothing else. Probe must FAIL — there is no real
// video stream.
const fakeFFprobeOnlyCoverArt = `#!/bin/sh
cat <<'JSON'
{
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "png",
      "pix_fmt": "rgba",
      "width": 1000,
      "height": 1500,
      "avg_frame_rate": "0/0",
      "disposition": { "attached_pic": 1 }
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "mp3"
    }
  ],
  "format": { "duration": "1410.5", "bit_rate": "320000" }
}
JSON
`

func TestProbe_HEVCWith10Bit(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe.sh")
	writeScript(t, probeBin, fakeFFprobeHEVC)

	prober := NewProber(probeBin)

	// The path doesn't need to exist — the fake script ignores argv.
	result, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err != nil {
		t.Fatalf("Probe() error = %v, want nil", err)
	}

	if result.Codec != "hevc" {
		t.Errorf("Codec = %q, want %q", result.Codec, "hevc")
	}
	if result.PixFmt != "yuv420p10le" {
		t.Errorf("PixFmt = %q, want %q", result.PixFmt, "yuv420p10le")
	}
	if result.FPS != "24000/1001" {
		t.Errorf("FPS = %q, want %q", result.FPS, "24000/1001")
	}
	if result.Width != 1920 {
		t.Errorf("Width = %d, want 1920", result.Width)
	}
	if result.Height != 1080 {
		t.Errorf("Height = %d, want 1080", result.Height)
	}
	if !result.HasAudio {
		t.Errorf("HasAudio = false, want true")
	}
	if len(result.SubTracks) != 2 {
		t.Errorf("SubTracks len = %d, want 2; tracks: %v", len(result.SubTracks), result.SubTracks)
	}
	if result.FontAttachments != 3 {
		t.Errorf("FontAttachments = %d, want 3", result.FontAttachments)
	}
	// VideoPath is the input path echoed back.
	if result.VideoPath == "" {
		t.Errorf("VideoPath is empty, want the probed file path")
	}
}

func TestProbe_NoVideoStream_Error(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe_no_video.sh")
	writeScript(t, probeBin, fakeFFprobeNoVideo)

	prober := NewProber(probeBin)

	_, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err == nil {
		t.Fatalf("Probe() error = nil, want non-nil (no video stream)")
	}
}

func TestProbe_FFprobeFails_Error(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe_fail.sh")
	writeScript(t, probeBin, fakeFFprobeFails)

	prober := NewProber(probeBin)

	_, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err == nil {
		t.Fatalf("Probe() error = nil, want non-nil (ffprobe exec failure)")
	}
}

func TestProbe_MultipleVideoStreams_PicksLargest(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe_multi.sh")
	writeScript(t, probeBin, fakeFFprobeMultiVideo)

	prober := NewProber(probeBin)

	result, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err != nil {
		t.Fatalf("Probe() error = %v, want nil", err)
	}

	// Should pick the 4K (3840x2160) stream, not the 720p one.
	if result.Codec != "hevc" {
		t.Errorf("Codec = %q, want %q (4K stream)", result.Codec, "hevc")
	}
	if result.Width != 3840 || result.Height != 2160 {
		t.Errorf("dims = %dx%d, want 3840x2160", result.Width, result.Height)
	}
	if !result.HasAudio {
		t.Errorf("HasAudio = false, want true (flac stream present)")
	}
}

func TestProbe_CoverArtSkipped_RealVideoWins(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe_cover.sh")
	writeScript(t, probeBin, fakeFFprobeCoverArtPlusReal)

	prober := NewProber(probeBin)

	result, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err != nil {
		t.Fatalf("Probe() error = %v, want nil", err)
	}

	// The mjpeg cover-art stream (attached_pic=1) must be skipped; the real
	// h264 720p stream wins.
	if result.Codec != "h264" {
		t.Errorf("Codec = %q, want %q (real stream, cover art skipped)", result.Codec, "h264")
	}
	if result.Width != 1280 || result.Height != 720 {
		t.Errorf("dims = %dx%d, want 1280x720 (real stream)", result.Width, result.Height)
	}
	if result.PixFmt != "yuv420p" {
		t.Errorf("PixFmt = %q, want %q (real stream, not cover art's yuvj420p)", result.PixFmt, "yuv420p")
	}
}

func TestProbe_OnlyCoverArt_Error(t *testing.T) {
	dir := t.TempDir()
	probeBin := filepath.Join(dir, "fake_ffprobe_only_cover.sh")
	writeScript(t, probeBin, fakeFFprobeOnlyCoverArt)

	prober := NewProber(probeBin)

	_, err := prober.Probe(context.Background(), filepath.Join(dir, "fake.mkv"))
	if err == nil {
		t.Fatalf("Probe() error = nil, want non-nil (only cover art, no real video stream)")
	}
}
