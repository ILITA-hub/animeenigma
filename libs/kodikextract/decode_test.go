package kodikextract

import "testing"

func TestDecodeSrc(t *testing.T) {
	// Real encoded src captured from /ftor (Quintessential Quintuplets ep1, 720p).
	// Decodes (ROT 18 + base64) to a //cloud.solodcdn.com manifest URL.
	const enc = "Tg9rjO91HK5hj2fdHOVsjq5rj20dlFVtkvDejO9pHPUdVhNrUhYgUEUbUERqVK00UBUfTBs0GrIbVLYeHBlqVBIhUrppThC3VLRuHrVuUBI0GBk5VLG2UBlsVrY2UrUhVBG3VhI2WrQeUrGeVrIhUrQdVhQeTu1eVLxwjPU6jENciEHtk3YcjBV1WI"

	got, ok := DecodeSrc(enc)
	if !ok {
		t.Fatal("DecodeSrc returned ok=false, want true")
	}
	if !contains(got, "cloud.solodcdn.com") || !contains(got, "mp4:hls:manifest.m3u8") {
		t.Fatalf("decoded URL unexpected: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
