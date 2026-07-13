# Raw-Library File Manager — UX Overhaul (①) — Design

- **Date:** 2026-07-13
- **Status:** Approved (design) → ready for implementation plan
- **Baseline it evolves:** [`2026-07-11-torrent-file-manager-design.md`](./2026-07-11-torrent-file-manager-design.md)
  (which shipped the current `/admin/raw-library` Files panel). That spec's **LOCKED**
  guiding principle still holds verbatim: *the `library` service is the ONE owner of every
  file verb; `storage` (`:8099`) stays a dumb internal object-I/O backend.* Nothing here
  changes that.
- **Owner request (this round):**
  > In `/admin/raw-library`: (1) a distinct tab for the file manager (Torrent Client /
  > File Manager); (2) show *what anime* a numeric folder is (title from DB); (3) AWS-S3-like
  > affordances — a `../` parent entry + URL wiring
  > `…/file-manager/<backend>/<full-path>`; (4) remove "Add torrent by hand"; (5) an
  > "Upload file/folder" button.

## 0. Scope split (owner-approved sequencing: ① → ③ → ②)

The five asks decompose into three projects of very different size/risk. This spec is **①
only**; ② and ③ get their own spec→plan→build cycles later.

- **① File Manager UX (THIS SPEC)** — tabs, URL wiring, `../`, anime-title transcript,
  remove "add by hand". **Frontend-only** (+ one trivial API-client line). No
  library/storage/catalog backend change.
- **③ Orphan 24h auto-delete GC (next)** — backend scheduled sweep of objects linked to no
  episode, older than 24h; reuses the Evictor/`Classify` machinery from the baseline spec.
  **The orphan *highlight* lives here**, not in ①, because it shares ③'s linked-vs-unlinked
  detection.
- **② Upload → decode → subs → wire (last)** — streaming upload endpoint + reuse of the
  encode→HLS→register-episode pipeline for a non-torrent source + subtitle ingestion +
  anime-link UI. Backend-heavy. **The functional "Upload file/folder" button ships in ②**;
  ① deliberately does NOT ship a dead/disabled placeholder.

---

## 1. Problem

`RawLibrary.vue` is a single ~860-line view that stacks four unrelated sections (stats,
search, jobs, files) on one scrolling page at one route (`/admin/raw-library`). The Files
panel has no deep-linking (browse state is lost on reload/share), no `../` affordance, and
the numeric object-store folders (`aeProvider/<id>/`) are opaque — the operator can't tell
`aeProvider/11981/` from `aeProvider/4901/` without cross-referencing another page. The
"Add torrent by hand" magnet form (baseline §6) is redundant with the search→Queue flow and
the owner wants it gone.

## 2. Design

### 2.1 Page → two tabs (reuse DS `Tabs.vue`)

Split the monolith into a thin tab host + two focused panels, reusing
`components/ui/Tabs.vue` (`variant="underline"`, the `AdminPolicy.vue` pattern). `Tabs.vue`
mounts only the active panel's slot.

```
views/admin/RawLibrary.vue                 ← tab host: header + <Tabs> + route-synced activeTab
views/admin/rawlibrary/TorrentClient.vue   ← today's stats + search + jobs sections
views/admin/rawlibrary/FileManager.vue     ← today's Files panel (② upload lands here)
views/admin/rawlibrary/format.ts           ← shared formatBytes/formatGB/formatPct helpers
```

- **Torrent Client** tab = default; **File Manager** tab = the browser.
- **Accepted behaviour change:** because only the active panel is mounted, the jobs/stats
  polling (5s/30s intervals) **pauses while the File Manager tab is active** and resumes on
  return. Acceptable for an admin surface; documented so it isn't mistaken for a bug.
- The split is also a health win: it keeps ②'s upload additions in a focused `FileManager.vue`
  instead of growing the monolith.

### 2.2 Routing / URL wiring

Two route records, **both** rendering `RawLibrary.vue`; the host derives the active tab from
the matched route name.

| URL | Tab | Restored on reload/share |
|---|---|---|
| `/admin/raw-library` (`admin-raw-library`) | Torrent Client (default) | — |
| `/admin/raw-library/file-manager/:backend/:path(.*)?` (`admin-raw-library-files`) | File Manager | backend + browsed folder |

- `:backend` ∈ `work | minio | s3` (the API's existing `domain` param — single source of
  truth). `:path` is a **catch-all capturing slashes** (e.g. `aeProvider/11981/RAW/1/`).
- Folder navigation, backend switch, and the `../` row all `router.push` new URLs, so
  browser Back/Forward walk history naturally. The panel watches route params → `loadFiles`.
- **Bare / malformed:** `/admin/raw-library/file-manager` with no backend normalizes to
  `minio` at root (redirect-in-place via `router.replace`). Unknown backend → `minio` root.
- **`s3` vs `firstvds` (resolved):** URL segment stays `s3` (matches the API param); the UI
  *labels* it **"S3 · firstvds"**. No `firstvds` alias in the path — there is exactly one S3
  backend, and coupling the URL to a host name would rot if the target moves.

### 2.3 File Manager additions

1. **`../` parent row** — a synthetic first entry rendered whenever `prefix !== ''` (AWS-S3
   console style), navigating up one path segment. Complements (does not replace) the
   existing breadcrumb. Pure frontend.
2. **Anime-title "transcript"** — for numeric folders directly under `aeProvider/` (the
   folder name **is** the Shikimori ID; confirmed via `RawPrefix(job.ShikimoriID, ep)` →
   `aeProvider/<ShikimoriID>/RAW/<ep>/`), render `11981 — Frieren: Beyond Journey's End`.
   - Resolve via the existing `GET /api/anime/shikimori/{id}` (new one-line
     `animeApi.resolveShikimori(id)` in `api/client.ts`).
   - A **module-level `Map` cache** (persists across panel remounts within the session) +
     capped-parallel fetch, fired only for numeric-named dirs at the *current* level.
   - Title text = `name || name_ru || id` (mirrors the existing pending-link display).
     Graceful: a 404 / miss shows the bare id, never blocks the listing.
   - **Zero backend change.** (If per-folder chattiness ever bites, a batch
     `by-shikimori` endpoint is a later optimization — explicitly out of scope now.)

### 2.4 Removals & deferrals

- **Remove "Add torrent by hand" entirely** — delete the magnet `<form>`, the
  `magnet`/`magnetTitle`/`magnetStorage` refs, `addMagnet()`, and the
  `player.adminLibrary.files.add.*` i18n keys (en/ru/ja). This **supersedes baseline §6/§8's**
  "magnet paste covers add-by-hand". The `source:"manual"` job path stays valid on the
  backend; only the FE form is removed. Search→Queue remains the sole add path.
- **Upload button → deferred to ②** (no dead placeholder). FileManager toolbar is laid out
  to accept it.
- **Orphan highlight → deferred to ③** (shares ③'s detection). ① stays frontend-only.

## 3. i18n (en/ru/ja parity — required)

- **Add:** tab labels (`torrentClient`, `fileManager`), `../` aria/label, transcript
  separator/format, "S3 · firstvds" backend label.
- **Remove:** `player.adminLibrary.files.add.*`.

## 4. Testing

- Move/refit `views/__tests__/RawLibrary.files.spec.ts` → target `FileManager.vue`.
- New specs: route-sync (URL ↔ active tab ↔ browsed folder), `../` navigation (present only
  when not at root; goes up one segment), transcript cache (numeric-dir resolve + dedupe +
  graceful 404), and that "add by hand" is gone.
- `/frontend-verify` (DS-lint + i18n en/ru/ja parity + real `bun run build`) before commit.
- Manual smoke (opt-in, owner's call): deep-link a folder, reload restores it; `../` walks
  up; a numeric folder shows its title; Back/Forward work.

## 5. Process notes

- **Prototype pass (resolved):** straight-to-Vue. This is multi-component *rework* but low
  visual novelty — it reuses existing DS primitives (glass-card, table, `Tabs.vue`,
  breadcrumb). The `design-prototyping` sandbox is available if the tabbed layout wants a
  look first, but not planned.
- Golden-rule compliance: all work in the `feat/raw-library-file-manager-ux` worktree;
  land via `bin/ae-land.sh`; `/animeenigma-after-update` after push.

## 6. Non-goals (this spec)

- No backend changes (no library/storage/catalog edits, no new/changed endpoints).
- No upload (②), no orphan highlight or GC (③), no move/rename/copy.
- No batch title-resolution endpoint (per-folder cached resolve is sufficient for ①).
- No change to delete/download semantics or the baseline's locked delete-safety rules.

## 7. Effort metrics (per `.planning/CONVENTIONS.md` — no days)

- **UXΔ = +2 (Better)** — deep-linkable, self-explaining file manager; removes clutter;
  numeric folders become legible. Squarely an operator-quality-of-life win.
- **CDI = 0.03 × 8** — small spread (one Vue view split into 3 + router + 1 API-client
  line; no backend), low shift (behaviour-preserving relocation + two additive affordances +
  one removal), Effort_Fib 8.
- **MVQ = Griffin 85% / 80%** — methodically composes existing DS + routing patterns into a
  cleaner surface; low slop risk (no new backend contract, reuses `Tabs.vue` / existing
  `/api/library/files` + `/api/anime/shikimori/{id}`).
