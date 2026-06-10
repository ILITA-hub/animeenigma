package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when JWT_SECRET is unset")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	defer os.Unsetenv("JWT_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8093 {
		t.Errorf("port = %d; want 8093", cfg.Server.Port)
	}
	if cfg.Database.Database != "animeenigma" {
		t.Errorf("db = %q; want animeenigma", cfg.Database.Database)
	}
	if cfg.Economy.StarterBonus != 300 {
		t.Errorf("starter = %d; want 300", cfg.Economy.StarterBonus)
	}
	if !cfg.Enabled {
		t.Error("Enabled default = false; want true")
	}
}

func TestLoad_PullDefaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	defer os.Unsetenv("JWT_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e := cfg.Economy
	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"PullCostX1", e.PullCostX1, 100},
		{"PullCostX10", e.PullCostX10, 900},
		{"PityThreshold", int64(e.PityThreshold), 90},
		{"WeightN", int64(e.WeightN), 69},
		{"WeightR", int64(e.WeightR), 22},
		{"WeightSR", int64(e.WeightSR), 8},
		{"WeightSSR", int64(e.WeightSSR), 1},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d; want %d", c.name, c.got, c.want)
		}
	}
}

func TestLoad_StarterBonusOverride(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	os.Setenv("GACHA_STARTER_BONUS", "500")
	defer func() { os.Unsetenv("JWT_SECRET"); os.Unsetenv("GACHA_STARTER_BONUS") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Economy.StarterBonus != 500 {
		t.Errorf("starter = %d; want 500", cfg.Economy.StarterBonus)
	}
}
