package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// appleServerData mirrors the top-level shape of the
// <script id="serialized-server-data"> JSON blob on Apple Music pages.
type appleServerData struct {
	Data []struct {
		Data struct {
			Sections []appleSection `json:"sections"`
		} `json:"data"`
	} `json:"data"`
}

type appleSection struct {
	Items []appleItem `json:"items"`
}

type appleItem struct {
	Title         string `json:"title"`
	ArtistName    string `json:"artistName"`
	TertiaryLinks []struct {
		Title string `json:"title"` // album name
	} `json:"tertiaryLinks"`
	Artwork *appleArtwork `json:"artwork"`
}

type appleArtwork struct {
	Dictionary struct {
		URL string `json:"url"` // template with {w}, {h}, {f} placeholders
	} `json:"dictionary"`
}

// resolveArtworkURL replaces Apple's {w}x{h}bb.{f} template placeholders
// with concrete values for a 300x300 JPEG.
func resolveArtworkURL(tpl string) string {
	if tpl == "" {
		return ""
	}
	r := strings.NewReplacer("{w}", "300", "{h}", "300", "{f}", "jpg")
	return r.Replace(tpl)
}

// fetchAppleMusicPlaylist scrapes a public Apple Music playlist page and extracts
// track info from the embedded server data.
// Returns (playlistName, artworkURL, tracks, error) where tracks are [title, artist, album, coverURL].
func fetchAppleMusicPlaylist(pageURL string) (string, string, []PlaylistTrack, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("apple music: invalid URL: %w", err)
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", nil, fmt.Errorf("apple music: fetch failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("apple music: response body close failed", "err", cerr.Error())
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("apple music: page returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", nil, fmt.Errorf("apple music: failed to read body: %w", err)
	}

	htmlStr := string(body)

	// Parse the serialized-server-data for everything: playlist name, artwork, tracks.
	playlistName, artworkURL, tracks, err := extractServerData(htmlStr)
	if err != nil {
		return "", "", nil, err
	}

	// Fall back to JSON-LD for the playlist name if the server data header was missing.
	if playlistName == "" {
		playlistName = extractPlaylistNameFromJSONLD(htmlStr)
	}
	if playlistName == "" {
		playlistName = "Apple Music Playlist"
	}

	slog.Info("apple music: parsed playlist", "name", playlistName, "tracks", len(tracks), "artwork", artworkURL != "")
	return playlistName, artworkURL, tracks, nil
}

// extractServerData parses the <script id="serialized-server-data"> blob for
// the playlist name, playlist artwork URL (from the header section), and tracks with artwork.
func extractServerData(htmlStr string) (string, string, []PlaylistTrack, error) {
	scripts := extractScriptByID(htmlStr, "serialized-server-data")
	if len(scripts) == 0 {
		return "", "", nil, fmt.Errorf("apple music: no serialized-server-data found in page")
	}

	var ssd appleServerData
	if err := json.Unmarshal([]byte(scripts[0]), &ssd); err != nil {
		return "", "", nil, fmt.Errorf("apple music: failed to parse server data: %w", err)
	}

	var playlistName string
	var artworkURL string
	var tracks []PlaylistTrack

	for _, outer := range ssd.Data {
		for _, sec := range outer.Data.Sections {
			if len(sec.Items) == 0 {
				continue
			}

			// The header section has the playlist title but no artistName.
			// The tracks section has artistName on every item.
			if sec.Items[0].ArtistName == "" {
				// Header section — grab the playlist name and artwork.
				if playlistName == "" && sec.Items[0].Title != "" {
					playlistName = sec.Items[0].Title
				}
				if artworkURL == "" && sec.Items[0].Artwork != nil {
					tpl := sec.Items[0].Artwork.Dictionary.URL
					if tpl != "" {
						// Use a larger size for the playlist cover (card background).
						r := strings.NewReplacer("{w}", "600", "{h}", "600", "{f}", "jpg")
						artworkURL = r.Replace(tpl)
					}
				}
				continue
			}

			// Track section.
			tracks = make([]PlaylistTrack, 0, len(sec.Items))
			for _, item := range sec.Items {
				album := ""
				if len(item.TertiaryLinks) > 0 {
					album = item.TertiaryLinks[0].Title
				}
				coverURL := ""
				if item.Artwork != nil {
					coverURL = resolveArtworkURL(item.Artwork.Dictionary.URL)
				}
				tracks = append(tracks, PlaylistTrack{
					Title:      item.Title,
					Artist:     item.ArtistName,
					MainArtist: item.ArtistName,
					Album:      album,
					CoverURL:   coverURL,
				})
			}
		}
	}

	if len(tracks) == 0 {
		return "", "", nil, fmt.Errorf("apple music: no track data found in server data")
	}
	return playlistName, artworkURL, tracks, nil
}

// extractPlaylistNameFromJSONLD pulls the playlist title from the JSON-LD MusicPlaylist block.
func extractPlaylistNameFromJSONLD(htmlStr string) string {
	for _, raw := range extractScriptByType(htmlStr, "application/ld+json") {
		var pl struct {
			Type string `json:"@type"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(raw), &pl); err == nil && pl.Type == "MusicPlaylist" {
			return pl.Name
		}
	}
	return ""
}

// extractScriptByType finds all <script type="X"> contents in the HTML.
func extractScriptByType(htmlBody, scriptType string) []string {
	var results []string
	tok := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken {
			continue
		}
		tn, hasAttr := tok.TagName()
		if string(tn) != "script" || !hasAttr {
			continue
		}
		match := false
		for {
			key, val, more := tok.TagAttr()
			if string(key) == "type" && string(val) == scriptType {
				match = true
			}
			if !more {
				break
			}
		}
		if match {
			tok.Next()
			results = append(results, string(tok.Text()))
		}
	}
	return results
}

// extractScriptByID finds <script id="X"> contents in the HTML.
func extractScriptByID(htmlBody, id string) []string {
	var results []string
	tok := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken {
			continue
		}
		tn, hasAttr := tok.TagName()
		if string(tn) != "script" || !hasAttr {
			continue
		}
		match := false
		for {
			key, val, more := tok.TagAttr()
			if string(key) == "id" && string(val) == id {
				match = true
			}
			if !more {
				break
			}
		}
		if match {
			tok.Next()
			results = append(results, string(tok.Text()))
		}
	}
	return results
}
