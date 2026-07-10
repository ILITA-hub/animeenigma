package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// SettingsJSON is a free-form knob object persisted as JSON text (dialect-neutral,
// like StringList — works on Postgres runtime AND the sqlite in-memory test DB).
// Empty ⇒ "{}". It round-trips verbatim: the service validates it is a JSON object,
// the FE owns its shape via the descriptor registry.
type SettingsJSON []byte

func (s SettingsJSON) raw() []byte {
	if len(s) == 0 {
		return []byte("{}")
	}
	return s
}

func (s SettingsJSON) Value() (driver.Value, error) { return string(s.raw()), nil }

func (s *SettingsJSON) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		*s = SettingsJSON("{}")
	case []byte:
		*s = SettingsJSON(append([]byte(nil), t...))
	case string:
		*s = SettingsJSON(t)
	default:
		return errors.New("SettingsJSON: unsupported Scan type")
	}
	if len(*s) == 0 {
		*s = SettingsJSON("{}")
	}
	return nil
}

// MarshalJSON emits the raw object (so the wire shows `"settings":{...}`, not a
// base64 []byte). UnmarshalJSON stores the incoming object bytes verbatim.
func (s SettingsJSON) MarshalJSON() ([]byte, error)  { return s.raw(), nil }
func (s *SettingsJSON) UnmarshalJSON(b []byte) error { *s = SettingsJSON(append([]byte(nil), b...)); return nil }

// MaintenanceRoutine is one admin-controllable background routine's intent+status.
//
// GORM gotcha: NO `default:` tag on Enabled — GORM omits a zero-value bool carrying
// a default, so a future false would silently store true. Seed writes it explicitly;
// updates go through a column-scoped Updates map (see repo).
type MaintenanceRoutine struct {
	ID          string       `gorm:"primaryKey;size:64" json:"id"`
	Enabled     bool         `gorm:"not null" json:"enabled"`
	Settings    SettingsJSON `gorm:"type:text" json:"settings"`
	LastRunAt   *time.Time   `json:"lastRunAt"`
	LastOK      *bool        `json:"lastOk"`
	LastSummary string       `gorm:"size:512" json:"lastSummary"`
	NextRunAt   *time.Time   `json:"nextRunAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// SeedRoutines returns insert-if-absent defaults — all enabled, knob values equal to
// today's real behavior, so first boot changes nothing. Slice order = admin-list
// display order (host routines first, then in-cluster).
func SeedRoutines() []MaintenanceRoutine {
	r := func(id, settings string) MaintenanceRoutine {
		return MaintenanceRoutine{ID: id, Enabled: true, Settings: SettingsJSON(settings)}
	}
	return []MaintenanceRoutine{
		r("maintenance_bot", `{"auto_apply_max_risk":"medium","suppressed_alerts":[]}`),
		r("provider_recovery", `{"model":"sonnet"}`),
		r("git_autosync", `{}`),
		r("disk_prune", `{"high_water_pct":80}`),
		r("build_cache_prune", `{}`),
		r("subtitle_probe", `{}`),
		r("shikimori_sync", `{}`),
		r("playability_canary", `{}`),
		r("provider_self_heal", `{"promote_after":"24h","probe_every":"6h"}`),
	}
}

// Compile-time proof SettingsJSON satisfies the GORM interfaces.
var _ driver.Valuer = SettingsJSON(nil)
var _ json.Marshaler = SettingsJSON(nil)
