// place for confs/variables in use by the UI

package backend

// configFields is the single source of truth for the settings this web UI
// currently owns. VisibleWhen / RequiredWhen drive the settings UI; the wizard
// uses bespoke HTML but references the same logical rules.

/* var configFields = []FieldDef{
	// ── Discovery ──────────────────────────────────────────────────
	{
		Key: "LISTENBRAINZ_USER", Label: "ListenBrainz Username",
		Type: "text", Section: "discovery",
		Placeholder: "e.g. musiclover42", Required: true,
	},

	// ── Media System ───────────────────────────────────────────────
	{
		Key: "EXPLO_SYSTEM", Label: "Media System",
		Type: "select", Section: "system", Required: true,
		Options: []Option{
			{Value: "jellyfin", Label: "Jellyfin"},
			{Value: "emby", Label: "Emby"},
			{Value: "plex", Label: "Plex"},
			{Value: "subsonic", Label: "Subsonic"},
			{Value: "mpd", Label: "MPD"},
		},
	},
	{
		Key: "SYSTEM_URL", Label: "Server URL",
		Type: "url", Section: "system",
		Placeholder:  "e.g. http://192.168.1.100:8096",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", In: netSystems},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", In: netSystems},
	},
	{
		Key: "API_KEY", Label: "API Key",
		Type: "text", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
	},
	{
		Key: "LIBRARY_NAME", Label: "Library Name",
		Type: "text", Section: "system",
		Placeholder: "e.g. Music",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
	},
	{
		Key: "SYSTEM_USERNAME", Label: "Username",
		Type: "text", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},
	{
		Key: "SYSTEM_PASSWORD", Label: "Password",
		Type: "password", Section: "system",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},
	{
		Key: "PLAYLIST_DIR", Label: "Playlist Directory",
		Type: "text", Section: "system",
		Hint:         "Explo writes .m3u files here — MPD reads them as playlists.",
		VisibleWhen:  &Condition{Field: "EXPLO_SYSTEM", Eq: "mpd"},
		RequiredWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "mpd"},
	},
	{
		Key: "SLEEP", Label: "Library Scan Wait (minutes)",
		Type: "text", Section: "system",
		Placeholder: "2",
		Hint:        "How long to wait after triggering a library scan before creating playlists.",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", In: apiKeySystems},
	},
	{
		Key: "PUBLIC_PLAYLIST", Label: "Public Playlists",
		Type: "text", Section: "system",
		Hint:        "Set to true to make playlists visible to all users (Subsonic).",
		VisibleWhen: &Condition{Field: "EXPLO_SYSTEM", Eq: "subsonic"},
	},

	// ── Downloader ─────────────────────────────────────────────────
	{
		Key: "DOWNLOAD_DIR", Label: "Download directory",
		Type: "text", Section: "downloader",
		Placeholder: "e.g. /data/ or ./downloads/",
		Required:    true,
	},
	{
		Key: "USE_SUBDIRECTORY", Label: "Use playlist subfolders",
		Type: "text", Section: "downloader",
		Hint: "When enabled, Explo creates a subfolder per playlist inside the download directory.",
	},
	{
		Key: "YOUTUBE_API_KEY", Label: "YouTube API Key",
		Type: "text", Section: "downloader",
		Placeholder:  "AIza…",
		Hint:         "Required when using YouTube. Enable the YouTube Data API v3.",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "youtube"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "youtube"},
	},
	{
		Key: "SLSKD_URL", Label: "Slskd URL",
		Type: "url", Section: "downloader",
		Placeholder:  "e.g. http://192.168.1.100:5030",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
	},
	{
		Key: "SLSKD_API_KEY", Label: "Slskd API Key",
		Type: "text", Section: "downloader",
		VisibleWhen:  &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
		RequiredWhen: &Condition{Field: "DOWNLOAD_SERVICES", Contains: "slskd"},
	},
} */

// FieldDef describes a single configurable env var.
// Injected into the page as window.__FIELDS__ for the settings UI to consume.
type FieldDef struct {
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Type         string     `json:"type"`    // text | password | url | select
	Section      string     `json:"section"` // discovery | system | downloader
	Placeholder  string     `json:"placeholder,omitempty"`
	Hint         string     `json:"hint,omitempty"`
	Required     bool       `json:"required,omitempty"`
	Options      []Option   `json:"options,omitempty"`      // for type=select
	VisibleWhen  *Condition `json:"visibleWhen,omitempty"`  // hide field when condition is false
	RequiredWhen *Condition `json:"requiredWhen,omitempty"` // conditionally required
}

/* var netSystems = []string{"jellyfin", "emby", "plex", "subsonic"}
var apiKeySystems = []string{"jellyfin", "emby", "plex"} */

// playlistDef is the single source of truth for a supported playlist type.
// To add a new playlist: append one entry here and add the matching entry in
// PLAYLISTS in the frontend Settings.jsx.
type playlistDef struct {
	EnvPrefix       string // e.g. "WEEKLY_EXPLORATION"
	DefaultSchedule string // cron expression
	DefaultFlags    string // CLI flags for the run
}

var playlistDefs = map[string]playlistDef{
	"weekly-exploration": {"WEEKLY_EXPLORATION", "15 00 * * 2", "--playlist weekly-exploration"},
	"weekly-jams":        {"WEEKLY_JAMS", "30 00 * * 1", "--playlist weekly-jams"},
	"daily-jams":         {"DAILY_JAMS", "15 01 * * *", "--playlist daily-jams"},
	"on-repeat":          {"ON_REPEAT", "0 12 1 * *", "--playlist on-repeat"},
}

// allConfigKeys is the complete set of env keys the web UI reads and writes.
var allConfigKeys = []string{
	"LISTENBRAINZ_USER", "LISTENBRAINZ_DISCOVERY",
	"WEEKLY_EXPLORATION_SCHEDULE", "WEEKLY_EXPLORATION_FLAGS",
	"WEEKLY_JAMS_SCHEDULE", "WEEKLY_JAMS_FLAGS",
	"DAILY_JAMS_SCHEDULE", "DAILY_JAMS_FLAGS",
	"ON_REPEAT_SCHEDULE", "ON_REPEAT_FLAGS",
	"EXPLO_SYSTEM", "SYSTEM_URL", "API_KEY", "LIBRARY_NAME",
	"SYSTEM_USERNAME", "SYSTEM_PASSWORD", "PLAYLIST_DIR", "SLEEP", "PUBLIC_PLAYLIST",
	"DOWNLOAD_DIR", "USE_SUBDIRECTORY", "DOWNLOAD_SUBDIRECTORY_FORMAT",
	"DOWNLOAD_SERVICES", "YOUTUBE_API_KEY", "TRACK_EXTENSION", "FILTER_LIST",
	"SLSKD_URL", "SLSKD_API_KEY",
	"WIZARD_COMPLETE",
}
