package agent

import (
	"context"
	"fmt"

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
// The Stats fields BytesRead and BytesWritten are set to int64(frames) as a
// pragmatic proxy for throughput reporting (the caller cares about frame counts,
// not raw byte sizes, at this layer).
func (p *PipelineProcessor) Process(ctx context.Context, inSeg, outSeg string) (Stats, error) {
	pStats, err := upscale.Process(ctx, inSeg, outSeg, p.model, p.scale, p.workDir)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		BytesRead:    int64(pStats.Frames),
		BytesWritten: int64(pStats.Frames),
	}, nil
}
