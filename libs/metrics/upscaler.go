package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// UpscaleWorkersConnected is the number of workers currently connected,
	// labelled by gpu_model, image_version, and model. worker_id is NOT a
	// label here — use the bounded label set only.
	UpscaleWorkersConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_workers_connected",
		Help: "Number of upscale workers currently connected to the hub.",
	}, []string{"gpu_model", "image_version", "model"})

	// UpscaleLeaseExpiredTotal counts spot preemptions / expired leases that
	// the sweeper has re-leased.
	UpscaleLeaseExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "upscale_lease_expired_total",
		Help: "Total number of segment leases expired and re-leased by the sweeper (spot preemptions).",
	})

	// UpscaleCommandTotal counts issued control-plane commands by type.
	UpscaleCommandTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "upscale_command_total",
		Help: "Total number of control-plane commands issued to workers, by command type.",
	}, []string{"type"})

	// UpscaleEnrollTotal counts worker enroll attempts by result.
	UpscaleEnrollTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "upscale_enroll_total",
		Help: "Total number of worker enroll attempts, by result (ok|bad_token|error).",
	}, []string{"result"})

	// UpscaleEdgeRequestsTotal counts requests arriving at the edge (ext.) by
	// path and HTTP status.
	UpscaleEdgeRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "upscale_edge_requests_total",
		Help: "Total number of requests received at the upscaler edge, by path and status.",
	}, []string{"path", "status"})

	// UpscaleSegmentDuration records per-segment pipeline stage durations.
	// stage values: download, decode, inference, encode, upload.
	// worker_id is NOT a label (cardinality discipline).
	UpscaleSegmentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "upscale_segment_duration_seconds",
		Help:    "Duration of each upscale pipeline stage per segment, in seconds.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	}, []string{"stage"})

	// UpscaleJobProgressRatio is the fraction of segments completed for the
	// currently active job (0.0–1.0).
	UpscaleJobProgressRatio = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "upscale_job_progress_ratio",
		Help: "Progress of the active upscale job as a fraction of segments completed (0–1).",
	})

	// UpscaleJobEtaSeconds is the estimated seconds remaining for the active job.
	UpscaleJobEtaSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "upscale_job_eta_seconds",
		Help: "Estimated seconds remaining for the active upscale job.",
	})

	// UpscaleQueueDepth is the number of jobs in each queue status bucket.
	UpscaleQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_queue_depth",
		Help: "Number of upscale jobs per status (queued, upscaling, finalizing, done, failed).",
	}, []string{"status"})

	// Worker-reported performance gauges. worker_id is NOT a label.

	// UpscaleWorkerGPUUtil is the instantaneous GPU utilisation reported by workers.
	UpscaleWorkerGPUUtil = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_worker_gpu_util",
		Help: "GPU utilisation (0–1) reported by upscale workers.",
	}, []string{"gpu_model", "image_version"})

	// UpscaleWorkerVRAMUsedBytes is the VRAM consumption reported by workers.
	UpscaleWorkerVRAMUsedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_worker_vram_used_bytes",
		Help: "VRAM used in bytes reported by upscale workers.",
	}, []string{"gpu_model", "image_version"})

	// UpscaleDecodeFPS is the video-decode throughput reported by workers.
	UpscaleDecodeFPS = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_decode_fps",
		Help: "Video decode throughput (frames/s) reported by upscale workers.",
	}, []string{"gpu_model", "image_version"})

	// UpscaleInferenceFPS is the AI inference throughput reported by workers.
	UpscaleInferenceFPS = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_inference_fps",
		Help: "AI inference throughput (frames/s) reported by upscale workers.",
	}, []string{"gpu_model", "image_version"})

	// UpscaleEncodeFPS is the video-encode throughput reported by workers.
	UpscaleEncodeFPS = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "upscale_encode_fps",
		Help: "Video encode throughput (frames/s) reported by upscale workers.",
	}, []string{"gpu_model", "image_version"})
)

// RecordWorkerTelemetry updates the worker-reported performance gauges.
// Takes primitives — callers must unpack protocol.MetricsPayload themselves
// so libs/metrics never imports a service-internal package.
func RecordWorkerTelemetry(gpuModel, imageVersion string, gpuUtil, vramBytes, decodeFPS, inferenceFPS, encodeFPS float64) {
	labels := prometheus.Labels{"gpu_model": gpuModel, "image_version": imageVersion}
	UpscaleWorkerGPUUtil.With(labels).Set(gpuUtil)
	UpscaleWorkerVRAMUsedBytes.With(labels).Set(vramBytes)
	UpscaleDecodeFPS.With(labels).Set(decodeFPS)
	UpscaleInferenceFPS.With(labels).Set(inferenceFPS)
	UpscaleEncodeFPS.With(labels).Set(encodeFPS)
}
