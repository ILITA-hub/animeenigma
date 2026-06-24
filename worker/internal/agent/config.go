package agent

import (
	"os"
	"strconv"
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

	// Model is the name of the upscale model to use from the registry.
	// Read from env MODEL; defaults to "mock".
	Model string

	// WorkDir is the directory where temporary frame files are written during
	// processing. Read from env WORK_DIR; defaults to os.TempDir().
	WorkDir string

	// Scale is the integer upscale factor (e.g. 2 or 4).
	// Read from env SCALE; defaults to 2.
	Scale int
}

// LoadConfig reads the worker configuration from environment variables.
func LoadConfig() Config {
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "batch"
	}
	model := os.Getenv("MODEL")
	if model == "" {
		model = "mock"
	}
	workDir := os.Getenv("WORK_DIR")
	if workDir == "" {
		workDir = os.TempDir()
	}
	scale := 2
	if s := os.Getenv("SCALE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			scale = n
		}
	}
	return Config{
		ServerURL:   os.Getenv("SERVER_URL"),
		EnrollToken: os.Getenv("ENROLL_TOKEN"),
		Mode:        mode,
		APIKey:      os.Getenv("API_KEY"),
		Model:       model,
		WorkDir:     workDir,
		Scale:       scale,
	}
}
