package client

import (
	"encoding/json"
	"testing"
)

// BUG-15: playlist name/description were interpolated with fmt into
// hand-built JSON, so a quote, backslash or newline produced invalid
// JSON and the server answered 400.
func TestPlaylistBodies_HostileNamesRoundTrip(t *testing.T) {
	name := `My "Best"\Playlist` + "\nHits"
	descr := "line1\nline2 \"quoted\" \\ backslash"

	songs := json.RawMessage(`["a","b"]`)

	create, err := jfCreatePlaylistBody(name, songs, "user-1", true)
	if err != nil {
		t.Fatalf("jfCreatePlaylistBody: %v", err)
	}
	var decoded struct {
		Name  string          `json:"Name"`
		IDs   json.RawMessage `json:"Ids"`
		User  string          `json:"UserId"`
		Media string          `json:"MediaType"`
	}
	if err := json.Unmarshal(create, &decoded); err != nil {
		t.Fatalf("create body is not valid JSON: %v\n%s", err, create)
	}
	if decoded.Name != name || decoded.User != "user-1" || decoded.Media != "Audio" || string(decoded.IDs) != string(songs) {
		t.Errorf("create body = %s, want name/ids/user/media preserved", create)
	}

	update, err := jfUpdatePlaylistBody("5", name, descr, false)
	if err != nil {
		t.Fatalf("jfUpdatePlaylistBody: %v", err)
	}
	var decodedUpdate struct {
		ID       string   `json:"Id"`
		Name     string   `json:"Name"`
		Overview string   `json:"Overview"`
		Genres   []string `json:"Genres"`
		Tags     []string `json:"Tags"`
	}
	if err := json.Unmarshal(update, &decodedUpdate); err != nil {
		t.Fatalf("update body is not valid JSON: %v\n%s", err, update)
	}
	if decodedUpdate.Name != name || decodedUpdate.Overview != descr || decodedUpdate.Genres == nil || decodedUpdate.Tags == nil {
		t.Errorf("update body = %s, want fields preserved with empty arrays, not null", update)
	}

	emby, err := embyUpdatePlaylistBody("9", name, descr)
	if err != nil {
		t.Fatalf("embyUpdatePlaylistBody: %v", err)
	}
	var decodedEmby struct {
		ID       string `json:"Id"`
		Name     string `json:"Name"`
		Overview string `json:"Overview"`
	}
	if err := json.Unmarshal(emby, &decodedEmby); err != nil {
		t.Fatalf("emby body is not valid JSON: %v\n%s", err, emby)
	}
	if decodedEmby.Name != name || decodedEmby.Overview != descr {
		t.Errorf("emby body = %s, want name/overview preserved", emby)
	}
}
