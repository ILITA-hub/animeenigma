package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

type fakeCapSvc struct {
	rep domain.CapabilityReport
	err error
}

func (f fakeCapSvc) Report(_ context.Context, animeID string) (domain.CapabilityReport, error) {
	rep := f.rep
	rep.AnimeID = animeID
	return rep, f.err
}

func TestCapabilitiesHandler_OK(t *testing.T) {
	rep := domain.CapabilityReport{
		Families: []domain.SourceFamily{
			{Family: "ourenglish", Providers: []domain.ProviderCap{{Provider: "allanime"}}},
		},
	}
	h := handler.NewCapabilitiesHandler(fakeCapSvc{rep: rep}, nil)
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data domain.CapabilityReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AnimeID != "abc" {
		t.Errorf("expected AnimeID=abc, got %q", body.Data.AnimeID)
	}
	if len(body.Data.Families) != 1 {
		t.Errorf("expected 1 family, got %d", len(body.Data.Families))
	}
	if body.Data.Families[0].Family != "ourenglish" {
		t.Errorf("expected family=ourenglish, got %q", body.Data.Families[0].Family)
	}
	if len(body.Data.Families[0].Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(body.Data.Families[0].Providers))
	}
}

// blockingCapSvc blocks in Report until the passed ctx is cancelled, then
// returns ctx.Err(). It records how long Report ran so the test can assert the
// handler imposed its own request-scoped deadline (rather than waiting on the
// upstream client's 30s timeout / hanging forever).
type blockingCapSvc struct {
	ranFor chan time.Duration
}

func (f *blockingCapSvc) Report(ctx context.Context, _ string) (domain.CapabilityReport, error) {
	start := time.Now()
	<-ctx.Done()
	f.ranFor <- time.Since(start)
	return domain.CapabilityReport{}, ctx.Err()
}

// TestCapabilitiesHandler_AppliesTimeout asserts the handler bounds the
// capability fan-out with its own context.WithTimeout. Without it, a stuck
// upstream leg (e.g. the Kodik parser) would hang the request up to the server
// WriteTimeout. The fake Report blocks until ctx fires; the handler's timeout
// must release it well under any global server budget.
func TestCapabilitiesHandler_AppliesTimeout(t *testing.T) {
	svc := &blockingCapSvc{ranFor: make(chan time.Duration, 1)}
	h := handler.NewCapabilitiesHandler(svc, nil)
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		// Handler returned — it must have bounded the call itself.
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not return within 10s — no request-scoped timeout applied")
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on timed-out report, got %d", rec.Code)
	}

	select {
	case d := <-svc.ranFor:
		if d > 10*time.Second {
			t.Fatalf("Report ran for %v — handler timeout too loose", d)
		}
	case <-time.After(time.Second):
		t.Fatal("Report did not observe ctx cancellation")
	}
}

func TestCapabilitiesHandler_ServiceError(t *testing.T) {
	// The handler wraps any service error via errors.Internal → httputil.Error,
	// which maps CodeInternal → 500 regardless of the underlying error type.
	h := handler.NewCapabilitiesHandler(fakeCapSvc{err: context.DeadlineExceeded}, nil)
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}
