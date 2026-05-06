package anilist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger returns a *logger.Logger usable in tests. Falls back to a
// minimal default config so test failures are visible.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.Default()
}

// happyPathHandler returns a handler that serves the given Media payload.
func happyPathHandler(t *testing.T, capture *http.Request) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			*capture = *r
			capture.Body = io.NopCloser(strings.NewReader(string(body)))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Media": {
					"tags": [
						{"name": "Slice of Life", "rank": 90, "category": "Theme-Other", "isAdult": false, "isGeneralSpoiler": false},
						{"name": "Time Travel", "rank": 75, "category": "Cast-Traits", "isAdult": false, "isGeneralSpoiler": true},
						{"name": "Romance", "rank": 60, "category": "Theme-Romance", "isAdult": false, "isGeneralSpoiler": false}
					]
				}
			}
		}`))
	}
}

func TestAniListClient_FetchTags_HappyPath(t *testing.T) {
	srv := httptest.NewServer(happyPathHandler(t, nil))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	tags, err := c.FetchTags(context.Background(), 12345)

	require.NoError(t, err)
	require.Len(t, tags, 3)
	assert.Equal(t, "Slice of Life", tags[0].Name)
	assert.Equal(t, 90, tags[0].Rank)
	assert.Equal(t, "Theme-Other", tags[0].Category)
	assert.False(t, tags[0].IsAdult)
	assert.False(t, tags[0].IsGeneralSpoiler)
	assert.Equal(t, "Time Travel", tags[1].Name)
	assert.True(t, tags[1].IsGeneralSpoiler)
}

func TestAniListClient_FetchTags_EmptyTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"Media": {"tags": []}}}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	tags, err := c.FetchTags(context.Background(), 99999)

	require.NoError(t, err)
	assert.Len(t, tags, 0)
}

func TestAniListClient_FetchTags_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors": [{"message": "Invalid Media id"}]}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	tags, err := c.FetchTags(context.Background(), 0)

	require.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "anilist")
}

func TestAniListClient_FetchTags_NetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // server is dead before the call

	c := NewClientWithBaseURL(url, testLogger(t))
	tags, err := c.FetchTags(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "anilist")
}

func TestAniListClient_FetchTags_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	tags, err := c.FetchTags(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "anilist")
}

func TestAniListClient_FetchTags_UserAgent(t *testing.T) {
	gotUA := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"Media": {"tags": []}}}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	_, err := c.FetchTags(context.Background(), 1)
	require.NoError(t, err)

	assert.Equal(t, "AnimeEnigma/1.0 (https://animeenigma.ru)", gotUA)
}

func TestAniListClient_FetchTags_QueryShape(t *testing.T) {
	gotBody := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"Media": {"tags": []}}}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL, testLogger(t))
	_, err := c.FetchTags(context.Background(), 12345)
	require.NoError(t, err)

	// Decode to confirm structure (the literal whitespace in the query may
	// be JSON-escaped, so assert via parse + substring).
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(gotBody), &parsed))
	q, ok := parsed["query"].(string)
	require.True(t, ok, "request body must include query field")
	assert.Contains(t, q, "Media(id:")
	assert.Contains(t, q, "tags { name rank category isAdult isGeneralSpoiler }")
	vars, ok := parsed["variables"].(map[string]interface{})
	require.True(t, ok)
	// JSON unmarshals numbers as float64.
	assert.Equal(t, float64(12345), vars["id"])
}

func TestSlugifyTagName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Slice of Life", "slice_of_life"},
		{"Sci-Fi", "sci_fi"},
		{"Boys' Love", "boys_love"},
		{"", ""},
		{"  Action  ", "action"},
		{"A & B", "a_b"},
		{"Mahō Shōjo", "mah_sh_jo"}, // ASCII-only regex collapses ō/Ō to underscores
		{"___Mecha___", "mecha"},
		{"Multi   Space", "multi_space"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, SlugifyTagName(tc.in), "input=%q", tc.in)
		})
	}
}
