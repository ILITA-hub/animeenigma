// Package upscale provides a pluggable registry of upscale model implementations.
// Each model is registered via its init() function and can be retrieved by name.
package upscale

import (
	"context"
	"fmt"
)

// Model is the abstraction over a frame-upscale backend.
// Implementations run the upscaling tool over the frames in framesDir,
// writing same-named output files into outDir.
type Model interface {
	// Name returns the unique registry key for this model (e.g. "mock", "realtime").
	Name() string
	// Upscale processes all frame files in framesDir and writes upscaled versions
	// into outDir with the same filenames. scale is the integer upscale factor
	// (e.g. 2 or 4). ctx cancellation must be respected.
	Upscale(ctx context.Context, framesDir, outDir string, scale int) error
}

// registry holds all registered Model implementations, keyed by Name().
var registry = map[string]Model{}

// Register adds m to the global registry. It is called from init() in
// each model's source file (mock.go, realesrgan.go, …).
// Panics if a model with the same name has already been registered — this
// catches accidental double-registration at startup rather than silently
// overwriting a model.
func Register(m Model) {
	if _, exists := registry[m.Name()]; exists {
		panic("upscale: model already registered: " + m.Name())
	}
	registry[m.Name()] = m
}

// Get returns the registered Model with the given name, or an error if
// no model with that name has been registered.
func Get(name string) (Model, error) {
	m, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("upscale: unknown model %q", name)
	}
	return m, nil
}
