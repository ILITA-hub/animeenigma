package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseScraperEpisodes(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantDub   bool
		wantOK    bool
	}{
		{
			name:      "dub present on one episode",
			body:      `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2,"has_dub":true}]}}`,
			wantCount: 2, wantDub: true, wantOK: true,
		},
		{
			name:      "sub only",
			body:      `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2}]}}`,
			wantCount: 2, wantDub: false, wantOK: true,
		},
		{
			name:      "empty episode list is not a verdict",
			body:      `{"data":{"episodes":[]}}`,
			wantCount: 0, wantDub: false, wantOK: false,
		},
		{
			name:      "garbage is not a verdict",
			body:      `not json`,
			wantCount: 0, wantDub: false, wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, dub, ok := parseScraperEpisodes([]byte(tt.body))
			assert.Equal(t, tt.wantCount, count)
			assert.Equal(t, tt.wantDub, dub)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// Honesty rule: a provider-pinned call may only PROMOTE to true. A sub-only
// answer from one provider must not erase a true verdict established by
// another — miruro in particular is DUB-only.
func TestBackfillEnglishFlags_PinnedCallNeverWritesFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "gogoanime",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}]}}`))

	assert.True(t, f.hasEnglish, "has_english should still be promoted")
	assert.Nil(t, f.englishDub, "pinned sub-only call must not write a dub verdict")
}

func TestBackfillEnglishFlags_PinnedCallStillPromotesTrue(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "miruro",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":true}]}}`))

	if assert.NotNil(t, f.englishDub) {
		assert.True(t, *f.englishDub)
	}
}

func TestBackfillEnglishFlags_UnpinnedCallWritesFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}]}}`))

	if assert.NotNil(t, f.englishDub) {
		assert.False(t, *f.englishDub, "an unpinned chain-wide call is a real negative verdict")
	}
}
