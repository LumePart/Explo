package models

// for structs used across the project

type Track struct {
	Album                     string
	AlbumArtist               string
	ID                        string
	Artist                    string // All artists as returned by LB
	MainArtist                string
	MainArtistID              string
	ReleaseCountry            string
	ReleaseStatus             string
	ReleaseType               string
	CleanTitle                string // Title as returned by LB
	Title                     string // Title as built in listenbrainz.go
	File                      string // File name
	Size                      int    // File size
	Present                   bool   // is track present in the system or not
	Duration                  int    // Track duration in milliseconds (not available for every track)
	CoverURL                  string // External cover art URL (Cover Art Archive), used at run-time to download art
	OriginalDate              string
	OriginalYear              int
	Genres                    string
	ISRCs                     []string
	MusicBrainzReleaseGroupID string
	MusicBrainzAlbumArtistID  string
	MusicBrainzTrackID        string
	MusicBrainzAlbumID        string
	MusicBrainzReleaseTrackID string
	MusicBrainzArtistID       string
}
