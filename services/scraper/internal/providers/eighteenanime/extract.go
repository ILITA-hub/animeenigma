package eighteenanime

import (
	"fmt"
	"regexp"
)

// ExtractedSource is a resolved playable stream from an embed mirror.
type ExtractedSource struct {
	URL     string // direct mp4 or m3u8 URL
	Referer string // Referer the HLS proxy must inject ("" if none required)
	IsHLS   bool   // true => m3u8 (turbovid), false => progressive mp4 (mp4upload)
	Quality string // e.g. "FullHD"
}

// mp4uploadSrcRe matches the jwplayer object-literal form on the mp4upload embed
// page: player.src({type:"video/mp4",src:"https://aN.mp4upload.com:183/d/<tok>/video.mp4"}).
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

// turbovidM3U8Re matches the first .m3u8 URL in the turbovid jwplayer config.
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
