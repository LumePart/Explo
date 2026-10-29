package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// admin_configured tells the UI whether Plex admin credentials exist without
// ever exposing them.
func TestHandleGetConfig_AdminConfigured(t *testing.T) {
	const (
		key  = "SECRET-ADMIN-KEY-111"
		user = "secret-admin-user-222"
		pass = "SECRET-ADMIN-PASS-333"
	)
	tests := []struct {
		name string
		file string
		env  map[string]string
		want bool
	}{
		{"api key only", "ADMIN_API_KEY=" + key + "\n", nil, true},
		{"user and password", "ADMIN_SYSTEM_USERNAME=" + user + "\nADMIN_SYSTEM_PASSWORD=" + pass + "\n", nil, true},
		{"user without password", "ADMIN_SYSTEM_USERNAME=" + user + "\n", nil, false},
		{"none", "", nil, false},
		{"env only api key", "", map[string]string{"ADMIN_API_KEY": key}, true},
		{"env user and file password", "ADMIN_SYSTEM_PASSWORD=" + pass + "\n", map[string]string{"ADMIN_SYSTEM_USERNAME": user}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"ADMIN_API_KEY", "ADMIN_SYSTEM_USERNAME", "ADMIN_SYSTEM_PASSWORD"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			s, envPath := newSettings(t)
			if err := os.WriteFile(envPath, []byte(tt.file), 0o600); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			s.HandleGetConfig(rec, httptest.NewRequest(http.MethodGet, "/api/ui/config", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var resp struct {
				AdminConfigured *bool `json:"admin_configured"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.AdminConfigured == nil || *resp.AdminConfigured != tt.want {
				t.Errorf("admin_configured = %v, want %v", resp.AdminConfigured, tt.want)
			}
			for _, secret := range []string{key, user, pass} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Errorf("response leaks %q: %s", secret, rec.Body.String())
				}
			}
		})
	}
}

func TestWizardStep2_WritesAdminAPIKeyUnderTheKeyConfigReads(t *testing.T) {
	s, envPath := newSettings(t)
	payload, _ := json.Marshal(map[string]any{"system": "plex", "admin_api_key": "k123"})
	rec := httptest.NewRecorder()
	s.HandleWizardStep2(rec, httptest.NewRequest(http.MethodPost, "/api/ui/wizard/step2", bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(written, []byte("ADMIN_API_KEY=k123")) {
		t.Errorf("env lacks ADMIN_API_KEY=k123:\n%s", written)
	}
	if bytes.Contains(written, []byte("ADMIN_SYSTEM_APIKEY=k123")) {
		t.Errorf("env still has ADMIN_SYSTEM_APIKEY=k123:\n%s", written)
	}
}
