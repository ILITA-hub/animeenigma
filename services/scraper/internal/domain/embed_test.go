package domain

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// fakeExtractor implements EmbedExtractor for tests. Match logic is a simple
// substring check on the embed URL.
type fakeExtractor struct {
	name  string
	match string
}

func (f fakeExtractor) Name() string                  { return f.name }
func (f fakeExtractor) Matches(embedURL string) bool  { return strings.Contains(embedURL, f.match) }
func (f fakeExtractor) Extract(_ context.Context, _ string, _ http.Header) (*Stream, error) {
	return &Stream{}, nil
}

// TestEmbedExtractorInterface_Compiles is a compile-time assertion: any
// consumer who claims to implement EmbedExtractor must satisfy the full
// (Name, Matches, Extract) method set.
func TestEmbedExtractorInterface_Compiles(t *testing.T) {
	t.Parallel()
	var _ EmbedExtractor = (*fakeExtractor)(nil)
	var e EmbedExtractor = fakeExtractor{name: "x", match: "x"}
	if e.Name() != "x" {
		t.Errorf("fakeExtractor.Name() = %q; want %q", e.Name(), "x")
	}
}

// TestRegistry_RegisterAndFind verifies the basic Register / Find flow.
func TestRegistry_RegisterAndFind(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(fakeExtractor{name: "megacloud", match: "megacloud"})
	r.Register(fakeExtractor{name: "kwik", match: "kwik.cx"})

	got, err := r.Find("https://megacloud.tv/embed-2/abc")
	if err != nil {
		t.Fatalf("Find: unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("Find returned nil extractor")
	}
	if got.Name() != "megacloud" {
		t.Errorf("Find returned extractor %q; want %q", got.Name(), "megacloud")
	}
}

// TestRegistry_RegistrationOrderPreserved verifies that Names() returns
// extractors in the order they were registered. This matters because Find
// iterates in registration order and returns the first match.
func TestRegistry_RegistrationOrderPreserved(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(fakeExtractor{name: "megacloud", match: "megacloud"})
	r.Register(fakeExtractor{name: "kwik", match: "kwik"})
	r.Register(fakeExtractor{name: "streamtape", match: "streamtape"})

	got := r.Names()
	want := []string{"megacloud", "kwik", "streamtape"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v; want %v", got, want)
	}
}

// TestRegistry_FindReturnsFirstMatch verifies that when two extractors would
// both match a URL, the first-registered wins. This is the "primary extractor
// first" contract.
func TestRegistry_FindReturnsFirstMatch(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(fakeExtractor{name: "primary", match: "shared"})
	r.Register(fakeExtractor{name: "secondary", match: "shared"})

	got, err := r.Find("https://shared.example.com/x")
	if err != nil {
		t.Fatalf("Find: unexpected err: %v", err)
	}
	if got.Name() != "primary" {
		t.Errorf("Find returned %q; want %q (first-registered)", got.Name(), "primary")
	}
}

// TestRegistry_FindNoMatch verifies that an unknown embed URL returns
// (nil, ErrNoMatchingExtractor).
func TestRegistry_FindNoMatch(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(fakeExtractor{name: "megacloud", match: "megacloud"})

	got, err := r.Find("https://unknown-embed.example.com/x")
	if got != nil {
		t.Errorf("Find returned non-nil extractor %v; want nil on miss", got)
	}
	if !errors.Is(err, ErrNoMatchingExtractor) {
		t.Errorf("Find err = %v; want errors.Is match ErrNoMatchingExtractor", err)
	}
}

// TestRegistry_EmptyRegistryNames verifies Names() returns an empty (non-nil)
// slice for a fresh registry — convenient for JSON marshaling of /health.
func TestRegistry_EmptyRegistryNames(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	got := r.Names()
	if got == nil {
		t.Errorf("Names() on empty registry = nil; want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("Names() on empty registry = %v; want empty", got)
	}
}
