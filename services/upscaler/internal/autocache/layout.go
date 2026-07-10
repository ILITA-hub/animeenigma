// Package autocache holds the upscaler service's object-storage layout helpers.
// This mirrors services/library/internal/autocache/layout.go's RawPrefix shape
// but for the UPSCALED variant produced by the upscaler pipeline.
package autocache

import "fmt"

// UpscaledPrefix returns the bucket-relative object prefix (always trailing
// slash) for an upscaled episode under the unified pool layout:
//
//	aeProvider/<shikimoriID>/UPSCALED-<scaleHeight>p/<episode>/
//
// scaleHeight is the target output resolution (e.g. 1080 → "UPSCALED-1080p").
// Callers append "playlist.m3u8" when building a public URL.
//
// Bucket note: output uploads go through the storage service (class
// "upscaled"), so this prefix lands in the service's per-backend bucket —
// `raw-library` on both minio and external s3, matching the library layout
// the future play path will build URLs against — NOT the legacy local
// `upscaler-output` bucket the retired direct-MinIO output path used.
func UpscaledPrefix(shikimoriID string, episode, scaleHeight int) string {
	return fmt.Sprintf("aeProvider/%s/UPSCALED-%dp/%d/", shikimoriID, scaleHeight, episode)
}
