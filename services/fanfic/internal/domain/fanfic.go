package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Fanfic is one generated fanfiction, owned by the user who generated it.
type Fanfic struct {
	ID               string         `gorm:"type:uuid;primaryKey" json:"id"`
	UserID           string         `gorm:"type:uuid;index;not null" json:"-"`
	AnimeID          string         `gorm:"type:uuid;index" json:"anime_id"`
	AnimeShikimoriID string         `gorm:"size:32;index" json:"anime_shikimori_id"`
	AnimeTitle       string         `gorm:"size:512" json:"anime_title"`
	AnimeJapanese    string         `gorm:"size:512" json:"anime_japanese"`
	AnimePoster      string         `gorm:"size:1024" json:"anime_poster"`
	Characters       datatypes.JSON `gorm:"type:jsonb" json:"characters"`
	Tags             datatypes.JSON `gorm:"type:jsonb" json:"tags"`
	Length           string         `gorm:"size:16" json:"length"`
	POV              string         `gorm:"size:16" json:"pov"`
	Rating           string         `gorm:"size:16" json:"rating"`
	Language         string         `gorm:"size:8" json:"language"`
	AuthorUsername   string         `gorm:"size:64" json:"author_username,omitempty"`
	SpotlightCredit  bool           `gorm:"default:false" json:"spotlight_credit"`
	AIGenerated      bool           `gorm:"default:false;index" json:"ai_generated"`
	Prompt           string         `gorm:"type:text" json:"prompt"`
	Canon            bool           `gorm:"default:false" json:"canon"`
	PartCount        int            `gorm:"default:1" json:"part_count"`
	Title            string         `gorm:"size:512" json:"title"`
	Content          string         `gorm:"type:text" json:"content"`
	Model            string         `gorm:"size:64" json:"model"`
	TokenUsage       int            `json:"token_usage"`
	Status           string         `gorm:"size:16;index" json:"status"`
	ErrorMsg         string         `gorm:"type:text" json:"error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// Status values.
const (
	StatusGenerating = "generating"
	StatusComplete   = "complete"
	StatusFailed     = "failed"
)

func (Fanfic) TableName() string { return "fanfics" }

// BeforeCreate populates ID in Go so the row gets an id on every dialect
// (Postgres's gen_random_uuid() default is not available on SQLite, which
// hosts the in-memory repo tests). Every insert goes through GORM Create, so
// this is the single dialect-independent ID-generation mechanism.
func (f *Fanfic) BeforeCreate(*gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	return nil
}
