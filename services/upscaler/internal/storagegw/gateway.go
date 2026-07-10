// Package storagegw is the upscaler service's single adapter over
// libs/storageclient — the same thin-adapter pattern as
// services/library/internal/storagegw. It carries the upscaler's ONE piece of
// storage-service knowledge: finalized HLS output is user content of class
// "upscaled", whose destination backend the storage service resolves
// (STORAGE_CLASS_UPSCALED, external s3 in prod).
//
// Note the bucket change this implies: the legacy direct-MinIO writer put HLS
// output in the local `upscaler-output` bucket; via the storage service the
// SAME `aeProvider/<shikimoriID>/UPSCALED-<h>p/<episode>/` prefix now lands in
// the service's per-backend bucket (`raw-library` on both backends) — the
// identical-layout principle that lets the future play path build
// library-style URLs.
//
// It holds NO object-store credentials: placement + presigning live behind the
// storage service (services/storage/, Docker-network-only). Internal artifacts
// (model-weight tars, per-job log dumps) are NOT user content and deliberately
// stay on the direct internal/minio writer.
package storagegw

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/storageclient"
)

// ClassUpscaled is the storage-service placement class for finalized upscaled
// HLS output. Mirrors services/storage/internal/domain.ClassUpscaled (the
// storage service is a separate module; the string is the wire contract).
const ClassUpscaled = "upscaled"

// Gateway wraps a *storageclient.Client + the upload concurrency used for the
// concurrent-segment PUT fan-out (playlist.m3u8 always uploads last — client
// ordering).
type Gateway struct {
	client      *storageclient.Client
	concurrency int
}

// New constructs a Gateway. concurrency <= 0 lets the client fall back to its
// own default (8).
func New(client *storageclient.Client, concurrency int) *Gateway {
	return &Gateway{client: client, concurrency: concurrency}
}

// Upload is the orchestrator's finalize write path: every file in filePaths
// PUTs to {prefix}{basename} on the backend the storage service resolves for
// ClassUpscaled (no override — upscaled has a fixed placement). Returns the
// resolved backend id so the caller records where the output landed.
func (g *Gateway) Upload(ctx context.Context, prefix string, filePaths []string) (string, error) {
	return g.client.UploadFiles(ctx, ClassUpscaled, "", prefix, filePaths, g.concurrency)
}
