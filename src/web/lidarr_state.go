package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const ratingStateFileVersion = 1
const maxRatingRetries = 3

type RatingStateEntry struct {
	RatedAt        string `json:"rated_at,omitempty"`
	SyncedAt       string `json:"synced_at,omitempty"`
	Artist         string `json:"artist,omitempty"`
	Album          string `json:"album,omitempty"`
	LidarrArtistID int    `json:"lidarr_artist_id,omitempty"`
	LidarrAlbumID  int    `json:"lidarr_album_id,omitempty"`
	Status         string `json:"status,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
}

type ratingStateFile struct {
	Version      int                         `json:"version"`
	WebhookToken string                      `json:"webhook_token,omitempty"`
	Entries      map[string]RatingStateEntry `json:"entries"`
}

// RatingState persists the set of Plex ratingKeys we've already processed (or
// permanently failed on) so that webhook + poll paths don't double-process.
type RatingState struct {
	mu   sync.Mutex
	path string
	data ratingStateFile
}

func NewRatingState(path string) *RatingState {
	return &RatingState{
		path: path,
		data: ratingStateFile{Version: ratingStateFileVersion, Entries: map[string]RatingStateEntry{}},
	}
}

func (s *RatingState) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read rating state: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return fmt.Errorf("parse rating state: %w", err)
	}
	if s.data.Entries == nil {
		s.data.Entries = map[string]RatingStateEntry{}
	}
	return nil
}

// Has reports whether a ratingKey is "done" — either successfully synced or
// permanently failed (retry count exceeded). Returns false for entries that
// should still be retried.
func (s *RatingState) Has(ratingKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.data.Entries[ratingKey]
	if !ok {
		return false
	}
	if entry.Status == "ok" {
		return true
	}
	return entry.RetryCount >= maxRatingRetries
}

func (s *RatingState) Get(ratingKey string) (RatingStateEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data.Entries[ratingKey]
	return entry, ok
}

// Mark records a successful sync. Persists immediately.
func (s *RatingState) Mark(ratingKey string, entry RatingStateEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Entries[ratingKey] = entry
	return s.flushLocked()
}

// IncrementRetry bumps the retry counter for a failed event and persists.
func (s *RatingState) IncrementRetry(ratingKey, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.data.Entries[ratingKey]
	entry.RetryCount++
	entry.Status = "failed:" + reason
	if entry.SyncedAt == "" {
		entry.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.data.Entries[ratingKey] = entry
	return s.flushLocked()
}

// WebhookToken returns the persisted webhook secret, generating it on first use.
func (s *RatingState) WebhookToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.WebhookToken != "" {
		return s.data.WebhookToken, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate webhook token: %w", err)
	}
	s.data.WebhookToken = base64.RawURLEncoding.EncodeToString(buf)
	if err := s.flushLocked(); err != nil {
		return "", err
	}
	return s.data.WebhookToken, nil
}

func (s *RatingState) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	payload, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	// Atomic write via temp-file + rename. Falls back to a direct write when
	// rename fails (e.g. the file is a Docker bind mount, where renaming over
	// the inode returns EBUSY).
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "lidarr_synced.*.tmp")
	if err == nil {
		tmpName := tmp.Name()
		_, writeErr := tmp.Write(payload)
		closeErr := tmp.Close()
		if writeErr == nil && closeErr == nil {
			if err := os.Rename(tmpName, s.path); err == nil {
				return nil
			}
		}
		_ = os.Remove(tmpName)
	}

	return os.WriteFile(s.path, payload, 0o644)
}
