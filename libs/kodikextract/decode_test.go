package kodikextract

import (
	"strings"
	"testing"
)

func TestDecodeSrc(t *testing.T) {
	// Real encoded src captured from /ftor (Quintessential Quintuplets ep1, 720p).
	// Decodes (ROT 18 + base64) to a //cloud.solodcdn.com manifest URL.
	const enc = "Tg9rjO91HK5hj2fdHOVsjq5rj20dlFVtkvDejO9pHPUdVhNrUhYgUEUbUERqVK00UBUfTBs0GrIbVLYeHBlqVBIhUrppThC3VLRuHrVuUBI0GBk5VLG2UBlsVrY2UrUhVBG3VhI2WrQeUrGeVrIhUrQdVhQeTu1eVLxwjPU6jENciEHtk3YcjBV1WI"

	got, ok := DecodeSrc(enc)
	if !ok {
		t.Fatal("DecodeSrc returned ok=false, want true")
	}
	if !strings.Contains(got, "cloud.solodcdn.com") || !strings.Contains(got, "mp4:hls:manifest.m3u8") {
		t.Fatalf("decoded URL unexpected: %q", got)
	}
}
