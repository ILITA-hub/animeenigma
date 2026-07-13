package handler

// Verifies the admin List endpoint overlays live (cached) download progress onto
// downloading jobs, while leaving non-downloading rows' persisted progress_pct
// untouched and jobs without a cache entry at their persisted value.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

type fakeProgReader struct {
	vals map[string]int
	gots [][]string
}

func (f *fakeProgReader) GetProgressMany(_ context.Context, ids []string) (map[string]int, error) {
	f.gots = append(f.gots, ids)
	out := make(map[string]int, len(ids))
	for _, id := range ids {
		if v, ok := f.vals[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func TestJobsHandler_List_OverlaysLiveProgress(t *testing.T) {
	store := newFakeStoreH()
	store.listResp = []domain.Job{
		{ID: "d1", Status: domain.JobStatusDownloading, ProgressPct: 0},   // stale DB 0 → live 73
		{ID: "d2", Status: domain.JobStatusDownloading, ProgressPct: 0},   // downloading, no live entry → stays 0
		{ID: "e1", Status: domain.JobStatusEncoding, ProgressPct: 100},    // not downloading → untouched
	}
	h := NewJobsHandler(store, &fakeDiskGuard{}, &fakeCanceller{}, nil, 20, nil)
	reader := &fakeProgReader{vals: map[string]int{"d1": 73, "e1": 5}}
	h.SetProgressReader(reader)

	r := httptest.NewRequest(http.MethodGet, "/api/library/jobs?status=downloading,encoding", nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data struct {
			Jobs []domain.Job `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]int{}
	for _, j := range resp.Data.Jobs {
		got[j.ID] = j.ProgressPct
	}
	if got["d1"] != 73 {
		t.Errorf("d1 progress = %d, want 73 (live overlay)", got["d1"])
	}
	if got["d2"] != 0 {
		t.Errorf("d2 progress = %d, want 0 (no live entry)", got["d2"])
	}
	if got["e1"] != 100 {
		t.Errorf("e1 progress = %d, want 100 (encoding not overlaid despite live entry)", got["e1"])
	}

	// Only downloading jobs are looked up (e1 must not be queried).
	if len(reader.gots) != 1 {
		t.Fatalf("expected exactly one cache lookup, got %d", len(reader.gots))
	}
	for _, id := range reader.gots[0] {
		if id == "e1" {
			t.Errorf("encoding job e1 must not be looked up in the live cache")
		}
	}
}

func TestJobsHandler_List_NoProgressReader_NoOp(t *testing.T) {
	store := newFakeStoreH()
	store.listResp = []domain.Job{{ID: "d1", Status: domain.JobStatusDownloading, ProgressPct: 12}}
	h := NewJobsHandler(store, &fakeDiskGuard{}, &fakeCanceller{}, nil, 20, nil)
	// No SetProgressReader → persisted progress served as-is.

	r := httptest.NewRequest(http.MethodGet, "/api/library/jobs", nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	var resp struct {
		Data struct {
			Jobs []domain.Job `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data.Jobs) != 1 || resp.Data.Jobs[0].ProgressPct != 12 {
		t.Fatalf("want persisted pct 12 served unchanged, got %+v", resp.Data.Jobs)
	}
}
