package miruro

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vectorFile mirrors the testdata/transform_vectors.json shape we need
// for the table-driven test. Extra fields in the JSON are ignored.
type vectorFile struct {
	KeyFindings   map[string]any `json:"key_findings"`
	Vectors       []vectorEntry  `json:"vectors"`
	NegativeCases []negativeCase `json:"negative_cases"`
}

type vectorEntry struct {
	Name              string         `json:"name"`
	EndpointPath      string         `json:"endpoint_path"`
	Method            string         `json:"method"`
	Query             map[string]any `json:"query"`
	ExpectedURLQueryE string         `json:"expected_url_query_e"`
	ExpectedFullURL   string         `json:"expected_full_url"`
}

type negativeCase struct {
	Name          string `json:"name"`
	EndpointPath  string `json:"endpoint_path"`
	ExpectedError string `json:"expected_error"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	path := filepath.Join("testdata", "transform_vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var vf vectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if len(vf.Vectors) < 3 {
		t.Fatalf("expected at least 3 vectors, got %d", len(vf.Vectors))
	}
	return vf
}

// TestTransformProxyURL verifies the base64url-of-canonical-JSON
// transform produces the expected upstream-accepted strings for the
// info / episodes / sources GET endpoints. Vectors are sourced from
// testdata/transform_vectors.json (captured live 2026-05-20 from the
// production server against Miruro for Frieren AniList 154587).
func TestTransformProxyURL(t *testing.T) {
	vf := loadVectors(t)

	// Validate the discovery summary asserts the GET path does NOT use
	// the VITE_PROXY_OBF_KEY. This is a regression guard: if a future
	// vectors file flips this flag, the test should explicitly fail so a
	// human re-reviews the architecture before changing the Go port.
	if used, ok := vf.KeyFindings["VITE_PROXY_OBF_KEY_used"].(bool); !ok || used {
		t.Fatalf("testdata vectors claim VITE_PROXY_OBF_KEY is used — this would invalidate TransformProxyURL's ignored obfKey arg")
	}

	for _, v := range vf.Vectors {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			// Each vector exercises BuildSecurePipeURL through the full
			// path (since TransformProxyURL alone only handles the
			// query-less GET form).
			got, err := BuildSecurePipeURL("", v.EndpointPath, v.Query)
			if err != nil {
				t.Fatalf("BuildSecurePipeURL(%q, %v): %v", v.EndpointPath, v.Query, err)
			}
			if got != v.ExpectedFullURL {
				t.Errorf("URL mismatch:\n  got:  %s\n  want: %s", got, v.ExpectedFullURL)
			}
			// Also drill into the bare `e=` param.
			gotE := strings.SplitN(got, "?e=", 2)
			if len(gotE) != 2 {
				t.Fatalf("got URL has no ?e= param: %s", got)
			}
			if gotE[1] != v.ExpectedURLQueryE {
				t.Errorf("e= mismatch:\n  got:  %s\n  want: %s", gotE[1], v.ExpectedURLQueryE)
			}
		})
	}

	t.Run("TransformProxyURL_ignores_obfKey", func(t *testing.T) {
		// The function signature accepts an obfKey but the GET path
		// does NOT consume it. Two distinct keys must produce identical
		// output for the same endpoint.
		a, errA := TransformProxyURL("info/154587", []byte("AAAAAAAAAAAAAAAA"))
		b, errB := TransformProxyURL("info/154587", []byte("BBBBBBBBBBBBBBBB"))
		if errA != nil || errB != nil {
			t.Fatalf("unexpected errors: %v / %v", errA, errB)
		}
		if a != b {
			t.Errorf("obfKey changed output: got distinct values\n  a=%s\n  b=%s", a, b)
		}
	})
}

// TestTransformProxyURL_NegativeCases asserts validation errors are
// returned for malformed inputs that the upstream would never accept.
func TestTransformProxyURL_NegativeCases(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantErr  error
	}{
		{"empty_endpoint", "", ErrEmptyEndpoint},
		{"absolute_slash_prefix", "/api/info/154587", ErrAbsoluteEndpoint},
		{"absolute_api_prefix", "api/info/154587", ErrAbsoluteEndpoint},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := TransformProxyURL(tc.endpoint, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("TransformProxyURL(%q): err=%v want=%v", tc.endpoint, err, tc.wantErr)
			}
		})
	}
}

// TestDecodePipeKey ensures the static VITE_PIPE_OBF_KEY hex decodes to
// the expected 16-byte payload and that malformed input is rejected.
func TestDecodePipeKey(t *testing.T) {
	// Per spike SPIKE-MIRURO.md: VITE_PIPE_OBF_KEY = 71951034f8fbcf53d89db52ceb3dc22c
	const upstreamKey = "71951034f8fbcf53d89db52ceb3dc22c"
	b, err := DecodePipeKey(upstreamKey)
	if err != nil {
		t.Fatalf("decode upstream key: %v", err)
	}
	if len(b) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(b))
	}
	// Round-trip
	if got := hex.EncodeToString(b); got != upstreamKey {
		t.Errorf("round-trip: got %q want %q", got, upstreamKey)
	}

	t.Run("bad_hex", func(t *testing.T) {
		_, err := DecodePipeKey("not-hex-at-all")
		if !errors.Is(err, ErrInvalidPipeKey) {
			t.Errorf("expected ErrInvalidPipeKey, got %v", err)
		}
	})
	t.Run("wrong_length", func(t *testing.T) {
		_, err := DecodePipeKey("aabbccdd") // 4 bytes
		if !errors.Is(err, ErrInvalidPipeKey) {
			t.Errorf("expected ErrInvalidPipeKey, got %v", err)
		}
	})
}

// TestDecodeObfuscatedResponse_GzipPath synthesizes a gzip(json) →
// base64url payload and verifies DecodeObfuscatedResponse round-trips
// it back to the original JSON when x-obfuscated="1".
func TestDecodeObfuscatedResponse_GzipPath(t *testing.T) {
	original := []byte(`{"hello":"world","n":42}`)
	encoded := encodeForTest(t, original, false, nil)

	got, err := DecodeObfuscatedResponse(encoded, XObfuscatedGzip, nil)
	if err != nil {
		t.Fatalf("DecodeObfuscatedResponse: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("round-trip mismatch:\n  got:  %s\n  want: %s", got, original)
	}
}

// TestDecodeObfuscatedResponse_XorGzipPath synthesizes a
// base64url(xor_cycle(gzip(json), key)) payload and verifies the round-
// trip for x-obfuscated="2" with the real upstream key.
func TestDecodeObfuscatedResponse_XorGzipPath(t *testing.T) {
	const upstreamKey = "71951034f8fbcf53d89db52ceb3dc22c"
	key, err := DecodePipeKey(upstreamKey)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	original := []byte(`{"hello":"xor-world","arr":[1,2,3]}`)
	encoded := encodeForTest(t, original, true, key)

	got, err := DecodeObfuscatedResponse(encoded, XObfuscatedXorGz, key)
	if err != nil {
		t.Fatalf("DecodeObfuscatedResponse: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("round-trip mismatch:\n  got:  %s\n  want: %s", got, original)
	}

	t.Run("xor_path_requires_key", func(t *testing.T) {
		_, err := DecodeObfuscatedResponse(encoded, XObfuscatedXorGz, nil)
		if !errors.Is(err, ErrInvalidPipeKey) {
			t.Errorf("expected ErrInvalidPipeKey, got %v", err)
		}
	})
}

// TestDecodeObfuscatedResponse_Plain treats an absent x-obfuscated
// header as plain JSON and returns the body verbatim.
func TestDecodeObfuscatedResponse_Plain(t *testing.T) {
	original := []byte(`{"plain":"json"}`)
	got, err := DecodeObfuscatedResponse(original, XObfuscatedNone, nil)
	if err != nil {
		t.Fatalf("plain decode: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("plain decode mismatch")
	}
}

// TestDecodeObfuscatedResponse_UnknownHeader returns the sentinel error
// for unrecognised x-obfuscated values.
func TestDecodeObfuscatedResponse_UnknownHeader(t *testing.T) {
	_, err := DecodeObfuscatedResponse([]byte("x"), "99", nil)
	if !errors.Is(err, ErrUnknownObfuscation) {
		t.Errorf("expected ErrUnknownObfuscation, got %v", err)
	}
}

// TestDecodeObfuscatedResponse_SizeCap synthesizes a gzip bomb that
// inflates above MaxDecodedResponseBytes and confirms the decoder
// rejects it with ErrDecodedTooLarge.
func TestDecodeObfuscatedResponse_SizeCap(t *testing.T) {
	bigJSON := bytes.Repeat([]byte("A"), MaxDecodedResponseBytes+1024)
	encoded := encodeForTest(t, bigJSON, false, nil)

	_, err := DecodeObfuscatedResponse(encoded, XObfuscatedGzip, nil)
	if !errors.Is(err, ErrDecodedTooLarge) {
		t.Errorf("expected ErrDecodedTooLarge, got %v", err)
	}
}

// TestDecodeObfuscatedResponse_OnePieceClass — ISS-015 regression-lock.
// The 4 MiB cap rejected One Piece's 6.04 MiB episodes JSON in
// production on 2026-05-22. The new 16 MiB cap MUST accept a synthesized
// 8 MiB response (One Piece + 33% growth headroom). If a future
// maintainer ever lowers MaxDecodedResponseBytes below 8 MiB without
// confirming the One Piece episodes payload has shrunk upstream, this
// test fails immediately rather than re-breaking production.
func TestDecodeObfuscatedResponse_OnePieceClass(t *testing.T) {
	const onePieceClassSize = 8 << 20 // 8 MiB — One Piece + 33% headroom
	if MaxDecodedResponseBytes < onePieceClassSize {
		t.Fatalf("MaxDecodedResponseBytes=%d is below the One Piece-class "+
			"floor (%d). Lowering this cap will reject One Piece episodes "+
			"in production. See ISS-015.", MaxDecodedResponseBytes, onePieceClassSize)
	}
	body := bytes.Repeat([]byte("A"), onePieceClassSize)
	encoded := encodeForTest(t, body, false, nil)

	got, err := DecodeObfuscatedResponse(encoded, XObfuscatedGzip, nil)
	if err != nil {
		t.Fatalf("DecodeObfuscatedResponse: %v", err)
	}
	if len(got) != onePieceClassSize {
		t.Errorf("decoded length = %d; want %d", len(got), onePieceClassSize)
	}
}

// TestSoftCapWarnBytes_Invariants — ISS-015 consistency lock. The soft
// cap exists to give ops a structured-log heads-up BEFORE the hard cap
// breaks anything. Both invariants below must hold:
//
//  1. SoftCapWarnBytes < MaxDecodedResponseBytes — otherwise the warn
//     is useless (fires only when the hard cap has already rejected).
//  2. SoftCapWarnBytes >= 50% of MaxDecodedResponseBytes — otherwise
//     the warn fires too often on normal upstream growth and ops tunes
//     it out. 75% is the documented target; 50% is the hard floor.
//
// This test does NOT pin the exact ratio (which may be re-tuned with
// upstream growth) but blocks pathological mis-configurations.
func TestSoftCapWarnBytes_Invariants(t *testing.T) {
	if SoftCapWarnBytes >= MaxDecodedResponseBytes {
		t.Errorf("SoftCapWarnBytes=%d must be strictly less than "+
			"MaxDecodedResponseBytes=%d", SoftCapWarnBytes, MaxDecodedResponseBytes)
	}
	if SoftCapWarnBytes*2 < MaxDecodedResponseBytes {
		t.Errorf("SoftCapWarnBytes=%d is below 50%% of MaxDecodedResponseBytes=%d; "+
			"the warn will fire on normal upstream growth and lose signal value",
			SoftCapWarnBytes, MaxDecodedResponseBytes)
	}
}

// TestDecodeObfuscatedResponse_SoftCapAccepts — payloads between
// SoftCapWarnBytes and MaxDecodedResponseBytes must decode successfully
// (the warn is logged separately by the client.go caller; this layer
// only enforces the hard cap).
func TestDecodeObfuscatedResponse_SoftCapAccepts(t *testing.T) {
	size := SoftCapWarnBytes + 1024 // just above the soft cap
	if size >= MaxDecodedResponseBytes {
		t.Skip("soft cap + 1 KiB exceeds hard cap; cannot construct accept case")
	}
	body := bytes.Repeat([]byte("B"), size)
	encoded := encodeForTest(t, body, false, nil)

	got, err := DecodeObfuscatedResponse(encoded, XObfuscatedGzip, nil)
	if err != nil {
		t.Fatalf("DecodeObfuscatedResponse must accept payloads above the soft cap "+
			"but below the hard cap; got error: %v", err)
	}
	if len(got) != size {
		t.Errorf("decoded length = %d; want %d", len(got), size)
	}
}

// encodeForTest produces the on-the-wire payload the upstream would
// emit for `body`. xorWithKey applies the same XOR-cycle the SPA
// reverses on receipt.
func encodeForTest(t *testing.T, body []byte, xorWithKey bool, key []byte) []byte {
	t.Helper()
	var gzbuf bytes.Buffer
	zw := gzip.NewWriter(&gzbuf)
	if _, err := zw.Write(body); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	raw := gzbuf.Bytes()
	if xorWithKey {
		if len(key) == 0 {
			t.Fatalf("xor requested but key is nil")
		}
		out := make([]byte, len(raw))
		for i := range raw {
			out[i] = raw[i] ^ key[i%len(key)]
		}
		raw = out
	}
	return []byte(base64.RawURLEncoding.EncodeToString(raw))
}
