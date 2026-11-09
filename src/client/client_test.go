package client

import (
	"testing"

	"explo/src/models"
)

func TestBestMatchArtist(t *testing.T) {
	// Plex results usually have an empty Artist (originalTitle) and the artist in Artists[0]
	sylvan := SearchResult{Title: "Numb", Album: "Free Love", Artists: []string{"Sylvan Esso"}, Duration: 189, Path: "/m/Numb.flac"}
	linkin := SearchResult{Title: "Numb", Album: "Meteora", Artists: []string{"Linkin Park"}, Duration: 187, Path: "/m/LP - Numb.flac"}
	cake := SearchResult{Title: "I Will Survive", Album: "Fashion Nugget", Artists: []string{"CAKE"}, Duration: 310, Path: "/m/I Will Survive.flac"}

	tests := []struct {
		name    string
		track   models.Track
		results []SearchResult
		want    string // expected Artists[0], "" for no match
	}{
		{"same title, other artist", models.Track{CleanTitle: "Numb", MainArtist: "Linkin Park", Album: "Meteora", Duration: 187000}, []SearchResult{sylvan}, ""},
		{"other artist, no album", models.Track{CleanTitle: "Numb", MainArtist: "Linkin Park", Duration: 187000}, []SearchResult{sylvan}, ""},
		{"cover, no duration", models.Track{CleanTitle: "I Will Survive", MainArtist: "Gloria Gaynor", Album: "Love Tracks"}, []SearchResult{cake}, ""},
		{"cover, no album", models.Track{CleanTitle: "I Will Survive", MainArtist: "Gloria Gaynor", Duration: 198000}, []SearchResult{cake}, ""},
		{"no path on either side", models.Track{CleanTitle: "Numb", MainArtist: "Linkin Park"}, []SearchResult{{Title: "Numb", Artists: []string{"Sylvan Esso"}}}, ""},
		{"right artist wins", models.Track{CleanTitle: "Numb", MainArtist: "Linkin Park", Album: "Meteora", Duration: 187000}, []SearchResult{sylvan, linkin}, "Linkin Park"},
		{"right artist, no album", models.Track{CleanTitle: "Numb", MainArtist: "Linkin Park"}, []SearchResult{sylvan, linkin}, "Linkin Park"},
	}
	for _, tt := range tests {
		got, ok := BestMatch(&tt.track, tt.results, 60)
		if tt.want == "" {
			if ok {
				t.Errorf("%s: matched %v with score %d, want no match", tt.name, got.Artists, got.Score)
			}
			continue
		}
		if !ok || got.Artists[0] != tt.want {
			t.Errorf("%s: got %v (ok=%v), want %s", tt.name, got.Artists, ok, tt.want)
		}
	}
}
