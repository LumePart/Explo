package util

import (
	"strings"
	"testing"

	"explo/src/models"
)

// BUG-24: tag sets leave some fields empty per format, producing
// "=value" ffmpeg metadata entries for e.g. m4a/wav and ape files.
func fullTrack() models.Track {
	return models.Track{
		File:                        "song.m4a",
		Title:                       "Song",
		Artist:                      "Artist",
		Album:                       "Album",
		AlbumArtist:                 "Album Artist",
		ArtistSort:                  "Artist, A",
		OriginalDate:                "2020-01-01",
		Genres:                      "Rock",
		Media:                       "CD",
		ReleaseType:                 "album",
		ReleaseStatus:               "official",
		MusicBrainzReleaseGroupID:   "rg",
		MusicBrainzAlbumArtistID:    "aa",
		MusicBrainzTrackID:          "t",
		MusicBrainzAlbumID:          "a",
		MusicBrainzReleaseTrackID:   "rt",
		MusicBrainzArtistID:         "ar",
		OriginalYear:                2020,
		TrackNumber:                 3,
		TrackTotal:                  12,
		DiscNumber:                  1,
		DiscTotal:                   2,
		ISRCs:                       []string{"US123"},
	}
}

func TestBuildffmpegMetadata_HasNoEmptyKeys(t *testing.T) {
	for _, file := range []string{"song.m4a", "song.wav", "song.ape", "song.mp3", "song.flac", "song.opus"} {
		track := fullTrack()
		track.File = file
		for _, entry := range BuildffmpegMetadata(track) {
			if strings.HasPrefix(entry, "=") {
				t.Errorf("%s: entry %q has an empty key", file, entry)
			}
			if !strings.Contains(entry, "=") {
				t.Errorf("%s: entry %q is not key=value", file, entry)
			}
		}
	}
}

func TestAddStringTag_SkipsEmptyKey(t *testing.T) {
	if got := addStringTag(nil, "", "value"); len(got) != 0 {
		t.Errorf("addStringTag empty key = %v, want nothing", got)
	}
	if got := addIntTag(nil, "", 3); len(got) != 0 {
		t.Errorf("addIntTag empty key = %v, want nothing", got)
	}
	if got := addStringTag(nil, "title", "Song"); len(got) != 1 {
		t.Errorf("addStringTag normal = %v, want one entry", got)
	}
}
