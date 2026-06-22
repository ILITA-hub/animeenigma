package kodik

import (
	"sync"
	"testing"
)

// TestClient_TokenConcurrentAccess exercises the token getters/setters from
// many goroutines so `go test -race` catches any regression of the data race
// the singleton client previously had (token/tokenExpires read+written without
// a lock). Pure in-memory — no network (NewClientWithToken).
func TestClient_TokenConcurrentAccess(t *testing.T) {
	c := NewClientWithToken("seed")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.storeToken("rotated") }()
		go func() { defer wg.Done(); _ = c.tokenSnapshot() }()
	}
	wg.Wait()

	if got := c.tokenSnapshot(); got != "rotated" && got != "seed" {
		t.Fatalf("unexpected token after concurrent access: %q", got)
	}
}
