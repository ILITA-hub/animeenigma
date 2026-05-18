// Package minio is the library service's HLS uploader. Wraps
// github.com/minio/minio-go/v7 with:
//
//   - Idempotent bucket bootstrap (`raw-library` is created at service
//     start; BucketAlreadyOwnedByYou is swallowed).
//   - Concurrent segment upload + LAST playlist upload so HLS clients
//     never see a playlist referring to an unfinished segment.
//   - Content-Type per extension (`application/vnd.apple.mpegurl`
//     for .m3u8, `video/mp2t` for .ts).
//   - URLFor helper that returns the internal `http://endpoint/bucket/path`
//     URL the streaming proxy fronts (bucket ACL is server-side-only —
//     no public exposure direct from MinIO).
package minio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

// Config holds connection + bucket knobs.
type Config struct {
	Endpoint          string
	AccessKey         string
	SecretKey         string
	Bucket            string
	UseSSL            bool
	UploadConcurrency int // default 8 when <= 0
}

// Uploader is the slice of *minio.Client the Writer uses. Pulled out
// so unit tests can swap in a fake without spinning up a real MinIO.
// PutObject's `size` param is int64 to match the SDK; callers pass
// the result of os.Stat.Size().
//
// Phase 5 adds ListObjects / CopyObject / RemoveObject for the
// post-hoc shikimori-link Move() helper (server-side copy from
// pending/{job_id}/{ep}/ to {shikimori_id}/{ep}/).
type Uploader interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error
	PutObject(ctx context.Context, bucket, object string, reader interface {
		Read(p []byte) (int, error)
	}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	CopyObject(ctx context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error)
	RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error
}

// minioClientAdapter wraps the real *minio.Client to satisfy
// Uploader. The SDK's PutObject takes an `io.Reader`; we forward the
// caller-provided reader.
type minioClientAdapter struct {
	c *minio.Client
}

func (a *minioClientAdapter) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return a.c.BucketExists(ctx, bucket)
}

func (a *minioClientAdapter) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	return a.c.MakeBucket(ctx, bucket, opts)
}

func (a *minioClientAdapter) PutObject(ctx context.Context, bucket, object string, reader interface {
	Read(p []byte) (int, error)
}, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	// minio-go expects io.Reader; our type matches it structurally.
	return a.c.PutObject(ctx, bucket, object, readerAdapter{r: reader}, size, opts)
}

func (a *minioClientAdapter) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return a.c.ListObjects(ctx, bucket, opts)
}

func (a *minioClientAdapter) CopyObject(ctx context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error) {
	return a.c.CopyObject(ctx, dst, src)
}

func (a *minioClientAdapter) RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error {
	return a.c.RemoveObject(ctx, bucket, object, opts)
}

// readerAdapter promotes an "interface{ Read }" to io.Reader for the SDK.
type readerAdapter struct {
	r interface {
		Read(p []byte) (int, error)
	}
}

func (a readerAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }

// Writer is the public façade.
type Writer struct {
	cfg      Config
	uploader Uploader
	log      *logger.Logger
}

// New constructs a Writer backed by a real *minio.Client.
func New(cfg Config, log *logger.Logger) (*Writer, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio endpoint required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "raw-library"
	}
	if cfg.UploadConcurrency <= 0 {
		cfg.UploadConcurrency = 8
	}
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}
	return &Writer{cfg: cfg, uploader: &minioClientAdapter{c: c}, log: log}, nil
}

// newWriterWithUploader is the test seam — bypasses the real
// minio.Client construction so unit tests inject a fake Uploader.
func newWriterWithUploader(cfg Config, u Uploader, log *logger.Logger) *Writer {
	if cfg.Bucket == "" {
		cfg.Bucket = "raw-library"
	}
	if cfg.UploadConcurrency <= 0 {
		cfg.UploadConcurrency = 8
	}
	return &Writer{cfg: cfg, uploader: u, log: log}
}

// EnsureBucket creates cfg.Bucket if absent. Idempotent: swallows
// BucketAlreadyOwnedByYou / BucketAlreadyExists ErrorResponse codes
// so two library instances starting concurrently can both succeed.
func (w *Writer) EnsureBucket(ctx context.Context) error {
	exists, err := w.uploader.BucketExists(ctx, w.cfg.Bucket)
	if err == nil && exists {
		return nil
	}
	if err != nil && !isBucketRaceError(err) {
		// BucketExists shouldn't normally fail without permissions —
		// but if MinIO is mid-startup it can; we fall through to
		// MakeBucket and rely on the idempotent code path below.
		if w.log != nil {
			w.log.Warnw("minio bucket exists probe failed; attempting MakeBucket",
				"bucket", w.cfg.Bucket, "error", err)
		}
	}
	mkErr := w.uploader.MakeBucket(ctx, w.cfg.Bucket, minio.MakeBucketOptions{})
	if mkErr == nil {
		return nil
	}
	if isBucketRaceError(mkErr) {
		return nil
	}
	return fmt.Errorf("minio make bucket: %w", mkErr)
}

// isBucketRaceError reports whether the err is one of the "bucket
// already exists" responses we want to swallow.
func isBucketRaceError(err error) bool {
	if err == nil {
		return false
	}
	var er minio.ErrorResponse
	if errors.As(err, &er) {
		return er.Code == "BucketAlreadyOwnedByYou" || er.Code == "BucketAlreadyExists"
	}
	// Fall back to message-substring match — older SDK versions return
	// a plain error in some code paths.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "bucketalreadyownedbyyou") ||
		strings.Contains(msg, "bucketalreadyexists")
}

// contentTypeFor maps a filename extension to its Content-Type.
func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	default:
		return "application/octet-stream"
	}
}

// Upload PUTs every file in filePaths to {bucket}/{prefix}/{basename}.
//
// Order semantics (LOCKED):
//   1. Segments upload concurrently via errgroup with SetLimit(cfg.UploadConcurrency).
//   2. The playlist (basename == "playlist.m3u8") uploads LAST on the
//      main goroutine — never in parallel with segments. This
//      guarantees HLS clients never see a playlist referring to a
//      not-yet-uploaded segment.
//   3. On any segment error, the playlist is never uploaded.
//
// Returns total bytes PUT to MinIO (sum of os.Stat sizes for files
// successfully uploaded). The encoder worker uses this for
// library_upload_bytes_total.
func (w *Writer) Upload(ctx context.Context, prefix string, filePaths []string) (int64, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	var playlist string
	var segments []string
	for _, p := range filePaths {
		if filepath.Base(p) == "playlist.m3u8" {
			playlist = p
			continue
		}
		segments = append(segments, p)
	}

	mu := newByteCounter()

	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(w.cfg.UploadConcurrency)
	for _, seg := range segments {
		seg := seg
		eg.Go(func() error {
			n, err := w.putFile(gctx, prefix, seg)
			if err != nil {
				return err
			}
			mu.add(n)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return mu.value(), err
	}

	// Now the playlist — main goroutine, after every segment is done.
	if playlist != "" {
		n, err := w.putFile(ctx, prefix, playlist)
		if err != nil {
			return mu.value(), err
		}
		mu.add(n)
	}
	return mu.value(), nil
}

// putFile PUTs a single file at {bucket}/{prefix}{basename}.
func (w *Writer) putFile(ctx context.Context, prefix, path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	object := prefix + filepath.Base(path)
	_, err = w.uploader.PutObject(ctx, w.cfg.Bucket, object, f, st.Size(), minio.PutObjectOptions{
		ContentType: contentTypeFor(path),
	})
	if err != nil {
		return 0, fmt.Errorf("put %s: %w", object, err)
	}
	return st.Size(), nil
}

// byteCounter is a simple mutex-protected int64 accumulator used to
// total uploaded bytes across the concurrent segment goroutines.
type byteCounter struct {
	mu sync.Mutex
	n  int64
}

func newByteCounter() *byteCounter { return &byteCounter{} }

func (c *byteCounter) add(n int64) {
	c.mu.Lock()
	c.n += n
	c.mu.Unlock()
}

func (c *byteCounter) value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// ListObjectsByPrefix drains the channel returned by ListObjects for
// every key under prefix (recursive) and returns a sorted []string.
// Used by the Phase-5 Link handler to parse the existing episode
// number out of the pending/{job_id}/{ep}/ path before issuing the
// Move() call.
func (w *Writer) ListObjectsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	ch := w.uploader.ListObjects(ctx, w.cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	var keys []string
	for obj := range ch {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	// Deterministic order — callers may grep for patterns.
	sortStrings(keys)
	return keys, nil
}

// Move performs a server-side copy of every object under srcPrefix to
// dstPrefix (preserving the relative tail), THEN removes the originals.
// Semantics:
//   - srcPrefix and dstPrefix are normalized to end with "/".
//   - On any CopyObject error: abort, do NOT remove sources → no data loss.
//   - After all copies succeed: RemoveObject each source. Remove errors
//     are logged but do NOT propagate (orphan source is recoverable;
//     data integrity is preserved).
//   - Empty src → returns an error (no objects to move).
//
// Phase-5 Link handler calls this AFTER validating the job row exists
// + is `done` + has no shikimori_id yet.
func (w *Writer) Move(ctx context.Context, srcPrefix, dstPrefix string) error {
	if !strings.HasSuffix(srcPrefix, "/") {
		srcPrefix = srcPrefix + "/"
	}
	if !strings.HasSuffix(dstPrefix, "/") {
		dstPrefix = dstPrefix + "/"
	}
	keys, err := w.ListObjectsByPrefix(ctx, srcPrefix)
	if err != nil {
		return fmt.Errorf("move list: %w", err)
	}
	if len(keys) == 0 {
		return fmt.Errorf("move: no objects under %s", srcPrefix)
	}

	// Step 1 — copy every object. Abort on first error; sources stay intact.
	for _, srcKey := range keys {
		tail := strings.TrimPrefix(srcKey, srcPrefix)
		dstKey := dstPrefix + tail
		_, copyErr := w.uploader.CopyObject(ctx,
			minio.CopyDestOptions{Bucket: w.cfg.Bucket, Object: dstKey},
			minio.CopySrcOptions{Bucket: w.cfg.Bucket, Object: srcKey},
		)
		if copyErr != nil {
			return fmt.Errorf("move copy %s → %s: %w", srcKey, dstKey, copyErr)
		}
	}

	// Step 2 — remove sources. Soft-fail: log + continue.
	for _, srcKey := range keys {
		if rmErr := w.uploader.RemoveObject(ctx, w.cfg.Bucket, srcKey, minio.RemoveObjectOptions{}); rmErr != nil {
			if w.log != nil {
				w.log.Warnw("move: failed to remove source object (data preserved at dst, source orphan)",
					"src", srcKey, "error", rmErr)
			}
		}
	}
	return nil
}

// sortStrings is a tiny in-place ascending sort used by
// ListObjectsByPrefix. Avoids importing "sort" twice in the file.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// URLFor returns the internal MinIO HTTP URL for a bucket-relative
// path. NOT a presigned URL — the bucket ACL is server-side-only and
// the streaming proxy fronts public access.
func (w *Writer) URLFor(path string) string {
	scheme := "http"
	if w.cfg.UseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, w.cfg.Endpoint, w.cfg.Bucket, path)
}
