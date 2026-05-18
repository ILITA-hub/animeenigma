package allanime

import "net/http"

// SetHTTPClientForTest replaces the client's *http.Client. Used by
// cross-package tests in services/catalog/internal/service that
// point a real *allanime.Client at an httptest.Server without
// copying the in-package rewrite-transport scaffolding.
//
// Production code MUST NOT call this. The "ForTest" suffix is the
// project convention for test-only seams that must live in a
// non-_test.go file because the consumer lives in a different
// package (mirrors services/library/internal/metrics.GetJobsTotalForTest).
func SetHTTPClientForTest(c *Client, hc *http.Client) {
	c.httpClient = hc
}
