// Package storagegw is the library service's single adapter over
// libs/storageclient. It binds the configured upload concurrency once and
// exposes exactly the method shapes the library's internal consumers (encoder
// pool, evictor, storyboard backfill, jobs Link handler, episodes handler,
// legacy-admin migrator, batchingest CLI) declare as their local interface
// seams — so main.go injects ONE *Gateway everywhere the deleted
// internal/minio.Writer used to be injected.
//
// It holds NO object-store credentials: all placement + presigning lives behind
// the storage service (services/storage/, Docker-network-only). Every call is a
// thin forward to the client, which unwraps the {success,data,error} envelope.
package storagegw

import (
	"context"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// Gateway wraps a *storageclient.Client + the upload concurrency the encoder /
// batchingest use for the concurrent-segment PUT fan-out.
type Gateway struct {
	client      *storageclient.Client
	concurrency int
}

// New constructs a Gateway. concurrency <= 0 lets the client fall back to its
// own default (8).
func New(client *storageclient.Client, concurrency int) *Gateway {
	return &Gateway{client: client, concurrency: concurrency}
}

// Upload is the encoder / batchingest write path: class + override resolve the
// destination backend (only ClassLibraryManual honors override), and the
// resolved backend id is returned so the caller records where the files landed.
// Segments upload concurrently; playlist.m3u8 uploads LAST (client ordering).
func (g *Gateway) Upload(ctx context.Context, class, override, prefix string, files []string) (string, error) {
	return g.client.UploadFiles(ctx, class, override, prefix, files, g.concurrency)
}

// UploadStoryboard pushes the sprite sheets + VTT to the SAME backend the
// episode's HLS lives on. It is a ClassLibraryManual upload with override set to
// the known backend id (manual honors override), so an s3 episode's sprites land
// on s3 and a minio episode's on minio. Consumed best-effort by both the encoder
// (which passes the storage its own Upload just resolved) and the storyboard
// backfill (which passes the episode row's Storage).
func (g *Gateway) UploadStoryboard(ctx context.Context, storage, prefix string, sheetPaths []string, vttPath string) error {
	files := make([]string, 0, len(sheetPaths)+1)
	files = append(files, sheetPaths...)
	files = append(files, vttPath)
	_, err := g.client.UploadFiles(ctx, domain.ClassLibraryManual, storage, prefix, files, g.concurrency)
	return err
}

// DownloadPrefix streams every object under prefix (on backend `storage`) into
// destDir — the read side of the storyboard backfill's local ffmpeg input.
func (g *Gateway) DownloadPrefix(ctx context.Context, storage, prefix, destDir string) error {
	return g.client.DownloadPrefix(ctx, storage, prefix, destDir)
}

// DeletePrefix hard-deletes every object under prefix on backend `storage` — the
// evictor's object half. The client's deleted-count is dropped; the evictor only
// needs success/failure.
func (g *Gateway) DeletePrefix(ctx context.Context, storage, prefix string) error {
	_, err := g.client.DeletePrefix(ctx, storage, prefix)
	return err
}

// Move server-side-relocates every object under srcPrefix to dstPrefix WITHIN
// backend `storage` — the legacy-admin migrator (fixed BackendMinio) and the
// jobs Link handler (the job's resolved storage) both use it.
func (g *Gateway) Move(ctx context.Context, storage, srcPrefix, dstPrefix string) error {
	return g.client.Move(ctx, storage, srcPrefix, dstPrefix)
}

// List returns the bucket-relative objects (key + size) under prefix on backend
// `storage` — the jobs Link handler parses the pending episode number from the
// first key.
func (g *Gateway) List(ctx context.Context, storage, prefix string) ([]storageclient.Object, error) {
	return g.client.List(ctx, storage, prefix)
}

// DownloadURL returns a presigned GET URL for exactly `key` on backend
// `storage` — the file manager's download handler (Task 5) fetches this
// server-side and streams the bytes to the admin (MinIO's presigned host is
// internal-only, so the browser can't fetch it directly). Reuses the
// storage service's existing /download-urls endpoint via DownloadURLs.
func (g *Gateway) DownloadURL(ctx context.Context, storage, key string) (string, error) {
	urls, err := g.client.DownloadURLs(ctx, storage, key)
	if err != nil {
		return "", err
	}
	for _, u := range urls {
		// download-urls returns Name relative to the requested prefix. For a
		// single-key prefix this is either "" (Name == prefix, i.e. an exact
		// object match — "" is trivially a suffix of key) or a basename-style
		// tail, so a suffix match covers both shapes.
		if key == u.Name || strings.HasSuffix(key, u.Name) {
			return u.URL, nil
		}
	}
	if len(urls) == 1 {
		return urls[0].URL, nil
	}
	return "", errors.NotFound("object")
}

// URLFor returns the public-facing playlist/segment URL the streaming proxy
// fronts: the backend's base URL + "/" + path. Rides the client's BaseURLs
// cache, so the episodes handler's per-row calls don't each round-trip.
func (g *Gateway) URLFor(ctx context.Context, storage, path string) (string, error) {
	return g.client.URLFor(ctx, storage, path)
}
