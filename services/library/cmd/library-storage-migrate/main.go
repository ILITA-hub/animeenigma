// Command library-storage-migrate is a one-shot, restart-safe operator tool
// that migrates library episode content from the local self-hosted MinIO
// backend to external S3, one episode prefix at a time, with a copy-verify-flip-
// delete safety sequence.
//
// Selection is `storage='minio' AND source=<-source>` (default autocache): admin
// content stays local, and a row already flipped to s3 is invisible to the
// selection query — so the run is idempotent and a re-run only ever picks up
// rows still awaiting migration.
//
// Per-episode sequence (NON-NEGOTIABLE — this is destructive prod data):
//
//  1. List(minio, prefix)                 → source object count + total bytes
//  2. CopyPrefix(minio → s3, prefix)      → server-side cross-backend copy
//  3. List(s3, prefix)                    → target object count + total bytes
//  4. verify count AND bytes match (and > 0). Any mismatch → LOG + SKIP,
//     leaving BOTH the row (still minio) and the local objects UNTOUCHED.
//  5. UpdateStorage(row, 's3')            → flip the DB row (s3 now authoritative)
//  6. DeletePrefix(minio, prefix)         → reclaim local disk
//
// A crash between (2) and (5) leaves the row on minio → re-run recopies (a safe
// same-key overwrite) and re-verifies. A crash between (5) and (6) leaves a
// benign dual-presence state (row=s3, stale minio objects) — the RECONCILE pass
// (below) heals it on the next run by re-verifying s3 then deleting the minio
// leftovers.
//
// Flip-failure matrix (step 5 outcomes; the evictor selects from the same
// storage='minio' pool with NO coordination, so the row can vanish or change
// under us between selection and flip):
//
//	UpdateStorage → NotFound (row vanished — evictor race):
//	    the evictor deletes objects THEN the row, so the minio prefix is its
//	    responsibility — leave minio alone. Our just-copied s3 prefix is an
//	    orphan nothing references (reconcile only revisits rows flipped to
//	    s3), so UNDO it (delete the s3 prefix). Count as skip.
//	UpdateStorage → other error, re-read shows storage='s3':
//	    a concurrent flip won; the copy is verified and the row is
//	    authoritative on s3 — success. No deletes here: local cleanup belongs
//	    to whoever flipped (or the next run's reconcile pass).
//	UpdateStorage → other error, re-read shows storage='minio':
//	    flip genuinely failed. UNDO the s3 copy (it would leak forever
//	    otherwise), leave minio + row untouched, count as skip — the next run
//	    reselects and retries the whole sequence.
//	UpdateStorage → other error, re-read shows NotFound:
//	    row vanished mid-flip — same as the NotFound case: undo s3, leave
//	    minio alone, skip.
//	UpdateStorage → other error, re-read ALSO errors:
//	    state unknowable — touch NOTHING (the s3 prefix may leak until an
//	    operator re-runs; that beats deleting data whose DB state is
//	    unknown). Count as skip, log loudly.
//
// It runs INSIDE the library container (DB + storage-service reachability) via
// `docker compose run --rm --entrypoint /app/library-storage-migrate library
// [flags]`. It never touches the running service.
//
// Flags:
//
//	-source  episode source filter (default "autocache")
//	-dry-run print the full plan (episodes, prefixes, sizes) and exit; zero writes
//	-limit N cap the number of episodes MIGRATED this run (0 = no cap)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ILITA-hub/animeenigma/libs/database"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

// objectStore is the narrow slice of *storageclient.Client the migration
// sequence uses — a seam so the safety-critical branches of migrateOne /
// reconcileLeftovers are unit-testable with a fake.
type objectStore interface {
	List(ctx context.Context, storage, prefix string) ([]storageclient.Object, error)
	CopyPrefix(ctx context.Context, fromStorage, toStorage, prefix string) (int, int64, error)
	DeletePrefix(ctx context.Context, storage, prefix string) (int, error)
}

// episodeStore is the narrow slice of *repo.EpisodeRepository migrateOne uses.
type episodeStore interface {
	UpdateStorage(ctx context.Context, id string, storage string) error
	GetByID(ctx context.Context, id string) (*domain.Episode, error)
}

func main() {
	var (
		source = flag.String("source", "autocache", "episode source filter (admin|autocache); only these rows are considered")
		dryRun = flag.Bool("dry-run", false, "print the full migration plan and exit without any writes")
		limit  = flag.Int("limit", 0, "cap the number of episodes MIGRATED this run (0 = no cap)")
	)
	flag.Parse()

	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load", "error", err)
	}

	ctx := context.Background()

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect", "error", err)
	}
	defer db.Close()
	episodeRepo := repo.NewEpisodeRepository(db.DB)

	// Object I/O goes straight through the storage-service client — this CLI
	// needs List / CopyPrefix / DeletePrefix, which the higher-level storagegw
	// adapter does not expose.
	client := storageclient.New(cfg.Storage.URL)

	src := domain.EpisodeSource(*source)

	// --- Selection ---------------------------------------------------------
	// Rows still on minio awaiting migration.
	pending, err := episodeRepo.ListByStorageSource(ctx, domain.BackendMinio, src)
	if err != nil {
		log.Fatalw("list migratable episodes", "error", err)
	}
	// Rows already flipped to s3 — reconcile candidates (a crash between flip and
	// delete may have left stale minio objects under their prefix).
	flipped, err := episodeRepo.ListByStorageSource(ctx, domain.BackendS3, src)
	if err != nil {
		log.Fatalw("list already-migrated episodes", "error", err)
	}

	// --- Dry-run plan ------------------------------------------------------
	if *dryRun {
		printPlan(ctx, log, client, pending, flipped, *limit, *source)
		return
	}

	// --- Reconcile pass: heal any crash-between-flip-and-delete leftovers --
	reconciled, reconcileSkipped := reconcileLeftovers(ctx, log, client, flipped)

	// --- Migrate pass ------------------------------------------------------
	var migrated, skipped int
	var migratedBytes int64
	for i, ep := range pending {
		if *limit > 0 && migrated >= *limit {
			log.Infow("reached -limit; stopping migrate pass", "limit", *limit, "processed", i)
			break
		}
		ok, bytes := migrateOne(ctx, log, client, episodeRepo, ep)
		if ok {
			migrated++
			migratedBytes += bytes
		} else {
			skipped++
		}
	}

	log.Infow("storage migration complete",
		"source", *source,
		"migrated", migrated, "migrated_bytes", migratedBytes,
		"skipped", skipped,
		"reconciled", reconciled, "reconcile_skipped", reconcileSkipped,
		"pending_selected", len(pending), "flipped_scanned", len(flipped))
	fmt.Printf("\nMIGRATED=%d BYTES=%d SKIPPED=%d RECONCILED=%d RECONCILE_SKIPPED=%d\n",
		migrated, migratedBytes, skipped, reconciled, reconcileSkipped)
	if skipped > 0 || reconcileSkipped > 0 {
		// Non-zero exit so the operator notices any left-behind row, but only
		// AFTER every other row was processed (one bad prefix never aborts the run).
		os.Exit(1)
	}
}

// migrateOne runs the copy-verify-flip-delete sequence for a single episode.
// Returns (true, bytesMigrated) on success; (false, 0) on any skip. A skip
// leaves BOTH the DB row (still minio, if it still exists) and the local
// objects untouched; a just-copied s3 prefix is undone where the flip-failure
// matrix (package doc) says it would otherwise leak.
func migrateOne(
	ctx context.Context,
	log *logger.Logger,
	store objectStore,
	episodes episodeStore,
	ep domain.Episode,
) (bool, int64) {
	prefix := ep.MinioPath
	fields := []any{"episode_id", ep.ID, "shikimori_id", ep.ShikimoriID, "episode", ep.EpisodeNumber, "prefix", prefix}

	// 1. Source inventory.
	srcObjs, err := store.List(ctx, domain.BackendMinio, prefix)
	if err != nil {
		log.Errorw("migrate skip: list minio failed", append(fields, "error", err)...)
		return false, 0
	}
	srcCount, srcBytes := countBytes(srcObjs)
	if srcCount == 0 {
		log.Warnw("migrate skip: row on minio but no objects under prefix (anomaly)", fields...)
		return false, 0
	}

	// 2. Cross-backend copy (server-side).
	copied, copiedBytes, err := store.CopyPrefix(ctx, domain.BackendMinio, domain.BackendS3, prefix)
	if err != nil {
		log.Errorw("migrate skip: copy minio to s3 failed", append(fields, "error", err)...)
		return false, 0
	}

	// 3. Target inventory.
	dstObjs, err := store.List(ctx, domain.BackendS3, prefix)
	if err != nil {
		log.Errorw("migrate skip: list s3 failed after copy (local objects left intact)", append(fields, "error", err)...)
		return false, 0
	}
	dstCount, dstBytes := countBytes(dstObjs)

	// 4. Verify BOTH count and bytes match. Mismatch → skip, untouched.
	if dstCount != srcCount || dstBytes != srcBytes {
		log.Errorw("migrate skip: post-copy verify MISMATCH — leaving row on minio",
			append(fields,
				"src_count", srcCount, "src_bytes", srcBytes,
				"dst_count", dstCount, "dst_bytes", dstBytes,
				"copied", copied, "copied_bytes", copiedBytes)...)
		return false, 0
	}

	// 5. Flip the DB row — s3 is now authoritative. Failure outcomes follow
	// the flip-failure matrix in the package doc.
	if err := episodes.UpdateStorage(ctx, ep.ID, domain.BackendS3); err != nil {
		if isNotFound(err) {
			// Row vanished between selection and flip (evictor race). The
			// evictor owns the minio prefix; our s3 copy is an unreferenced
			// orphan — undo it.
			undoS3Copy(ctx, log, store, prefix, fields, "row vanished before flip (evicted concurrently)")
			return false, 0
		}
		// Transient flip failure — re-read to decide which state we are in.
		cur, gerr := episodes.GetByID(ctx, ep.ID)
		switch {
		case gerr == nil && cur.Storage == domain.BackendS3:
			// A concurrent flip won; the copy is verified and the row is
			// authoritative on s3 — success. Local cleanup belongs to whoever
			// flipped (or the next run's reconcile pass).
			log.Warnw("flip errored but row is already s3 (concurrent flip won); leaving local cleanup to that run",
				append(fields, "error", err)...)
			return true, srcBytes
		case gerr == nil:
			// Row still on minio: the flip genuinely failed. Undo the s3 copy —
			// nothing would ever revisit it (reconcile only iterates rows
			// already flipped to s3), so it would leak forever.
			undoS3Copy(ctx, log, store, prefix, fields, fmt.Sprintf("flip row to s3 failed: %v", err))
			return false, 0
		case isNotFound(gerr):
			// Row vanished mid-flip — same as the NotFound flip outcome.
			undoS3Copy(ctx, log, store, prefix, fields, "row vanished during flip (evicted concurrently)")
			return false, 0
		default:
			// State unknowable — touch NOTHING. A leaked s3 prefix beats
			// deleting data whose DB state is unknown.
			log.Errorw("migrate skip: flip failed AND re-read failed — leaving both prefixes and the row untouched",
				append(fields, "flip_error", err, "reread_error", gerr)...)
			return false, 0
		}
	}

	// 6. Reclaim local disk.
	deleted, err := store.DeletePrefix(ctx, domain.BackendMinio, prefix)
	if err != nil {
		// Row is already s3 (authoritative + verified); the stale minio objects
		// are benign and the RECONCILE pass will clean them on the next run.
		log.Warnw("row flipped to s3 but minio cleanup failed (benign dual-presence; reconcile next run)", append(fields, "error", err)...)
		return true, srcBytes
	}

	log.Infow("episode migrated minio to s3",
		append(fields, "objects", srcCount, "bytes", srcBytes, "deleted_local", deleted)...)
	return true, srcBytes
}

// undoS3Copy deletes the just-copied s3 prefix after a failed/moot flip so an
// unreferenced copy does not leak (the reconcile pass only revisits rows that
// DID flip to s3 — an unflipped or vanished row's s3 objects would otherwise
// be orphaned forever). Never touches minio. Best-effort: a failed undo is
// loudly logged for manual cleanup — nothing references the prefix either way.
func undoS3Copy(ctx context.Context, log *logger.Logger, store objectStore, prefix string, fields []any, reason string) {
	deleted, err := store.DeletePrefix(ctx, domain.BackendS3, prefix)
	if err != nil {
		log.Errorw("migrate skip: "+reason+"; undo of the s3 copy ALSO failed — orphaned s3 prefix needs manual cleanup",
			append(fields, "error", err)...)
		return
	}
	log.Errorw("migrate skip: "+reason+" — undid the s3 copy",
		append(fields, "s3_deleted", deleted)...)
}

// isNotFound reports whether err is the libs/errors NotFound domain error.
func isNotFound(err error) bool {
	appErr, ok := liberrors.IsAppError(err)
	return ok && appErr.Code == liberrors.CodeNotFound
}

// reconcileLeftovers heals crash-between-flip-and-delete states: for each row
// already flipped to s3, if the local minio prefix still holds objects, re-verify
// the s3 copy matches then delete the minio leftovers. A row whose minio prefix
// is already empty is a no-op (the common, fully-migrated case). Returns
// (cleaned, skipped).
func reconcileLeftovers(
	ctx context.Context,
	log *logger.Logger,
	store objectStore,
	flipped []domain.Episode,
) (int, int) {
	var cleaned, skipped int
	for _, ep := range flipped {
		prefix := ep.MinioPath
		fields := []any{"episode_id", ep.ID, "shikimori_id", ep.ShikimoriID, "episode", ep.EpisodeNumber, "prefix", prefix}

		minioObjs, err := store.List(ctx, domain.BackendMinio, prefix)
		if err != nil {
			log.Errorw("reconcile skip: list minio failed", append(fields, "error", err)...)
			skipped++
			continue
		}
		minioCount, minioBytes := countBytes(minioObjs)
		if minioCount == 0 {
			continue // already clean — the expected steady state
		}

		// Stale local objects present. Only delete them if s3 still verifies.
		s3Objs, err := store.List(ctx, domain.BackendS3, prefix)
		if err != nil {
			log.Errorw("reconcile skip: list s3 failed", append(fields, "error", err)...)
			skipped++
			continue
		}
		s3Count, s3Bytes := countBytes(s3Objs)
		if s3Count != minioCount || s3Bytes != minioBytes {
			log.Errorw("reconcile skip: s3 does not match stale minio leftovers — leaving both",
				append(fields, "minio_count", minioCount, "minio_bytes", minioBytes, "s3_count", s3Count, "s3_bytes", s3Bytes)...)
			skipped++
			continue
		}
		deleted, err := store.DeletePrefix(ctx, domain.BackendMinio, prefix)
		if err != nil {
			log.Errorw("reconcile skip: delete minio leftovers failed", append(fields, "error", err)...)
			skipped++
			continue
		}
		log.Infow("reconciled dual-presence leftover: deleted stale minio objects",
			append(fields, "objects", minioCount, "bytes", minioBytes, "deleted", deleted)...)
		cleaned++
	}
	return cleaned, skipped
}

// printPlan lists the full migration plan (dry-run) with zero writes: every
// selected episode + its source-side object count/bytes, plus any reconcile
// candidates (already-flipped rows whose minio prefix still holds objects).
func printPlan(
	ctx context.Context,
	log *logger.Logger,
	client *storageclient.Client,
	pending, flipped []domain.Episode,
	limit int,
	source string,
) {
	fmt.Printf("=== DRY RUN — storage migration plan (source=%s) ===\n", source)
	fmt.Printf("Selected %d episode(s) on minio awaiting migration", len(pending))
	if limit > 0 {
		fmt.Printf(" (would migrate at most %d this run)", limit)
	}
	fmt.Println(":")

	var planCount int
	var planBytes int64
	for i, ep := range pending {
		objs, err := client.List(ctx, domain.BackendMinio, ep.MinioPath)
		if err != nil {
			fmt.Printf("  [%2d] %s  shikimori=%s ep=%d  <LIST ERROR: %v>\n", i+1, ep.MinioPath, ep.ShikimoriID, ep.EpisodeNumber, err)
			continue
		}
		c, b := countBytes(objs)
		capped := ""
		if limit > 0 && i >= limit {
			capped = "  (beyond -limit; not this run)"
		} else {
			planCount += c
			planBytes += b
		}
		fmt.Printf("  [%2d] %s  shikimori=%s ep=%d  source=%s  objects=%d  bytes=%d%s\n",
			i+1, ep.MinioPath, ep.ShikimoriID, ep.EpisodeNumber, ep.Source, c, b, capped)
	}

	// Reconcile candidates: flipped rows whose minio prefix still has objects.
	var reconcileCandidates int
	for _, ep := range flipped {
		objs, err := client.List(ctx, domain.BackendMinio, ep.MinioPath)
		if err != nil {
			continue
		}
		if c, _ := countBytes(objs); c > 0 {
			reconcileCandidates++
			fmt.Printf("  [reconcile] %s  shikimori=%s ep=%d  stale minio objects=%d (row already s3)\n",
				ep.MinioPath, ep.ShikimoriID, ep.EpisodeNumber, c)
		}
	}

	fmt.Printf("\nPLAN: episodes_selected=%d  would_migrate_objects=%d  would_migrate_bytes=%d  reconcile_candidates=%d\n",
		len(pending), planCount, planBytes, reconcileCandidates)
	fmt.Println("(dry-run: nothing copied, flipped, or deleted)")
	log.Infow("dry-run plan printed",
		"source", source, "episodes_selected", len(pending),
		"would_migrate_bytes", planBytes, "reconcile_candidates", reconcileCandidates)
}

// countBytes totals an object listing's count and byte size.
func countBytes(objs []storageclient.Object) (int, int64) {
	var bytes int64
	for _, o := range objs {
		bytes += o.Size
	}
	return len(objs), bytes
}
