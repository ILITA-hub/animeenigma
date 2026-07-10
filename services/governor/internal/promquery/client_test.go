package promquery

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	  {"metric":{"__name__":"ae:pressure_level:preview"},"value":[1760000000,"2"]}
	]}}`
}

func TestFetchVerdict_ParsesBreachesAndSignals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/query", r.URL.Path)
		assert.Equal(t, AllSignalsQuery, r.URL.Query().Get("query"))
		_, _ = w.Write([]byte(vectorResponse()))
	}))
	defer srv.Close()

	v, err := NewClient(srv.URL).FetchVerdict(context.Background())
	require.NoError(t, err)

	assert.Equal(t, domain.LevelCritical, v.Target)
	// psi_cpu_some breaches BOTH tiers -> reported once at critical.
	require.Len(t, v.Reasons, 1)
	assert.Equal(t, domain.Reason{Signal: "psi_cpu_some", Severity: domain.SeverityCritical}, v.Reasons[0])
	assert.Equal(t, 0.52, v.Signals["ae:host_psi_cpu_some:ratio"])
	assert.Equal(t, 0.42, v.Signals["ae:host_mem_available:ratio"])
}

func TestFetchVerdict_QuietBoxIsNormal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
		  {"metric":{"__name__":"ae:pressure_breach:elevated","signal":"psi_cpu_some"},"value":[1,"0"]},
		  {"metric":{"__name__":"ae:host_psi_cpu_some:ratio"},"value":[1,"0.05"]}
		]}}`))
	}))
	defer srv.Close()

	v, err := NewClient(srv.URL).FetchVerdict(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.LevelNormal, v.Target)
	assert.Empty(t, v.Reasons)
}

func TestFetchVerdict_ErrorPaths(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL).FetchVerdict(context.Background())
		assert.Error(t, err)
	})
	t.Run("bad status field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"error"}`))
		}))
		defer srv.Close()
		_, err := NewClient(srv.URL).FetchVerdict(context.Background())
		assert.Error(t, err)
	})
	t.Run("unreachable", func(t *testing.T) {
		_, err := NewClient("http://127.0.0.1:1").FetchVerdict(context.Background())
		assert.Error(t, err)
	})
}
