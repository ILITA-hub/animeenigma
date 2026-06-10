package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/google/uuid"
)

const maxImageBytes = 10 << 20 // 10 MiB

// objectStore is a thin adapter over *videoutils.Storage.Upload — defined here
// so tests can inject a fake without importing the MinIO client.
type objectStore interface {
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
}

// allowedContentTypes maps MIME types to file extensions.
var allowedContentTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// allowedKinds is the valid set of object key prefixes.
var allowedKinds = map[string]bool{
	"cards":   true,
	"banners": true,
}

// ImageService handles image ingest from a file upload or a remote URL,
// storing the result in MinIO under a predictable key.
type ImageService struct {
	store    objectStore
	client   *http.Client
	maxBytes int64
}

// NewImageService creates an ImageService with a 10s HTTP client timeout.
func NewImageService(store objectStore) *ImageService {
	return &ImageService{
		store: store,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxBytes: maxImageBytes,
	}
}

// resolveExt returns the file extension for a content type, or an error if
// the content type is not in the allowlist.
func resolveExt(contentType string) (string, error) {
	// Strip parameters like "; charset=utf-8"
	ct := strings.SplitN(contentType, ";", 2)[0]
	ct = strings.TrimSpace(ct)
	if ext, ok := allowedContentTypes[ct]; ok {
		return ext, nil
	}
	return "", apperrors.InvalidInput(fmt.Sprintf("unsupported image content type: %q (allowed: jpeg, png, webp, gif)", contentType))
}

// resolveExtFromFilename returns the extension from a filename, checking
// against the allowlist (by reversing ext→mime map).
func resolveExtFromFilename(filename string) (string, error) {
	lower := strings.ToLower(filename)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		if strings.HasSuffix(lower, ext) {
			// Normalise .jpeg → .jpg
			if ext == ".jpeg" {
				return ".jpg", nil
			}
			return ext, nil
		}
	}
	return "", apperrors.InvalidInput(fmt.Sprintf("unsupported image file extension for %q (allowed: jpg/jpeg, png, webp, gif)", filename))
}

// makeKey builds the object key: "{kind}/{uuid}{ext}".
func makeKey(kind, ext string) string {
	return fmt.Sprintf("%s/%s%s", kind, uuid.NewString(), ext)
}

// IngestFromURL downloads the image at rawURL and stores it under
// {kind}/{uuid}.{ext}. Returns the object key.
func (s *ImageService) IngestFromURL(ctx context.Context, rawURL, kind string) (string, error) {
	if !allowedKinds[kind] {
		return "", apperrors.InvalidInput(fmt.Sprintf("invalid kind %q (must be cards or banners)", kind))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", apperrors.InvalidInput(fmt.Sprintf("invalid URL: %v", err))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch image URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperrors.InvalidInput(fmt.Sprintf("remote server returned status %d", resp.StatusCode))
	}

	// Reject oversized responses early via Content-Length header.
	if resp.ContentLength > s.maxBytes {
		return "", apperrors.InvalidInput(
			fmt.Sprintf("image too large: Content-Length %d exceeds %d byte cap", resp.ContentLength, s.maxBytes))
	}

	ct := resp.Header.Get("Content-Type")
	ext, err := resolveExt(ct)
	if err != nil {
		return "", err
	}

	// Read the body up to maxBytes+1; if we read more than maxBytes it means
	// a lying Content-Length header — reject it.
	buf, err := io.ReadAll(io.LimitReader(resp.Body, s.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if int64(len(buf)) > s.maxBytes {
		return "", apperrors.InvalidInput(
			fmt.Sprintf("image exceeds %d byte cap", s.maxBytes))
	}

	key := makeKey(kind, ext)
	if err := s.store.Upload(ctx, key, bytes.NewReader(buf), int64(len(buf)), ct); err != nil {
		return "", fmt.Errorf("store image: %w", err)
	}
	return key, nil
}

// IngestUpload stores an uploaded file under {kind}/{uuid}.{ext}. Returns the key.
// contentType is the MIME type from the multipart header; filename is used as a
// fallback for extension resolution when contentType is generic.
func (s *ImageService) IngestUpload(ctx context.Context, file io.Reader, filename, contentType, kind string) (string, error) {
	if !allowedKinds[kind] {
		return "", apperrors.InvalidInput(fmt.Sprintf("invalid kind %q (must be cards or banners)", kind))
	}

	// Resolve extension: try content type first, then filename.
	ext, err := resolveExt(contentType)
	if err != nil {
		var extErr error
		ext, extErr = resolveExtFromFilename(filename)
		if extErr != nil {
			// Return the content-type error since it's more informative.
			return "", err
		}
	}

	// Read up to maxBytes+1 to enforce the cap.
	buf, err := io.ReadAll(io.LimitReader(file, s.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if int64(len(buf)) > s.maxBytes {
		return "", apperrors.InvalidInput(
			fmt.Sprintf("uploaded image exceeds %d byte cap", s.maxBytes))
	}

	// Normalise content type to the allowlisted MIME.
	ct := contentType
	if _, ok := allowedContentTypes[ct]; !ok {
		// Fall back to deriving from extension
		for mime, e := range allowedContentTypes {
			if e == ext {
				ct = mime
				break
			}
		}
	}

	key := makeKey(kind, ext)
	if err := s.store.Upload(ctx, key, bytes.NewReader(buf), int64(len(buf)), ct); err != nil {
		return "", fmt.Errorf("store upload: %w", err)
	}
	return key, nil
}
