package models

// for structs used across the project

type Track struct {
	Album        string
	AlbumMBID    string
	ID           string
	Artist       string // All artists as returned by LB
	ArtistMBID   string
	MainArtist   string
	MainArtistID string
	CleanTitle   string // Title as returned by LB
	Title        string // Title as built in listenbrainz.go
	File         string // File name
	Size         int    // File size
	Present      bool   // is track present in the system or not
	Duration     int    // Track duration in milliseconds (not available for every track)
}
