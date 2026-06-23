package domain

import "time"

type UpscaleModel struct {
	Name       string    `gorm:"type:text;primaryKey;column:name" json:"name"`
	Version    string    `gorm:"type:text;primaryKey;column:version" json:"version"`
	Checksum   string    `gorm:"type:text;not null;column:checksum" json:"checksum"`
	ObjectPath string    `gorm:"type:text;not null;column:object_path" json:"object_path"`
	Builtin    bool      `gorm:"type:boolean;not null;default:false;column:builtin" json:"builtin"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UpscaleModel) TableName() string {
	return "upscale_models"
}
