package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

type Config struct {
	Server   ServerConfig
	Redis    cache.Config
	Database database.Config

	CatalogURL   string        // internal catalog base (membership, structure, streams)
	GatewayURL   string        // public gateway base — ffmpeg reads hls-proxy through it
	Interval     time.Duration // pause between probes (after each probe completes)
	Workers      int           // concurrent in-process probe loops (clamped 1..4)
	UnitBudget   time.Duration // hard per-unit budget; may exceed Interval (pause runs after the probe)
	ReprobeTTL   time.Duration // verified/inconclusive re-probe age
	FFmpegPath   string
	PythonPath   string
	AnalyzersDir string
	WorkDir      string
	WorkerOn     bool

	// Pins is the parsed CV_PIN_ANIME operator directive: anime UUID →
	// preferred provider ("" = whole-title pin). Format:
	// "uuid[:provider],uuid2[:provider2]". Pinned titles rank above any
	// organic score, bypass cooldowns, and plan the preferred provider's
	// skip family first — a temporary lever for "probe THIS now".
	Pins map[string]string

	// Banded prioritization (spec §3). BandWeights is the per-claim lottery
	// [Band1, Band2, Band3]; FreshWindow is the ± window on next_episode_at that
	// floats a just-aired ongoing to the front of Band 1; IdleCooldown is the
	// long cooldown a settled idle-backfill title gets; IdleWindow pages the tail.
	BandWeights  [3]int
	FreshWindow  time.Duration
	IdleCooldown time.Duration
	IdleWindow   int

	SkipEnabled      bool          // gate for the OP/ED skip-probe lane, once the verify lane is settled
	SkipBudget       time.Duration // hard per-skip-task budget (locate or pair)
	SkipHeadWindow   time.Duration // head-of-episode audio window extracted for OP matching
	SkipTailWindow   time.Duration // tail-of-episode audio window extracted for ED matching
	SkipMinMatch     time.Duration // shortest accepted OP/ED match length
	SkipMaxMatch     time.Duration // longest accepted OP/ED match length
	SkipSimThreshold float64       // opskip analyzer similarity threshold, 0..1
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8101)},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		Database: database.Config{
			Host: getEnv("DB_HOST", "localhost"), Port: getEnvInt("DB_PORT", 5432),
			User: getEnv("DB_USER", "postgres"), Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"), SSLMode: getEnv("DB_SSLMODE", "disable"),
		},
		CatalogURL: getEnv("CV_CATALOG_URL", "http://catalog:8081"),
		GatewayURL: getEnv("CV_GATEWAY_URL", "http://gateway:8000"),
		Interval:   getEnvDuration("CV_INTERVAL", 10*time.Second),
		Workers:    clampWorkers(getEnvInt("CV_WORKERS", 2)),
		// 240s: 120s browser-engine stream resolve + fragment pulls + whisper.
		// Live-E2E measured 2026-07-17 (spec §2 revisit): 50s starved every
		// real (non-synth) unit — resolve alone exceeded it.
		UnitBudget:   getEnvDuration("CV_UNIT_BUDGET", 240*time.Second),
		ReprobeTTL:   getEnvDuration("CV_REPROBE_TTL", 720*time.Hour),
		FFmpegPath:   getEnv("CV_FFMPEG_PATH", "ffmpeg"),
		PythonPath:   getEnv("CV_PYTHON", "python3"),
		AnalyzersDir: getEnv("CV_ANALYZERS_DIR", "/app/analyzers"),
		WorkDir:      getEnv("CV_WORKDIR", "/tmp/cv"),
		WorkerOn:     getEnv("CV_WORKER_ENABLED", "true") != "false",
		Pins:         parsePins(getEnv("CV_PIN_ANIME", "")),

		BandWeights:  parseBandWeights(getEnv("CV_BAND_WEIGHTS", "")),
		FreshWindow:  getEnvDuration("CV_FRESH_WINDOW", 48*time.Hour),
		IdleCooldown: getEnvDuration("CV_IDLE_COOLDOWN", 168*time.Hour),
		IdleWindow:   getEnvInt("CV_IDLE_WINDOW", 100),

		SkipEnabled:      getEnv("CV_SKIP_ENABLED", "true") != "false",
		SkipBudget:       getEnvDuration("CV_SKIP_BUDGET", 480*time.Second),
		SkipHeadWindow:   getEnvDuration("CV_SKIP_HEAD_WINDOW", 480*time.Second),
		SkipTailWindow:   getEnvDuration("CV_SKIP_TAIL_WINDOW", 480*time.Second),
		SkipMinMatch:     getEnvDuration("CV_SKIP_MIN_MATCH", 50*time.Second),
		SkipMaxMatch:     getEnvDuration("CV_SKIP_MAX_MATCH", 150*time.Second),
		SkipSimThreshold: getEnvFloat("CV_SKIP_SIM_THRESHOLD", 0.75),
	}
	if cfg.Interval < 10*time.Second {
		return nil, fmt.Errorf("CV_INTERVAL too small: %s", cfg.Interval)
	}
	return cfg, nil
}

// clampWorkers silently clamps CV_WORKERS to [1,4] — the in-process probe
// pool has no supervisor to report a misconfiguration to, so out-of-range
// values are corrected rather than rejected (unlike the Interval floor,
// which errors: a too-small interval is a resource-exhaustion risk, an
// out-of-range worker count is not).
func clampWorkers(n int) int {
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

// parsePins parses CV_PIN_ANIME ("uuid[:provider],uuid2") into the
// animeID → preferred-provider map. Malformed pieces (empty after trim) are
// dropped silently — an operator env, not user input.
func parsePins(s string) map[string]string {
	pins := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, provider, _ := strings.Cut(part, ":")
		if id = strings.TrimSpace(id); id != "" {
			pins[id] = strings.TrimSpace(provider)
		}
	}
	return pins
}

// parseBandWeights parses CV_BAND_WEIGHTS ("60,30,10") into the Band1/2/3 claim
// lottery weights. Malformed input, wrong arity, or an all-zero (meaningless)
// total falls back to the 60/30/10 default — an operator env, not user input.
func parseBandWeights(s string) [3]int {
	def := [3]int{60, 30, 10}
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return def
	}
	var w [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return def
		}
		w[i] = n
	}
	if w[0]+w[1]+w[2] == 0 {
		return def
	}
	return w
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
