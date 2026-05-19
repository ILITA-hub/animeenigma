package handler

import (
	"encoding/json"
	"errors"
	"io"
)

// MaxImporterResponseBytes is the upper bound applied to any JSON
// response we accept from an external importer source (MAL,
// Shikimori, etc). 50 MiB is generous for full-list exports;
// anything larger is pathological and likely an abuse signal.
const MaxImporterResponseBytes int64 = 50 * 1024 * 1024

// ErrResponseTooLarge is returned when the external API's body would
// exceed the configured limit.
var ErrResponseTooLarge = errors.New("external response exceeds size limit")

// DecodeJSONLimited reads at most `limit` bytes from r and JSON-decodes
// into out. If the body is at or beyond the limit, returns
// ErrResponseTooLarge.
func DecodeJSONLimited(r io.Reader, out interface{}, limit int64) error {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	if err := json.NewDecoder(lr).Decode(out); err != nil {
		if lr.N <= 0 {
			return ErrResponseTooLarge
		}
		return err
	}
	if lr.N <= 0 {
		return ErrResponseTooLarge
	}
	return nil
}
