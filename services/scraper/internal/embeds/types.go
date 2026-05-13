// types.go — small shared interfaces / contracts for the embeds package.
//
// Phase 21 SCRAPER-HEAL-03: HostingExtractor is the optional surface every
// URL-host-bound embed extractor implements. main.go iterates the
// extractor set to build a host→Name() map used by gogoanime's
// SortByPriority + cold-path metric labels.
package embeds

// HostingExtractor is the optional surface every URL-host-bound embed
// extractor implements. Hosts() returns the lowercase host list this
// extractor matches (host equality OR strict subdomain — same as Matches).
//
// Implementations: VibePlayerExtractor, StreamHGExtractor, EarnvidsExtractor
// (Phase 18). KwikExtractor and the megacloud client do NOT currently
// implement this — they are not part of the gogoanime priority chain.
type HostingExtractor interface {
	Name() string
	Hosts() []string
}
