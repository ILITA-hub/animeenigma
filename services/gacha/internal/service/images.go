package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
	"github.com/google/uuid"
	xdraw "golang.org/x/image/draw"
)

const maxImageBytes = 10 << 20 // 10 MiB

const (
	maxDimension = 2048 // longest side after ingest; larger uploads are downscaled
	jpegQuality  = 85
)

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
	// allowPrivate is a TEST-ONLY escape hatch. When true the SSRF guards
	// (scheme + private/loopback IP rejection) are bypassed so unit tests can
	// reach an httptest server on 127.0.0.1. Never set in production.
	allowPrivate bool
}

// NewImageService creates an ImageService with a 10s HTTP client timeout and an
// SSRF-guarded dialer: the admin-supplied image URL (finding #20) is fetched by
// a client whose net.Dialer.Control rejects any post-DNS connection to a
// private/loopback/link-local address — closing DNS-rebind and redirect-to-
// internal bypasses on every hop, not just the initial host.
func NewImageService(store objectStore) *ImageService {
	s := &ImageService{store: store, maxBytes: maxImageBytes}
	s.client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
				Control:   s.dialControl,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return s
}

// dialControl is the net.Dialer.Control hook. It runs on the concrete ip:port
// the dialer is about to connect to (after DNS resolution), so it blocks
// private targets reached via a hostname or a redirect, unless the test escape
// hatch is set.
func (s *ImageService) dialControl(network, address string, c syscall.RawConn) error {
	if s.allowPrivate {
		return nil
	}
	return netguard.DenyPrivateControl(network, address, c)
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

// optimize recompresses ingested art: oversized images are downscaled to fit
// maxDimension, fully-opaque images become JPEG q85 (anime art PNGs are
// typically 8-10x smaller as JPEG), transparent ones stay PNG (JPEG has no
// alpha). GIF (animation) and WebP (no stdlib encoder) pass through, as does
// anything that fails to decode — optimization must never fail an upload.
// Returns (bytes, contentType, ext).
func optimize(buf []byte, ct string) ([]byte, string, string) {
	if ct != "image/png" && ct != "image/jpeg" {
		return buf, ct, extForCT(ct)
	}
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return buf, ct, extForCT(ct)
	}

	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	scaled := false
	if longest > maxDimension {
		scale := float64(maxDimension) / float64(longest)
		dst := image.NewRGBA(image.Rect(0, 0,
			int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		img = dst
		scaled = true
	}

	// Opaque images always normalize to JPEG: anime art PNGs are typically
	// 8-10x smaller as JPEG, and format normalization (one predictable
	// content-type for all opaque art) is worth it even on the rare
	// synthetic/flat-color input where JPEG's fixed block overhead loses to
	// PNG's deflate.
	if isOpaque(img) {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err == nil {
			return out.Bytes(), "image/jpeg", ".jpg"
		}
		return buf, ct, extForCT(ct)
	}

	// Transparency: only worth re-encoding when we actually downscaled.
	if scaled {
		var out bytes.Buffer
		if err := png.Encode(&out, img); err == nil && out.Len() < len(buf) {
			return out.Bytes(), "image/png", ".png"
		}
	}
	return buf, ct, extForCT(ct)
}

// isOpaque reports whether every pixel has full alpha.
func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

func extForCT(ct string) string {
	if ext, ok := allowedContentTypes[ct]; ok {
		return ext
	}
	return ""
}

// IngestFromURL downloads the image at rawURL and stores it under
// {kind}/{uuid}.{ext}. Returns the object key.
func (s *ImageService) IngestFromURL(ctx context.Context, rawURL, kind string) (string, error) {
	if !allowedKinds[kind] {
		return "", apperrors.InvalidInput(fmt.Sprintf("invalid kind %q (must be cards or banners)", kind))
	}

	// Cheap pre-flight (no DNS): scheme must be http/https and an IP-literal
	// host must not be private. The dial-time Control hook is the authoritative
	// rebind-safe layer; this just yields a fast, clear rejection.
	if !s.allowPrivate {
		if err := netguard.ValidatePublicURL(rawURL); err != nil {
			return "", apperrors.InvalidInput(fmt.Sprintf("disallowed image URL: %v", err))
		}
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
	if _, err := resolveExt(ct); err != nil {
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

	// Normalise the content type (strip "; charset=..." etc.) before handing
	// it to optimize — it must be the bare MIME the ext was resolved from.
	trimmedCT := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	buf2, ct2, ext2 := optimize(buf, trimmedCT)

	key := makeKey(kind, ext2)
	if err := s.store.Upload(ctx, key, bytes.NewReader(buf2), int64(len(buf2)), ct2); err != nil {
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

	buf2, ct2, ext2 := optimize(buf, ct)

	key := makeKey(kind, ext2)
	if err := s.store.Upload(ctx, key, bytes.NewReader(buf2), int64(len(buf2)), ct2); err != nil {
		return "", fmt.Errorf("store upload: %w", err)
	}
	return key, nil
}
