// Package miruro implements the Miruro provider's upstream "obfuscation"
// transforms, which despite the menacing name are transport-encoding
// adapters around base64url + gzip + (optional) XOR-cycling with the
// VITE_PIPE_OBF_KEY.
//
// [VERIFIED: testdata/transform_vectors.json, SPIKE-MIRURO.md]
//
// Discovery summary (SCRAPER-HEAL-34 spike, 2026-05-20):
//
//   Request URL  = host + "/api/secure/pipe?e=" + base64url-no-pad(
//                    json({path: <endpoint>,
//                          method: <GET|POST>,
//                          query:  <map>,
//                          body:   <object|null>,
//                          version: <optional protocol version string>}))
//
//   Response body (x-obfuscated: "1") = base64url-no-pad(gzip(jsonBody))
//   Response body (x-obfuscated: "2") = base64url-no-pad(
//                                         xor_cycle(gzip(jsonBody),
//                                                   pipeObfKey))
//   Response body (x-obfuscated absent) = plain JSON
//
// No HMAC, no AES, no VITE_PROXY_OBF_KEY usage on the GET path. Stdlib
// only — this file deliberately holds zero third-party imports.
//
// VITE_PROXY_OBF_KEY is preserved as an argument to TransformProxyURL for
// signature compatibility with SCRAPER-HEAL-37's `obfKey` plumbing, even
// though it is unused on the GET path. The Miruro POST flow (which Wave 2
// does NOT exercise, since info/episodes/sources are all GETable) uses an
// ECDH-ES JWE envelope; if that ever becomes necessary, a sibling
// jweEnvelope() helper would live in this file, NOT in a third-party dep.
package miruro

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

// Default Miruro host (the React SPA's origin). The /api/secure/pipe
// endpoint is rooted here, not on pro.ultracloud.cc — the latter is
// VITE_PROXY_A, a backup route the SPA does NOT actually use.
const DefaultMiruroHost = "https://www.miruro.tv"

// SecurePipePath is the constant Miruro endpoint that takes the
// base64url-of-JSON request descriptor as the `e=` query parameter.
const SecurePipePath = "/api/secure/pipe"

// Header sentinel values for the response decoder. Values are upstream-
// defined; do not change without re-running the spike.
const (
	XObfuscatedNone   = ""
	XObfuscatedGzip   = "1"
	XObfuscatedXorGz  = "2"
)

// MaxDecodedResponseBytes caps the gunzip output to defend against
// inflation-bomb DoS (T-28-00-03). Largest legitimate Miruro response
// observed during the spike was ~1.3 MiB (`info/154587`); 4 MiB leaves
// headroom for future endpoints that bundle more episodes.
const MaxDecodedResponseBytes = 4 << 20 // 4 MiB

var (
	// ErrEmptyEndpoint is returned by TransformProxyURL when the caller
	// passes an empty endpoint string.
	ErrEmptyEndpoint = errors.New("miruro: endpoint path must not be empty")

	// ErrAbsoluteEndpoint is returned when the endpoint starts with '/'
	// or '/api/' — those prefixes are server-side concerns; callers must
	// pass the unprefixed name (e.g. "info/154587", not "/api/info/154587").
	ErrAbsoluteEndpoint = errors.New("miruro: endpoint must not start with '/' or '/api/'")

	// ErrInvalidPipeKey is returned when the supplied PIPE OBF key cannot
	// be hex-decoded or has the wrong byte length (must decode to 16
	// bytes per the upstream constant).
	ErrInvalidPipeKey = errors.New("miruro: pipe obf key must hex-decode to 16 bytes")

	// ErrUnknownObfuscation is returned by DecodeObfuscatedResponse when
	// the x-obfuscated header carries a value other than "", "1", or "2".
	ErrUnknownObfuscation = errors.New("miruro: unknown x-obfuscated value")

	// ErrDecodedTooLarge is returned when a gunzipped response exceeds
	// MaxDecodedResponseBytes.
	ErrDecodedTooLarge = errors.New("miruro: decoded response exceeded size cap")
)

// requestDescriptor is the JSON payload Miruro's SPA serializes into the
// `e=` query parameter on every makeSecureGet call. Field names and
// ordering MUST match the SPA's JSON.stringify output exactly — Go's
// encoding/json produces objects in the order fields appear in the
// struct, which the test's golden vectors lock down.
type requestDescriptor struct {
	Path    string         `json:"path"`
	Method  string         `json:"method"`
	Query   map[string]any `json:"query"`
	Body    any            `json:"body"`
	Version string         `json:"version,omitempty"`
}

// TransformProxyURL builds the value of the `e=` query parameter that
// Miruro's `/api/secure/pipe` endpoint consumes for a GET request to the
// upstream `endpoint`. The `obfKey` argument is unused on the GET path
// (see package doc) and is kept solely so the caller signature in Wave 2
// (Plan 28-04) does not need to fork for GET vs POST.
//
// Returns the base64url-no-padding-encoded JSON descriptor. The caller
// is responsible for assembling the full request URL — typically via
// BuildSecurePipeURL.
//
// Validation:
//   - empty endpoint            → ErrEmptyEndpoint
//   - endpoint starts with '/'  → ErrAbsoluteEndpoint
//   - endpoint starts with 'api/' or '/api/' → ErrAbsoluteEndpoint
func TransformProxyURL(endpoint string, obfKey []byte) (string, error) {
	_ = obfKey // intentionally ignored on the GET path
	return transformGet(endpoint, "GET", nil, nil)
}

// transformGet is the internal worker for TransformProxyURL.
func transformGet(endpoint, method string, query map[string]any, body any) (string, error) {
	if endpoint == "" {
		return "", ErrEmptyEndpoint
	}
	if strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "api/") {
		return "", ErrAbsoluteEndpoint
	}

	desc := requestDescriptor{
		Path:   endpoint,
		Method: method,
		Query:  query,
		Body:   body,
	}
	raw, err := marshalCanonicalDescriptor(desc)
	if err != nil {
		return "", fmt.Errorf("miruro: marshal descriptor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// marshalCanonicalDescriptor produces a deterministic JSON byte string
// equivalent to JSON.stringify's output for the requestDescriptor shape.
// Critically: the keys must appear in the order {path, method, query,
// body[, version]} and the inner `query` map must be sorted by key —
// Go's encoding/json sorts map keys by default, which matches JS's
// V8/SpiderMonkey for-in iteration order on string keys with integer
// values (the keys Miruro uses are all simple ASCII).
//
// The function also enforces: if query is nil, emit `{}`; if body is
// nil, emit `null`. This is what the SPA produces and what the golden
// vectors expect.
func marshalCanonicalDescriptor(d requestDescriptor) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	if err := writeJSONField(&buf, "path", d.Path, true); err != nil {
		return nil, err
	}
	if err := writeJSONField(&buf, "method", d.Method, false); err != nil {
		return nil, err
	}

	// "query": <object or {}>
	buf.WriteString(`,"query":`)
	if d.Query == nil || len(d.Query) == 0 {
		buf.WriteString("{}")
	} else {
		if err := writeSortedObject(&buf, d.Query); err != nil {
			return nil, err
		}
	}

	// "body": <value or null>
	buf.WriteString(`,"body":`)
	if d.Body == nil {
		buf.WriteString("null")
	} else {
		bb, err := json.Marshal(d.Body)
		if err != nil {
			return nil, err
		}
		buf.Write(bb)
	}

	if d.Version != "" {
		if err := writeJSONField(&buf, "version", d.Version, false); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// writeJSONField writes `"key":<json(value)>` to buf, prefixed with a
// comma when `first` is false.
func writeJSONField(buf *bytes.Buffer, key, value string, first bool) error {
	if !first {
		buf.WriteByte(',')
	}
	kb, err := json.Marshal(key)
	if err != nil {
		return err
	}
	buf.Write(kb)
	buf.WriteByte(':')
	vb, err := json.Marshal(value)
	if err != nil {
		return err
	}
	buf.Write(vb)
	return nil
}

// writeSortedObject writes a JSON object with keys sorted in
// lexicographic order, matching JS's natural string-key iteration order
// for the kinds of keys Miruro uses (alphanumeric ASCII).
func writeSortedObject(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m[k])
		if err != nil {
			return err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return nil
}

// BuildSecurePipeURL assembles the full upstream URL for a GET request
// against `endpoint` with optional `query` parameters. The `host`
// argument lets the caller route through `pro.ultracloud.cc` /
// `pru.ultracloud.cc` failover hosts, but the default host
// (DefaultMiruroHost) is what the live SPA uses.
//
// Returns the fully-qualified URL or an error if validation fails.
func BuildSecurePipeURL(host, endpoint string, query map[string]any) (string, error) {
	if host == "" {
		host = DefaultMiruroHost
	}
	e, err := transformGet(endpoint, "GET", query, nil)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(host)
	if err != nil {
		return "", fmt.Errorf("miruro: parse host: %w", err)
	}
	u.Path = SecurePipePath
	q := u.Query()
	q.Set("e", e)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// DecodeObfuscatedResponse converts Miruro's secure-pipe response body
// into the underlying JSON bytes. The `xObfuscated` argument is the
// raw value of the `x-obfuscated` response header. `pipeKey` is the
// hex-decoded VITE_PIPE_OBF_KEY (16 bytes); pass nil when the header is
// "" or "1" since the key is only consumed for "2".
//
// Behavior:
//   - "" (no x-obfuscated header): body is plain JSON; returned verbatim.
//   - "1": body = base64url(gzip(json)). Decode + gunzip.
//   - "2": body = base64url(xor_cycle(gzip(json), pipeKey)). Decode +
//          XOR with the cycling key + gunzip.
//
// The function caps the gunzip output at MaxDecodedResponseBytes to
// defend against gzip-bomb DoS.
func DecodeObfuscatedResponse(body []byte, xObfuscated string, pipeKey []byte) ([]byte, error) {
	switch xObfuscated {
	case XObfuscatedNone:
		// Treat as plain JSON.
		return body, nil
	case XObfuscatedGzip:
		raw, err := decodeBase64URLLoose(body)
		if err != nil {
			return nil, fmt.Errorf("miruro: base64url decode: %w", err)
		}
		return gunzipCapped(raw)
	case XObfuscatedXorGz:
		if len(pipeKey) == 0 {
			return nil, ErrInvalidPipeKey
		}
		raw, err := decodeBase64URLLoose(body)
		if err != nil {
			return nil, fmt.Errorf("miruro: base64url decode: %w", err)
		}
		// XOR each byte with the cycling key.
		for i := range raw {
			raw[i] ^= pipeKey[i%len(pipeKey)]
		}
		return gunzipCapped(raw)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownObfuscation, xObfuscated)
	}
}

// decodeBase64URLLoose decodes base64url, tolerant of presence/absence
// of padding (the SPA strips padding; some intermediaries may add it).
// Also trims trailing whitespace/newlines that curl-style fetches leave.
func decodeBase64URLLoose(body []byte) ([]byte, error) {
	// Trim trailing newlines/CR/whitespace.
	s := strings.TrimRight(string(body), " \t\r\n")
	// Convert URL-safe alphabet to standard alphabet (RawURLEncoding
	// already does this, but the StdEncoding fallback below handles the
	// padded-input case).
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	// Try URL alphabet first (which is what the SPA produces).
	if out, err := base64.URLEncoding.DecodeString(s); err == nil {
		return out, nil
	}
	// Fall back to standard alphabet for resilience.
	return base64.StdEncoding.DecodeString(s)
}

// gunzipCapped gunzips `data` and returns the result, refusing to read
// more than MaxDecodedResponseBytes from the gzip stream.
func gunzipCapped(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("miruro: gzip header: %w", err)
	}
	defer gz.Close()
	// LimitReader returns EOF cleanly at the cap; we then check whether
	// the underlying stream is exhausted to distinguish "exactly at cap"
	// from "would have been larger."
	limited := io.LimitReader(gz, MaxDecodedResponseBytes+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("miruro: gzip decompress: %w", err)
	}
	if len(out) > MaxDecodedResponseBytes {
		return nil, ErrDecodedTooLarge
	}
	return out, nil
}

// DecodePipeKey hex-decodes the VITE_PIPE_OBF_KEY string into the 16-byte
// key the upstream uses for x-obfuscated: 2 XOR-cycling. Returns
// ErrInvalidPipeKey if hex-decode fails or the length is wrong.
func DecodePipeKey(hexKey string) ([]byte, error) {
	b, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPipeKey, err)
	}
	if len(b) != 16 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidPipeKey, len(b))
	}
	return b, nil
}
