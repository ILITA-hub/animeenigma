// Package netguard provides SSRF guards shared across services: classifying an
// IP as non-public, a net.Dialer.Control hook that rejects connections to
// private addresses AFTER DNS resolution (closing DNS-rebind gaps), a cheap
// pre-flight URL validator, and a first-party host check used to exempt
// configured internal hosts (MinIO, the stealth-scraper sidecar) from the
// private-IP / https-only policy.
package netguard

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// IsPrivateIP reports whether ip is in a range that must not be reachable from
// a server-side ("SSRF-able") request: loopback, RFC1918 private, link-local
// unicast (which includes the 169.254.169.254 cloud-metadata endpoint),
// link-/interface-local and ordinary multicast, IPv6 unique-local (fc00::/7),
// CGNAT 100.64.0.0/10 (RFC6598), and the unspecified address. A nil ip is
// treated as private so callers fail closed.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10 — not covered by any net.IP helper. The block is
	// 100.64.0.0 .. 100.127.255.255, i.e. first octet 100 and the top two bits
	// of the second octet set to 01.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 0x40 {
		return true
	}
	return false
}

// DenyPrivateControl is a net.Dialer.Control hook. It runs on the concrete
// ip:port the dialer is about to connect to (i.e. AFTER DNS resolution), so it
// blocks DNS-rebind attacks that a pre-flight URL check cannot. Returning an
// error aborts the dial. Use it on http.Client transports that must only ever
// reach public hosts (the playability probe, the admin image-by-URL ingest).
func DenyPrivateControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("netguard: bad dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("netguard: dial address %q is not an IP literal", host)
	}
	if IsPrivateIP(ip) {
		return fmt.Errorf("netguard: blocked dial to private address %s", ip)
	}
	return nil
}

// ValidatePublicURL is a cheap pre-flight check (no DNS): the scheme must be
// http or https, the host must be present, and an IP-literal host must not be
// private. Hostnames pass — the authoritative, rebind-safe enforcement is
// DenyPrivateControl at dial time. It returns a descriptive error on rejection.
func ValidatePublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("netguard: unparseable URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("netguard: disallowed scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("netguard: empty host in URL")
	}
	if ip := net.ParseIP(host); ip != nil && IsPrivateIP(ip) {
		return fmt.Errorf("netguard: private IP-literal host %s", ip)
	}
	return nil
}

// HostIsFirstParty reports whether host equals, or is a strict subdomain of,
// any entry in allow (case-insensitive, trailing dot tolerated). It is used to
// exempt configured internal hosts (MinIO, stealth-scraper) from the
// private-IP and https-only guards, since they legitimately resolve to
// docker-private IPs and may be reached over http.
func HostIsFirstParty(host string, allow []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(a, ".")))
		if a == "" {
			continue
		}
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}
