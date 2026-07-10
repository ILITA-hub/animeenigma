# Storage Service — single authority for user-content object storage

**Date:** 2026-07-10 · **Status:** approved by owner (chat, 2026-07-10)
**Origin:** feedback `2026-07-09T06-41-45_tNeymik_manual` — «Расширить сторадж на s3 (Там многа места)», expanded by owner direction: full S3 for auto/torrent ingests, MinIO reserved for admin manual uploads, admin destination choice, dual-presence allowed with a player server selector, and a single storage ingest service that controls all S3/MinIO usage for user content.

## Problem

- Host disk at 81% (94G free of 503G); local MinIO `raw-library` holds 50G and grows with every ingest.
- An external S3 (`s3.firstvds.ru`) with ample space sits unused except for DB backups.
- Benchmarked from the server: S3 TTFB ~180–200ms vs ~3ms local; sustained read ~59MB/s (~100× realtime for 1080p). Latency-only penalty (~+0.5–1s on startup/seek), streaming-safe.
- Two services (library, upscaler) carry copy-pasted MinIO writers with hardcoded buckets — adding a second backend per-service would multiply the debt.

## Decision

New Go microservice **`services/storage/`** (port **8099**, Docker-network-internal only, no gateway route) — the single authority for where user content lives and how it gets there. It owns both backends and all placement policy. No other service constructs a MinIO/S3 client for user content; the embedded writers in library and upscaler are deleted.

### Backends

| id | Endpoint | Bucket | Notes |
|---|---|---|---|
| `minio` | `minio:9000` (no SSL) | `raw-library` | local, fast (~3ms TTFB) |
| `s3` | `s3.firstvds.ru` (SSL) | `raw-library` (created 2026-07-10, `LocationConstraint=default`) | ample space; creds = existing `S3_ACCESS_KEY`/`S3_SECRET_KEY` |

Identical key layout on both (`aeProvider/<shikimoriID>/<TRACK>/<episode>/…`), so `storage id + path` fully addresses any object.

### Placement policy (service-owned, env-overridable)

| Content class | Default backend | Override |
|---|---|---|
| `library-auto` (autocache/auto-torrent jobs) | `s3` | none — always cloud |
| `library-manual` (admin RawLibrary jobs, batchingest) | `minio` | per-job admin choice (`s3`\|`minio`) |
| `upscaled` (upscaler output tracks) | `s3` | none |
| Phase 3: `gacha-cards`, `streaming-upload`, avatars | TBD at phase 3 | — |

### API (all `/internal/storage/*`, Docker-net-only)

- `POST /internal/storage/ingest-urls` — `{class, prefix, files[], override?}` → `{storage, urls: [{name, put_url}]}`. Batch of presigned PUT URLs; consumers upload with plain stdlib HTTP. Upload ordering semantics (segments first, `playlist.m3u8` last) stay with the caller.
- `POST /internal/storage/move` — `{storage, from_prefix, to_prefix}` server-side prefix move (the `pending/<job>` → linked flow).
- `DELETE /internal/storage/prefix` — `{storage, prefix}` for eviction/cleanup.
- `GET /internal/storage/resolve?storage=&path=` — canonical URL builder; the only place that knows `http://minio:9000/raw-library/...` vs `https://s3.firstvds.ru/raw-library/...`.
- `GET /internal/storage/health` — probes both backends.

**Deliberately stateless** — no object-registry DB. The owning service records the returned `storage` id in its domain row. One source of truth per object; a registry can be added behind the same API later if quotas/global inventory are ever needed.

**Rejected alternatives:** MinIO ILM tiering (transparent, but no admin choice and no player server dimension); full data-plane service with bytes streamed through it (double bandwidth on-box, staging-volume coupling across containers — presigned-PUT handout centralizes equally with the service staying tiny); per-service dual writers (multiplies the existing copy-paste debt).

## Reads: unchanged fast path

Player → streaming HLS proxy → backend. `libs/videoutils` presign seam becomes **multi-storage**: a `Storage` per backend, upstream signer picks by URL host, S3 host joins first-party hosts. Per-segment presigning is too hot for an HTTP hop to another service — the storage service governs writes/moves/deletes/resolution, not the read data plane. Catalog HMAC-signs S3 URLs exactly as MinIO URLs today (signed ⇒ proxy-trusted, no allowlist entry).

## Data model (library DB)

- `library_episodes` + `storage text NOT NULL DEFAULT 'minio'`; unique constraint becomes `(shikimori_id, episode_number, storage)` — dual presence allowed, never twice in one backend. Hand-written SQL migration (GORM won't alter constraints).
- `library_jobs` + `storage` — fixed at job creation (autocache ⇒ `s3`; manual ⇒ admin pick, default `minio`).

## Catalog — "check both places"

- ae episode enumeration = **union** across storages (episode counts, `partial_library`, capabilities see everything; capability wire model unchanged — server surfaces at stream-resolve time, consistent with the EN chain).
- `GET /api/anime/{id}/ae/stream` gains `?server=minio|s3`. Episodes present in both return `servers: [{id:'minio',label:'Local'},{id:'s3',label:'Cloud'}]`; **default = Local** when both exist. Single-copy episodes return no server list.

## Frontend

- **aePlayer:** ae adapter forwards `combo.server`, surfaces `stream.servers` → existing Source-panel Server section, WT combo token, and auto-failover server-dodge light up for ae without new mechanisms (all already generic).
- **Admin `RawLibrary.vue`:** destination picker at job creation (default MinIO; auto jobs ignore it), storage badge on job/episode rows.
- `library-batchingest` gains `-storage` flag (default `minio`).

## Migration (one-time, via `services/storage/cmd/storage-migrate`)

29 `source='autocache'` episodes (~24GB): copy prefix MinIO→S3, verify object count+bytes, flip row to `storage='s3'`, delete local prefix. ~1–1.5h at measured throughput. The 79 admin episodes (~25GB) stay local. Phase 2 migrates existing `UPSCALED-*` prefixes identically.

## Operational notes

- Torrent download dirs and ffmpeg staging remain plain local filesystem (transient, not stored user content). DiskGuard unchanged — staging is local regardless of destination.
- Pool eviction applies only to `storage='minio'` episodes; S3 content is never auto-evicted.
- Env: storage service gets `STORAGE_MINIO_*` + `STORAGE_S3_*` (S3 side defaults from existing `S3_*` vars in `docker/.env`).

## Phasing

1. **Phase 1:** `services/storage` + library refactor + catalog/streaming/player server dimension + autocache migration.
2. **Phase 2:** upscaler onto the service (writer deleted, `upscaled` class ⇒ S3) + UPSCALED backfill migration.
3. **Phase 3 (separate effort):** gacha-cards, legacy streaming `POST /upload`, avatars — the "everything via the storage service" end state.

## Success criteria

- New auto/torrent ingests land on `s3.firstvds.ru` and play through aePlayer.
- Admin can pick MinIO or S3 per manual job; dual-present episodes show a Local/Cloud server choice in the player.
- Library and upscaler contain no MinIO client code; all writes flow through `services/storage`.
- ~24GB freed from local disk after migration; migrated titles verified playing from S3.
