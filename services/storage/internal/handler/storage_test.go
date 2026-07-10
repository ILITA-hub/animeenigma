package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/service"
)

// fakeBackends is the Backends test double — no real MinIO/S3 in unit
// tests. It records every storage id it was asked to act on so tests can
// assert placement routed correctly, and counts destructive-op invocations
// so tests can assert that rejected requests never reach the backend.
type fakeBackends struct {
	ingestCalls []string
	moveCalls   int
	copyCalls   int
	deleteCalls int
}

func (f *fakeBackends) IngestURLs(_ context.Context, storage, prefix string, files []string) ([]domain.PutURL, error) {
	f.ingestCalls = append(f.ingestCalls, storage)
	urls := make([]domain.PutURL, len(files))
	for i, name := range files {
		urls[i] = domain.PutURL{Name: name, URL: "https://example.invalid/" + storage + "/" + prefix + name}
	}
	return urls, nil
}

func (f *fakeBackends) DownloadURLs(context.Context, string, string) ([]domain.GetURL, error) {
	return nil, nil
}

func (f *fakeBackends) Move(context.Context, string, string, string) (int, error) {
	f.moveCalls++
	return 0, nil
}

func (f *fakeBackends) Copy(context.Context, string, string, string) (int, int64, error) {
	f.copyCalls++
	return 0, 0, nil
}

func (f *fakeBackends) DeletePrefix(context.Context, string, string) (int, error) {
	f.deleteCalls++
	return 0, nil
}

func (f *fakeBackends) List(context.Context, string, string) ([]domain.Object, error) {
	return nil, nil
}

func (f *fakeBackends) BaseURLs() map[string]string {
	return map[string]string{"minio": "http://minio:9000/raw-library"}
}

func (f *fakeBackends) Health(context.Context) map[string]string {
	return map[string]string{"minio": "up", "s3": "down"}
}

func newTestHandler() (*StorageHandler, *fakeBackends) {
	fb := &fakeBackends{}
	placement := service.NewPlacement(map[string]string{
		domain.ClassLibraryAuto:   domain.BackendS3,
		domain.ClassLibraryManual: domain.BackendMinio,
		domain.ClassUpscaled:      domain.BackendS3,
	}, false, logger.Default())
	return NewStorageHandler(fb, placement, logger.Default()), fb
}

// envelope mirrors the {success,data} shape httputil.OK/Error emits.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func TestIngestURLs_Placement(t *testing.T) {
	cases := []struct {
		name        string
		class       string
		override    string
		wantStatus  int
		wantStorage string
	}{
		{"library-auto default routes to s3", domain.ClassLibraryAuto, "", http.StatusOK, domain.BackendS3},
		{"library-manual default routes to minio", domain.ClassLibraryManual, "", http.StatusOK, domain.BackendMinio},
		{"library-manual override s3 routes to s3", domain.ClassLibraryManual, domain.BackendS3, http.StatusOK, domain.BackendS3},
		{"unknown class is rejected", "bogus-class", "", http.StatusBadRequest, ""},
		{"override on library-auto is rejected", domain.ClassLibraryAuto, domain.BackendMinio, http.StatusBadRequest, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, fb := newTestHandler()
			body, _ := json.Marshal(domain.IngestURLsRequest{
				Class: tc.class, Prefix: "p/", Files: []string{"a.txt"}, Override: tc.override,
			})
			req := httptest.NewRequest(http.MethodPost, "/internal/storage/ingest-urls", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			h.IngestURLs(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if len(fb.ingestCalls) != 0 {
					t.Fatalf("backend should not have been called on a rejected request, got calls=%v", fb.ingestCalls)
				}
				return
			}

			var env envelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			var resp domain.IngestURLsResponse
			if err := json.Unmarshal(env.Data, &resp); err != nil {
				t.Fatalf("decode data: %v", err)
			}
			if resp.Storage != tc.wantStorage {
				t.Fatalf("storage = %q, want %q", resp.Storage, tc.wantStorage)
			}
			if len(resp.URLs) != 1 || resp.URLs[0].Name != "a.txt" {
				t.Fatalf("unexpected urls: %+v", resp.URLs)
			}
			// Assert against the exported constant, not an independent
			// literal — the wire-reported lifetime must always match the
			// duration the backends actually presign with.
			if want := int(service.PresignExpiry.Seconds()); resp.ExpiresIn != want {
				t.Fatalf("expires_in = %d, want %d (service.PresignExpiry)", resp.ExpiresIn, want)
			}
			if len(fb.ingestCalls) != 1 || fb.ingestCalls[0] != tc.wantStorage {
				t.Fatalf("backend called with storage=%v, want [%s]", fb.ingestCalls, tc.wantStorage)
			}
		})
	}
}

// TestEmptyPrefix_Rejected guards against the bucket-wipe hazard: an empty
// prefix matches EVERY object, so DeletePrefix/Move/Copy with an empty
// prefix must 400 without the backend ever being invoked.
func TestEmptyPrefix_Rejected(t *testing.T) {
	t.Run("delete-prefix empty prefix is 400 and backend untouched", func(t *testing.T) {
		h, fb := newTestHandler()
		body, _ := json.Marshal(domain.DeletePrefixRequest{Storage: domain.BackendMinio, Prefix: ""})
		req := httptest.NewRequest(http.MethodDelete, "/internal/storage/prefix", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.DeletePrefix(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if fb.deleteCalls != 0 {
			t.Fatalf("DeletePrefix backend called %d times on a rejected request, want 0", fb.deleteCalls)
		}
	})

	t.Run("move empty from_prefix is 400 and backend untouched", func(t *testing.T) {
		h, fb := newTestHandler()
		body, _ := json.Marshal(domain.MoveRequest{Storage: domain.BackendMinio, FromPrefix: "", ToPrefix: "dst/"})
		req := httptest.NewRequest(http.MethodPost, "/internal/storage/move", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Move(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if fb.moveCalls != 0 {
			t.Fatalf("Move backend called %d times on a rejected request, want 0", fb.moveCalls)
		}
	})

	t.Run("move empty to_prefix is 400 and backend untouched", func(t *testing.T) {
		h, fb := newTestHandler()
		body, _ := json.Marshal(domain.MoveRequest{Storage: domain.BackendMinio, FromPrefix: "src/", ToPrefix: ""})
		req := httptest.NewRequest(http.MethodPost, "/internal/storage/move", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Move(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if fb.moveCalls != 0 {
			t.Fatalf("Move backend called %d times on a rejected request, want 0", fb.moveCalls)
		}
	})

	t.Run("copy empty prefix is 400 and backend untouched", func(t *testing.T) {
		h, fb := newTestHandler()
		body, _ := json.Marshal(domain.CopyPrefixRequest{FromStorage: domain.BackendS3, ToStorage: domain.BackendMinio, Prefix: ""})
		req := httptest.NewRequest(http.MethodPost, "/internal/storage/copy", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Copy(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if fb.copyCalls != 0 {
			t.Fatalf("Copy backend called %d times on a rejected request, want 0", fb.copyCalls)
		}
	})
}

func TestHealth_JSONShape(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/internal/storage/health", nil)
	rec := httptest.NewRecorder()

	h.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var health map[string]string
	if err := json.Unmarshal(env.Data, &health); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if health["minio"] != "up" || health["s3"] != "down" {
		t.Fatalf("unexpected health shape: %+v", health)
	}
}
