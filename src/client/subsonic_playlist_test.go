package client

import (
	"reflect"
	"testing"

	"explo/src/models"
)

// BUG-12: CreatePlaylist sent every track's ID as songId, including
// tracks never matched in Subsonic (Present=false, discovery ID or
// empty), and the server rejected the whole playlist over one song.
func TestSubsonicSongIDs_SkipsUnmatchedTracks(t *testing.T) {
	tracks := []*models.Track{
		{ID: "song-1", Present: true},
		{ID: "mbid-not-a-song-id", Present: false},
		{ID: "", Present: true},
		{ID: "song-2", Present: true},
	}

	got := subsonicSongIDs(tracks)
	want := []string{"song-1", "song-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subsonicSongIDs = %v, want %v", got, want)
	}
}

func TestSubsonicSongIDs_NoneMatched(t *testing.T) {
	tracks := []*models.Track{{ID: "x"}, {ID: ""}}
	if got := subsonicSongIDs(tracks); len(got) != 0 {
		t.Errorf("subsonicSongIDs = %v, want none", got)
	}
}
