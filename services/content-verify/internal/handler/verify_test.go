package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

type handlerFixture struct {
	h   *VerifyHandler
	sig *signals.Signals
}

func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()

	// Membership endpoint for the (unused-by-these-tests but constructor
	// required) queue.Engine.
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"ongoing":[],"top":[]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sig := signals.New(rdb)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ContentVerification{}); err != nil {
		t.Fatal(err)
	}
	store := repo.NewStore(db)

	engine := queue.NewEngine(cat, sig, store, 720*time.Hour, false, nil)
	h := NewVerifyHandler(store, sig, engine, nil)

	// Seed one row so Verdicts has something to summarize.
	err = store.UpsertUnit(context.Background(), "a-1", "gogoanime", domain.UnitVerdict{
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 3,
		Status: domain.StatusVerified, Audio: &domain.AudioVerdict{Lang: "en", Confidence: 0.98, Verified: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	return &handlerFixture{h: h, sig: sig}
}

func TestVerdictsReturnsSummaryAndUnits(t *testing.T) {
	f := newHandlerFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/internal/verify/verdicts?anime_id=a-1", nil)
	rec := httptest.NewRecorder()
	f.h.Verdicts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			AnimeID   string `json:"anime_id"`
			Providers []struct {
				Provider string                 `json:"provider"`
				Summary  domain.ProviderSummary `json:"summary"`
				Units    []domain.UnitVerdict   `json:"units"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if !body.Success {
		t.Fatal("success must be true")
	}
	if body.Data.AnimeID != "a-1" {
		t.Fatalf("anime_id = %q", body.Data.AnimeID)
	}
	if len(body.Data.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", body.Data.Providers)
	}
	p := body.Data.Providers[0]
	if p.Provider != "gogoanime" {
		t.Fatalf("provider = %q", p.Provider)
	}
	if p.Summary.Status != "verified" {
		t.Fatalf("summary.status = %q, want verified: %+v", p.Summary.Status, p.Summary)
	}
	if len(p.Units) != 1 || p.Units[0].Key.Server != "hd-1" {
		t.Fatalf("units = %+v", p.Units)
	}
}

func TestVerdictsMissingAnimeIDIs400(t *testing.T) {
	f := newHandlerFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/verify/verdicts", nil)
	rec := httptest.NewRecorder()
	f.h.Verdicts(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHintRecordsVisitAndReturns204(t *testing.T) {
	f := newHandlerFixture(t)
	body, _ := json.Marshal(map[string]string{"anime_id": "a-1", "visitor": "u:alice", "source": "visit"})
	req := httptest.NewRequest(http.MethodPost, "/internal/verify/hint", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	f.h.Hint(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if n := f.sig.UniqueVisitors(context.Background(), "a-1"); n != 1 {
		t.Fatalf("UniqueVisitors = %d, want 1", n)
	}
}

func TestHintWithoutVisitorIs400(t *testing.T) {
	f := newHandlerFixture(t)
	body, _ := json.Marshal(map[string]string{"anime_id": "a-1", "source": "visit"})
	req := httptest.NewRequest(http.MethodPost, "/internal/verify/hint", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	f.h.Hint(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestQueueReturnsEntriesEnvelope(t *testing.T) {
	f := newHandlerFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/verify/queue", nil)
	rec := httptest.NewRecorder()
	f.h.Queue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Entries []queue.QueueEntry `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if !body.Success {
		t.Fatal("success must be true")
	}
	if body.Data.Entries == nil {
		t.Fatal("entries must be an (empty) array, not null")
	}
}
