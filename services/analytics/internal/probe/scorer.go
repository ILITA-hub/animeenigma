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

func Rollup(provider string, verdicts []Verdict) ProviderVerdict {
	pv := ProviderVerdict{Provider: provider, Status: StatusDown}
	if len(verdicts) == 0 {
		return pv
	}
	resolved := false
	counts := map[streamprobe.Reason]int{}
	firstServer := map[streamprobe.Reason]string{}
	for _, v := range verdicts {
		if v.Playable() {
			pv.Status = StatusUp
			return pv
		}
		if v.Stage == StagePlayback {
			resolved = true
		}
		counts[v.Reason]++
		if _, ok := firstServer[v.Reason]; !ok {
			firstServer[v.Reason] = v.Server
		}
	}
	if resolved {
		pv.Status = StatusDegraded
	}
	// dominant reason
	var domR streamprobe.Reason
	best := -1
	for r, c := range counts {
		if c > best {
			best, domR = c, r
		}
	}
	pv.Reason = string(domR) + " on " + serverShortLabel(firstServer[domR])
	return pv
}
