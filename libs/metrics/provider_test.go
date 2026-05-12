package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestProviderHealthUp_NameAndLabels asserts the gauge family exists with
// the canonical name and label set. The gauge family is the public contract
// referenced by Grafana dashboards and alert rules — renaming any of these
// strings breaks the dashboard.
func TestProviderHealthUp_NameAndLabels(t *testing.T) {
	ProviderHealthUp.Reset()
	ProviderHealthUp.WithLabelValues("test", "search").Set(1)

	got := testutil.ToFloat64(ProviderHealthUp.WithLabelValues("test", "search"))
	if got != 1.0 {
		t.Fatalf("ProviderHealthUp{provider=test, stage=search} = %v; want 1.0", got)
	}

	name, labels := descMeta(t, ProviderHealthUp)
	if name != "provider_health_up" {
		t.Errorf("metric name = %q; want %q", name, "provider_health_up")
	}
	wantLabels := map[string]bool{"provider": true, "stage": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// TestProviderProbeLastTick_NameAndLabels confirms the heartbeat gauge name
// and single-label cardinality.
func TestProviderProbeLastTick_NameAndLabels(t *testing.T) {
	ProviderProbeLastTick.Reset()
	ProviderProbeLastTick.WithLabelValues("test").SetToCurrentTime()

	name, labels := descMeta(t, ProviderProbeLastTick)
	if name != "provider_probe_last_tick_timestamp" {
		t.Errorf("metric name = %q; want %q", name, "provider_probe_last_tick_timestamp")
	}
	wantLabels := map[string]bool{"provider": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// TestParserZeroMatchTotal_IncrementsCorrectly verifies the counter name,
// labels, and that .Inc() bumps the value (this is the SCRAPER-NF-04 metric
// that was missing before Phase 17).
func TestParserZeroMatchTotal_IncrementsCorrectly(t *testing.T) {
	ParserZeroMatchTotal.Reset()
	before := testutil.ToFloat64(ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))
	ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item").Inc()
	after := testutil.ToFloat64(ParserZeroMatchTotal.WithLabelValues("animepahe", "episode_list_item"))
	if d := after - before; d != 1.0 {
		t.Fatalf("ParserZeroMatchTotal delta = %v; want 1.0", d)
	}

	name, labels := descMeta(t, ParserZeroMatchTotal)
	if name != "parser_zero_match_total" {
		t.Errorf("metric name = %q; want %q", name, "parser_zero_match_total")
	}
	wantLabels := map[string]bool{"provider": true, "selector": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// --- helpers ----------------------------------------------------------------

// descMeta extracts (FQName, labelNames) from any collector via Describe().
// Prometheus exposes the descriptor string in the form:
//
//	Desc{fqName: "metric_name", help: "...", constLabels: {}, variableLabels: [a b]}
//
// We parse out fqName and variableLabels from that string. The format is
// stable across client_golang versions (it's the canonical Stringer output).
func descMeta(t *testing.T, c prometheus.Collector) (string, []string) {
	t.Helper()
	ch := make(chan *prometheus.Desc, 4)
	c.Describe(ch)
	close(ch)
	d, ok := <-ch
	if !ok {
		t.Fatalf("Describe yielded zero descriptors")
	}
	s := d.String()
	fq := extractField(s, `fqName: "`, `"`)
	if fq == "" {
		t.Fatalf("could not extract fqName from %q", s)
	}
	// variableLabels printed as `variableLabels: {provider,stage}` (newer
	// client_golang) or `variableLabels: [provider stage]` (older). Try both.
	labelsRaw := extractField(s, "variableLabels: {", "}")
	if labelsRaw == "" {
		labelsRaw = extractField(s, "variableLabels: [", "]")
	}
	if labelsRaw == "" {
		t.Fatalf("could not extract variableLabels from %q", s)
	}
	// Split on whitespace or comma.
	labelsRaw = strings.ReplaceAll(labelsRaw, ",", " ")
	var labels []string
	for _, p := range strings.Fields(labelsRaw) {
		labels = append(labels, p)
	}
	return fq, labels
}

func extractField(s, prefix, suffix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	j := strings.Index(rest, suffix)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func labelSetEqual(got []string, want map[string]bool) bool {
	if len(got) != len(want) {
		return false
	}
	for _, g := range got {
		if !want[g] {
			return false
		}
	}
	return true
}
