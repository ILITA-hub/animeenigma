package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Download fetches a capability-signed input segment from the server's data
// plane and writes it to destPath.
//
// URL shape: {baseURL}?handle=<handle>&exp=<exp>&sig=<sig>
//
// The worker's worker_id is sent as X-Worker-Id so the server can bind the
// request to the lease holder (Task 11b follow-up: server-side check pending).
// If cfg carries an API key it is also sent as X-API-Key.
func Download(ctx context.Context, cfg Config, workerID, baseURL, handle, exp, sig, destPath string) error {
	u, err := appendCapabilityQuery(baseURL, handle, exp, sig)
	if err != nil {
		return fmt.Errorf("download: build URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("download: build request: %w", err)
	}
	setWorkerHeaders(req, cfg, workerID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, "download"); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("download: create dest file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("download: stream body: %w", err)
	}
	return nil
}

// Upload streams the file at srcPath to the server's data plane using a
// capability-signed PUT.
//
// URL shape: {baseURL}?handle=<handle>&exp=<exp>&sig=<sig>
//
// Same identity headers as Download.
func Upload(ctx context.Context, cfg Config, workerID, baseURL, handle, exp, sig, srcPath string) error {
	u, err := appendCapabilityQuery(baseURL, handle, exp, sig)
	if err != nil {
		return fmt.Errorf("upload: build URL: %w", err)
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("upload: open src file: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, f)
	if err != nil {
		return fmt.Errorf("upload: build request: %w", err)
	}
	setWorkerHeaders(req, cfg, workerID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload: request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, "upload"); err != nil {
		return err
	}
	return nil
}

// appendCapabilityQuery appends the three capability query parameters to
// baseURL. All three values are URL-escaped so base64 sigs (containing +/=)
// cannot corrupt the URL.
func appendCapabilityQuery(baseURL, handle, exp, sig string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("handle", handle)
	q.Set("exp", exp)
	q.Set("sig", sig)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// setWorkerHeaders attaches the worker identity and optional API-key headers
// to req.
func setWorkerHeaders(req *http.Request, cfg Config, workerID string) {
	// Send worker identity so the server can bind the request to the lease.
	if workerID != "" {
		req.Header.Set("X-Worker-Id", workerID)
	}
	// API key for edge auth (CF mTLS replaces this in a future hardening pass).
	if cfg.APIKey != "" {
		req.Header.Set("X-API-Key", cfg.APIKey)
	}
}

// checkStatus returns a typed error for non-2xx HTTP responses.
func checkStatus(resp *http.Response, op string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: 401 unauthorized (session may have expired)", op)
	case http.StatusForbidden:
		return fmt.Errorf("%s: 403 forbidden", op)
	case http.StatusNotFound:
		return fmt.Errorf("%s: 404 not found", op)
	case http.StatusConflict:
		return fmt.Errorf("%s: 409 conflict (segment already uploaded)", op)
	case http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%s: 413 request entity too large", op)
	default:
		return fmt.Errorf("%s: unexpected status %d", op, resp.StatusCode)
	}
}
