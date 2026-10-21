package discovery

import (
	"testing"

	"explo/src/models"
)

func TestFilterNonAlbums(t *testing.T) {
	tracks := []*models.Track{
		{CleanTitle: "Album Song", MainArtist: "A", ReleaseType: "Album"},
		{CleanTitle: "Hit Single", MainArtist: "B", ReleaseType: "Single"},
		{CleanTitle: "Short EP", MainArtist: "C", ReleaseType: "EP"},
		{CleanTitle: "Lowercase Album", MainArtist: "D", ReleaseType: "album"},
		{CleanTitle: "Unenriched", MainArtist: "E"},
		nil,
	}

	kept := FilterNonAlbums(tracks)

	if len(kept) != 3 {
		t.Fatalf("kept %d tracks, want 3 (two albums and the unenriched one)", len(kept))
	}
	for _, track := range kept {
		switch track.CleanTitle {
		case "Album Song", "Lowercase Album", "Unenriched":
		default:
			t.Errorf("kept %q, want only albums and unknown types", track.CleanTitle)
		}
	}
}

func TestFilterNonAlbums_KeepsEverythingWhenAllAlbums(t *testing.T) {
	tracks := []*models.Track{
		{CleanTitle: "One", ReleaseType: "Album"},
		{CleanTitle: "Two", ReleaseType: "Album"},
	}

	if kept := FilterNonAlbums(tracks); len(kept) != 2 {
		t.Errorf("kept %d tracks, want 2", len(kept))
	}
}
