package discovery

import (
	"encoding/json"
	"testing"
)

// BUG-03: ListenBrainz metadata with an empty artists array panicked on
// recArtists[0], crashing the whole run.
func TestTrackFromRecording_EmptyArtistsDoesNotPanic(t *testing.T) {
	var md Metadata
	raw := `{"artist":{"name":"Solo Credit","artists":[]},"recording":{"name":"Song","length":123},"release":{"name":"Album"}}`
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		t.Fatal(err)
	}

	c := &ListenBrainz{}
	track := c.trackFromRecording("mbid-1", md, false)

	if track.MusicBrainzArtistID != "" {
		t.Errorf("MusicBrainzArtistID = %q, want empty for missing artists", track.MusicBrainzArtistID)
	}
	if track.MainArtist != "Solo Credit" || track.Title != "Song" {
		t.Errorf("track = %+v, want credit-name fallback and title preserved", track)
	}
}

func TestTrackFromRecording_WithArtistsKeepsArtistID(t *testing.T) {
	var md Metadata
	raw := `{"artist":{"name":"A feat. B","artists":[{"artist_mbid":"a1","name":"A"},{"artist_mbid":"b2","name":"B"}]},"recording":{"name":"Song","length":1},"release":{"name":"Album"}}`
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		t.Fatal(err)
	}

	c := &ListenBrainz{}
	track := c.trackFromRecording("mbid-2", md, false)

	if track.MusicBrainzArtistID != "a1" {
		t.Errorf("MusicBrainzArtistID = %q, want a1", track.MusicBrainzArtistID)
	}
	if track.MainArtist != "A" {
		t.Errorf("MainArtist = %q, want A", track.MainArtist)
	}
}
