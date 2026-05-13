package streamprobe

import "testing"

func TestReason_AllReasons_Length(t *testing.T) {
	got := AllReasons()
	if len(got) != 7 {
		t.Fatalf("AllReasons() returned %d values; want 7", len(got))
	}
}

func TestReason_ValuesMatchExpectedTokens(t *testing.T) {
	tests := []struct {
		name     string
		reason   Reason
		expected string
	}{
		{"playable", ReasonPlayable, "playable"},
		{"ad_decoy", ReasonAdDecoy, "ad_decoy"},
		{"zero_match", ReasonZeroMatch, "zero_match"},
		{"status_403", ReasonStatus403, "status_403"},
		{"signed_url_expired", ReasonSignedURLExpired, "signed_url_expired"},
		{"cdn_unreachable", ReasonCDNUnreachable, "cdn_unreachable"},
		{"empty_response", ReasonEmptyResponse, "empty_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.reason) != tt.expected {
				t.Fatalf("Reason value = %q; want %q", string(tt.reason), tt.expected)
			}
		})
	}
}

func TestReason_StringRoundtrip(t *testing.T) {
	if Reason("ad_decoy") != ReasonAdDecoy {
		t.Fatalf("Reason(\"ad_decoy\") != ReasonAdDecoy")
	}
	if Reason("playable") != ReasonPlayable {
		t.Fatalf("Reason(\"playable\") != ReasonPlayable")
	}
}

func TestReason_AllReasons_DeclarationOrder(t *testing.T) {
	want := []Reason{
		ReasonPlayable,
		ReasonAdDecoy,
		ReasonZeroMatch,
		ReasonStatus403,
		ReasonSignedURLExpired,
		ReasonCDNUnreachable,
		ReasonEmptyResponse,
	}
	got := AllReasons()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllReasons()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
