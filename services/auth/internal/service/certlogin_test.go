package service

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// issueTestClientCert issues a leaf via the real CertService and returns its
// PEM (as nginx's $ssl_client_escaped_cert would deliver it, URL-escaped).
func issueTestClientCert(t *testing.T, svc *CertService, user *domain.User) (escapedPEM string, fingerprint string) {
	t.Helper()
	resp, err := svc.IssueCertificate(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	p12, _ := base64.StdEncoding.DecodeString(resp.P12Base64)
	_, leaf, _, err := pkcs12.DecodeChain(p12, resp.Password)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	return url.QueryEscape(string(pemBytes)), certFingerprint(leaf)
}

// newTestAuthServiceWithCerts wires an AuthService (fake cache + fake user
// getter, same shape as newTestAuthService) plus a *CertService sharing the
// fake user store so a test can register users visible to both.
func newTestAuthServiceWithCerts(t *testing.T) (*AuthService, *CertService, *fakeUserStore) {
	t.Helper()
	auth := newTestAuthService(t)
	users := auth.magicUserGetter.(*fakeUserStore)
	certs, _ := newTestCertService(t)
	return auth, certs, users
}

func TestHandshakeCertLogin_HappyPath(t *testing.T) {
	auth, certs, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-1", Username: "alice", CertAutoLogin: true}
	users.users[user.ID] = user

	escapedPEM, _ := issueTestClientCert(t, certs, user)

	token, err := auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if err != nil || token == "" {
		t.Fatalf("HandshakeCertLogin: token=%q err=%v", token, err)
	}

	resp, err := auth.ConsumeCertLoginToken(context.Background(), token, SessionContext{})
	if err != nil || resp == nil || resp.AccessToken == "" {
		t.Fatalf("first consume should succeed; got %v,%v", resp, err)
	}
	if resp.User == nil || resp.User.ID != user.ID {
		t.Fatalf("consumed session bound to wrong user: %+v", resp.User)
	}

	if _, err := auth.ConsumeCertLoginToken(context.Background(), token, SessionContext{}); err == nil {
		t.Fatalf("second consume must fail (single-use)")
	}
}

// TestHandshakeCertLogin_NilCADoesNotPanic guards against a nil CA cert
// reaching pool.AddCert (which panics on a nil *x509.Certificate) — e.g. a
// deployment where EnsureCA hasn't run yet. It must fail cleanly with
// Unauthorized instead of crashing the handler goroutine.
func TestHandshakeCertLogin_NilCADoesNotPanic(t *testing.T) {
	auth, wellFormedCerts, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-nilca", Username: "frank", CertAutoLogin: true}
	users.users[user.ID] = user
	// A validly-parseable leaf PEM, so the handler reaches the caCert nil
	// check instead of bailing out earlier on a PEM-decode failure.
	escapedPEM, _ := issueTestClientCert(t, wellFormedCerts, user)

	certs := &CertService{caStore: &fakeCAStore{}, certStore: newFakeCertStore()} // caCert left nil: CA never loaded

	_, err := auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeUnauthorized {
		t.Fatalf("want Unauthorized for nil CA; got %v", err)
	}
}

// TestConsumeCertLoginToken_ConcurrentSingleUse fires many concurrent
// consumes of the same one-time token and asserts exactly one wins — the
// finding this guards against is a non-atomic Get-then-Delete letting two
// racing consumers both read the token before either deletes it.
func TestConsumeCertLoginToken_ConcurrentSingleUse(t *testing.T) {
	auth, certs, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-race", Username: "eve", CertAutoLogin: true}
	users.users[user.ID] = user

	escapedPEM, _ := issueTestClientCert(t, certs, user)
	token, err := auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if err != nil || token == "" {
		t.Fatalf("HandshakeCertLogin: token=%q err=%v", token, err)
	}

	const workers = 20
	var successes int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			resp, err := auth.ConsumeCertLoginToken(context.Background(), token, SessionContext{})
			if err == nil && resp != nil {
				atomic.AddInt32(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("single-use token must be consumed exactly once under concurrency; got %d successes", successes)
	}
}

func TestHandshakeCertLogin_DisabledToggle(t *testing.T) {
	auth, certs, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-2", Username: "bob", CertAutoLogin: false}
	users.users[user.ID] = user

	escapedPEM, _ := issueTestClientCert(t, certs, user)

	_, err := auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if !errors.Is(err, ErrCertAutoLoginDisabled) {
		t.Fatalf("want ErrCertAutoLoginDisabled; got %v", err)
	}
}

func TestHandshakeCertLogin_RevokedCert(t *testing.T) {
	auth, certs, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-3", Username: "carol", CertAutoLogin: true}
	users.users[user.ID] = user

	resp, err := certs.IssueCertificate(context.Background(), user, "laptop")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	p12, _ := base64.StdEncoding.DecodeString(resp.P12Base64)
	_, leaf, _, err := pkcs12.DecodeChain(p12, resp.Password)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	escapedPEM := url.QueryEscape(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})))

	if err := certs.RevokeCertificate(context.Background(), resp.Certificate.ID, user.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeUnauthorized {
		t.Fatalf("want Unauthorized; got %v", err)
	}
}

func TestHandshakeCertLogin_ForeignCert(t *testing.T) {
	auth, certs, users := newTestAuthServiceWithCerts(t)
	user := &domain.User{ID: "u-4", Username: "dave", CertAutoLogin: true}
	users.users[user.ID] = user

	// Second CertService with its own fresh CA — cert issued there is
	// unrelated to the first service's trust chain.
	foreignCerts, _ := newTestCertService(t)
	escapedPEM, _ := issueTestClientCert(t, foreignCerts, user)

	_, err := auth.HandshakeCertLogin(context.Background(), "SUCCESS", escapedPEM, certs)
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeUnauthorized {
		t.Fatalf("want Unauthorized for foreign-CA cert; got %v", err)
	}

	// Even a valid PEM (from the RIGHT CA) must fail if verify != "SUCCESS".
	validEscapedPEM, _ := issueTestClientCert(t, certs, user)
	_, err = auth.HandshakeCertLogin(context.Background(), "NONE", validEscapedPEM, certs)
	if appErr, ok := liberrors.IsAppError(err); !ok || appErr.Code != liberrors.CodeUnauthorized {
		t.Fatalf("want Unauthorized for verify!=SUCCESS; got %v", err)
	}
}
