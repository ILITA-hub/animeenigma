// Package domain holds the wire types for the storage service. These are a
// contract: the client lib and every consumer service built in later tasks
// decode these exact JSON shapes, so field names/tags must not drift without
// updating every consumer.
package domain

// Backend ids — the two S3-compatible object stores the service brokers.
const (
	BackendMinio = "minio"
	BackendS3    = "s3"
)

// Content classes — the placement policy keys. See service.Placement.
const (
	ClassLibraryAuto   = "library-auto"
	ClassLibraryManual = "library-manual"
	ClassUpscaled      = "upscaled"
)

// IngestURLsRequest is the body of POST /internal/storage/ingest-urls.
type IngestURLsRequest struct {
	Class    string   `json:"class"`
	Prefix   string   `json:"prefix"`   // trailing slash, bucket-relative
	Files    []string `json:"files"`    // basenames
	Override string   `json:"override"` // "", "minio", "s3" — only honored for library-manual
}

// PutURL is one presigned PUT entry in IngestURLsResponse.
type PutURL struct {
	Name string `json:"name"`
	URL  string `json:"put_url"`
}

// IngestURLsResponse is the response of POST /internal/storage/ingest-urls.
type IngestURLsResponse struct {
	Storage   string   `json:"storage"`
	URLs      []PutURL `json:"urls"`
	ExpiresIn int      `json:"expires_in"` // seconds
}

// DownloadURLsRequest is the body of POST /internal/storage/download-urls.
type DownloadURLsRequest struct {
	Storage string `json:"storage"`
	Prefix  string `json:"prefix"`
}

// GetURL is one presigned GET entry in DownloadURLsResponse.
type GetURL struct {
	Name string `json:"name"` // key relative to the requested prefix
	URL  string `json:"get_url"`
}

// DownloadURLsResponse is the response of POST /internal/storage/download-urls.
type DownloadURLsResponse struct {
	URLs []GetURL `json:"urls"`
}

// MoveRequest is the body of POST /internal/storage/move.
type MoveRequest struct {
	Storage    string `json:"storage"`
	FromPrefix string `json:"from_prefix"`
	ToPrefix   string `json:"to_prefix"`
}

// MoveResponse is the response of POST /internal/storage/move.
type MoveResponse struct {
	Moved int `json:"moved"`
}

// CopyPrefixRequest is the body of POST /internal/storage/copy.
type CopyPrefixRequest struct {
	FromStorage string `json:"from_storage"`
	ToStorage   string `json:"to_storage"`
	Prefix      string `json:"prefix"`
}

// CopyResponse is the response of POST /internal/storage/copy.
type CopyResponse struct {
	Copied int   `json:"copied"`
	Bytes  int64 `json:"bytes"`
}

// DeletePrefixRequest is the body of DELETE /internal/storage/prefix.
type DeletePrefixRequest struct {
	Storage string `json:"storage"`
	Prefix  string `json:"prefix"`
}

// DeleteResponse is the response of DELETE /internal/storage/prefix.
type DeleteResponse struct {
	Deleted int `json:"deleted"`
}

// Object is one bucket-relative key + size, as returned by List.
type Object struct {
	Key  string `json:"key"` // bucket-relative
	Size int64  `json:"size"`
}

// ListResponse is the response of GET /internal/storage/list.
type ListResponse struct {
	Objects []Object `json:"objects"`
}
