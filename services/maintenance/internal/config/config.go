package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server           ServerConfig
	Telegram         TelegramConfig
	Grafana          GrafanaConfig
	Claude           ClaudeConfig
	Admins           []string
	SuppressedAlerts []string // alert keys to ignore (e.g., "Parser Failure Rate:kodik")
	StatePath        string
	IssuePath        string
	// ReportsAuthToken, when set, is the shared secret required on the
	// /api/reports endpoint (X-Maintenance-Token header). The player service
	// is the sole legitimate caller. When empty the endpoint stays open and
	// the daemon logs a WARN at startup. See finding #39 (autonomous-fix
	// trigger): an unauthenticated report can drive a write+deploy+git-push.
	ReportsAuthToken string
	// PlayerInternalURL is the base URL of the player service's internal
	// feedback API (the bot runs on the host; player publishes 127.0.0.1:8083).
	PlayerInternalURL string
	// AttachmentsDir is where Telegram attachments are saved on the host so
	// the Claude dispatcher can Read them (relative paths resolve against
	// PROJECT_ROOT).
	AttachmentsDir string
	// FeedbackBaseURL prefixes /admin/feedback deep links in Telegram replies.
	FeedbackBaseURL string
	// TestMode is a future-hook flag (Phase 23 Plan 23-03 / T-23-10
	// mitigation): when MAINTENANCE_TEST_MODE=true, the dispatcher MAY
	// short-circuit before invoking the Claude CLI / Telegram client so
	// integration tests can post synthetic alerts at a live binary without
	// triggering a real fix. Plan 23-03 only plumbs the field; consuming
	// callers land in a future plan.
	TestMode   bool
	CatalogURL string
	// PolicyURL is policy-service's base URL — the maintenancegate client
	// consults it to check whether the maintenance_bot routine is enabled and
	// its auto_apply_max_risk cap. Docker default; overridden to localhost in
	// maintenance.env since the bot is host-native.
	PolicyURL string
}

type GrafanaConfig struct {
	URL          string
	PollInterval int // seconds between Grafana alert checks
	WebhookUser  string
	WebhookPass  string
	// APIUser/APIPass authenticate the OUTBOUND alertmanager poll (the
	// safety-net reconcile). Distinct from WebhookUser/Pass (which secure the
	// INBOUND webhook). When APIPass is empty the poll is skipped — webhook
	// delivery still works.
	APIUser string
	APIPass string
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type TelegramConfig struct {
	BotToken string
	ChatID   int64
}

type ClaudeConfig struct {
	Path        string
	Model       string
	CodeModel   string
	ProjectRoot string
	PromptPath  string
	TimeoutSec  int
}

func Load() (*Config, error) {
	token := getEnv("TELEGRAM_BOT_TOKEN", "")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	chatIDStr := getEnv("TELEGRAM_ADMIN_CHAT_ID", "")
	if chatIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_ADMIN_CHAT_ID is required")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_ADMIN_CHAT_ID must be an integer: %w", err)
	}

	adminsStr := getEnv("ADMIN_USERNAMES", "tNeymik,NANDIorg_9")
	admins := strings.Split(adminsStr, ",")
	for i := range admins {
		admins[i] = strings.TrimSpace(admins[i])
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8087),
		},
		Telegram: TelegramConfig{
			BotToken: token,
			ChatID:   chatID,
		},
		Grafana: GrafanaConfig{
			URL:          getEnv("GRAFANA_URL", "http://localhost:3004"),
			PollInterval: getEnvInt("GRAFANA_POLL_INTERVAL", 600),
			WebhookUser:  getEnv("GRAFANA_WEBHOOK_USER", "grafana"),
			WebhookPass:  getEnv("GRAFANA_WEBHOOK_PASS", ""),
			APIUser:      getEnv("GRAFANA_API_USER", "admin"),
			APIPass:      getEnv("GRAFANA_API_PASS", ""),
		},
		Claude: ClaudeConfig{
			Path:        getEnv("CLAUDE_PATH", "/root/.local/bin/claude"),
			Model:       getEnv("DEFAULT_MODEL", "sonnet"),
			CodeModel:   getEnv("CODE_FIX_MODEL", "opus"),
			ProjectRoot: getEnv("PROJECT_ROOT", "/data/animeenigma"),
			PromptPath:  getEnv("MAINTENANCE_PROMPT_PATH", ".claude/maintenance-prompt.md"),
			TimeoutSec:  getEnvInt("CLAUDE_TIMEOUT_SEC", 300),
		},
		Admins:            admins,
		SuppressedAlerts:  parseSuppressed(getEnv("SUPPRESSED_ALERTS", "")),
		StatePath:         getEnv("STATE_PATH", ".claude/maintenance-state.json"),
		IssuePath:         getEnv("ISSUES_PATH", "docs/issues/issues.json"),
		ReportsAuthToken:  getEnv("REPORTS_AUTH_TOKEN", ""),
		PlayerInternalURL: getEnv("PLAYER_INTERNAL_URL", "http://localhost:8083"),
		AttachmentsDir:    getEnv("ATTACHMENTS_DIR", ".claude/maintenance-attachments"),
		FeedbackBaseURL:   getEnv("FEEDBACK_BASE_URL", "https://animeenigma.org"),
		TestMode:          getEnvBool("MAINTENANCE_TEST_MODE", false),
		CatalogURL:        getEnv("CATALOG_URL", "http://catalog:8081"),
		PolicyURL:         getEnv("POLICY_URL", "http://policy:8098"),
	}, nil
}

// getEnvBool parses the env-var value as a canonical Go boolean ("true" /
// "false" / "1" / "0" / "T" / "F" — see strconv.ParseBool). Returns
// defaultVal on parse error or when the var is unset. Used by TestMode so
// the production default (false) is robust to typos in operator config.
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	// Be strict: only "true" enables — guards against accidental activation
	// from values like "yes" or arbitrary strings. The config_test asserts
	// this contract.
	if val == "true" {
		return true
	}
	return false
}

func parseSuppressed(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
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
