package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPITokens_GenerateVerifyRevoke(t *testing.T) {
	store := NewAPITokens(filepath.Join(t.TempDir(), "api_token"))

	if store.Verify("explo_bogus") {
		t.Fatal("Verify accepted a token before any was generated")
	}

	token, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(token, tokenPrefix) {
		t.Errorf("token %q lacks the recognizable prefix", token)
	}
	if !store.Verify(token) {
		t.Error("Verify rejected the just-generated token")
	}
	if store.Verify(token + "x") {
		t.Error("Verify accepted a tampered token")
	}
	if store.Verify("not-our-prefix-token") {
		t.Error("Verify accepted a foreign token")
	}

	// Only the hash is stored: the file must not contain the token.
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(store.path), "api_token"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Error("token file contains the raw token; only the hash may be stored")
	}
	if info, err := os.Stat(store.path); err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("token file mode = %v, err = %v; want 0600", info.Mode(), err)
	}

	// Regenerating rotates: the old token must stop working.
	token2, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !store.Verify(token2) {
		t.Error("Verify rejected the rotated token")
	}
	if store.Verify(token) {
		t.Error("Verify still accepted the pre-rotation token")
	}
}

func TestAPITokens_RevokeInvalidates(t *testing.T) {
	store := NewAPITokens(filepath.Join(t.TempDir(), "api_token"))
	token, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if store.Verify(token) {
		t.Error("Verify accepted a revoked token")
	}
	if err := store.Revoke(); err != nil {
		t.Errorf("double Revoke: %v", err)
	}
}
