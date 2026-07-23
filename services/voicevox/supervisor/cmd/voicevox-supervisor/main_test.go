package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadRESPBulkString(t *testing.T) {
	got, err := readRESPBulkString(bufio.NewReader(strings.NewReader("$1\r\n2\r\n")))
	if err != nil || got != "2" {
		t.Fatalf("got %q, %v; want 2, nil", got, err)
	}
}

func TestReadRESPBulkStringRejectsMissingAndMalformed(t *testing.T) {
	for _, input := range []string{"$-1\r\n", "+OK\r\n", "$2\r\n1\r\n"} {
		if _, err := readRESPBulkString(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Fatalf("input %q unexpectedly accepted", input)
		}
	}
}

func TestDurationOrDefault(t *testing.T) {
	t.Setenv("VOICEVOX_TEST_DURATION", "250ms")
	if got := durationOrDefault("VOICEVOX_TEST_DURATION", 0); got.String() != "250ms" {
		t.Fatalf("got %s", got)
	}
}
