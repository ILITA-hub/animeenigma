package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testCfg returns a Config with the given APIKey for transfer tests.
func testCfg(apiKey string) Config {
	return Config{ServerURL: "http://unused", APIKey: apiKey}
}

// TestDownload_CapabilityQueryAndHeaders verifies that Download appends the
// three capability query params (handle, exp, sig) URL-escaped, sends
// X-Worker-Id, and (when set) X-API-Key.
func TestDownload_CapabilityQueryAndHeaders(t *testing.T) {
	t.Parallel()

	const (
		wantHandle   = "hdl+1=="
		wantExp      = "9999999999"
		wantSig      = "sig+abc/def="
		wantWorkerID = "worker-42"
		wantAPIKey   = "key-secret"
		wantBody     = "segment-bytes"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("handle"); got != wantHandle {
			http.Error(w, "bad handle: "+got, http.StatusBadRequest)
			return
		}
		if got := q.Get("exp"); got != wantExp {
			http.Error(w, "bad exp: "+got, http.StatusBadRequest)
			return
		}
		if got := q.Get("sig"); got != wantSig {
			http.Error(w, "bad sig: "+got, http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("X-Worker-Id"); got != wantWorkerID {
			http.Error(w, "missing X-Worker-Id: "+got, http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("X-API-Key"); got != wantAPIKey {
			http.Error(w, "missing X-API-Key: "+got, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(wantBody)) //nolint:errcheck
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.seg")
	cfg := testCfg(wantAPIKey)

	err := Download(context.Background(), cfg, wantWorkerID,
		srv.URL+"/seg", wantHandle, wantExp, wantSig, dest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != wantBody {
		t.Errorf("downloaded body = %q, want %q", got, wantBody)
	}
}

// TestDownload_SigEscaped verifies that + and = in the sig are percent-encoded
// in the URL (not interpreted as space/form value).
func TestDownload_SigEscaped(t *testing.T) {
	t.Parallel()

	rawSig := "a+b/c="
	var gotRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.seg")
	_ = Download(context.Background(), testCfg(""), "wid", srv.URL, "h", "e", rawSig, dest)

	// The sig value in the raw query must be percent-encoded.
	// url.Values.Encode uses QueryEscape which turns + → %2B, / → %2F, = → %3D.
	escapedSig := url.QueryEscape(rawSig)
	if !strings.Contains(gotRawQuery, "sig="+escapedSig) {
		t.Errorf("raw query %q does not contain escaped sig %q", gotRawQuery, "sig="+escapedSig)
	}
}

// TestDownload_401 verifies that a 401 response is returned as an error.
func TestDownload_401(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := Download(context.Background(), testCfg(""), "w", srv.URL, "h", "e", "s",
		filepath.Join(t.TempDir(), "x"))
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q should mention 401", err)
	}
}

// TestDownload_5xx verifies that a 5xx response is returned as an error.
func TestDownload_5xx(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := Download(context.Background(), testCfg(""), "w", srv.URL, "h", "e", "s",
		filepath.Join(t.TempDir(), "x"))
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

// TestUpload_StreamsFile verifies that Upload sends the file content via PUT.
func TestUpload_StreamsFile(t *testing.T) {
	t.Parallel()

	const (
		wantHandle   = "put-hdl"
		wantExp      = "1111"
		wantSig      = "put-sig+="
		wantWorkerID = "worker-99"
		wantAPIKey   = "api-key-up"
		fileContent  = "upscaled-segment-data"
	)

	var gotBody []byte
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		q := r.URL.Query()
		if q.Get("handle") != wantHandle || q.Get("exp") != wantExp || q.Get("sig") != wantSig {
			http.Error(w, "bad capability", http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-Worker-Id") != wantWorkerID {
			http.Error(w, "missing X-Worker-Id", http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-API-Key") != wantAPIKey {
			http.Error(w, "missing X-API-Key", http.StatusBadRequest)
			return
		}
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "in.seg")
	if err := os.WriteFile(src, []byte(fileContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testCfg(wantAPIKey)
	err := Upload(context.Background(), cfg, wantWorkerID,
		srv.URL, wantHandle, wantExp, wantSig, src)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if string(gotBody) != fileContent {
		t.Errorf("body = %q, want %q", gotBody, fileContent)
	}
}

// TestUpload_413 verifies that a 413 response is returned as an error.
func TestUpload_413(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "x")
	os.WriteFile(src, []byte("data"), 0600) //nolint:errcheck

	err := Upload(context.Background(), testCfg(""), "w", srv.URL, "h", "e", "s", src)
	if err == nil {
		t.Fatal("expected error on 413")
	}
	if !strings.Contains(err.Error(), "413") {
		t.Errorf("error %q should mention 413", err)
	}
}

// TestUpload_409 verifies that a 409 response is returned as an error.
func TestUpload_409(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "x")
	os.WriteFile(src, []byte("data"), 0600) //nolint:errcheck

	err := Upload(context.Background(), testCfg(""), "w", srv.URL, "h", "e", "s", src)
	if err == nil {
		t.Fatal("expected error on 409")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error %q should mention 409", err)
	}
}

// TestUpload_NoAPIKey verifies that X-API-Key is omitted when cfg.APIKey is empty.
func TestUpload_NoAPIKey(t *testing.T) {
	t.Parallel()

	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := filepath.Join(t.TempDir(), "x")
	os.WriteFile(src, []byte("d"), 0600) //nolint:errcheck

	_ = Upload(context.Background(), testCfg(""), "w", srv.URL, "h", "e", "s", src)
	if gotAPIKey != "" {
		t.Errorf("X-API-Key should be absent when cfg.APIKey is empty, got %q", gotAPIKey)
	}
}
