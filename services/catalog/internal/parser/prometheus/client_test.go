package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func vectorJSON(value string) string {
	return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1716600000,%q]}]}}`, value)
}

func TestQuery_ParsesVectorValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, vectorJSON("48201.5"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.Query(context.Background(), "sum(http_requests_total)")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got != 48201.5 {
		t.Fatalf("want 48201.5, got %v", got)
	}
}

func TestQuery_EmptyResultIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Query(context.Background(), "x"); err == nil {
		t.Fatal("expected error on empty result")
	}
}

func TestQuery_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Query(context.Background(), "x"); err == nil {
		t.Fatal("expected error on non-200")
	}
}

func TestHealth_AllUpAndUptime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		switch {
		case q == "count(up == 0) or vector(0)":
			fmt.Fprint(w, vectorJSON("0"))
		case q == "avg(avg_over_time(up[7d]))":
			fmt.Fprint(w, vectorJSON("0.994"))
		default:
			t.Errorf("unexpected query %q", q)
		}
	}))
	defer srv.Close()

	allUp, pct, err := NewClient(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !allUp {
		t.Fatal("want allUp=true")
	}
	if pct < 99.3 || pct > 99.5 {
		t.Fatalf("want ~99.4, got %v", pct)
	}
}

func TestHealth_SomeDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == "count(up == 0) or vector(0)" {
			fmt.Fprint(w, vectorJSON("2"))
			return
		}
		fmt.Fprint(w, vectorJSON("0.8"))
	}))
	defer srv.Close()
	allUp, _, err := NewClient(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if allUp {
		t.Fatal("want allUp=false when 2 targets down")
	}
}
