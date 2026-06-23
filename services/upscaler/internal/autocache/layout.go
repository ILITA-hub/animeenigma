// Package autocache holds the upscaler service's MinIO storage layout helpers.
// This mirrors services/library/internal/autocache/layout.go's RawPrefix shape
// but for the UPSCALED variant produced by the upscaler pipeline.
package autocache

import "fmt"

// UpscaledPrefix returns the bucket-relative MinIO prefix (always trailing
// slash) for an upscaled episode under the unified pool layout:
//
//	aeProvider/<shikimoriID>/UPSCALED-<scaleHeight>p/<episode>/
//
// scaleHeight is the target output resolution (e.g. 1080 → "UPSCALED-1080p").
// Callers append "playlist.m3u8" when building a public URL.
func UpscaledPrefix(shikimoriID string, episode, scaleHeight int) string {
	return fmt.Sprintf("aeProvider/%s/UPSCALED-%dp/%d/", shikimoriID, scaleHeight, episode)
}
