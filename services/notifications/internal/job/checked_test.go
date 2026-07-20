package job

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestChecked(t *testing.T) (*CheckedStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return NewCheckedStore(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestCheckedMarkAndRead(t *testing.T) {
	s, _ := newTestChecked(t)
	ctx := context.Background()

	// Nothing checked yet → empty map.
	require.Empty(t, s.LastChecked(ctx, []string{"a1", "a2"}))

	s.MarkChecked(ctx, []string{"a1"})
	got := s.LastChecked(ctx, []string{"a1", "a2"})
	_, ok := got["a1"]
	require.True(t, ok, "a1 must be marked checked")
	_, ok = got["a2"]
	require.False(t, ok, "a2 was never checked")
}

func TestCheckedEmptyInput(t *testing.T) {
	s, _ := newTestChecked(t)
	require.Empty(t, s.LastChecked(context.Background(), nil))
	s.MarkChecked(context.Background(), nil) // no panic
}
