package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEnglishDubRepo struct {
	candidates []domain.EnglishDubCandidate
	touched    []string
	promoted   int64
	promoteErr error
}

func (f *fakeEnglishDubRepo) ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error) {
	return f.candidates, nil
}
func (f *fakeEnglishDubRepo) TouchEnglishDubChecked(ctx context.Context, animeID string) error {
	f.touched = append(f.touched, animeID)
	return nil
}
func (f *fakeEnglishDubRepo) CountEnglishDubUnchecked(ctx context.Context) (int64, error) {
	return int64(len(f.candidates)), nil
}
func (f *fakeEnglishDubRepo) PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error) {
	return f.promoted, f.promoteErr
}

type fakeEnglishDubProbe struct {
	calls  []string
	prefer []string
	status int
	body   string
	err    error
}

func (f *fakeEnglishDubProbe) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	f.calls = append(f.calls, animeID)
	f.prefer = append(f.prefer, prefer)
	return f.status, []byte(f.body), f.err
}

type fakeShed struct{ level int }

func (f *fakeShed) ShouldShed(min int) bool { return f.level >= min }

func newTestBackfiller(r englishDubRepo, p englishDubProbe, s shedChecker) *EnglishDubBackfiller {
	return NewEnglishDubBackfiller(r, p, s, EnglishDubBackfillConfig{
		Interval:   time.Minute,
		OngoingAge: 7 * 24 * time.Hour,
		StaleAge:   30 * 24 * time.Hour,
	}, logger.Default())
}

// The probe must run unpinned so the hook is allowed to write a negative
// verdict — a pinned call can only ever promote.
func TestBackfiller_ProbesUnpinned(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":true}]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	require.Equal(t, []string{"a1"}, p.calls)
	assert.Equal(t, []string{""}, p.prefer, "backfill probe must not be pinned to a provider")
}

// An unreachable provider must still stamp, or the loop re-picks the same
// title on every tick forever.
func TestBackfiller_StampsOnFailedProbe(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{err: errors.New("scraper unreachable")}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

func TestBackfiller_StampsOnNon200(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 503, body: `{}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

// A 200 with an empty list is not a verdict either — stamp so we rotate.
func TestBackfiller_StampsOnEmptyEpisodeList(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

// On a good verdict the hook inside GetScraperEpisodes owns the write, so the
// backfiller must NOT stamp on top of it. The winner must be a dub-tagging
// provider (gogoanime) for a negative verdict to be trustworthy — see
// dubTaggingProviders.
func TestBackfiller_DoesNotTouchOnGoodVerdict(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":false}],"meta":{"provider":"gogoanime"}}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Empty(t, r.touched, "the lazy hook already wrote the verdict")
}

// A positive verdict is trustworthy from any winner, so the hook always
// writes it — the backfiller must not stamp on top.
func TestBackfiller_DoesNotTouchOnPositiveVerdictFromAnyWinner(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":true}],"meta":{"provider":"miruro"}}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Empty(t, r.touched, "the lazy hook already wrote the positive verdict")
}

// When the winning provider can't tag dub (e.g. miruro) or meta.provider is
// missing entirely, backfillEnglishFlags withholds the negative write — it
// only wrote has_english, not has_english_dub/english_dub_checked_at. The
// backfiller must stamp itself so the title still rotates out of the
// candidate list instead of being re-probed every tick.
func TestBackfiller_StampsWhenWinnerCantTagDub(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":false}],"meta":{"provider":"miruro"}}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched, "an untrustworthy negative must still rotate the candidate")
}

func TestBackfiller_StampsWhenMetaProviderMissing(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":false}]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched, "a missing meta.provider must still rotate the candidate")
}

func TestBackfiller_ShedsUnderPressure(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1}]}}`}

	newTestBackfiller(r, p, &fakeShed{level: 1}).tick(context.Background())

	assert.Empty(t, p.calls, "no provider calls while the governor reports Elevated+")
}

func TestBackfiller_NoCandidatesIsQuiet(t *testing.T) {
	r := &fakeEnglishDubRepo{}
	p := &fakeEnglishDubProbe{}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Empty(t, p.calls)
	assert.Empty(t, r.touched)
}

// A missing content_verifications table (pre-content-verify deploy) must not
// kill the loop.
func TestBackfiller_PromoteErrorIsNonFatal(t *testing.T) {
	r := &fakeEnglishDubRepo{
		candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}},
		promoteErr: errors.New("relation content_verifications does not exist"),
	}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":true}]}}`}
	b := newTestBackfiller(r, p, &fakeShed{})

	b.promote(context.Background())
	b.tick(context.Background())

	assert.Equal(t, []string{"a1"}, p.calls, "probe must still run after a failed promote")
}
