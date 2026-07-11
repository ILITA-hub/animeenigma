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

// filesObjectStore is the slice of *storagegw.Gateway the file manager needs:
// listing objects under a prefix (Browse), minting a presigned GET for a single
// key (Download), and hard-deleting a whole prefix (Delete, Task 6).
type filesObjectStore interface {
	List(ctx context.Context, storage, prefix string) ([]storageclient.Object, error)
	DownloadURL(ctx context.Context, storage, key string) (string, error)
	DeletePrefix(ctx context.Context, storage, prefix string) error
}

// filesEpisodeIndex is the slice of *repo.EpisodeRepository the handler needs to
// annotate a synthesized object-store folder with the library_episodes row that
// owns it (POOL-03's MinioPath prefix).
type filesEpisodeIndex interface {
	ListPool(ctx context.Context) ([]domain.Episode, error)
}

// filesConfig is the slice of *repo.AutocacheConfigRepository the handler needs
// to feed autocache.Classify's freshness windows.
type filesConfig interface {
	Get(ctx context.Context) (*domain.AutocacheConfig, error)
}

// filesEpisodeEvictor is the slice of *autocache.Evictor the handler needs
// (Task 6's Delete): the objects-first, DB-reconciled single-episode eviction
// path.
type filesEpisodeEvictor interface {
	DeleteEpisodeByID(ctx context.Context, id string) error
}

// filesActive is the slice of *service.ActiveTorrents the handler needs
// (Task 6's Delete): refuses to delete a work-dir prefix a job is still
// reading/writing.
type filesActive interface {
	Infohashes(ctx context.Context) (map[string]struct{}, error)
}

// FilesHandler implements the admin file-manager routes over the torrent
// working dir (domain=work) and the object stores (domain=minio|s3). Browse and
// Download are this task; Delete is added in Task 6 on the same struct.
type FilesHandler struct {
	work     *service.WorkDir
	store    filesObjectStore
	episodes filesEpisodeIndex
	config   filesConfig
	evictor  filesEpisodeEvictor
	active   filesActive
	// httpGet is a seam over http.DefaultClient.Do so Download's presigned-URL
	// fetch can be pointed at an httptest.Server in tests.
	httpGet func(ctx context.Context, url string) (*http.Response, error)
	log     *logger.Logger
}

// NewFilesHandler constructs the handler. httpGet defaults to a real
// http.DefaultClient GET; tests override it via a fresh struct literal or by
// reassigning the field after construction.
func NewFilesHandler(work *service.WorkDir, store filesObjectStore, episodes filesEpisodeIndex,
	config filesConfig, evictor filesEpisodeEvictor, active filesActive, log *logger.Logger) *FilesHandler {
	return &FilesHandler{
		work: work, store: store, episodes: episodes, config: config,
		evictor: evictor, active: active, log: log,
		httpGet: func(ctx context.Context, url string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultClient.Do(req)
		},
	}
}

// fileEpisodeDTO annotates a synthesized object-store folder with the
// library_episodes row that owns it.
type fileEpisodeDTO struct {
	EpisodeID   string `json:"episode_id"`
	ShikimoriID string `json:"shikimori_id"`
	Episode     *int   `json:"episode,omitempty"`
	Source      string `json:"source"`
	Freshness   string `json:"freshness"`
}

// fileEntryDTO is one row in a Browse listing: either a directory (a jailed
// work-dir subdir, or a synthesized one-level-deep object-store "folder") or a
// file/object.
type fileEntryDTO struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"` // "dir" | "file"
	Size    int64           `json:"size"`
	Key     string          `json:"key,omitempty"`
	Episode *fileEpisodeDTO `json:"episode,omitempty"`
}

// browseResponseDTO is the Browse response body (wrapped by httputil.OK in the
// {success,data} envelope).
type browseResponseDTO struct {
	Domain     string         `json:"domain"`
	Prefix     string         `json:"prefix"`
	Breadcrumb []string       `json:"breadcrumb"`
	Entries    []fileEntryDTO `json:"entries"`
}

// validDomain confirms domain is one of the three surfaces the file manager
// understands: the torrent working dir, or one of the two object-store
// backends.
func validDomain(d string) bool { return d == "work" || d == "minio" || d == "s3" }

// breadcrumb splits a prefix into its path segments for the admin UI's
// clickable breadcrumb trail. Empty prefix (the root) yields an empty slice,
// not [""].
func breadcrumb(prefix string) []string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return []string{}
	}
	return strings.Split(prefix, "/")
}

// Browse handles GET /api/library/files?domain=work|minio|s3&prefix=<p>.
//
// domain=work lists one level of the jailed torrent working dir directly via
// service.WorkDir.List. domain=minio|s3 lists every object under prefix
// (object stores have no real directories) and synthesizes one folder level
// from the first "/" after the prefix — mirroring how a conventional file
// browser would present the flat key space. Any synthesized folder that
// exactly matches a library_episodes row's MinioPath is annotated with that
// episode's id/source/freshness so the admin can tell "this folder is episode
// 1, sourced by autocache, and stale" without cross-referencing another page.
func (h *FilesHandler) Browse(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	prefix := strings.TrimPrefix(r.URL.Query().Get("prefix"), "/")
	if !validDomain(dom) {
		httputil.BadRequest(w, "domain must be work|minio|s3")
		return
	}
	if dom == "work" {
		entries, err := h.work.List(prefix)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		out := make([]fileEntryDTO, 0, len(entries))
		for _, e := range entries {
			kind := "file"
			if e.IsDir {
				kind = "dir"
			}
			out = append(out, fileEntryDTO{Name: e.Name, Kind: kind, Size: e.Size})
		}
		sortEntries(out)
		httputil.OK(w, browseResponseDTO{Domain: dom, Prefix: prefix, Breadcrumb: breadcrumb(prefix), Entries: out})
		return
	}

	// Object store: list recursively under prefix, synthesize one folder level.
	objs, err := h.store.List(r.Context(), dom, prefix)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	epByPath := h.episodeIndexForStorage(r.Context(), dom)
	// A Get failure must not panic the listing: autocache.Classify dereferences
	// cfg, so fall back to a zero-value config (all windows 0 days) rather than
	// passing a nil *domain.AutocacheConfig through to it. Annotated episodes
	// just read as maximally stale in that (rare, already-degraded) case.
	cfg, err := h.config.Get(r.Context())
	if err != nil || cfg == nil {
		cfg = &domain.AutocacheConfig{}
	}
	now := time.Now()

	dirSizes := map[string]int64{}
	dirOrder := []string{}
	var files []fileEntryDTO
	for _, o := range objs {
		rest := strings.TrimPrefix(o.Key, prefix)
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			d := rest[:i]
			if _, seen := dirSizes[d]; !seen {
				dirOrder = append(dirOrder, d)
			}
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
			epNum := ep.EpisodeNumber
			e.Episode = &fileEpisodeDTO{
				EpisodeID: ep.ID, ShikimoriID: ep.ShikimoriID, Episode: &epNum,
				Source: string(ep.Source), Freshness: string(autocache.Classify(ep, cfg, now)),
			}
		}
		entries = append(entries, e)
	}
	entries = append(entries, files...)
	sortEntries(entries)
	httputil.OK(w, browseResponseDTO{Domain: dom, Prefix: prefix, Breadcrumb: breadcrumb(prefix), Entries: entries})
}

// episodeIndexForStorage builds a MinioPath -> Episode lookup scoped to
// storage, so Browse can annotate a synthesized folder in O(1). A ListPool
// error is swallowed (episode annotation is best-effort UI sugar, not required
// for the listing itself to succeed).
func (h *FilesHandler) episodeIndexForStorage(ctx context.Context, storage string) map[string]domain.Episode {
	pool, err := h.episodes.ListPool(ctx)
	if err != nil {
		return map[string]domain.Episode{}
	}
	m := make(map[string]domain.Episode, len(pool))
	for _, ep := range pool {
		if ep.Storage == storage {
			m[ep.MinioPath] = ep
		}
	}
	return m
}

// sortEntries orders dirs before files, each alphabetically by name.
func sortEntries(e []fileEntryDTO) {
	sort.SliceStable(e, func(i, j int) bool {
		if (e[i].Kind == "dir") != (e[j].Kind == "dir") {
			return e[i].Kind == "dir"
		}
		return e[i].Name < e[j].Name
	})
}

// Download handles GET /api/library/files/download?domain=&key=. It streams
// bytes to the admin: object stores are fetched server-side from a presigned
// URL (MinIO's host is internal-only, so the browser can't fetch it directly);
// the work dir is served straight from disk within the jail.
func (h *FilesHandler) Download(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	key := r.URL.Query().Get("key")
	if !validDomain(dom) || key == "" {
		httputil.BadRequest(w, "domain (work|minio|s3) and key are required")
		return
	}
	if dom == "work" {
		abs, err := h.work.Resolve(key)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(abs)+"\"")
		http.ServeFile(w, r, abs)
		return
	}
	url, err := h.store.DownloadURL(r.Context(), dom, key)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	resp, err := h.httpGet(r.Context(), url)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(key)+"\"")
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
