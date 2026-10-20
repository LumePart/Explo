package client

import (
	"strings"
	"testing"
)

// BUG-13: with SYSTEM_USERNAME unset, CreatePlaylist sent the API key
// as the Jellyfin UserId and creation always failed.
func TestPickUserID(t *testing.T) {
	users := []JFUser{{ID: "1", Name: "Alice"}, {ID: "2", Name: "Bob"}}

	tests := []struct {
		name     string
		users    []JFUser
		username string
		wantID   string
		wantErr  string
	}{
		{"named user, case-insensitive", users, "alice", "1", ""},
		{"named user missing", users, "Carol", "", "Carol"},
		{"no username, single user", users[:1], "", "1", ""},
		{"no username, many users", users, "", "", "SYSTEM_USERNAME"},
		{"no username, no users", nil, "", "", "SYSTEM_USERNAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := pickUserID(tt.users, tt.username)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("pickUserID = %q, %v; want an error mentioning %q", id, err, tt.wantErr)
				}
				return
			}
			if err != nil || id != tt.wantID {
				t.Errorf("pickUserID = %q, %v; want %q, nil", id, err, tt.wantID)
			}
		})
	}
}
