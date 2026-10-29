package config

import (
	"fmt"

	"explo/src/web/backend/users"
)

// ApplyUser overlays one user's identity onto cfg. It does no disk I/O; the
// caller loads users with users.Load. An unknown or disabled id returns an
// error and leaves cfg untouched.
func (cfg *Config) ApplyUser(id string, all []users.User) error {
	if users.IsReservedID(id) {
		return fmt.Errorf("user id %q is reserved", id)
	}
	var target *users.User
	for i := range all {
		if all[i].ID == id {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("unknown user %q", id)
	}
	if !target.Enabled {
		return fmt.Errorf("user %q is disabled", id)
	}

	// Typed PLAYLIST_SHARE_WITH is deliberately ignored.
	var share []string
	for _, u := range all {
		if u.ID != id && u.Enabled {
			share = append(share, u.PlexUser)
		}
	}

	cfg.DiscoveryCfg.Listenbrainz.User = target.ListenBrainzUser
	cfg.DiscoveryCfg.Listenbrainz.UserToken = target.ListenBrainzToken
	cfg.ClientCfg.Creds.User = target.PlexUser
	cfg.ClientCfg.OwnerName = target.Name
	cfg.ClientCfg.ShareWith = share
	cfg.UserID = target.ID
	return nil
}
