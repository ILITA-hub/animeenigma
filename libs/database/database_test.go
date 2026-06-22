package database

import (
	"testing"
	"time"
)

func TestResolvePoolSettings(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantMaxOpen  int
		wantMaxIdle  int
		wantLifetime time.Duration
	}{
		{
			name:         "zero values fall back to fleet-safe defaults",
			cfg:          Config{},
			wantMaxOpen:  10,
			wantMaxIdle:  2,
			wantLifetime: time.Hour,
		},
		{
			name:         "explicit non-zero values pass through unchanged",
			cfg:          Config{MaxOpenConns: 50, MaxIdleConns: 8, ConnMaxLifetime: 30 * time.Minute},
			wantMaxOpen:  50,
			wantMaxIdle:  8,
			wantLifetime: 30 * time.Minute,
		},
		{
			name:         "negative values fall back to defaults",
			cfg:          Config{MaxOpenConns: -1, MaxIdleConns: -5, ConnMaxLifetime: -time.Second},
			wantMaxOpen:  10,
			wantMaxIdle:  2,
			wantLifetime: time.Hour,
		},
		{
			name:         "only max-open set; idle and lifetime default",
			cfg:          Config{MaxOpenConns: 20},
			wantMaxOpen:  20,
			wantMaxIdle:  2,
			wantLifetime: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMaxOpen, gotMaxIdle, gotLifetime := resolvePoolSettings(tt.cfg)
			if gotMaxOpen != tt.wantMaxOpen {
				t.Errorf("maxOpen = %d, want %d", gotMaxOpen, tt.wantMaxOpen)
			}
			if gotMaxIdle != tt.wantMaxIdle {
				t.Errorf("maxIdle = %d, want %d", gotMaxIdle, tt.wantMaxIdle)
			}
			if gotLifetime != tt.wantLifetime {
				t.Errorf("lifetime = %v, want %v", gotLifetime, tt.wantLifetime)
			}
		})
	}
}
