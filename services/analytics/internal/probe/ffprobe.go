package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// VideoProber validates that media bytes contain a decodable video stream.
type VideoProber interface {
	Probe(ctx context.Context, mediaBytes []byte) error
}

// FFprobe shells out to ffprobe, reading the segment from stdin.
type FFprobe struct{ path string }

func NewFFprobe(path string) *FFprobe {
	if path == "" {
		path = "ffprobe"
	}
	return &FFprobe{path: path}
}

type ffprobeOut struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

func (f *FFprobe) Probe(ctx context.Context, media []byte) error {
	if len(media) == 0 {
		return fmt.Errorf("empty media")
	}
	cmd := exec.CommandContext(ctx, f.path,
		"-v", "error", "-print_format", "json", "-show_streams", "-")
	cmd.Stdin = bytes.NewReader(media)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}
	var parsed ffprobeOut
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return fmt.Errorf("ffprobe decode: %w", err)
	}
	for _, s := range parsed.Streams {
		if s.CodecType == "video" && s.CodecName != "" {
			return nil
		}
	}
	return fmt.Errorf("no video stream")
}
