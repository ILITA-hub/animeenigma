package videoutils

import "testing"

// TestFirstPartyAddr verifies the dial-time first-party classification that
// decides whether a host may reach a private IP (finding #64/#65). Only the
// configured internal hosts are exempt; matching is exact (no subdomain),
// case-insensitive, port-tolerant.
func TestFirstPartyAddr(t *testing.T) {
	fp := []string{"minio", "stealth-scraper"}

	allow := []string{"minio:9000", "minio", "stealth-scraper:3000", "MINIO:9000"}
	for _, a := range allow {
		if !firstPartyAddr(a, fp) {
			t.Errorf("firstPartyAddr(%q) = false, want true", a)
		}
	}

	deny := []string{
		"cdn.evil.com:443",
		"169.254.169.254:80",
		"minio.evil.com:443", // suffix imposter must NOT be first-party
		"api.minio:80",       // subdomain is NOT an exact first-party host
		"8.8.8.8:53",
		"",
	}
	for _, a := range deny {
		if firstPartyAddr(a, fp) {
			t.Errorf("firstPartyAddr(%q) = true, want false", a)
		}
	}
}
