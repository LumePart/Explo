package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"explo/src/web/backend/app"
)

// newSettingsInDir is newSettings with a known data dir, so tests can seed users.json.
func newSettingsInDir(t *testing.T) (*Settings, string, string) {
	t.Helper()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	return NewSettings(app.Config{WebEnvPath: envPath, WebDataDir: dir}), envPath, dir
}

func seedUsers(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const oneUser = `[{"id":"gabe","name":"Gabe","enabled":true}]`

func postWizard(t *testing.T, h func(http.ResponseWriter, *http.Request), body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/api/ui/wizard", bytes.NewReader(payload)))

	return rec
}

func readEnv(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	return string(b)
}

func TestWizardStep1_MultiUser(t *testing.T) {
	t.Run("absent keys keep stored values", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		if err := os.WriteFile(envPath, []byte("LISTENBRAINZ_USER=old\nLISTENBRAINZ_USER_TOKEN=oldtok\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rec := postWizard(t, s.HandleWizardStep1, map[string]any{"discovery_mode": "api", "playlists": []string{}})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		env := readEnv(t, envPath)
		if !strings.Contains(env, "LISTENBRAINZ_USER=old\n") || !strings.Contains(env, "LISTENBRAINZ_USER_TOKEN=oldtok") {
			t.Errorf("stored values changed:\n%s", env)
		}
	})
	t.Run("absent keys with no prior values write nothing", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		rec := postWizard(t, s.HandleWizardStep1, map[string]any{"discovery_mode": "api"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		env := readEnv(t, envPath)
		for _, k := range []string{"LISTENBRAINZ_USER=", "LISTENBRAINZ_USER_TOKEN="} {
			for line := range strings.SplitSeq(env, "\n") {
				if strings.HasPrefix(line, k) && line != k {
					t.Errorf("unexpected %q", line)
				}
			}
		}
		if !strings.Contains(env, "LISTENBRAINZ_DISCOVERY=api") {
			t.Errorf("discovery not saved:\n%s", env)
		}
	})
	t.Run("explicit value is written", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		rec := postWizard(t, s.HandleWizardStep1, map[string]any{"user": "new", "userToken": "tok", "discovery_mode": "api"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if env := readEnv(t, envPath); !strings.Contains(env, "LISTENBRAINZ_USER=new") || !strings.Contains(env, "LISTENBRAINZ_USER_TOKEN=tok") {
			t.Errorf("not written:\n%s", env)
		}
	})
	t.Run("explicit empty user still 400", func(t *testing.T) {
		s, _, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		if rec := postWizard(t, s.HandleWizardStep1, map[string]any{"user": ""}); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("no users: empty user 400", func(t *testing.T) {
		s, _, _ := newSettingsInDir(t)
		if rec := postWizard(t, s.HandleWizardStep1, map[string]any{}); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("no users: values written", func(t *testing.T) {
		s, envPath, _ := newSettingsInDir(t)
		rec := postWizard(t, s.HandleWizardStep1, map[string]any{"user": "u", "userToken": "t", "discovery_mode": "api"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if env := readEnv(t, envPath); !strings.Contains(env, "LISTENBRAINZ_USER=u") || !strings.Contains(env, "LISTENBRAINZ_USER_TOKEN=t") {
			t.Errorf("not written:\n%s", env)
		}
	})
	t.Run("corrupt users.json 500", func(t *testing.T) {
		s, _, dir := newSettingsInDir(t)
		seedUsers(t, dir, "{not json")
		if rec := postWizard(t, s.HandleWizardStep1, map[string]any{"user": "u"}); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}

func TestWizardStep2_MultiUser(t *testing.T) {
	base := map[string]any{"system": "plex", "url": "http://x"}
	t.Run("absent username keeps stored", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		if err := os.WriteFile(envPath, []byte("SYSTEM_USERNAME=plexold\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if rec := postWizard(t, s.HandleWizardStep2, base); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if env := readEnv(t, envPath); !strings.Contains(env, "SYSTEM_USERNAME=plexold") {
			t.Errorf("username lost:\n%s", env)
		}
	})
	t.Run("absent username with no prior writes nothing", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		if rec := postWizard(t, s.HandleWizardStep2, base); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		for line := range strings.SplitSeq(readEnv(t, envPath), "\n") {
			if strings.HasPrefix(line, "SYSTEM_USERNAME=") && line != "SYSTEM_USERNAME=" {
				t.Errorf("unexpected %q", line)
			}
		}
	})
	t.Run("explicit username written", func(t *testing.T) {
		s, envPath, dir := newSettingsInDir(t)
		seedUsers(t, dir, oneUser)
		body := map[string]any{"system": "plex", "url": "http://x", "username": "newname"}
		if rec := postWizard(t, s.HandleWizardStep2, body); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if env := readEnv(t, envPath); !strings.Contains(env, "SYSTEM_USERNAME=newname") {
			t.Errorf("not written:\n%s", env)
		}
	})
	t.Run("no users: username written, empty clears", func(t *testing.T) {
		s, envPath, _ := newSettingsInDir(t)
		if err := os.WriteFile(envPath, []byte("SYSTEM_USERNAME=old\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if rec := postWizard(t, s.HandleWizardStep2, base); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if env := readEnv(t, envPath); strings.Contains(env, "SYSTEM_USERNAME=old") {
			t.Errorf("expected overwrite as today:\n%s", env)
		}
	})
	t.Run("corrupt users.json 500", func(t *testing.T) {
		s, _, dir := newSettingsInDir(t)
		seedUsers(t, dir, "{not json")
		if rec := postWizard(t, s.HandleWizardStep2, base); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}
