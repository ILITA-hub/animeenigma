package controlplane

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
)

// TestMain initialises the capability secret once for all controlplane tests.
// Using "test-secret" so VerifySession can actually validate minted handles.
func TestMain(m *testing.M) {
	capability.Init("test-secret")
	os.Exit(m.Run())
}

// --------------------------------------------------------------------------
// Handwritten fakes
// --------------------------------------------------------------------------

// fakeTokenStore is an in-memory single-use token store.
//
//   - tokens[token] = false  → token is valid and not yet consumed
//   - tokens[token] = true   → token has already been consumed
//   - token absent           → token was never issued
type fakeTokenStore struct {
	mu         sync.Mutex
	tokens     map[string]bool // token → consumed?
	consumeErr error           // if set, Consume returns this error regardless
}

func newFakeStore(validTokens ...string) *fakeTokenStore {
	m := make(map[string]bool, len(validTokens))
	for _, t := range validTokens {
		m[t] = false
	}
	return &fakeTokenStore{tokens: m}
}

func (f *fakeTokenStore) Consume(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.consumeErr != nil {
		return f.consumeErr
	}
	consumed, exists := f.tokens[token]
	if !exists || consumed {
		return ErrTokenNotFound
	}
	f.tokens[token] = true
	return nil
}

// fakeWorkerUpsert captures upserted workers in memory.
type fakeWorkerUpsert struct {
	mu      sync.Mutex
	workers []*domain.UpscaleWorker
	err     error // if set, Upsert returns this error
}

func (f *fakeWorkerUpsert) Upsert(_ context.Context, w *domain.UpscaleWorker) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workers = append(f.workers, w)
	return nil
}

func (f *fakeWorkerUpsert) last() *domain.UpscaleWorker {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.workers) == 0 {
		return nil
	}
	return f.workers[len(f.workers)-1]
}

// --------------------------------------------------------------------------
// Handle tests
// --------------------------------------------------------------------------

// TestHandle_ValidToken verifies the happy path:
//   - EnrollResponse carries a non-empty WorkerID
//   - The minted session verifies correctly with VerifySession
func TestHandle_ValidToken(t *testing.T) {
	store := newFakeStore("secret-token-1")
	workers := &fakeWorkerUpsert{}

	resp, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "secret-token-1"})
	if err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if resp.WorkerID == "" {
		t.Error("EnrollResponse.WorkerID is empty, want non-empty UUID")
	}
	// Session fields are non-empty when capability is configured.
	if resp.Handle == "" || resp.Exp == "" || resp.Sig == "" {
		t.Errorf("session triple (handle=%q exp=%q sig=%q) should be non-empty when capability is configured",
			resp.Handle, resp.Exp, resp.Sig)
	}
	// The session must verify as of now.
	if !VerifySession(resp.WorkerID, resp.Exp, resp.Sig, time.Now()) {
		t.Error("VerifySession returned false for freshly minted session")
	}
}

// TestHandle_ValidToken_WorkerUpserted checks that Upsert is called with the
// correct worker ID and status="idle".
func TestHandle_ValidToken_WorkerUpserted(t *testing.T) {
	store := newFakeStore("token-upsert")
	workers := &fakeWorkerUpsert{}

	resp, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "token-upsert"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	w := workers.last()
	if w == nil {
		t.Fatal("no worker was upserted")
	}
	if w.WorkerID != resp.WorkerID {
		t.Errorf("upserted WorkerID = %q, want %q", w.WorkerID, resp.WorkerID)
	}
	if w.Status != "idle" {
		t.Errorf("upserted Status = %q, want %q", w.Status, "idle")
	}
}

// TestHandle_ValidToken_TokenConsumed verifies that a token cannot be reused.
func TestHandle_ValidToken_TokenConsumed(t *testing.T) {
	store := newFakeStore("one-time-token")
	workers := &fakeWorkerUpsert{}

	// First call: must succeed.
	if _, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "one-time-token"}); err != nil {
		t.Fatalf("first Handle call: %v", err)
	}
	// Second call with the same token: must fail with ErrTokenNotFound.
	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "one-time-token"})
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("second call: got %v, want ErrTokenNotFound", err)
	}
}

// TestHandle_UnknownToken verifies that an unissued token is rejected.
func TestHandle_UnknownToken(t *testing.T) {
	store := newFakeStore() // no valid tokens
	workers := &fakeWorkerUpsert{}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "not-issued"})
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("got %v, want ErrTokenNotFound", err)
	}
	// No worker must have been upserted.
	if workers.last() != nil {
		t.Error("worker was upserted despite invalid token")
	}
}

// TestHandle_StoreError verifies fail-closed behaviour when the token store
// returns an unexpected error (e.g. Redis outage).
func TestHandle_StoreError(t *testing.T) {
	storeErr := errors.New("redis: connection refused")
	store := &fakeTokenStore{
		tokens:     map[string]bool{"tok": false},
		consumeErr: storeErr,
	}
	workers := &fakeWorkerUpsert{}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "tok"})
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
	if workers.last() != nil {
		t.Error("worker was upserted despite store error (fail-closed violation)")
	}
}

// TestHandle_DBError verifies that a database error is propagated.
func TestHandle_DBError(t *testing.T) {
	store := newFakeStore("db-error-token")
	dbErr := errors.New("db: constraint violation")
	workers := &fakeWorkerUpsert{err: dbErr}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "db-error-token"})
	if err == nil {
		t.Fatal("expected error from DB failure, got nil")
	}
}

// TestHandle_ConsumedToken_SecondCall is an explicit test for the second-call
// rejection scenario (complementary to TestHandle_ValidToken_TokenConsumed,
// examining the error type more carefully).
func TestHandle_ConsumedToken_SecondCall(t *testing.T) {
	store := newFakeStore("consumed-tok")
	workers := &fakeWorkerUpsert{}

	// Consume successfully.
	if _, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "consumed-tok"}); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Attempt replay.
	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "consumed-tok"})
	if err == nil {
		t.Fatal("replay should be rejected")
	}
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("replay error = %v, want ErrTokenNotFound", err)
	}
	// Exactly one worker should have been upserted.
	workers.mu.Lock()
	n := len(workers.workers)
	workers.mu.Unlock()
	if n != 1 {
		t.Errorf("upserted worker count = %d, want 1", n)
	}
}
