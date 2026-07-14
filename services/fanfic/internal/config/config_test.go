package config

import (
	"os"
	"reflect"
	"testing"
)

// setRequired sets the two env vars Load() hard-requires and returns a
// cleanup func restoring the previous (unset) state.
func setRequired(t *testing.T) {
	t.Helper()
	os.Setenv("JWT_SECRET", "x")
	os.Setenv("FANFIC_GROQ_API_KEY", "x")
	t.Cleanup(func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("FANFIC_GROQ_API_KEY")
	})
}

func TestLoad_RequiresJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when JWT_SECRET is unset")
	}
}

func TestLoad_RequiresGroqAPIKey(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	os.Unsetenv("FANFIC_GROQ_API_KEY")
	defer os.Unsetenv("JWT_SECRET")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when FANFIC_GROQ_API_KEY is unset")
	}
}

func TestLoad_DailyDefaults(t *testing.T) {
	setRequired(t)
	os.Unsetenv("TELEGRAM_ALERTS_BOT_TOKEN")
	os.Unsetenv("TELEGRAM_ADMIN_CHAT_ID")
	os.Unsetenv("FANFIC_DAILY_ANIME_POOL")
	os.Unsetenv("FANFIC_BOT_LANGUAGE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AlertsBotToken != "" {
		t.Errorf("AlertsBotToken = %q; want empty (Noop fallback)", cfg.AlertsBotToken)
	}
	if cfg.AlertsChatID != "" {
		t.Errorf("AlertsChatID = %q; want empty (Noop fallback)", cfg.AlertsChatID)
	}
	if cfg.BotLanguage != "ru" {
		t.Errorf("BotLanguage = %q; want ru", cfg.BotLanguage)
	}
	want := []string{"20", "21", "1735", "52991", "16498", "5114"}
	if !reflect.DeepEqual(cfg.DailyAnimePool, want) {
		t.Errorf("DailyAnimePool = %v; want %v", cfg.DailyAnimePool, want)
	}
}

func TestLoad_DailyAnimePoolCSVTrimsAndDropsEmpty(t *testing.T) {
	setRequired(t)
	os.Setenv("FANFIC_DAILY_ANIME_POOL", " 1, 2 ,,3,")
	defer os.Unsetenv("FANFIC_DAILY_ANIME_POOL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"1", "2", "3"}
	if !reflect.DeepEqual(cfg.DailyAnimePool, want) {
		t.Errorf("DailyAnimePool = %v; want %v", cfg.DailyAnimePool, want)
	}
}

func TestLoad_AlertsAndLangOverride(t *testing.T) {
	setRequired(t)
	os.Setenv("TELEGRAM_ALERTS_BOT_TOKEN", "bot-token")
	os.Setenv("TELEGRAM_ADMIN_CHAT_ID", "-100123")
	os.Setenv("FANFIC_BOT_LANGUAGE", "en")
	defer func() {
		os.Unsetenv("TELEGRAM_ALERTS_BOT_TOKEN")
		os.Unsetenv("TELEGRAM_ADMIN_CHAT_ID")
		os.Unsetenv("FANFIC_BOT_LANGUAGE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AlertsBotToken != "bot-token" {
		t.Errorf("AlertsBotToken = %q; want bot-token", cfg.AlertsBotToken)
	}
	if cfg.AlertsChatID != "-100123" {
		t.Errorf("AlertsChatID = %q; want -100123", cfg.AlertsChatID)
	}
	if cfg.BotLanguage != "en" {
		t.Errorf("BotLanguage = %q; want en", cfg.BotLanguage)
	}
}
