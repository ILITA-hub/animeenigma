// Package health — Phase 17 scraper liveness probe + cache + sliding window.
//
// CONTRACT: The five stage strings below appear VERBATIM as Prometheus label
// values in queries (`{stage="stream_segment"}`) and in alert rules. Renaming
// any of them breaks dashboards. Treat as a versioned contract.
package health

const (
	StageSearch        = "search"
	StageEpisodes      = "episodes"
	StageServers       = "servers"
	StageStream        = "stream"
	StageStreamSegment = "stream_segment"
)

// AllStages lists the five canonical pipeline stages in execution order.
// The probe iterates this slice top-down; on first failure, subsequent stages
// are NOT exercised (short-circuit — the upstream is broken; running more
// requests is wasted load).
var AllStages = []string{
	StageSearch,
	StageEpisodes,
	StageServers,
	StageStream,
	StageStreamSegment,
}
