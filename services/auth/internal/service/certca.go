package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// caStore is satisfied by *repo.CertRepository and by in-package test fakes.
type caStore interface {
	GetCA(ctx context.Context) (*domain.AuthCA, error)
	SaveCA(ctx context.Context, ca *domain.AuthCA) error
}

const caCommonName = "AnimeEnigma User CA"

// caValidity ≈ 20 years — longer than any leaf (10y) it will ever sign.
const caValidity = 20 * 365 * 24 * time.Hour

// CertService owns the platform user-CA and everything signed by it.
// Constructed in main.go; EnsureCA must succeed before the router starts.
type CertService struct {
	caStore caStore
	log     *logger.Logger

	mu     sync.RWMutex
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte
}

func NewCertService(store caStore, log *logger.Logger) *CertService {
	return &CertService{caStore: store, log: log}
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
