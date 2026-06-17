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

// TestParserUnplayableTotal_IncrementsCorrectly verifies the SCRAPER-HEAL-06
// counter: name `parser_unplayable_total`, labels `{provider, server, reason}`,
// and that .Inc() bumps the value by exactly 1.0.
func TestParserUnplayableTotal_IncrementsCorrectly(t *testing.T) {
	ParserUnplayableTotal.Reset()
	before := testutil.ToFloat64(ParserUnplayableTotal.WithLabelValues("gogoanime", "vibeplayer", "ad_decoy"))
	ParserUnplayableTotal.WithLabelValues("gogoanime", "vibeplayer", "ad_decoy").Inc()
	after := testutil.ToFloat64(ParserUnplayableTotal.WithLabelValues("gogoanime", "vibeplayer", "ad_decoy"))
	if d := after - before; d != 1.0 {
		t.Fatalf("ParserUnplayableTotal delta = %v; want 1.0", d)
	}

	name, labels := descMeta(t, ParserUnplayableTotal)
	if name != "parser_unplayable_total" {
		t.Errorf("metric name = %q; want %q", name, "parser_unplayable_total")
	}
	wantLabels := map[string]bool{"provider": true, "server": true, "reason": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// TestParserAdDecoyTotal_IncrementsCorrectly verifies the SCRAPER-HEAL-06
// dedicated ad-decoy subset counter: name `parser_ad_decoy_total`, labels
// `{provider, server}`, and .Inc() bumps by 1.0.
func TestParserAdDecoyTotal_IncrementsCorrectly(t *testing.T) {
	ParserAdDecoyTotal.Reset()
	before := testutil.ToFloat64(ParserAdDecoyTotal.WithLabelValues("gogoanime", "vibeplayer"))
	ParserAdDecoyTotal.WithLabelValues("gogoanime", "vibeplayer").Inc()
	after := testutil.ToFloat64(ParserAdDecoyTotal.WithLabelValues("gogoanime", "vibeplayer"))
	if d := after - before; d != 1.0 {
		t.Fatalf("ParserAdDecoyTotal delta = %v; want 1.0", d)
	}

	name, labels := descMeta(t, ParserAdDecoyTotal)
	if name != "parser_ad_decoy_total" {
		t.Errorf("metric name = %q; want %q", name, "parser_ad_decoy_total")
	}
	wantLabels := map[string]bool{"provider": true, "server": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// TestPlayabilityCanaryRunsTotal_IncrementsCorrectly verifies the SCRAPER-HEAL-13
// counter: name `playability_canary_runs_total`, labels `{provider, server,
// result, reason, anime_slot}`, and that .Inc() bumps the value by exactly 1.0.
func TestPlayabilityCanaryRunsTotal_IncrementsCorrectly(t *testing.T) {
	PlayabilityCanaryRunsTotal.Reset()
	before := testutil.ToFloat64(PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime", "streamhg", "pass", "playable", "anchor_frieren"))
	PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime", "streamhg", "pass", "playable", "anchor_frieren").Inc()
	after := testutil.ToFloat64(PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime", "streamhg", "pass", "playable", "anchor_frieren"))
	if d := after - before; d != 1.0 {
		t.Fatalf("PlayabilityCanaryRunsTotal delta = %v; want 1.0", d)
	}

	name, labels := descMeta(t, PlayabilityCanaryRunsTotal)
	if name != "playability_canary_runs_total" {
		t.Errorf("metric name = %q; want %q", name, "playability_canary_runs_total")
	}
	wantLabels := map[string]bool{"provider": true, "server": true, "result": true, "reason": true, "anime_slot": true}
	if !labelSetEqual(labels, wantLabels) {
		t.Errorf("labels = %v; want %v", labels, wantLabels)
	}
}

// TestPlayabilityCanaryRunsTotal_AllReasonsAccepted exercises every streamprobe
// Reason string value as a `reason` label value — guarantees the canary can
// emit every possible Probe outcome without panicking. Same string-identity
// dance as TestParserUnplayableTotal_AllReasonsAccepted (no streamprobe import
// to keep the metrics package dependency-free).
func TestPlayabilityCanaryRunsTotal_AllReasonsAccepted(t *testing.T) {
	PlayabilityCanaryRunsTotal.Reset()
	reasons := []string{
		"playable",
		"ad_decoy",
		"zero_match",
		"status_403",
		"signed_url_expired",
		"cdn_unreachable",
		"empty_response",
	}
	for _, reason := range reasons {
		reason := reason
		t.Run(reason, func(t *testing.T) {
			c := PlayabilityCanaryRunsTotal.WithLabelValues("gogoanime", "streamhg", "pass", reason, "anchor_frieren")
			if c == nil {
				t.Fatalf("WithLabelValues(reason=%q) returned nil", reason)
			}
			c.Inc()
		})
	}
}

// TestAnimeSlots_ExactlyFive verifies the canonical anime_slot literal values
// used as the canary metric's `anime_slot` label domain. Single source of
// truth — Grafana panels, the canary job, and the alert rules in Plan 23-03
// all key off these strings (per CONTEXT.md D4: literal strings, not numeric
// indexes).
func TestAnimeSlots_ExactlyFive(t *testing.T) {
	slots := AnimeSlots()
	if len(slots) != 5 {
		t.Fatalf("AnimeSlots length = %d; want 5", len(slots))
	}
	want := []string{"anchor_frieren", "anchor_one_piece", "recent_1", "recent_2", "recent_3"}
	for i, w := range want {
		if slots[i] != w {
			t.Errorf("AnimeSlots()[%d] = %q; want %q", i, slots[i], w)
		}
	}
}

// TestParserUnplayableTotal_AllReasonsAccepted exercises every value of the
// libs/streamprobe.ReasonEnum as a `reason` label value. The metrics package
// does NOT import libs/streamprobe (to keep the package dependency-free and
// avoid a cyclic potential), so this table test enforces string identity by
// listing the 7 canonical values verbatim. Any future addition to the enum
// MUST also be added here.
func TestParserUnplayableTotal_AllReasonsAccepted(t *testing.T) {
	ParserUnplayableTotal.Reset()
	reasons := []string{
		"playable",
		"ad_decoy",
		"zero_match",
		"status_403",
		"signed_url_expired",
		"cdn_unreachable",
		"empty_response",
	}
	for _, reason := range reasons {
		reason := reason
		t.Run(reason, func(t *testing.T) {
			c := ParserUnplayableTotal.WithLabelValues("gogoanime", "vibeplayer", reason)
			if c == nil {
				t.Fatalf("WithLabelValues(provider=gogoanime, server=vibeplayer, reason=%q) returned nil", reason)
			}
			// Must not panic.
			c.Inc()
		})
	}
}

func TestProviderEnabledAndInfo_Exist(t *testing.T) {
	ProviderEnabled.WithLabelValues("animepahe").Set(0)
	ProviderEnabled.WithLabelValues("allanime").Set(1)
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("animepahe")); got != 0 {
		t.Errorf("provider_enabled{animepahe} = %v; want 0", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("allanime")); got != 1 {
		t.Errorf("provider_enabled{allanime} = %v; want 1", got)
	}

	ProviderInfo.WithLabelValues("animepahe", "disabled", "Cloudflare challenge", "moved to CF").Set(1)
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("animepahe", "disabled", "Cloudflare challenge", "moved to CF")); got != 1 {
		t.Errorf("provider_info{...} = %v; want 1", got)
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
