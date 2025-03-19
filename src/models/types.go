package models

// for structs used across the project

type Track struct {
	Album  string
	ID string
	Artist string // All artists as returned by LB
	MainArtist string
	CleanTitle string // Title as returned by LB
	Title  string // Title as built in BuildTracks()
	File   string // File name
	Present bool // is track present in the system or not
}