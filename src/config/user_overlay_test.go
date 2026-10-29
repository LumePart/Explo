package config

import (
	"reflect"
	"testing"

	"explo/src/web/backend/users"
)

func sampleUsers() []users.User {
	return []users.User{
		{ID: "alice", Name: "Alice", ListenBrainzUser: "alice-lb", ListenBrainzToken: "tok-a", PlexUser: "alice-plex", Enabled: true},
		{ID: "bob", Name: "Bob", ListenBrainzUser: "bob-lb", ListenBrainzToken: "tok-b", PlexUser: "bob-plex", Enabled: false},
		{ID: "carol", Name: "Carol", ListenBrainzUser: "carol-lb", ListenBrainzToken: "tok-c", PlexUser: "carol-plex", Enabled: true},
	}
}

func baseCfg() Config {
	var cfg Config
	cfg.System = "plex"
	cfg.DiscoveryCfg.Listenbrainz.User = "env-lb"
	cfg.DiscoveryCfg.Listenbrainz.UserToken = "env-tok"
	cfg.DiscoveryCfg.Listenbrainz.SingleArtist = true
	cfg.ClientCfg.Creds.User = "env-plex"
	cfg.ClientCfg.Creds.APIKey = "key"
	cfg.ClientCfg.URL = "http://plex"
	cfg.ClientCfg.ShareWith = []string{"typed"}
	cfg.ClientCfg.OwnerName = "typed-owner"
	return cfg
}

func TestApplyUser_OverlaysOnlyUserFields(t *testing.T) {
	cfg := baseCfg()
	want := baseCfg()
	if err := cfg.ApplyUser("alice", sampleUsers()); err != nil {
		t.Fatal(err)
	}
	if cfg.DiscoveryCfg.Listenbrainz.User != "alice-lb" || cfg.DiscoveryCfg.Listenbrainz.UserToken != "tok-a" {
		t.Errorf("listenbrainz not overlaid: %+v", cfg.DiscoveryCfg.Listenbrainz)
	}
	if cfg.ClientCfg.Creds.User != "alice-plex" || cfg.ClientCfg.OwnerName != "Alice" || cfg.UserID != "alice" {
		t.Errorf("client/user fields wrong: %+v %q", cfg.ClientCfg, cfg.UserID)
	}
	// Revert overlaid fields on a copy; everything else must be unchanged.
	want.DiscoveryCfg.Listenbrainz.User = "alice-lb"
	want.DiscoveryCfg.Listenbrainz.UserToken = "tok-a"
	want.ClientCfg.Creds.User = "alice-plex"
	want.ClientCfg.OwnerName = "Alice"
	want.ClientCfg.ShareWith = cfg.ClientCfg.ShareWith
	want.UserID = "alice"
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("unexpected changes:\n got %+v\nwant %+v", cfg, want)
	}
}

func TestApplyUser_ComputesShareList(t *testing.T) {
	cfg := baseCfg()
	if err := cfg.ApplyUser("alice", sampleUsers()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"carol-plex"}; !reflect.DeepEqual(cfg.ClientCfg.ShareWith, want) {
		t.Errorf("ShareWith = %v, want %v", cfg.ClientCfg.ShareWith, want)
	}
}

func TestApplyUser_UnknownAndDisabledErrorLeaveCfgUntouched(t *testing.T) {
	for _, id := range []string{"nobody", "bob"} {
		cfg := baseCfg()
		if err := cfg.ApplyUser(id, sampleUsers()); err == nil {
			t.Errorf("%s: expected error", id)
		}
		if !reflect.DeepEqual(cfg, baseCfg()) {
			t.Errorf("%s: cfg modified: %+v", id, cfg)
		}
	}
}

func TestApplyUser_ReservedAllRefused(t *testing.T) {
	cfg := baseCfg()
	all := []users.User{{ID: "all", Name: "All", ListenBrainzToken: "t", Enabled: true}}
	if err := cfg.ApplyUser("all", all); err == nil {
		t.Fatal("want error for reserved id")
	}
	if cfg.UserID != "" {
		t.Error("cfg modified")
	}
}
