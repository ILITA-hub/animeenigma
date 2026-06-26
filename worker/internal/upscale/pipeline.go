package upscale

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// FFmpegBin is the path to the ffmpeg binary used by Process.
// Tests can override this to point to a fake shell script.
// Tests that mutate this variable must NOT run in parallel (t.Parallel()).
// Production uses the default "ffmpeg", resolved via PATH.
var FFmpegBin = "ffmpeg"

// Stats holds per-stage timing metrics from the decode→model→encode pipeline.
type Stats struct {
	// DecodeFPS is the frame extraction rate (frames decoded per second).
	DecodeFPS float64
	// InferenceFPS is the model throughput (frames upscaled per second).
	InferenceFPS float64
	// EncodeFPS is the encoding rate (frames encoded per second).
	EncodeFPS float64
	// Frames is the total number of frames processed.
	Frames int
}

// fps computes frames per second, guarding against divide-by-zero.
func fps(frames int, elapsed time.Duration) float64 {
	const epsilon = 1e-9
	secs := elapsed.Seconds()
	if secs < epsilon {
		secs = epsilon
	}
	return float64(frames) / secs
}

// Process runs the full decode→model→encode pipeline for a single video segment.
//
// Steps:
//  1. Create two temp dirs under workDir: frames-in-* and frames-out-*
//  2. Run ffmpeg to extract frames from inSegPath into frames-in dir as PPM files
//  3. Count extracted frames
//  4. Run model.Upscale to upscale frames from frames-in to frames-out
//  5. Run ffmpeg to encode upscaled frames into outSegPath as matroska
//  6. Return timing Stats
//
// workDir must exist; use os.TempDir() if no specific directory is required.
func Process(ctx context.Context, inSegPath, outSegPath string, model Model, scale int, workDir string) (Stats, error) {
	// 1. Create temporary frame directories under workDir.
	framesInDir, err := os.MkdirTemp(workDir, "frames-in-")
	if err != nil {
		return Stats{}, fmt.Errorf("pipeline: create frames-in dir: %w", err)
	}
	defer os.RemoveAll(framesInDir) //nolint:errcheck

	framesOutDir, err := os.MkdirTemp(workDir, "frames-out-")
	if err != nil {
		return Stats{}, fmt.Errorf("pipeline: create frames-out dir: %w", err)
	}
	defer os.RemoveAll(framesOutDir) //nolint:errcheck

	// 2. Decode: extract frames from inSegPath as PPM files.
	decodeStart := time.Now()
	outPattern := filepath.Join(framesInDir, "%06d.ppm")
	decodeCmd := exec.CommandContext(ctx, FFmpegBin, "-i", inSegPath, outPattern)
	decodeErrBuf := newRingBuffer(2048)
	decodeCmd.Stderr = decodeErrBuf // bounded capture; ffmpeg diagnostics go to stderr
	if err := decodeCmd.Run(); err != nil {
		return Stats{}, fmt.Errorf("pipeline: ffmpeg decode: %w; output: %s", err, decodeErrBuf.String())
	}
	decodeElapsed := time.Since(decodeStart)

	// 3. Count extracted frames.
	entries, err := os.ReadDir(framesInDir)
	if err != nil {
		return Stats{}, fmt.Errorf("pipeline: read frames-in dir: %w", err)
	}
	frameCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			frameCount++
		}
	}

	// 4. Run model upscale.
	inferStart := time.Now()
	if err := model.Upscale(ctx, framesInDir, framesOutDir, scale); err != nil {
		return Stats{}, fmt.Errorf("pipeline: model upscale: %w", err)
	}
	inferElapsed := time.Since(inferStart)

	// 5. Encode: assemble upscaled frames into outSegPath.
	encodeStart := time.Now()
	inPattern := filepath.Join(framesOutDir, "%06d.ppm")
	encodeCmd := exec.CommandContext(ctx, FFmpegBin,
		"-framerate", "24",
		"-i", inPattern,
		"-c:v", "libx264",
		"-crf", "16",
		"-f", "matroska",
		outSegPath,
	)
	encodeErrBuf := newRingBuffer(2048)
	encodeCmd.Stderr = encodeErrBuf // bounded capture; ffmpeg diagnostics go to stderr
	if err := encodeCmd.Run(); err != nil {
		return Stats{}, fmt.Errorf("pipeline: ffmpeg encode: %w; output: %s", err, encodeErrBuf.String())
	}
	encodeElapsed := time.Since(encodeStart)

	// 6. Compute stats (guard divide-by-zero in fps helper).
	return Stats{
		DecodeFPS:    fps(frameCount, decodeElapsed),
		InferenceFPS: fps(frameCount, inferElapsed),
		EncodeFPS:    fps(frameCount, encodeElapsed),
		Frames:       frameCount,
	}, nil
}
