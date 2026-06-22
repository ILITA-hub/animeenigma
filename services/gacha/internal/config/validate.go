package config

import "fmt"

// Validate rejects nonsensical economy settings at boot (audit medium #19).
// Without it, operator misconfig silently broke fairness/economy: PityThreshold<=0
// forces the top tier on the first roll; a negative pull cost credits the wallet
// on a pull; all-zero weights make every roll the top-of-pool tier. main() must
// fatal on a non-nil return.
func (c *Config) Validate() error {
	e := c.Economy
	if e.PityThreshold <= 0 {
		return fmt.Errorf("GACHA_PITY_THRESHOLD must be > 0, got %d", e.PityThreshold)
	}
	if e.PullCostX1 < 0 || e.PullCostX10 < 0 {
		return fmt.Errorf("GACHA_PULL_COST_X1/X10 must be >= 0, got %d/%d", e.PullCostX1, e.PullCostX10)
	}
	if e.DailyStreakStep <= 0 {
		return fmt.Errorf("GACHA_DAILY_STREAK_STEP must be > 0, got %d", e.DailyStreakStep)
	}
	if e.StarterBonus < 0 || e.DailyBase < 0 {
		return fmt.Errorf("GACHA_STARTER_BONUS and GACHA_DAILY_BASE must be >= 0, got %d/%d", e.StarterBonus, e.DailyBase)
	}
	sum := 0
	for _, w := range []int{e.WeightN, e.WeightR, e.WeightSR, e.WeightSSR} {
		if w < 0 {
			return fmt.Errorf("gacha tier weights must be >= 0, got N=%d R=%d SR=%d SSR=%d",
				e.WeightN, e.WeightR, e.WeightSR, e.WeightSSR)
		}
		sum += w
	}
	if sum == 0 {
		return fmt.Errorf("at least one gacha tier weight must be > 0 (all weights are 0)")
	}
	return nil
}
