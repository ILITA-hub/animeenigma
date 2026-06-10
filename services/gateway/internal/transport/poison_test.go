package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// nextProbe is a terminal handler that records whether it was reached and
// writes a sentinel body, so tests can tell "passed through" from "poisoned".
func nextProbe(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		_, _ = w.Write([]byte("REAL"))
	})
}

func doReq(mw func(http.Handler) http.Handler, remoteAddr, path string) (*httptest.ResponseRecorder, bool) {
	reached := false
	h := mw(nextProbe(&reached))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

func TestPoison_TargetIP_GetsFakeExport(t *testing.T) {
	mw := PoisonMiddleware([]string{"159.195.37.56"}, logger.Default())
	rec, reached := doReq(mw, "159.195.37.56:51000", "/api/users/export/json")

	if reached {
		t.Fatal("real handler must NOT be reached for a poisoned IP on a poisoned path")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var resp poisonExportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("poison body is not valid JSON: %v", err)
	}
	if resp.TotalEntries == 0 || len(resp.Entries) == 0 {
		t.Fatal("poison export must contain fabricated entries")
	}
	if resp.TotalEntries != len(resp.Entries) {
		t.Fatalf("total_entries (%d) must match len(entries) (%d)", resp.TotalEntries, len(resp.Entries))
	}
	e := resp.Entries[0]
	if e.Title == "" || e.AnimeenigmaID == "" || len(e.Genres) == 0 {
		t.Fatalf("fabricated entry looks implausible: %+v", e)
	}
}

func TestPoison_NonTargetIP_PassesThrough(t *testing.T) {
	mw := PoisonMiddleware([]string{"159.195.37.56"}, logger.Default())
	rec, reached := doReq(mw, "8.8.8.8:40000", "/api/users/export/json")

	if !reached {
		t.Fatal("a normal client must reach the real handler")
	}
	if rec.Body.String() != "REAL" {
		t.Fatalf("normal client must get the real response, got %q", rec.Body.String())
	}
}

func TestPoison_TargetIP_UnknownPath_PassesThrough(t *testing.T) {
	mw := PoisonMiddleware([]string{"159.195.37.56"}, logger.Default())
	rec, reached := doReq(mw, "159.195.37.56:51000", "/api/anime?search=naruto")

	if !reached {
		t.Fatal("poisoned IP on a path without a generator must pass through (stealth)")
	}
	if rec.Body.String() != "REAL" {
		t.Fatalf("want real passthrough body, got %q", rec.Body.String())
	}
}

func TestPoison_CIDR_Match(t *testing.T) {
	mw := PoisonMiddleware([]string{"159.195.37.0/24"}, logger.Default())
	_, reached := doReq(mw, "159.195.37.99:1234", "/api/users/export/json")
	if reached {
		t.Fatal("IP inside a poisoned CIDR must be poisoned")
	}
	_, reached2 := doReq(mw, "159.195.38.99:1234", "/api/users/export/json")
	if !reached2 {
		t.Fatal("IP outside the poisoned CIDR must pass through")
	}
}

func TestPoison_EmptySet_IsPassThrough(t *testing.T) {
	mw := PoisonMiddleware(nil, logger.Default())
	_, reached := doReq(mw, "159.195.37.56:51000", "/api/users/export/json")
	if !reached {
		t.Fatal("empty poison set must be a zero-overhead pass-through")
	}
}

func TestPoison_ReRandomizesPerRequest(t *testing.T) {
	mw := PoisonMiddleware([]string{"159.195.37.56"}, logger.Default())
	r1, _ := doReq(mw, "159.195.37.56:1", "/api/users/export/json")
	r2, _ := doReq(mw, "159.195.37.56:2", "/api/users/export/json")
	if r1.Body.String() == r2.Body.String() {
		t.Fatal("two poison responses must differ (no stable fingerprint to diff against)")
	}
}
