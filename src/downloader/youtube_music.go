package downloader

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type YoutubeMusic struct {
	*Youtube
	Cfg         cfg.YoutubeMusic
}

type PythonSearchResult struct {
	VideoID string `json:"videoId"`
	Title   string `json:"title"`
}

func NewYoutubeMusic(ytmusicCfg cfg.YoutubeMusic, discovery, downloadDir string, httpClient *util.HttpClient) *YoutubeMusic {
	yCfg := cfg.Youtube{
		APIKey:     "", // YT Music doesn't need the API key in this scenario
		FfmpegPath: ytmusicCfg.FfmpegPath,
		YtdlpPath:  ytmusicCfg.YtdlpPath,
		Filters:    ytmusicCfg.Filters,
	}
	
	// Reuse NewYoutube to reuse implementation that provides GetTrack etc.
	underlying := NewYoutube(yCfg, discovery, downloadDir, httpClient)
	
	return &YoutubeMusic{
		Youtube:    underlying,
		Cfg:       ytmusicCfg,
	}
}

func (c *YoutubeMusic) QueryTrack(track *models.Track) error {
	query := fmt.Sprintf("%s - %s", track.Title, track.Artist)
	
	log.Printf("Querying YTMusic for track %s - %s", track.Title, track.Artist)

	cmd := exec.Command("python3", "search_ytmusic.py", query, "1")
	
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ytmusicapi subprocess failed: %w", err)
	}
	
	var results []PythonSearchResult
	if err := json.Unmarshal(out, &results); err != nil {
		return fmt.Errorf("failed to parse ytmusicapi JSON: %w", err)
	}
	
	if len(results) == 0 {
		return fmt.Errorf("no YouTube Music track found for: %s - %s", track.Title, track.Artist)
	}
	
	track.ID = results[0].VideoID
	log.Printf("Matched track %s - %s => videoId %s", track.Title, track.Artist, track.ID)
	
	return nil
}
