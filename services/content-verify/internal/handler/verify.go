package handler

import "net/http"

// VerifyHandler serves the internal verdicts/hint/queue API. Wired in Task 9.
type VerifyHandler struct{}

func (h *VerifyHandler) Verdicts(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
func (h *VerifyHandler) Hint(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
func (h *VerifyHandler) Queue(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
