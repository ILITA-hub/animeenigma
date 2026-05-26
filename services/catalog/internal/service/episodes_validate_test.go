package service

// Watch-Together workstream / Phase 04 — WT-STATE-02.
//
// Unit coverage for EpisodesValidateService. Uses handwritten fakes
// (no testify/mock) to inject the EpisodesLookupService + AnimeRepo
// surfaces.

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// fakeLookup implements episodesLookupAdapter via injected closures.
// Each call to LatestAvailable consumes the next closure in `script`;
// running out is a test bug (calls more than expected).
type fakeLookup struct {
	t      *testing.T
	script []func(shikimoriID, player, translationID, watchType, language string) (EpisodesLookupResult, error)
	calls  int
}

func (f *fakeLookup) LatestAvailable(
	_ context.Context,
	shikimoriID, player, translationID, watchType, language string,
) (EpisodesLookupResult, error) {
	if f.calls >= len(f.script) {
		f.t.Fatalf("fakeLookup: unexpected call #%d to LatestAvailable", f.calls+1)
	}
	fn := f.script[f.calls]
	f.calls++
	return fn(shikimoriID, player, translationID, watchType, language)
}

// fakeAnimeRepo implements animeRepoAdapter.
type fakeAnimeRepoValidate struct {
	byShikimori map[string]*domain.Anime
	err         error
}

func (f *fakeAnimeRepoValidate) GetByShikimoriID(_ context.Context, sid string) (*domain.Anime, error) {
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.byShikimori[sid]
	if !ok {
		return nil, nil
	}
	return a, nil
}

// returns a script entry that yields a fixed LatestAvailableEpisode.
func returnLatest(n int) func(string, string, string, string, string) (EpisodesLookupResult, error) {
	return func(_, _, _, _, _ string) (EpisodesLookupResult, error) {
		return EpisodesLookupResult{LatestAvailableEpisode: n}, nil
	}
}

func returnErr(err error) func(string, string, string, string, string) (EpisodesLookupResult, error) {
	return func(_, _, _, _, _ string) (EpisodesLookupResult, error) {
		return EpisodesLookupResult{}, err
	}
}

// -----------------------------------------------------------------
// Happy paths
// -----------------------------------------------------------------

func TestValidateEpisode_Kodik_Happy(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(12),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "5", "42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid || got.Reason != "" {
		t.Fatalf("want Valid=true Reason=empty, got %+v", got)
	}
	if lookup.calls != 1 {
		t.Fatalf("want 1 lookup call, got %d", lookup.calls)
	}
}

func TestValidateEpisode_AnimeLib_Happy(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(24),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "animelib", "10", "100", "sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatalf("want Valid=true, got %+v", got)
	}
}

func TestValidateEpisode_OurEnglish_Permissive_Happy(t *testing.T) {
	lookup := &fakeLookup{t: t} // must not be called
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "ourenglish", "3", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatalf("ourenglish permissive: want Valid=true, got %+v", got)
	}
	if lookup.calls != 0 {
		t.Fatalf("ourenglish must not consult lookup, got %d calls", lookup.calls)
	}
}

func TestValidateEpisode_Hanime_Permissive_Happy(t *testing.T) {
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, &fakeAnimeRepoValidate{}, nil)
	got, err := svc.ValidateEpisode(context.Background(), "111", "hanime", "1", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatalf("hanime permissive: want Valid=true, got %+v", got)
	}
}

func TestValidateEpisode_Raw_Permissive_Happy(t *testing.T) {
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, &fakeAnimeRepoValidate{}, nil)
	got, err := svc.ValidateEpisode(context.Background(), "111", "raw", "7", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatalf("raw permissive: want Valid=true, got %+v", got)
	}
}

// -----------------------------------------------------------------
// Negative — Valid=false branches (200 OK from handler)
// -----------------------------------------------------------------

func TestValidateEpisode_Kodik_EpisodeOverMax(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(5),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "9999", "42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonEpisodeUnavailable {
		t.Fatalf("want Valid=false Reason=%q, got %+v", ReasonEpisodeUnavailable, got)
	}
}

func TestValidateEpisode_Kodik_NonNumericEpisode(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(12),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "abc", "42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonEpisodeUnavailable {
		t.Fatalf("want Valid=false Reason=%q, got %+v", ReasonEpisodeUnavailable, got)
	}
}

func TestValidateEpisode_Kodik_ZeroOrNegativeEpisode(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(12),
		returnLatest(12),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	for _, ep := range []string{"0", "-3"} {
		got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", ep, "42", "")
		if err != nil {
			t.Fatalf("ep=%q unexpected error: %v", ep, err)
		}
		if got.Valid || got.Reason != ReasonEpisodeUnavailable {
			t.Fatalf("ep=%q want Valid=false Reason=%q, got %+v", ep, ReasonEpisodeUnavailable, got)
		}
	}
}

func TestValidateEpisode_Kodik_EmptyEpisode_FullMode(t *testing.T) {
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnLatest(12),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "", "42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonEpisodeUnavailable {
		t.Fatalf("empty episode in full-mode: want Valid=false Reason=%q, got %+v",
			ReasonEpisodeUnavailable, got)
	}
}

func TestValidateEpisode_Raw_EmptyEpisode_Permissive(t *testing.T) {
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, &fakeAnimeRepoValidate{}, nil)
	got, err := svc.ValidateEpisode(context.Background(), "111", "raw", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonEpisodeUnavailable {
		t.Fatalf("permissive empty episode: want Valid=false Reason=%q, got %+v",
			ReasonEpisodeUnavailable, got)
	}
}

func TestValidateEpisode_AnimeLib_TranslationNotFound(t *testing.T) {
	// LatestAvailable returns AppError(CodeNotFound) — translation
	// invalid for this (anime, player). Maps to TRANSLATION_UNAVAILABLE.
	lookup := &fakeLookup{t: t, script: []func(string, string, string, string, string) (EpisodesLookupResult, error){
		returnErr(apperrors.NotFound("no episode available for combo")),
	}}
	svc := NewEpisodesValidateService(lookup, &fakeAnimeRepoValidate{}, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "animelib", "1", "999", "sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonTranslationUnavailable {
		t.Fatalf("want Valid=false Reason=%q, got %+v", ReasonTranslationUnavailable, got)
	}
}

// -----------------------------------------------------------------
// Translation-omitted (player-change) mode
// -----------------------------------------------------------------

func TestValidateEpisode_PlayerChange_Kodik_AnimeExists(t *testing.T) {
	repo := &fakeAnimeRepoValidate{
		byShikimori: map[string]*domain.Anime{
			"57466": {ID: "uuid-1", ShikimoriID: "57466"},
		},
	}
	// LatestAvailable MUST NOT be called in translation-omitted mode.
	lookup := &fakeLookup{t: t}
	svc := NewEpisodesValidateService(lookup, repo, nil)

	got, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "1", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Valid {
		t.Fatalf("player-change with existing anime: want Valid=true, got %+v", got)
	}
	if lookup.calls != 0 {
		t.Fatalf("player-change must not consult lookup, got %d calls", lookup.calls)
	}
}

func TestValidateEpisode_PlayerChange_AnimeLib_AnimeMissing(t *testing.T) {
	repo := &fakeAnimeRepoValidate{
		byShikimori: map[string]*domain.Anime{}, // empty
	}
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, repo, nil)

	got, err := svc.ValidateEpisode(context.Background(), "999999", "animelib", "1", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valid || got.Reason != ReasonPlayerUnavailable {
		t.Fatalf("player-change with missing anime: want Valid=false Reason=%q, got %+v",
			ReasonPlayerUnavailable, got)
	}
}

func TestValidateEpisode_PlayerChange_RepoError(t *testing.T) {
	repo := &fakeAnimeRepoValidate{err: errors.New("db blew up")}
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, repo, nil)

	_, err := svc.ValidateEpisode(context.Background(), "57466", "kodik", "1", "", "")
	if err == nil {
		t.Fatalf("want error from repo failure, got nil")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok || appErr.Code != apperrors.CodeInternal {
		t.Fatalf("want apperrors.CodeInternal, got %v", err)
	}
}

// -----------------------------------------------------------------
// Input validation — returns *error* (mapped to HTTP 400)
// -----------------------------------------------------------------

func TestValidateEpisode_UnknownPlayer(t *testing.T) {
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, &fakeAnimeRepoValidate{}, nil)
	_, err := svc.ValidateEpisode(context.Background(), "57466", "bogus", "1", "42", "")
	if err == nil {
		t.Fatalf("want error for unknown player, got nil")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok || appErr.Code != apperrors.CodeInvalidInput {
		t.Fatalf("want apperrors.CodeInvalidInput, got %v", err)
	}
}

func TestValidateEpisode_EmptyShikimoriID(t *testing.T) {
	svc := NewEpisodesValidateService(&fakeLookup{t: t}, &fakeAnimeRepoValidate{}, nil)
	_, err := svc.ValidateEpisode(context.Background(), "", "kodik", "1", "42", "")
	if err == nil {
		t.Fatalf("want error for empty shikimori_id, got nil")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok || appErr.Code != apperrors.CodeInvalidInput {
		t.Fatalf("want apperrors.CodeInvalidInput, got %v", err)
	}
}

// -----------------------------------------------------------------
// IsValidPlayer (small helper, but exercised by handler tests too)
// -----------------------------------------------------------------

func TestIsValidPlayer(t *testing.T) {
	for _, p := range []string{"kodik", "animelib", "ourenglish", "hanime", "raw"} {
		if !IsValidPlayer(p) {
			t.Errorf("IsValidPlayer(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"", "bogus", "Kodik", " kodik"} {
		if IsValidPlayer(p) {
			t.Errorf("IsValidPlayer(%q) = true, want false", p)
		}
	}
}
