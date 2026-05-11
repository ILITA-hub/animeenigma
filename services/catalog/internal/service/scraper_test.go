package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// --- Fakes used by the scraper-service tests ----------------------------

// fakeAnimeFetcher is a minimal implementation of the animeFetcher
// interface that scraper.go is contracted against. It returns whatever
// (anime, err) tuple the test sets up.
type fakeAnimeFetcher struct {
	anime *domain.Anime
	err   error
	calls int32
}

func (f *fakeAnimeFetcher) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.anime, f.err
}

// fakeScraperForwarder records the args every method was called with so the
// service-layer tests can assert pass-through behavior.
type fakeScraperForwarder struct {
	gotEpisodesMALID int
	gotEpisodesPrefer string

	gotServersMALID   int
	gotServersEpisode string
	gotServersPrefer  string

	gotStreamMALID    int
	gotStreamEpisode  string
	gotStreamServer   string
	gotStreamCategory string
	gotStreamPrefer   string

	gotHealthCalls int32

	// Reply for the next call.
	replyStatus int
	replyBody   []byte
	replyErr    error
}

func (f *fakeScraperForwarder) GetEpisodes(ctx context.Context, malID int, prefer string) (int, []byte, error) {
	f.gotEpisodesMALID = malID
	f.gotEpisodesPrefer = prefer
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperForwarder) GetServers(ctx context.Context, malID int, episodeID, prefer string) (int, []byte, error) {
	f.gotServersMALID = malID
	f.gotServersEpisode = episodeID
	f.gotServersPrefer = prefer
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperForwarder) GetStream(ctx context.Context, malID int, episodeID, serverID, category, prefer string) (int, []byte, error) {
	f.gotStreamMALID = malID
	f.gotStreamEpisode = episodeID
	f.gotStreamServer = serverID
	f.gotStreamCategory = category
	f.gotStreamPrefer = prefer
	return f.replyStatus, f.replyBody, f.replyErr
}

func (f *fakeScraperForwarder) GetHealth(ctx context.Context) (int, []byte, error) {
	atomic.AddInt32(&f.gotHealthCalls, 1)
	return f.replyStatus, f.replyBody, f.replyErr
}

// newScraperOps builds a scraperOps under test using the supplied fakes.
func newScraperOps(repo animeFetcher, scr scraperForwarder) *scraperOps {
	return &scraperOps{animeRepo: repo, scraperClient: scr}
}

// --- Tests --------------------------------------------------------------

// Test 1 — happy path: ShikimoriID parses to int, scraper forwards.
func TestCatalogService_GetScraperEpisodes_HappyPath(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-1", ShikimoriID: "12345"}}
	scr := &fakeScraperForwarder{
		replyStatus: http.StatusServiceUnavailable,
		replyBody:   []byte(`{"error":"not-yet-implemented","phase":15}`),
		replyErr:    nil,
	}
	ops := newScraperOps(repo, scr)

	status, body, err := ops.GetScraperEpisodes(context.Background(), "uuid-1", "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 503 {
		t.Errorf("status = %d, want 503", status)
	}
	if !strings.Contains(string(body), "not-yet-implemented") {
		t.Errorf("body = %q, missing not-yet-implemented", string(body))
	}
	if scr.gotEpisodesMALID != 12345 {
		t.Errorf("scraper got malID=%d, want 12345", scr.gotEpisodesMALID)
	}
	if scr.gotEpisodesPrefer != "animepahe" {
		t.Errorf("scraper got prefer=%q, want animepahe", scr.gotEpisodesPrefer)
	}
}

// Test 2 — animeRepo returns NotFound → service returns NotFound.
func TestCatalogService_GetScraperEpisodes_AnimeNotFound(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: nil, err: liberrors.NotFound("anime")}
	scr := &fakeScraperForwarder{}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperEpisodes(context.Background(), "00000000-0000-0000-0000-000000000000", "")
	if err == nil {
		t.Fatal("expected error for unknown anime, got nil")
	}
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeNotFound {
		t.Errorf("err = %v, want NotFound AppError", err)
	}
	if scr.gotEpisodesMALID != 0 {
		t.Error("scraper should not be called when anime lookup fails")
	}
}

// Test 3 — anime has neither ShikimoriID nor MALID → 422 marker (errMalIDUnavailable).
func TestCatalogService_GetScraperEpisodes_NoMALOrShikimoriID(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-2", ShikimoriID: "", MALID: ""}}
	scr := &fakeScraperForwarder{}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperEpisodes(context.Background(), "uuid-2", "")
	if err == nil {
		t.Fatal("expected error for missing mal_id, got nil")
	}
	if !errors.Is(err, ErrMalIDUnavailable) {
		t.Errorf("err = %v, want errors.Is(err, ErrMalIDUnavailable)", err)
	}
	if scr.gotEpisodesMALID != 0 {
		t.Error("scraper should not be called when mal_id is unavailable")
	}
}

// Test 4 — ShikimoriID field is what's used when valid (project memory:
// Shikimori IDs == MAL IDs for anime).
func TestCatalogService_GetScraperEpisodes_ShikimoriIDFromShikimoriField(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-3", ShikimoriID: "42", MALID: ""}}
	scr := &fakeScraperForwarder{replyStatus: 503, replyBody: []byte(`{}`)}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperEpisodes(context.Background(), "uuid-3", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scr.gotEpisodesMALID != 42 {
		t.Errorf("scraper got malID=%d, want 42 (from ShikimoriID)", scr.gotEpisodesMALID)
	}
}

// Test 5 — ShikimoriID is garbage, MALID is valid → service uses MALID.
func TestCatalogService_GetScraperEpisodes_ShikimoriIDInvalid_FallbackMALID(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-4", ShikimoriID: "not-a-number", MALID: "777"}}
	scr := &fakeScraperForwarder{replyStatus: 503, replyBody: []byte(`{}`)}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperEpisodes(context.Background(), "uuid-4", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scr.gotEpisodesMALID != 777 {
		t.Errorf("scraper got malID=%d, want 777 (MALID fallback)", scr.gotEpisodesMALID)
	}
}

// Test 6 — GetScraperServers passes episode + prefer through.
func TestCatalogService_GetScraperServers_PassesEpisodeQuery(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-5", ShikimoriID: "5"}}
	scr := &fakeScraperForwarder{replyStatus: 503, replyBody: []byte(`{}`)}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperServers(context.Background(), "uuid-5", "ep-1", "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scr.gotServersMALID != 5 {
		t.Errorf("scraper got malID=%d, want 5", scr.gotServersMALID)
	}
	if scr.gotServersEpisode != "ep-1" {
		t.Errorf("scraper got episode=%q, want ep-1", scr.gotServersEpisode)
	}
	if scr.gotServersPrefer != "animepahe" {
		t.Errorf("scraper got prefer=%q, want animepahe", scr.gotServersPrefer)
	}
}

// Test 7 — GetScraperStream passes every query through.
func TestCatalogService_GetScraperStream_PassesAllQuery(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "uuid-6", ShikimoriID: "9"}}
	scr := &fakeScraperForwarder{replyStatus: 503, replyBody: []byte(`{}`)}
	ops := newScraperOps(repo, scr)

	_, _, err := ops.GetScraperStream(context.Background(), "uuid-6", "ep-2", "srv-1", "sub", "animepahe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scr.gotStreamMALID != 9 {
		t.Errorf("scraper got malID=%d, want 9", scr.gotStreamMALID)
	}
	if scr.gotStreamEpisode != "ep-2" {
		t.Errorf("scraper got episode=%q, want ep-2", scr.gotStreamEpisode)
	}
	if scr.gotStreamServer != "srv-1" {
		t.Errorf("scraper got server=%q, want srv-1", scr.gotStreamServer)
	}
	if scr.gotStreamCategory != "sub" {
		t.Errorf("scraper got category=%q, want sub", scr.gotStreamCategory)
	}
	if scr.gotStreamPrefer != "animepahe" {
		t.Errorf("scraper got prefer=%q, want animepahe", scr.gotStreamPrefer)
	}
}

// Test 8 — GetScraperHealth does NOT call the animeRepo. It is a
// service-wide endpoint (the path-level animeId is structural only).
func TestCatalogService_GetScraperHealth_NoLookup(t *testing.T) {
	repo := &fakeAnimeFetcher{anime: &domain.Anime{ID: "ignored", ShikimoriID: "1"}}
	scr := &fakeScraperForwarder{
		replyStatus: http.StatusOK,
		replyBody:   []byte(`{"success":true,"data":{"providers":{}}}`),
	}
	ops := newScraperOps(repo, scr)

	status, body, err := ops.GetScraperHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if !strings.Contains(string(body), "providers") {
		t.Errorf("body = %q, missing providers", string(body))
	}
	if atomic.LoadInt32(&repo.calls) != 0 {
		t.Error("animeRepo.GetByID must NOT be called for health")
	}
	if atomic.LoadInt32(&scr.gotHealthCalls) != 1 {
		t.Error("scraper.GetHealth must be called exactly once")
	}
}
