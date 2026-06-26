package agent

import (
	"context"
	"os"
	"sync/atomic"

	"github.com/ILITA-hub/animeenigma/worker/internal/upscale"
)

// PipelineProcessor implements Processor using the full decode→model→encode
// pipeline (upscale.Process). It replaces the Task-15 CopyProcessor stub with
// the real upscaling pipeline.
type PipelineProcessor struct {
	model   upscale.Model
	scale   int
	workDir string

	// live holds the most recent *Stats produced by Process, published
	// atomically so Telemetry's statsSource can read fps without a lock while
	// Process runs concurrently. nil until the first segment completes.
	live atomic.Pointer[Stats]
}

// Process runs the full decode→model→encode pipeline for a single video segment.
//
// BytesRead and BytesWritten are set from the actual file sizes of inSeg and
// outSeg respectively. Pipeline fps metrics are surfaced from the upscale.Stats.
func (p *PipelineProcessor) Process(ctx context.Context, inSeg, outSeg string) (Stats, error) {
	pStats, err := upscale.Process(ctx, inSeg, outSeg, p.model, p.scale, p.workDir)
	if err != nil {
		return Stats{}, err
	}

	var bytesRead, bytesWritten int64
	if fi, err := os.Stat(inSeg); err == nil {
		bytesRead = fi.Size()
	}
	if fi, err := os.Stat(outSeg); err == nil {
		bytesWritten = fi.Size()
	}

	st := Stats{
		BytesRead:    bytesRead,
		BytesWritten: bytesWritten,
		DecodeFPS:    pStats.DecodeFPS,
		InferenceFPS: pStats.InferenceFPS,
		EncodeFPS:    pStats.EncodeFPS,
		Frames:       pStats.Frames,
	}
	// Publish the latest measured throughput so Telemetry.statsSource can
	// surface real fps on subsequent metrics frames.
	p.live.Store(&st)
	return st, nil
}

// LiveStats implements StatsSource: it returns the most recent Stats measured by
// Process (zero value until the first segment completes). Safe for concurrent
// use with Process — reads an atomically-published pointer.
func (p *PipelineProcessor) LiveStats() Stats {
	if s := p.live.Load(); s != nil {
		return *s
	}
	return Stats{}
}
