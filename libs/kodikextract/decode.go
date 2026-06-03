// Package kodikextract turns a Kodik iframe embed URL into the real HLS
// .m3u8 stream URLs, so the stream can be played ad-free in our own player.
package kodikextract

import (
	"encoding/base64"
	"strings"
)

const upperAlpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const lowerAlpha = "abcdefghijklmnopqrstuvwxyz"

// DecodeSrc decodes one Kodik `links[...].src` value into a stream URL.
//
// Kodik Caesar-shifts the base64 string (letters only; digits/symbols left
// as-is) before serving it. The shift varies per response, so we brute-force
// all 26 rotations and accept the candidate that base64-decodes to a string
// containing the "mp4:hls:manifest" marker.
func DecodeSrc(src string) (string, bool) {
	for rot := 0; rot < 26; rot++ {
		shifted := rotateLetters(src, rot)
		if pad := (4 - len(shifted)%4) % 4; pad > 0 {
			shifted += strings.Repeat("=", pad)
		}
		decoded, err := base64.StdEncoding.DecodeString(shifted)
		if err != nil {
			continue
		}
		out := string(decoded)
		if strings.Contains(out, "mp4:hls:manifest") {
			return out, true
		}
	}
	return "", false
}

func rotateLetters(s string, n int) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(upperAlpha[(int(c-'A')+n)%26])
		case c >= 'a' && c <= 'z':
			b.WriteByte(lowerAlpha[(int(c-'a')+n)%26])
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
