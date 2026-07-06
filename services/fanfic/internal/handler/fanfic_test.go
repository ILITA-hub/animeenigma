package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
)

type fakeGen struct{ events []string }

// NOTE: emit is typed as the named service.Emit, not the structurally
// identical literal func(string, any) error — see the comment on the
// generator interface in fanfic.go. This is a deliberate deviation from the
// task-6 brief's verbatim test code, required for *fakeGen to actually
// satisfy the same interface as the real *service.Generator (Go interface
// satisfaction requires identical parameter types; a defined type is never
// identical to an unnamed type with the same underlying signature).
func (f *fakeGen) Generate(_ context.Context, userID string, _ domain.GenerateRequest, emit service.Emit) error {
	_ = emit("meta", map[string]any{"id": "x", "user": userID})
	_ = emit("delta", map[string]any{"text": "# T\n\nhi"})
	_ = emit("done", map[string]any{"id": "x"})
	return nil
}

type fakeLib struct{ deleted bool }

func (f *fakeLib) List(_ context.Context, _ string, _, _ int) ([]domain.Fanfic, int64, error) {
	return []domain.Fanfic{{ID: "x", Title: "T"}}, 1, nil
}
func (f *fakeLib) Get(_ context.Context, _, id string) (*domain.Fanfic, error) {
	return &domain.Fanfic{ID: id, Title: "T", Content: "hi"}, nil
}
func (f *fakeLib) SoftDelete(_ context.Context, _, _ string) error { f.deleted = true; return nil }

func withUser(r *http.Request) *http.Request {
	claims := &authz.Claims{UserID: "u-1"}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

func TestGenerate_SSE(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/generate",
		strings.NewReader(`{"anime":{"title":"Frieren"},"length":"oneshot","pov":"third","rating":"teen","language":"ru"}`)))
	rec := httptest.NewRecorder()
	h.Generate(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: meta", "event: delta", "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE body missing %q; got:\n%s", want, body)
		}
	}
}

func TestGenerate_ValidationError(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/generate",
		strings.NewReader(`{"anime":{"title":""},"length":"oneshot","pov":"third","rating":"teen","language":"ru"}`)))
	rec := httptest.NewRecorder()
	h.Generate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTags(t *testing.T) {
	h := NewHandler(&fakeGen{}, &fakeLib{}, nil)
	rec := httptest.NewRecorder()
	h.Tags(rec, httptest.NewRequest(http.MethodGet, "/api/fanfic/tags", nil))
	var resp struct {
		Data []domain.Tag `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) == 0 {
		t.Error("expected curated tags")
	}
}
