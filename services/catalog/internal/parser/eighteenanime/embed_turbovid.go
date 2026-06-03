package eighteenanime

import (
	"fmt"
	"regexp"
)

var turbovidM3U8Re = regexp.MustCompile(`(https?://[^"'\s]+\.m3u8[^"'\s]*)`)

func extractTurbovid(html string) (*ExtractedSource, error) {
	m := turbovidM3U8Re.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("eighteenanime: turbovid m3u8 not found")
	}
	return &ExtractedSource{
		URL:     m[1],
		Referer: "", // turboviplay.com master + turbosplayer.com variants need no Referer (verified)
		IsHLS:   true,
		Quality: "FullHD",
	}, nil
}
