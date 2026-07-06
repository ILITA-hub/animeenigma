// Package allanimeokru is a single EN-sub domain.Provider (id "allanime-okru")
// that pairs AllAnime's GraphQL discovery with ok.ru stream resolution.
//
// Discovery (FindID / ListEpisodes / episodeSourceURLs) hits AllAnime's
// api.allanime.day GraphQL, which works from our datacenter egress. Streaming
// keeps ONLY the "Ok" (ok.ru) sources and resolves them with the ok.ru
// extractor (static data-options → okcdn.ru HLS), deliberately avoiding
// AllAnime's Cloudflare-Turnstile-walled /apivtwo/clock endpoint (unsolvable
// from our egress). EN sub/dub only — never raw.
//
// Folded 2026-07-06 from the former `okru` provider + `allanime` discovery
// package; AllAnime's own clock/probe stream path was dropped as dead code.
package allanimeokru
