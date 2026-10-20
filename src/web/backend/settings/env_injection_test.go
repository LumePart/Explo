package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"explo/src/web/backend/app"
)

// BUG-09: a settings value containing a line break was written raw into
// .env, injecting arbitrary extra keys on the next start.
func TestUpdateEnvKeys_RejectsLineBreakInjection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := "EXISTING=1\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Settings{cfg: app.Config{WebEnvPath: path}}

	for name, updates := range map[string]map[string]string{
		"value newline": {"NOTE": "x\nAPI_KEY=pwned"},
		"value CR":      {"NOTE": "x\rAPI_KEY=pwned"},
		"key newline":   {"NOTE\nAPI_KEY": "pwned"},
		"key equals":    {"NOTE=API_KEY": "pwned"},
		"empty key":     {"": "pwned"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := s.UpdateEnvKeys(updates, nil); err == nil {
				t.Errorf("UpdateEnvKeys(%v) = nil, want rejection", updates)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Errorf("env file changed after rejected update: %q", data)
			}
		})
	}
}

func TestUpdateEnvKeys_StillWritesNormalValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("EXISTING=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Settings{cfg: app.Config{WebEnvPath: path}}

	if err := s.UpdateEnvKeys(map[string]string{"NOTE": "hello world #1", "NEW_KEY": "abc"}, nil); err != nil {
		t.Fatalf("UpdateEnvKeys normal values: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s.ParseEnvText(string(data))
	if got["NOTE"] != "hello world #1" || got["NEW_KEY"] != "abc" || got["EXISTING"] != "1" {
		t.Errorf("parsed env = %v, want NOTE/NEW_KEY written and EXISTING preserved; raw:\n%s", got, data)
	}
	if strings.Contains(string(data), "pwned") {
		t.Error("injected content present")
	}
}
