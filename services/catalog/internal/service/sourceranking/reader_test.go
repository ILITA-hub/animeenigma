package sourceranking

import (
	"context"
	"testing"
)

// fakeGetter is the minimal cache surface the reader needs.
type fakeGetter struct{ vals map[string]string }

func (f fakeGetter) GetString(_ context.Context, key string) (string, bool) {
	v, ok := f.vals[key]
	return v, ok
}

func TestReadRanking_GlobalAndAnime(t *testing.T) {
	f := fakeGetter{vals: map[string]string{
		"player_ranking:global":       `[{"provider":"kodik","score":0.9,"reached_rate":0.95,"ok_rate":0.97,"p95_ms":1800,"stall_rate":0.02,"samples":120}]`,
		"player_ranking:anime:uuid-1": `[{"provider":"allanime","score":0.8,"reached_rate":0.85,"ok_rate":0.9,"p95_ms":2200,"stall_rate":0.05,"samples":30}]`,
	}}
	r := NewReader(f)
	out := r.Read(context.Background(), "uuid-1")
	if len(out.Global) != 1 || out.Global[0].Provider != "kodik" {
		t.Errorf("global = %+v", out.Global)
	}
	if len(out.PerAnime) != 1 || out.PerAnime[0].Provider != "allanime" {
		t.Errorf("perAnime = %+v", out.PerAnime)
	}
}

func TestReadRanking_MissingKeysAreEmpty(t *testing.T) {
	r := NewReader(fakeGetter{vals: map[string]string{}})
	out := r.Read(context.Background(), "nope")
	if len(out.Global) != 0 || len(out.PerAnime) != 0 {
		t.Errorf("want empty, got %+v", out)
	}
}
