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

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
	Cookie   CookieConfig
	Telegram TelegramConfig

	// GuestTokenTTL is the lifetime of an ephemeral Watch Together guest
	// JWT minted by POST /api/auth/guest (AUTH_GUEST_TOKEN_TTL, default 6h).
	// Guest tokens are access-only (no refresh), so a longer-than-access TTL
	// keeps mid-session re-mint churn rare. See libs/authz GenerateGuestToken.
	GuestTokenTTL time.Duration

	// MagicLinkTargetBase is the destination domain for cross-domain SSO
	// bridge redirects (MAGIC_LINK_TARGET_BASE, default https://animeenigma.org).
	// Generate redirects to this base; Login consumes the token here.
	// Trailing slash is stripped at load time.
	MagicLinkTargetBase string

	// WebAuthn configures the passkey (FIDO2/WebAuthn) relying party.
	WebAuthn WebAuthnConfig
}

// WebAuthnConfig configures the passkey relying party identity. RPID is the
// effective domain authenticators bind credentials to; RPOrigins are the
// fully-qualified origins the browser ceremony is allowed to complete from.
type WebAuthnConfig struct {
	RPID      string
	RPOrigins []string
}

type TelegramConfig struct {
	BotToken      string
	BotName       string
	WebhookSecret string
	WebhookURL    string
}

type CookieConfig struct {
	Domain   string
	Secure   bool
	SameSite string
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", time.Hour),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Cookie: CookieConfig{
			Domain:   getEnv("COOKIE_DOMAIN", ""),
			Secure:   getEnvBool("COOKIE_SECURE", false),
			SameSite: getEnv("COOKIE_SAMESITE", "Lax"),
		},
		Telegram: TelegramConfig{
			BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
			BotName:       getEnv("TELEGRAM_BOT_NAME", ""),
			WebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
			WebhookURL:    getEnv("TELEGRAM_WEBHOOK_URL", ""),
		},
		GuestTokenTTL:       getEnvDuration("AUTH_GUEST_TOKEN_TTL", 6*time.Hour),
		MagicLinkTargetBase: strings.TrimRight(getEnv("MAGIC_LINK_TARGET_BASE", "https://animeenigma.org"), "/"),
		WebAuthn: WebAuthnConfig{
			RPID:      getEnv("WEBAUTHN_RP_ID", "animeenigma.org"),
			RPOrigins: parseRPOrigins(getEnv("WEBAUTHN_RP_ORIGINS", "https://animeenigma.org")),
		},
	}, nil
}

// parseRPOrigins splits a comma-separated WEBAUTHN_RP_ORIGINS value into
// trimmed, non-empty origins. Without trimming, an operator-typed
// "https://a.com, https://b.com" would leave a leading-space origin that the
// browser's real Origin header can never match, silently breaking the
// ceremony for that origin.
func parseRPOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		origins = append(origins, p)
	}
	return origins
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
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
