// Package okru is a scraper provider that serves AllAnime's "Ok" (ok.ru)
// sources WITHOUT touching AllAnime's Cloudflare-Turnstile-walled
// /apivtwo/clock endpoint.
//
// Discovery (FindID / ListEpisodes) is delegated to an internal allanime
// provider — the api.allanime.day GraphQL works fine from our egress; only the
// clock leg is walled. For ListServers / GetStream, okru reads the episode's
// source list via allanime.Provider.EpisodeSourceURLs, keeps ONLY the "Ok"
// sources (ok.ru/videoembed/<id>), and resolves them with the ok.ru extractor
// (static data-options → okcdn.ru HLS). EN sub/dub only — NEVER raw (raw =
// JP-audio-no-burned-subs, served library-only by the catalog Raw resolver).
package okru
