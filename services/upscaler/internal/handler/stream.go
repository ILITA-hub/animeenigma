package handler

import (
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// clearWriteDeadline removes the absolute per-response write deadline so a large
// or long-lived stream isn't severed mid-flight by http.Server.WriteTimeout —
// which is an ABSOLUTE deadline from when the response started, so a heartbeat
// cannot rescue it. A zero time means "no deadline"; short requests finish well
// inside the global limit and are unaffected.
//
// Non-fatal: on failure it logs warnMsg (with the caller's kvs plus the error)
// and the caller proceeds to stream under the global deadline.
func clearWriteDeadline(w http.ResponseWriter, log *logger.Logger, warnMsg string, kvs ...any) {
	rc := http.NewResponseController(w)
	if rc == nil {
		return
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Warnw(warnMsg, append(kvs, "error", err)...)
	}
}
