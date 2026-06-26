package agent

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

const approxTol = 1e-3

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < approxTol
}

// drainFrames reads frames from ch until the deadline or until pred returns true.
// Returns the first matching frame if found.
func drainFrames(t *testing.T, ch <-chan []byte, deadline time.Duration, frameType string, pred func(wire.Frame) bool) (wire.Frame, bool) {
	t.Helper()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return wire.Frame{}, false
		case raw, ok := <-ch:
			if !ok {
				return wire.Frame{}, false
			}
			var f wire.Frame
			if err := json.Unmarshal(raw, &f); err != nil {
				continue
			}
			if f.Type != frameType {
				continue
			}
			if pred == nil || pred(f) {
				return f, true
			}
		}
	}
}

// writeFakeNvidiaSmi writes a fake nvidia-smi shell script that prints the given
// CSV line to stdout and exits 0. Returns the path to the script.
func writeFakeNvidiaSmi(t *testing.T, output string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "nvidia-smi")
	content := "#!/bin/sh\necho '" + output + "'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake nvidia-smi: %v", err)
	}
	return script
}

// TestTelemetry_MetricsFrameFromFakeNvidiaSmi verifies that Telemetry correctly
// parses nvidia-smi CSV output and fills MetricsPayload fields.
func TestTelemetry_MetricsFrameFromFakeNvidiaSmi(t *testing.T) {
	t.Parallel()

	smiPath := writeFakeNvidiaSmi(t, "NVIDIA GeForce RTX 4090, 85, 16384, 24576, 75, 320.00")

	ch := make(chan []byte, 64)
	tel := NewTelemetry(ch, 1*time.Second, 20*time.Millisecond, nil)
	tel.NvidiaSmiPath = smiPath

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go tel.Run(ctx, "test-job", 0)

	frame, found := drainFrames(t, ch, 500*time.Millisecond, "metrics", nil)
	if !found {
		t.Fatal("expected metrics frame within 500ms")
	}

	var payload wire.MetricsPayload
	if err := frame.Decode(&payload); err != nil {
		t.Fatalf("decode MetricsPayload: %v", err)
	}

	if payload.GPUModel != "NVIDIA GeForce RTX 4090" {
		t.Errorf("GPUModel: got %q, want %q", payload.GPUModel, "NVIDIA GeForce RTX 4090")
	}
	if !approxEqual(payload.GPUUtil, 85.0) {
		t.Errorf("GPUUtil: got %v, want 85.0", payload.GPUUtil)
	}
	wantVRAMUsed := float64(16384) * 1024 * 1024
	if !approxEqual(payload.VRAMUsedBytes, wantVRAMUsed) {
		t.Errorf("VRAMUsedBytes: got %v, want %v", payload.VRAMUsedBytes, wantVRAMUsed)
	}
	if !approxEqual(payload.GPUTempC, 75.0) {
		t.Errorf("GPUTempC: got %v, want 75.0", payload.GPUTempC)
	}
	if !approxEqual(payload.GPUPowerW, 320.0) {
		t.Errorf("GPUPowerW: got %v, want 320.0", payload.GPUPowerW)
	}
}

// TestTelemetry_HeartbeatFrameEmitted verifies that a heartbeat frame is emitted
// within the expected time window with the correct JobID.
func TestTelemetry_HeartbeatFrameEmitted(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	tel := NewTelemetry(ch, 20*time.Millisecond, 1*time.Second, nil)
	// Use a non-existent path so metrics never fire (avoid race on smi call).
	tel.NvidiaSmiPath = "/nonexistent/nvidia-smi"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const jobID = "hb-job-42"
	go tel.Run(ctx, jobID, 3)

	frame, found := drainFrames(t, ch, 200*time.Millisecond, "heartbeat", nil)
	if !found {
		t.Fatal("expected heartbeat frame within 200ms")
	}

	var payload wire.HeartbeatPayload
	if err := frame.Decode(&payload); err != nil {
		t.Fatalf("decode HeartbeatPayload: %v", err)
	}
	if payload.JobID != jobID {
		t.Errorf("JobID: got %q, want %q", payload.JobID, jobID)
	}
	if payload.SegmentIdx != 3 {
		t.Errorf("SegmentIdx: got %d, want 3", payload.SegmentIdx)
	}
}

// TestTelemetry_FallbackToZerosOnMissingSmi verifies that when nvidia-smi is not
// available, a metrics frame is still emitted with GPUUtil==0.0 (no crash).
func TestTelemetry_FallbackToZerosOnMissingSmi(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	tel := NewTelemetry(ch, 1*time.Second, 20*time.Millisecond, nil)
	tel.NvidiaSmiPath = "/nonexistent/nvidia-smi"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go tel.Run(ctx, "fallback-job", 0)

	frame, found := drainFrames(t, ch, 500*time.Millisecond, "metrics", nil)
	if !found {
		t.Fatal("expected metrics frame even with missing nvidia-smi")
	}

	var payload wire.MetricsPayload
	if err := frame.Decode(&payload); err != nil {
		t.Fatalf("decode MetricsPayload: %v", err)
	}
	if payload.GPUUtil != 0.0 {
		t.Errorf("GPUUtil: got %v, want 0.0 (fallback zeros)", payload.GPUUtil)
	}
}

// TestTelemetry_PipelineFpsFromStatsSource verifies that fps values from the
// statsSource function are reflected in the metrics payload.
func TestTelemetry_PipelineFpsFromStatsSource(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	statsSource := func() Stats {
		return Stats{
			DecodeFPS:    12.5,
			InferenceFPS: 6.0,
			EncodeFPS:    11.0,
		}
	}
	tel := NewTelemetry(ch, 1*time.Second, 20*time.Millisecond, statsSource)
	tel.NvidiaSmiPath = "/nonexistent/nvidia-smi"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go tel.Run(ctx, "fps-job", 0)

	frame, found := drainFrames(t, ch, 500*time.Millisecond, "metrics", nil)
	if !found {
		t.Fatal("expected metrics frame within 500ms")
	}

	var payload wire.MetricsPayload
	if err := frame.Decode(&payload); err != nil {
		t.Fatalf("decode MetricsPayload: %v", err)
	}
	if !approxEqual(payload.DecodeFPS, 12.5) {
		t.Errorf("DecodeFPS: got %v, want 12.5", payload.DecodeFPS)
	}
	if !approxEqual(payload.InferenceFPS, 6.0) {
		t.Errorf("InferenceFPS: got %v, want 6.0", payload.InferenceFPS)
	}
	if !approxEqual(payload.EncodeFPS, 11.0) {
		t.Errorf("EncodeFPS: got %v, want 11.0", payload.EncodeFPS)
	}
}
