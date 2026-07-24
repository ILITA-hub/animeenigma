package domain

import "time"

// WebAuthnCredential is one enrolled passkey. CredentialID is the
// base64url-encoded raw credential id (what authenticators return); the
// webauthn library's Credential.ID round-trips through it.
type WebAuthnCredential struct {
	ID           string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string `gorm:"type:uuid;index" json:"-"`
	CredentialID string `gorm:"type:text;uniqueIndex" json:"-"`
	PublicKey    []byte `json:"-"`
	SignCount    uint32 `json:"-"`
	Transports   string `gorm:"size:128" json:"-"`
	AAGUID       []byte `json:"-"`
	// BackupEligible/BackupState mirror the authenticator's BE/BS flags from
	// enrollment. go-webauthn rejects any login whose stored BackupEligible
	// differs from the assertion's BE flag, and synced passkeys (iCloud
	// Keychain, Google Password Manager) always set it — dropping the flag
	// breaks every such login with a bare 401.
	BackupEligible bool       `json:"-"`
	BackupState    bool       `json:"-"`
	Name           string     `gorm:"size:64" json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// TableName pins the GORM table name to webauthn_credentials (the default
// pluralization would otherwise land on "web_authn_credentials").
func (WebAuthnCredential) TableName() string {
	return "webauthn_credentials"
}
