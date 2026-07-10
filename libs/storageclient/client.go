// Package storageclient is a thin Go client for the internal storage
// service (services/storage/, port 8099, Docker-network-only — see
// services/storage/internal/domain/storage.go for the wire contract and
// services/storage/internal/handler/storage.go for the routes).
//
// Every route response is wrapped in the repo-wide {"success":bool,
// "data":...,"error":{...}} envelope emitted by libs/httputil.OK/Error —
// this client unwraps it on every call so callers only ever see the
// unwrapped payload or a plain error.
//
// This package intentionally does NOT import services/storage/internal/*
// (it's a separate Go module, consumed by other services across a process
// boundary) — the request/response wire types below are this client's own
// copy of that contract, kept in lockstep by hand.
package storageclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// defaultTimeout is the http.Client timeout for every request this client
// makes, including file PUTs/GETs — large HLS segments are expected to
// stay well under 30s even on a slow self-hosted link.
const defaultTimeout = 30 * time.Second

// longOpTimeout is the http.Client timeout for BULK server-side operations
// (CopyPrefix): a cross-backend prefix copy streams every object through the
// storage service sequentially, so a multi-GB episode legitimately takes
// minutes — the 2026-07-10 storage migration saw a 1.8GB prefix blow through
// the 30s default. Must stay <= the storage service's server WriteTimeout for
// these routes (services/storage/cmd/storage-api/main.go).
const longOpTimeout = 20 * time.Minute

// baseURLsTTL is how long a BaseURLs() response is cached before the next
// call re-fetches it. URLFor() rides this cache, so repeated URLFor calls
// for the same Client don't each round-trip to the storage service.
const baseURLsTTL = 5 * time.Minute

// defaultUploadConcurrency mirrors services/library/internal/minio/writer.go's
// default when UploadFiles is called with concurrency <= 0.
const defaultUploadConcurrency = 8

// Client is a thin HTTP client for the storage service. Safe for concurrent
// use — BaseURLs()/URLFor() share a mutex-guarded cache.
type Client struct {
	baseURL string
	http    *http.Client
	// longHTTP is the long-timeout twin of http, used only by bulk
	// server-side operations (CopyPrefix) that legitimately run for minutes.
	longHTTP *http.Client

	mu         sync.Mutex
	baseURLs   map[string]string
	baseURLsAt time.Time
}

// New constructs a Client for the storage service reachable at baseURL
// (e.g. "http://storage:8099"). Trailing slashes are trimmed.
func New(baseURL string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: defaultTimeout},
		longHTTP: &http.Client{Timeout: longOpTimeout},
	}
}

// PutURL is one presigned PUT entry, as returned by IngestURLs.
type PutURL struct {
	Name string `json:"name"`
	URL  string `json:"put_url"`
}

// GetURL is one presigned GET entry, as returned internally by
// download-urls (surfaced to callers only via DownloadPrefix, which
// downloads every entry itself).
type GetURL struct {
	Name string `json:"name"`
	URL  string `json:"get_url"`
}

// Object is one bucket-relative key + size, as returned by List.
type Object struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
}

// IngestResult is the response of IngestURLs: the backend the caller was
// routed to, plus one presigned PUT URL per requested file.
type IngestResult struct {
	Storage string
	URLs    []PutURL
}

// --- wire types (unexported — this client's private copy of the contract
// documented in services/storage/internal/domain/storage.go) ---

type ingestURLsRequest struct {
	Class    string   `json:"class"`
	Prefix   string   `json:"prefix"`
	Files    []string `json:"files"`
	Override string   `json:"override"`
}

type ingestURLsResponse struct {
	Storage   string   `json:"storage"`
	URLs      []PutURL `json:"urls"`
	ExpiresIn int      `json:"expires_in"`
}

type downloadURLsRequest struct {
	Storage string `json:"storage"`
	Prefix  string `json:"prefix"`
}

type downloadURLsResponse struct {
	URLs []GetURL `json:"urls"`
}

type moveRequest struct {
	Storage    string `json:"storage"`
	FromPrefix string `json:"from_prefix"`
	ToPrefix   string `json:"to_prefix"`
}

type moveResponse struct {
	Moved int `json:"moved"`
}

type copyPrefixRequest struct {
	FromStorage string `json:"from_storage"`
	ToStorage   string `json:"to_storage"`
	Prefix      string `json:"prefix"`
}

type copyResponse struct {
	Copied int   `json:"copied"`
	Bytes  int64 `json:"bytes"`
}

type deletePrefixRequest struct {
	Storage string `json:"storage"`
	Prefix  string `json:"prefix"`
}

type deleteResponse struct {
	Deleted int `json:"deleted"`
}

type listResponse struct {
	Objects []Object `json:"objects"`
}

// envelope mirrors libs/httputil.Response — every storage-service route
// wraps its payload this way.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IngestURLs calls POST /internal/storage/ingest-urls: resolves the
// destination backend for class (honoring override, only meaningful for
// library-manual per the service's placement policy) and returns one
// presigned PUT URL per file (basenames, no path).
func (c *Client) IngestURLs(ctx context.Context, class, prefix string, files []string, override string) (*IngestResult, error) {
	req := ingestURLsRequest{Class: class, Prefix: prefix, Files: files, Override: override}
	var resp ingestURLsResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/storage/ingest-urls", req, &resp); err != nil {
		return nil, err
	}
	return &IngestResult{Storage: resp.Storage, URLs: resp.URLs}, nil
}

// UploadFiles is IngestURLs + the concurrent-PUT upload ordering ported
// verbatim from services/library/internal/minio/writer.go:199-269:
//
//  1. Every file except playlist.m3u8 uploads concurrently via an errgroup
//     capped at concurrency (default 8 when <= 0).
//  2. playlist.m3u8, if present, uploads LAST on the calling goroutine —
//     never in parallel with the other files — so HLS clients never see a
//     playlist referencing a not-yet-uploaded segment.
//  3. If any non-playlist upload fails, the playlist is never uploaded.
//
// Returns the storage backend id IngestURLs resolved to (so the caller can
// record where the files ended up).
func (c *Client) UploadFiles(ctx context.Context, class, override, prefix string, filePaths []string, concurrency int) (string, error) {
	if len(filePaths) == 0 {
		return "", fmt.Errorf("storageclient: UploadFiles called with no files")
	}

	names := make([]string, len(filePaths))
	for i, p := range filePaths {
		names[i] = filepath.Base(p)
	}

	result, err := c.IngestURLs(ctx, class, prefix, names, override)
	if err != nil {
		return "", fmt.Errorf("storageclient: ingest-urls: %w", err)
	}
	urlByName := make(map[string]string, len(result.URLs))
	for _, u := range result.URLs {
		urlByName[u.Name] = u.URL
	}

	var playlist string
	var others []string
	for _, p := range filePaths {
		if filepath.Base(p) == "playlist.m3u8" {
			playlist = p
			continue
		}
		others = append(others, p)
	}

	if concurrency <= 0 {
		concurrency = defaultUploadConcurrency
	}

	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(concurrency)
	for _, p := range others {
		p := p
		eg.Go(func() error {
			putURL, ok := urlByName[filepath.Base(p)]
			if !ok {
				return fmt.Errorf("storageclient: no presigned PUT URL for %s", filepath.Base(p))
			}
			return c.putFile(gctx, putURL, p)
		})
	}
	if err := eg.Wait(); err != nil {
		return "", err
	}

	// Now the playlist — main goroutine, after every other file is done.
	if playlist != "" {
		putURL, ok := urlByName[filepath.Base(playlist)]
		if !ok {
			return "", fmt.Errorf("storageclient: no presigned PUT URL for %s", filepath.Base(playlist))
		}
		if err := c.putFile(ctx, putURL, playlist); err != nil {
			return "", err
		}
	}

	return result.Storage, nil
}

// contentTypeFor maps a filename extension to its Content-Type. Ported
// verbatim from services/library/internal/minio/writer.go:199-212.
func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".jpg":
		return "image/jpeg"
	case ".vtt":
		return "text/vtt"
	default:
		return "application/octet-stream"
	}
}

// putFile PUTs a single local file to putURL, setting Content-Type (an
// unsigned header — S3-compatible presigned PUT URLs accept it) and
// Content-Length from the file's size.
func (c *Client) putFile(ctx context.Context, putURL, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("storageclient: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("storageclient: stat %s: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, f)
	if err != nil {
		return fmt.Errorf("storageclient: build PUT request for %s: %w", path, err)
	}
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", contentTypeFor(path))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("storageclient: PUT %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("storageclient: PUT %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

// Move calls POST /internal/storage/move — server-side prefix move within
// one backend (e.g. relocating pending/<job>/ to the linked shikimori id
// prefix after a title match).
func (c *Client) Move(ctx context.Context, storage, fromPrefix, toPrefix string) error {
	req := moveRequest{Storage: storage, FromPrefix: fromPrefix, ToPrefix: toPrefix}
	var resp moveResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/storage/move", req, &resp); err != nil {
		return err
	}
	return nil
}

// CopyPrefix calls POST /internal/storage/copy — cross-backend prefix copy
// (e.g. minio -> s3 migration). Returns the number of objects copied and
// their total size in bytes.
func (c *Client) CopyPrefix(ctx context.Context, fromStorage, toStorage, prefix string) (int, int64, error) {
	req := copyPrefixRequest{FromStorage: fromStorage, ToStorage: toStorage, Prefix: prefix}
	var resp copyResponse
	// Long-timeout client: a multi-GB prefix streams through the storage
	// service sequentially and legitimately takes minutes.
	if err := c.doJSONLong(ctx, http.MethodPost, "/internal/storage/copy", req, &resp); err != nil {
		return 0, 0, err
	}
	return resp.Copied, resp.Bytes, nil
}

// DeletePrefix calls DELETE /internal/storage/prefix — eviction/cleanup.
// Returns the number of objects deleted.
func (c *Client) DeletePrefix(ctx context.Context, storage, prefix string) (int, error) {
	req := deletePrefixRequest{Storage: storage, Prefix: prefix}
	var resp deleteResponse
	if err := c.doJSON(ctx, http.MethodDelete, "/internal/storage/prefix", req, &resp); err != nil {
		return 0, err
	}
	return resp.Deleted, nil
}

// List calls GET /internal/storage/list?storage=&prefix=.
func (c *Client) List(ctx context.Context, storage, prefix string) ([]Object, error) {
	q := url.Values{}
	q.Set("storage", storage)
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	var resp listResponse
	path := "/internal/storage/list?" + q.Encode()
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}

// BaseURLs calls GET /internal/storage/base-urls — the {backend_id:
// base_url} map — and caches the result for baseURLsTTL. URLFor reads
// through this same cache. Returns a fresh copy on every call so a caller
// mutating the returned map can never corrupt the shared cache.
func (c *Client) BaseURLs(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	if c.baseURLs != nil && time.Since(c.baseURLsAt) < baseURLsTTL {
		cached := copyStringMap(c.baseURLs)
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	var resp map[string]string
	if err := c.doJSON(ctx, http.MethodGet, "/internal/storage/base-urls", nil, &resp); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.baseURLs = resp
	c.baseURLsAt = time.Now()
	c.mu.Unlock()
	return copyStringMap(resp), nil
}

func copyStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// URLFor returns the public-facing URL for path within storage:
// BaseURLs()[storage] + "/" + path. Rides the BaseURLs cache.
func (c *Client) URLFor(ctx context.Context, storage, path string) (string, error) {
	urls, err := c.BaseURLs(ctx)
	if err != nil {
		return "", err
	}
	base, ok := urls[storage]
	if !ok {
		return "", fmt.Errorf("storageclient: unknown storage %q (known: %v)", storage, keysOf(urls))
	}
	return base + "/" + path, nil
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// DownloadPrefix calls POST /internal/storage/download-urls for every
// object under prefix, then GETs each one into destDir/<name> (name is the
// key relative to prefix, per DownloadURLsResponse — nested names create
// subdirectories under destDir as needed).
func (c *Client) DownloadPrefix(ctx context.Context, storage, prefix, destDir string) error {
	req := downloadURLsRequest{Storage: storage, Prefix: prefix}
	var resp downloadURLsResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/storage/download-urls", req, &resp); err != nil {
		return err
	}

	for _, u := range resp.URLs {
		if err := c.downloadOne(ctx, u, destDir); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) downloadOne(ctx context.Context, u GetURL, destDir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.URL, nil)
	if err != nil {
		return fmt.Errorf("storageclient: build GET request for %s: %w", u.Name, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("storageclient: GET %s: %w", u.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("storageclient: GET %s: status %d: %s", u.Name, resp.StatusCode, string(body))
	}

	dest := filepath.Join(destDir, u.Name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("storageclient: mkdir for %s: %w", u.Name, err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("storageclient: create %s: %w", dest, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("storageclient: write %s: %w", dest, err)
	}
	return nil
}

// doJSON performs one JSON request against the storage service, decoding
// the {success,data,error} envelope every route responds with. reqBody may
// be nil (GET/DELETE-with-no-body); out may be nil when the caller doesn't
// need the payload.
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, out interface{}) error {
	return c.doJSONWith(ctx, c.http, method, path, reqBody, out)
}

// doJSONLong is doJSON on the long-timeout client — for bulk server-side
// operations (CopyPrefix) that legitimately run for minutes.
func (c *Client) doJSONLong(ctx context.Context, method, path string, reqBody, out interface{}) error {
	return c.doJSONWith(ctx, c.longHTTP, method, path, reqBody, out)
}

func (c *Client) doJSONWith(ctx context.Context, hc *http.Client, method, path string, reqBody, out interface{}) error {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("storageclient: encode request body: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("storageclient: build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("storageclient: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("storageclient: %s %s: decode response (status %d): %w", method, path, resp.StatusCode, err)
	}

	if !env.Success {
		if env.Error != nil {
			return fmt.Errorf("storageclient: %s %s: %s: %s", method, path, env.Error.Code, env.Error.Message)
		}
		return fmt.Errorf("storageclient: %s %s: request failed (status %d)", method, path, resp.StatusCode)
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("storageclient: %s %s: decode data: %w", method, path, err)
		}
	}
	return nil
}
