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
	SuppressedAlerts []string // alert keys to ignore (e.g., "Parser Failure Rate:hianime")
	StatePath        string
	IssuePath        string
}

type GrafanaConfig struct {
	URL          string
	PollInterval int // seconds between Grafana alert checks
	WebhookUser  string
	WebhookPass  string
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
		},
		Claude: ClaudeConfig{
			Path:        getEnv("CLAUDE_PATH", "/root/.local/bin/claude"),
			Model:       getEnv("DEFAULT_MODEL", "sonnet"),
			CodeModel:   getEnv("CODE_FIX_MODEL", "opus"),
			ProjectRoot: getEnv("PROJECT_ROOT", "/data/animeenigma"),
			PromptPath:  getEnv("MAINTENANCE_PROMPT_PATH", ".claude/maintenance-prompt.md"),
			TimeoutSec:  getEnvInt("CLAUDE_TIMEOUT_SEC", 300),
		},
		Admins:           admins,
		SuppressedAlerts: parseSuppressed(getEnv("SUPPRESSED_ALERTS", "")),
		StatePath:        getEnv("STATE_PATH", ".claude/maintenance-state.json"),
		IssuePath:        getEnv("ISSUES_PATH", "docs/issues/issues.json"),
	}, nil
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
