package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Recommendations struct {
	Payload struct {
		Count          int     `json:"count"`
		Entity         string  `json:"entity"`
		LastUpdated    int     `json:"last_updated"`
		Mbids []struct {
			LatestListenedAt time.Time `json:"latest_listened_at"`
			RecordingMbid    string    `json:"recording_mbid"`
			Score            float64   `json:"score"`
		}`json:"mbids"`
		TotalMbidCount int     `json:"total_mbid_count"`
		UserName       string  `json:"user_name"`
	} `json:"payload"`
}

type Recording struct {
	Length int    `json:"length"`
	Name   string `json:"name"`
	Rels   []any  `json:"rels"`
}

type Metadata struct {
	Recording struct {
		Length int    `json:"length"`
		Name   string `json:"name"`
		Rels   []any  `json:"rels"`
	} `json:"recording"`
	Release struct {
		AlbumArtistName  string `json:"album_artist_name"`
		CaaID            int64  `json:"caa_id"`
		CaaReleaseMbid   string `json:"caa_release_mbid"`
		Mbid             string `json:"mbid"`
		Name             string `json:"name"`
		ReleaseGroupMbid string `json:"release_group_mbid"`
		Year             int    `json:"year"`
	} `json:"release"`
}


type Recordings map[string]Metadata

type Playlists struct {
	Playlist []struct {
		Data struct {
			Date time.Time `json:"date"`
			Identifier string `json:"identifier"`
			Extension struct {
				HTTPSMusicbrainzOrgDocJspfPlaylist struct {
					AdditionalMetadata struct {
						AlgorithmMetadata struct {
							SourcePatch string `json:"source_patch"`
						} `json:"algorithm_metadata"`
					} `json:"additional_metadata"`
				} `json:"https://musicbrainz.org/doc/jspf#playlist"`
			} `json:"extension"`
		} `json:"playlist"`
	} `json:"playlists"`
}

type Exploration struct {
	Playlist struct {
		Annotation string    `json:"annotation"`
		Creator    string    `json:"creator"`
		Date       time.Time `json:"date"`
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
		Tracks      []struct {
			Album     string `json:"album"`
			Creator   string `json:"creator"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
		} `json:"track"`
	} `json:"playlist"`
}

type Track []struct {
	Album string
	Artist string
	Title string
}

func getReccs(cfg Listenbrainz) []string{
	var reccs Recommendations
	var mbids []string

	body := lbRequest(fmt.Sprintf("cf/recommendation/user/%s/recording", cfg.User))
	
	json.Unmarshal(body, &reccs)

	for _, rec := range reccs.Payload.Mbids {
		mbids = append(mbids, rec.RecordingMbid)
	}

	if mbids == nil {
		log.Fatal("no recommendations found, exiting...")
	}
	return mbids
}

func getTracks(mbids []string) Track {
	var tracks Track
	var recordings Recordings 
	str_mbids := strings.Join(mbids, ",")

	body := lbRequest(fmt.Sprintf("metadata/recording/?recording_mbids=%s&inc=release",str_mbids))

	json.Unmarshal(body, &recordings)

	for _, recording := range recordings {
		tracks = append(tracks, struct{Album string; Artist string; Title string}{
			Album: recording.Release.Name,
			Artist: recording.Release.AlbumArtistName,
			Title: recording.Recording.Name,
		})
	}

	return tracks

}

func getWeeklyExploration(cfg Listenbrainz) (string, error) {

	var playlists Playlists

	body := lbRequest(fmt.Sprintf("user/%s/playlists/createdfor",cfg.User))
	
	json.Unmarshal(body, &playlists)

	for _ ,playlist := range playlists.Playlist {

		_, currentWeek := time.Now().Local().ISOWeek()
		_, creationWeek := playlist.Data.Date.ISOWeek()

		if playlist.Data.Extension.HTTPSMusicbrainzOrgDocJspfPlaylist.AdditionalMetadata.AlgorithmMetadata.SourcePatch == "weekly-exploration" && currentWeek == creationWeek {
			exploration := playlist.Data.Identifier
			id := strings.Split(exploration,"/")
			return id[len(id) - 1], nil
		}
	}
	return "", fmt.Errorf("failed to get exploration playlist")
}

func parseWeeklyExploration(identifier string) Track {

	var tracks Track

	var exploration Exploration

	body := lbRequest(fmt.Sprintf("playlist/%s", identifier))

	json.Unmarshal(body, &exploration)

	for _, track := range exploration.Playlist.Tracks {
		tracks = append(tracks, struct{Album string; Artist string; Title string}{
			Album: track.Album,
			Artist: track.Creator,
			Title: track.Title,
		})
	}
	return tracks

}

func lbRequest(path string) []byte { // Handle ListenBrainz API requests

	client := http.Client{}

	reqString := fmt.Sprintf("https://api.listenbrainz.org/1/%s", path)
	req, err := http.NewRequest(http.MethodGet, reqString, nil)
	if err != nil {
		log.Fatalf("Failed to initialize request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to make request: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}
	return body
}