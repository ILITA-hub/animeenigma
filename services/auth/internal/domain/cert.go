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

// CAInfo describes the platform user CA for user-facing display: the
// settings modal shows these fingerprints so users can compare them against
// the OS trust prompt raised when importing a .p12 that bundles this CA.
// SHA-1 is included because Windows' prompt shows a SHA-1 thumbprint.
type CAInfo struct {
	Subject           string `json:"subject"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	FingerprintSHA1   string `json:"fingerprint_sha1"`
}
