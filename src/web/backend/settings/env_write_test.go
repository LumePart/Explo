package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A .env bind-mounted as a single file rejects the atomic rename with
// "device or resource busy". The write must fall back to truncating the
// file in place instead of failing every settings save.
func TestWriteEnvFileAtomic_FallsBackWhenRenameFails(t *testing.T) {
	old := renameFile
	t.Cleanup(func() { renameFile = old })
	renameFile = func(_, _ string) error {
		return errors.New("rename /opt/explo/.env-1940118763.tmp /opt/explo/.env: device or resource busy")
	}

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXISTING=1\n"), 0644); err != nil {
		t.Fatalf("seeding env file: %v", err)
	}

	if err := writeEnvFileAtomic(path, []byte("EXISTING=1\nALBUMS_ONLY=true\n")); err != nil {
		t.Fatalf("writeEnvFileAtomic() = %v, want the in-place fallback to succeed", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	if string(got) != "EXISTING=1\nALBUMS_ONLY=true\n" {
		t.Errorf("env file = %q, want the new content written in place", got)
	}
}

// When both the rename and the fallback fail, the caller must see both
// causes, not a silent success. Writing over a directory fails the
// fallback while the stubbed rename fails the fast path.
func TestWriteEnvFileAtomic_ReportsBothFailures(t *testing.T) {
	old := renameFile
	t.Cleanup(func() { renameFile = old })
	renameFile = func(_, _ string) error { return errors.New("device or resource busy") }

	dir := t.TempDir()
	err := writeEnvFileAtomic(dir, []byte("A=1\n"))
	if err == nil {
		t.Fatal("writeEnvFileAtomic() = nil, want an error when both paths fail")
	}
	t.Logf("combined error: %v", err)
}
