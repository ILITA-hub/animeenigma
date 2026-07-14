package handler

import "strings"

// providerRoster is the injected roster membership check (roster.Client.Known).
// Set at construction; nil-safe for tests that don't care (falls back to the
// synthetic set only).
type providerRoster interface{ Known(name string) bool }

// syntheticProviders are player-surface ids that are NOT stream_providers rows
// but legitimately appear as combo.provider on player events:
//   - "kodik": the capability/FE id for the kodik-noads row (the alias lives in
//     catalog capability assembly; analytics accepts both spellings).
//   - "offline": the PWA offline-downloads synthetic provider.
var syntheticProviders = map[string]struct{}{"kodik": {}, "offline": {}}

// whitelistProvider returns the canonical (lowercased) provider key when it is
// a roster row or a known synthetic, else "". Player-telemetry provider becomes
// the source-ranking GROUP BY target, so an unwhitelisted value injects
// arbitrary rows into the ranking aggregates (audit medium #2). AUTO-608: the
// roster is fetched live from catalog (DB = source of truth), so a new
// provider's telemetry records without a code change here.
func whitelistProvider(s string, roster providerRoster) string {
	k := strings.ToLower(strings.TrimSpace(s))
	if _, ok := syntheticProviders[k]; ok {
		return k
	}
	if roster != nil && roster.Known(k) {
		return k
	}
	return ""
}
