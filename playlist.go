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
		fullFile := fmt.Sprintf("%s%s.mp3\n",cfg.Youtube.DownloadDir, file)
		_, err := f.Write([]byte(fullFile))
		if err != nil {
			log.Printf("Failed to write song to file: %s", err.Error())
		}
	}
	return nil
}

func (cfg *Config) getPlaylistName() {
	playlistName := "Discover-Weekly"
	
	if cfg.Persist {
		year, week := time.Now().ISOWeek()
		playlistName = fmt.Sprintf("%s-%v-Week%v", playlistName, year, week)
	}
	cfg.PlaylistName = playlistName
}

func createPlaylist(cfg Config, songs []Song, files []string) error {

	if cfg.System == "" {
		return fmt.Errorf("could not get music system")
	}
	
	switch cfg.System {
	case "subsonic":

		if err := subsonicScan(cfg); err != nil {
			return fmt.Errorf("failed to schedule a library scan")
		}
		if err := subsonicPlaylist(cfg, songs, cfg.PlaylistName); err != nil {
			return fmt.Errorf("failed to create subsonic playlist: %w", err)
		}
		return nil
	
	case "jellyfin":

		if err := refreshJfLibrary(cfg); err != nil {
			return fmt.Errorf("failed to refresh library")
		}
		if err := createJfPlaylist(cfg, songs); err != nil {
			return fmt.Errorf("failed to create playlist")
		}
	case "mpd": 

		if err := createM3U(cfg, cfg.PlaylistName, files); err != nil {
			return fmt.Errorf("failed to create M3U playlist: %w", err)
		}
		return nil
	}
	return fmt.Errorf("something very strange happened")
}

func handlePlaylistDeletion(cfg Config) error {
		deleteSongs(cfg.Youtube)
		
		switch cfg.System {
		case "subsonic":
				playlists, err := getDiscoveryPlaylist(cfg)
				if err != nil {
					return err
				}

				if err := delSubsonicPlaylists(playlists, cfg); err != nil {
					return err
			}
			return nil
		case "jellyfin":
			ID, err := findJfPlaylist(cfg)
			if err != nil {
				return err
			}
			if err := deleteJfPlaylist(cfg, ID); err != nil {
				return err
			}
			return nil
		case "mpd":
			if err := os.Remove(cfg.PlaylistDir+cfg.PlaylistName+".m3u"); err != nil {
				return err
			}
		}
	return nil
}