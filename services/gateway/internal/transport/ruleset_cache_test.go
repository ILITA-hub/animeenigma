package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestRulesetCache_failStatic_keepsLastGood(t *testing.T) {
	calls := 0
	fetch := func(ctx context.Context) (rulesetSnapshot, error) {
		calls++
		if calls == 1 {
			return rulesetSnapshot{Flags: map[string]audience{"fanfic": {Roles: []string{"admin"}}}}, nil
		}
		return rulesetSnapshot{}, errors.New("upstream down")
	}
	c := newRulesetCache(fetch, logger.Default())
	c.refresh(context.Background()) // success → loaded
	snap, loaded := c.snapshot()
	if !loaded || len(snap.Flags) != 1 {
		t.Fatalf("after first refresh: loaded=%v flags=%d", loaded, len(snap.Flags))
	}
	c.refresh(context.Background()) // error → keep previous
	snap, loaded = c.snapshot()
	if !loaded || snap.Flags["fanfic"].Roles[0] != "admin" {
		t.Fatalf("fail-static broken: loaded=%v snap=%+v", loaded, snap)
	}
}

func TestRulesetCache_coldStart_notLoaded(t *testing.T) {
	c := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{}, errors.New("down")
	}, logger.Default())
	c.refresh(context.Background())
	if _, loaded := c.snapshot(); loaded {
		t.Fatal("cold start with only-failing fetch must stay unloaded")
	}
}
