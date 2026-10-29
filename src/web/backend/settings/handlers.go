package settings

import (
	"encoding/json"
	"explo/src/web/backend/users"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"explo/src/util"
	"explo/src/web"
	"explo/src/web/backend/defs"
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

	// Admin credentials are not in AllConfigKeys (they are secrets), so the UI
	// only learns whether they are present, never their values.
	lookup := func(key string) string {
		if v := fileValues[key]; v != "" {
			return v
		}
		return os.Getenv(key)
	}
	adminConfigured := lookup("ADMIN_API_KEY") != "" ||
		(lookup("ADMIN_SYSTEM_USERNAME") != "" && lookup("ADMIN_SYSTEM_PASSWORD") != "")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ConfigResponse{Values: values, Sources: sources, AdminConfigured: adminConfigured}); err != nil {
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
	if err := validateEnvText(string(data)); err != nil {
		http.Error(w, "invalid .env content: "+err.Error(), http.StatusBadRequest)
		return
	}
	envFileMu.Lock()
	err = writeEnvFileAtomic(s.cfg.WebEnvPath, data)
	envFileMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleResetConfig resets all settings and restarts the container.
func (s *Settings) HandleResetConfig(w http.ResponseWriter, r *http.Request) {
	envFileMu.Lock()
	err := writeEnvFileAtomic(s.cfg.WebEnvPath, web.SampleEnv)
	envFileMu.Unlock()
	if err != nil {
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
		ID      string `json:"id"`
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

	if def, ok := defs.PlaylistDefs[body.ID]; ok {
		envPrefix = def.EnvPrefix
		defaultFlags = def.DefaultFlags
	} else if defs.CustomIDRe.MatchString(body.ID) {
		envPrefix = util.CustomEnvPrefix(body.Name)
		defaultFlags = "--playlist " + body.ID
	} else {
		http.Error(w, "unknown playlist name", http.StatusBadRequest)
		return
	}
	var currentFlags string
	if data, err := os.ReadFile(s.cfg.WebEnvPath); err == nil {
		currentFlags = s.ParseEnvText(string(data))[envPrefix+"_FLAGS"]
	}
	mergeFlags := func(base string) string {
		injected := []string{"--replace-playlist=false", "--clean-downloads", localOnlyFlag}
		var extras []string
		for _, f := range injected {
			if strings.Contains(currentFlags, f) {
				extras = append(extras, f)
			}
		}
		if len(extras) == 0 {
			return base
		}
		return base + " " + strings.Join(extras, " ")
	}

	updates := map[string]string{}
	if !body.Enabled {
		// Toggle off — truly disable, regardless of day value carried over from state
		updates[envPrefix+"_SCHEDULE"] = ""
		updates[envPrefix+"_FLAGS"] = ""
	} else if body.Day == -2 {
		// "Never" — keep playlist active for manual runs but remove auto-schedule
		updates[envPrefix+"_SCHEDULE"] = ""
		updates[envPrefix+"_FLAGS"] = mergeFlags(defaultFlags)
	} else {
		dom := "*"
		dow := "*"
		if body.Day == 100 {
			dom = "1"
		} else if body.Day >= 0 {
			dow = fmt.Sprintf("%d", body.Day)
		}
		updates[envPrefix+"_SCHEDULE"] = fmt.Sprintf("%d %d %s * %s", body.Minute, body.Hour, dom, dow)
		updates[envPrefix+"_FLAGS"] = mergeFlags(defaultFlags)
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

// HandleSaveAlbumsOnly writes ALBUMS_ONLY=true/false to the .env file.
func (s *Settings) HandleSaveAlbumsOnly(w http.ResponseWriter, r *http.Request) {
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
	if err := s.UpdateEnvKeys(map[string]string{"ALBUMS_ONLY": val}, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// localOnlyFlag builds the playlist from tracks already in the library instead
// of downloading. It is the same flag the docs suggest putting in *_FLAGS by
// hand; the UI just writes it for you.
const localOnlyFlag = "--download-mode=skip"

// playlistFlagsKey resolves which env var holds a playlist's flags, and what
// those flags are when the file says nothing, for built-in and custom
// playlists alike.
func playlistFlagsKey(id, name string) (flagsKey, defaultFlags string, err error) {
	if def, ok := defs.PlaylistDefs[id]; ok {
		return def.EnvPrefix + "_FLAGS", def.DefaultFlags, nil
	}
	if defs.CustomIDRe.MatchString(id) {
		return util.CustomEnvPrefix(name) + "_FLAGS", "--playlist " + id, nil
	}
	return "", "", fmt.Errorf("unknown playlist name")
}

// setPlaylistFlag adds or removes one flag in a playlist's FLAGS env var,
// leaving every other flag as it found it. Adding a flag that is already there
// and removing one that is not are both no-ops, so a repeated request cannot
// duplicate or mangle anything.
func (s *Settings) setPlaylistFlag(id, name, flag string, enabled bool) error {
	flagsKey, defaultFlags, err := playlistFlagsKey(id, name)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(s.cfg.WebEnvPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	currentFlags := s.ParseEnvText(string(data))[flagsKey]
	if currentFlags == "" {
		currentFlags = defaultFlags
	}

	newFlags := currentFlags
	switch hasFlag := strings.Contains(currentFlags, flag); {
	case enabled && !hasFlag:
		newFlags = strings.TrimSpace(currentFlags + " " + flag)
	case !enabled && hasFlag:
		newFlags = strings.TrimSpace(strings.ReplaceAll(currentFlags, flag, ""))
		for strings.Contains(newFlags, "  ") {
			newFlags = strings.ReplaceAll(newFlags, "  ", " ")
		}
	}

	return s.UpdateEnvKeys(map[string]string{flagsKey: newFlags}, web.SampleEnv)
}

// HandleSaveReplacePlaylist injects or removes --replace-playlist=false from a playlist's FLAGS env var.
func (s *Settings) HandleSaveReplacePlaylist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Replace bool   `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// The flag is the negative: present means "do not replace".
	if err := s.setPlaylistFlag(body.ID, body.Name, "--replace-playlist=false", !body.Replace); err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleSaveLocalOnly injects or removes --download-mode=skip from one
// playlist's FLAGS env var, so a playlist can be built from the library alone
// while the others keep downloading.
func (s *Settings) HandleSaveLocalOnly(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		LocalOnly bool   `json:"local_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.setPlaylistFlag(body.ID, body.Name, localOnlyFlag, body.LocalOnly); err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// statusFor keeps an unrecognised playlist a client error while anything else
// stays a server error.
func statusFor(err error) int {
	if err != nil && err.Error() == "unknown playlist name" {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// HandleSaveCleanDownloads injects or removes --clean-downloads from every playlist's FLAGS env var.
func (s *Settings) HandleSaveCleanDownloads(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	const cleanFlag = "--clean-downloads"

	data, err := os.ReadFile(s.cfg.WebEnvPath)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	current := s.ParseEnvText(string(data))

	updates := map[string]string{}
	for _, def := range defs.PlaylistDefs {
		flagsKey := def.EnvPrefix + "_FLAGS"
		flags := current[flagsKey]
		if flags == "" {
			flags = def.DefaultFlags
		}
		hasFlag := strings.Contains(flags, cleanFlag)
		if body.Enabled && !hasFlag {
			flags = strings.TrimSpace(flags + " " + cleanFlag)
		} else if !body.Enabled && hasFlag {
			flags = strings.TrimSpace(strings.ReplaceAll(flags, cleanFlag, ""))
			for strings.Contains(flags, "  ") {
				flags = strings.ReplaceAll(flags, "  ", " ")
			}
		}
		updates[flagsKey] = flags
	}

	if err := s.UpdateEnvKeys(updates, web.SampleEnv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// hasUsers reports whether multi-user mode is active (users.json has users).
func (s *Settings) hasUsers() (bool, error) {
	list, err := users.Load(s.cfg.WebDataDir)
	if err != nil {
		return false, err
	}

	return len(list) > 0, nil
}

// handleWizardStep1 saves discovery settings (username + enabled playlists with default schedules).
func (s *Settings) HandleWizardStep1(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User          *string  `json:"user"`
		UserToken     *string  `json:"userToken"`
		Playlists     []string `json:"playlists"`
		DiscoveryMode string   `json:"discovery_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	// With users configured, absent user/userToken keys mean "keep the stored value".
	multi, err := s.hasUsers()
	if err != nil {
		http.Error(w, "loading users: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if (body.User == nil && !multi) || (body.User != nil && *body.User == "") {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}

	enabled := make(map[string]bool, len(body.Playlists))
	for _, p := range body.Playlists {
		enabled[p] = true
	}

	updates := map[string]string{
		"LISTENBRAINZ_DISCOVERY": body.DiscoveryMode,
	}
	if body.User != nil {
		updates["LISTENBRAINZ_USER"] = *body.User
	}
	if body.UserToken != nil {
		updates["LISTENBRAINZ_USER_TOKEN"] = *body.UserToken
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
		System              string  `json:"system"`
		URL                 string  `json:"url"`
		APIKey              string  `json:"api_key"`
		LibraryName         string  `json:"library_name"`
		Username            *string `json:"username"`
		Password            string  `json:"password"`
		PlaylistDir         string  `json:"playlist_dir"`
		Sleep               string  `json:"sleep"`
		AdminAPIKey         string  `json:"admin_api_key"`
		AdminSystemUsername string  `json:"admin_system_username"`
		AdminSystemPassword string  `json:"admin_system_password"`

		PublicPlaylist bool `json:"public_playlist"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if body.System == "" {
		http.Error(w, "system is required", http.StatusBadRequest)
		return
	}

	// With users configured, an absent username key means "keep SYSTEM_USERNAME".
	multi, err := s.hasUsers()
	if err != nil {
		http.Error(w, "loading users: "+err.Error(), http.StatusInternalServerError)
		return
	}

	publicPlaylist := ""
	if body.PublicPlaylist {
		publicPlaylist = "true"
	}
	updates := map[string]string{
		"EXPLO_SYSTEM":          body.System,
		"SYSTEM_URL":            body.URL,
		"API_KEY":               body.APIKey,
		"LIBRARY_NAME":          body.LibraryName,
		"SYSTEM_PASSWORD":       body.Password,
		"PLAYLIST_DIR":          body.PlaylistDir,
		"SLEEP":                 body.Sleep,
		"PUBLIC_PLAYLIST":       publicPlaylist,
		"ADMIN_SYSTEM_USERNAME": body.AdminSystemUsername,
		"ADMIN_SYSTEM_PASSWORD": body.AdminSystemPassword,
		"ADMIN_API_KEY":         body.AdminAPIKey,
	}
	switch {
	case body.Username != nil:
		updates["SYSTEM_USERNAME"] = *body.Username
	case !multi:
		updates["SYSTEM_USERNAME"] = ""
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
		DownloadDir       string   `json:"download_dir"`
		UseSubdirectory   bool     `json:"use_subdirectory"`
		SlskdMigrateDL    bool     `json:"slskd_migrate_downloads"`
		DownloadServices  []string `json:"download_services"`
		YoutubeAPIKey     string   `json:"youtube_api_key"`
		TrackExtension    string   `json:"track_extension"` // yt-dlp
		FilterList        string   `json:"filter_list"`
		SlskdURL          string   `json:"slskd_url"`
		SlskdAPIKey       string   `json:"slskd_api_key"`
		SlskdAlbumMode    bool     `json:"slskd_album_mode"`
		LidarrURL         string   `json:"lidarr_url"`
		LidarrAPIKey      string   `json:"lidarr_api_key"`
		LidarrMigrateDL   bool     `json:"lidarr_migrate_downloads"`
		LidarrRootFolder  string   `json:"lidarr_root_folder"`
		Extensions        string   `json:"extensions"` // slskd
		MinBitRate        int      `json:"min_bitrate"`
		MaxBitRate        int      `json:"max_bitrate"`
		SizePreference    string   `json:"size_preference"`
		ReleasePreference string   `json:"release_preference"`
		// A pointer because this one defaults to on: a plain bool cannot tell
		// "the client cleared it" from "the client did not send it", and would
		// quietly disable the feature on every payload that omits the key.
		PreferOriginalRelease *bool `json:"prefer_original_release"`
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
	slskdMigrateDL := "false"
	if body.SlskdMigrateDL {
		slskdMigrateDL = "true"
	}
	lidarrMigrateDL := "false"
	if body.LidarrMigrateDL {
		lidarrMigrateDL = "true"
	}
	albumMode := "false"
	if body.SlskdAlbumMode {
		albumMode = "true"
	}
	// An empty value would blank the key rather than record the default.
	if body.SizePreference == "" {
		body.SizePreference = "none"
	}
	if body.ReleasePreference == "" {
		body.ReleasePreference = "smaller"
	}
	preferOriginal := "true"
	if body.PreferOriginalRelease != nil && !*body.PreferOriginalRelease {
		preferOriginal = "false"
	}
	updates := map[string]string{
		"DOWNLOAD_DIR":             body.DownloadDir,
		"USE_SUBDIRECTORY":         useSubdir,
		"SLSKD_MIGRATE_DOWNLOADS":  slskdMigrateDL,
		"DOWNLOAD_SERVICES":        joined,
		"YOUTUBE_API_KEY":          body.YoutubeAPIKey,
		"TRACK_EXTENSION":          body.TrackExtension, // yt-dlp
		"FILTER_LIST":              body.FilterList,
		"SLSKD_URL":                body.SlskdURL,
		"SLSKD_API_KEY":            body.SlskdAPIKey,
		"SLSKD_ALBUM_MODE":         albumMode,
		"LIDARR_URL":               body.LidarrURL,
		"LIDARR_API_KEY":           body.LidarrAPIKey,
		"LIDARR_MIGRATE_DOWNLOADS": lidarrMigrateDL,
		"LIDARR_ROOT_FOLDER":       body.LidarrRootFolder,
		"EXTENSIONS":               body.Extensions, // slskd
		"MIN_BITRATE":              strconv.Itoa(body.MinBitRate),
		"MAX_BITRATE":              strconv.Itoa(body.MaxBitRate),
		"SIZE_PREFERENCE":          body.SizePreference,
		"RELEASE_PREFERENCE":       body.ReleasePreference,

		"PREFER_ORIGINAL_RELEASE": preferOriginal,

		"WIZARD_COMPLETE": "true",
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
