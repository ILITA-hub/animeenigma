package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // SHA-1 only for the display thumbprint (matches Windows' trust prompt)
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// caStore is satisfied by *repo.CertRepository and by in-package test fakes.
type caStore interface {
	GetCA(ctx context.Context) (*domain.AuthCA, error)
	SaveCA(ctx context.Context, ca *domain.AuthCA) error
}

// userCertStore is satisfied by *repo.CertRepository and by test fakes.
type userCertStore interface {
	CreateUserCert(ctx context.Context, c *domain.UserCertificate) error
	ListUserCerts(ctx context.Context, userID string) ([]domain.UserCertificate, error)
	GetByFingerprint(ctx context.Context, fp string) (*domain.UserCertificate, error)
	RevokeUserCert(ctx context.Context, id, userID string) error
	TouchUserCert(ctx context.Context, id string) error
}

const caCommonName = "AnimeEnigma User CA"

// caValidity ≈ 20 years — longer than any leaf (10y) it will ever sign.
const caValidity = 20 * 365 * 24 * time.Hour

// leafValidity ≈ 10 years, matching the platform's revoke-only session model.
const leafValidity = 10 * 365 * 24 * time.Hour

const maxCertsPerUser = 20

var timeNow = time.Now

// CertService owns the platform user-CA and everything signed by it.
// Constructed in main.go; EnsureCA must succeed before the router starts.
type CertService struct {
	caStore   caStore
	certStore userCertStore
	log       *logger.Logger

	mu     sync.RWMutex
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte
}

func NewCertService(store caStore, certStore userCertStore, log *logger.Logger) *CertService {
	return &CertService{caStore: store, certStore: certStore, log: log}
}

// EnsureCA loads the CA from the DB, generating and persisting a fresh one on
// first boot. Idempotent; safe to call on every startup. On a create race
// between two replicas, the loser re-reads the winner's row.
func (s *CertService) EnsureCA(ctx context.Context) error {
	row, err := s.caStore.GetCA(ctx)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); !ok || appErr.Code != errors.CodeNotFound {
			return fmt.Errorf("load ca: %w", err)
		}
		row, err = s.generateCA(ctx)
		if err != nil {
			return err
		}
	}
	return s.adopt(row)
}

func (s *CertService) generateCA(ctx context.Context) (*domain.AuthCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ca key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca serial: %w", err)
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: caCommonName, Organization: []string{"AnimeEnigma"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create ca cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal ca key: %w", err)
	}
	row := &domain.AuthCA{
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
	}
	if err := s.caStore.SaveCA(ctx, row); err != nil {
		// Lost a create race with another replica — adopt the winner's CA.
		if existing, gerr := s.caStore.GetCA(ctx); gerr == nil {
			return existing, nil
		}
		return nil, err
	}
	s.log.Infow("generated platform user CA", "cn", caCommonName)
	return row, nil
}

// adopt parses the row's PEMs into the in-memory signing state.
func (s *CertService) adopt(row *domain.AuthCA) error {
	certBlock, _ := pem.Decode([]byte(row.CertPEM))
	if certBlock == nil {
		return fmt.Errorf("ca cert pem: no block")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse ca cert: %w", err)
	}
	keyBlock, _ := pem.Decode([]byte(row.KeyPEM))
	if keyBlock == nil {
		return fmt.Errorf("ca key pem: no block")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse ca key: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.caCert = cert
	s.caKey = key
	s.caPEM = []byte(row.CertPEM)
	return nil
}

// CAPEM returns the CA certificate PEM (public — served at /cert/ca.pem and
// installed on the host for nginx ssl_client_certificate).
func (s *CertService) CAPEM() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.caPEM
}

// CAInfo returns the CA subject + fingerprints for user-facing display (the
// settings modal shows them next to the .p12 install instructions so users
// can verify the OS trust prompt).
func (s *CertService) CAInfo() domain.CAInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return domain.CAInfo{
		Subject:           s.caCert.Subject.String(),
		FingerprintSHA256: certFingerprint(s.caCert),
		FingerprintSHA1:   certFingerprintSHA1(s.caCert),
	}
}

// certFingerprint returns the lowercase sha256-hex of the cert's DER bytes.
func certFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// certFingerprintSHA1 returns the lowercase sha1-hex of the cert's DER bytes.
// Display-only (Windows' trust prompt shows a SHA-1 thumbprint) — never used
// for a security decision.
//
//nolint:gosec
func certFingerprintSHA1(cert *x509.Certificate) string {
	sum := sha1.Sum(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// generateP12Password returns a ~10-char human-typeable password. Charset
// avoids ambiguous glyphs (0/O, 1/l). Each character is sampled via
// rand.Int against the charset length to avoid modulo bias.
func generateP12Password() (string, error) {
	const charset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	charsetLen := big.NewInt(int64(len(charset)))
	b := make([]byte, 10)
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("rand: %w", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

// IssueCertificate creates a client cert for the user, persists its
// fingerprint mapping, and returns a password-protected PKCS#12 bundle.
// The private key is generated here and NEVER persisted server-side.
func (s *CertService) IssueCertificate(ctx context.Context, user *domain.User, name string) (*domain.IssueCertResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Certificate"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	existing, err := s.certStore.ListUserCerts(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	active := 0
	for _, c := range existing {
		if c.RevokedAt == nil {
			active++
		}
	}
	if active >= maxCertsPerUser {
		return nil, errors.InvalidInput("certificate limit reached")
	}

	s.mu.RLock()
	caCert, caKey := s.caCert, s.caKey
	s.mu.RUnlock()
	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("ca not initialized")
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("leaf serial: %w", err)
	}
	now := timeNow()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: user.Username, Organization: []string{"AnimeEnigma"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create leaf cert: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse leaf cert: %w", err)
	}

	password, err := generateP12Password()
	if err != nil {
		return nil, err
	}
	p12, err := pkcs12.Modern.Encode(leafKey, leaf, []*x509.Certificate{caCert}, password)
	if err != nil {
		return nil, fmt.Errorf("encode p12: %w", err)
	}

	row := &domain.UserCertificate{
		UserID:            user.ID,
		Name:              name,
		FingerprintSHA256: certFingerprint(leaf),
		Serial:            serial.Text(16),
		NotAfter:          leaf.NotAfter,
	}
	if err := s.certStore.CreateUserCert(ctx, row); err != nil {
		return nil, err
	}

	s.log.Infow("issued client certificate", "user_id", user.ID, "cert_id", row.ID, "name", name)
	metrics.AuthEventsTotal.WithLabelValues("cert_issued", "success").Inc()

	return &domain.IssueCertResponse{
		Certificate: row,
		P12Base64:   base64.StdEncoding.EncodeToString(p12),
		Password:    password,
	}, nil
}

func (s *CertService) ListCertificates(ctx context.Context, userID string) ([]domain.UserCertificate, error) {
	out, err := s.certStore.ListUserCerts(ctx, userID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.UserCertificate{}
	}
	return out, nil
}

func (s *CertService) RevokeCertificate(ctx context.Context, id, userID string) error {
	if err := s.certStore.RevokeUserCert(ctx, id, userID); err != nil {
		return err
	}
	s.log.Infow("revoked client certificate", "user_id", userID, "cert_id", id)
	metrics.AuthEventsTotal.WithLabelValues("cert_revoked", "user_action").Inc()
	return nil
}
