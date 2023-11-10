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
	Payload Payload `json:"payload"`
}
type Mbids struct {
	LatestListenedAt time.Time `json:"latest_listened_at"`
	RecordingMbid    string    `json:"recording_mbid"`
	Score            float64   `json:"score"`
}
type Payload struct {
	Count          int     `json:"count"`
	Entity         string  `json:"entity"`
	LastUpdated    int     `json:"last_updated"`
	Mbids          []Mbids `json:"mbids"`
	ModelID        string  `json:"model_id"`
	ModelURL       string  `json:"model_url"`
	Offset         int     `json:"offset"`
	TotalMbidCount int     `json:"total_mbid_count"`
	UserName       string  `json:"user_name"`
}

type Recording struct {
	Length int    `json:"length"`
	Name   string `json:"name"`
	Rels   []any  `json:"rels"`
}
type Release struct {
	AlbumArtistName  string `json:"album_artist_name"`
	CaaID            int64  `json:"caa_id"`
	CaaReleaseMbid   string `json:"caa_release_mbid"`
	Mbid             string `json:"mbid"`
	Name             string `json:"name"`
	ReleaseGroupMbid string `json:"release_group_mbid"`
	Year             int    `json:"year"`
}

type Metadata struct {
	Recording Recording `json:"recording"`
	Release Release `json:"release"`
}


type Tracks map[string]Metadata


func getReccs(cfg Listenbrainz) []string{
	var reccs Recommendations
	var mbids []string
	client := http.Client{}

	reqString := fmt.Sprintf("https://api.listenbrainz.org/1/cf/recommendation/user/%s/recording", cfg.User)
	req, err := http.NewRequest(http.MethodGet, reqString, nil)
	if err != nil {
		log.Fatal("Error initiating request")
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Error making request")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error making request")
	}
	json.Unmarshal(body, &reccs)

	for _, rec := range reccs.Payload.Mbids {
		mbids = append(mbids, rec.RecordingMbid)
	}

	if mbids == nil || resp.StatusCode == 204   {
		log.Fatal("no recommendations found, exiting...")
	}
	
	return mbids
}

func getTracks(mbids []string) Tracks {

	var tracks Tracks 
	str_mbids := strings.Join(mbids, ",")
	client := http.Client{}

	reqString := fmt.Sprintf("https://api.listenbrainz.org/1/metadata/recording/?recording_mbids=%s&inc=release",str_mbids)
	req, err := http.NewRequest(http.MethodGet, reqString, nil)
	if err != nil {
		log.Fatal("Error initiating request")
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Error making request")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error making request")
	}
	err = json.Unmarshal(body, &tracks)
	if err != nil {
		log.Fatalf("Error unmarshaling response: %s", err)
	}

	return tracks

}