package gormtrace

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInstrumentGORM_RegistersPlugin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := InstrumentGORM(db); err != nil {
		t.Fatalf("InstrumentGORM returned error: %v", err)
	}
}
