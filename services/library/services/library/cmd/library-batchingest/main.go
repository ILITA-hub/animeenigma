// Command library-batchingest is a one-shot operator tool that ingests a
// MULTI-FILE torrent pack (one continuous-numbered season run, e.g.
// EP01..EP48 across 4 seasons) into MinIO as playable HLS episodes.
//
// The library service's normal pipeline is one-job → one-file (the encoder
// picks only the single largest file in a download). A season pack is many
// files spanning several catalog entries (each season is its own
// shikimori_id), so it cannot be wired through a single job. This tool
// reuses the EXACT same components the encoder worker uses — the ffmpeg
// Transcoder, the MinIO Writer, and the EpisodeRepository — but loops over
// every file in a directory and maps each to its (shikimori_id, episode)
// by arithmetic on the episode number parsed from the filename:
//
//	N        = the integer captured by -pattern (1-based, continuous)
//	season   = (N-1)/epsPerSeason            (0-based index into -shikimori)
//	episode  = (N-1)%epsPerSeason + 1        (1-based within the season)
//
// It runs INSIDE the library container (which has ffmpeg + MinIO + DB
// reachability) via `docker compose run --rm --entrypoint
// /app/library-batchingest library ...`. It never touches the running
// service or its single-episode pipeline.
//
// Idempotent: an episode whose (shikimori_id, episode_number) row already
// exists is skipped (no wasted transcode) unless -force is given, in which
// case the MinIO objects are re-written and the existing row is kept.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/config"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/ffmpeg"
	lminio "github.com/ILITA-hub/animeenigma/services/library/internal/minio"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
)

// noopInvalMetrics satisfies service.InvalidationMetrics without pulling in
// the Prometheus collectors (this is a short-lived CLI, not the service).
type noopInvalMetrics struct{}

func (noopInvalMetrics) IncCacheInvalidation(string) {}

// fileJob is one resolved mapping from a source file to its target.
type fileJob struct {
	path        string
	n           int // 1-based continuous episode number from the filename
	shikimoriID string
	episode     int // 1-based within the season
}

func main() {
	var (
		dir          = flag.String("dir", "", "directory containing the pack's video files (required)")
		pattern      = flag.String("pattern", `EP(\d{1,3})`, "regex with ONE capture group for the continuous episode number")
		epsPerSeason = flag.Int("eps-per-season", 12, "episodes per season (for season/episode split)")
		shikimoriCSV = flag.String("shikimori", "", "comma-separated shikimori IDs ordered by season (required), e.g. 54974,55821,58555,59385")
		exts         = flag.String("exts", ".mkv,.mp4,.webm,.avi", "comma-separated video extensions to consider")
		only         = flag.String("only", "", "optional inclusive N range to limit the run, e.g. 1-12 (default: all)")
		concurrency  = flag.Int("concurrency", 1, "number of files to transcode in parallel (CPU-bound; keep low)")
		force        = flag.Bool("force", false, "re-encode + re-upload even if the episode row already exists")
		dryRun       = flag.Bool("dry-run", false, "print the file→(shikimori,episode) mapping and exit without encoding")
	)
	flag.Parse()

	log := logger.Default()
	defer func() { _ = log.Sync() }()

	if *dir == "" || *shikimoriCSV == "" {
		log.Errorw("missing required flags", "dir", *dir, "shikimori", *shikimoriCSV)
		flag.Usage()
		os.Exit(2)
	}

	seasonIDs := splitCSV(*shikimoriCSV)
	if len(seasonIDs) == 0 {
		log.Fatalw("no shikimori IDs parsed from -shikimori")
	}
	re, err := regexp.Compile(*pattern)
	if err != nil {
		log.Fatalw("bad -pattern regex", "pattern", *pattern, "error", err)
	}
	if re.NumSubexp() < 1 {
		log.Fatalw("-pattern must contain exactly one capture group for the episode number", "pattern", *pattern)
	}
	lowN, highN := parseRange(*only)
	extSet := map[string]bool{}
	for _, e := range splitCSV(*exts) {
		extSet[strings.ToLower(strings.TrimSpace(e))] = true
	}

	// Resolve the file→target mapping.
	jobs, err := resolveJobs(*dir, re, extSet, *epsPerSeason, seasonIDs, lowN, highN)
	if err != nil {
		log.Fatalw("resolve jobs failed", "error", err)
	}
	if len(jobs) == 0 {
		log.Warnw("no matching files found", "dir", *dir, "pattern", *pattern)
		return
	}

	fmt.Printf("Resolved %d file(s):\n", len(jobs))
	for _, j := range jobs {
		fmt.Printf("  EP%02d → shikimori %s episode %d   %s\n", j.n, j.shikimoriID, j.episode, filepath.Base(j.path))
	}
	if *dryRun {
		fmt.Println("(dry-run: nothing encoded)")
		return
	}

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

	writer, err := lminio.New(lminio.Config{
		Endpoint:          cfg.Minio.Endpoint,
		AccessKey:         cfg.Minio.AccessKey,
		SecretKey:         cfg.Minio.SecretKey,
		Bucket:            cfg.Minio.Bucket,
		UseSSL:            cfg.Minio.UseSSL,
		UploadConcurrency: cfg.Minio.UploadConcurrency,
	}, log)
	if err != nil {
		log.Fatalw("minio init", "error", err)
	}
	if err := writer.EnsureBucket(ctx); err != nil {
		log.Fatalw("minio ensure bucket", "error", err)
	}

	transcoder := ffmpeg.NewTranscoder(ffmpeg.Config{
		BinaryPath:     cfg.Encode.FfmpegBin,
		FfprobePath:    cfg.Encode.FfprobeBin,
		Tmpdir:         cfg.Encode.Tmpdir,
		MaxBitrateKbps: cfg.Encode.MaxBitrateKbps,
	}, log)

	invalidator := service.NewCatalogInvalidator(service.InvalidatorConfig{
		CatalogInternalAPIURL: cfg.CatalogInternal.APIURL,
		Timeout:               cfg.CatalogInternal.Timeout,
	}, noopInvalMetrics{}, log)

	// Process files with a bounded worker pool. A single file's failure is
	// recorded and does not abort the run.
	var (
		doneCnt, skipCnt, failCnt int
		touchedShiki              = map[string]bool{}
	)
	sem := make(chan struct{}, max1(*concurrency))
	results := make(chan string, len(jobs))
	type outcome struct {
		kind  string // "done" | "skip" | "fail"
		shiki string
		msg   string
	}
	outc := make(chan outcome, len(jobs))

	for _, j := range jobs {
		j := j
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			res := ingestOne(ctx, log, transcoder, writer, episodeRepo, cfg.Minio.Bucket, j, *force)
			outc <- res
			results <- res.kind
		}()
	}
	// Drain.
	for range jobs {
		o := <-outc
		switch o.kind {
		case "done":
			doneCnt++
			touchedShiki[o.shiki] = true
		case "skip":
			skipCnt++
		default:
			failCnt++
			log.Errorw("file ingest failed", "detail", o.msg)
		}
	}

	// Bust the catalog raw-source cache once per touched anime so `ae` lights up.
	for shiki := range touchedShiki {
		invalidator.Invalidate(ctx, shiki)
	}

	log.Infow("batch ingest complete", "done", doneCnt, "skipped", skipCnt, "failed", failCnt, "total", len(jobs))
	fmt.Printf("\nDONE=%d SKIPPED=%d FAILED=%d TOTAL=%d\n", doneCnt, skipCnt, failCnt, len(jobs))
	if failCnt > 0 {
		os.Exit(1)
	}
}

// ingestOne mirrors EncoderPool.processJob steps 5–9 for a single file.
func ingestOne(
	ctx context.Context,
	log *logger.Logger,
	transcoder *ffmpeg.Transcoder,
	writer *lminio.Writer,
	episodeRepo *repo.EpisodeRepository,
	bucket string,
	j fileJob,
	force bool,
) (out struct {
	kind  string
	shiki string
	msg   string
}) {
	out.shiki = j.shikimoriID
	prefix := fmt.Sprintf("%s/%d/", j.shikimoriID, j.episode)

	// Idempotency: skip before the expensive transcode unless -force.
	existing, err := episodeRepo.GetByShikimoriEpisode(ctx, j.shikimoriID, j.episode)
	exists := err == nil && existing != nil
	if exists && !force {
		log.Infow("episode already exists; skipping", "shikimori_id", j.shikimoriID, "episode", j.episode)
		out.kind = "skip"
		return
	}

	log.Infow("transcoding", "file", filepath.Base(j.path), "shikimori_id", j.shikimoriID, "episode", j.episode)
	result, err := transcoder.Transcode(ctx, j.path)
	if err != nil {
		out.kind, out.msg = "fail", fmt.Sprintf("transcode %s: %v", filepath.Base(j.path), err)
		return
	}
	defer func() {
		if result.PlaylistPath != "" {
			_ = os.RemoveAll(filepath.Dir(result.PlaylistPath))
		}
	}()

	files := append([]string{}, result.SegmentPaths...)
	files = append(files, result.PlaylistPath)
	bytes, err := writer.Upload(ctx, prefix, files)
	if err != nil {
		out.kind, out.msg = "fail", fmt.Sprintf("upload %s: %v", prefix, err)
		return
	}

	if !exists {
		dur := result.DurationSec
		size := result.SizeBytes
		ep := &domain.Episode{
			ShikimoriID:   j.shikimoriID,
			EpisodeNumber: j.episode,
			MinioPath:     prefix,
			DurationSec:   &dur,
			SizeBytes:     &size,
		}
		if err := episodeRepo.Create(ctx, ep); err != nil {
			if appErr, ok := liberrors.IsAppError(err); ok && appErr.Code == liberrors.CodeAlreadyExists {
				log.Warnw("episode row appeared concurrently; MinIO objects refreshed",
					"shikimori_id", j.shikimoriID, "episode", j.episode)
			} else {
				out.kind, out.msg = "fail", fmt.Sprintf("create episode row %s/%d: %v", j.shikimoriID, j.episode, err)
				return
			}
		}
	}

	log.Infow("episode ingested",
		"shikimori_id", j.shikimoriID, "episode", j.episode,
		"minio_prefix", prefix, "duration_sec", result.DurationSec,
		"size_bytes", result.SizeBytes, "upload_bytes", bytes, "forced", force)
	out.kind = "done"
	return
}

// resolveJobs walks dir (non-recursive + one level) and maps each matching
// video file to its target via the continuous-numbering arithmetic.
func resolveJobs(dir string, re *regexp.Regexp, extSet map[string]bool, epsPerSeason int, seasonIDs []string, lowN, highN int) ([]fileJob, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var jobs []fileJob
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !extSet[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		m := re.FindStringSubmatch(name)
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			continue
		}
		if lowN > 0 && n < lowN {
			continue
		}
		if highN > 0 && n > highN {
			continue
		}
		seasonIdx := (n - 1) / epsPerSeason
		if seasonIdx >= len(seasonIDs) {
			// File beyond the provided season list — skip rather than misfile.
			continue
		}
		jobs = append(jobs, fileJob{
			path:        filepath.Join(dir, name),
			n:           n,
			shikimoriID: seasonIDs[seasonIdx],
			episode:     (n-1)%epsPerSeason + 1,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].n < jobs[j].n })
	return jobs, nil
}

func splitCSV(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseRange parses "lo-hi" (or "" → 0,0 = unbounded) into inclusive bounds.
func parseRange(s string) (int, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0
	}
	parts := strings.SplitN(s, "-", 2)
	lo, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	if len(parts) == 1 {
		return lo, lo
	}
	hi, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return lo, hi
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
