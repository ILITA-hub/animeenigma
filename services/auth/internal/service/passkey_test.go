package service

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// fakePasskeyStore implements passkeyStore in memory for the tests below.
type fakePasskeyStore struct {
	byUser map[string][]domain.WebAuthnCredential
	byCred map[string]*domain.WebAuthnCredential
}

func newFakePasskeyStore() *fakePasskeyStore {
	return &fakePasskeyStore{
		byUser: make(map[string][]domain.WebAuthnCredential),
		byCred: make(map[string]*domain.WebAuthnCredential),
	}
}

func (f *fakePasskeyStore) Create(_ context.Context, c *domain.WebAuthnCredential) error {
	if c.ID == "" {
		c.ID = "cred-" + c.CredentialID
	}
	f.byUser[c.UserID] = append(f.byUser[c.UserID], *c)
	f.byCred[c.CredentialID] = c
	return nil
}

func (f *fakePasskeyStore) ListByUser(_ context.Context, userID string) ([]domain.WebAuthnCredential, error) {
	return f.byUser[userID], nil
}

func (f *fakePasskeyStore) GetByCredentialID(_ context.Context, credID string) (*domain.WebAuthnCredential, error) {
	c, ok := f.byCred[credID]
	if !ok {
		return nil, liberrors.NotFound("passkey")
	}
	return c, nil
}

func (f *fakePasskeyStore) UpdateSignCount(_ context.Context, id string, count uint32, lastUsed time.Time) error {
	for cid, c := range f.byCred {
		if c.ID == id {
			c.SignCount = count
			c.LastUsedAt = &lastUsed
			f.byCred[cid] = c
		}
	}
	return nil
}

func (f *fakePasskeyStore) Delete(_ context.Context, id, userID string) error {
	rows := f.byUser[userID]
	for i, r := range rows {
		if r.ID == id {
			f.byUser[userID] = append(rows[:i], rows[i+1:]...)
			delete(f.byCred, r.CredentialID)
			return nil
		}
	}
	return liberrors.NotFound("passkey")
}

// newTestPasskeyService builds a PasskeyService wired to in-memory fakes.
func newTestPasskeyService(t *testing.T) (*PasskeyService, *fakeCacheStore, *fakeUserStore) {
	t.Helper()
	c := newFakeCache()
	users := &fakeUserStore{users: map[string]*domain.User{}}
	svc, err := NewPasskeyService(
		config.WebAuthnConfig{RPID: "localhost", RPOrigins: []string{"https://localhost"}},
		newFakePasskeyStore(),
		users,
		c,
		logger.Default(),
	)
	if err != nil {
		t.Fatalf("NewPasskeyService: %v", err)
	}
	return svc, c, users
}

// ---------------------------------------------------------------------------
// toLibraryCredential
// ---------------------------------------------------------------------------

func TestToLibraryCredential_RoundTrip(t *testing.T) {
	rawID := []byte{1, 2, 3, 4, 5}
	row := &domain.WebAuthnCredential{
		CredentialID: base64.RawURLEncoding.EncodeToString(rawID),
		PublicKey:    []byte("public-key-bytes"),
		SignCount:    7,
		Transports:   "internal,hybrid",
		AAGUID:       []byte{9, 9, 9},
	}
	cred, err := toLibraryCredential(row)
	if err != nil {
		t.Fatalf("toLibraryCredential: %v", err)
	}
	if string(cred.ID) != string(rawID) {
		t.Fatalf("ID = %v, want %v", cred.ID, rawID)
	}
	if string(cred.PublicKey) != "public-key-bytes" {
		t.Fatalf("PublicKey = %q", cred.PublicKey)
	}
	if cred.Authenticator.SignCount != 7 {
		t.Fatalf("SignCount = %d", cred.Authenticator.SignCount)
	}
	if len(cred.Transport) != 2 || string(cred.Transport[0]) != "internal" || string(cred.Transport[1]) != "hybrid" {
		t.Fatalf("Transport = %v", cred.Transport)
	}
	if string(cred.Authenticator.AAGUID) != string([]byte{9, 9, 9}) {
		t.Fatalf("AAGUID = %v", cred.Authenticator.AAGUID)
	}
}

// ---------------------------------------------------------------------------
// BeginRegistration / BeginLogin
// ---------------------------------------------------------------------------

func TestBeginRegistration_StoresCeremony(t *testing.T) {
	svc, c, users := newTestPasskeyService(t)
	user := &domain.User{ID: "u-1", Username: "alice"}
	users.users[user.ID] = user

	opts, ceremonyID, err := svc.BeginRegistration(context.Background(), user)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	if opts == nil {
		t.Fatal("opts must not be nil")
	}
	if ceremonyID == "" {
		t.Fatal("ceremonyID must not be empty")
	}
	if ok, _ := c.Exists(context.Background(), cache.KeyWebAuthnCeremony(ceremonyID)); !ok {
		t.Fatal("ceremony was not stored in cache")
	}
	if opts.Response.AuthenticatorSelection.ResidentKey != protocol.ResidentKeyRequirementRequired {
		t.Fatalf("ResidentKey = %q, want required (discoverable credentials are mandatory for usernameless login)",
			opts.Response.AuthenticatorSelection.ResidentKey)
	}
}

func TestBeginLogin_StoresCeremony(t *testing.T) {
	svc, c, _ := newTestPasskeyService(t)

	opts, ceremonyID, err := svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if opts == nil {
		t.Fatal("opts must not be nil")
	}
	if ceremonyID == "" {
		t.Fatal("ceremonyID must not be empty")
	}
	if ok, _ := c.Exists(context.Background(), cache.KeyWebAuthnCeremony(ceremonyID)); !ok {
		t.Fatal("ceremony was not stored in cache")
	}
}

// ---------------------------------------------------------------------------
// FinishLogin
// ---------------------------------------------------------------------------

func TestFinishLogin_ExpiredCeremony(t *testing.T) {
	svc, _, _ := newTestPasskeyService(t)
	_, err := svc.FinishLogin(context.Background(), "does-not-exist", nil)
	if err == nil {
		t.Fatal("want error for unknown/expired ceremony")
	}
	appErr, ok := liberrors.IsAppError(err)
	if !ok || appErr.Code != liberrors.CodeUnauthorized {
		t.Fatalf("want Unauthorized AppError; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List / Delete
// ---------------------------------------------------------------------------

func TestPasskeyList_EmptyIsNeverNil(t *testing.T) {
	svc, _, _ := newTestPasskeyService(t)
	rows, err := svc.List(context.Background(), "u-nobody")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if rows == nil {
		t.Fatal("List must return an empty slice, not nil")
	}
}

func TestPasskeyDelete_NotFoundForOtherUser(t *testing.T) {
	svc, _, _ := newTestPasskeyService(t)
	store := newFakePasskeyStore()
	svc.store = store
	row := &domain.WebAuthnCredential{ID: "pk-1", UserID: "owner", CredentialID: "cred-1"}
	_ = store.Create(context.Background(), row)

	if err := svc.Delete(context.Background(), "pk-1", "someone-else"); err == nil {
		t.Fatal("want NotFound deleting another user's passkey")
	}
	if err := svc.Delete(context.Background(), "pk-1", "owner"); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
}
