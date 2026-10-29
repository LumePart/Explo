package client

import "strings"

// ShareTitle names a recipient's copy of a playlist after its owner. A blank
// owner leaves the name unchanged.
func ShareTitle(owner, playlistName string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return playlistName
	}
	return owner + "'s " + playlistName
}
