package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseScraperEpisodes(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantCount    int
		wantDub      bool
		wantProvider string
		wantOK       bool
	}{
		{
			name:         "dub present on one episode",
			body:         `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2,"has_dub":true}],"meta":{"provider":"gogoanime"}}}`,
			wantCount:    2, wantDub: true, wantProvider: "gogoanime", wantOK: true,
		},
		{
			name:         "sub only",
			body:         `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2}],"meta":{"provider":"gogoanime"}}}`,
			wantCount:    2, wantDub: false, wantProvider: "gogoanime", wantOK: true,
		},
		{
			name:         "missing meta.provider",
			body:         `{"data":{"episodes":[{"number":1,"has_dub":false}]}}`,
			wantCount:    1, wantDub: false, wantProvider: "", wantOK: true,
		},
		{
			name:      "empty episode list is not a verdict",
			body:      `{"data":{"episodes":[]}}`,
			wantCount: 0, wantDub: false, wantProvider: "", wantOK: false,
		},
		{
			name:      "garbage is not a verdict",
			body:      `not json`,
			wantCount: 0, wantDub: false, wantProvider: "", wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, dub, provider, ok := parseScraperEpisodes([]byte(tt.body))
			assert.Equal(t, tt.wantCount, count)
			assert.Equal(t, tt.wantDub, dub)
			assert.Equal(t, tt.wantProvider, provider)
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

// Honesty rule (I-1): only gogoanime tags has_dub per episode; every other
// provider leaves it at the zero value, so its "no dub" is absence of
// evidence, not evidence of absence. A negative verdict is trustworthy only
// when the call was unpinned AND the winning provider is a dub-tagging one.
func TestBackfillEnglishFlags_UnpinnedGogoanimeWinnerWritesFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}],"meta":{"provider":"gogoanime"}}}`))

	if assert.NotNil(t, f.englishDub) {
		assert.False(t, *f.englishDub, "an unpinned chain-wide answer from gogoanime is a real negative verdict")
	}
}

func TestBackfillEnglishFlags_UnpinnedMiruroWinnerDoesNotWriteFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}],"meta":{"provider":"miruro"}}}`))

	assert.True(t, f.hasEnglish, "has_english should still be promoted")
	assert.Nil(t, f.englishDub, "miruro doesn't tag has_dub, so its \"no dub\" isn't trustworthy")
}

func TestBackfillEnglishFlags_UnpinnedMissingMetaDoesNotWriteFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(context.Background(), "a1", "",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}]}}`))

	assert.True(t, f.hasEnglish, "has_english should still be promoted")
	assert.Nil(t, f.englishDub, "a missing meta.provider is never trustworthy for a negative verdict")
}
