// Package domain holds the library service's shared domain models.
//
// The Release type is the canonical normalized shape produced by every
// parser under services/library/internal/parser/* — Nyaa.si, AnimeTosho,
// and any future torrent indexer adapters. It is the input to the merger
// in internal/service/search.go and ultimately the JSON-serialized payload
// returned by GET /api/library/search.
//
// Field naming and tags MUST match the lock in
// .planning/workstreams/raw-jp/phases/02-nyaa-animetosho-search-clients/02-CONTEXT.md
// (the Phase 5 admin UI consumes this struct as-is).
package domain

import "time"

// Release is one torrent entry surfaced by a library search.
//
// InfoHash is the only stable identifier across providers — different
// indexers emit magnet URIs with different tracker / dn parameters for
// the same content, so the merger dedupes on lowercase-hex InfoHash and
// not on Magnet. Source is the lowercase provider name ("nyaa" or
// "animetosho") used by the admin UI to render provenance chips.
//
// MALID is populated only on the AnimeTosho MAL-feed path — Nyaa does
// not expose MAL IDs, so its releases always have MALID == 0. The
// merger uses this field to rank "AnimeTosho hits that match the
// caller's requested MAL ID" to the top of the result slice.
type Release struct {
	Title     string    `json:"title"`
	Magnet    string    `json:"magnet"`
	InfoHash  string    `json:"info_hash"`
	Uploader  string    `json:"uploader,omitempty"`
	Quality   string    `json:"quality,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	Source    string    `json:"source"`
	MALID     int       `json:"mal_id,omitempty"`
	// Seeders is the live swarm-health signal. Only the Jackett provider
	// populates it today (Nyaa/AnimeTosho leave it 0); the Jackett client
	// ranks its results by this field DESC so dead swarms sink, and the
	// admin search UI renders it so an operator never queues a peerless
	// torrent. omitempty so older providers' rows stay byte-for-byte the same.
	Seeders int       `json:"seeders,omitempty"`
	FoundAt time.Time `json:"found_at"`
}
