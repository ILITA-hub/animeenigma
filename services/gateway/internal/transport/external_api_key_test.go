package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExternalAPIKeyMiddleware_NoKey — missing X-API-Key header → 401 with
// generic body; no internal detail (stack/host/path) in the response.
func TestExternalAPIKeyMiddleware_NoKey(t *testing.T) {
	mw := ExternalAPIKeyMiddleware("secret-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no key: status = %d; want 401", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Error("response body must not be empty")
	}
	// Must not leak any internal detail.
	for _, leak := range []string{"stack", "internal", "host", "upscaler", "panic"} {
		if containsCI(body, leak) {
			t.Errorf("response body leaks internal detail %q: %s", leak, body)
		}
	}
}

// TestExternalAPIKeyMiddleware_CorrectKey — correct X-API-Key → passes to next.
func TestExternalAPIKeyMiddleware_CorrectKey(t *testing.T) {
	const key = "correct-key"
	mw := ExternalAPIKeyMiddleware(key)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	req.Header.Set("X-API-Key", key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("correct key: status = %d; want 200", rec.Code)
	}
	if !called {
		t.Error("next handler was not called with correct key")
	}
}

// TestExternalAPIKeyMiddleware_WrongKey — wrong key → 401 with generic body.
func TestExternalAPIKeyMiddleware_WrongKey(t *testing.T) {
	mw := ExternalAPIKeyMiddleware("correct-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d; want 401", rec.Code)
	}
}

// TestExternalAPIKeyMiddleware_EmptyConfiguredKey — when EXTERNAL_API_KEY is
// empty (unconfigured), the middleware MUST reject ALL requests fail-closed.
// An empty key would otherwise let any request through (empty == any) or
// allow trivially empty header values — both are security bugs.
func TestExternalAPIKeyMiddleware_EmptyConfiguredKey(t *testing.T) {
	mw := ExternalAPIKeyMiddleware("") // empty = not configured
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, headerVal := range []string{"", "anything", "correct-key"} {
		req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
		if headerVal != "" {
			req.Header.Set("X-API-Key", headerVal)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("empty configured key, header=%q: status = %d; want 401 (fail-closed)",
				headerVal, rec.Code)
		}
	}
}

// TestExternalAPIKeyMiddleware_ConstantTime — timing: both correct and wrong
// key go through subtle.ConstantTimeCompare; this test just ensures no panic
// on various key lengths and the outcomes are correct.
func TestExternalAPIKeyMiddleware_ConstantTime(t *testing.T) {
	cases := []struct {
		configured string
		provided   string
		want       int
	}{
		{"abc", "abc", http.StatusOK},
		{"abc", "ab", http.StatusUnauthorized},
		{"abc", "abcd", http.StatusUnauthorized},
		{"abc", "", http.StatusUnauthorized},
		{"a-long-secret-key-32bytes-xxxxxxxx", "a-long-secret-key-32bytes-xxxxxxxx", http.StatusOK},
	}

	for _, tc := range cases {
		mw := ExternalAPIKeyMiddleware(tc.configured)
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
		req.Header.Set("X-API-Key", tc.provided)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != tc.want {
			t.Errorf("configured=%q provided=%q: status=%d want=%d",
				tc.configured, tc.provided, rec.Code, tc.want)
		}
	}
}

// containsCI reports whether s contains substr (case-insensitive).
func containsCI(s, substr string) bool {
	sLow := toLower(s)
	subLow := toLower(substr)
	return len(sLow) >= len(subLow) && containsStr(sLow, subLow)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
