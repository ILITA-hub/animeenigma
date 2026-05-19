package handler

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestDecodeJSONLimited_AcceptsUnderLimit(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"x":1}`))
	var out map[string]int
	if err := DecodeJSONLimited(body, &out, 1024); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out["x"] != 1 {
		t.Fatalf("got %v", out)
	}
}

func TestDecodeJSONLimited_RejectsOverLimit(t *testing.T) {
	// 2KB of JSON garbage well beyond a 1KB limit.
	huge := append([]byte(`{"junk":"`), bytes.Repeat([]byte("a"), 2048)...)
	huge = append(huge, []byte(`"}`)...)
	body := io.NopCloser(bytes.NewReader(huge))
	var out map[string]string
	err := DecodeJSONLimited(body, &out, 1024)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
	// The error may be ErrResponseTooLarge OR a json error caused by truncation —
	// either is acceptable; the invariant is "did not silently buffer 2048 bytes".
}

func TestDecodeJSONLimited_LimitExactBoundary(t *testing.T) {
	// A body of (limit + 1) bytes — one byte past the configured ceiling —
	// must trip ErrResponseTooLarge. We set N = limit + 1 on the
	// LimitedReader, so a `limit + 1`-byte body consumes every available
	// byte and leaves N == 0 after Decode, which is our "potentially
	// truncated" signal.
	body := io.NopCloser(strings.NewReader(`{"x":1}`)) // 7 bytes
	var out map[string]int
	err := DecodeJSONLimited(body, &out, 6)
	if err == nil {
		t.Fatal("expected error at exact boundary")
	}
}
