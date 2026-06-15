package capability

import (
	"context"
	"testing"
)

type stubHealthClient struct {
	status int
	body   []byte
	err    error
}

func (s stubHealthClient) GetScraperHealth(_ context.Context) (int, []byte, error) {
	return s.status, s.body, s.err
}

func TestScraperHealth_Parse(t *testing.T) {
	// Mirrors the real scraper response shape: enriched providers map with
	// top-level `up` bool + separate `playable` map.
	body := []byte(`{"success":true,"data":{"providers":{"allanime":{"up":true,"provider":"allanime","stages":{}},"nineanime":{"up":false,"provider":"nineanime","stages":{}}},"playable":{"allanime":true}}}`)
	h := NewScraperHealth(stubHealthClient{status: 200, body: body})
	got, err := h.ProviderHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	al := got["allanime"]
	if !al.Up {
		t.Errorf("allanime: expected Up=true, got false")
	}
	if al.Playable == nil || !*al.Playable {
		t.Errorf("allanime: expected Playable=true, got %v", al.Playable)
	}
	ni := got["nineanime"]
	if ni.Up {
		t.Errorf("nineanime: expected Up=false, got true")
	}
	if ni.Playable != nil {
		t.Errorf("nineanime: expected Playable=nil (absent from playable map), got %v", ni.Playable)
	}
}

func TestScraperHealth_Non200(t *testing.T) {
	h := NewScraperHealth(stubHealthClient{status: 503, body: []byte(`{}`)})
	if _, err := h.ProviderHealth(context.Background()); err == nil {
		t.Error("expected error on non-200 status")
	}
}

func TestScraperHealth_ClientError(t *testing.T) {
	h := NewScraperHealth(stubHealthClient{status: 0, body: nil, err: context.DeadlineExceeded})
	if _, err := h.ProviderHealth(context.Background()); err == nil {
		t.Error("expected error when client errors")
	}
}

func TestScraperHealth_BadJSON(t *testing.T) {
	h := NewScraperHealth(stubHealthClient{status: 200, body: []byte(`not json`)})
	if _, err := h.ProviderHealth(context.Background()); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestScraperHealth_EmptyProviders(t *testing.T) {
	body := []byte(`{"success":true,"data":{"providers":{},"playable":{}}}`)
	h := NewScraperHealth(stubHealthClient{status: 200, body: body})
	got, err := h.ProviderHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}
