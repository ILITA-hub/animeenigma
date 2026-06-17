package feedback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestClient_CreateStatusUpload(t *testing.T) {
	var gotCreate CreateRequest
	var gotStatus map[string]string
	var gotFile []byte
	var gotFilename string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/internal/feedback":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotCreate)
			w.Write([]byte(`{"success":true,"data":{"id":"2026-06-10T12-00-00_user_telegram","status":"new"}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/internal/feedback/abc/status":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotStatus)
			w.Write([]byte(`{"success":true,"data":{"id":"abc","status":"in_progress"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/internal/feedback/abc/attachments":
			f, hdr, err := r.FormFile("file")
			if err != nil {
				t.Errorf("form file: %v", err)
				w.WriteHeader(400)
				return
			}
			gotFilename = hdr.Filename
			gotFile, _ = io.ReadAll(f)
			w.Write([]byte(`{"success":true,"data":{"id":"abc","name":"shot.png","bytes":7}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, logger.Default())

	id, err := c.Create(CreateRequest{Username: "user", PlayerType: "telegram", Description: "hi", Source: "telegram"})
	if err != nil || id != "2026-06-10T12-00-00_user_telegram" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	if gotCreate.PlayerType != "telegram" || gotCreate.Description != "hi" {
		t.Errorf("create payload: %+v", gotCreate)
	}

	if err := c.SetStatus("abc", StatusInProgress, "maintenance-bot"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if gotStatus["status"] != "in_progress" || gotStatus["updated_by"] != "maintenance-bot" {
		t.Errorf("status payload: %+v", gotStatus)
	}

	name, err := c.UploadAttachment("abc", "shot.png", []byte("PNGDATA"))
	if err != nil || name != "shot.png" {
		t.Fatalf("upload: name=%q err=%v", name, err)
	}
	if string(gotFile) != "PNGDATA" || gotFilename != "shot.png" {
		t.Errorf("upload payload: file=%q name=%q", gotFile, gotFilename)
	}
}

// "resolved" is human-only — the client must hard-downgrade any resolved
// write to "ai_done" before it reaches the wire (CLAUDE.md feedback-triage).
func TestClient_SetStatusResolvedDowngradedToAIDone(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		sent = m["status"]
		w.Write([]byte(`{"success":true,"data":{"id":"abc","status":"ai_done"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, logger.Default())
	if err := c.SetStatus("abc", StatusResolved, "maintenance-bot"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if sent != StatusAIDone {
		t.Fatalf("resolved must be downgraded to ai_done on the wire, got %q", sent)
	}
}

func TestClient_TrySetStatusEmptyIDNoop(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	c := NewClient(srv.URL, logger.Default())
	c.TrySetStatus("", StatusInProgress)
	if called {
		t.Error("TrySetStatus with empty id must not call the API")
	}
}
