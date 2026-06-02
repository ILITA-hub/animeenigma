// Package service holds stateless helpers for the analytics service.
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// HashIP returns sha256(ip + dailySalt) as lowercase hex, where dailySalt
// = configured salt + the UTC date. The daily rotation means the hash
// cannot be reversed to the raw IP across days, and we NEVER persist the
// raw IP. Empty ip → empty hash.
func HashIP(ip, salt string, now time.Time) string {
	if ip == "" {
		return ""
	}
	day := now.UTC().Format("2006-01-02")
	sum := sha256.Sum256([]byte(ip + "|" + salt + "|" + day))
	return hex.EncodeToString(sum[:])
}
