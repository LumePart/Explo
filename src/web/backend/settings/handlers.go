package settings

import (
	"net/http"
	"encoding/json"
	"os"
	"log/slog"
	"io"
	"syscall"
	"time"
	"fmt"
	"strings"
	"net/url"

	"explo/src/web"
	"explo/src/web/backend/defs"
	"explo/src/util"
)

// handleGetConfig returns resolved config as JSON: { values, sources }.
// File keys are checked first because cleanenv sets them as OS env vars on startup,
// so checking os.LookupEnv first would misclassify all file keys as "env".
// Only keys present in the OS environment but absent from the file are marked "env".
func (s *Settings) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.cfg.WebEnvPath)
	var fileValues map[string]string
	if err == nil {
		fileValues = s.ParseEnvText(string(data))
	} else {
		fileValues = s.ParseEnvText(string(web.SampleEnv))
	}

	configKeys := defs.AllConfigKeys
	values := make(map[string]string, len(configKeys))
	sources := make(map[string]string, len(configKeys))
	for _, key := range configKeys {
		if v, ok := fileValues[key]; ok && v != "" {
			values[key] = v
			sources[key] = "file"
		} else if v, ok := os.LookupEnv(key); ok && v != "" {
			values[key] = v
			sources[key] = "env"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ConfigResponse{Values: values, Sources: sources}); err != nil {
		slog.Error("failed encoding config to http", "msg", err.Error())
	}
}

// handleGetConfigRaw returns the raw .env file contents as plain text.
func (s *Settings) HandleGetConfigRaw(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.cfg.WebEnvPath)
	if err != nil {
		data = web.SampleEnv
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		slog.Error("failed writing http response", "msg", err.Error())
	}
}

// handleSaveConfig writes the posted plain-text body directly to the .env file.
func (s *Settings) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(s.cfg.WebEnvPath, data, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleResetConfig resets all settings and restarts the container.
func (s *Settings) HandleResetConfig(w http.ResponseWriter, r *http.Request) {
	if err := os.WriteFile(s.cfg.WebEnvPath, web.SampleEnv, 0600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := syscall.Kill(1, syscall.SIGTERM); err != nil {
			slog.Warn("failed to kill process", "msg", err.Error())
		}

	}()
}

// handleSaveSchedule updates a single playlist's schedule in the .env file.
func (s *Settings) HandleSaveSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Day     int    `json:"day"` // 0=Sun…6=Sat, -1=every day
		Hour    int    `json:"hour"`
		Minute  int    `json:"minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	var envPrefix string
	var defaultFlags string

	if def, ok := defs.PlaylistDefs[body.Name]; ok {
		envPrefix = def.EnvPrefix
		defaultFlags = def.DefaultFlags
	} else if defs.CustomIDRe.MatchString(body.Name) {
		envPrefix = util.CustomEnvPrefix(body.Name)
		defaultFlags = "--playlist " + body.Name
	} else {
		http.Error(w, "unknown playlist name", http.StatusBadRequest)
		return
	}

	updates := map[string]string{}
	if !body.Enabled {
		// Toggle off — truly disable, regardless of day value carried over from state
		updates[envPrefix+"_SCHEDULE"] = ""
		updates[envPrefix+"_FLAGS"] = ""
	} else if body.Day == -2 {
		// "Never" — keep playlist active for manual runs but remove auto-schedule
		updates[envPrefix+"_SCHEDULE"] = ""
		updates[envPrefix+"_FLAGS"] = defaultFlags
	} else {
		dom := "*"
		dow := "*"
		if body.Day == 100 {
			dom = "1"
		} else if body.Day >= 0 {
			dow = fmt.Sprintf("%d", body.Day)
		}
		updates[envPrefix+"_SCHEDULE"] = fmt.Sprintf("%d %d %s * %s", body.Minute, body.Hour, dom, dow)
		updates[envPrefix+"_FLAGS"] = defaultFlags
	}

	if err := s.UpdateEnvKeys(updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleSavePathTemplate writes the PATH_TEMPLATE key to the .env file.
func (s *Settings) HandleSavePathTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Template string `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.UpdateEnvKeys(map[string]string{"PATH_TEMPLATE": body.Template}, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleSaveEnrichMetadata writes ENRICH_TRACK_METADATA=true/false to the .env file.
func (s *Settings) HandleSaveEnrichMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	val := "false"
	if body.Enabled {
		val = "true"
	}
	if err := s.UpdateEnvKeys(map[string]string{"ENRICH_TRACK_METADATA": val}, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleWizardStep1 saves discovery settings (username + enabled playlists with default schedules).
func (s *Settings) HandleWizardStep1(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User          string   `json:"user"`
		UserToken     string   `json:"userToken"`
		Playlists     []string `json:"playlists"`
		DiscoveryMode string   `json:"discovery_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	enabled := make(map[string]bool, len(body.Playlists))
	for _, p := range body.Playlists {
		enabled[p] = true
	}

	updates := map[string]string{
		"LISTENBRAINZ_USER":      body.User,
		"LISTENBRAINZ_USER_TOKEN": body.UserToken,
		"LISTENBRAINZ_DISCOVERY": body.DiscoveryMode,
	}
	for name, def := range defs.PlaylistDefs {
		if enabled[name] {
			updates[def.EnvPrefix+"_SCHEDULE"] = def.DefaultSchedule
			updates[def.EnvPrefix+"_FLAGS"] = def.DefaultFlags
		} else {
			updates[def.EnvPrefix+"_SCHEDULE"] = ""
			updates[def.EnvPrefix+"_FLAGS"] = ""
		}
	}

	if err := s.UpdateEnvKeys(updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for k, v := range updates {
		if v == "" {
			if err := os.Unsetenv(k); err != nil {
				slog.Warn("failed to unset env variable", "key", k, "err", err)
			}
		} else {
			if err := os.Setenv(k, v); err != nil {
				slog.Warn("failed to set env variable", "key", k, "err", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// handleWizardStep2 saves media system configuration.
func (s *Settings) HandleWizardStep2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		System         string `json:"system"`
		URL            string `json:"url"`
		APIKey         string `json:"api_key"`
		LibraryName    string `json:"library_name"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		PlaylistDir    string `json:"playlist_dir"`
		Sleep          string `json:"sleep"`
		AdminAPIKey	   string `json:"admin_api_key"`
		AdminSystemUsername string `json:"admin_system_username"`
		AdminSystemPassword string `json:"admin_system_password"`

		PublicPlaylist bool   `json:"public_playlist"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if body.System == "" {
		http.Error(w, "system is required", http.StatusBadRequest)
		return
	}

	publicPlaylist := ""
	if body.PublicPlaylist {
		publicPlaylist = "true"
	}
	updates := map[string]string{
		"EXPLO_SYSTEM":    body.System,
		"SYSTEM_URL":      body.URL,
		"API_KEY":         body.APIKey,
		"LIBRARY_NAME":    body.LibraryName,
		"SYSTEM_USERNAME": body.Username,
		"SYSTEM_PASSWORD": body.Password,
		"PLAYLIST_DIR":    body.PlaylistDir,
		"SLEEP":           body.Sleep,
		"PUBLIC_PLAYLIST": publicPlaylist,
		"ADMIN_SYSTEM_USERNAME": body.AdminSystemUsername,
		"ADMIN_SYSTEM_PASSWORD": body.AdminSystemPassword,
		"ADMIN_SYSTEM_APIKEY": body.AdminAPIKey,
	}

	if err := s.UpdateEnvKeys(updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleWizardStep3 saves downloader configuration.
func (s *Settings) HandleWizardStep3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DownloadDir      string   `json:"download_dir"`
		UseSubdirectory  bool     `json:"use_subdirectory"`
		MigrateDownloads bool     `json:"migrate_downloads"`
		DownloadServices []string `json:"download_services"`
		YoutubeAPIKey    string   `json:"youtube_api_key"`
		TrackExtension   string   `json:"track_extension"` // yt-dlp
		FilterList       string   `json:"filter_list"`
		SlskdURL         string   `json:"slskd_url"`
		SlskdAPIKey      string   `json:"slskd_api_key"`
		Extensions       string   `json:"extensions"` // slskd
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.DownloadServices) == 0 {
		http.Error(w, "at least one download service is required", http.StatusBadRequest)
		return
	}
	joined := strings.Join(body.DownloadServices, ",")

	useSubdir := "false"
	if body.UseSubdirectory {
		useSubdir = "true"
	}
	migrateDL := "false"
	if body.MigrateDownloads {
		migrateDL = "true"
	}
	updates := map[string]string{
		"DOWNLOAD_DIR":      body.DownloadDir,
		"USE_SUBDIRECTORY":  useSubdir,
		"MIGRATE_DOWNLOADS": migrateDL,
		"DOWNLOAD_SERVICES": joined,
		"YOUTUBE_API_KEY":   body.YoutubeAPIKey,
		"TRACK_EXTENSION":   body.TrackExtension, // yt-dlp
		"FILTER_LIST":       body.FilterList,
		"SLSKD_URL":         body.SlskdURL,
		"SLSKD_API_KEY":     body.SlskdAPIKey,
		"EXTENSIONS":        body.Extensions, // slskd
		"WIZARD_COMPLETE":   "true",
	}

	if err := s.UpdateEnvKeys(updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleSetupStatus returns {"wizard_complete": bool} for first time setups. Public — no auth required.
func (s *Settings) HandleSetupStatus(w http.ResponseWriter, r *http.Request) {
	wizardComplete := false
	if data, err := os.ReadFile(s.cfg.WebEnvPath); err == nil {
		wizardComplete = s.ParseEnvText(string(data))["WIZARD_COMPLETE"] == "true"
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"wizard_complete": wizardComplete}); err != nil {
		slog.Error("failed encoding setup status", "err", err.Error())
	}
}

// handlePathTemplates handles GET and POST for /api/ui/path-templates.
func (s *Settings) HandlePathTemplates(w http.ResponseWriter, r *http.Request) {
	cfgDir := s.cfg.WebDataDir
	switch r.Method {
	case http.MethodGet:
		presets := loadPathTemplates(cfgDir)
		if presets == nil {
			presets = []PathTemplatePreset{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(presets); err != nil {
			slog.Error("failed encoding path templates", "err", err.Error())
		}
	case http.MethodPost:
		var body PathTemplatePreset
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.Name == "" || body.Template == "" {
			http.Error(w, "name and template are required", http.StatusBadRequest)
			return
		}
		presets := loadPathTemplates(cfgDir)
		presets = append(presets, body)
		if err := savePathTemplates(cfgDir, presets); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.Error("failed encoding path template", "err", err.Error())
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeletePathTemplate handles DELETE /api/ui/path-templates/{name}.
func (s *Settings) HandleDeletePathTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/ui/path-templates/")
	name, err := url.PathUnescape(raw)
	if err != nil || name == "" {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	cfgDir := s.cfg.WebDataDir
	presets := loadPathTemplates(cfgDir)
	filtered := presets[:0]
	for _, p := range presets {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(presets) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := savePathTemplates(cfgDir, filtered); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}