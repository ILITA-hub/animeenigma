# Torrent / Library File Manager — Design

- **Date:** 2026-07-11
- **Status:** Approved (design) → ready for implementation plan
- **Owner request (feedback `2026-07-04T06-30-58_tNeymik_manual`):**
  > «Сделать файловый менеджер под торренты, скачать/удалить/добавить ручками и тп»
  > *Build a file manager for torrents — download / delete / add by hand, etc.*
- **Chosen shape (owner-selected):** a **raw file / bucket browser** — an operator surface
  that browses the actual files (torrent working dir + MinIO + S3), with add-by-hand,
  delete, and download.

---

## 1. Guiding principle (owner directive — LOCKED)

> **There is exactly ONE service responsible for file work: the `library` service.**
> Every file verb — browse / add / delete / download — is implemented there and routed
> through its existing file-owning layer, so a single owner can keep files consistent and
> manage their lifecycle (freshness auto-delete). The `storage` service (`:8099`) stays a
> **dumb internal object-I/O backend** the library calls via `storagegw`; it is never a
> second file API.

Why the library *is* that owner (verified in code):

- It is the **only** service that runs the torrent client and writes downloads to disk
  (`services/library/internal/torrent/client.go`; `LIBRARY_TORRENT_DOWNLOAD_DIR=/data/torrents/{infohash}/`).
- It **already owns the freshness / eviction lifecycle** —
  `services/library/internal/autocache/{evictor,planner,layout}.go`, demand/expiry, pool budget.
- It already holds the `storagegw` client that delegates raw object I/O (list / delete /
  download-URL) to the `storage` service.

**Corollary — reuse, do not rebuild.** Freshness-based auto-delete already exists (the
Evictor's `Sweep` + `Classify`). The file manager *surfaces* it and *reuses* its delete
path; it introduces no parallel delete or lifecycle logic.

---

## 2. What already exists (reused verbatim)

| Need | Existing API (library) | Notes |
|---|---|---|
| **Delete a stored episode** (DB-reconciled) | `autocache.Evictor.evictOne(ctx, ep)` → `objects.DeletePrefix(ep.Storage, ep.MinioPath)` **then** `pool.DeleteByID(ep.ID)` | Objects-first ordering so a serving pointer is never orphaned. The **only** delete path we call for episodes. |
| **Freshness classification** | `autocache.Classify(ep, cfg, now)` → `fresh` / `stale` | Age windows from `AutocacheConfig` (per-source download/fetch days). Pure fn. |
| **Freshness auto-delete (already running)** | `autocache.Evictor.Sweep(ctx)` on a ticker | Surfaced read-only in the UI; never reimplemented. |
| **Add a torrent by hand** | `POST /api/library/jobs {source:"manual", magnet, title, storage}` | Backend already validates magnet + enqueues. Only a UI form is missing. |
| **List objects** | `storagegw.Gateway.List(storage, prefix)` | Recursive `{key,size}` over MinIO/S3 (via storage `:8099`). |
| **Presigned download URL** | `storagegw.Gateway.URLFor(storage, path)` | Per-file download for object-store files. |
| **Episode inventory** | `repo.EpisodeRepository` / `episodesHandler.List` | The DB view of what's stored. |

## 3. What is genuinely new (small)

All additions live **in the `library` service** + its admin UI. Nothing new in `storage`.

1. **`Evictor.DeleteEpisodeByID(ctx, id)`** — a thin **public** wrapper that takes the
   Evictor mutex, loads the episode, and calls the existing `evictOne`. Reuses the exact
   reconciliation the sweep uses; adds mutex discipline so a manual delete can't
   double-spend budget vs. a concurrent sweep/admit.
2. **Torrent-working-dir file ops** — local-FS list + delete under
   `LIBRARY_TORRENT_DOWNLOAD_DIR`, path-jailed to that root. Delete **refuses (409) if an
   in-flight job still owns that infohash** — the operator cancels the job (existing
   `DELETE /jobs/{id}`, which owns torrent lifecycle) to stop it — otherwise it removes the
   dir/file. These files are pre-encode and have **no** `library_episodes` row, so no DB
   reconcile. (The torrent client exposes no drop-by-infohash API, so the file manager
   never reaches into torrent internals; it defers to the jobs path that owns them.)
3. **Library HTTP handlers** exposing the browser (`/api/library/files/*`, admin-gated) —
   see §5.
4. **Frontend "Files" section** inside `/admin/raw-library` (`RawLibrary.vue`) — tree +
   breadcrumb + per-row actions + a "paste magnet" add form. i18n en/ru/ja.

---

## 4. Three browsable domains (one tree)

| Domain | Backing store | List via | Delete via | Download via |
|---|---|---|---|---|
| **Torrent work dir** `/data/torrents/{infohash}/` | local disk (library container) | new library FS-list | new library FS-delete (refuse if infohash's job is in-flight) | stream through library endpoint |
| **MinIO** (`raw-library` bucket) | object store (local) | `storagegw.List("minio", prefix)` | episode → `DeleteEpisodeByID`; orphan → `storagegw.DeletePrefix` (guarded) | `storagegw.URLFor` presigned redirect |
| **S3 cloud** (`s3.firstvds.ru`) | object store (offloaded) | `storagegw.List("s3", prefix)` | same as MinIO | same as MinIO |

**Folder synthesis.** `storage /list` is recursive (no delimiter), returning flat
bucket-relative keys. The library handler groups keys by their next path segment under the
requested prefix to present folder-like navigation; the UI shows a breadcrumb + one level
at a time. Episode prefixes (those matching a `library_episodes.MinioPath`) are annotated
with the episode's `{shikimori_id, episode, source, freshness, size}`.

---

## 5. Backend — new library endpoints (admin-gated, under existing `/api/library`)

```
GET    /api/library/files?domain=work|minio|s3&prefix=<p>
         → { entries: [{ name, kind:"dir"|"file", size, key?,
                          episode?: {shikimori_id, episode, source, freshness} }],
             breadcrumb: [...] }

GET    /api/library/files/download?domain=work|minio|s3&key=<k>
         → 302 to presigned URL (minio/s3) | streamed bytes (work)

DELETE /api/library/files?domain=work|minio|s3&key=<k>
         → work: FS delete (409 if the infohash's job is still in-flight)
           minio/s3: if key maps to an episode → Evictor.DeleteEpisodeByID
                     else (orphan) → storagegw.DeletePrefix   [requires ?confirm=1]
```

`POST /api/library/jobs` (manual add) is **unchanged** — the UI just calls it.

All routes are admin-only (the whole `/api/library/*` block already is at the gateway).
Path inputs for `domain=work` are cleaned and jailed to `LIBRARY_TORRENT_DOWNLOAD_DIR`
(reject `..`, absolute, symlink escape).

---

## 6. Frontend — "Files" section in `RawLibrary.vue`

- New collapsible **Files** section (sits alongside the existing Search / Jobs / Failed /
  Pending-link sections — same operator page, one route: `/admin/raw-library`).
- **Domain switch:** `Work dir · MinIO · S3` segmented control.
- **Breadcrumb + tree** (one level at a time): folders first, then files; each row shows
  name, size, and — for object-store episode prefixes — a **fresh/stale** badge + episode
  label. Per-row actions: **Download**, **Delete**.
- **Add by hand:** a "Paste magnet" form (magnet + title + storage `minio|s3`) → existing
  `POST /api/library/jobs {source:"manual"}`. `.torrent`-file upload is a **fast-follow**
  (decode → magnet), explicitly out of the first cut.
- **Delete UX:** episode-folder delete confirms with the episode label; raw orphan-object
  delete requires a typed confirm (destructive, unreconciled). Work-dir delete warns if the
  torrent is still active.
- Design-system compliant (bind semantic tokens; run `/frontend-verify`; i18n en/ru/ja).

---

## 7. Delete safety (explicit)

- **Episode-mapped object prefixes** delete through `DeleteEpisodeByID` → same
  objects-first, DB-reconciled path as freshness auto-delete. No orphaned rows, no broken
  serving pointers.
- **Orphan objects** (in a bucket, no `library_episodes` row) may be prefix-deleted raw,
  behind an explicit `?confirm=1` + typed UI confirm. This is the only "dumb" delete and it
  cannot desync the DB (there is no row to desync).
- **Work-dir files** are pre-encode scratch — plain FS delete after dropping the torrent.

## 8. Non-goals (YAGNI)

- No new freshness/TTL logic — it exists; we reuse `Sweep`/`Classify`.
- No move/rename/copy in the first cut (storage supports it; not requested).
- No `.torrent`-file upload in the first cut (magnet paste covers "add by hand").
- No second file API in the `storage` service; no gateway route for `:8099`.
- No per-`.ts` single-file delete inside a healthy episode (would break HLS) — episode is
  the delete unit for object stores.

## 9. Testing

- **Go unit:** `DeleteEpisodeByID` reuses `evictOne` (mock objectDeleter + poolAccountant);
  work-dir path-jail rejects `..`/abs/symlink; folder-synthesis grouping; orphan-vs-episode
  delete routing; `confirm` gate.
- **Handler tests:** the three routes (auth, param validation, 302 vs stream).
- **FE:** vitest for the Files section (domain switch, breadcrumb nav, delete confirm
  gating); `/frontend-verify` (DS-lint + i18n parity + real build).
- **Manual smoke:** browse each domain, add a magnet, delete an episode (confirm DB row +
  objects gone), download a file.

## 10. Effort metrics (per `.planning/CONVENTIONS.md` — no days)

- **UXΔ = +2 (Better)** — a real hands-on file manager the operator asked for; consolidates
  scattered mental model into one surface.
- **CDI = 0.03 × 13** — small spread (library service + one Vue view), low shift (adds
  endpoints + a UI section; reuses evictor/jobs/storagegw), Effort_Fib 13.
- **MVQ = Griffin 85% / 80%** — composes existing parts into a clean operator tool; low
  slop risk because every mutation reuses an owned API.
