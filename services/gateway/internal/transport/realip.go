package transport

import (
	"net"
	"net/http"
)

// RealClientIP sets r.RemoteAddr from the X-Real-IP header that our trusted
// edge (nginx) injects with $remote_addr.
//
// It deliberately replaces chi's middleware.RealIP, which trusts, in order,
// True-Client-IP, X-Real-IP, then the FIRST X-Forwarded-For entry. Two of
// those are client-spoofable in our topology: nginx never clears
// True-Client-IP, and it APPENDS to X-Forwarded-For (so a client-supplied
// value lands first). chi would then key the per-IP rate limiter, the POISON
// tarpit, and the access logs on a forged address. nginx always OVERWRITES
// X-Real-IP with the real peer ($remote_addr) and the gateway is only reachable
// through nginx, so X-Real-IP is the one trustworthy source. If it is absent or
// malformed we leave the real TCP peer (r.RemoteAddr) in place — fail safe.
func RealClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			if net.ParseIP(xrip) != nil {
				r.RemoteAddr = xrip
			}
		}
		next.ServeHTTP(w, r)
	})
}
