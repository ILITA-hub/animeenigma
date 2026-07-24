package service

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// ErrCertAutoLoginDisabled: the cert is valid and known, but the user has the
// auto-login toggle off. The handler maps this to a 403 the frontend
// negative-caches (vs the generic 401 it retries on next visit).
var ErrCertAutoLoginDisabled = errors.New(errors.CodeForbidden, "cert auto-login disabled")

// certLoginSession is the Redis value behind a one-time cert-login token.
type certLoginSession struct {
	UserID string `json:"user_id"`
	CertID string `json:"cert_id"`
}

const certLoginTokenPrefix = "cl_"

// HandshakeCertLogin validates a client cert presented on the mTLS vhost and
// mints a one-time main-origin login token. verify is nginx's
// $ssl_client_verify ("SUCCESS" on a validated optional handshake);
// escapedPEM is $ssl_client_escaped_cert (URL-escaped PEM). certs provides
// the CA pool and the fingerprint→user mapping.
//
// Defense in depth: the route is only reachable via the mTLS vhost (root mux,
// never proxied by the gateway), AND the PEM's signature chain is re-verified
// here against the platform CA — a forged header without a CA-signed cert
// fails either way.
func (s *AuthService) HandshakeCertLogin(ctx context.Context, verify, escapedPEM string, certs *CertService) (string, error) {
	if verify != "SUCCESS" {
		metrics.AuthEventsTotal.WithLabelValues("cert_login", "handshake_rejected").Inc()
		return "", errors.Unauthorized("client certificate required")
	}
	rawPEM, err := url.QueryUnescape(escapedPEM)
	if err != nil || strings.TrimSpace(rawPEM) == "" {
		return "", errors.Unauthorized("client certificate required")
	}
	block, _ := pem.Decode([]byte(rawPEM))
	if block == nil {
		return "", errors.Unauthorized("client certificate required")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", errors.Unauthorized("client certificate required")
	}

	certs.mu.RLock()
	caCert := certs.caCert
	certs.mu.RUnlock()
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		metrics.AuthEventsTotal.WithLabelValues("cert_login", "chain_rejected").Inc()
		return "", errors.Unauthorized("unknown certificate")
	}

	row, err := certs.certStore.GetByFingerprint(ctx, certFingerprint(leaf))
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("cert_login", "fingerprint_unknown").Inc()
		return "", errors.Unauthorized("unknown certificate")
	}
	user, err := s.magicUserGetter.GetByID(ctx, row.UserID)
	if err != nil {
		return "", errors.Unauthorized("unknown certificate")
	}
	if !user.CertAutoLogin {
		return "", ErrCertAutoLoginDisabled
	}

	_ = certs.certStore.TouchUserCert(ctx, row.ID)

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	token := certLoginTokenPrefix + hex.EncodeToString(b)
	val := &certLoginSession{UserID: user.ID, CertID: row.ID}
	if err := s.cache.Set(ctx, cache.KeyCertLogin(token), val, cache.TTLCertLogin); err != nil {
		return "", fmt.Errorf("store cert login token: %w", err)
	}
	metrics.AuthEventsTotal.WithLabelValues("cert_login", "token_minted").Inc()
	return token, nil
}

// ConsumeCertLoginToken redeems a one-time cert-login token for a session.
func (s *AuthService) ConsumeCertLoginToken(ctx context.Context, token string, sc SessionContext) (*domain.AuthResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.Unauthorized("invalid token")
	}
	var val certLoginSession
	if err := s.cache.Get(ctx, cache.KeyCertLogin(token), &val); err != nil {
		return nil, errors.Unauthorized("token not found or expired")
	}
	// Single-use: delete before minting so a replay finds nothing.
	_ = s.cache.Delete(ctx, cache.KeyCertLogin(token))

	user, err := s.magicUserGetter.GetByID(ctx, val.UserID)
	if err != nil {
		return nil, errors.Unauthorized("token user missing")
	}
	metrics.AuthEventsTotal.WithLabelValues("cert_login", "success").Inc()
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
