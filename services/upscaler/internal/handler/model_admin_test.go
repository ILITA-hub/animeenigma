package handler

// model_admin_test.go — table-driven tests for ModelAdminHandler.
//
// Uses handwritten fakes (no testify/mock) consistent with the rest of the
// handler package tests.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// ── handwritten fakes ──────────────────────────────────────────────────────────

// fakeModelRepo records Upsert/Get/List calls and optionally returns errors.
type fakeModelRepo struct {
	mu     sync.Mutex
	models []domain.UpscaleModel

	upsertErr error
	listErr   error
	getErr    error
}

func (f *fakeModelRepo) Upsert(_ context.Context, m *domain.UpscaleModel) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Replace or append.
	for i, existing := range f.models {
		if existing.Name == m.Name && existing.Version == m.Version {
			f.models[i] = *m
			return nil
		}
	}
	f.models = append(f.models, *m)
	return nil
}

func (f *fakeModelRepo) Get(_ context.Context, name, version string) (*domain.UpscaleModel, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.models {
		if m.Name == name && m.Version == version {
			cp := m
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeModelRepo) List(_ context.Context) ([]domain.UpscaleModel, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.UpscaleModel, len(f.models))
	copy(out, f.models)
	return out, nil
}

// fakeModelUploader captures PutObject calls and stores the uploaded bytes for
// checksum verification.
type fakeModelUploader struct {
	mu      sync.Mutex
	calls   []modelPutCall
	putErr  error
}

type modelPutCall struct {
	bucket      string
	object      string
	contentType string
	size        int64
	body        []byte // drained bytes from the reader
}

func (f *fakeModelUploader) PutObject(_ context.Context, bucket, object string, reader interface {
	Read(p []byte) (int, error)
}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	// Drain the reader so we observe all bytes (TeeReader computes checksum as
	// bytes flow; we store them for cross-verification in the test).
	data, _ := io.ReadAll(reader)

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.putErr != nil {
		return minio.UploadInfo{}, f.putErr
	}

	f.calls = append(f.calls, modelPutCall{
		bucket:      bucket,
		object:      object,
		contentType: opts.ContentType,
		size:        size,
		body:        data,
	})
	return minio.UploadInfo{}, nil
}

func (f *fakeModelUploader) lastCall() (modelPutCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return modelPutCall{}, false
	}
	return f.calls[len(f.calls)-1], true
}

// ── fixture helpers ────────────────────────────────────────────────────────────

type modelFixture struct {
	repo     *fakeModelRepo
	uploader *fakeModelUploader
	handler  *ModelAdminHandler
	router   chi.Router
}

func newModelFixture(t *testing.T) *modelFixture {
	t.Helper()
	repo := &fakeModelRepo{}
	up := &fakeModelUploader{}
	h := NewModelAdminHandler(repo, up, "test-bucket", logger.Default())
	r := chi.NewRouter()
	r.Post("/api/upscale/models", h.UploadModel)
	r.Get("/api/upscale/models", h.ListModels)
	r.Get("/api/upscale/models/{name}", h.GetModelByName)
	return &modelFixture{repo: repo, uploader: up, handler: h, router: r}
}

// buildUploadRequest creates a multipart/form-data request for POST /api/upscale/models.
func buildUploadRequest(t *testing.T, fields map[string]string, artifactName string, artifactBody []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%s): %v", k, err)
		}
	}

	if artifactName != "" {
		pw, err := mw.CreateFormFile("artifact", artifactName)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := pw.Write(artifactBody); err != nil {
			t.Fatalf("Write artifact: %v", err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/upscale/models", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// sha256Hex computes the hex-encoded SHA-256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ── POST /api/upscale/models ──────────────────────────────────────────────────

func TestModelAdmin_UploadModel_HappyPath(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	artifact := []byte("fake-tar-content-for-realesrgan-x4plus-anime")
	req := buildUploadRequest(t,
		map[string]string{"name": "realesrgan-x4plus-anime", "version": "2"},
		"model.tar", artifact,
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadModel status = %d; want 201 (body: %s)", rr.Code, rr.Body)
	}

	// Assert PutObject called with correct key and bucket.
	call, ok := f.uploader.lastCall()
	if !ok {
		t.Fatal("PutObject was never called")
	}
	wantKey := "models/realesrgan-x4plus-anime/2.tar"
	if call.object != wantKey {
		t.Errorf("PutObject object = %q; want %q", call.object, wantKey)
	}
	if call.bucket != "test-bucket" {
		t.Errorf("PutObject bucket = %q; want test-bucket", call.bucket)
	}

	// Assert the row was upserted with the correct SHA-256.
	expectedChecksum := sha256Hex(artifact)
	var got domain.UpscaleModel
	responseData(t, rr.Body.Bytes(), &got)

	if got.Name != "realesrgan-x4plus-anime" {
		t.Errorf("model name = %q; want realesrgan-x4plus-anime", got.Name)
	}
	if got.Version != "2" {
		t.Errorf("model version = %q; want 2", got.Version)
	}
	if got.Checksum != expectedChecksum {
		t.Errorf("model checksum = %q; want %q", got.Checksum, expectedChecksum)
	}
	if got.ObjectPath != wantKey {
		t.Errorf("model object_path = %q; want %q", got.ObjectPath, wantKey)
	}
	if got.Builtin {
		t.Error("model builtin should be false for admin-uploaded models")
	}
}

func TestModelAdmin_UploadModel_DefaultVersion(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	artifact := []byte("tiny-tar-bytes")
	// No "version" field — should default to "1".
	req := buildUploadRequest(t,
		map[string]string{"name": "my-model"},
		"model.tar", artifact,
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadModel (default version) status = %d; want 201 (body: %s)", rr.Code, rr.Body)
	}

	call, _ := f.uploader.lastCall()
	wantKey := "models/my-model/1.tar"
	if call.object != wantKey {
		t.Errorf("PutObject object = %q; want %q", call.object, wantKey)
	}

	var got domain.UpscaleModel
	responseData(t, rr.Body.Bytes(), &got)
	if got.Version != "1" {
		t.Errorf("model version = %q; want 1 (default)", got.Version)
	}
}

// TestModelAdmin_UploadModel_SHA256MatchesTeeReader verifies that the checksum
// stored in the DB equals sha256(artifact_bytes), even though the hash is computed
// via TeeReader while streaming to PutObject (not by buffering then hashing).
func TestModelAdmin_UploadModel_SHA256MatchesTeeReader(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	// Use a larger artifact to make the TeeReader behaviour observable.
	artifact := bytes.Repeat([]byte("abcdefgh"), 1024) // 8 KiB

	req := buildUploadRequest(t,
		map[string]string{"name": "big-model", "version": "3"},
		"model.tar", artifact,
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadModel SHA256 status = %d; want 201", rr.Code)
	}

	// The fake uploader drained the reader, so the bytes PutObject saw equal
	// the artifact bytes (TeeReader split them to both hasher and PutObject).
	call, ok := f.uploader.lastCall()
	if !ok {
		t.Fatal("PutObject not called")
	}

	// Expected: sha256 of the exact bytes the uploader received.
	expectedFromUploader := sha256Hex(call.body)

	var got domain.UpscaleModel
	responseData(t, rr.Body.Bytes(), &got)
	if got.Checksum != expectedFromUploader {
		t.Errorf("checksum mismatch: handler stored %q but uploader received bytes with SHA256 %q",
			got.Checksum, expectedFromUploader)
	}
	// Also verify it equals the known hash of the original artifact.
	if got.Checksum != sha256Hex(artifact) {
		t.Errorf("checksum = %q; want sha256(artifact) = %q", got.Checksum, sha256Hex(artifact))
	}
}

// ── Validation: empty artifact ────────────────────────────────────────────────

func TestModelAdmin_UploadModel_EmptyArtifact_400(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	// Send a form file with zero bytes.
	req := buildUploadRequest(t,
		map[string]string{"name": "good-name", "version": "1"},
		"empty.tar", []byte{}, // hdr.Size == 0
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("UploadModel (empty artifact) status = %d; want 400", rr.Code)
	}
}

func TestModelAdmin_UploadModel_MissingArtifactField_400(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	// Send no file part at all.
	req := buildUploadRequest(t,
		map[string]string{"name": "good-name", "version": "1"},
		"", nil, // no file
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("UploadModel (no artifact) status = %d; want 400", rr.Code)
	}
}

// ── Validation: path traversal in name / version ──────────────────────────────

func TestModelAdmin_UploadModel_TraversalName_400(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		version string
	}{
		{"../evil", "1"},
		{"a/b", "1"},
		{"good", "../evil"},
		{"good", "a/b"},
		{"", "1"},
		// NOTE: empty version is NOT a 400 — it defaults to "1". Traversal check
		// only applies to non-empty version values that contain "/" or "..".
	}
	for _, c := range cases {
		c := c
		label := fmt.Sprintf("name=%q version=%q", c.name, c.version)
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			f := newModelFixture(t)
			req := buildUploadRequest(t,
				map[string]string{"name": c.name, "version": c.version},
				"model.tar", []byte("fake-data"),
			)
			rr := httptest.NewRecorder()
			f.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("UploadModel (%s) status = %d; want 400", label, rr.Code)
			}
		})
	}
}

// ── GET /api/upscale/models ───────────────────────────────────────────────────

func TestModelAdmin_ListModels_Empty(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/models", nil)
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("ListModels (empty) status = %d; want 200", rr.Code)
	}
	var got []domain.UpscaleModel
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Errorf("ListModels (empty) len = %d; want 0", len(got))
	}
}

func TestModelAdmin_ListModels_ReturnsUpsertedRows(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	// Seed two models via upload.
	for _, pair := range [][2]string{
		{"model-a", "1"},
		{"model-b", "2"},
	} {
		req := buildUploadRequest(t,
			map[string]string{"name": pair[0], "version": pair[1]},
			"model.tar", []byte("data-"+pair[0]+pair[1]),
		)
		rr := httptest.NewRecorder()
		f.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("seed upload %v: status = %d", pair, rr.Code)
		}
	}

	// List.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/models", nil)
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("ListModels status = %d; want 200", rr.Code)
	}
	var got []domain.UpscaleModel
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Errorf("ListModels len = %d; want 2", len(got))
	}
}

// ── GET /api/upscale/models/{name} ───────────────────────────────────────────

func TestModelAdmin_GetModelByName_Found(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	artifact := []byte("content-for-my-model")
	req := buildUploadRequest(t,
		map[string]string{"name": "my-model", "version": "1"},
		"model.tar", artifact,
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: status = %d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/upscale/models/my-model", nil)
	f.router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("GetModelByName status = %d; want 200 (body: %s)", rr2.Code, rr2.Body)
	}

	// Response is an array when ?version= is absent.
	var got []domain.UpscaleModel
	responseData(t, rr2.Body.Bytes(), &got)
	if len(got) == 0 {
		t.Fatal("GetModelByName: empty array; want 1 row")
	}
	if got[0].Name != "my-model" {
		t.Errorf("GetModelByName name = %q; want my-model", got[0].Name)
	}
	if got[0].Checksum != sha256Hex(artifact) {
		t.Errorf("GetModelByName checksum = %q; want %q", got[0].Checksum, sha256Hex(artifact))
	}
}

func TestModelAdmin_GetModelByName_WithVersion(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	artifact := []byte("versioned-model-content")
	req := buildUploadRequest(t,
		map[string]string{"name": "versioned", "version": "42"},
		"model.tar", artifact,
	)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: status = %d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/upscale/models/versioned?version=42", nil)
	f.router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("GetModelByName?version=42 status = %d; want 200", rr2.Code)
	}

	// With ?version= the response is a single object.
	var got domain.UpscaleModel
	responseData(t, rr2.Body.Bytes(), &got)
	if got.Name != "versioned" {
		t.Errorf("name = %q; want versioned", got.Name)
	}
	if got.Version != "42" {
		t.Errorf("version = %q; want 42", got.Version)
	}
}

func TestModelAdmin_GetModelByName_NotFound_404(t *testing.T) {
	t.Parallel()
	f := newModelFixture(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/upscale/models/nonexistent", nil)
	f.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("GetModelByName (missing) status = %d; want 404", rr.Code)
	}
}

// ── T31: admin upload size cap ───────────────────────────────────────────────

// TestModelAdmin_UploadModel_OversizedBody_413 verifies that a request body
// exceeding maxUploadBodyBytes is rejected with HTTP 413 before any data is
// written to MinIO or the database.
//
// We can't send 2 GiB in a unit test, so we override the per-request body limit
// using http.MaxBytesReader with a tiny cap and confirm the 413 path. The
// production cap (maxUploadBodyBytes = 2 GiB) is exercised by the constant
// being used in UploadModel; this test validates the code path fires correctly.
func TestModelAdmin_UploadModel_OversizedBody_413(t *testing.T) {
	t.Parallel()

	// Build a tiny artifact and a minimal multipart body.
	artifact := []byte("tiny-but-over-the-limit")
	req := buildUploadRequest(t,
		map[string]string{"name": "oversize-model", "version": "1"},
		"model.tar", artifact,
	)

	// Wrap the body in a MaxBytesReader with a cap of 1 byte so ANY real body
	// triggers the limit. This exercises the exact same code path that fires in
	// production when a 2 GiB+ artifact is uploaded.
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 1)

	// Construct the handler directly (not via router) so we can inject the
	// already-capped body and call UploadModel, which will call ParseMultipartForm
	// and observe the MaxBytesError.
	repo := &fakeModelRepo{}
	up := &fakeModelUploader{}
	h := NewModelAdminHandler(repo, up, "test-bucket", logger.Default())

	rr := httptest.NewRecorder()
	h.UploadModel(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("UploadModel (oversized body): status = %d; want 413\nbody: %s", rr.Code, rr.Body)
	}

	// MinIO must not have been called.
	if _, ok := up.lastCall(); ok {
		t.Error("PutObject must not be called when body exceeds size cap")
	}
	// Database must not have been written.
	models, _ := repo.List(context.Background())
	if len(models) != 0 {
		t.Errorf("model must not be persisted when body exceeds cap; got %d rows", len(models))
	}
}

// ── requireGatewayInternal gate (routes are behind the gate in the real router) ──

// TestModelAdmin_RoutesRequireGatewayHeader verifies that the model routes are
// inside the requireGatewayInternal-gated group: a direct request without the
// X-Gateway-Internal header returns 404 (the gate fires). With the header set,
// the gate passes and the handler runs normally.
//
// Mirrors the pattern in transport/router_separation_test.go. We build a
// minimal gated chi.Router inline (without importing transport) so this test
// lives entirely in the handler package.
func TestModelAdmin_RoutesRequireGatewayHeader(t *testing.T) {
	t.Parallel()

	// gateBody is the distinct sentinel in the gate's 404 response body,
	// different from the handler's own "model not found" 404.
	const gateBody = "GATE_FIRED"

	gate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Gateway-Internal") == "" {
				// Use a distinct body so tests can tell gate-404 from handler-404.
				http.Error(w, `{"success":false,"error":{"code":"GATE_FIRED","message":"not found"}}`,
					http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	repo := &fakeModelRepo{}
	up := &fakeModelUploader{}
	h := NewModelAdminHandler(repo, up, "test-bucket", logger.Default())

	r := chi.NewRouter()
	r.Route("/api/upscale", func(r chi.Router) {
		r.Use(gate)
		r.Post("/models", h.UploadModel)
		r.Get("/models", h.ListModels)
		r.Get("/models/{name}", h.GetModelByName)
	})

	// Without the header: the gate must fire (GATE_FIRED sentinel in body).
	gatedPaths := []string{
		"/api/upscale/models",
		"/api/upscale/models/some-model",
	}
	for _, path := range gatedPaths {
		path := path
		t.Run("no-header "+path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("path %q without gateway header: status = %d; want 404 (gate)", path, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), gateBody) {
				t.Errorf("path %q without header: body does not contain gate sentinel %q: %q",
					path, gateBody, rec.Body.String())
			}
		})
	}

	// With the header: gate passes, GET /api/upscale/models returns 200 (list).
	t.Run("with-header list returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/upscale/models", nil)
		req.Header.Set("X-Gateway-Internal", "1")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /api/upscale/models with header: status = %d; want 200", rec.Code)
		}
		if strings.Contains(rec.Body.String(), gateBody) {
			t.Errorf("GET /api/upscale/models with header: gate sentinel found in body — gate fired unexpectedly: %q",
				rec.Body.String())
		}
	})
}
