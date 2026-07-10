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
	"github.com/go-chi/chi/v5"
)

type fakeGen struct {
	events []string

	// continue-specific: see fakeGen.Continue below.
	continueCalled bool
	continueEvents []string
}

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

// Continue emits continueEvents (defaults to none) and records that it was
// called, for TestContinue_StreamsWhenComplete.
func (f *fakeGen) Continue(_ context.Context, userID, id string, emit service.Emit) error {
	f.continueCalled = true
	for _, e := range f.continueEvents {
		_ = emit(e, map[string]any{"id": id, "user": userID})
	}
	return nil
}

type fakeLib struct {
	deleted     bool
	calledLimit int

	// get/getErr: when set, override the default Get behavior below, for
	// TestContinue_409OnNonComplete / TestContinue_StreamsWhenComplete.
	get    *domain.Fanfic
	getErr error
}

func (f *fakeLib) List(_ context.Context, _ string, limit, _ int) ([]domain.Fanfic, int64, error) {
	f.calledLimit = limit
	return []domain.Fanfic{{ID: "x", Title: "T"}}, 1, nil
}
func (f *fakeLib) Get(_ context.Context, _, id string) (*domain.Fanfic, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.get != nil {
		return f.get, nil
	}
	return &domain.Fanfic{ID: id, Title: "T", Content: "hi"}, nil
}
func (f *fakeLib) SoftDelete(_ context.Context, _, _ string) error { f.deleted = true; return nil }

func withUser(r *http.Request) *http.Request {
	claims := &authz.Claims{UserID: "u-1"}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

// withURLParam injects a chi route param into the request context, mirroring
// the pattern used across other services' handler tests (e.g.
// services/library/internal/handler/episodes_test.go) since this file has no
// pre-existing chi-param helper of its own.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
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

func TestList_ClampsLimit(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"limit=-1", 20},
		{"limit=0", 20},
		{"limit=500", 100},
		{"limit=50", 50},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			lib := &fakeLib{}
			h := NewHandler(&fakeGen{}, lib, nil)
			req := withUser(httptest.NewRequest(http.MethodGet, "/api/fanfic?"+tc.query, nil))
			rec := httptest.NewRecorder()
			h.List(rec, req)
			if lib.calledLimit != tc.want {
				t.Errorf("List called with limit = %d, want %d", lib.calledLimit, tc.want)
			}
		})
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

func TestContinue_409OnNonComplete(t *testing.T) {
	repo := &fakeLib{get: &domain.Fanfic{ID: "f1", UserID: "u-1", Status: domain.StatusGenerating}}
	h := NewHandler(&fakeGen{}, repo, nil)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/f1/continue", nil))
	req = withURLParam(req, "id", "f1")
	rec := httptest.NewRecorder()
	h.Continue(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestContinue_StreamsWhenComplete(t *testing.T) {
	repo := &fakeLib{get: &domain.Fanfic{ID: "f1", UserID: "u-1", Status: domain.StatusComplete}}
	gen := &fakeGen{continueEvents: []string{"meta", "delta", "done"}}
	h := NewHandler(gen, repo, nil)

	req := withUser(httptest.NewRequest(http.MethodPost, "/api/fanfic/f1/continue", nil))
	req = withURLParam(req, "id", "f1")
	rec := httptest.NewRecorder()
	h.Continue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	if !gen.continueCalled {
		t.Fatal("expected gen.Continue to be called")
	}
}
