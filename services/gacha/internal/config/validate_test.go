package config_test

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
)

func validEconomy() config.EconomyConfig {
	return config.EconomyConfig{
		StarterBonus: 300, DailyBase: 50, DailyStreakStep: 10, DailyStreakCap: 100,
		PullCostX1: 100, PullCostX10: 900, PityThreshold: 90,
		WeightN: 69, WeightR: 22, WeightSR: 8, WeightSSR: 1,
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (&config.Config{Economy: validEconomy()}).Validate(); err != nil {
		t.Fatalf("valid economy rejected: %v", err)
	}

	cases := map[string]func(*config.EconomyConfig){
		"pity <= 0 (forces top tier on first roll)": func(e *config.EconomyConfig) { e.PityThreshold = 0 },
		"negative x1 cost (credits on pull)":        func(e *config.EconomyConfig) { e.PullCostX1 = -1 },
		"negative x10 cost":                         func(e *config.EconomyConfig) { e.PullCostX10 = -1 },
		"streak step <= 0":                          func(e *config.EconomyConfig) { e.DailyStreakStep = 0 },
		"negative starter bonus":                    func(e *config.EconomyConfig) { e.StarterBonus = -5 },
		"negative daily base":                       func(e *config.EconomyConfig) { e.DailyBase = -1 },
		"all weights zero (always top-of-pool)":     func(e *config.EconomyConfig) { e.WeightN, e.WeightR, e.WeightSR, e.WeightSSR = 0, 0, 0, 0 },
		"negative weight":                           func(e *config.EconomyConfig) { e.WeightSSR = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := validEconomy()
			mutate(&e)
			if err := (&config.Config{Economy: e}).Validate(); err == nil {
				t.Fatalf("expected validation error for: %s", name)
			}
		})
	}
}
