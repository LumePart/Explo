package main

import (
	"explo/src/web/backend/playlist"
	"explo/src/web/backend/users"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// UserRunError reports which users' runs failed, in users.json order.
type UserRunError struct{ Failed []string }

func (e *UserRunError) Error() string {
	return "runs failed for users: " + strings.Join(e.Failed, ", ")
}

// ownerLookup resolves which user owns a custom playlist. It is a variable
// so tests can substitute it.
var ownerLookup = func(dataDir, playlistID string, _ []users.User) string {
	return playlist.OwnerOf(dataDir, playlistID)
}

// stripUserArgs removes every --user <v> / --user=<v> (also single-dash).
func stripUserArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--user" || a == "-user":
			i++ // drop the value too
		case strings.HasPrefix(a, "--user=") || strings.HasPrefix(a, "-user="):
		default:
			out = append(out, a)
		}
	}
	return out
}

// rewriteUserArg replaces any --user occurrences with a single --user <id>.
func rewriteUserArg(args []string, id string) []string {
	return append(stripUserArgs(args), "--user", id)
}

// cacheUserID picks the cache owner for a run. Custom playlist ids are unique
// across users and the UI reads them from the legacy cache/<id>.json path, so
// they use "" there; built-in playlists keep the per-user path.
func cacheUserID(playlistID, userID string) string {
	if strings.HasPrefix(playlistID, "custom-") {
		return ""
	}
	return userID
}

// execSelf re-runs this binary with args, inheriting stdio and env.
func execSelf(args []string) error {
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

// runAllUsers runs once per enabled user (file order) via run. With no users
// configured it runs once in single-user mode.
func runAllUsers(dataDir, playlist string, args []string, run func(args []string) error) error {
	all, err := users.Load(dataDir)
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	if len(all) == 0 {
		if err := run(stripUserArgs(args)); err != nil {
			return fmt.Errorf("single-user run (no users configured): %w", err)
		}
		return nil
	}
	enabled := users.Enabled(all)
	if len(enabled) == 0 {
		slog.Warn("no enabled users, nothing to run")
		return nil
	}
	if strings.HasPrefix(playlist, "custom-") {
		owner := ownerLookup(dataDir, playlist, enabled)
		var own []users.User
		for _, u := range enabled {
			if u.ID == owner {
				own = append(own, u)
			}
		}
		if len(own) == 0 {
			slog.Warn("no enabled owner for custom playlist", "playlist", playlist)
			return nil
		}
		enabled = own
	}

	var failed []string
	for _, u := range enabled {
		if users.IsReservedID(u.ID) {
			// Spawning "--user all" again would recurse forever.
			slog.Error("user id is reserved, refusing to run; rename this user", "user", u.ID)
			failed = append(failed, u.ID)
			continue
		}
		if u.ListenBrainzToken == "" {
			slog.Warn("user has no ListenBrainz token, skipping", "user", u.ID)
			continue
		}
		slog.Info("running for user", "user", u.ID)
		if err := run(rewriteUserArg(args, u.ID)); err != nil {
			slog.Error("run failed for user", "user", u.ID, "err", err.Error())
			failed = append(failed, u.ID)
		}
	}
	if len(failed) > 0 {
		return &UserRunError{Failed: failed}
	}
	return nil
}
