package controlplane

import (
	"context"
	"errors"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/google/uuid"
)

// ErrTokenNotFound is returned by EnrollTokenStore.Consume when the token was
// never issued or has already been consumed.
var ErrTokenNotFound = errors.New("enroll token not found or already consumed")

// EnrollTokenStore is the single-use token store abstraction.  The production
// implementation uses Redis (SetNX + key existence check); tests use an
// in-memory fake.
type EnrollTokenStore interface {
	// Consume atomically claims the token.
	//
	// Returns nil on success, ErrTokenNotFound when the token was never
	// issued or has already been consumed, or an underlying store error when
	// the store is unreachable (caller should reject, fail-closed).
	Consume(ctx context.Context, token string) error
}

// WorkerUpserter is the minimal write interface the enroll handler needs from
// the worker repository.
type WorkerUpserter interface {
	Upsert(ctx context.Context, w *domain.UpscaleWorker) error
}

// EnrollRequest is the payload sent by a worker at POST /worker/enroll.
type EnrollRequest struct {
	Token string `json:"token"`
}

// EnrollResponse is returned on a successful enrollment.
type EnrollResponse struct {
	WorkerID string `json:"worker_id"`
	Handle   string `json:"handle"`
	Exp      string `json:"exp"`
	Sig      string `json:"sig"`
}

// Handle processes a worker enroll request.  It is called by the transport
// layer (mounted at POST /worker/enroll in Task 11).
//
// Steps:
//  1. Atomically consume the single-use enroll token.
//  2. Generate a fresh UUID worker ID.
//  3. Upsert a minimal UpscaleWorker row with status="idle".
//  4. Mint a session capability handle bound to the new worker ID.
//  5. Return the EnrollResponse.
//
// Any error from the token store or the repository is propagated to the caller
// as-is; the transport layer maps them to appropriate HTTP status codes.
func Handle(ctx context.Context, store EnrollTokenStore, workers WorkerUpserter, req EnrollRequest) (EnrollResponse, error) {
	// 1. Consume the single-use token (fail-closed).
	if err := store.Consume(ctx, req.Token); err != nil {
		return EnrollResponse{}, err
	}

	// 2. Generate a fresh worker identity.
	workerID := uuid.New().String()

	// 3. Persist the worker row.
	w := &domain.UpscaleWorker{
		WorkerID: workerID,
		Status:   "idle",
	}
	if err := workers.Upsert(ctx, w); err != nil {
		return EnrollResponse{}, err
	}

	// 4. Mint the session capability handle.
	handle, exp, sig := MintSession(workerID, SessionTTL)

	// 5. Return the response.
	return EnrollResponse{
		WorkerID: workerID,
		Handle:   handle,
		Exp:      exp,
		Sig:      sig,
	}, nil
}
