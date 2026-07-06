package domain

import "time"

// RouletteMasterKey is a reserved SecretFeatureFlag.Key holding the global
// on/off switch for the whole «Секретная фича» footer roulette. It shares the
// flag table with the per-feature keys so the admin surface reads/writes a
// single store. It is deliberately not a real feature key (double-underscore
// sentinel) so it can never collide with a frontend SECRET_FEATURES entry.
const RouletteMasterKey = "__roulette__"

// SecretFeatureFlag is one admin-managed on/off override for the secret-feature
// roulette. Rows are sparse: a feature (or the master switch) with no row
// defaults to ENABLED. The canonical feature roster + client-side eligibility
// live in the frontend (utils/secretFeatures.ts) — the backend stores only the
// admin override, so it never has to duplicate the pool. Foreshadows the future
// role-based access management model (per-feature → per-role).
type SecretFeatureFlag struct {
	Key string `gorm:"primaryKey;size:64" json:"key"`
	// No `default:` tag on purpose: GORM omits a zero-value (false) field that
	// carries a default, so `Enabled:false` would silently store true. Rows are
	// always written with an explicit value via upsert, and absence resolves to
	// enabled=true in the service layer (fail-open) — so no DB default is needed.
	Enabled   bool      `gorm:"not null" json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SecretFeatureConfig is the admin-facing view: the resolved master switch plus
// the sparse map of explicit per-feature overrides (absent key ⇒ enabled).
type SecretFeatureConfig struct {
	RouletteEnabled bool            `json:"rouletteEnabled"`
	Features        map[string]bool `json:"features"`
}

// SecretFeaturePublicState is the anonymous-readable state the footer roulette
// consumes to enforce admin toggles. disabledKeys is the (usually empty) set of
// features explicitly turned off; everything else stays eligible client-side.
type SecretFeaturePublicState struct {
	RouletteEnabled bool     `json:"rouletteEnabled"`
	DisabledKeys    []string `json:"disabledKeys"`
}

// SetSecretFeatureFlagRequest is the PUT body for both the master switch and a
// per-feature toggle.
type SetSecretFeatureFlagRequest struct {
	Enabled bool `json:"enabled"`
}
