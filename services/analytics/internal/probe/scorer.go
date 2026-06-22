package probe

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func serverShortLabel(server string) string {
	switch {
	case strings.Contains(server, "type=hd-1"):
		return "HD-1"
	case strings.Contains(server, "type=hd-2"):
		return "HD-2"
	}
	if i := strings.Index(server, "//"); i >= 0 {
		rest := server[i+2:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return server
}

// Rollup scores a provider by the fraction of distinct anime slots that played
// (a slot passes iff ANY of its verdicts is playable): >50% Up, >0% Degraded,
// 0% Down. Deterministic dominant-reason label for Degraded/Down.
func Rollup(provider string, verdicts []Verdict) ProviderVerdict {
	pv := ProviderVerdict{Provider: provider, Status: StatusDown}
	if len(verdicts) == 0 {
		return pv
	}
	slotPass := map[AnimeSlot]bool{}
	slotSeen := map[AnimeSlot]bool{}
	counts := map[streamprobe.Reason]int{}
	firstServer := map[streamprobe.Reason]string{}
	for _, vd := range verdicts {
		slotSeen[vd.Slot] = true
		if vd.Playable() {
			slotPass[vd.Slot] = true
			continue
		}
		counts[vd.Reason]++
		if _, ok := firstServer[vd.Reason]; !ok {
			firstServer[vd.Reason] = vd.Server
		}
	}
	pass := 0
	for s := range slotSeen {
		if slotPass[s] {
			pass++
		}
	}
	ratio := float64(pass) / float64(len(slotSeen))
	switch {
	case ratio > 0.5:
		pv.Status = StatusUp
		return pv
	case ratio > 0:
		pv.Status = StatusDegraded
	default:
		pv.Status = StatusDown
	}
	var domR streamprobe.Reason
	best := -1
	for r, c := range counts {
		if c > best || (c == best && string(r) < string(domR)) {
			best, domR = c, r
		}
	}
	if best >= 0 {
		pv.Reason = string(domR) + " on " + serverShortLabel(firstServer[domR])
	}
	return pv
}
