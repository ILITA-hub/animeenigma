package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SkipDetected    = "detected"
	SkipNoMatch     = "no_match"
	SkipPendingFP   = "pending_fp"
	SkipUnreachable = "unreachable"
	// SkipAniskip: this side wasn't probed because crowdsourced AniSkip data
	// already covers it (owner directive 2026-07-18 — don't spend probe
	// budget re-verifying what AniSkip has; catalog serves AniSkip for it).
	// Terminal like no_match: never served from here, never re-dued.
	SkipAniskip = "aniskip"

	SkipKindOp = "op"
	SkipKindEd = "ed"
)

// SkipTiming: one row per (anime × provider × team × episode) skip probe.
type SkipTiming struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())" json:"id"`
	AnimeID    string    `gorm:"type:uuid;uniqueIndex:idx_skip_unit" json:"anime_id"`
	Provider   string    `gorm:"size:64;uniqueIndex:idx_skip_unit" json:"provider"`
	Team       string    `gorm:"size:128;uniqueIndex:idx_skip_unit;default:''" json:"team,omitempty"`
	Episode    int       `gorm:"uniqueIndex:idx_skip_unit" json:"episode"`
	OpStart    float64   `json:"op_start"`
	OpEnd      float64   `json:"op_end"`
	EdStart    float64   `json:"ed_start"`
	EdEnd      float64   `json:"ed_end"`
	OpStatus   string    `gorm:"size:16" json:"op_status"`
	EdStatus   string    `gorm:"size:16" json:"ed_status"`
	Confidence float64   `json:"confidence"`
	PairTried  bool      `json:"pair_tried,omitempty"`
	Fails      int       `json:"fails,omitempty"`
	ProbedAt   time.Time `json:"probed_at"`
}

// SkipFingerprint: season fingerprint, several per (anime, kind) allowed.
type SkipFingerprint struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())" json:"id"`
	AnimeID    string    `gorm:"type:uuid;index:idx_skip_fp_anime" json:"anime_id"`
	Kind       string    `gorm:"size:4;index:idx_skip_fp_anime" json:"kind"`
	Fp         FpInts    `gorm:"type:jsonb" json:"-"`
	Length     float64   `json:"length"`
	SourceNote string    `gorm:"size:128" json:"source_note"`
	CreatedAt  time.Time `json:"created_at"`
}

// FpInts serializes the raw chromaprint frames as JSON (sqlite + postgres).
type FpInts []uint32

func (f FpInts) Value() (driver.Value, error) { return json.Marshal(f) }

func (f *FpInts) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*f = nil
		return nil
	case []byte:
		return json.Unmarshal(v, f)
	case string:
		return json.Unmarshal([]byte(v), f)
	}
	return fmt.Errorf("fpints: unsupported scan type %T", src)
}

// BeforeCreate fills the UUID app-side so sqlite tests (no gen_random_uuid)
// behave like postgres.
func (t *SkipTiming) BeforeCreate(*gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	return nil
}

// BeforeCreate fills the UUID app-side so sqlite tests (no gen_random_uuid)
// behave like postgres.
func (f *SkipFingerprint) BeforeCreate(*gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	return nil
}
