package config

import (
	"errors"
	"testing"
)

func TestResolveSystemEnv(t *testing.T) {
	tests := []struct {
		name       string
		exp        string
		music      string
		initial    string
		wantSystem string
	}{
		{
			name:       "uses EXPLO_SYSTEM when present",
			exp:        "  Plex \n",
			music:      "emby",
			wantSystem: "plex",
		},
		{
			name:       "falls back to MUSIC_SYSTEM_TYPE",
			music:      "  JellyFin\t",
			wantSystem: "jellyfin",
		},
		{
			name:       "keeps existing config system if set",
			exp:        "subsonic",
			music:      "mpd",
			initial:    "  Emby ",
			wantSystem: "emby",
		},
		{
			name:       "empty when none provided",
			wantSystem: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EXPLO_SYSTEM", tt.exp)
			t.Setenv("MUSIC_SYSTEM_TYPE", tt.music)

			cfg := &Config{System: tt.initial}
			cfg.ResolveSystemEnv()

			if cfg.System != tt.wantSystem {
				t.Fatalf("system mismatch: got %q, want %q", cfg.System, tt.wantSystem)
			}
		})
	}
}

func TestTrimEnvValues(t *testing.T) {
	cfg := &Config{
		System:   " plex \n",
		LogLevel: " DEBUG ",
		ClientCfg: ClientConfig{
			URL:         " https://example.com/ \n",
			LibraryName: " Music ",
			Creds: Credentials{
				User:         " user \n",
				Password:     " pass \t",
				Listenbrainz: " lb_token \n",
			},
		},
		DownloadCfg: DownloadConfig{
			DownloadDir: " /data/music \n",
			Services:    []string{" youtube ", " ", "\nslskd\t"},
		},
		NotifyCfg: NotifyConfig{
			Discord: DiscordNotif{
				ChannelIDs: []string{" 123 ", "", " 456 "},
			},
			Http: HttpNotif{
				ReceiverURLs: []string{" https://a ", "   ", "https://b\n"},
			},
		},
	}

	cfg.TrimEnvValues()

	if cfg.System != "plex" {
		t.Fatalf("unexpected system after trim: %q", cfg.System)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Fatalf("unexpected log level after trim: %q", cfg.LogLevel)
	}
	if cfg.ClientCfg.URL != "https://example.com/" {
		t.Fatalf("unexpected URL after trim: %q", cfg.ClientCfg.URL)
	}
	if cfg.ClientCfg.Creds.User != "user" {
		t.Fatalf("unexpected user after trim: %q", cfg.ClientCfg.Creds.User)
	}
	if cfg.ClientCfg.Creds.Password != "pass" {
		t.Fatalf("unexpected password after trim: %q", cfg.ClientCfg.Creds.Password)
	}
	if cfg.ClientCfg.Creds.Listenbrainz != "lb_token" {
		t.Fatalf("unexpected listenbrainz token after trim: %q", cfg.ClientCfg.Creds.Listenbrainz)
	}
	if cfg.DownloadCfg.DownloadDir != "/data/music" {
		t.Fatalf("unexpected download dir after trim: %q", cfg.DownloadCfg.DownloadDir)
	}

	wantServices := []string{"youtube", "slskd"}
	if len(cfg.DownloadCfg.Services) != len(wantServices) {
		t.Fatalf("unexpected services length: got %d, want %d", len(cfg.DownloadCfg.Services), len(wantServices))
	}
	for i := range wantServices {
		if cfg.DownloadCfg.Services[i] != wantServices[i] {
			t.Fatalf("unexpected service at index %d: got %q, want %q", i, cfg.DownloadCfg.Services[i], wantServices[i])
		}
	}

	wantChannelIDs := []string{"123", "456"}
	if len(cfg.NotifyCfg.Discord.ChannelIDs) != len(wantChannelIDs) {
		t.Fatalf("unexpected channel IDs length: got %d, want %d", len(cfg.NotifyCfg.Discord.ChannelIDs), len(wantChannelIDs))
	}
	for i := range wantChannelIDs {
		if cfg.NotifyCfg.Discord.ChannelIDs[i] != wantChannelIDs[i] {
			t.Fatalf("unexpected channel id at index %d: got %q, want %q", i, cfg.NotifyCfg.Discord.ChannelIDs[i], wantChannelIDs[i])
		}
	}

	wantReceiverURLs := []string{"https://a", "https://b"}
	if len(cfg.NotifyCfg.Http.ReceiverURLs) != len(wantReceiverURLs) {
		t.Fatalf("unexpected receiver URLs length: got %d, want %d", len(cfg.NotifyCfg.Http.ReceiverURLs), len(wantReceiverURLs))
	}
	for i := range wantReceiverURLs {
		if cfg.NotifyCfg.Http.ReceiverURLs[i] != wantReceiverURLs[i] {
			t.Fatalf("unexpected receiver URL at index %d: got %q, want %q", i, cfg.NotifyCfg.Http.ReceiverURLs[i], wantReceiverURLs[i])
		}
	}
}

func TestCommonFixesNormalizesSystemAndURL(t *testing.T) {
	t.Setenv("EXPLO_SYSTEM", "")
	t.Setenv("MUSIC_SYSTEM_TYPE", " MPD \n")

	cfg := &Config{
		ClientCfg: ClientConfig{
			URL:         " http://localhost:4533/ \n",
			PlaylistDir: " /playlists ",
		},
		DownloadCfg: DownloadConfig{
			DownloadDir: " /data ",
			Slskd: Slskd{
				SlskdDir: " /slskd ",
			},
			Youtube: Youtube{
				FileExtension: ".opus",
			},
		},
	}

	cfg.CommonFixes()

	if cfg.System != "mpd" {
		t.Fatalf("unexpected system after common fixes: %q", cfg.System)
	}
	if cfg.ClientCfg.URL != "http://localhost:4533" {
		t.Fatalf("unexpected URL after common fixes: %q", cfg.ClientCfg.URL)
	}
	if cfg.ClientCfg.PlaylistDir != "/playlists/" {
		t.Fatalf("unexpected playlist dir after common fixes: %q", cfg.ClientCfg.PlaylistDir)
	}
	if cfg.DownloadCfg.DownloadDir != "/data/" {
		t.Fatalf("unexpected download dir after common fixes: %q", cfg.DownloadCfg.DownloadDir)
	}
	if cfg.DownloadCfg.Slskd.SlskdDir != "/slskd/" {
		t.Fatalf("unexpected slskd dir after common fixes: %q", cfg.DownloadCfg.Slskd.SlskdDir)
	}
	if cfg.DownloadCfg.Youtube.FileExtension != "opus" {
		t.Fatalf("unexpected file extension after common fixes: %q", cfg.DownloadCfg.Youtube.FileExtension)
	}
}

func TestApplyManualEnvFallbackMapping(t *testing.T) {
	t.Setenv("MUSIC_SYSTEM_TYPE", "  plex \n")
	t.Setenv("MUSIC_SYSTEM_URL", " https://music.local/ \n")
	t.Setenv("MUSIC_SYSTEM_TOKEN", "  system_token \n")
	t.Setenv("LISTENBRAINZ_USER", " lb_user \n")
	t.Setenv("LISTENBRAINZ_TOKEN", " lb_token \n")
	t.Setenv("DOWNLOAD_TYPE", " youtube \n")
	t.Setenv("DOWNLOAD_DIR", " /data/downloads \n")

	cfg := &Config{}
	cfg.applyManualEnvFallback()

	if cfg.System != "plex" {
		t.Fatalf("unexpected system from fallback: %q", cfg.System)
	}
	if cfg.ClientCfg.URL != "https://music.local/" {
		t.Fatalf("unexpected url from fallback: %q", cfg.ClientCfg.URL)
	}
	if cfg.ClientCfg.Creds.APIKey != "system_token" {
		t.Fatalf("unexpected api key from fallback: %q", cfg.ClientCfg.Creds.APIKey)
	}
	if cfg.ClientCfg.Creds.Listenbrainz != "lb_token" {
		t.Fatalf("unexpected listenbrainz token from fallback: %q", cfg.ClientCfg.Creds.Listenbrainz)
	}
	if cfg.DiscoveryCfg.Listenbrainz.User != "lb_user" {
		t.Fatalf("unexpected listenbrainz user from fallback: %q", cfg.DiscoveryCfg.Listenbrainz.User)
	}
	if len(cfg.DownloadCfg.Services) != 1 || cfg.DownloadCfg.Services[0] != "youtube" {
		t.Fatalf("unexpected services from fallback: %#v", cfg.DownloadCfg.Services)
	}
	if cfg.DownloadCfg.DownloadDir != "/data/downloads" {
		t.Fatalf("unexpected download dir from fallback: %q", cfg.DownloadCfg.DownloadDir)
	}
}

func TestReadEnvFallsBackOnWrongTypePtr(t *testing.T) {
	originalReadEnv := cleanenvReadEnv
	t.Cleanup(func() {
		cleanenvReadEnv = originalReadEnv
	})

	cleanenvReadEnv = func(any) error {
		return errors.New("wrong type ptr")
	}

	t.Setenv("MUSIC_SYSTEM_TYPE", " subsonic \n")
	t.Setenv("MUSIC_SYSTEM_URL", " http://subsonic.local/ \n")
	t.Setenv("MUSIC_SYSTEM_TOKEN", " token123 \n")
	t.Setenv("LISTENBRAINZ_USER", " lb_user \n")
	t.Setenv("LISTENBRAINZ_TOKEN", " lb_token \n")
	t.Setenv("DOWNLOAD_TYPE", " slskd \n")
	t.Setenv("DOWNLOAD_DIR", " /music/downloads \n")

	cfg := &Config{
		Flags: Flags{
			CfgPath: "__does_not_exist__.env",
		},
	}

	cfg.ReadEnv()

	if cfg.System != "subsonic" {
		t.Fatalf("unexpected system after wrong type ptr fallback: %q", cfg.System)
	}
	if cfg.ClientCfg.URL != "http://subsonic.local" {
		t.Fatalf("unexpected url after wrong type ptr fallback: %q", cfg.ClientCfg.URL)
	}
	if cfg.ClientCfg.Creds.APIKey != "token123" {
		t.Fatalf("unexpected api key after wrong type ptr fallback: %q", cfg.ClientCfg.Creds.APIKey)
	}
	if cfg.ClientCfg.Creds.Listenbrainz != "lb_token" {
		t.Fatalf("unexpected listenbrainz token after wrong type ptr fallback: %q", cfg.ClientCfg.Creds.Listenbrainz)
	}
	if cfg.DiscoveryCfg.Listenbrainz.User != "lb_user" {
		t.Fatalf("unexpected listenbrainz user after wrong type ptr fallback: %q", cfg.DiscoveryCfg.Listenbrainz.User)
	}
	if len(cfg.DownloadCfg.Services) != 1 || cfg.DownloadCfg.Services[0] != "slskd" {
		t.Fatalf("unexpected services after wrong type ptr fallback: %#v", cfg.DownloadCfg.Services)
	}
	if cfg.DownloadCfg.DownloadDir != "/music/downloads/" {
		t.Fatalf("unexpected download dir after wrong type ptr fallback: %q", cfg.DownloadCfg.DownloadDir)
	}
}
