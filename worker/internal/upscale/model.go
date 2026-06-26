// Package upscale provides the upscale model implementations (the built-in
// pure-Go mock plus realesrgan-ncnn-vulkan-backed models) and the Manager that
// registers and resolves them at runtime — see manager.go. Each model satisfies
// the Model interface below.
package upscale

import "context"

// Model is the abstraction over a frame-upscale backend.
// Implementations run the upscaling tool over the frames in framesDir,
// writing same-named output files into outDir.
type Model interface {
	// Name returns the unique key for this model (e.g. "mock", "realtime").
	Name() string
	// Upscale processes all frame files in framesDir and writes upscaled versions
	// into outDir with the same filenames. scale is the integer upscale factor
	// (e.g. 2 or 4). ctx cancellation must be respected.
	Upscale(ctx context.Context, framesDir, outDir string, scale int) error
}
