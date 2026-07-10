package storagegw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/storageclient"
)

// fake storage-service wire types — the client's private contract, re-declared
// here just enough to fake POST /internal/storage/ingest-urls.
type fakeIngestRequest struct {
	Class    string   `json:"class"`
	Prefix   string   `json:"prefix"`
	Files    []string `json:"files"`
	Override string   `json:"override"`
}

type fakePutURL struct {
	Name string `json:"name"`
	URL  string `json:"put_url"`
}

type fakeIngestResponse struct {
	Storage   string       `json:"storage"`
	URLs      []fakePutURL `json:"urls"`
	ExpiresIn int          `json:"expires_in"`
}

type fakeEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

// TestGatewayUpload_RoutesClassUpscaled is the adapter routing test: an output
// upload must hit ingest-urls with class "upscaled" (empty override), PUT every
// file, and return the backend id the storage service resolved — the value the
// orchestrator records on the job row.
func TestGatewayUpload_RoutesClassUpscaled(t *testing.T) {
	dir := t.TempDir()
	files := make([]string, 0, 3)
	for _, name := range []string{"seg_000.ts", "seg_001.ts", "playlist.m3u8"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("data-"+name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		files = append(files, p)
	}

	var mu sync.Mutex
	var gotIngest fakeIngestRequest
	var putNames []string

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/ingest-urls", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotIngest); err != nil {
			t.Errorf("decode ingest-urls request: %v", err)
		}
		urls := make([]fakePutURL, 0, len(gotIngest.Files))
		for _, f := range gotIngest.Files {
			urls = append(urls, fakePutURL{Name: f, URL: "http://" + r.Host + "/put/" + f})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeEnvelope{Success: true, Data: fakeIngestResponse{
			Storage: "s3", URLs: urls, ExpiresIn: 3600,
		}})
	})
	mux.HandleFunc("/put/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		putNames = append(putNames, filepath.Base(r.URL.Path))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	g := New(storageclient.New(srv.URL), 2)
	prefix := "aeProvider/777/UPSCALED-1080p/12/"
	storage, err := g.Upload(context.Background(), prefix, files)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// The resolved backend id must surface to the caller unchanged.
	if storage != "s3" {
		t.Errorf("resolved storage = %q, want s3", storage)
	}

	// Routing: class upscaled, no override, the orchestrator's exact prefix.
	if gotIngest.Class != ClassUpscaled {
		t.Errorf("ingest class = %q, want %q", gotIngest.Class, ClassUpscaled)
	}
	if gotIngest.Override != "" {
		t.Errorf("ingest override = %q, want empty (upscaled has fixed placement)", gotIngest.Override)
	}
	if gotIngest.Prefix != prefix {
		t.Errorf("ingest prefix = %q, want %q", gotIngest.Prefix, prefix)
	}

	// Every file PUT, playlist last (client ordering the adapter must not break).
	mu.Lock()
	defer mu.Unlock()
	if len(putNames) != 3 {
		t.Fatalf("got %d PUTs, want 3: %v", len(putNames), putNames)
	}
	if putNames[len(putNames)-1] != "playlist.m3u8" {
		t.Errorf("last PUT = %q, want playlist.m3u8 (order: %v)", putNames[len(putNames)-1], putNames)
	}
}
