package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/chi/v5"
)

// writeReport drops a minimal report JSON into dir using the production
// filename shape `{ts}_{user}_{type}.json`.
func writeReport(t *testing.T, dir, ts, user, ptype string, body map[string]interface{}) string {
	t.Helper()
	if body == nil {
		body = map[string]interface{}{}
	}
	body["username"] = user
	body["player_type"] = ptype
	body["timestamp"] = ts
	data, _ := json.MarshalIndent(body, "", "  ")
	name := ts + "_" + user + "_" + ptype + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return strings.TrimSuffix(name, ".json")
}

func newTestReportsHandler(t *testing.T) (*AdminReportsHandler, string) {
	t.Helper()
	dir := t.TempDir()
	return NewAdminReportsHandler(logger.Default(), dir, nil), dir
}

type listResp struct {
	Success bool `json:"success"`
	Data    struct {
		Items    []reportMeta `json:"items"`
		Total    int          `json:"total"`
		Page     int          `json:"page"`
		PageSize int          `json:"page_size"`
	} `json:"data"`
}

func TestAdminReports_List_SortFilterPaginate(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	writeReport(t, dir, "2026-06-01T10-00-00", "alice", "feedback", map[string]interface{}{"category": "bug", "description": "old bug"})
	writeReport(t, dir, "2026-06-03T10-00-00", "bob", "feedback", map[string]interface{}{"category": "feature", "description": "new idea"})
	writeReport(t, dir, "2026-06-02T10-00-00", "carol", "kodik", map[string]interface{}{"category": "issue", "description": "mid issue"})
	// sidecar status + decoy files must be ignored by the listing.
	_ = os.WriteFile(filepath.Join(dir, statusFileName), []byte(`{}`), 0600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0600)

	// Unfiltered: newest first by timestamp prefix.
	r := httptest.NewRequest(http.MethodGet, "/api/admin/reports", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp listResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if resp.Data.Total != 3 {
		t.Fatalf("total = %d, want 3", resp.Data.Total)
	}
	if resp.Data.Items[0].Username != "bob" {
		t.Errorf("first item = %q, want bob (newest)", resp.Data.Items[0].Username)
	}
	if resp.Data.Items[0].Status != "new" {
		t.Errorf("default status = %q, want new", resp.Data.Items[0].Status)
	}

	// Category filter.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?category=bug", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Username != "alice" {
		t.Errorf("category filter: total=%d items=%v", resp.Data.Total, resp.Data.Items)
	}

	// Type filter.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?type=kodik", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].PlayerType != "kodik" {
		t.Errorf("type filter: total=%d", resp.Data.Total)
	}

	// Pagination.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?page=1&page_size=2", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 3 || len(resp.Data.Items) != 2 {
		t.Errorf("paginate: total=%d items=%d, want total=3 items=2", resp.Data.Total, len(resp.Data.Items))
	}

	// Username filter: case-insensitive substring match.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?username=ALI", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Username != "alice" {
		t.Errorf("username filter: total=%d items=%v, want only alice", resp.Data.Total, resp.Data.Items)
	}

	// Username filter with no match yields an empty page, and surrounding
	// whitespace is trimmed before matching.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?username=%20zzz%20", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 0 {
		t.Errorf("username no-match: total=%d, want 0", resp.Data.Total)
	}

	// Username filter combines with category.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?username=bob&category=feature", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	resp = listResp{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Username != "bob" {
		t.Errorf("username+category filter: total=%d, want 1 (bob)", resp.Data.Total)
	}
}

func TestAdminReports_SetStatus_RoundTripAndFilter(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-05T12-00-00", "dave", "feedback", map[string]interface{}{"category": "bug", "description": "x"})

	// PATCH status -> resolved.
	body := strings.NewReader(`{"status":"resolved"}`)
	r := httptest.NewRequest(http.MethodPatch, "/api/admin/reports/"+id+"/status", body)
	r = withURLParam(r, "id", id)
	r = r.WithContext(authz.ContextWithClaims(r.Context(), &authz.Claims{Username: "admin1"}))
	w := httptest.NewRecorder()
	h.SetStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("set status code = %d (body=%s)", w.Code, w.Body.String())
	}

	// Sidecar file should reflect it.
	raw, _ := os.ReadFile(filepath.Join(dir, statusFileName))
	if !strings.Contains(string(raw), "resolved") || !strings.Contains(string(raw), "admin1") {
		t.Fatalf("sidecar missing status/updater: %s", raw)
	}

	// List now filters by status=resolved.
	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?status=resolved", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	var resp listResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Status != "resolved" {
		t.Errorf("status filter: total=%d", resp.Data.Total)
	}
}

func TestAdminReports_SetStatus_InvalidEnum(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-05T12-00-00", "dave", "feedback", nil)
	r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"banana"}`))
	r = withURLParam(r, "id", id)
	w := httptest.NewRecorder()
	h.SetStatus(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid status code = %d, want 400", w.Code)
	}
}

func TestAdminReports_SetStatus_AIDoneAccepted(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-05T12-00-00", "dave", "feedback", map[string]interface{}{"category": "bug", "description": "x"})

	body := strings.NewReader(`{"status":"ai_done"}`)
	r := httptest.NewRequest(http.MethodPatch, "/api/admin/reports/"+id+"/status", body)
	r = withURLParam(r, "id", id)
	r = r.WithContext(authz.ContextWithClaims(r.Context(), &authz.Claims{Username: "agent"}))
	w := httptest.NewRecorder()
	h.SetStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("set ai_done code = %d (body=%s)", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/api/admin/reports?status=ai_done", nil)
	w = httptest.NewRecorder()
	h.List(w, r)
	var resp listResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 1 || resp.Data.Items[0].Status != "ai_done" {
		t.Errorf("ai_done filter: total=%d", resp.Data.Total)
	}
}

func TestAdminReports_PathTraversalRejected(t *testing.T) {
	h, _ := newTestReportsHandler(t)
	for _, bad := range []string{"../secret", "..%2fsecret", "a/b", `a\b`, "_status", "with.dot"} {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r = withURLParam(r, "id", bad)
		w := httptest.NewRecorder()
		h.Get(w, r)
		if w.Code == http.StatusOK {
			t.Errorf("id %q: expected rejection, got 200", bad)
		}
	}
}

func TestAdminReports_Get_FullPayload(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-05T12-00-00", "erin", "feedback", map[string]interface{}{
		"category":    "feature",
		"description": "full description here",
		"page_html":   "<html></html>",
	})
	r := httptest.NewRequest(http.MethodGet, "/api/admin/reports/"+id, nil)
	r = withURLParam(r, "id", id)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get code = %d", w.Code)
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["page_html"] != "<html></html>" {
		t.Errorf("detail missing page_html: %v", resp.Data["page_html"])
	}
	if resp.Data["status"] != "new" {
		t.Errorf("detail status = %v, want new", resp.Data["status"])
	}
	if resp.Data["id"] != id {
		t.Errorf("detail id = %v, want %s", resp.Data["id"], id)
	}
}

// withURLParam injects a chi URL param into the request context for handler
// unit tests (mirrors how chi populates it during routing).
func withURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestAdminReports_Get_InjectsKindSource(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	id := writeReport(t, dir, "2026-06-03T10-00-00", "claude", "feedback",
		map[string]interface{}{"description": "agent todo", "source": "owner-todo"})
	// dismiss it — Get must still resolve it (deep-link bypasses the list filter)
	_ = os.WriteFile(filepath.Join(dir, statusFileName),
		[]byte(`{"`+id+`":{"status":"not_relevant","updated_at":"x","updated_by":"y"}}`), 0600)

	r := httptest.NewRequest(http.MethodGet, "/api/admin/reports/"+id, nil)
	r = withURLParam(r, "id", id)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (deep-link to dismissed item)", w.Code)
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data["source"] != "api" {
		t.Errorf("source = %v, want api", resp.Data["source"])
	}
	if resp.Data["kind"] != "todo" {
		t.Errorf("kind = %v, want todo", resp.Data["kind"])
	}
}

func TestAdminReports_List_KindSourceActiveFilters(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	// legacy user feedback (no kind/source) → feedback / feedback_form
	idFb := writeReport(t, dir, "2026-06-01T10-00-00", "alice", "feedback", map[string]interface{}{"category": "bug", "description": "user bug"})
	// legacy telegram → feedback / telegram
	writeReport(t, dir, "2026-06-02T10-00-00", "bot", "telegram", map[string]interface{}{"description": "tg msg", "source": "telegram"})
	// legacy AI ledger → todo / api
	writeReport(t, dir, "2026-06-03T10-00-00", "claude", "feedback", map[string]interface{}{"description": "agent todo", "source": "owner-todo"})
	// explicit manual idea
	writeReport(t, dir, "2026-06-04T10-00-00", "neymik", "feedback", map[string]interface{}{"description": "an idea", "kind": "idea", "source": "manual"})

	// mark the user-feedback item not_relevant in the sidecar
	_ = os.WriteFile(filepath.Join(dir, statusFileName),
		[]byte(`{"`+idFb+`":{"status":"not_relevant","updated_at":"x","updated_by":"y"}}`), 0600)

	get := func(qs string) listResp {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/reports?"+qs, nil)
		w := httptest.NewRecorder()
		h.List(w, r)
		var resp listResp
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return resp
	}

	// kind=todo → only the api ledger item
	if r := get("kind=todo"); r.Data.Total != 1 || r.Data.Items[0].Username != "claude" {
		t.Errorf("kind=todo: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// source=telegram → only the tg item, derived kind=feedback
	if r := get("source=telegram"); r.Data.Total != 1 || r.Data.Items[0].Kind != "feedback" {
		t.Errorf("source=telegram: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// kind=idea → explicit manual idea
	if r := get("kind=idea"); r.Data.Total != 1 || r.Data.Items[0].Source != "manual" {
		t.Errorf("kind=idea: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
	// status=active → excludes the not_relevant user-feedback item (3 of 4)
	if r := get("status=active"); r.Data.Total != 3 {
		t.Errorf("status=active: total=%d, want 3", r.Data.Total)
	}
	// derived source on the legacy user item is feedback_form (when shown)
	if r := get("source=feedback_form"); r.Data.Total != 1 || r.Data.Items[0].Username != "alice" {
		t.Errorf("source=feedback_form: total=%d items=%v", r.Data.Total, r.Data.Items)
	}
}
