// Package source provides acquisition and probing of original anime video
// files from the library torrent volume.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// ProbeResult holds the parsed metadata extracted from a video file via
// ffprobe. A Probe failure (ffprobe error or no video stream found) always
// returns an error — soft-failing to zero values is NOT acceptable here
// because the orchestrator needs accurate metadata for segmenting and remux.
type ProbeResult struct {
	// VideoPath is the path that was probed (echoed back for traceability).
	VideoPath string
	// Codec is the video codec name (e.g. "hevc", "h264", "av1").
	Codec string
	// PixFmt is the pixel format (e.g. "yuv420p10le", "yuv420p").
	PixFmt string
	// FPS is the avg_frame_rate string from ffprobe (e.g. "24000/1001").
	FPS string
	// Width and Height are the video dimensions in pixels.
	Width, Height int
	// HasAudio is true when at least one audio stream is present.
	HasAudio bool
	// SubTracks holds the stream indices of subtitle streams.
	SubTracks []int
	// FontAttachments is the count of attachment streams (embedded fonts).
	FontAttachments int
}

// ffprobeOutput is the minimal subset of ffprobe's JSON we parse.
// The streams array is new here vs library/ffmpeg/transcoder.go which only
// reads format.duration/bit_rate.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

type ffprobeStream struct {
	Index        int    `json:"index"`
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	PixFmt       string `json:"pix_fmt"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	AvgFrameRate string `json:"avg_frame_rate"`
}

// Prober runs ffprobe against a video file and parses the streams array.
// Create with NewProber; the binary path is injectable for testing.
type Prober struct {
	// ffprobePath is the path to the ffprobe binary.
	// Defaults to "ffprobe" (resolved via PATH) when constructed with NewProber("").
	ffprobePath string
}

// NewProber creates a Prober that uses ffprobeBin as the ffprobe binary.
// Pass "" to use the system default ("ffprobe" on PATH).
func NewProber(ffprobeBin string) *Prober {
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	return &Prober{ffprobePath: ffprobeBin}
}

// Probe runs ffprobe on path and returns the parsed ProbeResult.
// The ffprobe invocation matches services/library/internal/ffmpeg/transcoder.go
// probe() but additionally parses the streams[] array.
//
// Returns an error when:
//   - ffprobe exits non-zero or cannot be executed
//   - the output cannot be parsed as JSON
//   - no video stream is found in the output
func (p *Prober) Probe(ctx context.Context, path string) (ProbeResult, error) {
	cmd := exec.CommandContext(ctx, p.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe failed on %q: %w", path, err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe output parse failed for %q: %w", path, err)
	}

	result := ProbeResult{
		VideoPath: path,
	}

	// Pick the best video stream: largest by width*height. Track whether
	// any audio/subtitle/attachment streams exist.
	bestVideoArea := 0
	foundVideo := false

	for _, s := range parsed.Streams {
		switch s.CodecType {
		case "video":
			area := s.Width * s.Height
			if !foundVideo || area > bestVideoArea {
				result.Codec = s.CodecName
				result.PixFmt = s.PixFmt
				result.FPS = s.AvgFrameRate
				result.Width = s.Width
				result.Height = s.Height
				bestVideoArea = area
				foundVideo = true
			}
		case "audio":
			result.HasAudio = true
		case "subtitle":
			result.SubTracks = append(result.SubTracks, s.Index)
		case "attachment":
			result.FontAttachments++
		}
	}

	if !foundVideo {
		return ProbeResult{}, fmt.Errorf("no video stream found in %q", path)
	}

	return result, nil
}

// Probe is a package-level convenience that uses the system ffprobe binary.
// Tests should use NewProber(fakeBin).Probe(...) instead for determinism.
func Probe(ctx context.Context, path string) (ProbeResult, error) {
	return NewProber("").Probe(ctx, path)
}
