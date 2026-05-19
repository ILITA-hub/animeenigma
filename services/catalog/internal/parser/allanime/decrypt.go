package allanime

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// allanimeKeyMaterial is the static input AllAnime uses to derive the AES key
// for its `tobeparsed` blobs. The actual key is SHA256(allanimeKeyMaterial).
// Sourced from pystardust/ani-cli; matches AllAnime's web client.
const allanimeKeyMaterial = "Xot36i3lK3:v1"

// decryptTobeparsed decodes the AES-256-CTR `tobeparsed` blob returned by
// AllAnime's sources resolver into the underlying GraphQL data JSON
// (`{"episode":{"sourceUrls":[...]}}`).
//
// Format of the raw blob (after base64 decode):
//
//	[1 byte version (0x01)] [12-byte IV] [N-byte ciphertext] [16-byte GCM tag]
//
// The CTR counter is the 12-byte IV followed by the 32-bit big-endian value
// 0x00000002 (AES-GCM's per-block counter, starting at 2 because counter 1
// is reserved for the auth tag and 0 for the J0 init). We discard the trailing
// tag and decrypt as plain CTR — AllAnime's web client doesn't verify it.
func decryptTobeparsed(blob string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, err
	}
	if len(raw) < 1+12+16 {
		return nil, errors.New("allanime: tobeparsed blob too short")
	}

	iv := raw[1:13]
	ct := raw[13 : len(raw)-16]

	key := sha256.Sum256([]byte(allanimeKeyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	// CTR initial counter = IV || 0x00000002 (32-bit big-endian, starting at 2).
	ctr := make([]byte, 16)
	copy(ctr, iv)
	ctr[12], ctr[13], ctr[14], ctr[15] = 0x00, 0x00, 0x00, 0x02

	out := make([]byte, len(ct))
	cipher.NewCTR(block, ctr).XORKeyStream(out, ct)
	return out, nil
}
