package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"explo/src/web/backend/app"
	"explo/src/web/backend/defs"
)

// newSettings points a Settings at a throwaway .env.
func newSettings(t *testing.T) (*Settings, string) {
	t.Helper()

	envPath := filepath.Join(t.TempDir(), ".env")

	return NewSettings(app.Config{WebEnvPath: envPath}), envPath
}

// postStep3 drives the wizard's downloader step with the given body.
func postStep3(t *testing.T, s *Settings, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ui/wizard/step3", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	s.HandleWizardStep3(rec, req)

	return rec
}

// The toggle is only useful if it reaches the .env the downloader reads.
func TestWizardStep3_PersistsAlbumMode(t *testing.T) {
	tests := []struct {
		name string
		on   bool
		want string
	}{
		{"enabled", true, "SLSKD_ALBUM_MODE=true"},
		{"disabled", false, "SLSKD_ALBUM_MODE=false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, envPath := newSettings(t)

			rec := postStep3(t, s, map[string]any{
				"download_services": []string{"slskd"},
				"slskd_album_mode":  tt.on,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("step3 = %d (%s), want 200", rec.Code, rec.Body.String())
			}

			written, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatalf("reading written env: %v", err)
			}
			if !bytes.Contains(written, []byte(tt.want)) {
				t.Errorf("written env does not contain %q:\n%s", tt.want, written)
			}
		})
	}
}

// AllConfigKeys is what /api/config reads back. Leaving the key out of it is
// silent: the wizard writes the toggle, then shows it as off next time it is
// opened, because the value never reaches the frontend.
func TestAlbumModeIsReadableByTheUI(t *testing.T) {
	if !slices.Contains(defs.AllConfigKeys, "SLSKD_ALBUM_MODE") {
		t.Error("SLSKD_ALBUM_MODE missing from AllConfigKeys; the wizard toggle would always read back as off")
	}
}

// The bitrate range is only useful if the wizard writes what the downloader
// reads.
func TestWizardStep3_PersistsTheBitrateRange(t *testing.T) {
	s, envPath := newSettings(t)

	rec := postStep3(t, s, map[string]any{
		"download_services": []string{"slskd"},
		"extensions":        "flac,mp3",
		"min_bitrate":       192,
		"max_bitrate":       320,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("step3 = %d (%s), want 200", rec.Code, rec.Body.String())
	}

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	for _, want := range []string{"MIN_BITRATE=192", "MAX_BITRATE=320"} {
		if !bytes.Contains(written, []byte(want)) {
			t.Errorf("written env does not contain %q:\n%s", want, written)
		}
	}
}

// A zero ceiling is a real setting -- it means "no limit" -- so it has to be
// written rather than treated as absent.
func TestWizardStep3_WritesAZeroCeiling(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{
		"download_services": []string{"slskd"},
		"min_bitrate":       256,
		"max_bitrate":       0,
	})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	if !bytes.Contains(written, []byte("MAX_BITRATE=0")) {
		t.Errorf("a zero ceiling was not written:\n%s", written)
	}
}

// AllConfigKeys is what /api/config returns. A key missing from it is written
// and then never read back, so the control silently resets to its default on
// every reload with nothing to explain why.
func TestBitrateRangeIsReadableByTheUI(t *testing.T) {
	for _, key := range []string{"MIN_BITRATE", "MAX_BITRATE"} {
		if !slices.Contains(defs.AllConfigKeys, key) {
			t.Errorf("%s missing from AllConfigKeys; the control would always read back as its default", key)
		}
	}
}

// The preference is only useful if the wizard writes what the downloader reads.
func TestWizardStep3_PersistsTheSizePreference(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{
		"download_services": []string{"slskd"},
		"size_preference":   "smaller",
	})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	if !bytes.Contains(written, []byte("SIZE_PREFERENCE=smaller")) {
		t.Errorf("written env does not record the preference:\n%s", written)
	}
}

// An absent value must record the default rather than blanking the key, which
// is what UpdateEnvKeys does with an empty string.
func TestWizardStep3_DefaultsTheSizePreference(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{"download_services": []string{"slskd"}})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	assertEnvValue(t, written, "SIZE_PREFERENCE", "none")
}

func TestSizePreferenceIsReadableByTheUI(t *testing.T) {
	if !slices.Contains(defs.AllConfigKeys, "SIZE_PREFERENCE") {
		t.Error("SIZE_PREFERENCE missing from AllConfigKeys; the control would always read back as its default")
	}
}

// envValue reads a key's written value, ignoring commented lines. sample.env
// documents every key as a comment carrying its default, so a plain substring
// search for "KEY=default" matches that comment and passes whether or not the
// wizard wrote anything at all.
func envValue(written []byte, key string) string {
	for _, line := range strings.Split(string(written), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if name, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func assertEnvValue(t *testing.T, written []byte, key, want string) {
	t.Helper()

	if got := envValue(written, key); got != want {
		t.Errorf("%s = %q, want %q\n%s", key, got, want, written)
	}
}

func TestWizardStep3_PersistsTheReleasePreference(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{
		"download_services":  []string{"slskd"},
		"release_preference": "smaller",
	})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	if !bytes.Contains(written, []byte("RELEASE_PREFERENCE=smaller")) {
		t.Errorf("written env does not record the release preference:\n%s", written)
	}
}

// An absent value must record the default rather than blanking the key.
func TestWizardStep3_DefaultsTheReleasePreference(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{"download_services": []string{"slskd"}})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	assertEnvValue(t, written, "RELEASE_PREFERENCE", "smaller")
}

func TestReleasePreferenceIsReadableByTheUI(t *testing.T) {
	if !slices.Contains(defs.AllConfigKeys, "RELEASE_PREFERENCE") {
		t.Error("RELEASE_PREFERENCE missing from AllConfigKeys; the control would always read back as its default")
	}
}

func TestWizardStep3_PersistsPreferOriginalRelease(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{
		"download_services":       []string{"slskd"},
		"prefer_original_release": false,
	})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	assertEnvValue(t, written, "PREFER_ORIGINAL_RELEASE", "false")
}

// This one defaults to on, so an omitted key must not read as "off". A plain
// bool on the request body cannot tell the two apart, which is why the field is
// a pointer.
func TestWizardStep3_DefaultsPreferOriginalReleaseToOn(t *testing.T) {
	s, envPath := newSettings(t)

	postStep3(t, s, map[string]any{"download_services": []string{"slskd"}})

	written, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading written env: %v", err)
	}
	assertEnvValue(t, written, "PREFER_ORIGINAL_RELEASE", "true")
}

func TestPreferOriginalReleaseIsReadableByTheUI(t *testing.T) {
	if !slices.Contains(defs.AllConfigKeys, "PREFER_ORIGINAL_RELEASE") {
		t.Error("PREFER_ORIGINAL_RELEASE missing from AllConfigKeys; the toggle would always read back as its default")
	}
}
