package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// PasskeyService implements usernameless, discoverable-credential WebAuthn
// registration and login. Ceremony state (the library's SessionData) rides
// in Redis behind a short-lived opaque ceremony id, keyed via
// cache.KeyWebAuthnCeremony — never trusted from the client.
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
			s.log.Warnw("skipping undecodable passkey credential", "passkey_id", rows[i].ID, "error", cerr)
			continue
		}
		creds = append(creds, c)
	}
	return &waUser{user: user, creds: creds}, rows, nil
}

func newCeremonyID() (string, error) {
	return randomHexToken(16)
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
	// GetDel reads and deletes the ceremony atomically (single round trip)
	// so a ceremony can never be replayed, mirroring the one-time
	// cert-login token pattern (ConsumeCertLoginToken).
	var session webauthn.SessionData
	if err := s.cache.GetDel(ctx, cache.KeyWebAuthnCeremony(ceremonyID), &session); err != nil {
		return nil, errors.InvalidInput("ceremony expired")
	}

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
	if err := s.cache.GetDel(ctx, cache.KeyWebAuthnCeremony(ceremonyID), &session); err != nil {
		return nil, errors.Unauthorized("ceremony expired")
	}

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
	s.warnOnCloneWarning(matchedUser.ID, matched.ID, cred.Authenticator.CloneWarning)
	_ = s.store.UpdateSignCount(ctx, matched.ID, cred.Authenticator.SignCount, timeNow())
	metrics.AuthEventsTotal.WithLabelValues("passkey_login", "success").Inc()
	return matchedUser, nil
}

// warnOnCloneWarning surfaces the authenticator's clone-detection signal
// (WebAuthn Level 2 §7.2 step 17: the assertion's signature counter did not
// strictly increase past the stored value, which the spec flags as evidence
// the credential's private key may have been cloned onto multiple
// authenticators). This is warn-only by product decision: the login is
// still allowed to proceed and the sign count is still updated by the
// caller — we only get the incident onto the radar via a structured log
// line plus the passkey_login/clone_warning metric.
func (s *PasskeyService) warnOnCloneWarning(userID, passkeyID string, cloneWarning bool) {
	if !cloneWarning {
		return
	}
	s.log.Warnw("passkey clone warning: sign count regression", "user_id", userID, "passkey_id", passkeyID)
	metrics.AuthEventsTotal.WithLabelValues("passkey_login", "clone_warning").Inc()
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
