// Package domain holds the policy-service feature-flag model + pure resolver.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// Role strings mirror libs/authz roles WITHOUT importing it, so the domain stays
// dependency-free and trivially unit-testable.
const (
	RoleEveryone = "everyone"
	RoleUser     = "user"
	RoleAdmin    = "admin"
	RoleGuest    = "guest"
)

// RouletteMasterKey is the reserved flag key holding the global on/off switch for
// the «Секретная фича» roulette. Double-underscore sentinel so it can never
// collide with a real feature key. Its Roulette field carries the master state.
const RouletteMasterKey = "__roulette__"

// StringList is a []string persisted as JSON text so the same column works on
// both Postgres (runtime) and the sqlite in-memory DB used by repo tests. Using
// type:text (not Postgres text[]/jsonb) keeps it dialect-neutral.
type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	return string(b), err
}

func (s *StringList) Scan(v any) error {
	if v == nil {
		*s = StringList{}
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return errors.New("StringList: unsupported Scan type")
	}
	if len(b) == 0 {
		*s = StringList{}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(s))
}

// FeatureFlag is one admin-managed access rule. Key is the PK (string), so no
// UUID hook is needed for sqlite tests.
//
// GORM gotcha: NO `default:` tag on Roulette — GORM omits a zero-value bool that
// carries a default, so Roulette:false would silently store true. Rows are always
// written explicitly via the repo upsert; absence resolves fail-open in service.
type FeatureFlag struct {
	Key        string     `gorm:"primaryKey;size:64" json:"key"`
	Roles      StringList `gorm:"type:text" json:"roles"`
	AllowUsers StringList `gorm:"type:text" json:"allowUsers"`
	DenyUsers  StringList `gorm:"type:text" json:"denyUsers"`
	Roulette   bool       `gorm:"not null" json:"roulette"`
	FailSafe   string     `gorm:"size:16;not null" json:"failSafe"` // "admin" | "everyone"
	Label      string     `gorm:"size:128" json:"label"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Audience is the resolved targeting rule (JSON-facing, no GORM tags).
type Audience struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
}

func (f FeatureFlag) Audience() Audience {
	return Audience{Roles: f.Roles, AllowUsers: f.AllowUsers, DenyUsers: f.DenyUsers}
}

// CanAccess resolves whether (userID, role) may access this flag. Pure and
// order-sensitive: guest-deny → deny-list → allow-list → everyone → role.
func (f FeatureFlag) CanAccess(userID, role string) bool {
	if role == RoleGuest {
		return false
	}
	if userID != "" && contains(f.DenyUsers, userID) {
		return false
	}
	if userID != "" && contains(f.AllowUsers, userID) {
		return true
	}
	if contains(f.Roles, RoleEveryone) {
		return true
	}
	if role != "" && contains(f.Roles, role) {
		return true
	}
	return false
}

func contains(list StringList, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Ruleset is the compact all-flags snapshot the gateway caches (Phase 2). The
// reserved master key is collapsed into RouletteEnabled and excluded from Flags.
type Ruleset struct {
	RouletteEnabled bool                `json:"rouletteEnabled"`
	Flags           map[string]Audience `json:"flags"`
	FailSafe        map[string]string   `json:"failSafe"`
	Roulette        map[string]bool     `json:"roulette"`
}

// MineResponse is the per-user FE feed (Phase 4 consumer).
type MineResponse struct {
	Visible         []string `json:"visible"`
	Roulette        []string `json:"roulette"`
	RouletteEnabled bool     `json:"rouletteEnabled"`
}

// SeedFlags returns the insert-if-absent defaults so day-one behavior equals the
// pre-RBAC dark-ship state. admin(): admin-only (mirrors *_ADMIN_ONLY=true).
// everyone(): all-authenticated + roulette-eligible (the current SECRET_FEATURES
// roster). gacha is admin-access AND roulette-eligible but seeded roulette-OFF
// (mirrors catalog SecretFeatureDefaultsDisabled). The __roulette__ master is
// seeded separately by the service (defaults ON).
func SeedFlags() []FeatureFlag {
	admin := func(key, label string) FeatureFlag {
		return FeatureFlag{Key: key, Roles: StringList{RoleAdmin}, FailSafe: "admin", Label: label}
	}
	everyone := func(key, label string) FeatureFlag {
		return FeatureFlag{Key: key, Roles: StringList{RoleEveryone}, Roulette: true, FailSafe: "everyone", Label: label}
	}
	return []FeatureFlag{
		admin("fanfic", "Fanfic engine"),
		admin("profile-wall", "Profile showcase wall"),
		{Key: "gacha", Roles: StringList{RoleAdmin}, Roulette: false, FailSafe: "admin", Label: "Gacha «Лудка»"},
		everyone("anidle", "Anidle"),
		everyone("status", "Status page"),
		everyone("themes", "OP/ED themes"),
		everyone("game", "Game rooms"),
		everyone("downloads", "Downloads"),
		everyone("showcase-editor", "Showcase editor"),
		everyone("my-feedback", "My feedback"),
	}
}
