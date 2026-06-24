package agent

import "os"

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
}

// LoadConfig reads the worker configuration from environment variables.
func LoadConfig() Config {
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "batch"
	}
	return Config{
		ServerURL:   os.Getenv("SERVER_URL"),
		EnrollToken: os.Getenv("ENROLL_TOKEN"),
		Mode:        mode,
	}
}
