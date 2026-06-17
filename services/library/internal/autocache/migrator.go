package autocache

import (
	"context"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// EpisodeStore is the slice of the EpisodeRepository the one-time migrator needs.
// Defined as an interface seam so migrator_test.go can inject an in-memory fake
// (no live Postgres). *repo.EpisodeRepository satisfies it.
type EpisodeStore interface {
	// ListAdminLegacyPath returns episode rows still on the legacy
	// "{shikimori_id}/{ep}/" prefix (i.e. minio_path NOT LIKE 'aeProvider/%').
	// Already-migrated rows are excluded, which is the backbone of the
	// migrator's idempotency + restart-safety.
	ListAdminLegacyPath(ctx context.Context) ([]domain.Episode, error)
	// UpdateMinioPath repoints a single row's minio_path AFTER its MinIO
	// objects have been Moved.
	UpdateMinioPath(ctx context.Context, id, path string) error
}

// Mover is the slice of minio.Writer the migrator needs: a server-side
// copy-then-delete of every object under src into dst. *minio.Writer satisfies
// it. Defined as a seam so the test injects a fake without live MinIO.
//
// CONTRACT (see minio.Writer.Move): Move copies ALL objects first and, on ANY
// copy error, aborts WITHOUT deleting the sources — so a failed Move leaves the
// source intact and the migrator can safely leave the row on its old path and
// re-attempt on the next boot. The migrator therefore must NOT issue a separate
// delete; Move owns source removal.
type Mover interface {
	Move(ctx context.Context, src, dst string) error
}

// migratorLogger is the structured-logging seam (a subset of *logger.Logger,
// which embeds *zap.SugaredLogger). Keeps the migrator testable with a no-op.
type migratorLogger interface {
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
}

// Migrator performs the one-time relocation of pre-existing admin content into
// the unified autocache pool layout (spec §3.3 / POOL-02). For each admin
// library_episodes row still on the legacy "{shikimori_id}/{ep}/" prefix it
// server-side-Moves the MinIO objects into aeProvider/<mal>/RAW/<ep>/ and THEN
// repoints minio_path. It runs once at boot, AFTER migrations 005/006 apply and
// BEFORE the service serves traffic (spec §10 sequencing invariant: admin rows
// must be in the metered pool before the Phase-10 evictor exists).
//
// Safety properties:
//   - Copy-before-repoint: the row is repointed ONLY after Move succeeds, so a
//     reader following the per-row minio_path never points at a half-moved path.
//   - Idempotent: rows already on aeProvider/ are skipped (the store filter
//     excludes them; the migrator also defends in code).
//   - Restart-safe: a crash mid-run leaves un-migrated rows on the old prefix;
//     the next boot re-lists and re-attempts them. A Move that copied but whose
//     repoint failed re-copies harmlessly (same content, same dst) next boot.
//   - Non-fatal: a single-row failure logs and continues; the run never aborts,
//     and main.go must NOT Fatalw on its error (idempotent re-run on next boot).
//
// D2 (locked): only the RAW/ track exists in v1; SUB/DUB are reserved and are
// intentionally NOT handled here.
type Migrator struct {
	store EpisodeStore
	mover Mover
	log   migratorLogger
}

// NewMigrator wires the migrator from the episode store, the MinIO mover, and a
// logger.
func NewMigrator(store EpisodeStore, mover Mover, log migratorLogger) *Migrator {
	return &Migrator{store: store, mover: mover, log: log}
}

// Migrate relocates every legacy admin episode into the aeProvider/ pool layout
// and returns the count of rows successfully migrated. It never aborts the whole
// run on a single-row failure (see the type doc for the safety properties).
func (m *Migrator) Migrate(ctx context.Context) (migrated int, err error) {
	rows, err := m.store.ListAdminLegacyPath(ctx)
	if err != nil {
		return 0, err
	}

	for i := range rows {
		ep := rows[i]
		newPrefix := RawPrefix(ep.ShikimoriID, ep.EpisodeNumber)

		// Idempotency guard: skip rows already on the unified layout. The SQL
		// filter normally excludes these, but defend in code so a future caller
		// passing an unfiltered slice stays safe.
		if ep.MinioPath == newPrefix || strings.HasPrefix(ep.MinioPath, "aeProvider/") {
			continue
		}

		// Move FIRST. On error, Move has aborted before deleting sources (no
		// data loss) — leave the row on its old path and continue; next boot
		// re-attempts.
		if mvErr := m.mover.Move(ctx, ep.MinioPath, newPrefix); mvErr != nil {
			if m.log != nil {
				m.log.Warnw("autocache migrate: Move failed, leaving row on old path",
					"id", ep.ID, "src", ep.MinioPath, "dst", newPrefix, "error", mvErr)
			}
			continue
		}

		// Repoint ONLY after the copy succeeded. If this fails, the destination
		// objects already exist; a re-run sees the old prefix and re-copies
		// harmlessly. Do NOT count it as migrated.
		if upErr := m.store.UpdateMinioPath(ctx, ep.ID, newPrefix); upErr != nil {
			if m.log != nil {
				m.log.Errorw("autocache migrate: repoint failed AFTER Move (dst exists, re-run safe)",
					"id", ep.ID, "dst", newPrefix, "error", upErr)
			}
			continue
		}

		migrated++
	}

	return migrated, nil
}
