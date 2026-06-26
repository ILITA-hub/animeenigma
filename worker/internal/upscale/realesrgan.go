package upscale

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
)

// realesrganModel wraps the realesrgan-ncnn-vulkan command-line tool.
// The injectable bin field allows tests to substitute a fake shell script.
type realesrganModel struct {
	name      string // registry key: "realtime" or "best-quality"
	modelName string // -n flag value: "realesr-animevideov3" or "realesrgan-x4plus-anime"
	bin       string // path to realesrgan-ncnn-vulkan (default: "realesrgan-ncnn-vulkan")
	modelsDir string // -m flag value: directory containing .param/.bin weight files; empty = runtime default
}

// newRealesrgan constructs a realesrganModel. Used both in init() and in tests.
// modelsDir is passed as -m to the runtime; empty means the runtime resolves
// weights from its own default path (usually a "models/" subdir beside the binary).
func newRealesrgan(name, modelName, bin, modelsDir string) *realesrganModel {
	if bin == "" {
		bin = "realesrgan-ncnn-vulkan"
	}
	return &realesrganModel{name: name, modelName: modelName, bin: bin, modelsDir: modelsDir}
}

func (r *realesrganModel) Name() string { return r.name }

// Upscale shells out to realesrgan-ncnn-vulkan:
//
//	realesrgan-ncnn-vulkan -i {framesDir} -o {outDir} -s {scale} -n {modelName} [-m {modelsDir}]
//
// -m is only appended when modelsDir is non-empty so that the init()-registered
// built-in models do not force a specific search path.
// A bounded 2048-byte ring buffer captures stderr for error messages.
func (r *realesrganModel) Upscale(ctx context.Context, framesDir, outDir string, scale int) error {
	args := []string{
		"-i", framesDir,
		"-o", outDir,
		"-s", strconv.Itoa(scale),
		"-n", r.modelName,
	}
	if r.modelsDir != "" {
		args = append(args, "-m", r.modelsDir)
	}

	cmd := exec.CommandContext(ctx, r.bin, args...)
	ring := newRingBuffer(2048)
	cmd.Stderr = ring

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("realesrgan: %s: exit %w; stderr: %s", r.modelName, err, ring.String())
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ring buffer (bounded stderr capture) — local copy to keep this package
// self-contained. Mirrors the implementation in services/upscaler/internal/ffmpeg.
// ---------------------------------------------------------------------------

type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newRingBuffer(cap int) *ringBuffer {
	if cap <= 0 {
		cap = 2048
	}
	return &ringBuffer{cap: cap}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.buf = append(rb.buf, p...)
	if len(rb.buf) > rb.cap {
		rb.buf = append([]byte(nil), rb.buf[len(rb.buf)-rb.cap:]...)
	}
	return len(p), nil
}

func (rb *ringBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return string(rb.buf)
}
