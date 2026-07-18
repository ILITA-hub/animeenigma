package kage

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/nwaples/rardecode"
)

// subtitleEntry is one subtitle file found inside a downloaded archive.
type subtitleEntry struct {
	Name string
	Body []byte
}

// ExtractEpisode picks the subtitle file for the requested episode out of a
// Kage download. The payload is sniffed by magic bytes: RAR and ZIP archives
// are walked for subtitle entries; anything else is treated as a bare
// subtitle file (single-episode releases). filename is the upstream
// Content-Disposition name, used only for format sniffing of bare files.
//
// Selection: a single subtitle entry wins outright (movies, single-episode
// releases); otherwise the entry whose filename carries the episode number
// wins. The returned body is normalized to BOM-less UTF-8 (fansub archives
// mix UTF-8 and windows-1251).
func ExtractEpisode(payload []byte, filename string, episode int) ([]byte, string, error) {
	entries, err := listSubtitleEntries(payload, filename)
	if err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		return nil, "", fmt.Errorf("kage: no subtitle files in %q", filename)
	}

	var pick *subtitleEntry
	if len(entries) == 1 {
		pick = &entries[0]
	} else {
		for i := range entries {
			if episodeNumberFromName(entries[i].Name) == episode {
				pick = &entries[i]
				break
			}
		}
	}
	if pick == nil {
		return nil, "", fmt.Errorf("kage: episode %d not found among %d subtitle files in %q", episode, len(entries), filename)
	}

	body := normalizeSubtitleText(pick.Body)
	if !isLikelySubtitle(body) {
		return nil, "", fmt.Errorf("kage: %q does not look like a subtitle file", pick.Name)
	}
	return body, formatFromExt(pick.Name), nil
}

var (
	magicRAR = []byte("Rar!\x1a\x07") // covers v4 (…\x00) and v5 (…\x01\x00)
	magicZIP = []byte("PK\x03\x04")
)

// listSubtitleEntries walks the payload and returns every subtitle-typed file.
func listSubtitleEntries(payload []byte, filename string) ([]subtitleEntry, error) {
	switch {
	case bytes.HasPrefix(payload, magicRAR):
		return listRAR(payload)
	case bytes.HasPrefix(payload, magicZIP):
		return listZIP(payload)
	default:
		// Bare subtitle file (no archive wrapper).
		name := filename
		if name == "" {
			name = "subtitle.ass"
		}
		return []subtitleEntry{{Name: name, Body: payload}}, nil
	}
}

func listZIP(payload []byte) ([]subtitleEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("kage: open zip: %w", err)
	}
	var out []subtitleEntry
	for _, f := range zr.File {
		name := decodeArchiveName(f.Name)
		if !isSubtitleExt(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxResponseBytes))
		rc.Close()
		if err != nil {
			continue
		}
		out = append(out, subtitleEntry{Name: name, Body: body})
	}
	return out, nil
}

// listRAR walks a RAR archive sequentially (solid archives — the common Kage
// case — can only be read front to back, so every entry is materialized).
func listRAR(payload []byte) ([]subtitleEntry, error) {
	rr, err := rardecode.NewReader(bytes.NewReader(payload), "")
	if err != nil {
		return nil, fmt.Errorf("kage: open rar: %w", err)
	}
	var out []subtitleEntry
	for {
		h, err := rr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			// Partial archives still yield what was read so far.
			return out, nil
		}
		name := decodeArchiveName(h.Name)
		if h.IsDir || !isSubtitleExt(name) {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rr, maxResponseBytes))
		if err != nil {
			continue
		}
		out = append(out, subtitleEntry{Name: name, Body: body})
	}
}

// isSubtitleExt reports whether a filename carries a subtitle extension the
// frontend SubtitleOverlay can parse.
func isSubtitleExt(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".ass", ".ssa", ".srt", ".vtt":
		return true
	}
	return false
}

func formatFromExt(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	if ext == "ssa" {
		return "ass" // same parser family on the frontend
	}
	return ext
}

// decodeArchiveName fixes windows-1251 filenames stored without a UTF-8 flag
// (common in RU-authored archives).
func decodeArchiveName(name string) string {
	return string(cp1251ToUTF8([]byte(name)))
}

var (
	// reBracketGroup strips bracketed release tags: "[SubsPlease]", "(720p)",
	// "[820FC793]" — their digits must not be mistaken for episode numbers.
	reBracketGroup = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)
	// reDashEpisode matches the dominant fansub convention " - NN", tolerating
	// a version suffix (" - 01v2").
	reDashEpisode = regexp.MustCompile(`-\s*(\d{1,4})(?:v\d+)?\b`)
	// reSxxEyy matches the western "S01E07" convention.
	reSxxEyy = regexp.MustCompile(`(?i)s\d{1,2}e(\d{1,4})`)
	// reAnyNumber is the last-resort standalone number token (same v-suffix
	// tolerance).
	reAnyNumber = regexp.MustCompile(`\b(\d{1,4})(?:v\d+)?\b`)
	// reResolutionNoise removes resolution/codec tokens that survive bracket
	// stripping ("720p", "x264", "10bit").
	reResolutionNoise = regexp.MustCompile(`(?i)\b\d{3,4}p\b|\bx26[45]\b|\b10bit\b|\b8bit\b`)
)

// episodeNumberFromName extracts the most plausible episode number from a
// subtitle filename, or 0 when none is found. Bracketed groups and
// resolution tokens are stripped first; the " - NN" convention wins, with the
// LAST standalone number as fallback ("Gundam 00 - 05.ass" → 5).
func episodeNumberFromName(name string) int {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	base = reBracketGroup.ReplaceAllString(base, " ")
	base = reResolutionNoise.ReplaceAllString(base, " ")

	if ms := reDashEpisode.FindAllStringSubmatch(base, -1); len(ms) > 0 {
		if n, _ := strconv.Atoi(ms[len(ms)-1][1]); n > 0 {
			return n
		}
	}
	if m := reSxxEyy.FindStringSubmatch(base); m != nil {
		if n, _ := strconv.Atoi(m[1]); n > 0 {
			return n
		}
	}
	if ms := reAnyNumber.FindAllStringSubmatch(base, -1); len(ms) > 0 {
		if n, _ := strconv.Atoi(ms[len(ms)-1][1]); n > 0 {
			return n
		}
	}
	return 0
}

// normalizeSubtitleText converts the body to UTF-8 (fansub files are a mix of
// UTF-8-with-BOM and windows-1251) and strips the BOM.
func normalizeSubtitleText(b []byte) []byte {
	return cp1251ToUTF8(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}))
}

// isLikelySubtitle is a cheap structural check so an HTML error page or a
// stray README never reaches the player as a "subtitle".
func isLikelySubtitle(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "[Script Info]") || // ASS/SSA
		strings.Contains(s, "WEBVTT") || // VTT
		strings.Contains(s, "-->") // SRT/VTT cue timing
}
