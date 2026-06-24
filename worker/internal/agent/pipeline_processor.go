package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/ILITA-hub/animeenigma/worker/internal/upscale"
)

// PipelineProcessor implements Processor using the full decode→model→encode
// pipeline (upscale.Process). It replaces the Task-15 CopyProcessor stub with
// the real upscaling pipeline.
type PipelineProcessor struct {
	model   upscale.Model
	scale   int
	workDir string
}

// NewPipelineProcessor constructs a PipelineProcessor that looks up the named
// model in the upscale registry. Returns an error if the model is not found.
//
// scale is the integer upscale factor (e.g. 2 or 4).
// workDir is the directory where temporary frame files are written; pass
// os.TempDir() when no specific directory is required.
func NewPipelineProcessor(modelName string, scale int, workDir string) (*PipelineProcessor, error) {
	m, err := upscale.Get(modelName)
	if err != nil {
		return nil, fmt.Errorf("pipeline_processor: %w", err)
	}
	return &PipelineProcessor{model: m, scale: scale, workDir: workDir}, nil
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

	return Stats{
		BytesRead:    bytesRead,
		BytesWritten: bytesWritten,
		DecodeFPS:    pStats.DecodeFPS,
		InferenceFPS: pStats.InferenceFPS,
		EncodeFPS:    pStats.EncodeFPS,
		Frames:       pStats.Frames,
	}, nil
}
