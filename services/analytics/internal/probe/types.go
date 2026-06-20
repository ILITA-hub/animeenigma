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
)

// AnimeSlot labels which of the 4 per-run anime a probe targeted.
type AnimeSlot string

const (
	SlotAnchor          AnimeSlot = "anchor"
	SlotFeatured        AnimeSlot = "featured"
	SlotSpotlightRandom AnimeSlot = "spotlight_random"
	SlotRandom          AnimeSlot = "random"
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
}

func (v Verdict) Playable() bool { return v.Reason == streamprobe.ReasonPlayable }

// ProviderVerdict is the per-provider rollup across its anime/servers.
type ProviderVerdict struct {
	Provider string
	Status   Status
	Reason   string // dominant failure classification with locus, "" when up
}
