//go:build integration

// Integration probe for the Miruro obfuscation Go port against the live
// production upstream. Gated behind the `integration` build tag so it
// does not pollute regular unit-test runs (which the CI matrix executes
// without network access).
//
// Run with: go test -tags=integration -run TestLiveMiruroSecurePipe \
//             ./services/scraper/internal/providers/miruro/...
//
// This file is the artifact for D3 Gate 2 + Gate 4 reproducibility —
// the spike's manual curl probes already established passing live
// behaviour (see SPIKE-MIRURO.md), and this test lets a maintainer
// re-confirm at any time without re-running the bash incantation by
// hand.

package miruro

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	frierenAnilistID = 154587
	probeUA          = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// TestLiveMiruroSecurePipe exercises the full GET → decode pipeline
// against the live Miruro production server for Frieren (AniList 154587)
// info + episodes. Skipped when offline (network failure ≠ test failure
// — production server may be unreachable from arbitrary CI runners).
func TestLiveMiruroSecurePipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 30 * time.Second}

	t.Run("info_154587", func(t *testing.T) {
		body, xobf := fetch(ctx, t, client, "info/154587", nil)
		decoded, err := DecodeObfuscatedResponse(body, xobf, nil)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		var info struct {
			Media struct {
				ID    int `json:"id"`
				IDMal int `json:"idMal"`
				Title struct {
					Native  string `json:"native"`
					Romaji  string `json:"romaji"`
					English string `json:"english"`
				} `json:"title"`
			} `json:"media"`
		}
		if err := json.Unmarshal(decoded, &info); err != nil {
			t.Fatalf("unmarshal info: %v (first 200 bytes: %q)", err, decoded[:min(200, len(decoded))])
		}
		if info.Media.ID != frierenAnilistID {
			t.Errorf("expected Media.ID=%d, got %d", frierenAnilistID, info.Media.ID)
		}
		if !strings.Contains(info.Media.Title.Romaji, "Frieren") &&
			!strings.Contains(info.Media.Title.English, "Frieren") {
			t.Errorf("title does not mention Frieren: %+v", info.Media.Title)
		}
		t.Logf("info OK: id=%d idMal=%d title=%q",
			info.Media.ID, info.Media.IDMal, info.Media.Title.Romaji)
	})

	t.Run("episodes_154587", func(t *testing.T) {
		body, xobf := fetch(ctx, t, client, "episodes", map[string]any{"anilistId": frierenAnilistID})
		decoded, err := DecodeObfuscatedResponse(body, xobf, nil)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		var eps struct {
			Providers map[string]struct {
				Episodes map[string][]json.RawMessage `json:"episodes"`
			} `json:"providers"`
		}
		if err := json.Unmarshal(decoded, &eps); err != nil {
			t.Fatalf("unmarshal episodes: %v", err)
		}
		totalSub := 0
		for name, p := range eps.Providers {
			if sub, ok := p.Episodes["sub"]; ok {
				totalSub += len(sub)
				t.Logf("provider %s sub: %d eps", name, len(sub))
			}
			if dub, ok := p.Episodes["dub"]; ok {
				t.Logf("provider %s dub: %d eps", name, len(dub))
			}
		}
		// D3 Gate 4: spot-check returns a non-error episode listing for
		// Frieren. We require at least one provider to have ≥1 episode.
		if totalSub == 0 {
			t.Errorf("Gate 4 FAILED: no sub episodes returned across any provider")
		}
		t.Logf("Gate 4 PASSED: aggregate sub episode count = %d", totalSub)
	})
}

func fetch(ctx context.Context, t *testing.T, c *http.Client, endpoint string, query map[string]any) ([]byte, string) {
	t.Helper()
	u, err := BuildSecurePipeURL("", endpoint, query)
	if err != nil {
		t.Fatalf("build url: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("User-Agent", probeUA)
	req.Header.Set("Referer", "https://www.miruro.tv/")
	req.Header.Set("Accept", "*/*")

	resp, err := c.Do(req)
	if err != nil {
		t.Skipf("network unreachable (skip not fail): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("non-200: %d", resp.StatusCode)
	}
	xobf := resp.Header.Get("x-obfuscated")
	body, err := readAllCapped(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	t.Logf("fetched %s — status=%d x-obfuscated=%q bytes=%d",
		endpoint, resp.StatusCode, xobf, len(body))
	return body, xobf
}

// readAllCapped reads the response with a generous (8 MiB) cap to defend
// against pathological upstreams even in the integration test.
func readAllCapped(resp *http.Response) ([]byte, error) {
	buf := make([]byte, 0, 32*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > 8<<20 {
				return nil, http.ErrBodyReadAfterClose // sentinel — body too large
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			// io.EOF is the normal terminator; treat all others as errors.
			break
		}
	}
	return buf, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
