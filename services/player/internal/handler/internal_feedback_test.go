package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newInternalRouter(h *AdminReportsHandler) http.Handler {
	r := chi.NewRouter()
	r.Post("/internal/feedback", h.CreateInternal)
	r.Post("/internal/feedback/{id}/attachments", h.UploadAttachmentInternal)
	r.Patch("/internal/feedback/{id}/status", h.SetStatusInternal)
	r.Get("/admin/reports/{id}/attachments/{name}", h.GetAttachment)
	return r
}

func createEntry(t *testing.T, router http.Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("create: parse: %v", err)
	}
	id := resp.Data["id"]
	if id == "" {
		t.Fatalf("create: empty id in %s", rec.Body.String())
	}
	return id
}

func TestInternalFeedback_CreateWritesEntry(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	router := newInternalRouter(h)

	id := createEntry(t, router, `{
		"username": "тест юзер",
		"user_id": "tg:42",
		"player_type": "telegram",
		"category": "issue",
		"description": "что-то сломалось",
		"source": "telegram",
		"telegram_meta": {"message_id": 7, "forwarded_from": "SomeChannel"}
	}`)

	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		t.Fatalf("entry file not written: %v", err)
	}
	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("entry not valid JSON: %v", err)
	}
	if entry["description"] != "что-то сломалось" || entry["player_type"] != "telegram" {
		t.Errorf("unexpected entry content: %v", entry)
	}
	if entry["username"] != "тест юзер" {
		t.Errorf("original username must be preserved in JSON, got %v", entry["username"])
	}
	// Filename must be sanitized (no spaces/cyrillic) and prefixed with a timestamp.
	if strings.ContainsAny(id, " /\\") || !strings.HasSuffix(id, "_telegram") {
		t.Errorf("unexpected id shape: %q", id)
	}
	meta, _ := entry["telegram_meta"].(map[string]interface{})
	if meta == nil || meta["forwarded_from"] != "SomeChannel" {
		t.Errorf("telegram_meta not persisted: %v", entry["telegram_meta"])
	}
}

func TestInternalFeedback_CreateRejectsBadInput(t *testing.T) {
	h, _ := newTestReportsHandler(t)
	router := newInternalRouter(h)

	for name, body := range map[string]string{
		"bad player_type":   `{"username":"u","player_type":"kodik","description":"x"}`,
		"empty description": `{"username":"u","player_type":"telegram","description":"  "}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/internal/feedback", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", name, rec.Code)
		}
	}
}

func TestInternalFeedback_SetStatus(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	router := newInternalRouter(h)
	id := createEntry(t, router, `{"username":"u","player_type":"telegram","description":"x","source":"telegram"}`)

	req := httptest.NewRequest(http.MethodPatch, "/internal/feedback/"+id+"/status", strings.NewReader(`{"status":"in_progress"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set status: %d %s", rec.Code, rec.Body.String())
	}

	statuses := map[string]feedbackStatusEntry{}
	data, err := os.ReadFile(filepath.Join(dir, statusFileName))
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	if err := json.Unmarshal(data, &statuses); err != nil {
		t.Fatalf("sidecar parse: %v", err)
	}
	if statuses[id].Status != "in_progress" || statuses[id].UpdatedBy != "maintenance-bot" {
		t.Errorf("unexpected sidecar entry: %+v", statuses[id])
	}

	// invalid status rejected
	req = httptest.NewRequest(http.MethodPatch, "/internal/feedback/"+id+"/status", strings.NewReader(`{"status":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid status: expected 400, got %d", rec.Code)
	}
}

func TestInternalFeedback_AttachmentUploadAndServe(t *testing.T) {
	h, dir := newTestReportsHandler(t)
	router := newInternalRouter(h)
	id := createEntry(t, router, `{"username":"u","player_type":"telegram","description":"with file","source":"telegram"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "скрин 1.png")
	fw.Write([]byte("PNGDATA"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/internal/feedback/"+id+"/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("upload parse: %v", err)
	}
	name := resp.Data.Name
	if name == "" || strings.ContainsAny(name, " /\\") {
		t.Fatalf("unexpected stored name %q", name)
	}

	// File on disk under _attachments/{id}/
	if _, err := os.Stat(filepath.Join(dir, attachmentsDirName, id, name)); err != nil {
		t.Fatalf("attachment not on disk: %v", err)
	}

	// Entry JSON updated
	data, _ := os.ReadFile(filepath.Join(dir, id+".json"))
	var entry map[string]interface{}
	json.Unmarshal(data, &entry)
	atts, _ := entry["attachments"].([]interface{})
	if len(atts) != 1 || atts[0] != name {
		t.Errorf("attachments not recorded on entry: %v", entry["attachments"])
	}

	// Served back via the admin route
	req = httptest.NewRequest(http.MethodGet, "/admin/reports/"+id+"/attachments/"+name, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGDATA" {
		t.Errorf("serve: %d body %q", rec.Code, rec.Body.String())
	}

	// Traversal guard
	req = httptest.NewRequest(http.MethodGet, "/admin/reports/"+id+"/attachments/..%2f"+id+".json", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("traversal must be rejected, got 200")
	}

	// The _attachments dir must not surface in List()
	listReq := httptest.NewRequest(http.MethodGet, "/admin/reports", nil)
	listRec := httptest.NewRecorder()
	lr := chi.NewRouter()
	lr.Get("/admin/reports", h.List)
	lr.ServeHTTP(listRec, listReq)
	var list listResp
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("list parse: %v", err)
	}
	if list.Data.Total != 1 {
		t.Errorf("expected 1 entry in list, got %d", list.Data.Total)
	}
	if len(list.Data.Items) == 1 && len(list.Data.Items[0].Attachments) != 1 {
		t.Errorf("list row should carry attachments, got %+v", list.Data.Items[0].Attachments)
	}
}
