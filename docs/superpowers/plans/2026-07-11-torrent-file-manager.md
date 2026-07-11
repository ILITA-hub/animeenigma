# Torrent / Library File Manager — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the operator a file-manager surface in `/admin/raw-library` to browse the torrent working dir + MinIO + S3, add a torrent by hand, delete stored content, and download files — with every mutation routed through the library service's already-owned APIs.

**Architecture:** The `library` service is the single file authority. New admin endpoints under `/api/library/files/*` reuse the existing Evictor (episode delete), jobs pipeline (manual add), and `storagegw` (object list/URL/delete). A new local-FS helper handles the torrent working dir. The `storage` service (`:8099`) stays a dumb object backend the library calls. One new "Files" section in `RawLibrary.vue` consumes it.

**Tech Stack:** Go (chi, GORM, `anacrolix/torrent/metainfo`), `libs/httputil`, `libs/logger`, `libs/storageclient`; Vue 3 + TypeScript, `@/api/client` (axios), vue-i18n, shadcn-vue UI components.

## Global Constraints

- **Single file authority = library service.** No file logic in `storage`; no gateway route for `:8099`. (Design §1.)
- **Reuse, never rebuild:** episode delete → `autocache.Evictor` (`evictOne`); freshness → `autocache.Classify`/`Sweep`; manual add → `POST /api/library/jobs {source:"manual"}`; object I/O → `storagegw.Gateway`.
- **All `/api/library/*` routes are admin-gated at the gateway** (`services/gateway/internal/transport/router.go` `r.Route("/library")` → JWT + `AdminRoleMiddleware`). No extra server-side gate needed; `/files/*` inherits it.
- **Work-dir path input MUST be jailed** to `LIBRARY_TORRENT_DOWNLOAD_DIR` (default `/data/torrents`): reject `..`, absolute paths, and symlink escapes.
- **Go formatting:** do NOT run `gofmt -w` / `make fmt` (smart-quote landmine — see project memory). Write already-formatted code; rely on `go build`/`go vet`.
- **Frontend:** bind semantic DS tokens only; i18n parity across `src/locales/{en,ru,ja}.json`; run `/frontend-verify` before finishing (DS-lint + i18n + real `bun run build`).
- **Commits:** every commit includes the three co-authors:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Work in the `feat/torrent-file-manager` worktree** at `/data/animeenigma/.claude/worktrees/torrent-file-manager` — never edit the base tree. Use worktree-absolute paths for every edit.

---

## File Structure

**Backend (library service):**
- Modify `services/library/internal/autocache/evictor.go` — add `GetByID` to `poolAccountant`; add public `DeleteEpisodeByID`.
- Create `services/library/internal/service/workdir.go` — jailed FS list/delete/resolve for the torrent working dir.
- Create `services/library/internal/service/active_torrents.go` — active-infohash set derived from in-flight jobs (work-dir delete guard).
- Modify `services/library/internal/storagegw/gateway.go` — add `DownloadURL` (single-key presigned GET, reuses storage `/download-urls`).
- Create `services/library/internal/handler/files.go` — `FilesHandler` (Browse / Download / Delete).
- Modify `services/library/internal/transport/router.go` — register `/files*` routes + new handler param.
- Modify `services/library/cmd/library-api/main.go` — construct + wire `FilesHandler`.

**Frontend (web):**
- Modify `frontend/web/src/api/client.ts` — `adminLibraryApi` browse/download/delete methods.
- Modify `frontend/web/src/types/library.ts` — file-manager DTO types.
- Modify `frontend/web/src/views/admin/RawLibrary.vue` — new "Files" section.
- Modify `frontend/web/src/locales/{en,ru,ja}.json` — `adminLibrary.files.*` keys.

---

### Task 1: `Evictor.DeleteEpisodeByID` — episode delete via the existing `evictOne`

**Files:**
- Modify: `services/library/internal/autocache/evictor.go`
- Test: `services/library/internal/autocache/evictor_test.go` (add cases)

**Interfaces:**
- Consumes: existing `evictOne(ctx, ep)`, `poolAccountant`, `e.mu`.
- Produces: `func (e *Evictor) DeleteEpisodeByID(ctx context.Context, id string) error` — loads the episode and evicts it (objects then row) under the Evictor mutex. Used by Task 6.

- [ ] **Step 1: Write the failing test**

Add to `services/library/internal/autocache/evictor_test.go`. Reuse the existing test doubles in that file if present; otherwise this defines minimal fakes:

```go
func TestDeleteEpisodeByID_ReusesEvictOne(t *testing.T) {
	ep := domain.Episode{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/"}
	pool := &fakePool{getByID: map[string]domain.Episode{"ep-1": ep}}
	objs := &fakeObjects{}
	e := NewEvictor(&fakeCfg{}, pool, nil, objs, nil, nil)

	if err := e.DeleteEpisodeByID(context.Background(), "ep-1"); err != nil {
		t.Fatalf("DeleteEpisodeByID: %v", err)
	}
	if objs.deleted != "frieren/s1/e01/" || objs.deletedStorage != "minio" {
		t.Fatalf("expected object prefix delete, got storage=%q prefix=%q", objs.deletedStorage, objs.deleted)
	}
	if pool.deletedID != "ep-1" {
		t.Fatalf("expected row delete for ep-1, got %q", pool.deletedID)
	}
	// objects-first ordering: row must not be deleted if object delete fails.
}

func TestDeleteEpisodeByID_ObjectFailLeavesRow(t *testing.T) {
	ep := domain.Episode{ID: "ep-1", Storage: "minio", MinioPath: "x/"}
	pool := &fakePool{getByID: map[string]domain.Episode{"ep-1": ep}}
	objs := &fakeObjects{failDelete: true}
	e := NewEvictor(&fakeCfg{}, pool, nil, objs, nil, nil)

	if err := e.DeleteEpisodeByID(context.Background(), "ep-1"); err == nil {
		t.Fatal("expected error when object delete fails")
	}
	if pool.deletedID != "" {
		t.Fatalf("row must survive when object delete fails, got deleted %q", pool.deletedID)
	}
}
```

If the file has no `fakePool`/`fakeObjects`/`fakeCfg`, add:

```go
type fakePool struct {
	getByID   map[string]domain.Episode
	deletedID string
}
func (f *fakePool) SumPoolBytes(context.Context) (int64, error) { return 0, nil }
func (f *fakePool) ListStaleEvictionCandidates(context.Context, *domain.AutocacheConfig, time.Time) ([]domain.Episode, error) { return nil, nil }
func (f *fakePool) ListPool(context.Context) ([]domain.Episode, error) { return nil, nil }
func (f *fakePool) DeleteByID(_ context.Context, id string) error { f.deletedID = id; return nil }
func (f *fakePool) GetByID(_ context.Context, id string) (*domain.Episode, error) {
	ep, ok := f.getByID[id]
	if !ok { return nil, errors.NotFound("episode not found") }
	return &ep, nil
}

type fakeObjects struct {
	deleted, deletedStorage string
	failDelete              bool
}
func (f *fakeObjects) DeletePrefix(_ context.Context, storage, prefix string) error {
	if f.failDelete { return errors.New("boom") }
	f.deletedStorage, f.deleted = storage, prefix
	return nil
}

type fakeCfg struct{}
func (fakeCfg) Get(context.Context) (*domain.AutocacheConfig, error) { return &domain.AutocacheConfig{}, nil }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/autocache/ -run TestDeleteEpisodeByID -v`
Expected: FAIL — `e.DeleteEpisodeByID undefined` and/or `pool.GetByID` not in `poolAccountant`.

- [ ] **Step 3: Add `GetByID` to the `poolAccountant` interface**

In `evictor.go`, the `poolAccountant` interface (near the top) gains one line:

```go
type poolAccountant interface {
	SumPoolBytes(ctx context.Context) (int64, error)
	ListStaleEvictionCandidates(ctx context.Context, cfg *domain.AutocacheConfig, now time.Time) ([]domain.Episode, error)
	DeleteByID(ctx context.Context, id string) error
	ListPool(ctx context.Context) ([]domain.Episode, error)
	GetByID(ctx context.Context, id string) (*domain.Episode, error) // Task 1: manual delete
}
```

`*repo.EpisodeRepository` already implements `GetByID` (`services/library/internal/repo/episode.go:198`), so the production wiring needs no change.

- [ ] **Step 4: Implement `DeleteEpisodeByID`**

Add near `evictOne` in `evictor.go`:

```go
// DeleteEpisodeByID evicts a single episode by id through the SAME objects-first,
// DB-reconciled path the freshness sweep uses (evictOne). It takes the Evictor mutex
// so a manual delete can't race the sweep / pre-admit budget accounting. This is the
// ONLY delete path the admin file manager calls for object-store episodes.
func (e *Evictor) DeleteEpisodeByID(ctx context.Context, id string) error {
	ep, err := e.pool.GetByID(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.evictOne(ctx, *ep)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/autocache/ -run TestDeleteEpisodeByID -v`
Expected: PASS (both cases). Then `go build ./services/library/...` — Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/autocache/evictor.go services/library/internal/autocache/evictor_test.go
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): Evictor.DeleteEpisodeByID reuses evictOne for manual delete

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 2: `service.WorkDir` — jailed FS list/delete for the torrent working dir

**Files:**
- Create: `services/library/internal/service/workdir.go`
- Test: `services/library/internal/service/workdir_test.go`

**Interfaces:**
- Produces:
  - `type WorkDirEntry struct { Name string; IsDir bool; Size int64 }`
  - `func NewWorkDir(root string) *WorkDir`
  - `func (wd *WorkDir) List(rel string) ([]WorkDirEntry, error)`
  - `func (wd *WorkDir) Delete(rel string) error`
  - `func (wd *WorkDir) Resolve(rel string) (string, error)` — jailed absolute path (used by Download in Task 5).
  - Used by Tasks 4, 5, 6.

- [ ] **Step 1: Write the failing test**

```go
package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkDir_JailRejectsTraversal(t *testing.T) {
	wd := NewWorkDir(t.TempDir())
	for _, bad := range []string{"../etc", "/etc/passwd", "a/../../b", ".."} {
		if _, err := wd.Resolve(bad); err == nil {
			t.Fatalf("Resolve(%q) should be rejected", bad)
		}
	}
}

func TestWorkDir_ListAndDelete(t *testing.T) {
	root := t.TempDir()
	wd := NewWorkDir(root)
	ih := filepath.Join(root, "abcd1234")
	if err := os.MkdirAll(ih, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(ih, "video.mkv"), []byte("hello"), 0o644); err != nil { t.Fatal(err) }

	top, err := wd.List("")
	if err != nil { t.Fatal(err) }
	if len(top) != 1 || top[0].Name != "abcd1234" || !top[0].IsDir {
		t.Fatalf("unexpected top listing: %+v", top)
	}
	inside, err := wd.List("abcd1234")
	if err != nil { t.Fatal(err) }
	if len(inside) != 1 || inside[0].Name != "video.mkv" || inside[0].Size != 5 {
		t.Fatalf("unexpected inner listing: %+v", inside)
	}
	if err := wd.Delete("abcd1234"); err != nil { t.Fatal(err) }
	if _, err := os.Stat(ih); !os.IsNotExist(err) {
		t.Fatal("expected dir removed")
	}
}

func TestWorkDir_DeleteRootRefused(t *testing.T) {
	wd := NewWorkDir(t.TempDir())
	if err := wd.Delete(""); err == nil {
		t.Fatal("deleting the root must be refused")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/service/ -run TestWorkDir -v`
Expected: FAIL — `NewWorkDir undefined`.

- [ ] **Step 3: Implement `WorkDir`**

```go
package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// WorkDirEntry is one listing row from the torrent working dir.
type WorkDirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// WorkDir is a path-jailed view of LIBRARY_TORRENT_DOWNLOAD_DIR. Every rel path is
// lexically cleaned and confirmed to stay inside root (rejecting .., absolute paths,
// and symlink escapes) before any FS op — the file manager never touches disk outside
// the torrent working dir.
type WorkDir struct{ root string }

func NewWorkDir(root string) *WorkDir { return &WorkDir{root: filepath.Clean(root)} }

// Resolve returns the jailed absolute path for rel, or an error if it escapes root.
func (wd *WorkDir) Resolve(rel string) (string, error) {
	clean := filepath.Clean("/" + strings.TrimPrefix(rel, "/")) // force-anchor, strips ..
	abs := filepath.Join(wd.root, clean)
	if abs != wd.root && !strings.HasPrefix(abs, wd.root+string(os.PathSeparator)) {
		return "", errors.InvalidInput("path escapes working dir")
	}
	// Reject symlink escape: if the path exists, its real path must still be inside root.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		if real != wd.root && !strings.HasPrefix(real, wd.root+string(os.PathSeparator)) {
			return "", errors.InvalidInput("path escapes working dir")
		}
	}
	return abs, nil
}

// List returns the entries directly under rel (one level, not recursive).
func (wd *WorkDir) List(rel string) ([]WorkDirEntry, error) {
	abs, err := wd.Resolve(rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFound("path not found")
		}
		return nil, err
	}
	out := make([]WorkDirEntry, 0, len(des))
	for _, de := range des {
		e := WorkDirEntry{Name: de.Name(), IsDir: de.IsDir()}
		if info, ierr := de.Info(); ierr == nil {
			e.Size = info.Size()
		}
		out = append(out, e)
	}
	return out, nil
}

// Delete removes the file or directory (recursively) at rel. The root itself cannot
// be deleted.
func (wd *WorkDir) Delete(rel string) error {
	abs, err := wd.Resolve(rel)
	if err != nil {
		return err
	}
	if abs == wd.root {
		return errors.InvalidInput("refusing to delete the working-dir root")
	}
	return os.RemoveAll(abs)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/service/ -run TestWorkDir -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/service/workdir.go services/library/internal/service/workdir_test.go
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): jailed WorkDir list/delete for torrent working dir

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 3: `service.ActiveTorrents` — in-flight infohash set (work-dir delete guard)

**Files:**
- Create: `services/library/internal/service/active_torrents.go`
- Test: `services/library/internal/service/active_torrents_test.go`

**Interfaces:**
- Consumes: `repo.JobRepository.List(ctx, repo.JobFilter{Statuses, Limit})` returning `[]domain.Job` (each has `.Magnet`).
- Produces:
  - `type ActiveTorrents struct { jobs jobLister }` with `func NewActiveTorrents(jobs jobLister) *ActiveTorrents`
  - `func (a *ActiveTorrents) Infohashes(ctx context.Context) (map[string]struct{}, error)` — lowercase-hex infohashes of every job in an active status.
  - `type jobLister interface { List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error) }`
  - Used by Task 6 (work-dir delete refuses when the target infohash is active).

- [ ] **Step 1: Write the failing test**

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

type stubJobs struct{ jobs []domain.Job }
func (s *stubJobs) List(_ context.Context, _ repo.JobFilter) ([]domain.Job, error) { return s.jobs, nil }

func TestActiveTorrents_Infohashes(t *testing.T) {
	// A valid v1 magnet (40-hex infohash).
	const ih = "0123456789abcdef0123456789abcdef01234567"
	s := &stubJobs{jobs: []domain.Job{
		{Magnet: "magnet:?xt=urn:btih:" + ih + "&dn=x", Status: domain.JobStatusDownloading},
		{Magnet: "not-a-magnet", Status: domain.JobStatusQueued}, // skipped, not fatal
	}}
	a := NewActiveTorrents(s)
	set, err := a.Infohashes(context.Background())
	if err != nil { t.Fatal(err) }
	if _, ok := set[ih]; !ok {
		t.Fatalf("expected %s in active set, got %v", ih, set)
	}
	if len(set) != 1 {
		t.Fatalf("bad magnet must be skipped, got %v", set)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/service/ -run TestActiveTorrents -v`
Expected: FAIL — `NewActiveTorrents undefined`.

- [ ] **Step 3: Implement `ActiveTorrents`**

```go
package service

import (
	"context"
	"strings"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
)

// activeJobStatuses are the states in which a job's payload may still occupy the
// torrent working dir (download → encode reads the same {infohash}/ dir).
var activeJobStatuses = []domain.JobStatus{
	domain.JobStatusQueued,
	domain.JobStatusDownloading,
	domain.JobStatusEncoding,
	domain.JobStatusTranscoding,
	domain.JobStatusUploading,
}

type jobLister interface {
	List(ctx context.Context, f repo.JobFilter) ([]domain.Job, error)
}

// ActiveTorrents answers "is this infohash still in use by an in-flight job?" so the
// file manager refuses to delete a working-dir that a job is actively writing/reading.
type ActiveTorrents struct{ jobs jobLister }

func NewActiveTorrents(jobs jobLister) *ActiveTorrents { return &ActiveTorrents{jobs: jobs} }

func (a *ActiveTorrents) Infohashes(ctx context.Context) (map[string]struct{}, error) {
	rows, err := a.jobs.List(ctx, repo.JobFilter{Statuses: activeJobStatuses, Limit: 500})
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(rows))
	for _, j := range rows {
		m, err := metainfo.ParseMagnetUri(j.Magnet)
		if err != nil {
			continue // a malformed magnet can't map to a working dir
		}
		set[strings.ToLower(m.InfoHash.HexString())] = struct{}{}
	}
	return set, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/service/ -run TestActiveTorrents -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/service/active_torrents.go services/library/internal/service/active_torrents_test.go
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): ActiveTorrents infohash set for work-dir delete guard

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 4: `storagegw.Gateway.DownloadURL` — single-key presigned GET

**Files:**
- Modify: `services/library/internal/storagegw/gateway.go`
- Test: `services/library/internal/storagegw/gateway_test.go` (add case; create file if absent)

**Interfaces:**
- Consumes: `storageclient.Client.DownloadURLs(ctx, storage, prefix) ([]storageclient.GetURL, error)` (the client wrapper over storage `POST /download-urls`).
- Produces: `func (g *Gateway) DownloadURL(ctx context.Context, storage, key string) (string, error)` — presigned GET URL for exactly `key`. Used by Task 5.

> **Implementer note:** confirm the client method name with
> `grep -n "func (c \*Client) DownloadURL" libs/storageclient/client.go`. If the client
> exposes a single-key `DownloadURL`, wrap that directly. Otherwise use `DownloadURLs`
> with `prefix=key` and pick the entry whose `Name`/key matches, as below.

- [ ] **Step 1: Write the failing test**

```go
func TestGateway_DownloadURL_SingleKey(t *testing.T) {
	// fakeClient returns one presigned URL for the exact key.
	fc := &fakeStorageClient{getURLs: []storageclient.GetURL{{Name: "e01_1080p.ts", URL: "https://signed/e01_1080p.ts"}}}
	g := newTestGateway(fc)
	url, err := g.DownloadURL(context.Background(), "minio", "frieren/s1/e01/e01_1080p.ts")
	if err != nil { t.Fatal(err) }
	if url != "https://signed/e01_1080p.ts" {
		t.Fatalf("got %q", url)
	}
}
```

(Add a minimal `fakeStorageClient` implementing the `storageClient` seam already used by `Gateway`, or the concrete client interface. If the gateway holds a concrete `*storageclient.Client`, introduce a tiny interface `storageDownloader` the gateway embeds — mirror how the file is already structured.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/storagegw/ -run TestGateway_DownloadURL -v`
Expected: FAIL — `g.DownloadURL undefined`.

- [ ] **Step 3: Implement `DownloadURL`**

```go
// DownloadURL returns a presigned GET URL for exactly `key` on backend `storage`.
// The file manager's download handler fetches this server-side and streams the bytes
// to the admin (MinIO's presigned host is internal-only, so the browser can't fetch it
// directly). Reuses the storage service's /download-urls endpoint.
func (g *Gateway) DownloadURL(ctx context.Context, storage, key string) (string, error) {
	urls, err := g.client.DownloadURLs(ctx, storage, key)
	if err != nil {
		return "", err
	}
	for _, u := range urls {
		// download-urls returns Name relative to the requested prefix; for a single-key
		// prefix the match is the empty-suffix / basename entry.
		if key == u.Name || strings.HasSuffix(key, u.Name) {
			return u.URL, nil
		}
	}
	if len(urls) == 1 {
		return urls[0].URL, nil
	}
	return "", errors.NotFound("object not found")
}
```

Add `"strings"` and `libs/errors` imports if missing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/storagegw/ -v`
Expected: PASS. Then `go build ./services/library/...`.

- [ ] **Step 5: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/storagegw/
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): storagegw.DownloadURL single-key presigned GET

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 5: `FilesHandler.Browse` + `FilesHandler.Download`

**Files:**
- Create: `services/library/internal/handler/files.go`
- Test: `services/library/internal/handler/files_test.go`

**Interfaces:**
- Consumes: `service.WorkDir` (Task 2), `storagegw.Gateway` (List/URLFor/DownloadURL/DeletePrefix), `autocache.Classify`, `repo.EpisodeRepository.ListPool`, `service.ActiveTorrents` (Task 3), `autocache.Evictor.DeleteEpisodeByID` (Task 1).
- Produces (this task): `NewFilesHandler(...) *FilesHandler`, `func (h *FilesHandler) Browse(w, r)`, `func (h *FilesHandler) Download(w, r)`. `Delete` is added in Task 6 (same struct).

Define the seams + DTOs at the top of `files.go`:

```go
package handler

import (
	"context"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/autocache"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
)

type filesObjectStore interface {
	List(ctx context.Context, storage, prefix string) ([]storageclient.Object, error)
	DownloadURL(ctx context.Context, storage, key string) (string, error)
	DeletePrefix(ctx context.Context, storage, prefix string) error
}
type filesEpisodeIndex interface {
	ListPool(ctx context.Context) ([]domain.Episode, error)
}
type filesConfig interface {
	Get(ctx context.Context) (*domain.AutocacheConfig, error)
}
type filesEpisodeEvictor interface {
	DeleteEpisodeByID(ctx context.Context, id string) error
}
type filesActive interface {
	Infohashes(ctx context.Context) (map[string]struct{}, error)
}

type FilesHandler struct {
	work    *service.WorkDir
	store   filesObjectStore
	episodes filesEpisodeIndex
	config  filesConfig
	evictor filesEpisodeEvictor
	active  filesActive
	httpGet func(ctx context.Context, url string) (*http.Response, error) // seam for tests
	log     *logger.Logger
}

func NewFilesHandler(work *service.WorkDir, store filesObjectStore, episodes filesEpisodeIndex,
	config filesConfig, evictor filesEpisodeEvictor, active filesActive, log *logger.Logger) *FilesHandler {
	return &FilesHandler{
		work: work, store: store, episodes: episodes, config: config,
		evictor: evictor, active: active, log: log,
		httpGet: func(ctx context.Context, url string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil { return nil, err }
			return http.DefaultClient.Do(req)
		},
	}
}

type fileEpisodeDTO struct {
	EpisodeID   string `json:"episode_id"`
	ShikimoriID string `json:"shikimori_id"`
	Episode     *int   `json:"episode,omitempty"`
	Source      string `json:"source"`
	Freshness   string `json:"freshness"`
}
type fileEntryDTO struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"` // "dir" | "file"
	Size    int64           `json:"size"`
	Key     string          `json:"key,omitempty"`
	Episode *fileEpisodeDTO `json:"episode,omitempty"`
}
type browseResponseDTO struct {
	Domain     string         `json:"domain"`
	Prefix     string         `json:"prefix"`
	Breadcrumb []string       `json:"breadcrumb"`
	Entries    []fileEntryDTO `json:"entries"`
}

func validDomain(d string) bool { return d == "work" || d == "minio" || d == "s3" }

func breadcrumb(prefix string) []string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" { return []string{} }
	return strings.Split(prefix, "/")
}
```

- [ ] **Step 1: Write the failing tests**

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/service"
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
)

type fakeStore struct {
	objs      []storageclient.Object
	dlURL     string
	deleted   string
}
func (f *fakeStore) List(context.Context, string, string) ([]storageclient.Object, error) { return f.objs, nil }
func (f *fakeStore) DownloadURL(context.Context, string, string) (string, error) { return f.dlURL, nil }
func (f *fakeStore) DeletePrefix(_ context.Context, _ , p string) error { f.deleted = p; return nil }

type fakeEpisodes struct{ pool []domain.Episode }
func (f *fakeEpisodes) ListPool(context.Context) ([]domain.Episode, error) { return f.pool, nil }
type fakeConfig struct{}
func (fakeConfig) Get(context.Context) (*domain.AutocacheConfig, error) { return &domain.AutocacheConfig{AdminFreshDays: 3650}, nil }
type fakeEvictor struct{ deletedID string }
func (f *fakeEvictor) DeleteEpisodeByID(_ context.Context, id string) error { f.deletedID = id; return nil }
type fakeActive struct{ set map[string]struct{} }
func (f *fakeActive) Infohashes(context.Context) (map[string]struct{}, error) { return f.set, nil }

func newTestFiles(t *testing.T, store filesObjectStore, eps filesEpisodeIndex, ev filesEpisodeEvictor, act filesActive) *FilesHandler {
	t.Helper()
	return NewFilesHandler(service.NewWorkDir(t.TempDir()), store, eps, fakeConfig{}, ev, act, nil)
}

func TestBrowse_ObjectFolderSynthesisAndEpisodeAnnotation(t *testing.T) {
	store := &fakeStore{objs: []storageclient.Object{
		{Key: "frieren/s1/e01/e01.m3u8", Size: 2000},
		{Key: "frieren/s1/e01/e01_1080p.ts", Size: 240000000},
		{Key: "frieren/s1/e02/e02.m3u8", Size: 2000},
	}}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{
		{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", ShikimoriID: "1", Episode: &n, Source: domain.EpisodeSourceAdmin},
	}}
	h := newTestFiles(t, store, eps, &fakeEvictor{}, &fakeActive{})

	req := httptest.NewRequest(http.MethodGet, "/api/library/files?domain=minio&prefix=frieren/s1/", nil)
	rw := httptest.NewRecorder()
	h.Browse(rw, req)

	if rw.Code != http.StatusOK { t.Fatalf("status %d", rw.Code) }
	// Expect two dir entries e01, e02; e01 annotated with episode ep-1.
	// (decode body, assert kinds + episode annotation)
}

func TestBrowse_BadDomain400(t *testing.T) {
	h := newTestFiles(t, &fakeStore{}, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})
	req := httptest.NewRequest(http.MethodGet, "/api/library/files?domain=zzz", nil)
	rw := httptest.NewRecorder()
	h.Browse(rw, req)
	if rw.Code != http.StatusBadRequest { t.Fatalf("status %d", rw.Code) }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/handler/ -run TestBrowse -v`
Expected: FAIL — `h.Browse undefined`.

- [ ] **Step 3: Implement `Browse` + `Download`**

```go
// Browse handles GET /api/library/files?domain=work|minio|s3&prefix=<p>.
func (h *FilesHandler) Browse(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	prefix := strings.TrimPrefix(r.URL.Query().Get("prefix"), "/")
	if !validDomain(dom) {
		httputil.BadRequest(w, "domain must be work|minio|s3")
		return
	}
	if dom == "work" {
		entries, err := h.work.List(prefix)
		if err != nil { httputil.Error(w, err); return }
		out := make([]fileEntryDTO, 0, len(entries))
		for _, e := range entries {
			kind := "file"
			if e.IsDir { kind = "dir" }
			out = append(out, fileEntryDTO{Name: e.Name, Kind: kind, Size: e.Size})
		}
		sortEntries(out)
		httputil.OK(w, browseResponseDTO{Domain: dom, Prefix: prefix, Breadcrumb: breadcrumb(prefix), Entries: out})
		return
	}

	// object store: list recursively under prefix, synthesize one folder level.
	objs, err := h.store.List(r.Context(), dom, prefix)
	if err != nil { httputil.Error(w, err); return }
	epByPath := h.episodeIndexForStorage(r.Context(), dom)
	cfg, _ := h.config.Get(r.Context())
	now := time.Now()

	dirSizes := map[string]int64{}
	dirOrder := []string{}
	var files []fileEntryDTO
	for _, o := range objs {
		rest := strings.TrimPrefix(o.Key, prefix)
		if rest == "" { continue }
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			d := rest[:i]
			if _, seen := dirSizes[d]; !seen { dirOrder = append(dirOrder, d) }
			dirSizes[d] += o.Size
		} else {
			files = append(files, fileEntryDTO{Name: rest, Kind: "file", Size: o.Size, Key: o.Key})
		}
	}
	entries := make([]fileEntryDTO, 0, len(dirOrder)+len(files))
	for _, d := range dirOrder {
		full := prefix + d + "/"
		e := fileEntryDTO{Name: d, Kind: "dir", Size: dirSizes[d], Key: full}
		if ep, ok := epByPath[full]; ok {
			e.Episode = &fileEpisodeDTO{
				EpisodeID: ep.ID, ShikimoriID: ep.ShikimoriID, Episode: ep.Episode,
				Source: string(ep.Source), Freshness: string(autocache.Classify(ep, cfg, now)),
			}
		}
		entries = append(entries, e)
	}
	entries = append(entries, files...)
	sortEntries(entries)
	httputil.OK(w, browseResponseDTO{Domain: dom, Prefix: prefix, Breadcrumb: breadcrumb(prefix), Entries: entries})
}

func (h *FilesHandler) episodeIndexForStorage(ctx context.Context, storage string) map[string]domain.Episode {
	pool, err := h.episodes.ListPool(ctx)
	if err != nil { return map[string]domain.Episode{} }
	m := make(map[string]domain.Episode, len(pool))
	for _, ep := range pool {
		if ep.Storage == storage {
			m[ep.MinioPath] = ep
		}
	}
	return m
}

// sortEntries: dirs first, then files, each alphabetical.
func sortEntries(e []fileEntryDTO) {
	sort.SliceStable(e, func(i, j int) bool {
		if (e[i].Kind == "dir") != (e[j].Kind == "dir") {
			return e[i].Kind == "dir"
		}
		return e[i].Name < e[j].Name
	})
}

// Download handles GET /api/library/files/download?domain=&key=. It streams bytes to the
// admin: object stores are fetched server-side from a presigned URL (MinIO's host is
// internal-only); the work dir is served from disk within the jail.
func (h *FilesHandler) Download(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	key := r.URL.Query().Get("key")
	if !validDomain(dom) || key == "" {
		httputil.BadRequest(w, "domain (work|minio|s3) and key are required")
		return
	}
	if dom == "work" {
		abs, err := h.work.Resolve(key)
		if err != nil { httputil.Error(w, err); return }
		w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(abs)+"\"")
		http.ServeFile(w, r, abs)
		return
	}
	url, err := h.store.DownloadURL(r.Context(), dom, key)
	if err != nil { httputil.Error(w, err); return }
	resp, err := h.httpGet(r.Context(), url)
	if err != nil { httputil.Error(w, err); return }
	defer resp.Body.Close()
	w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(key)+"\"")
	if ct := resp.Header.Get("Content-Type"); ct != "" { w.Header().Set("Content-Type", ct) }
	if cl := resp.Header.Get("Content-Length"); cl != "" { w.Header().Set("Content-Length", cl) }
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
```

Complete the two Browse test assertions (decode `rw.Body` into `browseResponseDTO`, assert `Entries[0].Kind=="dir"`, `Entries[0].Name=="e01"`, `Entries[0].Episode.EpisodeID=="ep-1"`). Add a `TestDownload_WorkDirStreamsFile` (write a temp file, assert body + Content-Disposition) and `TestDownload_ObjectUsesPresignedFetch` (set `fakeStore.dlURL` to an `httptest.Server` URL, assert streamed body).

- [ ] **Step 4: Run to verify pass**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/handler/ -run 'TestBrowse|TestDownload' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/handler/files.go services/library/internal/handler/files_test.go
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): FilesHandler Browse + Download (work/minio/s3)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 6: `FilesHandler.Delete` + router + main.go wiring

**Files:**
- Modify: `services/library/internal/handler/files.go` (add `Delete`)
- Modify: `services/library/internal/handler/files_test.go` (add cases)
- Modify: `services/library/internal/transport/router.go`
- Modify: `services/library/cmd/library-api/main.go`
- Test: `services/library/internal/transport/router_test.go` (if it constructs the router, update the call)

**Interfaces:**
- Produces: `func (h *FilesHandler) Delete(w, r)` routing: `work` → active-guard + `WorkDir.Delete`; object key matching an episode `MinioPath` → `evictor.DeleteEpisodeByID`; object orphan → `store.DeletePrefix` behind `?confirm=1`.
- Consumes: everything wired in Task 5 + `ActiveTorrents` (Task 3).

- [ ] **Step 1: Write the failing tests**

```go
func TestDelete_EpisodeRoutesToEvictor(t *testing.T) {
	store := &fakeStore{}
	n := 1
	eps := &fakeEpisodes{pool: []domain.Episode{{ID: "ep-1", Storage: "minio", MinioPath: "frieren/s1/e01/", Episode: &n}}}
	ev := &fakeEvictor{}
	h := newTestFiles(t, store, eps, ev, &fakeActive{})

	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=frieren/s1/e01/", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusOK { t.Fatalf("status %d", rw.Code) }
	if ev.deletedID != "ep-1" { t.Fatalf("expected evictor delete ep-1, got %q", ev.deletedID) }
	if store.deleted != "" { t.Fatal("must not raw-delete an episode prefix") }
}

func TestDelete_OrphanNeedsConfirm(t *testing.T) {
	store := &fakeStore{}
	h := newTestFiles(t, store, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{})
	// no confirm → 409
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=stray/file.bin", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusConflict { t.Fatalf("status %d", rw.Code) }
	// with confirm → deletes prefix
	req2 := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=minio&key=stray/file.bin&confirm=1", nil)
	rw2 := httptest.NewRecorder()
	h.Delete(rw2, req2)
	if rw2.Code != http.StatusOK || store.deleted != "stray/file.bin" {
		t.Fatalf("status %d deleted %q", rw2.Code, store.deleted)
	}
}

func TestDelete_WorkDirActiveTorrentRefused(t *testing.T) {
	const ih = "abcd"
	h := newTestFiles(t, &fakeStore{}, &fakeEpisodes{}, &fakeEvictor{}, &fakeActive{set: map[string]struct{}{ih: {}}})
	req := httptest.NewRequest(http.MethodDelete, "/api/library/files?domain=work&key=abcd", nil)
	rw := httptest.NewRecorder()
	h.Delete(rw, req)
	if rw.Code != http.StatusConflict { t.Fatalf("expected 409 for active torrent, got %d", rw.Code) }
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go test ./services/library/internal/handler/ -run TestDelete -v`
Expected: FAIL — `h.Delete undefined`.

- [ ] **Step 3: Implement `Delete`**

```go
// Delete handles DELETE /api/library/files?domain=&key=[&confirm=1].
func (h *FilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	key := r.URL.Query().Get("key")
	confirm := r.URL.Query().Get("confirm") == "1"
	if !validDomain(dom) || key == "" {
		httputil.BadRequest(w, "domain (work|minio|s3) and key are required")
		return
	}

	if dom == "work" {
		// Refuse if the top-level infohash segment is still an in-flight job.
		ih := strings.ToLower(strings.SplitN(strings.Trim(key, "/"), "/", 2)[0])
		active, err := h.active.Infohashes(r.Context())
		if err != nil { httputil.Error(w, err); return }
		if _, busy := active[ih]; busy {
			httputil.JSON(w, http.StatusConflict, map[string]string{"error": "torrent still active — cancel its job first"})
			return
		}
		if err := h.work.Delete(key); err != nil { httputil.Error(w, err); return }
		httputil.OK(w, map[string]bool{"deleted": true})
		return
	}

	// object store: episode-mapped prefix → reconciled evictor delete.
	epByPath := h.episodeIndexForStorage(r.Context(), dom)
	if ep, ok := epByPath[strings.TrimSuffix(key, "/")+"/"]; ok {
		if err := h.evictor.DeleteEpisodeByID(r.Context(), ep.ID); err != nil { httputil.Error(w, err); return }
		httputil.OK(w, map[string]bool{"deleted": true})
		return
	}
	// orphan object/prefix → raw delete, guarded by explicit confirm.
	if !confirm {
		httputil.JSON(w, http.StatusConflict, map[string]string{"error": "orphan object (no episode row) — retry with confirm=1"})
		return
	}
	if err := h.store.DeletePrefix(r.Context(), dom, key); err != nil { httputil.Error(w, err); return }
	httputil.OK(w, map[string]bool{"deleted": true})
}
```

- [ ] **Step 4: Register routes in `router.go`**

Add `filesHandler *handler.FilesHandler` as a new parameter to `NewRouter` (after `autocacheInternalHandler`). Inside the `r.Route("/api/library", ...)` block, add:

```go
if filesHandler != nil {
	r.Get("/files", filesHandler.Browse)
	r.Get("/files/download", filesHandler.Download)
	r.Delete("/files", filesHandler.Delete)
}
```

- [ ] **Step 5: Wire in `main.go`**

After `episodesHandler` / the `evictor` construction (`evictor` already exists at `main.go:531`; `storageGW`, `episodeRepo`, `jobRepo`, `autocacheConfigRepo` all exist), add:

```go
workDir := service.NewWorkDir(cfg.Torrent.DownloadDir)
activeTorrents := service.NewActiveTorrents(jobRepo)
filesHandler := handler.NewFilesHandler(workDir, storageGW, episodeRepo, autocacheConfigRepo, evictor, activeTorrents, log)
```

Then pass `filesHandler` into the `transport.NewRouter(...)` call (add the argument in the same position as the new param). Update `router_test.go`'s `NewRouter(...)` call(s) to pass `nil` for the new handler where they don't exercise it.

> `storagegw.Gateway` must satisfy `filesObjectStore` — it now has `List`, `DeletePrefix`, and `DownloadURL` (Task 4). `*repo.EpisodeRepository` satisfies `filesEpisodeIndex` (`ListPool`) and `*repo.AutocacheConfigRepository` satisfies `filesConfig` (`Get`). `*autocache.Evictor` satisfies `filesEpisodeEvictor` (Task 1). `*service.ActiveTorrents` satisfies `filesActive`.

- [ ] **Step 6: Run tests + build**

Run:
```
cd /data/animeenigma/.claude/worktrees/torrent-file-manager && \
go test ./services/library/internal/handler/ ./services/library/internal/transport/ -v && \
go build ./services/library/...
```
Expected: PASS + clean build.

- [ ] **Step 7: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add services/library/internal/handler/files.go services/library/internal/handler/files_test.go services/library/internal/transport/router.go services/library/internal/transport/router_test.go services/library/cmd/library-api/main.go
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(library): /api/library/files delete + route wiring

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 7: Frontend — API client + types

**Files:**
- Modify: `frontend/web/src/types/library.ts`
- Modify: `frontend/web/src/api/client.ts`
- Test: `frontend/web/src/api/__tests__/` (add a small unit if the folder tests client methods; else covered by Task 8 component test)

**Interfaces:**
- Produces: `adminLibraryApi.browseFiles`, `adminLibraryApi.deleteFile`, `adminLibraryApi.fileDownloadUrl`; types `FileDomain`, `FileEntry`, `BrowseResponse`.

- [ ] **Step 1: Add types** to `src/types/library.ts`:

```ts
export type FileDomain = 'work' | 'minio' | 's3'

export interface FileEpisode {
  episode_id: string
  shikimori_id: string
  episode?: number
  source: string
  freshness: 'fresh' | 'stale'
}
export interface FileEntry {
  name: string
  kind: 'dir' | 'file'
  size: number
  key?: string
  episode?: FileEpisode
}
export interface BrowseResponse {
  domain: FileDomain
  prefix: string
  breadcrumb: string[]
  entries: FileEntry[]
}
```

- [ ] **Step 2: Add API methods** inside `adminLibraryApi` in `src/api/client.ts`:

```ts
  browseFiles: (domain: string, prefix = '') =>
    apiClient.get('/library/files', { params: { domain, ...(prefix ? { prefix } : {}) } }),
  deleteFile: (domain: string, key: string, confirm = false) =>
    apiClient.delete('/library/files', { params: { domain, key, ...(confirm ? { confirm: 1 } : {}) } }),
  downloadFile: (domain: string, key: string) =>
    apiClient.get('/library/files/download', { params: { domain, key }, responseType: 'blob' }),
```

- [ ] **Step 3: Type-check**

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager/frontend/web && bunx tsc --noEmit`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add frontend/web/src/types/library.ts frontend/web/src/api/client.ts
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(web): adminLibraryApi file-manager client + types

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

### Task 8: Frontend — "Files" section in `RawLibrary.vue` + i18n

**Files:**
- Modify: `frontend/web/src/views/admin/RawLibrary.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/views/__tests__/RawLibrary.files.spec.ts` (new)

**Interfaces:**
- Consumes: `adminLibraryApi.browseFiles/deleteFile/downloadFile/createJob`, types from Task 7, existing `Button/Input/Select/Badge/Spinner` + `useConfirm`.

- [ ] **Step 1: Add i18n keys** — add under the existing `adminLibrary` object in `en.json`:

```json
"files": {
  "title": "Files",
  "domain": { "work": "Torrent work dir", "minio": "MinIO", "s3": "S3 cloud" },
  "root": "root",
  "col": { "name": "Name", "size": "Size", "actions": "Actions" },
  "empty": "Empty",
  "download": "Download",
  "delete": "Delete",
  "fresh": "Fresh",
  "stale": "Stale",
  "add": { "title": "Add torrent by hand", "magnet": "Paste magnet link", "name": "Title", "storage": "Storage", "submit": "Enqueue" },
  "confirm": {
    "episode": "Delete episode {name}? Its files and library record are removed.",
    "orphan": "Delete orphan object {name}? This is a raw delete with no library record.",
    "work": "Delete {name} from the torrent working dir?"
  },
  "error": { "active": "Torrent still active — cancel its job first." }
}
```

Mirror the SAME keys with translated values in `ru.json` and `ja.json` (i18n parity is build-enforced). RU example values: `"title": "Файлы"`, `"domain": { "work": "Каталог торрентов", "minio": "MinIO", "s3": "Облако S3" }`, `"download": "Скачать"`, `"delete": "Удалить"`, `"add": { "title": "Добавить торрент вручную", "magnet": "Вставьте magnet-ссылку", ... }`. JA example: `"title": "ファイル"`, `"download": "ダウンロード"`, `"delete": "削除"`, etc. (translate every leaf).

- [ ] **Step 2: Add the Files section script logic** — in `RawLibrary.vue <script setup>`, add:

```ts
import type { FileDomain, FileEntry, BrowseResponse } from '@/types/library'

const fileDomain = ref<FileDomain>('minio')
const filePrefix = ref('')
const fileEntries = ref<FileEntry[]>([])
const fileBreadcrumb = ref<string[]>([])
const filesLoading = ref(false)
const magnet = ref('')
const magnetTitle = ref('')
const magnetStorage = ref<'minio' | 's3'>('minio')

const fileDomainOptions: SelectOption[] = [
  { value: 'work', label: t('player.adminLibrary.files.domain.work') },
  { value: 'minio', label: t('player.adminLibrary.files.domain.minio') },
  { value: 's3', label: t('player.adminLibrary.files.domain.s3') },
]

async function loadFiles(prefix = '') {
  filesLoading.value = true
  try {
    const { data } = await adminLibraryApi.browseFiles(fileDomain.value, prefix)
    const body = data as BrowseResponse
    fileEntries.value = body.entries
    fileBreadcrumb.value = body.breadcrumb
    filePrefix.value = body.prefix
  } finally {
    filesLoading.value = false
  }
}
function openEntry(e: FileEntry) {
  if (e.kind === 'dir') loadFiles((filePrefix.value ? filePrefix.value : '') + e.name + '/')
}
function crumbTo(idx: number) {
  loadFiles(fileBreadcrumb.value.slice(0, idx + 1).join('/') + '/')
}
function changeDomain(d: FileDomain) { fileDomain.value = d; loadFiles('') }

async function downloadEntry(e: FileEntry) {
  const key = e.key ?? (filePrefix.value + e.name)
  const { data } = await adminLibraryApi.downloadFile(fileDomain.value, key)
  const url = URL.createObjectURL(data as Blob)
  const a = document.createElement('a'); a.href = url; a.download = e.name; a.click()
  URL.revokeObjectURL(url)
}

const { confirm } = useConfirm()
async function deleteEntry(e: FileEntry) {
  const key = e.key ?? (filePrefix.value + e.name)
  const msgKey = fileDomain.value === 'work'
    ? 'player.adminLibrary.files.confirm.work'
    : e.episode ? 'player.adminLibrary.files.confirm.episode'
    : 'player.adminLibrary.files.confirm.orphan'
  if (!(await confirm(t(msgKey, { name: e.name })))) return
  try {
    await adminLibraryApi.deleteFile(fileDomain.value, key, !e.episode) // confirm=1 for orphan/raw
    await loadFiles(filePrefix.value)
  } catch (err: any) {
    if (err?.response?.status === 409) window.alert(t('player.adminLibrary.files.error.active'))
  }
}

async function addMagnet() {
  if (!magnet.value.trim() || !magnetTitle.value.trim()) return
  await adminLibraryApi.createJob({ magnet: magnet.value.trim(), title: magnetTitle.value.trim(), source: 'manual', storage: magnetStorage.value })
  magnet.value = ''; magnetTitle.value = ''
}

onMounted(() => { void loadFiles('') })
```

- [ ] **Step 3: Add the Files section template** — insert a new `<section>` (mirror the existing Jobs section markup/classes for DS consistency). Key elements, all bound to semantic tokens:

```vue
<section class="mb-8" aria-label="files">
  <h2 class="text-xl font-semibold text-foreground mb-3">{{ $t('player.adminLibrary.files.title') }}</h2>

  <!-- domain switch -->
  <div class="flex gap-2 mb-3">
    <Button v-for="o in fileDomainOptions" :key="o.value"
      :variant="fileDomain === o.value ? 'default' : 'outline'" size="sm"
      @click="changeDomain(o.value as FileDomain)">{{ o.label }}</Button>
  </div>

  <!-- add by hand -->
  <div class="flex flex-wrap gap-2 mb-3">
    <Input v-model="magnet" size="sm" :placeholder="$t('player.adminLibrary.files.add.magnet')" />
    <Input v-model="magnetTitle" size="sm" :placeholder="$t('player.adminLibrary.files.add.name')" />
    <Select v-model="magnetStorage" :options="storageOptions" size="sm" />
    <Button size="sm" :disabled="!magnet || !magnetTitle" @click="addMagnet">{{ $t('player.adminLibrary.files.add.submit') }}</Button>
  </div>

  <!-- breadcrumb -->
  <nav class="flex items-center gap-1 text-sm text-muted-foreground mb-2">
    <button class="hover:text-foreground" @click="loadFiles('')">{{ $t('player.adminLibrary.files.root') }}</button>
    <template v-for="(c, i) in fileBreadcrumb" :key="i">
      <span>/</span>
      <button class="hover:text-foreground" @click="crumbTo(i)">{{ c }}</button>
    </template>
  </nav>

  <Spinner v-if="filesLoading" />
  <ul v-else class="divide-y divide-border rounded-md border border-border">
    <li v-if="fileEntries.length === 0" class="p-3 text-sm text-muted-foreground">{{ $t('player.adminLibrary.files.empty') }}</li>
    <li v-for="e in fileEntries" :key="e.name" class="flex items-center justify-between gap-2 p-2">
      <button class="flex items-center gap-2 min-w-0 text-left" :class="e.kind === 'dir' ? 'text-foreground' : 'text-muted-foreground'" @click="openEntry(e)">
        <span class="truncate">{{ e.kind === 'dir' ? '📁' : '📄' }} {{ e.name }}</span>
        <Badge v-if="e.episode" :variant="e.episode.freshness === 'fresh' ? 'default' : 'secondary'">
          {{ e.episode.freshness === 'fresh' ? $t('player.adminLibrary.files.fresh') : $t('player.adminLibrary.files.stale') }}
        </Badge>
      </button>
      <div class="flex items-center gap-2 shrink-0">
        <span class="text-xs text-muted-foreground tabular-nums">{{ formatBytes(e.size) }}</span>
        <Button v-if="e.kind === 'file'" variant="ghost" size="sm" @click="downloadEntry(e)">{{ $t('player.adminLibrary.files.download') }}</Button>
        <Button variant="ghost" size="sm" @click="deleteEntry(e)">{{ $t('player.adminLibrary.files.delete') }}</Button>
      </div>
    </li>
  </ul>
</section>
```

Reuse the existing `storageOptions` and `formatBytes` helpers already in `RawLibrary.vue` (if `formatBytes` is absent, add a small one). Confirm `Badge` variant names against `@/components/ui/Badge.vue`.

- [ ] **Step 4: Component test**

Write `src/views/__tests__/RawLibrary.files.spec.ts` mocking `@/api/client` (`browseFiles` resolves a `BrowseResponse` with one dir + one file; `deleteFile` resolves). Mount `RawLibrary.vue`, assert the Files section renders the entries, clicking a dir calls `browseFiles` with the new prefix, and clicking Delete on a file calls `deleteFile`. Use `importOriginal` spread for the vue-i18n mock (project trap: bare mock + ui barrel crashes `createI18n`).

Run: `cd /data/animeenigma/.claude/worktrees/torrent-file-manager/frontend/web && bunx vitest run src/views/__tests__/RawLibrary.files.spec.ts`
Expected: PASS.

- [ ] **Step 5: Frontend pre-flight**

Run: `/frontend-verify` (DS-lint + i18n en/ru/ja parity + real `bun run build`). Fix any DS/i18n/build violations it reports.

- [ ] **Step 6: Commit**

```bash
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager add frontend/web/src/views/admin/RawLibrary.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/src/views/__tests__/RawLibrary.files.spec.ts
git -C /data/animeenigma/.claude/worktrees/torrent-file-manager commit -F - <<'MSG'
feat(web): file-manager section in raw-library admin

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
MSG
```

---

## Final verification (after all tasks)

- [ ] `cd /data/animeenigma/.claude/worktrees/torrent-file-manager && go build ./services/library/... && go test ./services/library/...`
- [ ] `cd frontend/web && bunx tsc --noEmit && bunx vitest run` (library-touching specs)
- [ ] `/frontend-verify` green (DS + i18n + build).
- [ ] Deploy to a dev/staging library + web (`make redeploy-library`, `make redeploy-web`) — worktree needs `docker/.env` present (copy from base tree; git-ignored) or deploy from base after merge.
- [ ] **Manual smoke:** open `/admin/raw-library` → Files: browse each domain (work/minio/s3), paste a magnet + Enqueue (appears in Jobs), download a file, delete an episode (verify objects gone AND `library_episodes` row gone), attempt to delete an active torrent's work dir (expect the "torrent still active" message).
- [ ] `/animeenigma-after-update` (simplify → changelog → push).

## Self-Review (completed by plan author)

- **Spec coverage:** browse work/minio/s3 (Tasks 5,8) ✓; add-by-hand (Tasks 7,8 reuse jobs) ✓; delete reconciled via evictor + orphan-confirm + work-dir guard (Tasks 1,6) ✓; download (Tasks 4,5) ✓; freshness surfaced via `Classify` (Task 5) ✓; single file authority = library, storage stays backend ✓; non-goals respected (no new freshness logic, no move/rename, no `.torrent` upload) ✓.
- **Spec refinement:** the spec's "delete drops the torrent first" is implemented as **refuse-if-active-job** (Task 6) because the torrent client exposes no drop-by-infohash API; the operator cancels the job (existing `DELETE /jobs/{id}`, which owns torrent lifecycle) to stop it. Spec §3/§4/§5 updated to match.
- **Placeholder scan:** no TBD/TODO; every code step shows real code; the one implementer-verify note (storageclient `DownloadURL` name) has a concrete fallback.
- **Type consistency:** `DeleteEpisodeByID`, `WorkDir{List,Delete,Resolve}`, `WorkDirEntry`, `ActiveTorrents.Infohashes`, `Gateway.DownloadURL`, `FilesHandler{Browse,Download,Delete}`, DTOs, and FE `FileEntry/BrowseResponse/FileDomain` are referenced identically across tasks.
