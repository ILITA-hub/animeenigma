package service

import (
	"testing"
	"time"
)

func TestHashIP(t *testing.T) {
	day := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	t.Run("deterministic within the same day", func(t *testing.T) {
		a := HashIP("203.0.113.7", "secret-salt", day)
		b := HashIP("203.0.113.7", "secret-salt", day.Add(3*time.Hour))
		if a != b {
			t.Fatalf("same ip+day must hash equal: %s != %s", a, b)
		}
	})

	t.Run("changes the next day", func(t *testing.T) {
		a := HashIP("203.0.113.7", "secret-salt", day)
		b := HashIP("203.0.113.7", "secret-salt", day.Add(24*time.Hour))
		if a == b {
			t.Fatal("ip hash must rotate daily")
		}
	})

	t.Run("does not contain the raw ip", func(t *testing.T) {
		h := HashIP("203.0.113.7", "secret-salt", day)
		if h == "" || len(h) != 64 {
			t.Fatalf("expected 64-char hex sha256, got %q", h)
		}
	})

	t.Run("empty ip yields empty hash", func(t *testing.T) {
		if HashIP("", "salt", day) != "" {
			t.Fatal("empty ip must yield empty hash")
		}
	})
}
