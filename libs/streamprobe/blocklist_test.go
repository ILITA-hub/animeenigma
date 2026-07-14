package streamprobe

import (
	"os"
	"strings"
	"testing"
)

func TestIsAdCDNHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"foo.ibyteimg.com", true},
		{"ibyteimg.com", true},
		{"IbyTeImG.com", true},
		{"p16-ad-sg.ibyteimg.com", true},
		{"p16-ad-sg-foo.example.com", true},
		{"sub.ad-site-i18n.example.org", true},
		{"tiktokcdn.com", true},
		{"foo.tiktokcdn.com", true},
		{"9hjkrt.nekostream.site", true},
		{"nekostream.site", true},
		{"example.com", false},
		{"premilkyway.com", false},
		{"dramiyos-cdn.com", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isAdCDNHost(tt.host)
			if got != tt.want {
				t.Fatalf("isAdCDNHost(%q) = %v; want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestBlocklist_ContainsExpectedSuffixes(t *testing.T) {
	data, err := os.ReadFile("blocklist.go")
	if err != nil {
		t.Fatalf("read blocklist.go: %v", err)
	}
	body := string(data)
	for _, suf := range []string{"ibyteimg.com", "p16-ad-sg", "ad-site-i18n", "tiktokcdn.com", "nekostream.site"} {
		if !strings.Contains(body, suf) {
			t.Fatalf("blocklist.go missing suffix %q", suf)
		}
	}
}
