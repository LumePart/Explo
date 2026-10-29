package playlist

import (
	"fmt"
	"os"
	"path/filepath"

	"explo/src/web/backend/users"
)

// CacheDir returns the playlist cache directory for a user. An empty userID
// yields the legacy single-user directory (dataDir/cache); otherwise
// dataDir/cache/users/<userID>. Ids failing users.IDRe return an error.
func CacheDir(dataDir, userID string) (string, error) {
	if userID == "" {
		return filepath.Join(dataDir, "cache"), nil
	}
	if !users.IDRe.MatchString(userID) {
		return "", fmt.Errorf("invalid user id %q", userID)
	}
	return filepath.Join(dataDir, "cache", "users", userID), nil
}

// RemoveUserCache deletes only the given user's cache directory. A missing
// directory is not an error; an empty or invalid id is.
func RemoveUserCache(dataDir, userID string) error {
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	dir, err := CacheDir(dataDir, userID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
