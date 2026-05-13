// server_priority.go — sort gogoanime ListServers output by the
// SCRAPER_SERVER_PRIORITY config + validate the list against the
// embeds.Registry's known extractor names.
//
// Phase 21 SCRAPER-HEAL-03. The priority KEY is the embed extractor's
// Name() (closed set of "streamhg" / "earnvids" / "vibeplayer" today),
// derived from the server URL's host via a pre-computed host→name map
// built once at boot from the embeds registry (see main.go).
package gogoanime

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// ValidatePriorityList rejects any entry in priority that is NOT in
// knownNames. Returns nil on success; returns an error LISTING the
// unknown names so the boot log surfaces the typo verbatim.
//
// SCRAPER-HEAL-03 risk mitigation: a SCRAPER_SERVER_PRIORITY env typo
// (e.g. streamg instead of streamhg) must fail-fast at startup, NOT
// silently sort the typo'd name into the trailing "unknown" bucket.
// Documented in CONTEXT.md §risks.
func ValidatePriorityList(priority, knownNames []string) error {
	known := make(map[string]struct{}, len(knownNames))
	for _, n := range knownNames {
		known[strings.ToLower(n)] = struct{}{}
	}
	var unknown []string
	for _, p := range priority {
		if _, ok := known[strings.ToLower(p)]; !ok {
			unknown = append(unknown, p)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("gogoanime: SCRAPER_SERVER_PRIORITY contains unknown server name(s): %s (known: %s)",
			strings.Join(unknown, ", "),
			strings.Join(knownNames, ", "))
	}
	return nil
}

// hostnameToExtractorName maps a server URL's hostname to the canonical
// embed extractor name used as the priority key.
//
// hostExtractor is a pre-built map[host]extractor-name. Callers build it
// ONCE from the embeds registry at boot — see main.go.
//
// Match policy: exact host match first, then suffix match for *.example.com
// style entries. Returns "" when no match.
func hostnameToExtractorName(rawURL string, hostExtractor map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ""
	}
	if name, ok := hostExtractor[host]; ok {
		return name
	}
	// suffix match — for *.example.com style registry hosts
	for suf, name := range hostExtractor {
		if strings.HasSuffix(host, "."+suf) {
			return name
		}
	}
	return ""
}

// SortByPriority reorders servers so entries whose extractor name appears
// earliest in priority come first. Entries whose extractor name is NOT in
// priority trail in their original source-HTML order (stable sort over the
// original-index tiebreaker).
//
// Returns a NEW slice — does not mutate the caller's input.
//
// hostExtractor is the pre-built host→extractor-name map (see
// hostnameToExtractorName).
func SortByPriority(servers []domain.Server, priority []string, hostExtractor map[string]string) []domain.Server {
	priIdx := make(map[string]int, len(priority))
	for i, p := range priority {
		priIdx[strings.ToLower(p)] = i
	}

	type priorityEntry struct {
		s      domain.Server
		pri    int // priority index; len(priority) for unknown
		origIx int
	}
	entries := make([]priorityEntry, len(servers))
	for i, s := range servers {
		name := hostnameToExtractorName(s.ID, hostExtractor)
		p, ok := priIdx[strings.ToLower(name)]
		if !ok {
			p = len(priority)
		}
		entries[i] = priorityEntry{s: s, pri: p, origIx: i}
	}

	// Insertion sort by (pri, origIx). len(entries) is always small in
	// practice (≤ 5 servers per anitaku.to episode page) so O(n²) is
	// effectively O(n) here AND the algorithm is stable by construction —
	// no need for reflect-driven sort.SliceStable.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j-1].pri > entries[j].pri; j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}

	out := make([]domain.Server, len(entries))
	for i, e := range entries {
		out[i] = e.s
	}
	return out
}
