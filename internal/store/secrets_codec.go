package store

import (
	"fmt"

	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/secrets"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

// sealValue encrypts a secret plaintext for storage. Plain sensitivity is stored as-is.
// Legacy: if box is nil and sensitivity is secret, refuse (S2 requires key for new secret material).
func (s *Store) sealValue(plaintext, sensitivity string) (string, error) {
	if !domain.IsSecret(sensitivity) {
		return plaintext, nil
	}
	if s.secrets == nil {
		return "", fmt.Errorf("%w: %s required to write secret values", launchpad.ErrBadRequest, secrets.EnvKey)
	}
	return s.secrets.Encrypt(plaintext)
}

// openValue decrypts a stored secret value for in-memory use.
// Non-secret values and legacy S1 plaintext secrets (no v1: prefix) pass through.
func (s *Store) openValue(stored, sensitivity string) (string, error) {
	if !domain.IsSecret(sensitivity) {
		return stored, nil
	}
	if !secrets.IsCiphertext(stored) {
		// Pre-S2 / unmigrated secret row: still plaintext in DB.
		return stored, nil
	}
	if s.secrets == nil {
		return "", fmt.Errorf("%w: %s required to decrypt secret values", launchpad.ErrBadRequest, secrets.EnvKey)
	}
	return s.secrets.Decrypt(stored)
}

// openConfigMaps decrypts secret values in a values map using the parallel sensitivity map.
func (s *Store) openConfigMaps(vals, sens map[string]string) (map[string]string, error) {
	if vals == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		sensitivity := ""
		if sens != nil {
			sensitivity = sens[k]
		}
		opened, err := s.openValue(v, sensitivity)
		if err != nil {
			return nil, fmt.Errorf("config key %q: %w", k, err)
		}
		out[k] = opened
	}
	return out, nil
}

// sealConfigMaps encrypts secret keys for durable storage (live tables or release snapshot).
func (s *Store) sealConfigMaps(vals, sens map[string]string) (map[string]string, error) {
	if vals == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		sensitivity := domain.SensitivityPlain
		if sens != nil && sens[k] != "" {
			sensitivity = sens[k]
		}
		sealed, err := s.sealValue(v, sensitivity)
		if err != nil {
			return nil, fmt.Errorf("config key %q: %w", k, err)
		}
		out[k] = sealed
	}
	return out, nil
}
