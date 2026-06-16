package sourceranking

import (
	"context"
	"testing"
	"time"
)

// fakeSetter captures the last SetString call so tests can assert on it.
type fakeSetter struct {
	key   string
	val   string
	ttl   time.Duration
	calls int
}

func (f *fakeSetter) SetString(_ context.Context, key, val string, ttl time.Duration) error {
	f.key, f.val, f.ttl = key, val, ttl
	f.calls++
	return nil
}

func TestSetFix_Valid(t *testing.T) {
	f := &fakeSetter{}
	w := NewWriter(f)
	if err := w.SetFix(context.Background(), "uuid-1", "allanime"); err != nil {
		t.Fatalf("SetFix err = %v", err)
	}
	if f.key != "srcfix:uuid-1" {
		t.Errorf("key = %q, want srcfix:uuid-1", f.key)
	}
	if f.val != "allanime" {
		t.Errorf("val = %q, want allanime", f.val)
	}
	if f.ttl != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", f.ttl)
	}
}

func TestSetFix_UnknownProvider(t *testing.T) {
	f := &fakeSetter{}
	w := NewWriter(f)
	if err := w.SetFix(context.Background(), "uuid-1", "bogus"); err == nil {
		t.Fatal("want error for unknown provider, got nil")
	}
	if f.calls != 0 {
		t.Errorf("expected no write, got %d calls", f.calls)
	}
}

func TestSetFix_EmptyAnimeID(t *testing.T) {
	f := &fakeSetter{}
	w := NewWriter(f)
	if err := w.SetFix(context.Background(), "", "allanime"); err == nil {
		t.Fatal("want error for empty animeID, got nil")
	}
	if f.calls != 0 {
		t.Errorf("expected no write, got %d calls", f.calls)
	}
}
