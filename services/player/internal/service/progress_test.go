package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// fakeLogicB stubs the repo Logic-B lookup seam.
type fakeLogicB struct {
	shikimoriID   string
	episodesAired int
	watching      bool
	err           error
	calls         int
}

func (f *fakeLogicB) LogicBContext(_ context.Context, _, _ string) (string, int, bool, error) {
	f.calls++
	return f.shikimoriID, f.episodesAired, f.watching, f.err
}

// fakeDemand captures Want() calls.
type fakeDemand struct {
	malID   string
	episode int
	reason  string
	calls   int
}

func (f *fakeDemand) Want(malID string, episode int, reason string) {
	f.calls++
	f.malID = malID
	f.episode = episode
	f.reason = reason
}

// fakeUpserter is a no-op progress upserter so UpdateProgress's persistence
// path succeeds without a DB. It satisfies progressUpserter.
type fakeUpserter struct {
	err error
}

func (f *fakeUpserter) UpsertProgress(_ context.Context, _ *domain.WatchProgress) error {
	return f.err
}

func newTestProgressService(up progressUpserter, lb logicBLookup, d demandFirer) *ProgressService {
	return &ProgressService{
		progressRepo: nil, // not exercised by the Logic-B fire-path tests
		prefService:  nil,
		upsert:       up,
		logicB:       lb,
		demand:       d,
		log:          logger.Default(),
	}
}

func jpReq(player, lang string, ep int) *domain.UpdateProgressRequest {
	return &domain.UpdateProgressRequest{
		AnimeID:       "anime-uuid",
		EpisodeNumber: ep,
		Progress:      30,
		Duration:      1400,
		Player:        player,
		Language:      lang,
	}
}

func TestUpdateProgress_FiresNextEpForJPAudioWatcher(t *testing.T) {
	cases := []struct {
		name   string
		player string
		lang   string
	}{
		{"ae player", "ae", "en"},
		{"raw player", "raw", "ja"},
		{"language ja", "kodik", "ja"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true}
			d := &fakeDemand{}
			s := newTestProgressService(&fakeUpserter{}, lb, d)

			_, err := s.UpdateProgress(context.Background(), "user-1", jpReq(tc.player, tc.lang, 5))
			if err != nil {
				t.Fatalf("UpdateProgress err = %v", err)
			}
			if d.calls != 1 {
				t.Fatalf("Want calls = %d, want 1", d.calls)
			}
			if d.malID != "57466" || d.episode != 6 || d.reason != "next_ep" {
				t.Errorf("Want(%q,%d,%q), want (57466,6,next_ep)", d.malID, d.episode, d.reason)
			}
		})
	}
}

func TestUpdateProgress_NoFireForNonJPCombo(t *testing.T) {
	cases := []struct{ player, lang string }{
		{"kodik", "ru"},
		{"animelib", "ru"},
		{"english", "en"},
	}
	for _, tc := range cases {
		lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true}
		d := &fakeDemand{}
		s := newTestProgressService(&fakeUpserter{}, lb, d)
		_, err := s.UpdateProgress(context.Background(), "user-1", jpReq(tc.player, tc.lang, 5))
		if err != nil {
			t.Fatalf("UpdateProgress err = %v", err)
		}
		if d.calls != 0 {
			t.Errorf("%s/%s fired %d demands, want 0", tc.player, tc.lang, d.calls)
		}
		if lb.calls != 0 {
			t.Errorf("%s/%s did %d lookups, want 0 (JP gate short-circuits)", tc.player, tc.lang, lb.calls)
		}
	}
}

func TestUpdateProgress_NoFireWhenNotWatching(t *testing.T) {
	lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: false}
	d := &fakeDemand{}
	s := newTestProgressService(&fakeUpserter{}, lb, d)
	_, err := s.UpdateProgress(context.Background(), "user-1", jpReq("ae", "ja", 5))
	if err != nil {
		t.Fatalf("UpdateProgress err = %v", err)
	}
	if d.calls != 0 {
		t.Errorf("not-watching fired %d demands, want 0", d.calls)
	}
}

func TestUpdateProgress_NoFireWhenNextBeyondAired(t *testing.T) {
	// Watching ep 12, only 12 aired → N+1=13 > aired → no fire.
	lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true}
	d := &fakeDemand{}
	s := newTestProgressService(&fakeUpserter{}, lb, d)
	_, err := s.UpdateProgress(context.Background(), "user-1", jpReq("ae", "ja", 12))
	if err != nil {
		t.Fatalf("UpdateProgress err = %v", err)
	}
	if d.calls != 0 {
		t.Errorf("over-aired fired %d demands, want 0", d.calls)
	}
}

func TestUpdateProgress_NoFireOnEmptyShikimoriID(t *testing.T) {
	lb := &fakeLogicB{shikimoriID: "", episodesAired: 12, watching: true}
	d := &fakeDemand{}
	s := newTestProgressService(&fakeUpserter{}, lb, d)
	_, err := s.UpdateProgress(context.Background(), "user-1", jpReq("ae", "ja", 5))
	if err != nil {
		t.Fatalf("UpdateProgress err = %v", err)
	}
	if d.calls != 0 {
		t.Errorf("empty shikimori_id fired %d demands, want 0", d.calls)
	}
}

func TestUpdateProgress_LookupErrorDoesNotFailHeartbeat(t *testing.T) {
	lb := &fakeLogicB{err: errors.New("db down")}
	d := &fakeDemand{}
	s := newTestProgressService(&fakeUpserter{}, lb, d)
	prog, err := s.UpdateProgress(context.Background(), "user-1", jpReq("ae", "ja", 5))
	if err != nil {
		t.Fatalf("lookup error must NOT fail UpdateProgress: %v", err)
	}
	if prog == nil {
		t.Fatal("expected a progress result despite the lookup error")
	}
	if d.calls != 0 {
		t.Errorf("lookup error fired %d demands, want 0", d.calls)
	}
}

func TestUpdateProgress_NilDemandIsSafe(t *testing.T) {
	lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true}
	s := newTestProgressService(&fakeUpserter{}, lb, nil)
	if _, err := s.UpdateProgress(context.Background(), "user-1", jpReq("ae", "ja", 5)); err != nil {
		t.Fatalf("nil demand must not break UpdateProgress: %v", err)
	}
}

func TestIsJPAudio(t *testing.T) {
	yes := [][2]string{{"ae", "en"}, {"raw", "en"}, {"kodik", "ja"}, {"ae", "ja"}}
	no := [][2]string{{"kodik", "ru"}, {"animelib", "ru"}, {"english", "en"}, {"", ""}}
	for _, c := range yes {
		if !isJPAudio(c[0], c[1]) {
			t.Errorf("isJPAudio(%q,%q) = false, want true", c[0], c[1])
		}
	}
	for _, c := range no {
		if isJPAudio(c[0], c[1]) {
			t.Errorf("isJPAudio(%q,%q) = true, want false", c[0], c[1])
		}
	}
}
