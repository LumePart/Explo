package config

import (
	"fmt"
	"strings"

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

	// Typed PLAYLIST_SHARE_WITH is deliberately ignored. The share list is the
	// target's ShareWith IDs resolved to Plex usernames; unknown, disabled,
	// self, no-PlexUser, duplicate and own-name entries are dropped silently.
	byID := make(map[string]users.User, len(all))
	for _, u := range all {
		byID[u.ID] = u
	}
	seen := map[string]bool{strings.ToLower(target.PlexUser): true}
	var share []string
	for _, sid := range target.ShareWith {
		u, ok := byID[sid]
		if !ok || sid == id || !u.Enabled || u.PlexUser == "" {
			continue
		}
		key := strings.ToLower(u.PlexUser)
		if seen[key] {
			continue
		}
		seen[key] = true
		share = append(share, u.PlexUser)
	}

	cfg.DiscoveryCfg.Listenbrainz.User = target.ListenBrainzUser
	cfg.DiscoveryCfg.Listenbrainz.UserToken = target.ListenBrainzToken
	cfg.ClientCfg.Creds.User = target.PlexUser
	cfg.ClientCfg.OwnerName = target.Name
	cfg.ClientCfg.ShareWith = share
	cfg.UserID = target.ID
	return nil
}
