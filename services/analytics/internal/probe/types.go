package probe

import "github.com/ILITA-hub/animeenigma/libs/streamprobe"

// Status is the per-provider rollup verdict shown on the dashboard.
type Status string

const (
	StatusUp       Status = "up"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

func (s Status) Gauge() float64 {
	switch s {
	case StatusUp:
		return 1.0
	case StatusDegraded:
		return 0.5
	default:
		return 0.0
	}
}

// Stage is the furthest pipeline step reached.
type Stage string

const (
	StageSearch   Stage = "search"
	StageEpisodes Stage = "episodes"
	StageServers  Stage = "servers"
	StageStream   Stage = "stream"
	StagePlayback Stage = "playback"
	// StageNotTried marks refs that were skipped because an earlier ref failed
	// and fail_fast was enabled. These are excluded from Rollup scoring.
	StageNotTried Stage = "not_tried"
)

// AnimeSlot labels which of the 4 per-run anime a probe targeted.
type AnimeSlot string

const (
	SlotAnchor          AnimeSlot = "anchor"
	SlotFeatured        AnimeSlot = "featured"
	SlotSpotlightRandom AnimeSlot = "spotlight_random"
	SlotRandom          AnimeSlot = "random"
	// SlotLibraryLatest labels an ae target — one of the newest distinct-anime
	// library uploads (carries its own Episode, unlike the scraper slots).
	SlotLibraryLatest AnimeSlot = "library_latest"
)

// ResolvedStream is one server's catalog-signed stream, ready to validate
// through the HLS proxy. Produced by Resolver.
type ResolvedStream struct {
	Provider  string
	AnimeUUID string
	AnimeName string // human-readable title for the dashboard reason column
	Slot      AnimeSlot
	Server    string // server id/name, e.g. "...type=hd-1..."
	MasterURL string
	Exp       string
	Sig       string
	Referer   string
	Stage     Stage // furthest stage reached when this was produced (StageStream on success)
}

// Verdict is the outcome of validating one ResolvedStream.
type Verdict struct {
	Provider  string
	AnimeUUID string
	AnimeName string
	Slot      AnimeSlot
	Server    string
	Stage     Stage
	Reason    streamprobe.Reason
	// Measurement fields, populated by HTTPValidator on the reached-playback
	// path (zero otherwise). Consumed by the engine to assemble TickMetrics.
	ManifestMs   int64
	SegmentMs    int64
	SegmentBytes int64
	CDNHost      string
	Quality      string
}

func (v Verdict) Playable() bool { return v.Reason == streamprobe.ReasonPlayable }

// ProviderVerdict is the per-provider rollup across its anime/servers.
type ProviderVerdict struct {
	Provider string
	Status   Status
	Reason   string // dominant failure classification with locus, "" when up
}

// TickMetrics is the JSON summary of one probe tick for a provider. Persisted to
// stream_providers.last_tick_metrics and rendered on the Grafana "Last Tick
// Metrics" panel. *Ms are milliseconds; ThroughputKbps is kilobits/sec.
type TickMetrics struct {
	At             string `json:"at"`
	Pass           bool   `json:"pass"`
	Reason         string `json:"reason"`
	ProviderUsed   string `json:"provider_used"`
	Anime          string `json:"anime"`
	Slot           string `json:"slot"`
	SampleSize     int    `json:"sample_size"`
	WarmupMs       int64  `json:"warmup_ms,omitempty"`
	ResolveMs      int64  `json:"resolve_ms"`
	ValidateMs     int64  `json:"validate_ms"`
	ThroughputKbps int64  `json:"throughput_kbps,omitempty"`
	CDNHost        string `json:"cdn_host,omitempty"`
	Quality        string `json:"quality,omitempty"`
}

// tickMeasure is the per-tick measurement probeProvider gathers from the top
// ref; RunOnce finalizes it into a TickMetrics (adding At/Pass/Reason/Warmup).
type tickMeasure struct {
	ResolveMs, ValidateMs, ThroughputKbps int64
	CDNHost, Quality, Anime, Slot         string
	SampleSize                            int
}
