package domain

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestStream_HasNoIframeURL is the compile-time-equivalent guard against
// silent EN-tier → Kodik-iframe fallback. It runs in `go test ./...` which CI
// runs on every PR, so adding `IframeURL` to the Stream type fails the build
// at the lint stage.
//
// Rationale: D-DEC §2.8, SCRAPER-FOUND-03, ROADMAP §15 success #3, ISS-008.
// The AnimeLib parser shipped this exact bug at commit 9347143 and we removed
// it manually. This test prevents the next regression.
func TestStream_HasNoIframeURL(t *testing.T) {
	t.Parallel()
	streamType := reflect.TypeOf(Stream{})
	for i := 0; i < streamType.NumField(); i++ {
		field := streamType.Field(i)
		if field.Name == "IframeURL" {
			t.Fatalf("Stream type has forbidden field %q. "+
				"See provider.go top-of-file comment for rationale.", field.Name)
		}
		jsonTag := field.Tag.Get("json")
		// JSON tag format: "name,opt1,opt2" — split and look at the name segment.
		jsonName := strings.SplitN(jsonTag, ",", 2)[0]
		if jsonName == "iframe_url" {
			t.Fatalf("Stream field %q has forbidden json tag %q. "+
				"See provider.go top-of-file comment for rationale.",
				field.Name, jsonTag)
		}
	}
}

// TestStream_AllowedFields locks in the exact set of fields on Stream so
// drift is caught by review, not silently merged. Plan 15-02 declares these
// five fields; future PRs adding new fields update this list explicitly.
func TestStream_AllowedFields(t *testing.T) {
	t.Parallel()
	expected := map[string]bool{
		"Sources": true,
		"Tracks":  true,
		"Intro":   true,
		"Outro":   true,
		"Headers": true,
	}
	streamType := reflect.TypeOf(Stream{})
	for i := 0; i < streamType.NumField(); i++ {
		name := streamType.Field(i).Name
		if !expected[name] {
			t.Errorf("Stream has unexpected field %q; update TestStream_AllowedFields if intentional", name)
		}
		delete(expected, name)
	}
	for name := range expected {
		t.Errorf("Stream missing expected field %q", name)
	}
}

// TestCategoryConstants locks in the exact string values for the Category enum.
// These leak into JSON DTOs, API contracts with catalog, and observability
// labels, so we want a build-failing test if anyone renames them.
func TestCategoryConstants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		got, want Category
	}{
		{CategorySub, "sub"},
		{CategoryDub, "dub"},
		{CategoryRaw, "raw"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("Category constant = %q; want %q", tc.got, tc.want)
		}
	}
}

// fakeProvider exists only to satisfy the Provider interface at compile time.
// It is NOT registered anywhere and never called.
type fakeProvider struct{}

func (fakeProvider) Name() string { return "fake" }
func (fakeProvider) FindID(context.Context, AnimeRef) (string, error) {
	return "", nil
}
func (fakeProvider) ListEpisodes(context.Context, string) ([]Episode, error) {
	return nil, nil
}
func (fakeProvider) ListServers(context.Context, string, string) ([]Server, error) {
	return nil, nil
}
func (fakeProvider) GetStream(context.Context, string, string, string, Category) (*Stream, error) {
	return nil, nil
}
func (fakeProvider) HealthCheck(context.Context) Health {
	return Health{Provider: "fake"}
}

// TestProviderInterface_Compiles is a compile-time assertion: if `fakeProvider`
// doesn't satisfy `Provider`, this file won't compile, so the test binary
// won't build. The runtime body just exercises one method to silence
// unused-warning linters.
func TestProviderInterface_Compiles(t *testing.T) {
	t.Parallel()
	var _ Provider = (*fakeProvider)(nil)
	var p Provider = fakeProvider{}
	if p.Name() != "fake" {
		t.Errorf("fakeProvider.Name() = %q; want %q", p.Name(), "fake")
	}
}

// TestStream_JSON_OmitsEmptyOptionals verifies the `omitempty` JSON tags on
// optional fields. This matters because the scraper → catalog API contract
// expects absent fields, not empty arrays, when nothing was found.
func TestStream_JSON_OmitsEmptyOptionals(t *testing.T) {
	t.Parallel()
	s := Stream{
		Sources: []Source{{URL: "https://example.com/master.m3u8", Type: "hls"}},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"sources"`) {
		t.Errorf("JSON missing \"sources\": %s", out)
	}
	for _, k := range []string{`"tracks"`, `"intro"`, `"outro"`, `"headers"`} {
		if strings.Contains(out, k) {
			t.Errorf("JSON should omit %s when empty; got %s", k, out)
		}
	}
}
