package agent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the worker agent configuration loaded from environment variables.
type Config struct {
	// ServerURL is the base URL of the upscaler control-plane server
	// (e.g. "http://upscaler:8095"). Read from env SERVER_URL.
	ServerURL string

	// EnrollToken is the one-time enrollment token issued by the server operator.
	// Read from env ENROLL_TOKEN.
	EnrollToken string

	// Mode controls the worker's operating mode. Defaults to "batch".
	// Read from env MODE.
	Mode string

	// APIKey is the optional edge API key sent as X-API-Key on data-plane
	// segment GET/PUT requests. Read from env API_KEY. When CF mTLS is
	// adopted the client cert will replace/augment this (CD-9).
	APIKey string

	// PreinstalledModels is the list of realesrgan model names baked into the
	// worker image and available without any download. Read from env
	// PREINSTALLED_MODELS (comma-separated, e.g. "realtime,best-quality").
	// Spaces around names are trimmed; empty tokens are dropped.
	// Unset or empty → nil (the worker boots with only the built-in mock).
	PreinstalledModels []string

	// ModelsDir is the directory that holds realesrgan model weight files
	// ({name}.param + {name}.bin). It is BOTH the lookup dir for
	// PreinstalledModels (baked into the image) AND the extraction target for
	// pull-on-demand Install (T29) — preinstalled and pulled weights share one
	// directory. Read from env MODELS_DIR; defaults to "/models" (the directory
	// the worker image provisions). The runtime user must be able to write here
	// for pull-on-demand to install pulled models.
	ModelsDir string

	// WorkDir is the directory where temporary frame files are written during
	// processing. Read from env WORK_DIR; defaults to os.TempDir().
	WorkDir string

	// Scale is the integer upscale factor (e.g. 2 or 4).
	// Read from env SCALE; defaults to 2.
	Scale int

	// HeartbeatInterval / MetricsInterval override the per-segment Telemetry
	// cadence. Read from env HEARTBEAT_INTERVAL / METRICS_INTERVAL (Go duration
	// strings, e.g. "5s", "200ms"); zero/unset leaves the Client defaults
	// (5s / 10s) in place. Exposed primarily so the e2e integration test can
	// drive fast emission without waiting on production cadences.
	HeartbeatInterval time.Duration
	MetricsInterval   time.Duration
}

// LoadConfig reads the worker configuration from environment variables.
func LoadConfig() Config {
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "batch"
	}
	workDir := os.Getenv("WORK_DIR")
	if workDir == "" {
		workDir = os.TempDir()
	}
	modelsDir := os.Getenv("MODELS_DIR")
	if modelsDir == "" {
		modelsDir = "/models"
	}
	scale := 2
	if s := os.Getenv("SCALE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			scale = n
		}
	}
	var hbInterval, metInterval time.Duration
	if s := os.Getenv("HEARTBEAT_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			hbInterval = d
		}
	}
	if s := os.Getenv("METRICS_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			metInterval = d
		}
	}

	// Parse PREINSTALLED_MODELS: comma-separated names, trimmed, empties dropped.
	var preinstalled []string
	if raw := os.Getenv("PREINSTALLED_MODELS"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			name := strings.TrimSpace(part)
			if name != "" {
				preinstalled = append(preinstalled, name)
			}
		}
	}

	return Config{
		ServerURL:          os.Getenv("SERVER_URL"),
		EnrollToken:        os.Getenv("ENROLL_TOKEN"),
		Mode:               mode,
		APIKey:             os.Getenv("API_KEY"),
		PreinstalledModels: preinstalled,
		ModelsDir:          modelsDir,
		WorkDir:            workDir,
		Scale:              scale,
		HeartbeatInterval:  hbInterval,
		MetricsInterval:    metInterval,
	}
}
