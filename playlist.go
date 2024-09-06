package main

import (
	"fmt"
	"os"
	"log"
	"time"
)


func createM3U(cfg Config, name string, files []string) error {
	f, err := os.OpenFile(cfg.PlaylistDir+name+".m3u", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	for _, file := range files {
		full_file := fmt.Sprintf("%s%s.mp3\n",cfg.Youtube.DownloadDir, file)
		_, err := f.Write([]byte(full_file))
		if err != nil {
			log.Printf("Failed to write song to file: %s", err.Error())
		}
	}
	return nil
}

func (cfg *Config) getPlaylistName(persist bool) {
	playlistName := "Discover-Weekly"
	if persist {
		year, week := time.Now().ISOWeek()
		playlistName = fmt.Sprintf("%s-%v-Week%v", playlistName, year, week)
	}
	cfg.PlaylistName = playlistName
}

func createPlaylist(cfg Config, songs, files []string, system string) error {

	if system != "subsonic" && system != "mpd" {
		return fmt.Errorf("unsupported music system: %s", system)
	}
	

	if !cfg.Persist {
		if err := handlePlaylistDeletion(cfg, system); err != nil {
			return fmt.Errorf("failed to delete playlist: %w", err)
		}
	}

	switch system {
	case "subsonic":

		scan(cfg.Subsonic)
		if err := subsonicPlaylist(cfg.Subsonic, songs, cfg.PlaylistName); err != nil {
			return fmt.Errorf("failed to create subsonic playlist: %w", err)
		}
		return nil
	
	case "mpd": 

		if err := createM3U(cfg, cfg.PlaylistName, files); err != nil {
			return fmt.Errorf("failed to create M3U playlist: %w", err)
		}
		return nil
	}
	return fmt.Errorf("something very strange happened")
}

func handlePlaylistDeletion(cfg Config, system string) error {
		deleteSongs(cfg.Youtube)
		
		switch system {
		case "subsonic":
				playlists, err := getDiscoveryPlaylist(cfg.Subsonic)
				if err != nil {
					return err
				}

				if err := delSubsonicPlaylists(playlists, cfg.Subsonic); err != nil {
					return err
			}
			return nil
			
		case "mpd":
			// Way to delete .m3u file
		}
	return nil
}