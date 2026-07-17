package queue

import (
	"context"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
)

// buildTestCatalog mirrors the catalogclient test mux (kept package-local:
// test helpers aren't exported across packages).
func buildTestCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":28}],"top":[{"id":"t1","name":"N","episodes_aired":47}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"anime_id":"a1","families":[{"family":"others","providers":[
			{"provider":"gogoanime","state":"active","group":"en","audios":["sub","dub"]},
			{"provider":"kodik","state":"active","group":"ru","audios":["sub","dub"]},
			{"provider":"hanime","state":"active","group":"adult","audios":["sub"]}]},
			{"family":"aeProvider","providers":[{"provider":"ae","state":"active","group":"firstparty","audios":["dub"],"lang":"en"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/kodik/translations", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":[{"id":610,"title":"AniLibria","type":"voice","episodes_count":28},{"id":734,"title":"Subs","type":"subtitles","episodes_count":28}]}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/episodes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("exclusive") != "true" {
			t.Errorf("scraper/episodes: exclusive=true not set: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("prefer") == "nineanime" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1},{"id":"ep-28","number":28}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("exclusive") != "true" {
			t.Errorf("scraper/servers: exclusive=true not set: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"},{"id":"hd-2","name":"HD-2","type":"dub"}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/stream", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("exclusive") != "true" {
			t.Errorf("scraper/stream: exclusive=true not set: %s", r.URL.RawQuery)
		}
		if q.Get("episode") == "" || q.Get("server") == "" {
			t.Errorf("scraper/stream: episode/server param missing: %s", r.URL.RawQuery)
		}
		if cat := q.Get("category"); cat != "sub" && cat != "dub" {
			t.Errorf("scraper/stream: category not sub|dub: %q", cat)
		}
		w.Write([]byte(`{"success":true,"data":{"stream":{"headers":{"Referer":"https://x/"},"sources":[{"url":"https://cdn/x.m3u8","exp":"1","sig":"s","type":"hls"}],"tracks":[{"file":"a.vtt","label":"English","kind":"captions"}],"intro":{"start":90,"end":180}}}}`))
	})
	return httptest.NewServer(mux)
}

func TestEnumerateUnits(t *testing.T) {
	srv := buildTestCatalog(t)
	defer srv.Close()
	c := catalogclient.New(srv.URL, srv.URL, srv.Client())
	units, err := EnumerateUnits(context.Background(), c, "a1", nil)
	if err != nil {
		t.Fatal(err)
	}
	// gogoanime: 2 servers → 2 units (episode = max number 28, EpisodeID ep-28);
	// kodik: 2 translations → 2 units; ae: 1 synth unit; hanime (adult): skipped.
	var gogo, kodik, ae, adult int
	for _, u := range units {
		switch u.Provider {
		case "gogoanime":
			gogo++
			if u.Episode != 28 || u.EpisodeID != "ep-28" {
				t.Fatalf("gogo unit episode: %+v", u)
			}
			if u.Episodes != 2 { // episode-list LENGTH (2 entries), not the max number
				t.Fatalf("gogo unit episodes-ready = %d, want 2: %+v", u.Episodes, u)
			}
		case "kodik":
			kodik++
			if u.Key.Team == "" {
				t.Fatalf("kodik unit needs team key: %+v", u)
			}
			if u.Episodes != 28 { // per-team episodes_count from the roster
				t.Fatalf("kodik unit episodes-ready = %d, want 28: %+v", u.Episodes, u)
			}
			// Kodik is synth-only (owner decision 2026-07-17): roster truth.
			if u.Synth == nil || u.Synth.Status != domain.StatusVerified {
				t.Fatalf("kodik unit must carry a verified synth verdict: %+v", u)
			}
			switch u.Key.Category {
			case "dub":
				if u.Synth.Audio == nil || u.Synth.Audio.Lang != "ru" || !u.Synth.Audio.Verified {
					t.Fatalf("kodik voice synth must claim verified ru dub: %+v", u.Synth)
				}
			case "sub":
				if !u.Synth.RawAudio || u.Synth.Hardsub == nil || u.Synth.Hardsub.Lang != "ru" || !u.Synth.Hardsub.Verified {
					t.Fatalf("kodik subtitles synth must claim raw audio + burned ru: %+v", u.Synth)
				}
			}
		case "ae":
			ae++
			if u.Synth == nil || u.Synth.Audio == nil || u.Synth.Audio.Lang != "en" {
				t.Fatalf("ae synth lang: %+v", u)
			}
		case "hanime":
			adult++
		}
	}
	if gogo != 2 || kodik != 2 || ae != 1 || adult != 0 {
		t.Fatalf("unit counts gogo=%d kodik=%d ae=%d adult=%d", gogo, kodik, ae, adult)
	}
}

func TestEnumerateAll(t *testing.T) {
	srv := buildTestCatalog(t)
	defer srv.Close()
	c := catalogclient.New(srv.URL, srv.URL, srv.Client())

	all, err := EnumerateAll(context.Background(), c, "a1", nil)
	if err != nil {
		t.Fatal(err)
	}

	// EnumerateUnits (the wrapper) must return the exact same verify units.
	units, err := EnumerateUnits(context.Background(), c, "a1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != len(all.Verify) {
		t.Fatalf("EnumerateUnits len=%d != EnumerateAll.Verify len=%d", len(units), len(all.Verify))
	}
	for i := range units {
		if !reflect.DeepEqual(units[i], all.Verify[i]) {
			t.Fatalf("EnumerateUnits[%d] = %+v != EnumerateAll.Verify[%d] = %+v", i, units[i], i, all.Verify[i])
		}
	}

	var gogo, kodik, hanime, ae int
	var gogoEpisodes []int
	for _, s := range all.Skip {
		switch s.Provider {
		case "gogoanime":
			gogo++
			gogoEpisodes = append(gogoEpisodes, s.Episode)
		case "kodik":
			kodik++
			if s.Team == "" {
				t.Fatalf("kodik skip unit needs Team: %+v", s)
			}
			if s.TeamID == 0 {
				t.Fatalf("kodik skip unit needs TeamID: %+v", s)
			}
		case "hanime":
			hanime++
		case "ae":
			ae++
		}
	}
	if gogo != 2 {
		t.Fatalf("gogoanime skip units = %d, want 2: %+v", gogo, all.Skip)
	}
	if len(gogoEpisodes) != 2 || gogoEpisodes[0] != 1 || gogoEpisodes[1] != 28 {
		t.Fatalf("gogoanime skip episodes not ascending [1,28]: %+v", gogoEpisodes)
	}
	if kodik != 56 { // 2 translations x 28 episodes
		t.Fatalf("kodik skip units = %d, want 56", kodik)
	}
	if hanime != 0 {
		t.Fatalf("hanime (adult) must have no skip units, got %d", hanime)
	}
	if ae != 0 {
		t.Fatalf("ae (firstparty) must have no skip units in v1, got %d", ae)
	}

	// Verify gogoanime EpisodeIDs ep-1/ep-28, ascending.
	var gogoIDs []string
	for _, s := range all.Skip {
		if s.Provider == "gogoanime" {
			gogoIDs = append(gogoIDs, s.EpisodeID)
		}
	}
	if len(gogoIDs) != 2 || gogoIDs[0] != "ep-1" || gogoIDs[1] != "ep-28" {
		t.Fatalf("gogoanime skip EpisodeIDs not [ep-1, ep-28]: %+v", gogoIDs)
	}
}
