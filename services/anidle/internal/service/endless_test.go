package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTokenStore struct{ m map[string]string }

func newFakeTokenStore() *fakeTokenStore { return &fakeTokenStore{m: map[string]string{}} }
func (f *fakeTokenStore) PutToken(_ context.Context, token, animeID string) error {
	f.m[token] = animeID
	return nil
}
func (f *fakeTokenStore) GetToken(_ context.Context, token string) (string, bool, error) {
	v, ok := f.m[token]
	return v, ok, nil
}

func TestEndless_NewRoundThenGuess(t *testing.T) {
	ts := newFakeTokenStore()
	n := 0
	pick := func(pool []PoolAnimeRef) PoolAnimeRef { n++; return pool[n%len(pool)] }
	svc := NewEndlessService(&fakePool{pool: samplePool()}, ts, pick)
	ctx := context.Background()

	round, err := svc.NewRound(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, round.RoundToken)

	secretID, ok, _ := ts.GetToken(ctx, round.RoundToken)
	require.True(t, ok)

	out, err := svc.Guess(ctx, round.RoundToken, secretID)
	require.NoError(t, err)
	assert.True(t, out.Solved)
	require.NotNil(t, out.Answer)
}

func TestEndless_Guess_BadToken(t *testing.T) {
	svc := NewEndlessService(&fakePool{pool: samplePool()}, newFakeTokenStore(),
		func(p []PoolAnimeRef) PoolAnimeRef { return p[0] })
	_, err := svc.Guess(context.Background(), "nope", "a")
	require.Error(t, err)
}
