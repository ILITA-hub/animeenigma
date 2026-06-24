package agent

import (
	"context"
	"io"
	"os"
)

// Stats holds basic metrics reported by a Processor after it finishes a segment.
type Stats struct {
	// BytesRead is the size of the input segment in bytes.
	BytesRead int64
	// BytesWritten is the size of the output segment in bytes.
	BytesWritten int64
	// DecodeFPS is the frame extraction rate (frames decoded per second).
	DecodeFPS float64
	// InferenceFPS is the model throughput (frames upscaled per second).
	InferenceFPS float64
	// EncodeFPS is the encoding rate (frames encoded per second).
	EncodeFPS float64
	// Frames is the total number of frames processed.
	Frames int
}

// Processor is the DI seam for the segment processing pipeline.
// Task 15 ships a stub implementation (CopyProcessor). The real
// decode→model→encode pipeline arrives in Task 17.
//
// The worker is intentionally non-functional end-to-end until Task 17
// wires the real pipeline via a concrete Processor.
type Processor interface {
	// Process reads the segment from inSeg path, runs the upscale pipeline,
	// and writes the result to outSeg path. Both paths are local files.
	// The caller owns inSeg and outSeg lifetimes; Process must not delete them.
	Process(ctx context.Context, inSeg, outSeg string) (Stats, error)
}

// StatsSource is an optional capability a Processor may implement to expose its
// latest measured pipeline Stats (DecodeFPS/InferenceFPS/EncodeFPS) for
// telemetry. The lease loop feeds LiveStats into Telemetry so metrics frames
// carry real fps. A Processor that does not implement this contributes zero fps
// (Telemetry still emits GPU/heartbeat data).
type StatsSource interface {
	// LiveStats returns the most recent Stats observed by this Processor.
	// It must be safe to call concurrently with Process.
	LiveStats() Stats
}

// CopyProcessor is the stub Processor shipped in Task 15.
// It copies the input file to the output file verbatim, allowing the full
// lease-loop machinery (download → process → upload → delete) to be exercised
// before the real pipeline is available.
//
// NOTE: the worker is NOT functionally upscaling until Task 17 replaces this
// stub with a concrete decode→model→encode Processor.
type CopyProcessor struct{}

// Process copies inSeg → outSeg and returns byte counts.
func (CopyProcessor) Process(_ context.Context, inSeg, outSeg string) (Stats, error) {
	in, err := os.Open(inSeg)
	if err != nil {
		return Stats{}, err
	}
	defer in.Close()

	out, err := os.Create(outSeg)
	if err != nil {
		return Stats{}, err
	}
	defer out.Close()

	n, err := io.Copy(out, in)
	if err != nil {
		return Stats{}, err
	}
	return Stats{BytesRead: n, BytesWritten: n}, nil
}
