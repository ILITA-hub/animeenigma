package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolClient_Fetch_DecodesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/guessgame/pool", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[
			{"id":"frieren","name_ru":"Фрирен","poster_url":"p","year":2023,"episodes":28,
			 "score":9.3,"status":"released","rating":"pg_13",
			 "genres":[{"id":"1","name":"Драма"}],"studios":[{"id":"s","name":"Madhouse"}],"tags":[]}
		]}`))
	}))
	defer srv.Close()

	c := NewPoolClient(srv.URL, 5*time.Second, nil)
	pool, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, pool, 1)
	assert.Equal(t, "frieren", pool[0].ID)
	assert.Equal(t, 2023, pool[0].Year)
	assert.Equal(t, "Madhouse", pool[0].Studios[0].Name)
	assert.Empty(t, pool[0].Tags)
}

func TestPoolClient_Fetch_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewPoolClient(srv.URL, 5*time.Second, nil).Fetch(context.Background())
	require.Error(t, err)
}
