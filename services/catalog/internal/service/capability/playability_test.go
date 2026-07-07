package capability

import (
	"math"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBlend_NormalizesAndWeights(t *testing.T) {
	scores := map[string]providerScore{
		"gogoanime":     {ThisAnimeWatch: 2, GlobalWatch: 40, RecentUp: 6},
		"allanime-okru": {ThisAnimeWatch: 1, GlobalWatch: 20, RecentUp: 3},
	}
	b := newBlendData(scores)
	// gogoanime is the max in every term → normalized terms all 1.0, up health.
	idxGogo := b.indexFor("gogoanime", domain.HealthUp)
	wantGogo := wThisAnime*1 + wGlobal*1 + wRecentUp*1 + wHealth*hUp
	if !approx(idxGogo, wantGogo) {
		t.Errorf("gogoanime index = %v, want %v", idxGogo, wantGogo)
	}
	// A down provider ranks below an up provider with identical raw scores.
	up := b.indexFor("allanime-okru", domain.HealthUp)
	down := b.indexFor("allanime-okru", domain.HealthDown)
	if !(up > down) {
		t.Errorf("up (%v) must exceed down (%v)", up, down)
	}
}

func TestBlend_UnknownProviderIsHealthOnly(t *testing.T) {
	b := newBlendData(map[string]providerScore{"x": {GlobalWatch: 5}})
	// A provider absent from the scores map contributes only its health term.
	if got := b.indexFor("not-present", domain.HealthUp); !approx(got, wHealth*hUp) {
		t.Errorf("absent provider index = %v, want health-only %v", got, wHealth*hUp)
	}
}

func TestBlend_NilIsHealthOnly(t *testing.T) {
	var b *blendData // analytics unavailable
	if got := b.indexFor("gogoanime", domain.HealthUp); !approx(got, wHealth*hUp) {
		t.Errorf("nil blend index = %v, want health-only %v", got, wHealth*hUp)
	}
	if got := b.thisAnimeWatch("gogoanime"); got != 0 {
		t.Errorf("nil blend thisAnimeWatch = %v, want 0", got)
	}
}

func TestParseScores_AliasesKodikNoads(t *testing.T) {
	// probe recent_up arrives under "kodik-noads"; cap id is "kodik" → merge.
	raw := map[string]providerScore{
		"kodik":       {GlobalWatch: 10, ThisAnimeWatch: 1},
		"kodik-noads": {RecentUp: 7},
	}
	merged := applyProviderAliases(raw)
	if _, ok := merged["kodik-noads"]; ok {
		t.Error("kodik-noads must be merged away")
	}
	k := merged["kodik"]
	if k.RecentUp != 7 || k.GlobalWatch != 10 || k.ThisAnimeWatch != 1 {
		t.Errorf("kodik merge wrong: %+v", k)
	}
}
