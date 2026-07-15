package main

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

// telegramEscaper neutralises Telegram Markdown control characters in dynamic
// content in one pass. Backticks are swapped for apostrophes (a backslash
// escape is not honoured inside code spans), the rest are backslash-escaped.
var telegramEscaper = strings.NewReplacer(
	"`", "'",
	"*", "\\*",
	"_", "\\_",
	"[", "\\[",
)

func escTelegram(s string) string {
	return telegramEscaper.Replace(s)
}

// truncateForTelegram caps s at 500 runes, delegating to telegram.TruncateRunes
// so this reply pipeline shares one rune-safe cut instead of risking a second,
// unsafe byte-slice truncation on the same Cyrillic-heavy error text.
func truncateForTelegram(s string) string {
	return telegram.TruncateRunes(s, 500)
}
