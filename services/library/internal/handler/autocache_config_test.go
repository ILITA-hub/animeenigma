package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// fakeConfigStore is the handler test stub — records the last Patch field
// map so tests can assert exactly which columns reached the store, and
// echoes a canned config back so the handler's OK path is exercised.
type fakeConfigStore struct {
	cfg        *domain.AutocacheConfig
	getErr     error
	patchErr   error
	lastFields map[string]any
	patchCalls int
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{cfg: &domain.AutocacheConfig{
		ID:                    1,
		Enabled:               true,
		BudgetBytes:           107374182400,
		AutoFreshDownloadDays: 10,
		AutoFreshFetchDays:    3,
		AdminFreshDays:        30,
		ActiveWatcherDays:     30,
		QualityCap:            1080,
		MinSeeders:            3,
		SweepIntervalMin:      20,
	}}
}

func (s *fakeConfigStore) Get(_ context.Context) (*domain.AutocacheConfig, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.cfg, nil
}

func (s *fakeConfigStore) Patch(_ context.Context, fields map[string]any) (*domain.AutocacheConfig, error) {
	s.patchCalls++
	s.lastFields = fields
	if s.patchErr != nil {
		return nil, s.patchErr
	}
	// Reflect min_seeders into the echoed config when present, so the
	// happy-path assertion can confirm the round-trip.
	if v, ok := fields["min_seeders"].(int); ok {
		s.cfg.MinSeeders = v
	}
	return s.cfg, nil
}

// envelope mirrors the {success,data} shape httputil.OK emits.
type envelope struct {
	Success bool                    `json:"success"`
	Data    domain.AutocacheConfig  `json:"data"`
}

// TestAutocacheConfig_Get_Envelope: GET returns 200 with all config
// fields wrapped in the {success,data} envelope.
func TestAutocacheConfig_Get_Envelope(t *testing.T) {
	store := newFakeConfigStore()
	h := NewAutocacheConfigHandler(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/library/autocache/config", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Get status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, w.Body.String())
	}
	if !env.Success {
		t.Errorf("success = false, want true")
	}
	if env.Data.BudgetBytes != 107374182400 || env.Data.MinSeeders != 3 || !env.Data.Enabled {
		t.Errorf("unexpected config payload: %+v", env.Data)
	}
}

// TestAutocacheConfig_Patch_SingleField: PATCH {"min_seeders":5} returns
// 200 and the store receives ONLY the min_seeders key.
func TestAutocacheConfig_Patch_SingleField(t *testing.T) {
	store := newFakeConfigStore()
	h := NewAutocacheConfigHandler(store, nil)

	body := bytes.NewBufferString(`{"min_seeders":5}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
	w := httptest.NewRecorder()
	h.Patch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Patch status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if store.patchCalls != 1 {
		t.Fatalf("patchCalls = %d, want 1", store.patchCalls)
	}
	if len(store.lastFields) != 1 {
		t.Fatalf("store received %d fields, want 1: %+v", len(store.lastFields), store.lastFields)
	}
	if v, ok := store.lastFields["min_seeders"].(int); !ok || v != 5 {
		t.Errorf("min_seeders field = %v, want 5", store.lastFields["min_seeders"])
	}
}

// TestAutocacheConfig_Patch_OutOfRange: a non-positive budget_bytes is
// rejected with 400 and the store is never written.
func TestAutocacheConfig_Patch_OutOfRange(t *testing.T) {
	store := newFakeConfigStore()
	h := NewAutocacheConfigHandler(store, nil)

	body := bytes.NewBufferString(`{"budget_bytes":0}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
	w := httptest.NewRecorder()
	h.Patch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Patch(budget_bytes=0) status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if store.patchCalls != 0 {
		t.Errorf("store was written %d times on an invalid patch, want 0", store.patchCalls)
	}
}

// TestAutocacheConfig_Patch_UpperBounds: WR-02 — every floor-only field now
// also has an upper bound; an absurdly-large value is rejected with 400 and the
// store is never written.
func TestAutocacheConfig_Patch_UpperBounds(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"budget_bytes over 100 TiB", `{"budget_bytes":9223372036854775807}`},
		{"auto_fresh_download_days over 3650", `{"auto_fresh_download_days":100000}`},
		{"auto_fresh_fetch_days over 3650", `{"auto_fresh_fetch_days":100000}`},
		{"admin_fresh_days over 3650", `{"admin_fresh_days":100000}`},
		{"active_watcher_days over 3650", `{"active_watcher_days":100000}`},
		{"min_seeders over 10000", `{"min_seeders":2000000000}`},
		{"sweep_interval_min over 1 week", `{"sweep_interval_min":2000000000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeConfigStore()
			h := NewAutocacheConfigHandler(store, nil)

			req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			h.Patch(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
			if store.patchCalls != 0 {
				t.Errorf("store written %d times on out-of-range value, want 0", store.patchCalls)
			}
		})
	}
}

// TestAutocacheConfig_Patch_QualityCapLadder: WR-03 — quality_cap accepts only
// the discrete ladder {480,720,1080,2160}; any other int is rejected with 400.
func TestAutocacheConfig_Patch_QualityCapLadder(t *testing.T) {
	valid := []int{480, 720, 1080, 2160}
	for _, q := range valid {
		store := newFakeConfigStore()
		h := NewAutocacheConfigHandler(store, nil)
		body := bytes.NewBufferString(`{"quality_cap":` + itoa(q) + `}`)
		req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
		w := httptest.NewRecorder()
		h.Patch(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("quality_cap=%d status = %d, want 200; body=%s", q, w.Code, w.Body.String())
		}
		if v, ok := store.lastFields["quality_cap"].(int); !ok || v != q {
			t.Errorf("quality_cap field = %v, want %d", store.lastFields["quality_cap"], q)
		}
	}

	invalid := []int{0, 137, 1081, 999999, 360, 1440}
	for _, q := range invalid {
		store := newFakeConfigStore()
		h := NewAutocacheConfigHandler(store, nil)
		body := bytes.NewBufferString(`{"quality_cap":` + itoa(q) + `}`)
		req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
		w := httptest.NewRecorder()
		h.Patch(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("quality_cap=%d status = %d, want 400; body=%s", q, w.Code, w.Body.String())
		}
		if store.patchCalls != 0 {
			t.Errorf("store written on invalid quality_cap=%d, want 0 calls", q)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// TestAutocacheConfig_Patch_MalformedBody: a non-JSON body returns 400.
func TestAutocacheConfig_Patch_MalformedBody(t *testing.T) {
	store := newFakeConfigStore()
	h := NewAutocacheConfigHandler(store, nil)

	body := bytes.NewBufferString(`{not json`)
	req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
	w := httptest.NewRecorder()
	h.Patch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Patch(malformed) status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if store.patchCalls != 0 {
		t.Errorf("store was written on a malformed body, want 0 calls")
	}
}

// TestAutocacheConfig_Patch_EmptyBody: a valid-JSON body with no writable
// keys returns 400 (no empty UPDATE).
func TestAutocacheConfig_Patch_EmptyBody(t *testing.T) {
	store := newFakeConfigStore()
	h := NewAutocacheConfigHandler(store, nil)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/library/autocache/config", body)
	w := httptest.NewRecorder()
	h.Patch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Patch(empty) status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if store.patchCalls != 0 {
		t.Errorf("store was written on an empty patch, want 0 calls")
	}
}
