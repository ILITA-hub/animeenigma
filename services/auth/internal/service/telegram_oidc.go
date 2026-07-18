package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

const (
	// oidcStateKeyPrefix prefixes the Redis key holding one login attempt's
	// PKCE verifier + post-login return path. The TTL bounds the window
	// between the redirect to Telegram and the callback.
	oidcStateKeyPrefix = "auth:tgoidc:"
	oidcStateTTL       = 5 * time.Minute
)

// ErrOIDCStateExpired distinguishes an expired/replayed/unknown state (user
// retries) from infrastructure failures (logged as errors).
var ErrOIDCStateExpired = stderrors.New("oidc state expired or already used")

// oidcState is the Redis-stored payload for one in-flight login attempt.
type oidcState struct {
	Verifier   string `json:"verifier"`
	ReturnPath string `json:"return_path"`
}

// TelegramOIDC drives the OAuth2 authorization-code + PKCE login against
// Telegram's OIDC provider (oauth.telegram.org). It is pure protocol — no DB
// access; the handler composes Complete's claims with
// AuthService.LoginWithTelegram, which owns find-or-create + sessions.
type TelegramOIDC struct {
	cfg   config.TelegramOIDCConfig
	cache cache.Cache
	log   *logger.Logger

	// Discovery is lazy so an unreachable oauth.telegram.org delays logins,
	// not service boot. mu guards first initialization; after that the
	// fields are read-only.
	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NewTelegramOIDC(cfg config.TelegramOIDCConfig, c cache.Cache, log *logger.Logger) *TelegramOIDC {
	return &TelegramOIDC{cfg: cfg, cache: c, log: log}
}

func (t *TelegramOIDC) ensureProvider(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.provider != nil {
		return nil
	}
	p, err := oidc.NewProvider(ctx, t.cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	t.provider = p
	t.oauth = &oauth2.Config{
		ClientID:     t.cfg.ClientID,
		ClientSecret: t.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  t.cfg.RedirectURL,
		// telegram:bot_access lets the auth bot message the user later
		// (notifications). phone is deliberately never requested.
		Scopes: []string{oidc.ScopeOpenID, "profile", "telegram:bot_access"},
	}
	t.verifier = p.Verifier(&oidc.Config{ClientID: t.cfg.ClientID})
	return nil
}

// Begin creates the state + PKCE verifier for one login attempt and returns
// the authorization URL to redirect the browser to.
func (t *TelegramOIDC) Begin(ctx context.Context, returnPath string) (string, error) {
	if t.cfg.ClientID == "" || t.cfg.ClientSecret == "" {
		return "", fmt.Errorf("telegram oidc not configured (TELEGRAM_OIDC_CLIENT_ID/SECRET)")
	}
	if err := t.ensureProvider(ctx); err != nil {
		return "", err
	}
	state := uuid.New().String()
	verifier := oauth2.GenerateVerifier()
	if err := t.cache.Set(ctx, oidcStateKeyPrefix+state, &oidcState{Verifier: verifier, ReturnPath: returnPath}, oidcStateTTL); err != nil {
		return "", fmt.Errorf("store oidc state: %w", err)
	}
	return t.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

// Complete consumes the state (single-use), exchanges the code, verifies the
// id_token, and maps its claims onto the TelegramWebhookUser shape consumed
// by AuthService.LoginWithTelegram. Also returns the stored return path.
func (t *TelegramOIDC) Complete(ctx context.Context, state, code string) (*domain.TelegramWebhookUser, string, error) {
	var st oidcState
	if err := t.cache.Get(ctx, oidcStateKeyPrefix+state, &st); err != nil {
		return nil, "", ErrOIDCStateExpired
	}
	// Single-use: delete before the exchange so a replayed callback cannot
	// race a second exchange with the same verifier.
	_ = t.cache.Delete(ctx, oidcStateKeyPrefix+state)

	if err := t.ensureProvider(ctx); err != nil {
		return nil, "", err
	}
	tok, err := t.oauth.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return nil, "", fmt.Errorf("code exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, "", fmt.Errorf("token response missing id_token")
	}
	idt, err := t.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, "", fmt.Errorf("verify id_token: %w", err)
	}
	var claims struct {
		Sub               string `json:"sub"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idt.Claims(&claims); err != nil {
		return nil, "", fmt.Errorf("parse claims: %w", err)
	}
	tgID, err := strconv.ParseInt(claims.Sub, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("non-numeric sub claim %q", claims.Sub)
	}
	return &domain.TelegramWebhookUser{
		ID:        tgID,
		FirstName: claims.Name,
		Username:  claims.PreferredUsername,
	}, st.ReturnPath, nil
}
