// Package users holds the multi-user model and its on-disk store.
// It must import only the standard library so any package can depend on it.
package users

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// User is one person served by an Explo instance.
type User struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	ListenBrainzUser  string `json:"listenbrainz_user"`
	ListenBrainzToken string `json:"listenbrainz_token"`
	PlexUser          string `json:"plex_user"`
	Enabled           bool   `json:"enabled"`
}

// IDRe is the shape every user ID must match.
var IDRe = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,31}$")

const (
	fileName = "users.json"
	maxIDLen = 32
)

// Slug derives an ID candidate from a display name.
func Slug(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '_' || r == '-':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxIDLen {
		s = strings.TrimRight(s[:maxIDLen], "-")
	}
	return s
}

func path(dataDir string) string { return filepath.Join(dataDir, fileName) }

// Load reads users.json. A missing file yields (nil, nil).
func Load(dataDir string) ([]User, error) {
	data, err := os.ReadFile(path(dataDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []User
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", fileName, err)
	}
	return out, nil
}

// Store is the only writer of users.json.
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore returns a Store rooted at dataDir.
func NewStore(dataDir string) *Store { return &Store{dir: dataDir} }

// Update loads the users, applies fn, and atomically saves the result,
// holding the lock throughout. If fn errors, nothing is saved.
func (s *Store) Update(fn func([]User) ([]User, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := Load(s.dir)
	if err != nil {
		return err
	}
	updated, err := fn(current)
	if err != nil {
		return err
	}
	return s.save(updated)
}

// save writes via a temp file in the same directory plus rename, mode 0600.
func (s *Store) save(list []User) error {
	if list == nil {
		list = []User{}
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "users-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path(s.dir))
}

// Validate checks a user against the rules. When editing is false the ID must
// not already exist in existing.
func Validate(u User, existing []User, editing bool) error {
	if strings.TrimSpace(u.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(u.ListenBrainzUser) == "" {
		return errors.New("listenbrainz user is required")
	}
	if strings.TrimSpace(u.PlexUser) == "" {
		return errors.New("plex user is required")
	}
	if !IDRe.MatchString(u.ID) {
		return fmt.Errorf("invalid user id %q", u.ID)
	}
	if !editing {
		for _, e := range existing {
			if e.ID == u.ID {
				return fmt.Errorf("user id %q already exists", u.ID)
			}
		}
	}
	return nil
}

// Enabled returns the enabled users in their original order.
func Enabled(list []User) []User {
	var out []User
	for _, u := range list {
		if u.Enabled {
			out = append(out, u)
		}
	}
	return out
}
