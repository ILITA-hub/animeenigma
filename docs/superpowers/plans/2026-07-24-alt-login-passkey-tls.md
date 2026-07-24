# Passkey + TLS Client-Cert Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add usernameless WebAuthn passkey login and TLS client-certificate auto-login per the approved spec `docs/superpowers/specs/2026-07-24-alt-login-passkey-tls-design.md`.

**Architecture:** Both methods terminate in the existing `createSessionAndAuthResponse` seam in `services/auth`. Passkeys use `github.com/go-webauthn/webauthn` with Redis ceremony state. Cert login uses a platform CA owned by the auth service; the mTLS handshake happens on a dedicated host-nginx vhost `cert.animeenigma.org` that proxies straight to auth's loopback port; the browser exchanges a 60s one-time token on the main origin for normal cookies.

**Tech Stack:** Go (chi, GORM, Redis via libs/cache), `go-webauthn/webauthn`, `software.sslmate.com/src/go-pkcs12`, Vue 3 + Pinia + `@simplewebauthn/browser`, host nginx.

## Global Constraints

- ALL work in the worktree **`/tmp/ae-alt-login`** (branch `feat-alt-login`). NEVER edit `/data/animeenigma`. Use absolute paths under `/tmp/ae-alt-login` in every edit (a relative path resolving into the base tree is a known failure mode).
- Commit per task with pathspecs (`git commit -m "..." -- <files>`), conventional-commit style. Push to main only in the final task.
- Go: shared `libs/errors` for domain errors; `libs/logger` structured logging; existing `metrics.AuthEventsTotal` counter (do NOT create new promauto counters — libs/metrics auto-registration trap).
- NEVER run `gofmt -w` / `goimports -w` on files containing Cyrillic comments (smart-quote landmine); format via targeted edits only.
- Frontend: `bun` (not npm), design-system tokens only (DS-lint hook fires on every edit), i18n parity en/ru/ja for every new key.
- No time-based effort units anywhere; feature metrics already scored in the spec.
- Env defaults must keep dev working with zero new env vars set: `WEBAUTHN_RP_ID` default `animeenigma.org`, `WEBAUTHN_RP_ORIGINS` default `https://animeenigma.org`, `VITE_CERT_LOGIN_BASE` default empty (probe disabled).

---

### Task 1: Cache key helpers (libs/cache)

**Files:**
- Modify: `libs/cache/keyclass.go` (append), `libs/cache/ttl.go` (append)
- Test: `libs/cache/keyclass_test.go` (append)

**Interfaces:**
- Produces: `cache.KeyCertLogin(token string) string` (`"certlogin:"+token`), `cache.TTLCertLogin = 60 * time.Second`, `cache.KeyWebAuthnCeremony(id string) string` (`"webauthn:"+id`), `cache.TTLWebAuthnCeremony = 5 * time.Minute`.

- [ ] **Step 1: Add key funcs + TTLs.** In `keyclass.go`, next to `KeyXDomainMagic`:

```go
// KeyCertLogin is the Redis key for a one-time TLS-cert login handoff token
// (minted by /cert/handshake-login, consumed by /api/auth/cert/consume).
func KeyCertLogin(token string) string {
	return "certlogin:" + token
}

// KeyWebAuthnCeremony is the Redis key for in-flight WebAuthn ceremony state
// (registration or login), keyed by a random ceremony id.
func KeyWebAuthnCeremony(id string) string {
	return "webauthn:" + id
}
```

In `ttl.go`, next to `TTLXDomainMagic`:

```go
	// TTLCertLogin bounds the window between a successful mTLS handshake on
	// cert.animeenigma.org and the main-origin cookie exchange.
	TTLCertLogin = 60 * time.Second
	// TTLWebAuthnCeremony bounds a WebAuthn register/login ceremony.
	TTLWebAuthnCeremony = 5 * time.Minute
```

- [ ] **Step 2: Test.** Append to `keyclass_test.go` (mirror the existing key-func test style in that file — assert exact prefixes):

```go
func TestKeyCertLogin(t *testing.T) {
	if got := KeyCertLogin("abc"); got != "certlogin:abc" {
		t.Fatalf("KeyCertLogin = %q", got)
	}
}

func TestKeyWebAuthnCeremony(t *testing.T) {
	if got := KeyWebAuthnCeremony("abc"); got != "webauthn:abc" {
		t.Fatalf("KeyWebAuthnCeremony = %q", got)
	}
}
```

- [ ] **Step 3: Run.** `cd /tmp/ae-alt-login/libs/cache && go test ./...` → PASS.
- [ ] **Step 4: Commit.** `git commit -m "feat(cache): cert-login + webauthn ceremony keys" -- libs/cache`

---

### Task 2: Platform CA bootstrap (auth service)

**Files:**
- Create: `services/auth/internal/domain/cert.go`, `services/auth/internal/repo/cert.go`, `services/auth/internal/service/certca.go`
- Test: `services/auth/internal/service/certca_test.go`

**Interfaces:**
- Produces:
  - `domain.AuthCA{ID int, CertPEM string, KeyPEM string, CreatedAt time.Time}` (table `auth_ca`, single row `ID=1`)
  - `domain.UserCertificate{ID, UserID, Name, FingerprintSHA256, Serial, NotAfter, CreatedAt, LastUsedAt, RevokedAt}` (table `user_certificates`)
  - `repo.CertRepository` with `GetCA`, `SaveCA`, `CreateUserCert`, `ListUserCerts(userID)`, `GetByFingerprint(fp)`, `RevokeUserCert(id, userID)`, `TouchUserCert(id)`
  - `service.CertService` struct with `EnsureCA(ctx) error` (idempotent) and `CAPEM() []byte`; internal parsed `caCert *x509.Certificate`, `caKey *ecdsa.PrivateKey`

- [ ] **Step 1: Domain models.** `services/auth/internal/domain/cert.go`:

```go
package domain

import (
	"time"
)

// AuthCA is the single-row platform user-CA (id=1). The private key lives in
// the DB on purpose: same trust domain as password hashes, backed up with the
// DB, and the auth service is its only consumer.
type AuthCA struct {
	ID        int       `gorm:"primaryKey" json:"-"`
	CertPEM   string    `gorm:"type:text" json:"-"`
	KeyPEM    string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"-"`
}

func (AuthCA) TableName() string { return "auth_ca" }

// UserCertificate maps an issued client certificate to its user. Authorization
// is ALWAYS by fingerprint — the cert subject is display-only.
type UserCertificate struct {
	ID                string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID            string     `gorm:"type:uuid;index" json:"-"`
	Name              string     `gorm:"size:64" json:"name"`
	FingerprintSHA256 string     `gorm:"size:64;uniqueIndex" json:"-"`
	Serial            string     `gorm:"size:64" json:"serial"`
	NotAfter          time.Time  `json:"not_after"`
	CreatedAt         time.Time  `json:"created_at"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
}

// IssueCertResponse is returned by POST /api/auth/cert/issue.
type IssueCertResponse struct {
	Certificate *UserCertificate `json:"certificate"`
	// P12Base64 is the PKCS#12 bundle (cert+key+CA), base64-encoded for JSON.
	P12Base64 string `json:"p12_base64"`
	// Password protects the P12 (iOS refuses empty-password imports). Shown
	// to the user exactly once.
	Password string `json:"password"`
}

// UpdateCertAutoLoginRequest toggles User.CertAutoLogin.
type UpdateCertAutoLoginRequest struct {
	Enabled bool `json:"enabled"`
}
```

- [ ] **Step 2: `User.CertAutoLogin` column.** In `services/auth/internal/domain/user.go`, add to the `User` struct after `ApiKeyHash`:

```go
	// CertAutoLogin: when true, a valid client-cert handshake on the mTLS
	// vhost silently logs this user in (spec 2026-07-24). Server-side SSoT.
	CertAutoLogin bool `gorm:"default:false" json:"cert_auto_login"`
```

- [ ] **Step 3: Repo.** `services/auth/internal/repo/cert.go` (mirror `session.go` style):

```go
package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

type CertRepository struct {
	db *gorm.DB
}

func NewCertRepository(db *gorm.DB) *CertRepository {
	return &CertRepository{db: db}
}

// GetCA returns the single CA row, or NotFound when none exists yet.
func (r *CertRepository) GetCA(ctx context.Context) (*domain.AuthCA, error) {
	var ca domain.AuthCA
	if err := r.db.WithContext(ctx).First(&ca, "id = 1").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("auth ca")
		}
		return nil, fmt.Errorf("get ca: %w", err)
	}
	return &ca, nil
}

// SaveCA inserts the CA row. A concurrent duplicate insert (two replicas
// booting at once) fails on the primary key — callers treat that as "someone
// else won, re-read".
func (r *CertRepository) SaveCA(ctx context.Context, ca *domain.AuthCA) error {
	ca.ID = 1
	if err := r.db.WithContext(ctx).Create(ca).Error; err != nil {
		return fmt.Errorf("save ca: %w", err)
	}
	return nil
}

func (r *CertRepository) CreateUserCert(ctx context.Context, c *domain.UserCertificate) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("create user cert: %w", err)
	}
	return nil
}

func (r *CertRepository) ListUserCerts(ctx context.Context, userID string) ([]domain.UserCertificate, error) {
	var out []domain.UserCertificate
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list user certs: %w", err)
	}
	return out, nil
}

// GetByFingerprint returns the non-revoked cert row for a fingerprint.
func (r *CertRepository) GetByFingerprint(ctx context.Context, fp string) (*domain.UserCertificate, error) {
	var c domain.UserCertificate
	err := r.db.WithContext(ctx).
		Where("fingerprint_sha256 = ? AND revoked_at IS NULL", fp).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("certificate")
		}
		return nil, fmt.Errorf("get cert by fingerprint: %w", err)
	}
	return &c, nil
}

// RevokeUserCert marks the user's cert revoked. NotFound when the row is not
// the caller's or already revoked (idempotent from the UI's point of view).
func (r *CertRepository) RevokeUserCert(ctx context.Context, id, userID string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserCertificate{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", time.Now())
	if res.Error != nil {
		return fmt.Errorf("revoke cert: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("certificate")
	}
	return nil
}

func (r *CertRepository) TouchUserCert(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domain.UserCertificate{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now()).Error
}
```

- [ ] **Step 4: Failing test for CA bootstrap.** `services/auth/internal/service/certca_test.go`. Use an in-package fake store (magiclink pattern — no live DB):

```go
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
```

- [ ] **Step 5: Run to verify failure.** `cd /tmp/ae-alt-login/services/auth && go test ./internal/service/ -run TestEnsureCA -v` → FAIL (CertService undefined).

- [ ] **Step 6: Implement.** `services/auth/internal/service/certca.go`:

```go
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
```

- [ ] **Step 7: Run tests.** `go test ./internal/service/ -run TestEnsureCA -v` → PASS. Then `go build ./...`.
- [ ] **Step 8: Commit.** `git commit -m "feat(auth): platform user-CA bootstrap + cert domain/repo" -- services/auth`

---

### Task 3: Certificate issuance, listing, revocation (.p12)

**Files:**
- Modify: `services/auth/internal/service/certca.go` (extend `CertService`), `services/auth/go.mod` (dep)
- Test: `services/auth/internal/service/usercert_test.go`

**Interfaces:**
- Consumes: `CertService.caCert/caKey` (Task 2), `repo.CertRepository` methods.
- Produces:
  - `CertService.IssueCertificate(ctx, user *domain.User, name string) (*domain.IssueCertResponse, error)` — leaf validity 10y, ECDSA P-256, subject `CN=<username>`; stores fingerprint row via `userCertStore`; returns p12 (base64) + one-time password.
  - `CertService.ListCertificates(ctx, userID) ([]domain.UserCertificate, error)`
  - `CertService.RevokeCertificate(ctx, id, userID string) error`
  - interface `userCertStore { CreateUserCert; ListUserCerts; GetByFingerprint; RevokeUserCert; TouchUserCert }` — add field `certStore userCertStore` to `CertService`, populate from `repo.CertRepository` in main.go; test fake in-package.

- [ ] **Step 1: Add dep.** `cd /tmp/ae-alt-login/services/auth && go get software.sslmate.com/src/go-pkcs12@v0.4.0 && go mod tidy` (NEVER `go work sync`).

- [ ] **Step 2: Failing test.** `usercert_test.go`:

```go
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
```

Note: the test uses `timeNow()` — define `var timeNow = time.Now` in `certca.go` (tiny seam, used by the fake only; production code keeps calling `time.Now()` directly elsewhere). The test also reads `resp.Certificate.FingerprintSHA256` — populate it in the returned struct even though the JSON tag hides it.

- [ ] **Step 3: Run to verify failure.** `go test ./internal/service/ -run 'TestIssue|TestRevoke' -v` → FAIL (IssueCertificate undefined).

- [ ] **Step 4: Implement.** Append to `certca.go`:

```go
// userCertStore is satisfied by *repo.CertRepository and by test fakes.
type userCertStore interface {
	CreateUserCert(ctx context.Context, c *domain.UserCertificate) error
	ListUserCerts(ctx context.Context, userID string) ([]domain.UserCertificate, error)
	GetByFingerprint(ctx context.Context, fp string) (*domain.UserCertificate, error)
	RevokeUserCert(ctx context.Context, id, userID string) error
	TouchUserCert(ctx context.Context, id string) error
}

var timeNow = time.Now

// leafValidity ≈ 10 years, matching the platform's revoke-only session model.
const leafValidity = 10 * 365 * 24 * time.Hour

const maxCertsPerUser = 20

// certFingerprint returns the lowercase sha256-hex of the cert's DER bytes.
func certFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// generateP12Password returns a ~10-char human-typeable password. Charset
// avoids ambiguous glyphs (0/O, 1/l).
func generateP12Password() (string, error) {
	const charset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
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
```

Add the needed imports to `certca.go` (`crypto/sha256`, `encoding/base64`, `encoding/hex`, `strings`, `github.com/ILITA-hub/animeenigma/libs/metrics`, `pkcs12 "software.sslmate.com/src/go-pkcs12"`). Add `certStore userCertStore` field to the `CertService` struct and a `store` param to `NewCertService`:

```go
func NewCertService(store caStore, certStore userCertStore, log *logger.Logger) *CertService {
	return &CertService{caStore: store, certStore: certStore, log: log}
}
```

(Update the Task 2 test's constructor usage if the compiler complains — the tests build the struct literal directly, which keeps working.)

- [ ] **Step 5: Run tests.** `go test ./internal/service/ -run 'TestIssue|TestRevoke|TestEnsureCA' -v` → PASS.
- [ ] **Step 6: Commit.** `git commit -m "feat(auth): client-cert issuance (.p12), listing, revocation" -- services/auth`

---

### Task 4: Handshake login + one-time token consume

**Files:**
- Create: `services/auth/internal/service/certlogin.go`
- Test: `services/auth/internal/service/certlogin_test.go`

**Interfaces:**
- Consumes: `CertService` (CA + fingerprint store), `AuthService.createSessionAndAuthResponse`, `cache.KeyCertLogin`/`TTLCertLogin` (Task 1).
- Produces (methods on `AuthService`, taking the cert service as a collaborator):
  - `AuthService.HandshakeCertLogin(ctx, verify string, escapedCertPEM string, certs *CertService) (token string, err error)` — errors: `errors.Unauthorized` (bad/unknown/revoked cert), `ErrCertAutoLoginDisabled` (sentinel).
  - `AuthService.ConsumeCertLoginToken(ctx, token string, sc SessionContext) (*domain.AuthResponse, error)`
  - `var ErrCertAutoLoginDisabled = errors.New(errors.CodeForbidden, "cert auto-login disabled")` — handler maps it to 403 `{"reason":"auto_login_disabled"}`.

- [ ] **Step 1: Failing tests.** `certlogin_test.go`. Reuse `fakeCAStore`/`fakeCertStore`; add a tiny in-memory `cache.Cache` fake if the package doesn't already have one (check `magiclink_test.go` first — it exercises MintMagicToken with a fake cache; REUSE that fake type). Test cases:

```go
package service

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/url"
	"testing"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

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
```

Then four tests (each builds `AuthService` the way `magiclink_test.go` does — fake cache + fake user getter/session store):

1. `TestHandshakeCertLogin_HappyPath` — user with `CertAutoLogin: true`; expect a token back; `ConsumeCertLoginToken(token)` returns an `AuthResponse` for that user; second consume of the same token errors (single-use).
2. `TestHandshakeCertLogin_DisabledToggle` — user with `CertAutoLogin: false` → `ErrCertAutoLoginDisabled`.
3. `TestHandshakeCertLogin_RevokedCert` — revoke the fingerprint first → `Unauthorized`.
4. `TestHandshakeCertLogin_ForeignCert` — cert signed by a DIFFERENT CA (spin up a second `CertService` with its own fresh `fakeCAStore`, issue there, present to the first) → `Unauthorized`. Also: `verify` header ≠ `"SUCCESS"` → `Unauthorized` even with a valid PEM.

- [ ] **Step 2: Run to verify failure.** `go test ./internal/service/ -run TestHandshakeCertLogin -v` → FAIL.

- [ ] **Step 3: Implement.** `certlogin.go`:

```go
package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
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
	if _, err := cryptoRandRead(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	token := certLoginTokenPrefix + hexEncode(b)
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
```

Use the real helpers instead of the pseudo-names: `cryptoRandRead` → `rand.Read` from `crypto/rand`, `hexEncode` → `hex.EncodeToString` (imports `crypto/rand`, `encoding/hex`). Add the missing `domain` import.

- [ ] **Step 4: Run tests.** `go test ./internal/service/ -v -run 'CertLogin'` → PASS; full `go test ./...` in `services/auth` → PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat(auth): mTLS handshake login + one-time token consume" -- services/auth`

---

### Task 5: WebAuthn passkey service

**Files:**
- Create: `services/auth/internal/domain/passkey.go`, `services/auth/internal/repo/passkey.go`, `services/auth/internal/service/passkey.go`
- Modify: `services/auth/internal/config/config.go` (WebAuthn config), `services/auth/go.mod` (dep)
- Test: `services/auth/internal/service/passkey_test.go`

**Interfaces:**
- Produces:
  - `domain.WebAuthnCredential{ID string(uuid PK), UserID string(index), CredentialID string(base64url, uniqueIndex), PublicKey []byte, SignCount uint32, Transports string, AAGUID []byte, Name string, CreatedAt, LastUsedAt *time.Time}` table `webauthn_credentials`
  - `repo.PasskeyRepository{Create, ListByUser, GetByCredentialID, UpdateSignCount(id string, count uint32, lastUsed time.Time), Delete(id, userID)}`
  - `service.PasskeyService` with:
    - `BeginRegistration(ctx, user *domain.User) (opts *protocol.CredentialCreation, ceremonyID string, err error)`
    - `FinishRegistration(ctx, user *domain.User, ceremonyID, name string, r *http.Request) (*domain.WebAuthnCredential, error)`
    - `BeginLogin(ctx) (opts *protocol.CredentialAssertion, ceremonyID string, err error)` (discoverable)
    - `FinishLogin(ctx, ceremonyID string, r *http.Request) (*domain.User, error)`
    - `List(ctx, userID)`, `Delete(ctx, id, userID)`
  - config: `WebAuthnConfig{RPID string, RPOrigins []string}` on `Config` (env `WEBAUTHN_RP_ID` default `animeenigma.org`; `WEBAUTHN_RP_ORIGINS` comma-separated, default `https://animeenigma.org`)

- [ ] **Step 1: Add dep.** `cd /tmp/ae-alt-login/services/auth && go get github.com/go-webauthn/webauthn@v0.11.2 && go mod tidy`

- [ ] **Step 2: Config.** In `config.go` add to `Config`: `WebAuthn WebAuthnConfig`; define:

```go
type WebAuthnConfig struct {
	RPID      string
	RPOrigins []string
}
```

and in `Load()`:

```go
		WebAuthn: WebAuthnConfig{
			RPID:      getEnv("WEBAUTHN_RP_ID", "animeenigma.org"),
			RPOrigins: strings.Split(getEnv("WEBAUTHN_RP_ORIGINS", "https://animeenigma.org"), ","),
		},
```

- [ ] **Step 3: Domain + repo.** `domain/passkey.go`:

```go
package domain

import "time"

// WebAuthnCredential is one enrolled passkey. CredentialID is the
// base64url-encoded raw credential id (what authenticators return); the
// webauthn library's Credential.ID round-trips through it.
type WebAuthnCredential struct {
	ID           string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string     `gorm:"type:uuid;index" json:"-"`
	CredentialID string     `gorm:"size:512;uniqueIndex" json:"-"`
	PublicKey    []byte     `json:"-"`
	SignCount    uint32     `json:"-"`
	Transports   string     `gorm:"size:128" json:"-"`
	AAGUID       []byte     `json:"-"`
	Name         string     `gorm:"size:64" json:"name"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}
```

`repo/passkey.go` (same shape as `cert.go`): `Create`, `ListByUser` (order `created_at DESC`), `GetByCredentialID(credID string)`, `UpdateSignCount(id string, count uint32, lastUsed time.Time)` (Updates map `sign_count`, `last_used_at`), `Delete(id, userID)` (hard `Delete` with `RowsAffected==0 → NotFound`).

- [ ] **Step 4: Service.** `service/passkey.go`:

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// passkeyStore is satisfied by *repo.PasskeyRepository and by test fakes.
type passkeyStore interface {
	Create(ctx context.Context, c *domain.WebAuthnCredential) error
	ListByUser(ctx context.Context, userID string) ([]domain.WebAuthnCredential, error)
	GetByCredentialID(ctx context.Context, credID string) (*domain.WebAuthnCredential, error)
	UpdateSignCount(ctx context.Context, id string, count uint32, lastUsed time.Time) error
	Delete(ctx context.Context, id, userID string) error
}

type PasskeyService struct {
	wa    *webauthn.WebAuthn
	store passkeyStore
	users userByIDGetter
	cache cache.Cache
	log   *logger.Logger
}

func NewPasskeyService(cfg config.WebAuthnConfig, store passkeyStore, users userByIDGetter, c cache.Cache, log *logger.Logger) (*PasskeyService, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "AnimeEnigma",
		RPID:          cfg.RPID,
		RPOrigins:     cfg.RPOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn init: %w", err)
	}
	return &PasskeyService{wa: wa, store: store, users: users, cache: c, log: log}, nil
}

// waUser adapts our user + credential rows to the library's webauthn.User.
type waUser struct {
	user  *domain.User
	creds []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return []byte(u.user.ID) }
func (u *waUser) WebAuthnName() string                       { return u.user.Username }
func (u *waUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// toLibraryCredential re-inflates a stored row into the library's type.
func toLibraryCredential(row *domain.WebAuthnCredential) (webauthn.Credential, error) {
	rawID, err := base64.RawURLEncoding.DecodeString(row.CredentialID)
	if err != nil {
		return webauthn.Credential{}, fmt.Errorf("credential id decode: %w", err)
	}
	var transports []protocol.AuthenticatorTransport
	for _, t := range strings.Split(row.Transports, ",") {
		if t != "" {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
	}
	return webauthn.Credential{
		ID:        rawID,
		PublicKey: row.PublicKey,
		Transport: transports,
		Authenticator: webauthn.Authenticator{
			AAGUID:    row.AAGUID,
			SignCount: row.SignCount,
		},
	}, nil
}

func (s *PasskeyService) loadWAUser(ctx context.Context, userID string) (*waUser, []domain.WebAuthnCredential, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for i := range rows {
		c, cerr := toLibraryCredential(&rows[i])
		if cerr != nil {
			continue
		}
		creds = append(creds, c)
	}
	return &waUser{user: user, creds: creds}, rows, nil
}

func newCeremonyID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// BeginRegistration starts an enroll ceremony for a logged-in user.
// Discoverable (resident) keys are REQUIRED so login can be usernameless.
func (s *PasskeyService) BeginRegistration(ctx context.Context, user *domain.User) (*protocol.CredentialCreation, string, error) {
	wu, _, err := s.loadWAUser(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	rrk := true
	opts, session, err := s.wa.BeginRegistration(wu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: &rrk,
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		return nil, "", fmt.Errorf("begin registration: %w", err)
	}
	id, err := newCeremonyID()
	if err != nil {
		return nil, "", err
	}
	if err := s.cache.Set(ctx, cache.KeyWebAuthnCeremony(id), session, cache.TTLWebAuthnCeremony); err != nil {
		return nil, "", fmt.Errorf("store ceremony: %w", err)
	}
	return opts, id, nil
}

// FinishRegistration validates the authenticator response and stores the new
// credential row named `name`.
func (s *PasskeyService) FinishRegistration(ctx context.Context, user *domain.User, ceremonyID, name string, r *http.Request) (*domain.WebAuthnCredential, error) {
	var session webauthn.SessionData
	if err := s.cache.Get(ctx, cache.KeyWebAuthnCeremony(ceremonyID), &session); err != nil {
		return nil, errors.InvalidInput("ceremony expired")
	}
	_ = s.cache.Delete(ctx, cache.KeyWebAuthnCeremony(ceremonyID))

	wu, rows, err := s.loadWAUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	cred, err := s.wa.FinishRegistration(wu, session, r)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("passkey_register", "failure").Inc()
		return nil, errors.InvalidInput("passkey verification failed")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("Passkey %d", len(rows)+1)
	}
	if len(name) > 64 {
		name = name[:64]
	}
	var transports []string
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	row := &domain.WebAuthnCredential{
		UserID:       user.ID,
		CredentialID: base64.RawURLEncoding.EncodeToString(cred.ID),
		PublicKey:    cred.PublicKey,
		SignCount:    cred.Authenticator.SignCount,
		Transports:   strings.Join(transports, ","),
		AAGUID:       cred.Authenticator.AAGUID,
		Name:         name,
	}
	if err := s.store.Create(ctx, row); err != nil {
		return nil, err
	}
	metrics.AuthEventsTotal.WithLabelValues("passkey_register", "success").Inc()
	s.log.Infow("passkey enrolled", "user_id", user.ID, "passkey_id", row.ID)
	return row, nil
}

// BeginLogin starts a usernameless (discoverable-credential) assertion.
func (s *PasskeyService) BeginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	opts, session, err := s.wa.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationPreferred),
	)
	if err != nil {
		return nil, "", fmt.Errorf("begin login: %w", err)
	}
	id, err := newCeremonyID()
	if err != nil {
		return nil, "", err
	}
	if err := s.cache.Set(ctx, cache.KeyWebAuthnCeremony(id), session, cache.TTLWebAuthnCeremony); err != nil {
		return nil, "", fmt.Errorf("store ceremony: %w", err)
	}
	return opts, id, nil
}

// FinishLogin validates the assertion and returns the authenticated user.
// The user is resolved from the authenticator's userHandle (our user UUID).
func (s *PasskeyService) FinishLogin(ctx context.Context, ceremonyID string, r *http.Request) (*domain.User, error) {
	var session webauthn.SessionData
	if err := s.cache.Get(ctx, cache.KeyWebAuthnCeremony(ceremonyID), &session); err != nil {
		return nil, errors.Unauthorized("ceremony expired")
	}
	_ = s.cache.Delete(ctx, cache.KeyWebAuthnCeremony(ceremonyID))

	var matched *domain.WebAuthnCredential
	var matchedUser *domain.User
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		row, err := s.store.GetByCredentialID(ctx, base64.RawURLEncoding.EncodeToString(rawID))
		if err != nil {
			return nil, err
		}
		if string(userHandle) != row.UserID {
			return nil, errors.Unauthorized("user handle mismatch")
		}
		user, err := s.users.GetByID(ctx, row.UserID)
		if err != nil {
			return nil, err
		}
		cred, err := toLibraryCredential(row)
		if err != nil {
			return nil, err
		}
		matched = row
		matchedUser = user
		return &waUser{user: user, creds: []webauthn.Credential{cred}}, nil
	}

	cred, err := s.wa.FinishDiscoverableLogin(handler, session, r)
	if err != nil || matched == nil || matchedUser == nil {
		metrics.AuthEventsTotal.WithLabelValues("passkey_login", "failure").Inc()
		return nil, errors.Unauthorized("passkey verification failed")
	}
	_ = s.store.UpdateSignCount(ctx, matched.ID, cred.Authenticator.SignCount, timeNow())
	metrics.AuthEventsTotal.WithLabelValues("passkey_login", "success").Inc()
	return matchedUser, nil
}

func (s *PasskeyService) List(ctx context.Context, userID string) ([]domain.WebAuthnCredential, error) {
	rows, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []domain.WebAuthnCredential{}
	}
	return rows, nil
}

func (s *PasskeyService) Delete(ctx context.Context, id, userID string) error {
	if err := s.store.Delete(ctx, id, userID); err != nil {
		return err
	}
	s.log.Infow("passkey deleted", "user_id", userID, "passkey_id", id)
	return nil
}
```

Add `"time"` import for the `time.Time` in `passkeyStore`. IMPORTANT version check: `errors.New(errors.CodeForbidden, ...)`, `webauthn.WithUserVerification` and `BeginDiscoverableLogin` signatures vary between go-webauthn minor versions — after `go get`, open `$(go env GOMODCACHE)/github.com/go-webauthn/...` or run `go doc github.com/go-webauthn/webauthn/webauthn WebAuthn` and adjust option names to the pinned version (v0.11.x uses `BeginDiscoverableLogin(opts ...LoginOption)` and `FinishDiscoverableLogin(handler DiscoverableUserHandler, session SessionData, response *http.Request)`); do the same for `RequireResidentKey` field availability.

- [ ] **Step 5: Tests.** `passkey_test.go` — full ceremony crypto can't run without an authenticator; test what's testable pure-Go:
  - `TestToLibraryCredential_RoundTrip`: build a `domain.WebAuthnCredential` with known base64url id/publickey/transports "internal,hybrid", convert, assert fields.
  - `TestBeginLogin_StoresCeremony` + `TestBeginRegistration_StoresCeremony`: with fake cache + fake store, assert a ceremony key was set and options non-nil, and that registration options demand resident keys (`opts.Response.AuthenticatorSelection.ResidentKey == protocol.ResidentKeyRequirementRequired`).
  - `TestFinishLogin_ExpiredCeremony`: fake cache returning miss → `Unauthorized`.
  Reuse/extend the in-package fake cache from `magiclink_test.go`.
- [ ] **Step 6: Run.** `go test ./internal/service/ -run Passkey -v` and `go build ./...` → PASS.
- [ ] **Step 7: Commit.** `git commit -m "feat(auth): webauthn passkey service (usernameless, discoverable)" -- services/auth`

---

### Task 6: Handlers, routes, DI

**Files:**
- Create: `services/auth/internal/handler/passkey.go`, `services/auth/internal/handler/cert.go`
- Modify: `services/auth/internal/transport/router.go`, `services/auth/cmd/auth-api/main.go`
- Test: `services/auth/internal/handler/` builds; route smoke via `go test ./...`

**Interfaces:**
- Consumes: everything from Tasks 2-5; `AuthHandler.setRefreshTokenCookie` / `setAccessTokenCookie` (exported wrappers below); `httputil.OK/Error`, `authz.ClaimsFromContext`.
- Produces routes:
  - public: `POST /api/auth/passkey/login/begin`, `POST /api/auth/passkey/login/finish`, `POST /api/auth/cert/consume`
  - protected: `POST /api/auth/passkey/register/begin`, `POST /api/auth/passkey/register/finish`, `GET /api/auth/passkeys`, `DELETE /api/auth/passkeys/{id}`, `POST /api/auth/cert/issue`, `GET /api/auth/certs`, `DELETE /api/auth/certs/{id}`, `PUT /api/auth/profile/cert-auto-login`
  - root (NOT under /api, unreachable via gateway): `GET /cert/handshake-login`, `GET /cert/ca.pem`

- [ ] **Step 1: Cookie access for sibling handlers.** `MagicLinkHandler` already receives `*AuthHandler` (see `main.go:116`) — follow that exact pattern: both new handlers take the `*AuthHandler` and call its unexported cookie helpers (same package).

- [ ] **Step 2: `handler/passkey.go`:**

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

type PasskeyHandler struct {
	passkeys *service.PasskeyService
	auth     *service.AuthService
	cookies  *AuthHandler
	log      *logger.Logger
}

func NewPasskeyHandler(passkeys *service.PasskeyService, auth *service.AuthService, cookies *AuthHandler, log *logger.Logger) *PasskeyHandler {
	return &PasskeyHandler{passkeys: passkeys, auth: auth, cookies: cookies, log: log}
}

// beginResponse wraps ceremony options with the ceremony id the client must
// echo back to finish.
type beginResponse struct {
	CeremonyID string `json:"ceremony_id"`
	Options    any    `json:"options"`
}

func (h *PasskeyHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	opts, id, err := h.passkeys.BeginRegistration(r.Context(), user)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, beginResponse{CeremonyID: id, Options: opts})
}

// RegisterFinish expects ?ceremony=<id>&name=<label> query params and the raw
// WebAuthn attestation JSON as the request body (the library parses r.Body).
func (h *PasskeyHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	row, err := h.passkeys.FinishRegistration(r.Context(), user, r.URL.Query().Get("ceremony"), r.URL.Query().Get("name"), r)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, row)
}

func (h *PasskeyHandler) LoginBegin(w http.ResponseWriter, r *http.Request) {
	opts, id, err := h.passkeys.BeginLogin(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, beginResponse{CeremonyID: id, Options: opts})
}

// LoginFinish completes a usernameless assertion; on success it mints a full
// session exactly like password/Telegram login (cookies + public response).
func (h *PasskeyHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
	user, err := h.passkeys.FinishLogin(r.Context(), r.URL.Query().Get("ceremony"), r)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	authResp, err := h.auth.SessionForUser(r.Context(), user, sessionContextFromReq(r))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	h.cookies.setRefreshTokenCookie(w, authResp.RefreshToken)
	h.cookies.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	httputil.OK(w, authResp.ToPublicResponse())
}

func (h *PasskeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	rows, err := h.passkeys.List(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rows)
}

func (h *PasskeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	if err := h.passkeys.Delete(r.Context(), chi.URLParam(r, "id"), claims.UserID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "deleted"})
}

var _ = json.Marshal // placeholder-import guard: remove if json ends up unused
```

Drop the last line if `encoding/json` is genuinely unused after implementation. Two service seams referenced here must be added to `AuthService` (in `service/auth.go`, next to `createSessionAndAuthResponse`):

```go
// GetUserByID exposes user lookup for sibling handlers (passkey/cert flows).
func (s *AuthService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

// SessionForUser mints a session for an already-authenticated user — the
// terminal step of passkey login (external proof happened in PasskeyService).
func (s *AuthService) SessionForUser(ctx context.Context, user *domain.User, sc SessionContext) (*domain.AuthResponse, error) {
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
```

Check `httputil` for the exact error/OK helper names before use (`grep -n "func OK\|func Error\|func Unauthorized\|func Forbidden" /tmp/ae-alt-login/libs/httputil/*.go`) and match them.

- [ ] **Step 3: `handler/cert.go`:**

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

type CertHandler struct {
	certs   *service.CertService
	auth    *service.AuthService
	users   *service.UserService
	cookies *AuthHandler
	log     *logger.Logger
}

func NewCertHandler(certs *service.CertService, auth *service.AuthService, users *service.UserService, cookies *AuthHandler, log *logger.Logger) *CertHandler {
	return &CertHandler{certs: certs, auth: auth, users: users, cookies: cookies, log: log}
}

// CAPem serves the CA certificate (public). The host setup step curls this
// into /etc/nginx/certs/ae-user-ca.pem.
func (h *CertHandler) CAPem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(h.certs.CAPEM())
}

type issueCertRequest struct {
	Name string `json:"name"`
}

func (h *CertHandler) Issue(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	var req issueCertRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	user, err := h.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	resp, err := h.certs.IssueCertificate(r.Context(), user, req.Name)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, resp)
}

func (h *CertHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	rows, err := h.certs.ListCertificates(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rows)
}

func (h *CertHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	if err := h.certs.RevokeCertificate(r.Context(), chi.URLParam(r, "id"), claims.UserID); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"status": "revoked"})
}

// UpdateAutoLogin toggles User.CertAutoLogin (settings modal toggle).
func (h *CertHandler) UpdateAutoLogin(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	var req domain.UpdateCertAutoLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid body"))
		return
	}
	if err := h.users.UpdateCertAutoLogin(r.Context(), claims.UserID, req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"cert_auto_login": req.Enabled})
}

// HandshakeLogin is the mTLS-vhost endpoint (root mux — NOT proxied by the
// gateway; only cert.animeenigma.org's nginx location can reach it from
// outside the Docker network). Returns a one-time token for
// POST /api/auth/cert/consume on the main origin.
func (h *CertHandler) HandshakeLogin(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.HandshakeCertLogin(
		r.Context(),
		r.Header.Get("X-AE-Cert-Verify"),
		r.Header.Get("X-AE-Cert-PEM"),
		h.certs,
	)
	if err != nil {
		if err == service.ErrCertAutoLoginDisabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"reason": "auto_login_disabled"})
			return
		}
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"token": token})
}

type consumeCertRequest struct {
	Token string `json:"token"`
}

// Consume exchanges a one-time cert-login token for a normal session (cookies
// on the main origin) — the terminal step of cert auto-login.
func (h *CertHandler) Consume(w http.ResponseWriter, r *http.Request) {
	var req consumeCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, errors.InvalidInput("invalid body"))
		return
	}
	authResp, err := h.auth.ConsumeCertLoginToken(r.Context(), req.Token, sessionContextFromReq(r))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	h.cookies.setRefreshTokenCookie(w, authResp.RefreshToken)
	h.cookies.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	httputil.OK(w, authResp.ToPublicResponse())
}
```

Add `UserService.UpdateCertAutoLogin(ctx, userID string, enabled bool) error` in `service/user.go` (GORM `Model(&domain.User{}).Where("id = ?", userID).Update("cert_auto_login", enabled)` via the user repo — add `UserRepository.UpdateCertAutoLogin` mirroring `UpdateApiKeyHash`).

- [ ] **Step 4: Routes.** In `transport/router.go`:
  - Root (after magic-link lines): `r.Get("/cert/handshake-login", certHandler.HandshakeLogin)` and `r.Get("/cert/ca.pem", certHandler.CAPem)`
  - Public `/api/auth` group: `r.Post("/passkey/login/begin", passkeyHandler.LoginBegin)`, `r.Post("/passkey/login/finish", passkeyHandler.LoginFinish)`, `r.Post("/cert/consume", certHandler.Consume)`
  - Protected group: `r.Post("/auth/passkey/register/begin", passkeyHandler.RegisterBegin)`, `r.Post("/auth/passkey/register/finish", passkeyHandler.RegisterFinish)`, `r.Get("/auth/passkeys", passkeyHandler.List)`, `r.Delete("/auth/passkeys/{id}", passkeyHandler.Delete)`, `r.Post("/auth/cert/issue", certHandler.Issue)`, `r.Get("/auth/certs", certHandler.List)`, `r.Delete("/auth/certs/{id}", certHandler.Revoke)`, `r.Put("/auth/profile/cert-auto-login", certHandler.UpdateAutoLogin)`
  - Extend `NewRouter`'s signature with `passkeyHandler *handler.PasskeyHandler, certHandler *handler.CertHandler`.

- [ ] **Step 5: DI in `cmd/auth-api/main.go`:**
  - AutoMigrate: `db.AutoMigrate(&domain.User{}, &domain.UserSession{}, &domain.AuthCA{}, &domain.UserCertificate{}, &domain.WebAuthnCredential{})`
  - After repos: `certRepo := repo.NewCertRepository(db.DB)`, `passkeyRepo := repo.NewPasskeyRepository(db.DB)`
  - Services: `certService := service.NewCertService(certRepo, certRepo, log)` then `if err := certService.EnsureCA(ctx); err != nil { log.Fatalw("ensure ca", "error", err) }`; `passkeyService, err := service.NewPasskeyService(cfg.WebAuthn, passkeyRepo, userRepo, redisCache, log)` (fatal on err).
  - Handlers: `passkeyHandler := handler.NewPasskeyHandler(passkeyService, authService, authHandler, log)`, `certHandler := handler.NewCertHandler(certService, authService, userService, authHandler, log)`; pass both to `transport.NewRouter`.

- [ ] **Step 6: Build + full tests.** `cd /tmp/ae-alt-login/services/auth && go build ./... && go test ./...` → PASS.
- [ ] **Step 7: Commit.** `git commit -m "feat(auth): passkey + cert handlers, routes, DI" -- services/auth`

---

### Task 7: Frontend — auth store + cert auto-login probe + passkey login button

**Files:**
- Create: `frontend/web/src/composables/useCertAutoLogin.ts`
- Modify: `frontend/web/src/stores/auth.ts`, `frontend/web/src/views/Auth.vue`, `frontend/web/src/router/index.ts`, `frontend/web/package.json` (dep)
- Test: `frontend/web/src/composables/__tests__/useCertAutoLogin.spec.ts`

**Interfaces:**
- Consumes: BE routes from Task 6; `import.meta.env.VITE_CERT_LOGIN_BASE`.
- Produces:
  - store: `passkeyLogin(): Promise<boolean>`, `consumeCertToken(token: string): Promise<boolean>`; logout() sets suppression.
  - composable: `tryCertAutoLogin(): Promise<boolean>` + exported constants `CERT_SUPPRESS_KEY = 'ae_cert_suppress'`, `CERT_NEG_CACHE_KEY = 'ae_cert_nolgn_until'`, and `clearCertSuppression()`.

- [ ] **Step 1: Dep.** `cd /tmp/ae-alt-login/frontend/web && bun add @simplewebauthn/browser`

- [ ] **Step 2: Store additions** (`stores/auth.ts`, inside the store setup):

```ts
  // ── Passkey (WebAuthn) login ──
  // Usernameless: empty allowCredentials; the authenticator picks a
  // discoverable credential and the server resolves the user by userHandle.
  const passkeyLogin = async (): Promise<boolean> => {
    error.value = null
    try {
      const { startAuthentication } = await import('@simplewebauthn/browser')
      const begin = await apiClient.post('/auth/passkey/login/begin')
      const beginData = begin.data?.data || begin.data
      const assertion = await startAuthentication({ optionsJSON: beginData.options.publicKey })
      const finish = await apiClient.post(
        `/auth/passkey/login/finish?ceremony=${encodeURIComponent(beginData.ceremony_id)}`,
        assertion,
      )
      const data = finish.data?.data || finish.data
      setToken(data.access_token)
      setUser(data.user)
      clearPreferenceCache()
      clearCertSuppressionFlags()
      return true
    } catch (err: unknown) {
      // NotAllowedError = user dismissed the browser prompt — not an error banner.
      if ((err as { name?: string })?.name === 'NotAllowedError') return false
      const e = err as { response?: { data?: { error?: { message?: string } } } }
      error.value = e.response?.data?.error?.message || i18n.global.t('auth.passkeyError')
      return false
    }
  }

  // ── TLS-cert auto-login: token exchange half (probe lives in the composable) ──
  const consumeCertToken = async (certToken: string): Promise<boolean> => {
    try {
      const response = await apiClient.post('/auth/cert/consume', { token: certToken })
      const data = response.data?.data || response.data
      setToken(data.access_token)
      setUser(data.user)
      clearPreferenceCache()
      return true
    } catch {
      return false
    }
  }

  // Explicit logout suppresses cert auto-login in THIS browser until the next
  // manual login (spec: logout must not bounce straight back in).
  function clearCertSuppressionFlags() {
    try {
      localStorage.removeItem('ae_cert_suppress')
      localStorage.removeItem('ae_cert_nolgn_until')
    } catch { /* privacy modes */ }
  }
```

In `logout()`, after `clearPreferenceCache()` add:

```ts
    try {
      localStorage.setItem('ae_cert_suppress', '1')
    } catch { /* privacy modes */ }
```

In `login()` (password) and in `checkDeepLink()`'s confirmed branch, add `clearCertSuppressionFlags()` after `setUser(...)`. Export `passkeyLogin` and `consumeCertToken` from the store's return object.

- [ ] **Step 3: Composable.** `composables/useCertAutoLogin.ts`:

```ts
import { useAuthStore } from '@/stores/auth'

export const CERT_SUPPRESS_KEY = 'ae_cert_suppress'
export const CERT_NEG_CACHE_KEY = 'ae_cert_nolgn_until'
const NEG_CACHE_MS = 24 * 60 * 60 * 1000
const PROBE_TIMEOUT_MS = 2500

function lsGet(key: string): string | null {
  try { return localStorage.getItem(key) } catch { return null }
}
function lsSet(key: string, val: string) {
  try { localStorage.setItem(key, val) } catch { /* privacy modes */ }
}

/**
 * Silently probes the mTLS vhost and, when the browser presents a valid
 * client certificate for a user with the auto-login toggle ON, exchanges the
 * returned one-time token for a session. Returns true when the user ended up
 * authenticated. Never throws.
 *
 * Skips (returns false immediately) when:
 *  - VITE_CERT_LOGIN_BASE is unset (feature off / dev)
 *  - logout suppression flag is set (cleared by the next manual login)
 *  - a recent probe answered "auto_login_disabled" (24h negative cache —
 *    avoids re-prompting toggled-off cert holders with the browser picker)
 */
export async function tryCertAutoLogin(): Promise<boolean> {
  const base = import.meta.env.VITE_CERT_LOGIN_BASE as string | undefined
  if (!base) return false
  if (lsGet(CERT_SUPPRESS_KEY) === '1') return false
  const negUntil = Number(lsGet(CERT_NEG_CACHE_KEY) || 0)
  if (negUntil && Date.now() < negUntil) return false

  try {
    const res = await fetch(`${base.replace(/\/$/, '')}/cert-login`, {
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    })
    if (res.status === 403) {
      const body = await res.json().catch(() => null)
      if (body?.reason === 'auto_login_disabled') {
        lsSet(CERT_NEG_CACHE_KEY, String(Date.now() + NEG_CACHE_MS))
      }
      return false
    }
    if (!res.ok) return false
    const payload = await res.json().catch(() => null)
    const token = payload?.data?.token ?? payload?.token
    if (!token) return false
    const ok = await useAuthStore().consumeCertToken(token)
    if (ok) notifyCertLogin()
    return ok
  } catch {
    // Timeout, TLS failure, user dismissed the cert picker, network error —
    // all mean "no auto-login this time"; fall through to the login page.
    return false
  }
}

/**
 * Small success toast after a silent cert login (owner request 2026-07-24):
 * the login page never shows, so this is the only feedback the user gets.
 * Find the project's EXISTING toast/notification mechanism first
 * (`grep -rln "toast\|Toast\|notification" frontend/web/src/composables frontend/web/src/components/ui | head`)
 * and use it with the i18n key `auth.certAutoLoginSuccess`. Only if the
 * project has NO toast primitive, fall back to a `sessionStorage` handoff
 * flag ('ae_cert_login_toast'='1') that App.vue reads once on mount and
 * renders as a small self-dismissing (4s) glass-card banner, bottom-right,
 * semantic tokens only.
 */
function notifyCertLogin() { /* implement per the comment above */ }
```

The `notifyCertLogin` body above is the ONE deliberately unresolved piece: it depends on which toast mechanism exists — resolve it at implementation time per the comment, and delete the comment scaffolding.

- [ ] **Step 4: Router guard.** In `router/index.ts`, in the auth guard (`if (to.meta.requiresAuth && !authStore.isAuthenticated ...)` branch at ~line 432), BEFORE `next({ name: 'auth' })`:

```ts
    // TLS-cert auto-login (spec 2026-07-24): probe the mTLS vhost before
    // showing the login page. On success the login page never renders.
    const { tryCertAutoLogin } = await import('@/composables/useCertAutoLogin')
    if (await tryCertAutoLogin()) {
      next()
      return
    }
```

(The guard callback is already `async`.)

- [ ] **Step 5: Auth.vue passkey button.** After the Telegram QR `</div>` block closing the "Active state" section (after the `</details>`, still inside the flex column), add:

```html
          <!-- Passkey login (spec 2026-07-24): small secondary action -->
          <button
            v-if="passkeySupported"
            type="button"
            class="inline-flex items-center gap-2 text-white/60 hover:text-white text-sm transition-colors"
            @click="loginWithPasskey"
          >
            <KeyRound class="w-4 h-4" aria-hidden="true" />
            {{ $t('auth.passkeyLogin') }}
          </button>
```

Script additions:

```ts
import { KeyRound } from 'lucide-vue-next'

const passkeySupported = typeof window !== 'undefined' && !!window.PublicKeyCredential

async function loginWithPasskey() {
  const ok = await authStore.passkeyLogin()
  if (ok) {
    cleanup()
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.push(returnUrl || '/')
  }
}
```

(lucide icons are NAMED imports — house rule.) Also in Auth.vue `onMounted`, before `startAuth()`, run the same probe so direct navigation to `/auth` benefits too:

```ts
  const { tryCertAutoLogin } = await import('@/composables/useCertAutoLogin')
  if (await tryCertAutoLogin()) {
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.replace(returnUrl || '/')
    return
  }
```

(make `onMounted`'s callback `async`).

- [ ] **Step 6: Vitest for the composable.** `__tests__/useCertAutoLogin.spec.ts` — mock `fetch` + pinia store; cases: (a) no VITE base → false, no fetch; (b) suppression flag → false, no fetch; (c) 403 auto_login_disabled → false + negative-cache key set; (d) 200 `{token}` → consumeCertToken called, returns its result; (e) fetch throws → false; (f) on consume success the cert-login success toast/notification path fires (mock it), on failure it does not. Stub `import.meta.env` via `vi.stubEnv('VITE_CERT_LOGIN_BASE', 'https://cert.example')`. Follow the existing vitest setup (beware the vue-i18n `createI18n` barrel mock trap — don't import the store's i18n side-effects; `vi.mock('@/api/client')`).
- [ ] **Step 7: Run.** `cd /tmp/ae-alt-login/frontend/web && bunx vitest run src/composables/__tests__/useCertAutoLogin.spec.ts && bunx vue-tsc --noEmit` (remember vue-tsc false-pass caveat: also run `bunx tsc --noEmit` if the project does elsewhere) → PASS.
- [ ] **Step 8: Commit.** `git commit -m "feat(web): passkey login button + TLS-cert auto-login probe" -- frontend/web`

---

### Task 8: Frontend — «Продвинутый логин» modal + settings card + i18n

**Files:**
- Create: `frontend/web/src/components/profile/AdvancedLoginModal.vue`
- Modify: `frontend/web/src/views/Profile.vue` (Settings tab, between the API Key card and `<ActiveSessionsCard/>`), `frontend/web/src/i18n/locales/en.json`, `ru.json`, `ja.json` (check actual locale file paths with `ls frontend/web/src/i18n*` / `grep -rn "auth.telegramLogin" frontend/web/src` first and follow the existing key layout)

**Interfaces:**
- Consumes: `/auth/passkeys` CRUD, `/auth/passkey/register/*`, `/auth/cert/*`, `/auth/certs`, `/auth/profile/cert-auto-login`, `authStore.user.cert_auto_login` (extend the FE `User` interface with `cert_auto_login?: boolean`).
- Produces: `<AdvancedLoginModal v-model:open="advancedLoginOpen" />`.

- [ ] **Step 1: Extend FE `User`.** In `stores/auth.ts` `User` interface add `cert_auto_login?: boolean`.

- [ ] **Step 2: Modal component.** Structure (use the project's existing dialog primitives — find them with `grep -rln "DialogContent\|<Dialog" frontend/web/src/components/profile | head`; mirror whichever modal ActiveSessionsCard-adjacent code uses; bind ONLY semantic tokens):
  - **Section «Passkeys»**: list from `GET /auth/passkeys` (name, `created_at`, `last_used_at` formatted like ActiveSessionsCard does), delete button per row (confirm via the project's existing confirm-dialog pattern; remember the reka Select/confirm auto-dismiss trap if a dropdown is involved — plain buttons here). «Добавить passkey»: name input + button → `POST /auth/passkey/register/begin` → `startRegistration({ optionsJSON: beginData.options.publicKey })` from `@simplewebauthn/browser` → `POST /auth/passkey/register/finish?ceremony=...&name=...` with the attestation JSON body → refresh list. Hide the whole section behind `window.PublicKeyCredential` support with an i18n fallback note.
  - **Section «TLS-сертификат»**: collapsible `<details>` instructions block (5 short platform paragraphs: Windows / macOS / iOS / Android / Linux — i18n keys below). «Выпустить сертификат»: name input → `POST /auth/cert/issue` → trigger download of the `.p12` (decode `p12_base64` → `Blob` → `URL.createObjectURL` → `<a download="animeenigma-<name>.p12">`) → show `password` once in a highlighted box with copy button + "won't be shown again" warning; clear it when the modal closes. Cert list from `GET /auth/certs`: name, created, expiry (`not_after`), last used, revoked badge, revoke button (confirm). Toggle «Входить автоматически при обнаружении сертификата»: bound to `authStore.user.cert_auto_login`, `PUT /auth/profile/cert-auto-login {enabled}`; disabled (with hint) when the list has no active (non-revoked) cert. On issue success, also `localStorage.removeItem('ae_cert_suppress')` + `localStorage.removeItem('ae_cert_nolgn_until')` (this browser now clearly wants cert login).
- [ ] **Step 3: Settings card.** In `Profile.vue` Settings tab, after the API Key card block insert a card matching the neighbors' classes:

```html
          <!-- Advanced login (passkeys + TLS certs) -->
          <div class="glass-card p-6">
            <div class="flex items-center justify-between gap-4">
              <div>
                <h3 class="text-white font-medium">{{ $t('profile.advancedLogin.title') }}</h3>
                <p class="text-white/50 text-sm mt-1">{{ $t('profile.advancedLogin.subtitle') }}</p>
              </div>
              <Button variant="secondary" size="sm" @click="advancedLoginOpen = true">
                {{ $t('profile.advancedLogin.open') }}
              </Button>
            </div>
          </div>
          <AdvancedLoginModal v-model:open="advancedLoginOpen" />
```

(match the actual card/Button classes used by the surrounding settings cards — read them first and copy their idiom.)

- [ ] **Step 4: i18n keys** (same tree in en/ru/ja; ru shown, translate en/ja to match):

```
auth.passkeyLogin        = "Войти через passkey"
auth.passkeyError        = "Не удалось войти через passkey"
auth.certAutoLoginSuccess = "Вы вошли по TLS-сертификату"
profile.advancedLogin.title            = "Продвинутый логин"
profile.advancedLogin.subtitle         = "Passkey и вход по TLS-сертификату"
profile.advancedLogin.open             = "Настроить"
profile.advancedLogin.passkeys.title   = "Passkeys"
profile.advancedLogin.passkeys.add     = "Добавить passkey"
profile.advancedLogin.passkeys.name    = "Название"
profile.advancedLogin.passkeys.empty   = "Пока нет ни одного passkey"
profile.advancedLogin.passkeys.unsupported = "Этот браузер не поддерживает passkey"
profile.advancedLogin.passkeys.delete  = "Удалить"
profile.advancedLogin.passkeys.lastUsed = "Последний вход: {date}"
profile.advancedLogin.cert.title       = "TLS-сертификат"
profile.advancedLogin.cert.issue       = "Выпустить сертификат"
profile.advancedLogin.cert.name        = "Название (например, «Ноутбук»)"
profile.advancedLogin.cert.password    = "Пароль для установки (показывается один раз)"
profile.advancedLogin.cert.passwordCopy = "Скопировать"
profile.advancedLogin.cert.passwordWarn = "Сохраните пароль — после закрытия окна он больше не появится"
profile.advancedLogin.cert.empty       = "Сертификаты ещё не выпускались"
profile.advancedLogin.cert.revoke      = "Отозвать"
profile.advancedLogin.cert.revoked     = "Отозван"
profile.advancedLogin.cert.expires     = "Действует до {date}"
profile.advancedLogin.cert.autoLogin   = "Входить автоматически при обнаружении сертификата"
profile.advancedLogin.cert.autoLoginHint = "Доступно после выпуска хотя бы одного сертификата"
profile.advancedLogin.cert.instructions = "Как установить сертификат"
profile.advancedLogin.cert.instructionsWindows = "Windows: откройте скачанный .p12, выберите «Текущий пользователь», введите пароль"
profile.advancedLogin.cert.instructionsMacos   = "macOS: откройте .p12, введите пароль — сертификат попадёт в Связку ключей"
profile.advancedLogin.cert.instructionsIos     = "iOS: скачайте .p12, затем Настройки → Основные → VPN и управление устройством → установить профиль"
profile.advancedLogin.cert.instructionsAndroid = "Android: Настройки → Безопасность → Шифрование и учётные данные → Установить сертификат (VPN и приложения)"
profile.advancedLogin.cert.instructionsLinux   = "Linux: импортируйте .p12 в браузер (Настройки → Конфиденциальность → Сертификаты)"
```

- [ ] **Step 5: `/frontend-verify`.** Run the `/frontend-verify` skill (DS-lint, i18n parity en/ru/ja, real `bun run build`, lucide/TS2614 traps). Fix everything it flags.
- [ ] **Step 6: Commit.** `git commit -m "feat(web): advanced-login modal (passkeys + TLS certs) in settings" -- frontend/web`

---

### Task 9: Infra reference conf, helper script, docs

**Files:**
- Create: `infra/nginx/cert.animeenigma.org.conf`, `bin/ae-cert-ca-install.sh`
- Modify: `docs/environment-variables.md` (auth section: `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_ORIGINS`; web build: `VITE_CERT_LOGIN_BASE`), `docker/docker-compose.yml` (web build arg + auth env passthrough — mirror how an existing `VITE_*` build arg flows; add `WEBAUTHN_RP_ID`/`WEBAUTHN_RP_ORIGINS` to the auth service env block with prod values)

- [ ] **Step 1: nginx reference vhost** (`infra/nginx/cert.animeenigma.org.conf`, style-matched to `ext.animeenigma.org.conf`):

```nginx
# cert.animeenigma.org — mTLS client-certificate auto-login vhost.
#
# DNS MUST be grey-cloud (DNS only): the whole point is that the browser's
# TLS handshake — including the client certificate — terminates HERE.
#
#   Browser (with AE-issued client cert)
#     → nginx (this vhost: ssl_verify_client optional against the platform CA)
#       → auth :8080 loopback  /cert/handshake-login   (NOT via gateway)
#
# TCP-only on purpose: NO quic/h3 listeners — nginx client-cert handling over
# HTTP/3 is unreliable, and the probe is a single tiny fetch anyway.
#
# This file mirrors the LIVE host deployment. It is a reference: nginx host
# configs live on the box, not in git. Setup runbook: docs/superpowers/specs/
# 2026-07-24-alt-login-passkey-tls-design.md (Deploy / ops checklist).

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name cert.animeenigma.org;

    ssl_certificate     /etc/letsencrypt/live/cert.animeenigma.org/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cert.animeenigma.org/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    # Platform user-CA (exported from auth via bin/ae-cert-ca-install.sh).
    # `optional`, not `on`: visitors without an AE cert get a normal handshake
    # and a 401 from auth — never a TLS-level failure.
    ssl_client_certificate /etc/nginx/certs/ae-user-ca.pem;
    ssl_verify_client      optional;
    ssl_verify_depth       1;

    # The ONLY route on this vhost. Proxies straight to the auth service's
    # loopback-published port (bypasses the gateway BY DESIGN: the route is
    # thereby unreachable from the main domain, so the trust headers below
    # cannot be spoofed by public traffic).
    location = /cert-login {
        limit_req zone=auth burst=10 nodelay;

        proxy_pass http://127.0.0.1:8080/cert/handshake-login;

        proxy_set_header X-AE-Cert-Verify $ssl_client_verify;
        proxy_set_header X-AE-Cert-PEM    $ssl_client_escaped_cert;
        proxy_set_header X-Real-IP        $remote_addr;
        proxy_set_header Host             $host;
    }

    location / { return 404; }
}

# HTTP :80 — ACME webroot for certbot + redirect.
server {
    listen 80;
    listen [::]:80;
    server_name cert.animeenigma.org;

    location /.well-known/acme-challenge/ {
        root /var/www/cert-acme;
        default_type "text/plain";
    }
    location / { return 301 https://$host$request_uri; }
}
```

Note for the live install: the `limit_req zone=auth` zone already exists in the host's http block (memory: DDoS hardening 2026-06-10); if the zone name differs on the box, reuse whatever `/api/auth/` uses there.

- [ ] **Step 2: CA install helper.** `bin/ae-cert-ca-install.sh` (chmod +x):

```bash
#!/usr/bin/env bash
# Exports the platform user-CA from the auth service and installs it where the
# cert.animeenigma.org vhost expects it, then reloads nginx.
# Run on the HOST after `make redeploy-auth` first boots the CA.
set -euo pipefail

DEST=/etc/nginx/certs/ae-user-ca.pem
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

curl -fsS http://127.0.0.1:8080/cert/ca.pem -o "$TMP"
grep -q "BEGIN CERTIFICATE" "$TMP" || { echo "unexpected CA payload"; exit 1; }

install -m 0644 "$TMP" "$DEST"
nginx -t && systemctl reload nginx
echo "installed $DEST"
```

- [ ] **Step 3: env docs + compose.** Document the three new vars in `docs/environment-variables.md` (defaults as in Global Constraints; `VITE_CERT_LOGIN_BASE=https://cert.animeenigma.org` for prod web builds, empty = probe disabled). Wire them in `docker/docker-compose.yml` following the existing patterns exactly (grep `VITE_` for the web build-arg chain incl. `frontend/web/Dockerfile` ARG/ENV lines; grep `TELEGRAM_BOT_TOKEN` for the auth env block).
- [ ] **Step 4: Commit.** `git commit -m "feat(infra): cert.animeenigma.org mTLS vhost reference + CA install helper + env docs" -- infra bin docs docker`

---

### Task 10: Verify, ship, deploy

- [ ] **Step 1: Full verification in the worktree.**

```bash
cd /tmp/ae-alt-login/services/auth && go build ./... && go test ./...
cd /tmp/ae-alt-login/libs/cache && go test ./...
cd /tmp/ae-alt-login/frontend/web && bunx vitest run && bunx vue-tsc --noEmit && bun run build
```

All green before proceeding.

- [ ] **Step 2: Push.** Pull-rebase-push loop to `origin main` (git-workflow §③).
- [ ] **Step 3: Host bring-up (Bash, careful sequencing).**
  1. `dig +short A cert.animeenigma.org` — MUST return `152.53.160.135`. If it still returns CF IPs (104.21.x/172.67.x), STOP and ask the owner to flip the record to DNS-only; everything below depends on it.
  2. `make redeploy-auth` (CA bootstraps; verify `curl -s http://127.0.0.1:8080/cert/ca.pem | head -1` → `-----BEGIN CERTIFICATE-----`).
  3. `mkdir -p /var/www/cert-acme && certbot certonly --webroot -w /var/www/cert-acme -d cert.animeenigma.org` (needs the :80 server block installed first — install the vhost with the ssl server block commented if certbot has no cert yet, then uncomment).
  4. `bash bin/ae-cert-ca-install.sh` (from the repo checkout on the host).
  5. Install the vhost into `/etc/nginx/sites-available/` + symlink to `sites-enabled/`, `nginx -t && systemctl reload nginx`.
  6. Rebuild web with the new build arg: ensure `VITE_CERT_LOGIN_BASE=https://cert.animeenigma.org` is in `docker/.env`, then `make redeploy-web`.
  7. End-to-end check: create a cert for the `ui_audit_bot` user via API (login → `POST /api/auth/cert/issue`), extract key+cert from the .p12 (`openssl pkcs12 -in test.p12 -out /tmp/claude-0/-data-animeenigma/*/scratchpad/test.pem -nodes -passin pass:<pwd>`), enable the toggle (`PUT /api/auth/profile/cert-auto-login {"enabled":true}`), then `curl -s https://cert.animeenigma.org/cert-login --cert /tmp/.../test.pem` → expect `{"token":"cl_..."}`; consume it: `curl -s -X POST https://animeenigma.org/api/auth/cert/consume -H 'Content-Type: application/json' -d '{"token":"cl_..."}'` → expect user JSON + cookies. Then revoke the test cert.
  8. Passkey smoke needs a real authenticator — verify via Playwright virtual authenticator if quick, else leave for the owner's manual check and SAY SO in the summary.
- [ ] **Step 4: `/animeenigma-after-update`** (simplify → lint/build → redeploy → health → Russian Trump-mode changelog entry → commit+push). Mention both features in one changelog group.
- [ ] **Step 5: Feedback status.** `bin/feedback-status 2026-07-23T15-56-53_tNeymik_telegram ai_done` and worktree cleanup ONLY after after-update is green (`git worktree remove /tmp/ae-alt-login && git -C /data/animeenigma worktree prune`).
