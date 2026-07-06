package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// Health is the liveness handler (GET + HEAD /health).
func Health(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
