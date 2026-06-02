package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeEraser struct {
	byUser, byAnon string
}

func (f *fakeEraser) EraseByUserID(_ context.Context, id string) error      { f.byUser = id; return nil }
func (f *fakeEraser) EraseByAnonymousID(_ context.Context, id string) error { f.byAnon = id; return nil }

func TestErase_ByUserID(t *testing.T) {
	er := &fakeEraser{}
	h := NewAdminHandler(er)
	req := httptest.NewRequest(http.MethodPost, "/internal/erase", strings.NewReader(`{"user_id":"u1"}`))
	rec := httptest.NewRecorder()
	h.Erase(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if er.byUser != "u1" {
		t.Fatalf("expected erase by user u1, got %q", er.byUser)
	}
}

func TestErase_RequiresAnIdentifier(t *testing.T) {
	h := NewAdminHandler(&fakeEraser{})
	req := httptest.NewRequest(http.MethodPost, "/internal/erase", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Erase(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
