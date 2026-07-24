package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// softwareAuthenticator is a minimal in-memory WebAuthn authenticator: it
// fabricates real attestation ("none" format) and assertion payloads with a
// P-256 key, so the full FinishRegistration → FinishLogin ceremony can run
// against the real go-webauthn validation, no browser needed. flags lets a
// test model a synced passkey (BE|BS set — what iCloud Keychain and Google
// Password Manager always send) vs a device-bound one.
type softwareAuthenticator struct {
	priv   *ecdsa.PrivateKey
	credID []byte
	flags  byte
}

const (
	testRPID   = "localhost"
	testOrigin = "https://localhost"

	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
	flagAT = 0x40
)

func newSoftwareAuthenticator(t *testing.T, flags byte) *softwareAuthenticator {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		t.Fatalf("credID: %v", err)
	}
	return &softwareAuthenticator{priv: priv, credID: credID, flags: flags}
}

// cborEncMode returns the CTAP2 canonical CBOR encoder shared by the
// attestation object and COSE key encodings below.
func cborEncMode(t *testing.T) cbor.EncMode {
	t.Helper()
	em, err := cbor.CTAP2EncOptions().EncMode()
	if err != nil {
		t.Fatalf("cbor enc mode: %v", err)
	}
	return em
}

func (a *softwareAuthenticator) coseKey(t *testing.T) []byte {
	t.Helper()
	xb := make([]byte, 32)
	yb := make([]byte, 32)
	a.priv.X.FillBytes(xb)
	a.priv.Y.FillBytes(yb)
	em := cborEncMode(t)
	out, err := em.Marshal(map[int]any{
		1:  2,  // kty: EC2
		3:  -7, // alg: ES256
		-1: 1,  // crv: P-256
		-2: xb,
		-3: yb,
	})
	if err != nil {
		t.Fatalf("cose marshal: %v", err)
	}
	return out
}

func (a *softwareAuthenticator) authData(t *testing.T, counter uint32, attested bool) []byte {
	t.Helper()
	h := sha256.Sum256([]byte(testRPID))
	var buf bytes.Buffer
	buf.Write(h[:])
	flags := a.flags
	if attested {
		flags |= flagAT
	}
	buf.WriteByte(flags)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], counter)
	buf.Write(c[:])
	if attested {
		buf.Write(make([]byte, 16)) // zero AAGUID
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(a.credID)))
		buf.Write(l[:])
		buf.Write(a.credID)
		buf.Write(a.coseKey(t))
	}
	return buf.Bytes()
}

func clientDataJSON(t *testing.T, typ, challenge string) []byte {
	t.Helper()
	j, err := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   challenge,
		"origin":      testOrigin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatalf("clientData: %v", err)
	}
	return j
}

func jsonBodyRequest(t *testing.T, body map[string]any) *http.Request {
	t.Helper()
	j, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, testOrigin+"/finish", bytes.NewReader(j))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// attestationRequest builds the register/finish body for the given challenge.
func (a *softwareAuthenticator) attestationRequest(t *testing.T, challenge string) *http.Request {
	t.Helper()
	em := cborEncMode(t)
	attObj, err := em.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": a.authData(t, 0, true),
	})
	if err != nil {
		t.Fatalf("attObj marshal: %v", err)
	}
	return jsonBodyRequest(t, map[string]any{
		"id":    b64u(a.credID),
		"rawId": b64u(a.credID),
		"type":  "public-key",
		"response": map[string]any{
			"attestationObject": b64u(attObj),
			"clientDataJSON":    b64u(clientDataJSON(t, "webauthn.create", challenge)),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": map[string]any{},
	})
}

// assertionRequest builds the login/finish body for the given challenge.
func (a *softwareAuthenticator) assertionRequest(t *testing.T, challenge, userID string, counter uint32) *http.Request {
	t.Helper()
	ad := a.authData(t, counter, false)
	cd := clientDataJSON(t, "webauthn.get", challenge)
	cdHash := sha256.Sum256(cd)
	signed := append(append([]byte{}, ad...), cdHash[:]...)
	digest := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, a.priv, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return jsonBodyRequest(t, map[string]any{
		"id":    b64u(a.credID),
		"rawId": b64u(a.credID),
		"type":  "public-key",
		"response": map[string]any{
			"authenticatorData": b64u(ad),
			"clientDataJSON":    b64u(cd),
			"signature":         b64u(sig),
			"userHandle":        b64u([]byte(userID)),
		},
		"clientExtensionResults": map[string]any{},
	})
}

// enrollAndLogin runs the full ceremony for an authenticator with the given
// flags and returns FinishLogin's result.
func enrollAndLogin(t *testing.T, flags byte) (*domain.User, error) {
	t.Helper()
	svc, _, users := newTestPasskeyService(t)
	user := &domain.User{ID: "11111111-2222-3333-4444-555555555555", Username: "alice"}
	users.users[user.ID] = user
	auth := newSoftwareAuthenticator(t, flags)
	ctx := context.Background()

	opts, ceremonyID, err := svc.BeginRegistration(ctx, user)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	row, err := svc.FinishRegistration(ctx, user, ceremonyID, "test key", auth.attestationRequest(t, opts.Response.Challenge.String()))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	if row.CredentialID != b64u(auth.credID) {
		t.Fatalf("stored CredentialID = %q, want %q", row.CredentialID, b64u(auth.credID))
	}

	loginOpts, loginCeremonyID, err := svc.BeginLogin(ctx)
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	return svc.FinishLogin(ctx, loginCeremonyID, auth.assertionRequest(t, loginOpts.Response.Challenge.String(), user.ID, 1))
}

// TestPasskeyCeremony_SyncedPasskey is the regression test for the 2026-07-24
// production 401: synced passkeys (iCloud Keychain, Google Password Manager)
// set the Backup Eligible + Backup State flags, and go-webauthn rejects a
// login whose stored credential does not carry the same BackupEligible flag
// ("BackupEligible flag inconsistency detected during login validation").
// The flags must therefore survive the DB round trip.
func TestPasskeyCeremony_SyncedPasskey(t *testing.T) {
	got, err := enrollAndLogin(t, flagUP|flagUV|flagBE|flagBS)
	if err != nil {
		t.Fatalf("FinishLogin with synced-passkey flags: %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("logged-in user = %q, want alice", got.Username)
	}
}

// TestPasskeyCeremony_DeviceBoundPasskey covers the no-BE/BS path (hardware
// keys without sync) — the pre-fix behavior that must keep working.
func TestPasskeyCeremony_DeviceBoundPasskey(t *testing.T) {
	got, err := enrollAndLogin(t, flagUP|flagUV)
	if err != nil {
		t.Fatalf("FinishLogin with device-bound flags: %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("logged-in user = %q, want alice", got.Username)
	}
}
