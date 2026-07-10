package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type fakeShed struct{ level int }

func (f *fakeShed) ShouldShed(min int) bool { return f.level >= min }
func (f *fakeShed) Level() int              { return f.level }

func TestSkipIfDegraded(t *testing.T) {
	s := &JobService{log: logger.Default()}

	if s.skipIfDegraded("top_anime_sync") {
		t.Fatal("nil shed checker must never skip")
	}

	shed := &fakeShed{level: 0}
	s.SetShedChecker(shed)
	if s.skipIfDegraded("top_anime_sync") {
		t.Fatal("level 0 must not skip")
	}

	shed.level = 1
	if !s.skipIfDegraded("top_anime_sync") {
		t.Fatal("level 1 must skip heavy jobs")
	}
	shed.level = 2
	if !s.skipIfDegraded("autocache_logic_a") {
		t.Fatal("level 2 must skip heavy jobs")
	}
}
