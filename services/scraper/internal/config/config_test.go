package config

import (
	"bytes"
	"log"
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

// TestLoad_ServerPriorityDefault — SCRAPER-HEAL-03: with no env set,
// Gogoanime.ServerPriority defaults to ["streamhg", "earnvids", "vibeplayer"]
// (the canonical safe order from CONTEXT.md D3).
func TestLoad_ServerPriorityDefault(t *testing.T) {
	unsetEnv(t, "SCRAPER_SERVER_PRIORITY",
		"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"ANIMEPAHE_BASE_URL", "SCRAPER_GOGOANIME_BASE_URL",
		"SCRAPER_ANIMEKAI_BASE_URL",
	)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"streamhg", "earnvids", "vibeplayer"}
	if got := cfg.Gogoanime.ServerPriority; !equalStringSlices(got, want) {
		t.Fatalf("ServerPriority = %v; want %v", got, want)
	}
}

// TestLoad_ServerPriorityOverride — env override changes the order, lowercases
// + trims, and drops empties.
func TestLoad_ServerPriorityOverride(t *testing.T) {
	setEnv(t, "SCRAPER_SERVER_PRIORITY", "earnvids,streamhg,vibeplayer")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"earnvids", "streamhg", "vibeplayer"}
	if got := cfg.Gogoanime.ServerPriority; !equalStringSlices(got, want) {
		t.Fatalf("ServerPriority = %v; want %v", got, want)
	}
}

// TestLoad_ServerPriorityWhitespace — whitespace + mixed case + empty entries
// are normalized.
func TestLoad_ServerPriorityWhitespace(t *testing.T) {
	setEnv(t, "SCRAPER_SERVER_PRIORITY", "  StreamHG , , VibePlayer ,")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"streamhg", "vibeplayer"}
	if got := cfg.Gogoanime.ServerPriority; !equalStringSlices(got, want) {
		t.Fatalf("ServerPriority = %v; want %v", got, want)
	}
}

// TestLoad_ServerPriorityEmptyFallsBackToDefault — explicit empty env string
// returns the canonical default rather than an empty slice. Matches the
// "unset env" path.
func TestLoad_ServerPriorityEmptyFallsBackToDefault(t *testing.T) {
	setEnv(t, "SCRAPER_SERVER_PRIORITY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"streamhg", "earnvids", "vibeplayer"}
	if got := cfg.Gogoanime.ServerPriority; !equalStringSlices(got, want) {
		t.Fatalf("ServerPriority = %v; want %v", got, want)
	}
}

// TestLoad_ServerPriorityAllEmpty — input consisting only of commas/whitespace
// collapses to the canonical default rather than an empty slice (so the
// orchestrator never silently disables priority sorting).
func TestLoad_ServerPriorityAllEmpty(t *testing.T) {
	setEnv(t, "SCRAPER_SERVER_PRIORITY", " , , ,")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"streamhg", "earnvids", "vibeplayer"}
	if got := cfg.Gogoanime.ServerPriority; !equalStringSlices(got, want) {
		t.Fatalf("ServerPriority = %v; want %v (empty input must fall back to default)", got, want)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLoad_AnimeKaiDefaults — with NO env vars set, AnimeKai is disabled
// and BaseURL defaults to https://anikai.to (the canonical mirror as of
// 2026-05-12; animekai.to 301s here).
func TestLoad_AnimeKaiDefaults(t *testing.T) {
	unsetEnv(t,
		"SCRAPER_ANIMEKAI_ENABLED", "SCRAPER_ANIMEKAI_BASE_URL",
		"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"ANIMEPAHE_BASE_URL",
	)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AnimeKai.Enabled {
		t.Fatalf("AnimeKai.Enabled = true; want false (default-off in production)")
	}
	if cfg.AnimeKai.BaseURL != "https://anikai.to" {
		t.Fatalf("AnimeKai.BaseURL = %q; want https://anikai.to", cfg.AnimeKai.BaseURL)
	}
}

// TestLoad_AnimeKaiEnabledTrue — SCRAPER_ANIMEKAI_ENABLED=true flips the flag.
func TestLoad_AnimeKaiEnabledTrue(t *testing.T) {
	unsetEnv(t, "SCRAPER_ANIMEKAI_BASE_URL")
	setEnv(t, "SCRAPER_ANIMEKAI_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AnimeKai.Enabled {
		t.Fatalf("AnimeKai.Enabled = false; want true")
	}
}

// TestLoad_AnimeKaiEnabledFalseExplicit — explicit "false" keeps the flag off.
func TestLoad_AnimeKaiEnabledFalseExplicit(t *testing.T) {
	unsetEnv(t, "SCRAPER_ANIMEKAI_BASE_URL")
	setEnv(t, "SCRAPER_ANIMEKAI_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AnimeKai.Enabled {
		t.Fatalf("AnimeKai.Enabled = true; want false")
	}
}

// TestLoad_AnimeKaiEnabledInvalid — unparseable value falls back to default.
// Matches the lenient getEnv* convention. Adversary cannot enable the
// provider via SCRAPER_ANIMEKAI_ENABLED=yes-please-enable.
func TestLoad_AnimeKaiEnabledInvalid(t *testing.T) {
	unsetEnv(t, "SCRAPER_ANIMEKAI_BASE_URL")
	setEnv(t, "SCRAPER_ANIMEKAI_ENABLED", "garbage")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v (must not error on garbage value)", err)
	}
	if cfg.AnimeKai.Enabled {
		t.Fatalf("AnimeKai.Enabled = true; want default false on unparseable value")
	}
}

// TestLoad_AnimeKaiBaseURLOverride — SCRAPER_ANIMEKAI_BASE_URL takes precedence.
func TestLoad_AnimeKaiBaseURLOverride(t *testing.T) {
	setEnv(t, "SCRAPER_ANIMEKAI_BASE_URL", "https://anikai.cc")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AnimeKai.BaseURL != "https://anikai.cc" {
		t.Fatalf("AnimeKai.BaseURL = %q; want https://anikai.cc", cfg.AnimeKai.BaseURL)
	}
}

// TestLoad_AnimeKaiInvalidBaseURL — malformed URL fails Load() at boot with
// an error that names the env var verbatim.
func TestLoad_AnimeKaiInvalidBaseURL(t *testing.T) {
	setEnv(t, "SCRAPER_ANIMEKAI_BASE_URL", "not-a-url")
	_, err := Load()
	if err == nil {
		t.Fatal("Load: nil error; want non-nil for malformed SCRAPER_ANIMEKAI_BASE_URL")
	}
	if !strings.Contains(err.Error(), "SCRAPER_ANIMEKAI_BASE_URL") {
		t.Fatalf("error %q must mention SCRAPER_ANIMEKAI_BASE_URL", err.Error())
	}
}

// TestGetEnvBool_LogsOnUnparseable — WR-03. Unparseable values fall back to
// the default (lenient convention preserved) BUT the helper MUST emit a
// WARN log line naming the env-var key and the rejected value so an
// operator who typo'd "yes-please" sees their value was rejected. The fix
// matters because SCRAPER_ANIMEKAI_ENABLED is the only gate between the
// escape-hatch stub being registered or not — silent fall-through means
// operators can think the flag is on when it isn't.
func TestGetEnvBool_LogsOnUnparseable(t *testing.T) {
	const probeKey = "SCRAPER_TEST_WARN_ANIMEKAI_BOOL"
	setEnv(t, probeKey, "yes-please")

	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	// Drop the timestamp prefix so the substring match below is stable.
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	got := getEnvBool(probeKey, false)
	if got {
		t.Fatalf("getEnvBool(%q, default=false) = true; want false (unparseable must fall back)", "yes-please")
	}
	logged := buf.String()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log output %q must contain WARN level (operator-visible diagnostic)", logged)
	}
	if !strings.Contains(logged, probeKey) {
		t.Errorf("log output %q must name the env-var key %q so operators can grep", logged, probeKey)
	}
	if !strings.Contains(logged, "yes-please") {
		t.Errorf("log output %q must include the rejected value so operators see their typo", logged)
	}
}

// TestGetEnvBool_Truthy — strconv.ParseBool semantics: "1", "true", "True",
// "TRUE", "t" all return true.
func TestGetEnvBool_Truthy(t *testing.T) {
	cases := []string{"1", "true", "True", "TRUE", "t"}
	const probeKey = "SCRAPER_TEST_ANIMEKAI_BOOL"
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			setEnv(t, probeKey, c)
			if got := getEnvBool(probeKey, false); !got {
				t.Errorf("getEnvBool(%q) = false; want true", c)
			}
		})
	}
}

// TestGetEnvBool_Falsy — "0", "false", "f", "False" all return false; AND
// unparseable values fall back to the default (also false here).
func TestGetEnvBool_Falsy(t *testing.T) {
	cases := []string{"0", "false", "f", "False", "garbage", "yes-please"}
	const probeKey = "SCRAPER_TEST_ANIMEKAI_BOOL"
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			setEnv(t, probeKey, c)
			// Default = true to prove unparseable falls back to default;
			// valid falsy values still return false.
			got := getEnvBool(probeKey, true)
			isCanonicalFalse := c == "0" || c == "false" || c == "f" || c == "False"
			if isCanonicalFalse && got {
				t.Errorf("getEnvBool(%q, default=true) = true; want false for canonical falsy value", c)
			}
			if !isCanonicalFalse && !got {
				t.Errorf("getEnvBool(%q, default=true) = false; want true (unparseable should fall back to default=true)", c)
			}
		})
	}
}
