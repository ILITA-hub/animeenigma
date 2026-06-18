package metrics

// RosterEntry is one provider row for roster-reflection metric emission. It is the
// minimal projection of a stream_providers row that the management metrics need.
type RosterEntry struct {
	Name        string
	Status      string // "enabled" | "degraded" | "disabled"
	Reason      string
	Description string
}

// EmitProviderRoster reflects a set of provider rows into the management metrics:
//   - provider_info{provider,status,reason,description} = 1 for EVERY entry
//     (roster reflection — disabled rows stay visible in the Grafana table).
//   - provider_enabled{provider} = 1 iff status == "enabled", else 0.
//
// Single-emitter contract: the caller passes ONLY the rows IT owns. Catalog owns
// scraper_operated=false rows; the scraper owns scraper_operated=true rows. The two
// sets partition the roster with no name overlap, so there are no duplicate series
// across Prometheus targets. Call at service boot.
func EmitProviderRoster(entries []RosterEntry) {
	for _, e := range entries {
		enabled := 0.0
		if e.Status == "enabled" {
			enabled = 1.0
		}
		ProviderEnabled.WithLabelValues(e.Name).Set(enabled)
		ProviderInfo.WithLabelValues(e.Name, e.Status, e.Reason, e.Description).Set(1)
	}
}
