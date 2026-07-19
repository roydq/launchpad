// Package secrets provides AES-GCM encryption for secret-typed config values (S2).
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

const (
	// EnvKey is the control-plane env var for the 32-byte AES key (base64-encoded).
	EnvKey = "LAUNCHPAD_SECRETS_KEY"

	// CipherPrefix marks ciphertext stored in config value columns / release snapshots.
	CipherPrefix = "v1:"

	keyBytes = 32
)

// Box encrypts and decrypts secret config values with AES-256-GCM.
type Box struct {
	gcm cipher.AEAD
}

// ParseKey builds a Box from a base64-encoded 32-byte key.
func ParseKey(encoded string) (*Box, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("%w: empty secrets key", launchpad.ErrBadRequest)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Also accept raw URL-safe base64 without padding variants.
		raw, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: secrets key must be base64: %v", launchpad.ErrBadRequest, err)
		}
	}
	if len(raw) != keyBytes {
		return nil, fmt.Errorf("%w: secrets key must decode to %d bytes, got %d", launchpad.ErrBadRequest, keyBytes, len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

// LoadFromEnv returns a Box from LAUNCHPAD_SECRETS_KEY, or (nil, nil) if unset.
// Invalid key material returns an error (fail loud at process start).
func LoadFromEnv() (*Box, error) {
	v := os.Getenv(EnvKey)
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	return ParseKey(v)
}

// IsCiphertext reports whether s uses the S2 ciphertext encoding.
func IsCiphertext(s string) bool {
	return strings.HasPrefix(s, CipherPrefix)
}

// Encrypt seals plaintext and returns CipherPrefix + base64(nonce||ciphertext).
func (b *Box) Encrypt(plaintext string) (string, error) {
	if b == nil || b.gcm == nil {
		return "", fmt.Errorf("%w: %s required to write secret values", launchpad.ErrBadRequest, EnvKey)
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := b.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return CipherPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a value produced by Encrypt.
func (b *Box) Decrypt(blob string) (string, error) {
	if b == nil || b.gcm == nil {
		return "", fmt.Errorf("%w: %s required to decrypt secret values", launchpad.ErrBadRequest, EnvKey)
	}
	if !IsCiphertext(blob) {
		return "", fmt.Errorf("%w: not a secrets ciphertext", launchpad.ErrBadRequest)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(blob, CipherPrefix))
	if err != nil {
		return "", fmt.Errorf("%w: corrupt secret ciphertext", launchpad.ErrBadRequest)
	}
	ns := b.gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("%w: corrupt secret ciphertext", launchpad.ErrBadRequest)
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("%w: secret decrypt failed (wrong key or corrupt data)", launchpad.ErrBadRequest)
	}
	return string(plain), nil
}
