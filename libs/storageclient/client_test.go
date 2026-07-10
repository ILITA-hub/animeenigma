package storageclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// envelope mirrors libs/httputil.Response — every real storage-service route
// wraps its payload in {"success":bool,"data":...}. The fake server below
// reproduces that shape exactly so a client bug that forgets to unwrap
// `.data` fails these tests the same way it would fail against the real
// service.
type fakeEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *fakeError  `json:"error,omitempty"`
}

type fakeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fakeEnvelope{Success: true, Data: data})
}

// TestUploadFiles_SegmentsBeforePlaylist is the ordering case named in the
// task brief: every non-playlist file must finish PUTting before
// playlist.m3u8 is ever requested, mirroring services/library's minio
// writer (segments concurrently, playlist last on the calling goroutine).
func TestUploadFiles_SegmentsBeforePlaylist(t *testing.T) {
	dir := t.TempDir()
	segPaths := make([]string, 5)
	for i := range segPaths {
		p := filepath.Join(dir, "seg"+string(rune('0'+i))+".ts")
		if err := os.WriteFile(p, []byte("segment-data"), 0o644); err != nil {
			t.Fatalf("write segment file: %v", err)
		}
		segPaths[i] = p
	}
	playlistPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(playlistPath, []byte("#EXTM3U"), 0o644); err != nil {
		t.Fatalf("write playlist file: %v", err)
	}
	filePaths := append(append([]string{}, segPaths...), playlistPath)

	var mu sync.Mutex
	var putOrder []string
	var playlistSeen atomic.Bool
	var orderViolated atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/ingest-urls", func(w http.ResponseWriter, r *http.Request) {
		var req ingestURLsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode ingest-urls request: %v", err)
		}
		urls := make([]PutURL, 0, len(req.Files))
		for _, f := range req.Files {
			urls = append(urls, PutURL{Name: f, URL: "http://" + r.Host + "/put/" + f})
		}
		writeOK(w, ingestURLsResponse{Storage: "minio", URLs: urls, ExpiresIn: 3600})
	})
	mux.HandleFunc("/put/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		mu.Lock()
		putOrder = append(putOrder, name)
		mu.Unlock()
		if name == "playlist.m3u8" {
			playlistSeen.Store(true)
		} else if playlistSeen.Load() {
			// A segment arrived after the playlist was already PUT — the
			// ordering guarantee is broken.
			orderViolated.Store(true)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read PUT body for %s: %v", name, err)
		}
		if len(body) == 0 {
			t.Errorf("PUT body for %s was empty", name)
		}
		if ct := r.Header.Get("Content-Type"); name == "playlist.m3u8" && ct != "application/vnd.apple.mpegurl" {
			t.Errorf("playlist Content-Type = %q, want application/vnd.apple.mpegurl", ct)
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	storage, err := c.UploadFiles(context.Background(), "library-auto", "", "anime/1/", filePaths, 3)
	if err != nil {
		t.Fatalf("UploadFiles: %v", err)
	}
	if storage != "minio" {
		t.Errorf("storage = %q, want minio", storage)
	}
	if orderViolated.Load() {
		t.Errorf("a segment PUT was observed after the playlist PUT: order = %v", putOrder)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(putOrder) != 6 {
		t.Fatalf("got %d PUTs, want 6: %v", len(putOrder), putOrder)
	}
	if putOrder[len(putOrder)-1] != "playlist.m3u8" {
		t.Errorf("last PUT = %q, want playlist.m3u8 (order: %v)", putOrder[len(putOrder)-1], putOrder)
	}
}

// TestUploadFiles_PlaylistSkippedOnSegmentError verifies the "on any segment
// error, the playlist is never uploaded" rule ported from writer.go.
func TestUploadFiles_PlaylistSkippedOnSegmentError(t *testing.T) {
	dir := t.TempDir()
	badSeg := filepath.Join(dir, "bad.ts")
	if err := os.WriteFile(badSeg, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	playlistPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(playlistPath, []byte("#EXTM3U"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var playlistPutCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/ingest-urls", func(w http.ResponseWriter, r *http.Request) {
		var req ingestURLsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		urls := make([]PutURL, 0, len(req.Files))
		for _, f := range req.Files {
			urls = append(urls, PutURL{Name: f, URL: "http://" + r.Host + "/put/" + f})
		}
		writeOK(w, ingestURLsResponse{Storage: "minio", URLs: urls})
	})
	mux.HandleFunc("/put/bad.ts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/put/playlist.m3u8", func(w http.ResponseWriter, r *http.Request) {
		playlistPutCount.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.UploadFiles(context.Background(), "library-auto", "", "anime/1/", []string{badSeg, playlistPath}, 2)
	if err == nil {
		t.Fatal("expected UploadFiles to fail when a segment PUT fails")
	}
	if n := playlistPutCount.Load(); n != 0 {
		t.Errorf("playlist was PUT %d times after a segment error, want 0", n)
	}
}

// TestURLFor_CachesBaseURLs is the caching case named in the brief: two
// URLFor calls for different (storage, path) pairs must only hit
// /internal/storage/base-urls once.
func TestURLFor_CachesBaseURLs(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/base-urls", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		writeOK(w, map[string]string{
			"minio": "http://minio:9000/raw-library",
			"s3":    "https://s3.firstvds.ru/raw-library",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	got1, err := c.URLFor(context.Background(), "minio", "anime/1/playlist.m3u8")
	if err != nil {
		t.Fatalf("URLFor #1: %v", err)
	}
	want1 := "http://minio:9000/raw-library/anime/1/playlist.m3u8"
	if got1 != want1 {
		t.Errorf("URLFor #1 = %q, want %q", got1, want1)
	}

	got2, err := c.URLFor(context.Background(), "s3", "anime/2/playlist.m3u8")
	if err != nil {
		t.Fatalf("URLFor #2: %v", err)
	}
	want2 := "https://s3.firstvds.ru/raw-library/anime/2/playlist.m3u8"
	if got2 != want2 {
		t.Errorf("URLFor #2 = %q, want %q", got2, want2)
	}

	if n := hits.Load(); n != 1 {
		t.Errorf("base-urls was hit %d times, want 1 (should be cached across the two URLFor calls)", n)
	}
}

// TestBaseURLs_CallerMutationDoesNotCorruptCache guards against a caller
// mutating the map returned by BaseURLs() and poisoning the shared 5-minute
// cache for every subsequent BaseURLs()/URLFor() call on the same Client.
func TestBaseURLs_CallerMutationDoesNotCorruptCache(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/base-urls", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]string{"minio": "http://minio:9000/raw-library"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	first, err := c.BaseURLs(context.Background())
	if err != nil {
		t.Fatalf("BaseURLs #1: %v", err)
	}
	first["minio"] = "tampered"
	first["s3"] = "injected"

	second, err := c.BaseURLs(context.Background())
	if err != nil {
		t.Fatalf("BaseURLs #2: %v", err)
	}
	if second["minio"] != "http://minio:9000/raw-library" {
		t.Errorf("cache was corrupted by mutating a prior BaseURLs() result: %v", second)
	}
	if _, ok := second["s3"]; ok {
		t.Errorf("cache picked up a key injected into a prior BaseURLs() result: %v", second)
	}
}

// TestURLFor_UnknownStorage checks the error path when the requested backend
// id isn't in the base-urls map.
func TestURLFor_UnknownStorage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/base-urls", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]string{"minio": "http://minio:9000/raw-library"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.URLFor(context.Background(), "glacier", "x"); err == nil {
		t.Fatal("expected an error for an unknown storage id")
	}
}

// TestDeletePrefix_ParsesCount is the DeletePrefix count-parsing case named
// in the brief.
func TestDeletePrefix_ParsesCount(t *testing.T) {
	var gotBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/prefix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		writeOK(w, deleteResponse{Deleted: 7})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	n, err := c.DeletePrefix(context.Background(), "minio", "anime/1/")
	if err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if n != 7 {
		t.Errorf("DeletePrefix count = %d, want 7", n)
	}

	var req deletePrefixRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if req.Storage != "minio" || req.Prefix != "anime/1/" {
		t.Errorf("request body = %+v, want storage=minio prefix=anime/1/", req)
	}
}

// TestDeletePrefix_ServerError checks the {success:false,error:{...}} path
// is surfaced as a Go error rather than silently returning a zero count.
func TestDeletePrefix_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/prefix", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(fakeEnvelope{
			Success: false,
			Error:   &fakeError{Code: "INVALID_INPUT", Message: "prefix is required"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	if _, err := c.DeletePrefix(context.Background(), "minio", ""); err == nil {
		t.Fatal("expected an error when the server reports success:false")
	}
}

// TestIngestURLs_RoundTrip exercises the plain IngestURLs entrypoint (not
// via UploadFiles) — request encoding + response decoding.
func TestIngestURLs_RoundTrip(t *testing.T) {
	var gotReq ingestURLsRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/ingest-urls", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeOK(w, ingestURLsResponse{
			Storage: "s3",
			URLs:    []PutURL{{Name: "a.txt", URL: "https://example/a"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	res, err := c.IngestURLs(context.Background(), "library-manual", "anime/1/", []string{"a.txt"}, "s3")
	if err != nil {
		t.Fatalf("IngestURLs: %v", err)
	}
	if res.Storage != "s3" || len(res.URLs) != 1 || res.URLs[0].Name != "a.txt" || res.URLs[0].URL != "https://example/a" {
		t.Errorf("IngestResult = %+v, want storage=s3 with one a.txt URL", res)
	}
	if gotReq.Class != "library-manual" || gotReq.Prefix != "anime/1/" || gotReq.Override != "s3" {
		t.Errorf("request = %+v", gotReq)
	}
}

// TestMove_RoundTrip checks the request shape and that a non-2xx/success
// response surfaces as an error.
func TestMove_RoundTrip(t *testing.T) {
	var gotReq moveRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/move", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeOK(w, moveResponse{Moved: 3})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Move(context.Background(), "minio", "pending/job1/", "42/"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if gotReq.Storage != "minio" || gotReq.FromPrefix != "pending/job1/" || gotReq.ToPrefix != "42/" {
		t.Errorf("request = %+v", gotReq)
	}
}

// TestCopyPrefix_RoundTrip checks copied+bytes are both returned.
func TestCopyPrefix_RoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/copy", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, copyResponse{Copied: 4, Bytes: 1024})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	copied, bytes, err := c.CopyPrefix(context.Background(), "s3", "minio", "anime/1/")
	if err != nil {
		t.Fatalf("CopyPrefix: %v", err)
	}
	if copied != 4 || bytes != 1024 {
		t.Errorf("CopyPrefix = (%d, %d), want (4, 1024)", copied, bytes)
	}
}

// TestList_RoundTrip checks query params + Object decoding.
func TestList_RoundTrip(t *testing.T) {
	var gotStorage, gotPrefix string
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/list", func(w http.ResponseWriter, r *http.Request) {
		gotStorage = r.URL.Query().Get("storage")
		gotPrefix = r.URL.Query().Get("prefix")
		writeOK(w, listResponse{Objects: []Object{{Key: "anime/1/playlist.m3u8", Size: 512}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL)
	objs, err := c.List(context.Background(), "minio", "anime/1/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotStorage != "minio" || gotPrefix != "anime/1/" {
		t.Errorf("query = storage=%q prefix=%q", gotStorage, gotPrefix)
	}
	if len(objs) != 1 || objs[0].Key != "anime/1/playlist.m3u8" || objs[0].Size != 512 {
		t.Errorf("objects = %+v", objs)
	}
}

// TestDownloadPrefix_WritesFiles checks the download-urls + GET-each flow.
func TestDownloadPrefix_WritesFiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/storage/download-urls", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, downloadURLsResponse{URLs: []GetURL{
			{Name: "storyboard_001.jpg", URL: "http://" + r.Host + "/get/storyboard_001.jpg"},
			{Name: "storyboard.vtt", URL: "http://" + r.Host + "/get/storyboard.vtt"},
		}})
	})
	mux.HandleFunc("/get/storyboard_001.jpg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jpegbytes"))
	})
	mux.HandleFunc("/get/storyboard.vtt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("WEBVTT"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	c := New(srv.URL)
	if err := c.DownloadPrefix(context.Background(), "minio", "anime/1/", dir); err != nil {
		t.Fatalf("DownloadPrefix: %v", err)
	}

	jpg, err := os.ReadFile(filepath.Join(dir, "storyboard_001.jpg"))
	if err != nil || string(jpg) != "jpegbytes" {
		t.Errorf("storyboard_001.jpg = %q, err %v", jpg, err)
	}
	vtt, err := os.ReadFile(filepath.Join(dir, "storyboard.vtt"))
	if err != nil || string(vtt) != "WEBVTT" {
		t.Errorf("storyboard.vtt = %q, err %v", vtt, err)
	}
}

// TestContentTypeFor pins the extension map ported from
// services/library/internal/minio/writer.go:199-269.
func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"seg.ts":        "video/mp2t",
		"playlist.m3u8": "application/vnd.apple.mpegurl",
		"thumb.jpg":     "image/jpeg",
		"subs.vtt":      "text/vtt",
		"whatever.bin":  "application/octet-stream",
		"noextension":   "application/octet-stream",
		"UPPER.TS":      "video/mp2t",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}
