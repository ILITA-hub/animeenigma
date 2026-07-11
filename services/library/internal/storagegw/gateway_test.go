package storagegw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/storageclient"
)

// envelope mirrors libs/httputil.Response — every real storage-service
// route wraps its payload in {"success":bool,"data":...}. Reproduced here
// (rather than imported) since it's an unexported test fixture private to
// libs/storageclient's own test file.
type envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envelope{Success: true, Data: data})
}

// newTestGateway spins up an httptest.Server implementing the storage
// service's /internal/storage/download-urls route (via handler) and wraps a
// real *storageclient.Client pointed at it in a *Gateway. Gateway.client is
// a concrete *storageclient.Client (see gateway.go) with no interface seam
// of its own, so — mirroring libs/storageclient's own test convention
// (client_test.go: real Client against a fake HTTP backend) — tests drive
// the real client against a fake server rather than introducing a
// gateway-only mock interface.
func newTestGateway(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(storageclient.New(srv.URL), 0)
}

func TestGateway_DownloadURL_SingleKey(t *testing.T) {
	var gotStorage, gotPrefix string
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Storage string `json:"storage"`
			Prefix  string `json:"prefix"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotStorage, gotPrefix = req.Storage, req.Prefix
		writeOK(w, struct {
			URLs []storageclient.GetURL `json:"urls"`
		}{URLs: []storageclient.GetURL{{Name: "e01_1080p.ts", URL: "https://signed/e01_1080p.ts"}}})
	})

	url, err := gw.DownloadURL(context.Background(), "minio", "frieren/s1/e01/e01_1080p.ts")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://signed/e01_1080p.ts" {
		t.Fatalf("got %q", url)
	}
	if gotStorage != "minio" || gotPrefix != "frieren/s1/e01/e01_1080p.ts" {
		t.Errorf("request = storage=%q prefix=%q", gotStorage, gotPrefix)
	}
}

func TestGateway_DownloadURL_NoMatch(t *testing.T) {
	gw := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, struct {
			URLs []storageclient.GetURL `json:"urls"`
		}{URLs: []storageclient.GetURL{
			{Name: "e01_1080p.ts", URL: "https://signed/e01_1080p.ts"},
			{Name: "e02_1080p.ts", URL: "https://signed/e02_1080p.ts"},
		}})
	})

	_, err := gw.DownloadURL(context.Background(), "minio", "frieren/s1/e01/e03_1080p.ts")
	if err == nil {
		t.Fatal("expected error for ambiguous/no-match result, got nil")
	}
}
