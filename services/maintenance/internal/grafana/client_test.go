package grafana

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const alertsPath = "/api/alertmanager/grafana/api/v2/alerts"

// The Grafana alertmanager API requires authentication. The client must send
// basic auth so the poll returns the JSON array of alerts instead of a 401
// error object.
func TestGetFiringAlerts_SendsBasicAuthAndParses(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		sawAuth = ok && u == "admin" && p == "secret"
		w.Header().Set("Content-Type", "application/json")
		if !sawAuth {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized","statusCode":401}`))
			return
		}
		_, _ = w.Write([]byte(`[{"labels":{"alertname":"High Error Rate","service":"streaming"},"annotations":{"summary":"boom"},"status":{"state":"active"}}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "admin", "secret")
	alerts, err := c.GetFiringAlerts()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawAuth {
		t.Fatal("client did not send basic auth credentials")
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(alerts))
	}
}

// A non-200 response (e.g. unauthenticated 401) returns a JSON OBJECT, not an
// array. The client must surface the status code clearly, NOT a confusing
// "cannot unmarshal object into []grafana.Alert" parse error.
func TestGetFiringAlerts_Non200_ReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized","statusCode":401}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", "")
	_, err := c.GetFiringAlerts()
	if err == nil {
		t.Fatal("expected an error on HTTP 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should mention status 401, got: %v", err)
	}
	if strings.Contains(err.Error(), "unmarshal") || strings.Contains(err.Error(), "parse") {
		t.Fatalf("error should be status-based, not a parse error: %v", err)
	}
}
