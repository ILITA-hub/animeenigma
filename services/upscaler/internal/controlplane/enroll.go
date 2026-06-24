package controlplane

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrTokenNotFound is returned when the enroll token was never issued or has
// already been consumed.
var ErrTokenNotFound = errors.New("enroll token not found or already consumed")

// mintSession is a package-level seam so tests can simulate an unconfigured
// capability (an empty session triple) WITHOUT mutating the global capability
// secret state — capability.Init is sync.Once-gated and cannot be reset from
// outside the capability package, so we override this variable in the C-1
// guard test and restore it via defer. Production always uses MintSession.
var mintSession = MintSession

// EnrollTokenStore is the single-use token store abstraction used by the
// fake-based orchestration unit test (Handle). The PRODUCTION enroll path uses
// the Postgres-backed GormEnrollStore.EnrollTx, which consumes the token and
// upserts the worker inside a single transaction (durable single-use, CD-14).
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

// sessionExpiresAt derives the persistable SessionExpiresAt (I-2) from the
// minted exp string (Unix seconds). Returns nil when exp is unparseable so a
// malformed exp never panics or persists a bogus timestamp.
func sessionExpiresAt(exp string) *time.Time {
	secs, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return nil
	}
	t := time.Unix(secs, 0).UTC()
	return &t
}

// Handle processes a worker enroll request via an injected EnrollTokenStore +
// WorkerUpserter. This is the non-transactional orchestration path exercised by
// the fake-based unit tests; the production path is GormEnrollStore.EnrollTx
// (single-transaction, durable single-use).
//
// Ordering (consistent with EnrollTx): mint+guard FIRST (pure, no side effects),
// THEN consume, THEN upsert. Minting before any state mutation means an
// unconfigured-capability deployment (C-1) is rejected before the token is
// consumed or a worker row is written — the token is NOT burned.
//
// Steps:
//  1. Generate a fresh UUID worker ID.
//  2. Mint a session capability handle bound to the worker ID; if the triple is
//     empty (capability not configured, C-1) reject without side effects.
//  3. Derive SessionExpiresAt from exp (I-2).
//  4. Atomically consume the single-use enroll token (fail-closed).
//  5. Upsert a minimal UpscaleWorker row with status="idle" + SessionExpiresAt.
//  6. Return the EnrollResponse.
func Handle(ctx context.Context, store EnrollTokenStore, workers WorkerUpserter, req EnrollRequest) (EnrollResponse, error) {
	// 1. Generate a fresh worker identity.
	workerID := uuid.New().String()

	// 2. Mint the session capability handle + C-1 guard (pure, no side effects yet).
	handle, exp, sig := mintSession(workerID, SessionTTL)
	if handle == "" || exp == "" || sig == "" {
		return EnrollResponse{}, errors.New("capability not configured; cannot mint session")
	}

	// 3. Derive SessionExpiresAt (I-2).
	sessionExp := sessionExpiresAt(exp)

	// 4. Consume the single-use token (fail-closed). Nothing has been mutated
	//    before this point, so a C-1 rejection above never burns the token.
	if err := store.Consume(ctx, req.Token); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			metrics.UpscaleEnrollTotal.WithLabelValues("bad_token").Inc()
		} else {
			metrics.UpscaleEnrollTotal.WithLabelValues("error").Inc()
		}
		return EnrollResponse{}, err
	}

	// 5. Persist the worker row with the derived session expiry.
	w := &domain.UpscaleWorker{
		WorkerID:         workerID,
		Status:           "idle",
		SessionExpiresAt: sessionExp,
	}
	if err := workers.Upsert(ctx, w); err != nil {
		metrics.UpscaleEnrollTotal.WithLabelValues("error").Inc()
		return EnrollResponse{}, err
	}

	// 6. Return the response.
	metrics.UpscaleEnrollTotal.WithLabelValues("ok").Inc()
	return EnrollResponse{
		WorkerID: workerID,
		Handle:   handle,
		Exp:      exp,
		Sig:      sig,
	}, nil
}

// GormEnrollStore is the production EnrollTokenStore backed by Postgres. Its
// EnrollTx method performs the token-consume AND the worker upsert inside a
// single transaction so a failed upsert rolls back the consume (the token is
// NOT burned and the operator can retry) — fixes I-3.
type GormEnrollStore struct {
	db *gorm.DB
}

// NewGormEnrollStore constructs a GormEnrollStore backed by db.
func NewGormEnrollStore(db *gorm.DB) *GormEnrollStore {
	return &GormEnrollStore{db: db}
}

// Consume atomically claims a single-use token via a conditional UPDATE that
// only matches an unconsumed row. Returns ErrTokenNotFound when the token is
// unknown or already consumed, or the underlying DB error (fail-closed). This
// satisfies EnrollTokenStore so a GormEnrollStore can be passed where the
// interface is expected; the full transactional enroll path is EnrollTx.
func (s *GormEnrollStore) Consume(ctx context.Context, token string) error {
	res := s.db.WithContext(ctx).Model(&domain.UpscaleEnrollToken{}).
		Where("token = ? AND consumed_at IS NULL", token).
		Update("consumed_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrTokenNotFound
	}
	return nil
}

// EnrollTx atomically consumes the token and upserts the worker row in a single
// transaction. The session is minted OUTSIDE/BEFORE the tx (minting is pure /
// in-memory) so the C-1 guard rejects an unconfigured-capability deployment
// before any DB mutation — the token stays unconsumed.
//
// Ordering:
//  1. Generate workerID + mint session + C-1 guard (pure, no side effects).
//  2. Derive SessionExpiresAt from exp (I-2).
//  3. In ONE db.Transaction: conditional consume UPDATE, then worker upsert.
//     - DB error on the consume → rollback (fail-closed), token NOT burned.
//     - 0 rows affected → ErrTokenNotFound (rollback).
//     - upsert error → rollback so the consume is undone (I-3), token NOT burned.
//  4. Return the EnrollResponse.
func (s *GormEnrollStore) EnrollTx(ctx context.Context, req EnrollRequest, ttl time.Duration) (EnrollResponse, error) {
	// 1. Mint first (pure) + C-1 guard.
	workerID := uuid.New().String()
	handle, exp, sig := mintSession(workerID, ttl)
	if handle == "" || exp == "" || sig == "" {
		return EnrollResponse{}, errors.New("capability not configured; cannot mint session")
	}

	// 2. Derive SessionExpiresAt (I-2).
	sessionExp := sessionExpiresAt(exp)

	// 3. Consume + upsert in a single transaction.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Atomic conditional consume — only matches an unconsumed row.
		res := tx.Model(&domain.UpscaleEnrollToken{}).
			Where("token = ? AND consumed_at IS NULL", req.Token).
			Update("consumed_at", time.Now().UTC())
		if res.Error != nil {
			return res.Error // DB error → fail-closed (rollback)
		}
		if res.RowsAffected != 1 {
			return ErrTokenNotFound // unknown or already-consumed → rollback
		}

		// Upsert the worker INSIDE the same tx so a failure rolls back the consume.
		w := &domain.UpscaleWorker{
			WorkerID:         workerID,
			Status:           "idle",
			SessionExpiresAt: sessionExp,
		}
		if uerr := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "worker_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gpu_info", "image_version", "models_available",
				"status", "current_job_id", "current_segment",
				"session_expires_at", "last_heartbeat_at",
			}),
		}).Create(w).Error; uerr != nil {
			return uerr
		}
		return nil
	})
	if err != nil {
		// Observe the production enroll outcome (the test-only Handle path already
		// does this; EnrollTx is the route the live /worker/enroll handler calls,
		// so without this the counter stays dark in prod). Classification mirrors
		// Handle: ErrTokenNotFound → "bad_token", anything else → "error". Counted
		// exactly once here (the router does not double-count).
		if errors.Is(err, ErrTokenNotFound) {
			metrics.UpscaleEnrollTotal.WithLabelValues("bad_token").Inc()
		} else {
			metrics.UpscaleEnrollTotal.WithLabelValues("error").Inc()
		}
		return EnrollResponse{}, err
	}

	// 4. Return the response.
	metrics.UpscaleEnrollTotal.WithLabelValues("ok").Inc()
	return EnrollResponse{
		WorkerID: workerID,
		Handle:   handle,
		Exp:      exp,
		Sig:      sig,
	}, nil
}
