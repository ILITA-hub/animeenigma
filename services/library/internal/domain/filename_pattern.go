package domain

import "time"

// FilenamePattern is the Go-side mirror of the library_filename_patterns
// row defined in migrations/003_library_filename_patterns.sql. Loaded
// once at startup by the filename detector and compiled into a
// uploader → *regexp.Regexp map.
//
// PatternRegex MUST have exactly ONE capture group enclosing the
// episode number. The detector parses the captured value via
// strconv.Atoi and clamps to [1, 9999].
type FilenamePattern struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	Uploader     string    `gorm:"type:text;not null;column:uploader" json:"uploader"`
	PatternRegex string    `gorm:"type:text;not null;column:pattern_regex" json:"pattern_regex"`
	Example      string    `gorm:"type:text;column:example" json:"example,omitempty"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName pins the table name.
func (FilenamePattern) TableName() string { return "library_filename_patterns" }
