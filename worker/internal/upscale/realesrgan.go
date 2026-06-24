package upscale

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
)

func init() {
	const defaultBin = "realesrgan-ncnn-vulkan"
	Register(newRealesrgan("realtime", "realesr-animevideov3", defaultBin))
	Register(newRealesrgan("best-quality", "realesrgan-x4plus-anime", defaultBin))
}

// realesrganModel wraps the realesrgan-ncnn-vulkan command-line tool.
// The injectable bin field allows tests to substitute a fake shell script.
type realesrganModel struct {
	name      string // registry key: "realtime" or "best-quality"
	modelName string // -n flag value: "realesr-animevideov3" or "realesrgan-x4plus-anime"
	bin       string // path to realesrgan-ncnn-vulkan (default: "realesrgan-ncnn-vulkan")
}

// newRealesrgan constructs a realesrganModel. Used both in init() and in tests.
func newRealesrgan(name, modelName, bin string) *realesrganModel {
	if bin == "" {
		bin = "realesrgan-ncnn-vulkan"
	}
	return &realesrganModel{name: name, modelName: modelName, bin: bin}
}

func (r *realesrganModel) Name() string { return r.name }

// Upscale shells out to realesrgan-ncnn-vulkan:
//
//	realesrgan-ncnn-vulkan -i {framesDir} -o {outDir} -s {scale} -n {modelName}
//
// A bounded 2048-byte ring buffer captures stderr for error messages.
func (r *realesrganModel) Upscale(ctx context.Context, framesDir, outDir string, scale int) error {
	args := []string{
		"-i", framesDir,
		"-o", outDir,
		"-s", strconv.Itoa(scale),
		"-n", r.modelName,
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
