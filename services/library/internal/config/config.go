package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

// Config is the library service top-level config. Phase 2 adds the
// Nyaa + AnimeTosho + LibrarySearch sub-configs that drive the
// search endpoint. Phase 3 adds the Torrent / Worker / Disk knobs
// (workstream raw-jp / v0.2). Phase 4 adds Encode + Minio for the
// ffmpeg/HLS transcoder + MinIO writer.
type Config struct {
	Server        ServerConfig
	Database      database.Config
	// Redis is added in v4.0 Phase 3 (AR-EFFECT-01) so the GORM db_read P95
	// ReadGate refresher has a Redis handle to snapshot the read_thresholds
	// hash. The shared REDIS_* trio is already provided by docker-compose.
	Redis         cache.Config
	JWT           authz.JWTConfig
	Nyaa          NyaaConfig
	AnimeTosho    AnimeToshoConfig
	Jackett       JackettConfig
	LibrarySearch LibrarySearchConfig
	Torrent       TorrentConfig
	Worker        WorkerConfig
	Disk          DiskConfig
	Encode        EncodeConfig
	Minio         MinioConfig
	// CatalogInternal — workstream raw-jp, Phase 06 (v0.2). After
	// every successful encode the library service POSTs to
	// {CATALOG_INTERNAL_API_URL}/internal/cache/invalidate/raw/{id}
	// to bust the catalog's source-decision cache.
	CatalogInternal CatalogInternalConfig
}

// CatalogInternalConfig drives the best-effort cache-bust webhook the
// library encoder fires after a successful job. APIURL defaults to
// the docker-network address of the catalog service. An empty value
// switches the invalidator to a no-op (the catalog 1h TTL covers
// correctness, only the fast-path is skipped).
type CatalogInternalConfig struct {
	APIURL  string
	Timeout time.Duration
}

// EncodeConfig drives services/library/internal/ffmpeg + the encoder
// worker pool (internal/service/encoder_worker.go).
type EncodeConfig struct {
	Workers        int
	Tmpdir         string
	FfmpegBin      string
	FfprobeBin     string
	MaxBitrateKbps int
	Threads        int
	Nice           int
}

// MinioConfig drives services/library/internal/minio.Writer.
type MinioConfig struct {
	Endpoint          string
	AccessKey         string
	SecretKey         string
	Bucket            string
	UseSSL            bool
	UploadConcurrency int
}

// TorrentConfig drives services/library/internal/torrent. All values
// are SPEC-locked defaults from 03-CONTEXT.md.
type TorrentConfig struct {
	DownloadDir    string
	MaxPeers       int
	UploadRateKBPS int
	SeedDuration   time.Duration
	StallTimeout   time.Duration
}

// WorkerConfig drives services/library/internal/service/download_worker.go.
type WorkerConfig struct {
	Count        int
	ProgressTick time.Duration
}

// DiskConfig drives services/library/internal/service/disk_guard.go.
// MinFreePct is the threshold the enqueue handler enforces (POST
// returns 507 when freePct < MinFreePct).
type DiskConfig struct {
	MinFreePct   int
	PollInterval time.Duration
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// NyaaConfig and AnimeToshoConfig are the per-client knobs. The default
// timeout + UA are shared (both providers tolerate the same User-Agent
// and 15s is appropriate for torrent indexers — slower and more
// variable than streaming APIs).
type NyaaConfig struct {
	BaseURL     string
	HTTPTimeout time.Duration
	UserAgent   string
}

type AnimeToshoConfig struct {
	BaseURL     string
	HTTPTimeout time.Duration
	UserAgent   string
}

// JackettConfig drives the Jackett primary-tier client
// (services/library/internal/parser/jackett). Enabled is derived: an empty
// APIKey leaves the primary tier off and the search falls back to the
// legacy Nyaa+AnimeTosho aggregator (dark-ship safe). Categories is an
// optional Torznab category allow-list (e.g. "5070" for TV/Anime); empty
// means all categories. BaseURL defaults to the docker-network DNS name —
// the host-bound 127.0.0.1:9117 is only reachable from the operator's
// browser, not from inside the library container.
type JackettConfig struct {
	BaseURL       string
	APIKey        string
	Categories    []string
	IndexerFilter string
	HTTPTimeout   time.Duration
	UserAgent     string
	Enabled       bool
}

// LibrarySearchConfig holds limits documented for the operator; the
// aggregator currently enforces these via package-level constants in
// internal/service/search.go. The struct is informational — Phase 3+
// may promote these to runtime config.
type LibrarySearchConfig struct {
	DefaultLimit int
	MaxLimit     int
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	// Phase 2: shared timeout + UA flow into both clients via env
	// (LIBRARY_SEARCH_TIMEOUT / LIBRARY_SEARCH_UA). Per-provider
	// overrides aren't useful in practice — both upstreams behave the
	// same on these knobs — so we keep one env var per concept.
	searchTimeout := getEnvDuration("LIBRARY_SEARCH_TIMEOUT", 15*time.Second)
	searchUA := getEnv("LIBRARY_SEARCH_UA", "AnimeEnigma/1.0 (library service)")

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8089),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "library"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Nyaa: NyaaConfig{
			BaseURL:     getEnv("NYAA_BASE_URL", "https://nyaa.si"),
			HTTPTimeout: searchTimeout,
			UserAgent:   searchUA,
		},
		AnimeTosho: AnimeToshoConfig{
			BaseURL:     getEnv("ANIMETOSHO_BASE_URL", "https://feed.animetosho.org"),
			HTTPTimeout: searchTimeout,
			UserAgent:   searchUA,
		},
		Jackett: JackettConfig{
			BaseURL: getEnv("JACKETT_BASE_URL", "http://jackett:9117"),
			APIKey:  getEnv("JACKETT_API_KEY", ""),
			// CSV → slice; empty env yields nil (all categories).
			Categories: splitCSV(getEnv("JACKETT_CATEGORIES", "")),
			// Aggregate-endpoint filter segment. "!status:failing" skips
			// indexers Jackett marks failing (broken ones stall the whole
			// aggregate ~100s, past JACKETT_TIMEOUT). "all" = unfiltered.
			IndexerFilter: getEnv("JACKETT_INDEXER_FILTER", "!status:failing"),
			// Jackett's aggregated `all` query fans out across ~20 indexers
			// and routinely takes ~20s — far longer than a single indexer,
			// hence a dedicated 30s default rather than the shared 15s.
			HTTPTimeout: getEnvDuration("JACKETT_TIMEOUT", 30*time.Second),
			UserAgent:   searchUA,
			Enabled:     getEnv("JACKETT_API_KEY", "") != "",
		},
		LibrarySearch: LibrarySearchConfig{
			DefaultLimit: getEnvInt("LIBRARY_SEARCH_DEFAULT_LIMIT", 50),
			MaxLimit:     getEnvInt("LIBRARY_SEARCH_MAX_LIMIT", 200),
		},
		Torrent: TorrentConfig{
			DownloadDir:    getEnv("LIBRARY_TORRENT_DOWNLOAD_DIR", "/data/torrents"),
			MaxPeers:       getEnvInt("LIBRARY_TORRENT_MAX_PEERS", 80),
			UploadRateKBPS: getEnvInt("LIBRARY_TORRENT_MAX_UPLOAD_RATE_KBPS", 1024),
			SeedDuration:   getEnvDuration("LIBRARY_TORRENT_SEED_DURATION", 24*time.Hour),
			StallTimeout:   getEnvDuration("LIBRARY_TORRENT_STALL_TIMEOUT", 30*time.Minute),
		},
		Worker: WorkerConfig{
			Count:        getEnvInt("LIBRARY_DOWNLOAD_WORKERS", 2),
			ProgressTick: getEnvDuration("LIBRARY_DOWNLOAD_PROGRESS_TICK", 5*time.Second),
		},
		Disk: DiskConfig{
			MinFreePct:   getEnvInt("LIBRARY_DISK_FREE_MIN_PCT", 20),
			PollInterval: getEnvDuration("LIBRARY_DISK_POLL_INTERVAL", 30*time.Second),
		},
		Encode: EncodeConfig{
			Workers:        getEnvInt("LIBRARY_ENCODE_WORKERS", 2),
			Tmpdir:         getEnv("LIBRARY_ENCODE_TMPDIR", "/tmp/encode"),
			FfmpegBin:      getEnv("LIBRARY_FFMPEG_BIN", "/usr/bin/ffmpeg"),
			FfprobeBin:     getEnv("LIBRARY_FFPROBE_BIN", "/usr/bin/ffprobe"),
			MaxBitrateKbps: getEnvInt("LIBRARY_ENCODE_MAX_BITRATE_KBPS", 5000),
			Threads:        getEnvInt("LIBRARY_ENCODE_THREADS", 3),
			Nice:           getEnvInt("LIBRARY_ENCODE_NICE", 15),
		},
		Minio: MinioConfig{
			Endpoint:          getEnv("LIBRARY_MINIO_ENDPOINT", "minio:9000"),
			AccessKey:         getEnv("LIBRARY_MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey:         getEnv("LIBRARY_MINIO_SECRET_KEY", "minioadmin"),
			Bucket:            getEnv("LIBRARY_MINIO_BUCKET", "raw-library"),
			UseSSL:            getEnvBool("LIBRARY_MINIO_USE_SSL", false),
			UploadConcurrency: getEnvInt("LIBRARY_MINIO_UPLOAD_CONCURRENCY", 8),
		},
		CatalogInternal: CatalogInternalConfig{
			APIURL:  getEnv("CATALOG_INTERNAL_API_URL", "http://catalog:8081"),
			Timeout: getEnvDuration("CATALOG_INTERNAL_API_TIMEOUT", 3*time.Second),
		},
	}, nil
}

// getEnvBool returns the boolean value of an env var, parsing common
// truthy/falsy strings. Defaults to defaultVal on parse failure or
// empty. Recognised true: "true", "1", "yes", "on" (case-insensitive).
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return defaultVal
}

// splitCSV parses a comma-separated env value into a trimmed, non-empty
// slice. Returns nil for an empty/whitespace input so callers can treat
// "unset" and "empty" identically.
func splitCSV(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
