package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMetricPathUsesMatchedRoutePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    string
	}{
		{
			name:    "signed stream tokens collapse into wildcard route",
			pattern: "/api/streaming/m/*",
			path:    "/api/streaming/m/eyJhbGciOiJIUzI1NiJ9/segment-123.ts",
			want:    "/api/streaming/m/:wildcard",
		},
		{
			name:    "named parameters retain their diagnostic name",
			pattern: "/api/anime/{id}/episodes/{episode}",
			path:    "/api/anime/42/episodes/7",
			want:    "/api/anime/:id/episodes/:episode",
		},
		{
			name:    "regex parameters discard the regex",
			pattern: "/worker/models/{name:[a-z]+}",
			path:    "/worker/models/upscaler",
			want:    "/worker/models/:name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			router := chi.NewRouter()
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r)
					got = metricPath(r)
				})
			})
			router.Method(http.MethodGet, tt.pattern, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))
			if got != tt.want {
				t.Fatalf("metricPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMetricPathCategorizesUnmatchedTraffic(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/api/random/attacker/value", want: "/api/:other"},
		{path: "/internal/probe/secret", want: "/internal/:other"},
		{path: "/admin/scanner.php", want: "/admin/:other"},
		{path: "/worker/unknown", want: "/worker/:other"},
		{path: "/assets/hash/app.js", want: "/static/:other"},
		{path: "/.well-known/security.txt", want: "/.well-known/:other"},
		{path: "/health/extended", want: "/health/:other"},
		{path: "/totally-random-1/unique-value", want: "/other"},
		{path: "/totally-random-2/another-value", want: "/other"},
	}

	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, tt.path, nil)
		if got := metricPath(r); got != tt.want {
			t.Errorf("metricPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
