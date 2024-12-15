package main

import (
	"fmt"
	"os"
	"log"
	"time"
)


func createM3U(cfg Config, name string, tracks []Track) error {
	f, err := os.OpenFile(cfg.PlaylistDir+name+".m3u", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	for _, track := range tracks {
		fullFile := fmt.Sprintf("%s%s.mp3\n",cfg.Youtube.DownloadDir, track.File)
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
		playlistName = fmt.Sprintf("%s-%d-Week%d", playlistName, year, week)
	}
	cfg.PlaylistName = playlistName
}

func checkTracks(cfg Config, tracks []Track) []Track { // Returns updated slice with Present status and song ID (if available)
	for i, track := range tracks {
		var ID string
		switch cfg.System {
		case "subsonic":
			ID, _ = searchTrack(cfg, track)

		case "jellyfin":
			ID, _ = getJfSong(cfg, track)

		case "plex":
			ID, _ = searchPlexSong(cfg, track)
		}
		if ID != "" {
			tracks[i].Present = true
			tracks[i].ID = ID
		}
	}
	return tracks
}

func createPlaylist(cfg Config, tracks []Track) error {

	if cfg.System == "" {
		return fmt.Errorf("could not get music system")
	}
	
	switch cfg.System {
	case "subsonic":

		if err := subsonicScan(cfg); err != nil {
			return fmt.Errorf("failed to schedule a library scan: %s", err.Error())
		}
		log.Printf("sleeping for %d minutes, to allow scan to complete..", cfg.Sleep)
		time.Sleep(time.Duration(cfg.Sleep) * time.Minute)

		if err := subsonicPlaylist(cfg, tracks); err != nil {
			return fmt.Errorf("failed to create subsonic playlist: %s", err.Error())
		}
		return nil
	
	case "jellyfin":

		if err := refreshJfLibrary(cfg); err != nil {
			return fmt.Errorf("failed to refresh library: %s", err.Error())
		}
		log.Printf("sleeping for %d minutes, to allow scan to complete..", cfg.Sleep)
		time.Sleep(time.Duration(cfg.Sleep) * time.Minute)

		if err := createJfPlaylist(cfg, tracks); err != nil {
			return fmt.Errorf("failed to create playlist: %s", err.Error())
		}
		return nil

	case "mpd": 

		if err := createM3U(cfg, cfg.PlaylistName, tracks); err != nil {
			return fmt.Errorf("failed to create M3U playlist: %s", err.Error())
		}
		return nil

	case "plex":
		if err := refreshPlexLibrary(cfg); err != nil {
			return fmt.Errorf("createPlaylist(): %s", err.Error())
		}
		log.Printf("sleeping for %d minutes, to allow scan to complete..", cfg.Sleep)
		time.Sleep(time.Duration(cfg.Sleep) * time.Minute)
		ID, err := getPlexServer(cfg)
		if err != nil {
			return fmt.Errorf("createPlaylist(): %s", err.Error())
		}
		key, err := createPlexPlaylist(cfg, ID)
		if err != nil {
			return fmt.Errorf("createPlaylist(): %s", err.Error())
		}
		addToPlexPlaylist(cfg, key, ID, tracks)
		
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
		case "plex":
			key, err := searchPlexPlaylist(cfg)
			if err != nil {
				return err
			}
			if err := deletePlexPlaylist(cfg, key); err != nil {
				return err
			}
		}
	return nil
}