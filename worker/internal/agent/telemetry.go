package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// Telemetry periodically emits heartbeat and metrics frames over the send channel.
// It is injectable for testing: set NvidiaSmiPath to a fake script, and provide
// a statsSource that returns the latest pipeline stats.
type Telemetry struct {
	// send is the outbound frame channel shared with the WS write pump.
	send chan<- []byte

	// heartbeatInterval controls how often heartbeat frames are emitted.
	heartbeatInterval time.Duration

	// metricsInterval controls how often metrics frames are emitted.
	metricsInterval time.Duration

	// statsSource returns the latest pipeline Stats for fps fields.
	// May be nil (defaults to zero Stats).
	statsSource func() Stats

	// NvidiaSmiPath is the path to the nvidia-smi binary.
	// Defaults to "nvidia-smi" (resolved via PATH).
	// Override in tests to point at a fake script.
	NvidiaSmiPath string
}

// NewTelemetry constructs a Telemetry instance with the given parameters.
func NewTelemetry(
	send chan<- []byte,
	heartbeatInterval, metricsInterval time.Duration,
	statsSource func() Stats,
) *Telemetry {
	return &Telemetry{
		send:              send,
		heartbeatInterval: heartbeatInterval,
		metricsInterval:   metricsInterval,
		statsSource:       statsSource,
		NvidiaSmiPath:     "nvidia-smi",
	}
}

// Run starts the telemetry loop. It emits heartbeat frames on heartbeatInterval
// and metrics frames on metricsInterval. Runs until ctx is cancelled.
func (t *Telemetry) Run(ctx context.Context, jobID string, segIdx int) {
	hbTicker := time.NewTicker(t.heartbeatInterval)
	defer hbTicker.Stop()
	metTicker := time.NewTicker(t.metricsInterval)
	defer metTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hbTicker.C:
			t.sendHeartbeat(jobID, segIdx)
		case <-metTicker.C:
			t.sendMetrics()
		}
	}
}

// sendHeartbeat emits a heartbeat frame on the send channel.
func (t *Telemetry) sendHeartbeat(jobID string, segIdx int) {
	f, err := wire.NewFrame("heartbeat", 0, wire.HeartbeatPayload{
		JobID:       jobID,
		SegmentIdx:  segIdx,
		ProgressPct: 0,
		ETASeconds:  0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: build heartbeat frame: %v\n", err)
		return
	}
	raw, err := json.Marshal(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: marshal heartbeat frame: %v\n", err)
		return
	}
	select {
	case t.send <- raw:
	default:
		// Drop if channel is full — telemetry is best-effort.
	}
}

// sendMetrics collects GPU stats and emits a metrics frame.
func (t *Telemetry) sendMetrics() {
	payload := t.collectMetrics()

	f, err := wire.NewFrame("metrics", 0, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: build metrics frame: %v\n", err)
		return
	}
	raw, err := json.Marshal(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: marshal metrics frame: %v\n", err)
		return
	}
	select {
	case t.send <- raw:
	default:
		// Drop if channel is full — telemetry is best-effort.
	}
}

// collectMetrics runs nvidia-smi and parses the output into a MetricsPayload.
// Falls back to rocm-smi or zero values if nvidia-smi fails.
func (t *Telemetry) collectMetrics() wire.MetricsPayload {
	payload := wire.MetricsPayload{
		ImageVersion: os.Getenv("IMAGE_VERSION"),
	}

	// Surface pipeline fps from statsSource.
	if t.statsSource != nil {
		s := t.statsSource()
		payload.DecodeFPS = s.DecodeFPS
		payload.InferenceFPS = s.InferenceFPS
		payload.EncodeFPS = s.EncodeFPS
	}

	// Try nvidia-smi.
	smiPath := t.NvidiaSmiPath
	if smiPath == "" {
		smiPath = "nvidia-smi"
	}

	out, err := exec.Command(smiPath,
		"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		// nvidia-smi unavailable — try rocm-smi (AMD GPUs).
		// For simplicity, just return zeros on failure.
		return payload
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		return payload
	}

	fields := strings.SplitN(line, ",", 6)
	if len(fields) < 6 {
		return payload
	}

	payload.GPUModel = strings.TrimSpace(fields[0])

	if v, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64); err == nil {
		payload.GPUUtil = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64); err == nil {
		payload.VRAMUsedBytes = v * 1024 * 1024
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64); err == nil {
		payload.VRAMTotalBytes = v * 1024 * 1024
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64); err == nil {
		payload.GPUTempC = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64); err == nil {
		payload.GPUPowerW = v
	}

	return payload
}
