package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// ---------------------------------------------------------------------------
// Test helpers (in-package fakes — no testify/mock)
// ---------------------------------------------------------------------------

// fakeCacheStore is a thread-safe in-memory cache.Cache implementation.
type fakeCacheStore struct {
	mu      sync.Mutex
	entries map[string]fakeCacheEntry
}

type fakeCacheEntry struct {
	raw     []byte
	expires time.Time
}

func newFakeCache() *fakeCacheStore {
	return &fakeCacheStore{entries: make(map[string]fakeCacheEntry)}
}

func (f *fakeCacheStore) Set(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key] = fakeCacheEntry{raw: b, expires: time.Now().Add(ttl)}
	return nil
}

func (f *fakeCacheStore) Get(_ context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[key]
	if !ok || time.Now().After(e.expires) {
		return fmt.Errorf("cache: key not found")
	}
	return json.Unmarshal(e.raw, dest)
}

func (f *fakeCacheStore) Delete(_ context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.entries, k)
	}
	return nil
}

func (f *fakeCacheStore) Exists(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[key]
	return ok && !time.Now().After(e.expires), nil
}

func (f *fakeCacheStore) GetOrSet(_ context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	f.mu.Lock()
	e, ok := f.entries[key]
	f.mu.Unlock()
	if ok && !time.Now().After(e.expires) {
		return json.Unmarshal(e.raw, dest)
	}
	val, err := fn()
	if err != nil {
		return err
	}
	return f.Set(context.Background(), key, val, ttl)
}

func (f *fakeCacheStore) Invalidate(_ context.Context, _ string) error { return nil }
func (f *fakeCacheStore) SetNX(_ context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.entries[key]; ok && !time.Now().After(e.expires) {
		return false, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	f.entries[key] = fakeCacheEntry{raw: b, expires: time.Now().Add(ttl)}
	return true, nil
}

// fakeSessionStore implements sessionHashFinder in memory.
type fakeSessionStore struct {
	mu       sync.Mutex
	sessions []*domain.UserSession
}

func (f *fakeSessionStore) FindAliveByHash(_ context.Context, hash string) (*domain.UserSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.RefreshTokenHash == hash && s.RevokedAt == nil {
			return s, nil
		}
	}
	return nil, fmt.Errorf("session not found")
}

func (f *fakeSessionStore) Create(_ context.Context, s *domain.UserSession) error {
	if s.ID == "" {
		s.ID = "sess-" + fmt.Sprint(time.Now().UnixNano())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append(f.sessions, s)
	return nil
}

// fakeUserStore implements userByIDGetter in memory.
type fakeUserStore struct {
	users map[string]*domain.User
}

func (f *fakeUserStore) GetByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user not found: %s", id)
}

// newTestAuthService builds an AuthService wired to in-memory fakes.
func newTestAuthService(t *testing.T) *AuthService {
	t.Helper()
	jwtCfg := authz.JWTConfig{
		Secret:         "test-secret-for-magic-link",
		Issuer:         "test",
		AccessTokenTTL: 15 * time.Minute,
	}
	log := logger.Default()
	svc := &AuthService{
		cache:      newFakeCache(),
		jwtManager: authz.NewJWTManager(jwtCfg),
		log:        log,
		magicSessionFinder: &fakeSessionStore{},
		magicUserGetter:    &fakeUserStore{users: map[string]*domain.User{}},
	}
	return svc
}

// newTestAuthServiceWithSession builds a service that already has one alive
// session and returns the raw refresh token for that session.
func newTestAuthServiceWithSession(t *testing.T) (*AuthService, string) {
	t.Helper()
	jwtCfg := authz.JWTConfig{
		Secret:         "test-secret-for-magic-link",
		Issuer:         "test",
		AccessTokenTTL: 15 * time.Minute,
	}
	log := logger.Default()

	userID := "user-test-123"
	user := &domain.User{
		ID:       userID,
		Username: "testuser",
		Role:     authz.RoleUser,
	}

	// Mint a raw refresh token and store its hash in the session store.
	rt, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}

	sessionStore := &fakeSessionStore{
		sessions: []*domain.UserSession{
			{
				ID:               "sess-fixed-1",
				UserID:           userID,
				RefreshTokenHash: hashRefreshToken(rt),
			},
		},
	}
	userStore := &fakeUserStore{
		users: map[string]*domain.User{userID: user},
	}

	svc := &AuthService{
		cache:              newFakeCache(),
		jwtManager:         authz.NewJWTManager(jwtCfg),
		log:                log,
		magicSessionFinder: sessionStore,
		magicUserGetter:    userStore,
	}
	return svc, rt
}

// ---------------------------------------------------------------------------
// SanitizeOldURL
// ---------------------------------------------------------------------------

func TestSanitizeOldURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/anime/abc", "/anime/abc"},
		{"/anime/abc?x=1&y=2", "/anime/abc?x=1&y=2"},
		{"", "/"},
		{"//evil.com", "/"},
		{"/\\evil.com", "/"},
		{"https://evil.com", "/"},
		{"http://evil.com/x", "/"},
		{"javascript:alert(1)", "/"},
		{"/path with space", "/path with space"},
		{"relative/no/leading/slash", "/"},
		{"/\t/control", "/"},
	}
	for _, c := range cases {
		if got := SanitizeOldURL(c.in); got != c.want {
			t.Errorf("SanitizeOldURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// MintMagicToken
// ---------------------------------------------------------------------------

func TestMintMagicToken_AnonymousReturnsEmpty(t *testing.T) {
	s := newTestAuthService(t)
	tok, err := s.MintMagicToken(context.Background(), "")
	if err != nil || tok != "" {
		t.Fatalf("want empty token,nil err; got %q,%v", tok, err)
	}
}

func TestMintMagicToken_ValidSessionMintsToken(t *testing.T) {
	s, rt := newTestAuthServiceWithSession(t)
	tok, err := s.MintMagicToken(context.Background(), rt)
	if err != nil || !strings.HasPrefix(tok, "ml_") {
		t.Fatalf("want ml_ token; got %q,%v", tok, err)
	}
}

// ---------------------------------------------------------------------------
// ConsumeMagicToken
// ---------------------------------------------------------------------------

func TestConsumeMagicToken_SingleUse(t *testing.T) {
	s, rt := newTestAuthServiceWithSession(t)
	tok, _ := s.MintMagicToken(context.Background(), rt)
	resp, err := s.ConsumeMagicToken(context.Background(), tok, SessionContext{})
	if err != nil || resp == nil || resp.AccessToken == "" {
		t.Fatalf("first consume should succeed; got %v,%v", resp, err)
	}
	if _, err := s.ConsumeMagicToken(context.Background(), tok, SessionContext{}); err == nil {
		t.Fatalf("second consume must fail (single-use)")
	}
}

func TestConsumeMagicToken_Unknown(t *testing.T) {
	s := newTestAuthService(t)
	if _, err := s.ConsumeMagicToken(context.Background(), "ml_deadbeef", SessionContext{}); err == nil {
		t.Fatalf("unknown token must error")
	}
}
