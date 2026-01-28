package animeparser

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// TitleVariants holds different representations of an anime title
type TitleVariants struct {
	Original   string   // Original title as provided
	Romaji     string   // Romanized Japanese
	English    string   // English title
	Japanese   string   // Japanese (kanji/hiragana/katakana)
	Synonyms   []string // Alternative titles
	Normalized string   // Normalized for search/matching
}

// NormalizeTitle creates a normalized version of a title for matching
func NormalizeTitle(title string) string {
	// Convert to lowercase
	result := strings.ToLower(title)

	// Remove diacritics and special characters
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ = transform.String(t, result)

	// Remove common suffixes/prefixes that vary between sources
	suffixes := []string{
		" (tv)", " tv", " (ova)", " ova", " (ona)", " ona",
		" (movie)", " movie", " the movie", " the animation",
		" season 1", " season 2", " season 3", " 1st season", " 2nd season", " 3rd season",
		" part 1", " part 2", " part i", " part ii",
	}
	for _, suffix := range suffixes {
		result = strings.TrimSuffix(result, suffix)
	}

	// Remove punctuation and extra whitespace
	re := regexp.MustCompile(`[^\w\s]`)
	result = re.ReplaceAllString(result, " ")
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// IsJapanese checks if a string contains Japanese characters
func IsJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

// IsRomaji checks if a string is likely romanized Japanese
func IsRomaji(s string) bool {
	// Check for common romaji patterns
	romajiPatterns := []string{
		"ou", "uu", "aa", "ii", "ei",
		"chi", "shi", "tsu",
		"ka", "ki", "ku", "ke", "ko",
		"sa", "su", "se", "so",
		"ta", "te", "to",
		"na", "ni", "nu", "ne", "no",
		"ha", "hi", "fu", "he", "ho",
		"ma", "mi", "mu", "me", "mo",
		"ya", "yu", "yo",
		"ra", "ri", "ru", "re", "ro",
		"wa", "wo", "nn",
	}

	lower := strings.ToLower(s)
	matches := 0
	for _, pattern := range romajiPatterns {
		if strings.Contains(lower, pattern) {
			matches++
		}
	}

	// If multiple romaji patterns found, likely romanized Japanese
	return matches >= 2
}

// ExtractSeasonInfo extracts season information from a title
type SeasonInfo struct {
	BaseName   string
	Season     int
	Part       int
	Year       int
	SeasonName string // "Spring", "Summer", "Fall", "Winter"
}

var seasonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(.+?)\s*(?:season\s*)?(\d+)(?:st|nd|rd|th)?\s*season`),
	regexp.MustCompile(`(?i)(.+?)\s*(\d+)(?:st|nd|rd|th)?\s*season`),
	regexp.MustCompile(`(?i)(.+?)\s*s(\d+)`),
	regexp.MustCompile(`(?i)(.+?)\s*part\s*(\d+|i{1,3}|iv|v)`),
	regexp.MustCompile(`(?i)(.+?)\s*(\d{4})`),
}

// ExtractSeason extracts season information from a title
func ExtractSeason(title string) SeasonInfo {
	info := SeasonInfo{BaseName: title}

	for _, pattern := range seasonPatterns {
		if matches := pattern.FindStringSubmatch(title); len(matches) >= 3 {
			info.BaseName = strings.TrimSpace(matches[1])
			// Parse season/part number
			numStr := strings.ToLower(matches[2])
			switch numStr {
			case "i":
				info.Season = 1
			case "ii":
				info.Season = 2
			case "iii":
				info.Season = 3
			case "iv":
				info.Season = 4
			case "v":
				info.Season = 5
			default:
				// Try to parse as number
				var n int
				if _, err := Atoi(numStr, &n); err == nil {
					if n > 1900 && n < 2100 {
						info.Year = n
					} else {
						info.Season = n
					}
				}
			}
			break
		}
	}

	return info
}

// Atoi is a helper to parse string to int
func Atoi(s string, n *int) (string, error) {
	var i int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			i = i*10 + int(r-'0')
		}
	}
	*n = i
	return s, nil
}

// CalculateTitleSimilarity calculates similarity between two titles (0-1)
func CalculateTitleSimilarity(title1, title2 string) float64 {
	norm1 := NormalizeTitle(title1)
	norm2 := NormalizeTitle(title2)

	if norm1 == norm2 {
		return 1.0
	}

	// Calculate Levenshtein-based similarity
	distance := levenshteinDistance(norm1, norm2)
	maxLen := max(len(norm1), len(norm2))
	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	row := make([]int, len(s2)+1)
	for i := range row {
		row[i] = i
	}

	for i := 1; i <= len(s1); i++ {
		prev := i
		for j := 1; j <= len(s2); j++ {
			current := row[j-1]
			if s1[i-1] != s2[j-1] {
				current = min(min(row[j-1]+1, prev+1), row[j]+1)
			}
			row[j-1] = prev
			prev = current
		}
		row[len(s2)] = prev
	}

	return row[len(s2)]
}
