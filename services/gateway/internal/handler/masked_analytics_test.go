package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

var testSecret = []byte("test-masked-analytics-secret")

func TestMaskedAnalyticsSegment_DeterministicAndRotating(t *testing.T) {
	now := time.Now()
	bucket := now.Unix() / maskedBucketSeconds
	a := maskedAnalyticsSegment(testSecret, bucket)
	b := maskedAnalyticsSegment(testSecret, bucket)
	if a != b {
		t.Fatal("segment must be deterministic within a bucket")
	}
	if len(a) != 24 {
		t.Fatalf("segment length = %d; want 24", len(a))
	}
	if a == maskedAnalyticsSegment(testSecret, bucket+1) {
		t.Fatal("segment must rotate across buckets")
	}
}

func TestValidMaskedSegment_CurrentAndPrevious(t *testing.T) {
	now := time.Now()
	bucket := now.Unix() / maskedBucketSeconds
	if !validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket), now) {
		t.Fatal("current bucket must validate")
	}
	if !validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket-1), now) {
		t.Fatal("previous bucket must validate (clock skew / session straddle)")
	}
	if validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket-2), now) {
		t.Fatal("stale bucket must be rejected")
	}
	if validMaskedSegment(testSecret, "deadbeefdeadbeefdeadbeef", now) {
		t.Fatal("forged segment must be rejected")
	}
}

func TestCurrentMaskedAnalyticsBase_Shape(t *testing.T) {
	base := CurrentMaskedAnalyticsBase(testSecret, time.Now())
	if len(base) != len("/api/")+24 || base[:5] != "/api/" {
		t.Fatalf("base = %q", base)
	}
}

// Invalid segment / unknown leaf never reach the proxy (h.proxy would nil-panic).
func TestMaskedAnalyticsHandler_RejectsWithoutProxying(t *testing.T) {
	h := NewMaskedAnalyticsHandler(nil, testSecret)
	r := chi.NewRouter()
	r.Post("/api/{maskedSeg}/{maskedEp}", h.Handle)

	for _, path := range []string{
		"/api/deadbeefdeadbeefdeadbeef/c", // forged segment
		"/api/" + maskedAnalyticsSegment(testSecret, time.Now().Unix()/maskedBucketSeconds) + "/zzz", // bad leaf
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d; want 404", path, rec.Code)
		}
	}
}

func TestMaskedPathHintMiddleware_SetsHeader(t *testing.T) {
	mw := MaskedPathHintMiddleware(testSecret)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	mw(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/status", nil))
	got := rec.Header().Get("X-AE-Cfg")
	if got != CurrentMaskedAnalyticsBase(testSecret, time.Now()) {
		t.Fatalf("X-AE-Cfg = %q", got)
	}
}
