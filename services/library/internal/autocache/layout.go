// Package autocache holds the unified first-party RAW storage layout and
// (in later phases) the accountant / migrator / evictor that meter it.
//
// D1 (locked): all autocache code lives inside services/library — no new
// microservice. This file is the single source of truth for the MinIO object
// layout so every writer, the URL builder, and the Phase-3 path migrator agree
// on exactly one prefix shape.
package autocache

import "fmt"

// RawPrefix returns the bucket-relative MinIO prefix (always trailing slash) for
// a first-party RAW episode under the unified pool layout (spec §3.1):
//
//	aeProvider/<malID>/RAW/<episode>/
//
// malID == shikimori_id (CONTEXT line 42) — the same number, no ID remapping.
// Callers append "playlist.m3u8" when building a public URL, and the trailing
// slash means minio.Writer.Move / Upload accept the prefix unchanged.
//
// D6 (locked): the path is uniform for admin and autocache content alike — the
// library_episodes.source column, NOT the path, distinguishes them.
//
// D2 (locked): only the RAW/ track is written in v1. SUB/ and DUB/ sub-prefixes
// are reserved for a future milestone and are intentionally NOT emitted here.
func RawPrefix(malID string, episode int) string {
	return fmt.Sprintf("aeProvider/%s/RAW/%d/", malID, episode)
}
