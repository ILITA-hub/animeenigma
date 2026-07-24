package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

type fakeCAStore struct {
	ca *domain.AuthCA
}

func (f *fakeCAStore) GetCA(ctx context.Context) (*domain.AuthCA, error) {
	if f.ca == nil {
		return nil, liberrors.NotFound("auth ca")
	}
	return f.ca, nil
}

func (f *fakeCAStore) SaveCA(ctx context.Context, ca *domain.AuthCA) error {
	f.ca = ca
	return nil
}

func TestEnsureCA_GeneratesAndReloads(t *testing.T) {
	store := &fakeCAStore{}
	svc := &CertService{caStore: store, log: logger.Default()}

	if err := svc.EnsureCA(context.Background()); err != nil {
		t.Fatalf("EnsureCA (generate): %v", err)
	}
	if store.ca == nil {
		t.Fatal("CA row not persisted")
	}
	block, _ := pem.Decode([]byte(store.ca.CertPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse generated CA cert: %v", err)
	}
	if !cert.IsCA {
		t.Fatal("generated cert is not a CA")
	}
	if cert.Subject.CommonName != "AnimeEnigma User CA" {
		t.Fatalf("CN = %q", cert.Subject.CommonName)
	}
	firstPEM := store.ca.CertPEM

	// Second call must load, not regenerate.
	svc2 := &CertService{caStore: store, log: logger.Default()}
	if err := svc2.EnsureCA(context.Background()); err != nil {
		t.Fatalf("EnsureCA (reload): %v", err)
	}
	if store.ca.CertPEM != firstPEM {
		t.Fatal("EnsureCA regenerated an existing CA")
	}
	if string(svc2.CAPEM()) != firstPEM {
		t.Fatal("CAPEM does not round-trip the stored PEM")
	}
}
