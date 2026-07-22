package promquery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

func vectorResponse() string {
	return `{"status":"success","data":{"resultType":"vector","result":[
	  {"metric":{"__name__":"ae:pressure_breach:elevated","signal":"psi_cpu_some"},"value":[1760000000,"1"]},
	  {"metric":{"__name__":"ae:pressure_breach:elevated","signal":"psi_io_full"},"value":[1760000000,"0"]},
	  {"metric":{"__name__":"ae:pressure_breach:critical","signal":"psi_cpu_some"},"value":[1760000000,"1"]},
	  {"metric":{"__name__":"ae:pressure_breach:critical","signal":"psi_io_full"},"value":[1760000000,"0"]},
	  {"metric":{"__name__":"ae:host_psi_cpu_some:ratio","instance":"node-exporter:9100"},"value":[1760000000,"0.52"]},
	  {"metric":{"__name__":"ae:host_mem_available:ratio","instance":"node-exporter:9100"},"value":[1760000000,"0.42"]},
	  {"metric":{"__name__":"ae:host_egress:bytes_per_second"},"value":[1760000000,"11000000"]},
	  {"metric":{"__name__":"ae:pressure_level:preview"},"value":[1760000000,"2"]}
	]}}`
}

// scalarResp returns a Prometheus scalar (the sample-age sub-query shape).
func scalarResp(val string) string {
	return `{"status":"success","data":{"resultType":"scalar","result":[1760000000,"` + val + `"]}}`
}

// serve routes the vector query to vectorResponse and anything else (the age
// sub-query) to a scalar of ageVal.
func serve(t *testing.T, ageVal string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query", r.URL.Path)
		q := r.URL.Query().Get("query")
		if q == AllSignalsQuery {
			_, _ = w.Write([]byte(vectorResponse()))
			return
		}
		assert.True(t, strings.Contains(q, "timestamp("), "non-vector query is the age sub-query")
		_, _ = w.Write([]byte(scalarResp(ageVal)))
	}))
}

func TestFetchVerdict_ParsesBreachesSignalsAndAge(t *testing.T) {
	srv := serve(t, "8")
	defer srv.Close()

	// uplink disabled -> egress not folded despite the egress signal present.
	v, err := NewClient(srv.URL, 0, 0.75, 0.90).FetchVerdict(context.Background())
	require.NoError(t, err)

	assert.Equal(t, domain.LevelCritical, v.Target)
	require.Len(t, v.Reasons, 1)
	assert.Equal(t, domain.Reason{Signal: "psi_cpu_some", Severity: domain.SeverityCritical}, v.Reasons[0])
	assert.Equal(t, 0.52, v.Signals["ae:host_psi_cpu_some:ratio"])
	assert.Equal(t, 8.0, v.SampleAgeSeconds, "age sub-query parsed")
	assert.Equal(t, 0.0, v.EgressFraction, "uplink disabled -> no egress fraction")
}

func TestFetchVerdict_EgressGovernance(t *testing.T) {
	srv := serve(t, "3")
	defer srv.Close()

	// egress 11 MB/s = 88 Mbps. Uplink 100 Mbps -> frac 0.88 (>= critical 0.85).
	v, err := NewClient(srv.URL, 100, 0.60, 0.85).FetchVerdict(context.Background())
	require.NoError(t, err)

	assert.InDelta(t, 0.88, v.EgressFraction, 1e-9)
	assert.Equal(t, domain.LevelCritical, v.Target)
	// egress_uplink critical reason present alongside psi_cpu_some.
	var sawEgress bool
	for _, r := range v.Reasons {
		if r.Signal == domain.EgressUplinkSignal {
			assert.Equal(t, domain.SeverityCritical, r.Severity)
			sawEgress = true
		}
	}
	assert.True(t, sawEgress, "egress_uplink breach folded into reasons")
	assert.Equal(t, 1.0, v.Score, "egress at/above critical fraction lifts score to 1.0")
}

func TestFetchVerdict_QuietBoxIsNormal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == AllSignalsQuery {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			  {"metric":{"__name__":"ae:pressure_breach:elevated","signal":"psi_cpu_some"},"value":[1,"0"]},
			  {"metric":{"__name__":"ae:host_psi_cpu_some:ratio"},"value":[1,"0.05"]}
			]}}`))
			return
		}
		_, _ = w.Write([]byte(scalarResp("2")))
	}))
	defer srv.Close()

	v, err := NewClient(srv.URL, 0, 0.75, 0.90).FetchVerdict(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.LevelNormal, v.Target)
	assert.Empty(t, v.Reasons)
}

func TestFetchVerdict_AgeSubQueryFailureIsUnknownNotStale(t *testing.T) {
	// Vector succeeds; the age sub-query 500s -> SampleAgeSeconds stays -1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == AllSignalsQuery {
			_, _ = w.Write([]byte(vectorResponse()))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	v, err := NewClient(srv.URL, 0, 0.75, 0.90).FetchVerdict(context.Background())
	require.NoError(t, err, "a failed age sub-query must not fail the whole verdict")
	assert.Equal(t, -1.0, v.SampleAgeSeconds, "unknown age, not a false staleness")
}

func TestFetchVerdict_ErrorPaths(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL, 0, 0.75, 0.90).FetchVerdict(context.Background())
		assert.Error(t, err)
	})
	t.Run("bad status field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error"}`))
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL, 0, 0.75, 0.90).FetchVerdict(context.Background())
		assert.Error(t, err)
	})
	t.Run("unreachable", func(t *testing.T) {
		_, err := NewClient("http://127.0.0.1:1", 0, 0.75, 0.90).FetchVerdict(context.Background())
		assert.Error(t, err)
	})
}
