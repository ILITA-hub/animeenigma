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
	titles        []string
	err           error
	calls         int
}

func (f *fakeLogicB) LogicBContext(_ context.Context, _, _ string) (string, int, bool, []string, error) {
	f.calls++
	return f.shikimoriID, f.episodesAired, f.watching, f.titles, f.err
}

// fakeDemand captures Want() calls.
type fakeDemand struct {
	malID   string
	episode int
	reason  string
	titles  []string
	calls   int
}

func (f *fakeDemand) Want(malID string, episode int, reason string, titles []string) {
	f.calls++
	f.malID = malID
	f.episode = episode
	f.reason = reason
	f.titles = titles
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
	return subReq(player, lang, "sub", ep)
}

func subReq(player, lang, watchType string, ep int) *domain.UpdateProgressRequest {
	return &domain.UpdateProgressRequest{
		AnimeID:       "anime-uuid",
		EpisodeNumber: ep,
		Progress:      30,
		Duration:      1400,
		Player:        player,
		Language:      lang,
		WatchType:     watchType,
	}
}

func TestUpdateProgress_FiresNextEpForRawAudioWatcher(t *testing.T) {
	// ANY sub combo carries original Japanese audio, plus the ae/raw players.
	cases := []struct {
		name      string
		player    string
		lang      string
		watchType string
	}{
		{"ae player", "ae", "en", "sub"},
		{"raw player", "raw", "ja", "sub"},
		{"kodik ru sub", "kodik", "ru", "sub"},
		{"english en sub (gogoanime)", "english", "en", "sub"},
		{"hianime en sub", "hianime", "en", "sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true, titles: []string{"JP", "Romaji", "EN"}}
			d := &fakeDemand{}
			s := newTestProgressService(&fakeUpserter{}, lb, d)

			_, err := s.UpdateProgress(context.Background(), "user-1", subReq(tc.player, tc.lang, tc.watchType, 5))
			if err != nil {
				t.Fatalf("UpdateProgress err = %v", err)
			}
			if d.calls != 1 {
				t.Fatalf("Want calls = %d, want 1", d.calls)
			}
			if d.malID != "57466" || d.episode != 6 || d.reason != "next_ep" {
				t.Errorf("Want(%q,%d,%q), want (57466,6,next_ep)", d.malID, d.episode, d.reason)
			}
			// Titles from LogicBContext must propagate to the demand (so the library
			// Planner can search trackers by title).
			if len(d.titles) != 3 || d.titles[0] != "JP" || d.titles[1] != "Romaji" || d.titles[2] != "EN" {
				t.Errorf("demand titles = %#v, want [JP Romaji EN]", d.titles)
			}
		})
	}
}

func TestUpdateProgress_NoFireForDubCombo(t *testing.T) {
	// DUB combos carry replaced (non-Japanese) audio — never trigger a RAW prefetch.
	cases := []struct{ player, lang, watchType string }{
		{"kodik", "ru", "dub"},
		{"english", "en", "dub"},
		{"hianime", "en", "dub"},
	}
	for _, tc := range cases {
		lb := &fakeLogicB{shikimoriID: "57466", episodesAired: 12, watching: true}
		d := &fakeDemand{}
		s := newTestProgressService(&fakeUpserter{}, lb, d)
		_, err := s.UpdateProgress(context.Background(), "user-1", subReq(tc.player, tc.lang, tc.watchType, 5))
		if err != nil {
			t.Fatalf("UpdateProgress err = %v", err)
		}
		if d.calls != 0 {
			t.Errorf("%s/%s/%s fired %d demands, want 0", tc.player, tc.lang, tc.watchType, d.calls)
		}
		if lb.calls != 0 {
			t.Errorf("%s/%s/%s did %d lookups, want 0 (dub gate short-circuits)", tc.player, tc.lang, tc.watchType, lb.calls)
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

func TestPrefersRawAudio(t *testing.T) {
	// {player, watchType}. ANY sub combo qualifies (raw JP audio + subtitles),
	// plus the always-raw ae/raw players. Only dub is excluded.
	yes := [][2]string{
		{"kodik", "sub"}, {"english", "sub"}, {"hianime", "sub"}, {"consumet", "sub"},
		{"ae", "sub"}, {"raw", "sub"}, {"ae", "dub"}, {"raw", "dub"},
	}
	no := [][2]string{{"kodik", "dub"}, {"english", "dub"}, {"hianime", "dub"}, {"", ""}}
	for _, c := range yes {
		if !prefersRawAudio(c[0], c[1]) {
			t.Errorf("prefersRawAudio(%q,%q) = false, want true", c[0], c[1])
		}
	}
	for _, c := range no {
		if prefersRawAudio(c[0], c[1]) {
			t.Errorf("prefersRawAudio(%q,%q) = true, want false", c[0], c[1])
		}
	}
}
