# 07-03 Catalog ae-resolver Audit — minio_path repoint transparency

**Phase:** 07-pool-foundation-config-migration · **Plan:** 03 · **Date:** 2026-06-17
**Requirement:** POOL-02 (admin-content migration without breaking playback)
**Spec refs:** §3.3 (one-time migration), §10 (sequencing invariant), 07-CONTEXT lines 75-77,
STATE.md "Migration audit" risk.

## Purpose

The one-time migrator (Task 2) repoints each admin episode's `library_episodes.minio_path`
from the legacy `{shikimori_id}/{ep}/` prefix to `aeProvider/<mal>/RAW/<ep>/`. This audit
confirms the catalog "ae" / raw resolution path builds the served stream URL **only from the
per-row `minio_path` / `minio_url`** — with no hardcoded MinIO object-path construction — so
the repoint is transparent end-to-end and **zero catalog edits are required**.

If a hardcoded prefix had been found it would have been fixed to consume `minio_url`; none was
found, so the deliverable is the documented confirmation.

## Files audited (read-only)

### 1. `services/catalog/internal/parser/library/client.go` — **CLEAN**

Thin HTTP client for the library service. It builds **request URLs only**, never MinIO object
paths, and reads the served URL verbatim from the `{success,data}` envelope:

- `EpisodeResponse.MinIOURL string \`json:"minio_url"\`` (line 52) — the served URL is decoded
  from the library envelope, not constructed.
- `GetEpisode` returns `env.Data` after asserting `env.Data.MinIOURL != ""` (lines 141-151).
- `ListEpisodes` returns `env.Data.Episodes`, each carrying `MinIOURL` from the envelope
  (`EpisodeListItem.MinIOURL`, line 68; decode lines 190-195).
- The only path assembly is the **request path** `"%s/api/library/episodes/%s/%s"` (line 120)
  and `"%s/api/library/episodes/%s"` (line 175) — an HTTP route, NOT a MinIO object key.
  These are explicitly acceptable: they address the library API, not the bucket, and are
  unaffected by where `minio_path` points.

**Verdict: CLEAN** — served URL comes solely from `minio_url`; repointing `minio_path` changes
the served URL transparently.

### 2. `services/catalog/internal/service/raw_resolver.go` — **CLEAN**

The first-party ("ae") raw resolution path. The served URL is consumed verbatim:

- `newLibraryStream(minioURL, quality string) *RawStream` (line 309) sets `URL: minioURL`
  (line 312) and signs that exact value via `streamsign.Sign(minioURL)` (line 310). No prefix
  is prepended, stripped, or reconstructed — `minioURL` is whatever the library client returned
  (`EpisodeResponse.MinIOURL` / `EpisodeListItem.MinIOURL`).

**Verdict: CLEAN** — no MinIO object-path construction; `minio_path` repoint is transparent.

## Supporting fact (library side)

`services/library/internal/handler/episodes.go` builds the public URL as
`h.urlBuilder.URLFor(ep.MinioPath + "playlist.m3u8")` (lines 92, 133) — i.e. **directly from the
per-row `minio_path`**. After the migrator repoints `minio_path` to `aeProvider/<mal>/RAW/<ep>/`,
the library emits `aeProvider/.../playlist.m3u8` automatically, and the catalog forwards that
`minio_url` unchanged. The repoint is therefore transparent across both services.

## Grep evidence

```
$ cd services/catalog
$ grep -rn 'aeProvider/' internal/service/raw_resolver.go internal/parser/library/client.go
  (no matches — exit 1)

$ grep -rnE '%s/%d/|%s/%s/|/RAW/|MinioPath|minio_path' \
      internal/service/raw_resolver.go internal/parser/library/client.go
  (no matches — exit 1)   # no MinIO object-path construction in either file

$ grep -rn '/api/library/episodes' internal/parser/library/client.go
  client.go:120:  u := fmt.Sprintf("%s/api/library/episodes/%s/%s", ...)   # request URL — acceptable
  client.go:175:  u := fmt.Sprintf("%s/api/library/episodes/%s", ...)      # request URL — acceptable
```

No hardcoded legacy `{shikimori_id}/{ep}/` prefix and no MinIO object-key assembly exist in
either catalog file. The only path construction is the library **request URL**, which is not an
object path and is unaffected by the repoint.

## Conclusion

Both catalog ae-resolver files are **CLEAN**. The served stream URL is built solely from the
per-row `minio_url` (decoded from the library `{success,data}` envelope, itself derived from
`library_episodes.minio_path`). The Task-2 migrator's `minio_path` repoint is transparent with
**no catalog edits required**. The Phase-10 evictor can rely on this: relocating admin content
into the metered pool does not break catalog raw playback.
