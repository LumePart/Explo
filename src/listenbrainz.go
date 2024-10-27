package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"explo/debug"
)

type Recommendations struct {
	Payload struct {
		Count       int    `json:"count"`
		Entity      string `json:"entity"`
		LastUpdated int    `json:"last_updated"`
		Mbids       []struct {
			LatestListenedAt time.Time `json:"latest_listened_at"`
			RecordingMbid    string    `json:"recording_mbid"`
			Score            float64   `json:"score"`
		} `json:"mbids"`
		TotalMbidCount int    `json:"total_mbid_count"`
		UserName       string `json:"user_name"`
	} `json:"payload"`
}

type Metadata struct {
	Artist struct {
		ArtistCreditID int `json:"artist_credit_id"`
		Artists        []struct {
			ArtistMbid string `json:"artist_mbid"`
			BeginYear  int    `json:"begin_year"`
			EndYear    int    `json:"end_year,omitempty"`
			JoinPhrase string `json:"join_phrase"`
			Name       string `json:"name"`
		} `json:"artists"`
		Name string `json:"name"`
	} `json:"artist"`
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
			Date       time.Time `json:"date"`
			Identifier string    `json:"identifier"`
			Title      string    `json:"title"`
		} `json:"playlist"`
	} `json:"playlists"`
}

type Exploration struct {
	Playlist struct {
		Annotation string    `json:"annotation"`
		Creator    string    `json:"creator"`
		Date       time.Time `json:"date"`
		Identifier string    `json:"identifier"`
		Title      string    `json:"title"`
		Tracks     []struct {
			Album      string `json:"album"`
			Creator    string `json:"creator"`
			Extension struct {
				HTTPSMusicbrainzOrgDocJspfTrack struct {
					AddedAt            time.Time `json:"added_at"`
					AddedBy            string    `json:"added_by"`
					AdditionalMetadata struct {
						Artists []struct {
							ArtistCreditName string `json:"artist_credit_name"`
							ArtistMbid       string `json:"artist_mbid"`
							JoinPhrase       string `json:"join_phrase"`
						} `json:"artists"`
						CaaID          int64  `json:"caa_id"`
						CaaReleaseMbid string `json:"caa_release_mbid"`
					} `json:"additional_metadata"`
					ArtistIdentifiers []string `json:"artist_identifiers"`
				} `json:"https://musicbrainz.org/doc/jspf#track"`
			} `json:"extension"`
			Identifier []string `json:"identifier"`
			Title      string `json:"title"`
		} `json:"track"`
	} `json:"playlist"`
}

type Track struct {
	Album  string
	SearchArtist string // used for searching in youtube
	MetadataArtist string
	Title  string
}

func getReccs(cfg Listenbrainz) []string {
	var reccs Recommendations
	var mbids []string

	body, err := lbRequest(fmt.Sprintf("cf/recommendation/user/%s/recording", cfg.User))
	if err != nil {
		log.Fatal(err.Error())
	}

	err = json.Unmarshal(body, &reccs)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("failed to unmarshal body: %s", err.Error())
	}

	for _, rec := range reccs.Payload.Mbids {
		mbids = append(mbids, rec.RecordingMbid)
	}

	if mbids == nil {
		log.Fatal("no recommendations found, exiting...")
	}
	return mbids
}

func getTracks(mbids []string, artistSeparator string) []Track {
	var tracks []Track
	var recordings Recordings
	str_mbids := strings.Join(mbids, ",")

	body, err := lbRequest(fmt.Sprintf("metadata/recording/?recording_mbids=%s&inc=release", str_mbids))
	if err != nil {
		log.Fatal(err.Error())
	}

	err = json.Unmarshal(body, &recordings)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("failed to unmarshal body: %s", err.Error())
	}
	for _, recording := range recordings {
		var metadataArtists []string
		for _, artist := range recording.Artist.Artists {
			metadataArtists = append(metadataArtists, artist.Name)
		}

		tracks = append(tracks, Track{
			Album:  recording.Release.Name,
			SearchArtist: recording.Release.AlbumArtistName,
			MetadataArtist: strings.Join(metadataArtists, artistSeparator),
			Title:  recording.Recording.Name,
		})
	}

	return tracks

}

func getWeeklyExploration(cfg Listenbrainz) (string, error) {
	var playlists Playlists

	body, err := lbRequest(fmt.Sprintf("user/%s/playlists/createdfor", cfg.User))
	if err != nil {
		log.Fatal(err.Error())
	}

	err = json.Unmarshal(body, &playlists)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("failed to unmarshal body: %s", err.Error())
	}

	for _, playlist := range playlists.Playlist {

		_, currentWeek := time.Now().Local().ISOWeek()
		_, creationWeek := playlist.Data.Date.ISOWeek()

		if strings.Contains(playlist.Data.Title, "Weekly Exploration") && currentWeek == creationWeek {
			id := strings.Split(playlist.Data.Identifier, "/")
			return id[len(id)-1], nil
		}
	}
	return "", fmt.Errorf("failed to get new exploration playlist, check if ListenBrainz has generated one this week")
}

func parseWeeklyExploration(identifier, artistSeparator string) []Track {
	var tracks []Track
	var exploration Exploration

	body, err := lbRequest(fmt.Sprintf("playlist/%s", identifier))
	if err != nil {
		log.Fatal(err.Error())
	}

	err = json.Unmarshal(body, &exploration)
	if err != nil {
		debug.Debug(fmt.Sprintf("response: %s", body))
		log.Fatalf("failed to unmarshal body: %s", err.Error())
	}

	for _, track := range exploration.Playlist.Tracks {
		var metadataArtists []string
		for _, artist := range track.Extension.HTTPSMusicbrainzOrgDocJspfTrack.AdditionalMetadata.Artists {
			metadataArtists = append(metadataArtists, artist.ArtistCreditName)
		}
		
		tracks = append(tracks, Track{
			Album:  track.Album,
			SearchArtist: track.Creator,
			MetadataArtist: strings.Join(metadataArtists, artistSeparator),
			Title:  track.Title,
		})
	}
	return tracks

}

func lbRequest(path string) ([]byte, error) { // Handle ListenBrainz API requests


	reqURL := fmt.Sprintf("https://api.listenbrainz.org/1/%s", path)
	
	body, err := makeRequest("GET", reqURL, nil, nil)
	
	if err != nil {
		return nil, err
	}
	return body, nil
}