package handler

// models_test.go — tests for ModelServeHandler (T27).
//
// Uses handwritten fakes (no testify/mock), consistent with segments_test.go
// and model_admin_test.go. Covers:
//   - valid handle + existing model + fake Uploader → 200 + body + headers
//   - bad/missing sig → 401
//   - sig minted over a DIFFERENT model name → 401 (name-binding)
//   - unknown model → 404
//   - traversal name (/, .., etc.) → 400

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ── handwritten fakes ──────────────────────────────────────────────────────────

// fakeModelGetter implements modelGetter (GetLatest).
type fakeModelGetter struct {
	model *domain.UpscaleModel
	err   error
}

func (f *fakeModelGetter) GetLatest(_ context.Context, _ string) (*domain.UpscaleModel, error) {
	return f.model, f.err
}

// fakeModelObjectGetter implements modelObjectGetter (GetObject).
type fakeModelObjectGetter struct {
	data []byte
	err  error
}

func (f *fakeModelObjectGetter) GetObject(_ context.Context, _, _ string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

const modelsTestCapSecret = "models-test-secret-capability-hmac"

// mintModelHandle returns a valid exp/sig for the given model name.
func mintModelHandle(name string) (exp, sig string) {
	_, e, s := capability.MintJobHandle(name, "model", 0, 15*time.Minute)
	return e, s
}

// mintModelHandleExpired returns an already-expired exp/sig.
func mintModelHandleExpired(name string) (exp, sig string) {
	_, e, s := capability.MintJobHandle(name, "model", 0, -1*time.Minute)
	return e, s
}

// buildModelServeRouter wires a ModelServeHandler to a chi router for end-to-end
// request testing. Also calls capability.Init with the test secret (once-gated).
func buildModelServeRouter(repo modelGetter, uploader modelObjectGetter) chi.Router {
	capability.Init(modelsTestCapSecret)
	h := NewModelServeHandler(repo, uploader, "test-bucket", logger.Default())
	r := chi.NewRouter()
	r.Get("/worker/models/{name}", h.ServeModel)
	return r
}

// doModelGet issues a GET /worker/models/{name}?exp=&sig= through the router.
func doModelGet(r chi.Router, name, exp, sig string) *httptest.ResponseRecorder {
	url := "/worker/models/" + name
	if exp != "" || sig != "" {
		url += "?exp=" + exp + "&sig=" + sig
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestModelServe_ValidHandle_200 — valid name-bound handle + existing model +
// fake Uploader → 200, body == artifact bytes, X-Model-Checksum and
// X-Model-Version headers set.
func TestModelServe_ValidHandle_200(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	artifact := []byte("fake-model-tar-bytes-here")
	model := &domain.UpscaleModel{
		Name:       "realesrgan-x4plus-anime",
		Version:    "2",
		Checksum:   "abc123checksum",
		ObjectPath: "models/realesrgan-x4plus-anime/2.tar",
	}

	repo := &fakeModelGetter{model: model}
	uploader := &fakeModelObjectGetter{data: artifact}
	r := buildModelServeRouter(repo, uploader)

	exp, sig := mintModelHandle("realesrgan-x4plus-anime")
	rec := doModelGet(r, "realesrgan-x4plus-anime", exp, sig)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid handle: status = %d, want 200; body = %q", rec.Code, rec.Body.String())
	}

	got, _ := io.ReadAll(rec.Body)
	if !bytes.Equal(got, artifact) {
		t.Errorf("body = %q, want %q", got, artifact)
	}
	if got := rec.Header().Get("X-Model-Checksum"); got != model.Checksum {
		t.Errorf("X-Model-Checksum = %q, want %q", got, model.Checksum)
	}
	if got := rec.Header().Get("X-Model-Version"); got != model.Version {
		t.Errorf("X-Model-Version = %q, want %q", got, model.Version)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/x-tar") && !strings.HasPrefix(ct, "application/octet-stream") {
		t.Errorf("Content-Type = %q, want application/x-tar", ct)
	}
}

// TestModelServe_BadSig_401 — missing or corrupted sig → 401.
func TestModelServe_BadSig_401(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	repo := &fakeModelGetter{model: &domain.UpscaleModel{
		Name: "realesrgan", Version: "1", Checksum: "c", ObjectPath: "models/realesrgan/1.tar",
	}}
	uploader := &fakeModelObjectGetter{data: []byte("data")}
	r := buildModelServeRouter(repo, uploader)

	t.Run("no_params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/worker/models/realesrgan", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("no params: status = %d, want 401", rec.Code)
		}
	})

	t.Run("corrupted_sig", func(t *testing.T) {
		exp, _ := mintModelHandle("realesrgan")
		rec := doModelGet(r, "realesrgan", exp, "invalidsig00000")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("corrupted sig: status = %d, want 401", rec.Code)
		}
	})

	t.Run("expired", func(t *testing.T) {
		exp, sig := mintModelHandleExpired("realesrgan")
		rec := doModelGet(r, "realesrgan", exp, sig)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expired sig: status = %d, want 401", rec.Code)
		}
	})
}

// TestModelServe_WrongName_401 — handle minted for "modelA" cannot be used to
// fetch "modelB". This proves name-binding: the HMAC covers the model name so a
// valid handle for one name must fail for another.
func TestModelServe_WrongName_401(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	repo := &fakeModelGetter{model: &domain.UpscaleModel{
		Name: "waifu2x", Version: "1", Checksum: "c", ObjectPath: "models/waifu2x/1.tar",
	}}
	uploader := &fakeModelObjectGetter{data: []byte("data")}
	r := buildModelServeRouter(repo, uploader)

	// Mint handle for "realesrgan", then request "waifu2x".
	exp, sig := mintModelHandle("realesrgan")
	rec := doModelGet(r, "waifu2x", exp, sig)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong-name: status = %d, want 401", rec.Code)
	}
}

// TestModelServe_UnknownModel_404 — valid handle but model not in registry → 404.
func TestModelServe_UnknownModel_404(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	repo := &fakeModelGetter{err: gorm.ErrRecordNotFound}
	uploader := &fakeModelObjectGetter{data: []byte("data")}
	r := buildModelServeRouter(repo, uploader)

	exp, sig := mintModelHandle("nonexistent-model")
	rec := doModelGet(r, "nonexistent-model", exp, sig)

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown model: status = %d, want 404; body=%q", rec.Code, rec.Body.String())
	}
	// Must not leak internal detail.
	body := rec.Body.String()
	for _, tok := range []string{"bucket", "raw-library", "models/", "gorm"} {
		if strings.Contains(strings.ToLower(body), tok) {
			t.Errorf("response body leaks %q: %s", tok, body)
		}
	}
}

// TestModelServe_TraversalName_400 — names containing '/', '..', or '\' must be
// rejected before any capability check or DB lookup.
func TestModelServe_TraversalName_400(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	// The repo and uploader should never be reached for traversal inputs.
	repo := &fakeModelGetter{model: &domain.UpscaleModel{
		Name: "x", Version: "1", Checksum: "c", ObjectPath: "models/x/1.tar",
	}}
	uploader := &fakeModelObjectGetter{data: []byte("data")}

	// We need to inject the traversal name directly as a chi route param,
	// since chi's router will normalize URL paths and may 404 them. Mirror the
	// pattern used in TestSegment_ForgedJobIDTraversal_Rejected.
	h := NewModelServeHandler(repo, uploader, "test-bucket", logger.Default())

	cases := []struct {
		name    string
		rawName string
	}{
		{"dotdot", ".."},
		{"slash_in_name", "foo/bar"},
		{"backslash", "foo\\bar"},
		{"empty_injected", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("name", tc.rawName)

			req := httptest.NewRequest(http.MethodGet, "/worker/models/x", nil)
			req = req.WithContext(
				context.WithValue(req.Context(), chi.RouteCtxKey, rctx),
			)
			rec := httptest.NewRecorder()
			h.ServeModel(rec, req)

			// Traversal → 400; but empty name or other guard outcomes may also be
			// 401 (capability check fires before model lookup but after traversal
			// guard) — what matters is that it is never 200.
			if rec.Code == http.StatusOK {
				t.Errorf("name=%q returned 200 — traversal guard not rejected", tc.rawName)
			}
			// The 400 traversal guard fires FIRST (before capability verify).
			if tc.rawName == ".." || strings.Contains(tc.rawName, "/") || strings.Contains(tc.rawName, "\\") {
				if rec.Code != http.StatusBadRequest {
					t.Errorf("name=%q: status = %d, want 400", tc.rawName, rec.Code)
				}
			}
		})
	}
}

// TestModelServe_MinIOError_500 — GetObject failure → 500 (generic, no leak).
func TestModelServe_MinIOError_500(t *testing.T) {
	capability.Init(modelsTestCapSecret)

	repo := &fakeModelGetter{model: &domain.UpscaleModel{
		Name: "realesrgan", Version: "1", Checksum: "c", ObjectPath: "models/realesrgan/1.tar",
	}}
	uploader := &fakeModelObjectGetter{err: errors.New("minio: connection refused")}
	r := buildModelServeRouter(repo, uploader)

	exp, sig := mintModelHandle("realesrgan")
	rec := doModelGet(r, "realesrgan", exp, sig)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("minio error: status = %d, want 500; body=%q", rec.Code, rec.Body.String())
	}
	// Must not leak "minio" or internal path in the response.
	body := rec.Body.String()
	for _, tok := range []string{"minio", "connection", "refused", "bucket", "models/"} {
		if strings.Contains(strings.ToLower(body), tok) {
			t.Errorf("response body leaks %q: %s", tok, body)
		}
	}
}
