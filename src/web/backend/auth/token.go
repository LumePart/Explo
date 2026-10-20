package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tokenPrefix marks Explo API tokens so they are recognizable in logs
// and config dumps (only the prefix is ever logged, never the secret).
const tokenPrefix = "explo_"

// tokenSecretBytes is the token entropy: 256 bits.
const tokenSecretBytes = 32

// APITokens issues and verifies read-only diagnostics tokens. Only the
// SHA256 hash is ever stored: a leaked token file shows nothing usable.
type APITokens struct {
	path string
}

// NewAPITokens stores the token hash at path (created with 0600).
func NewAPITokens(path string) *APITokens {
	return &APITokens{path: path}
}

// DefaultTokenPath keeps the hash inside the web data dir.
func DefaultTokenPath(webDataDir string) string {
	return filepath.Join(webDataDir, "api_token")
}

// Generate creates a fresh token, stores its hash (revoking any prior
// token), and returns the full token text. The text is shown once.
func (t *APITokens) Generate() (string, error) {
	secret := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := tokenPrefix + hex.EncodeToString(secret)
	sum := sha256.Sum256([]byte(token))
	if err := os.MkdirAll(filepath.Dir(t.path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(t.path, []byte(hex.EncodeToString(sum[:])+"\n"), 0600); err != nil {
		return "", err
	}
	return token, nil
}

// Revoke deletes the stored token hash, invalidating any issued token.
func (t *APITokens) Revoke() error {
	if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Verify reports whether token matches the stored hash, in constant time.
func (t *APITokens) Verify(token string) bool {
	if !strings.HasPrefix(token, tokenPrefix) {
		return false
	}
	raw, err := os.ReadFile(t.path)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(want) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}
