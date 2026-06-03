package eighteenanime

import (
	"fmt"
	"regexp"
)

// ExtractedSource is a resolved playable stream from an embed mirror.
// SHARED type — other extractors (turbovid) reuse it.
type ExtractedSource struct {
	URL     string // direct mp4 or m3u8 URL
	Referer string // Referer the proxy must inject ("" if none required)
	IsHLS   bool   // true => m3u8, false => progressive mp4
	Quality string // e.g. "FullHD"
}

var mp4uploadSrcRe = regexp.MustCompile(`src\s*:\s*"(https?://[^"]+\.mp4[^"]*)"`)

func extractMP4Upload(html string) (*ExtractedSource, error) {
	m := mp4uploadSrcRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("eighteenanime: mp4upload src not found")
	}
	return &ExtractedSource{
		URL:     m[1],
		Referer: "https://www.mp4upload.com/", // stream is 403 without this Referer (verified)
		IsHLS:   false,
		Quality: "FullHD",
	}, nil
}
