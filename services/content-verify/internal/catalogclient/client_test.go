package catalogclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func server(t *testing.T) *httptest.Server {
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

func TestClientDecodes(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	c := New(srv.URL, srv.URL, srv.Client())
	ctx := context.Background()

	m, err := c.Membership(ctx)
	if err != nil || len(m.Ongoing) != 1 || m.Ongoing[0].EpisodesAired != 28 {
		t.Fatalf("membership: %+v %v", m, err)
	}
	caps, err := c.Capabilities(ctx, "a1")
	if err != nil || len(caps) != 4 {
		t.Fatalf("caps: %+v %v", caps, err)
	}
	tr, err := c.KodikTranslations(ctx, "a1")
	if err != nil || len(tr) != 2 || tr[0].ID != 610 || tr[1].Type != "subtitles" {
		t.Fatalf("translations: %+v %v", tr, err)
	}
	eps, err := c.ScraperEpisodes(ctx, "a1", "gogoanime")
	if err != nil || len(eps) != 2 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	if eps, err := c.ScraperEpisodes(ctx, "a1", "nineanime"); err != nil || eps != nil {
		t.Fatalf("404 must be nil,nil: %v %v", eps, err)
	}
	srvs, err := c.ScraperServers(ctx, "a1", "ep-28", "gogoanime")
	if err != nil || len(srvs) != 2 || srvs[1].Type != "dub" {
		t.Fatalf("servers: %+v %v", srvs, err)
	}
	st, err := c.ScraperStream(ctx, "a1", "ep-28", "hd-1", "sub", "gogoanime")
	if err != nil || st.URL != "https://cdn/x.m3u8" || st.Referer != "https://x/" || st.Intro == nil || st.Intro.End != 180 {
		t.Fatalf("stream: %+v %v", st, err)
	}
}
