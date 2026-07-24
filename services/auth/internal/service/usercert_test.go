package service

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

type fakeCertStore struct {
	rows map[string]*domain.UserCertificate // by fingerprint
}

func newFakeCertStore() *fakeCertStore {
	return &fakeCertStore{rows: map[string]*domain.UserCertificate{}}
}

func (f *fakeCertStore) CreateUserCert(ctx context.Context, c *domain.UserCertificate) error {
	c.ID = "cert-" + c.FingerprintSHA256[:8]
	f.rows[c.FingerprintSHA256] = c
	return nil
}

func (f *fakeCertStore) ListUserCerts(ctx context.Context, userID string) ([]domain.UserCertificate, error) {
	var out []domain.UserCertificate
	for _, c := range f.rows {
		if c.UserID == userID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeCertStore) GetByFingerprint(ctx context.Context, fp string) (*domain.UserCertificate, error) {
	c, ok := f.rows[fp]
	if !ok || c.RevokedAt != nil {
		return nil, liberrors.NotFound("certificate")
	}
	return c, nil
}

func (f *fakeCertStore) RevokeUserCert(ctx context.Context, id, userID string) error {
	for _, c := range f.rows {
		if c.ID == id && c.UserID == userID && c.RevokedAt == nil {
			now := timeNow()
			c.RevokedAt = &now
			return nil
		}
	}
	return liberrors.NotFound("certificate")
}

func (f *fakeCertStore) TouchUserCert(ctx context.Context, id string) error { return nil }

func newTestCertService(t *testing.T) (*CertService, *fakeCertStore) {
	t.Helper()
	store := newFakeCertStore()
	svc := &CertService{caStore: &fakeCAStore{}, certStore: store, log: logger.Default()}
	if err := svc.EnsureCA(context.Background()); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	return svc, store
}

func TestIssueCertificate_SignedByCA(t *testing.T) {
	svc, store := newTestCertService(t)
	user := &domain.User{ID: "u-1", Username: "alice"}

	resp, err := svc.IssueCertificate(context.Background(), user, "laptop")
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	if resp.Password == "" || len(resp.Password) < 8 {
		t.Fatalf("weak p12 password %q", resp.Password)
	}

	p12, err := base64.StdEncoding.DecodeString(resp.P12Base64)
	if err != nil {
		t.Fatalf("p12 base64: %v", err)
	}
	key, leaf, caCerts, err := pkcs12.DecodeChain(p12, resp.Password)
	if err != nil {
		t.Fatalf("decode p12 with returned password: %v", err)
	}
	if key == nil || len(caCerts) != 1 {
		t.Fatalf("p12 missing key or CA chain (ca=%d)", len(caCerts))
	}
	if leaf.Subject.CommonName != "alice" {
		t.Fatalf("leaf CN = %q", leaf.Subject.CommonName)
	}

	// Leaf must verify against the CA.
	pool := x509.NewCertPool()
	pool.AddCert(caCerts[0])
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("leaf does not verify against CA: %v", err)
	}

	// Fingerprint row persisted for the right user.
	fp := certFingerprint(leaf)
	row, err := store.GetByFingerprint(context.Background(), fp)
	if err != nil {
		t.Fatalf("fingerprint row missing: %v", err)
	}
	if row.UserID != "u-1" || row.Name != "laptop" {
		t.Fatalf("row = %+v", row)
	}
}

func TestRevokeCertificate(t *testing.T) {
	svc, store := newTestCertService(t)
	user := &domain.User{ID: "u-1", Username: "alice"}
	resp, err := svc.IssueCertificate(context.Background(), user, "laptop")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := svc.RevokeCertificate(context.Background(), resp.Certificate.ID, "u-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.GetByFingerprint(context.Background(), resp.Certificate.FingerprintSHA256); err == nil {
		t.Fatal("revoked cert still resolvable by fingerprint")
	}
}
