package config

import (
	"os"
	"strings"
	"testing"
)

// helper to restore env after a test sets it.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if err := os.Setenv(key, val); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		prev, hadPrev := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		key := k
		had := hadPrev
		prevVal := prev
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(key, prevVal)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

// TestConfig_Load_Defaults — with no env vars set, Load() returns the
// docker-compose-friendly defaults: redis at "redis:6379", animepahe at
// https://animepahe.ru.
func TestConfig_Load_Defaults(t *testing.T) {
	unsetEnv(t,
		"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"ANIMEPAHE_BASE_URL",
	)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg.Redis.Host != "redis" {
		t.Errorf("Redis.Host = %q; want \"redis\"", cfg.Redis.Host)
	}
	if cfg.Redis.Port != 6379 {
		t.Errorf("Redis.Port = %d; want 6379", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "" {
		t.Errorf("Redis.Password = %q; want empty", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 0 {
		t.Errorf("Redis.DB = %d; want 0", cfg.Redis.DB)
	}
	if cfg.AnimePahe.BaseURL != "https://animepahe.ru" {
		t.Errorf("AnimePahe.BaseURL = %q; want https://animepahe.ru", cfg.AnimePahe.BaseURL)
	}
}

// TestConfig_Load_EnvOverride — REDIS_HOST / REDIS_PORT / ANIMEPAHE_BASE_URL
// env vars take precedence.
func TestConfig_Load_EnvOverride(t *testing.T) {
	setEnv(t, "REDIS_HOST", "other-host")
	setEnv(t, "REDIS_PORT", "6380")
	setEnv(t, "REDIS_PASSWORD", "secret")
	setEnv(t, "REDIS_DB", "3")
	setEnv(t, "ANIMEPAHE_BASE_URL", "https://example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg.Redis.Host != "other-host" {
		t.Errorf("Redis.Host = %q; want other-host", cfg.Redis.Host)
	}
	if cfg.Redis.Port != 6380 {
		t.Errorf("Redis.Port = %d; want 6380", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "secret" {
		t.Errorf("Redis.Password = %q; want secret", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 3 {
		t.Errorf("Redis.DB = %d; want 3", cfg.Redis.DB)
	}
	if cfg.AnimePahe.BaseURL != "https://example.com" {
		t.Errorf("AnimePahe.BaseURL = %q; want https://example.com", cfg.AnimePahe.BaseURL)
	}
}

// TestConfig_Load_InvalidPort — non-numeric REDIS_PORT falls back to the
// default 6379 (matches existing getEnvInt pattern).
func TestConfig_Load_InvalidPort(t *testing.T) {
	unsetEnv(t, "REDIS_HOST", "REDIS_PASSWORD", "REDIS_DB", "ANIMEPAHE_BASE_URL")
	setEnv(t, "REDIS_PORT", "not-a-number")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg.Redis.Port != 6379 {
		t.Errorf("Redis.Port = %d; want fallback 6379", cfg.Redis.Port)
	}
}

// TestConfig_Load_InvalidAnimePaheURL — missing scheme yields a Load() error,
// mirroring the existing MEGACLOUD_EXTRACTOR_URL validation behavior.
func TestConfig_Load_InvalidAnimePaheURL(t *testing.T) {
	unsetEnv(t, "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB")
	setEnv(t, "ANIMEPAHE_BASE_URL", "not-a-url")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil; want error for malformed ANIMEPAHE_BASE_URL")
	}
}

// TestLoad_GogoanimeConfig_DefaultsAndOverride pins Phase 18's new env-var
// surface — Gogoanime.BaseURL reads SCRAPER_GOGOANIME_BASE_URL; defaults to
// https://anitaku.to; rejects malformed URLs at boot with an error message
// that names the env var verbatim (so operators can grep logs).
func TestLoad_GogoanimeConfig_DefaultsAndOverride(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		unsetEnv(t, "SCRAPER_GOGOANIME_BASE_URL")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Gogoanime.BaseURL != "https://anitaku.to" {
			t.Fatalf("default = %q, want https://anitaku.to", cfg.Gogoanime.BaseURL)
		}
	})
	t.Run("override", func(t *testing.T) {
		setEnv(t, "SCRAPER_GOGOANIME_BASE_URL", "https://anitaku.io")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Gogoanime.BaseURL != "https://anitaku.io" {
			t.Fatalf("override = %q, want https://anitaku.io", cfg.Gogoanime.BaseURL)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		setEnv(t, "SCRAPER_GOGOANIME_BASE_URL", "not-a-url")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
		if !strings.Contains(err.Error(), "SCRAPER_GOGOANIME_BASE_URL") {
			t.Fatalf("error %q must mention SCRAPER_GOGOANIME_BASE_URL", err.Error())
		}
	})
}
