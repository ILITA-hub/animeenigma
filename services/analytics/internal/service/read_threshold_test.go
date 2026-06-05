package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeHashWriter records the last HSetThresholds call so the publish path is
// asserted without a live Redis. Handwritten fake — no testify/mock.
type fakeHashWriter struct {
	calls   int
	lastKey string
	lastTTL time.Duration
	last    map[string]string
	err     error
}

func (f *fakeHashWriter) HSetThresholds(_ context.Context, key string, fields map[string]string, ttl time.Duration) error {
	f.calls++
	f.lastKey = key
	f.lastTTL = ttl
	f.last = fields
	return f.err
}

func TestPublishReadThresholds(t *testing.T) {
	t.Run("publishes op|table fields with 48h ttl", func(t *testing.T) {
		fw := &fakeHashWriter{}
		s := NewReadThresholdService(nil, fw)
		err := s.PublishReadThresholds(context.Background(), map[string]float64{
			"catalog.X|animes": 80,
			"player.Y|watch":   12.5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw.calls != 1 {
			t.Fatalf("want 1 write, got %d", fw.calls)
		}
		if fw.lastKey != ReadThresholdsHashKey {
			t.Fatalf("want key %q, got %q", ReadThresholdsHashKey, fw.lastKey)
		}
		if fw.lastTTL != defaultHashTTL {
			t.Fatalf("want ttl %v, got %v", defaultHashTTL, fw.lastTTL)
		}
		if got := fw.last["catalog.X|animes"]; got != "80" {
			t.Fatalf("want field value 80, got %q", got)
		}
		if got := fw.last["player.Y|watch"]; got != "12.5" {
			t.Fatalf("want field value 12.5, got %q", got)
		}
	})

	t.Run("empty map is a no-op (never blanks live thresholds)", func(t *testing.T) {
		fw := &fakeHashWriter{}
		s := NewReadThresholdService(nil, fw)
		if err := s.PublishReadThresholds(context.Background(), map[string]float64{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fw.calls != 0 {
			t.Fatalf("empty map must NOT write, got %d writes", fw.calls)
		}
	})

	t.Run("propagates writer error", func(t *testing.T) {
		fw := &fakeHashWriter{err: errors.New("redis down")}
		s := NewReadThresholdService(nil, fw)
		if err := s.PublishReadThresholds(context.Background(), map[string]float64{"a|b": 1}); err == nil {
			t.Fatal("want error from writer, got nil")
		}
	})
}

func TestRecompute_NilConnOrWriterIsNoOp(t *testing.T) {
	// Degraded boot: a nil ClickHouse connection (dualwrite fell back to PG-only)
	// or a nil writer must be a no-op success, not a panic — the existing 48h
	// hash carries the GORM services until CH/Redis return.
	s := NewReadThresholdService(nil, &fakeHashWriter{})
	if err := s.Recompute(context.Background()); err != nil {
		t.Fatalf("nil conn Recompute should be a no-op success, got %v", err)
	}
	s2 := NewReadThresholdService(nil, nil)
	if err := s2.Recompute(context.Background()); err != nil {
		t.Fatalf("nil writer Recompute should be a no-op success, got %v", err)
	}
}
