package domain

import "testing"

// TestSkipStatusConstsSanity is a smoke test that the skip status/kind
// consts exist and hold the expected wire strings — these are consumed by
// later tasks (queue/handler) and by the frontend, so a rename here is a
// breaking change we want a test to flag.
func TestSkipStatusConstsSanity(t *testing.T) {
	cases := map[string]string{
		SkipDetected:    "detected",
		SkipNoMatch:     "no_match",
		SkipPendingFP:   "pending_fp",
		SkipUnreachable: "unreachable",
		SkipAniskip:     "aniskip",
		SkipKindOp:      "op",
		SkipKindEd:      "ed",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("const = %q, want %q", got, want)
		}
	}
}
