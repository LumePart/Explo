package main

import (
	"explo/debug"
	"fmt"
	"log"
	"os"
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
			log.Printf("failed to write song to file: %s", err.Error())
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

		case "emby":
			ID, _ = getEmbySong(cfg, track)
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

	description := "Created by Explo using recommendations from ListenBrainz" // Description to add to playlists
	

	// Helper func to sleep after refreshing library
	refreshLibrary := func() {
		log.Printf("[%s] Refreshing library...", cfg.System)
		time.Sleep(time.Duration(cfg.Sleep) * time.Minute)
	}

	switch cfg.System {
	case "subsonic":

		if err := subsonicScan(cfg); err != nil {
			return fmt.Errorf("failed to schedule a library scan: %s", err.Error())
		}
		refreshLibrary()

		ID, err := subsonicPlaylist(cfg, tracks)
		if err != nil {
			return fmt.Errorf("failed to create subsonic playlist: %s", err.Error())
		}
		if err := updSubsonicPlaylist(cfg, ID, description); err != nil {
			debug.Debug(fmt.Sprintf("failed to add comment to playlist: %s", err.Error()))
		}
		
		return nil
	
	case "jellyfin":

		if err := refreshJfLibrary(cfg); err != nil {
			return fmt.Errorf("failed to refresh library: %s", err.Error())
		}
		refreshLibrary()

		ID, err := createJfPlaylist(cfg, tracks)
		if err != nil {
			return fmt.Errorf("failed to create playlist: %s", err.Error())
		}
		if err := updateJfPlaylist(cfg, ID, description); err != nil {
			debug.Debug(fmt.Sprintf("failed to add overview to playlist: %s", err.Error()))
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
		refreshLibrary()

		serverID, err := getPlexServer(cfg)
		if err != nil {
			return fmt.Errorf("createPlaylist(): %s", err.Error())
		}
		playlistKey, err := createPlexPlaylist(cfg, serverID)
		if err != nil {
			return fmt.Errorf("createPlaylist(): %s", err.Error())
		}
		addToPlexPlaylist(cfg, playlistKey, serverID, tracks)

		if err := updatePlexPlaylist(cfg, playlistKey, description); err != nil {
			debug.Debug(fmt.Sprintf("failed to add summary to playlist: %s", err.Error()))
		}
		
		return nil

	case "emby":
		if err := refreshEmbyLibrary(cfg); err != nil {
			return fmt.Errorf("failed to refresh library: %s", err.Error())
		}
		refreshLibrary()

		_, err := createEmbyPlaylist(cfg, tracks)
		if err != nil {
			return fmt.Errorf("failed to create playlist: %s", err.Error())
		}
		/* if err := updateEmbyPlaylist(cfg, ID, description); err != nil { Not working in emby
			debug.Debug(fmt.Sprintf("failed to add overview to playlist: %s", err.Error()))
		} */

		return nil
	}
	return fmt.Errorf("unsupported system: %s", cfg.System)
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

		case "emby":
			ID, err := findEmbyPlaylist(cfg)
			if err != nil {
				return err
			}
			if err := deleteEmbyPlaylist(cfg, ID); err != nil {
				return err
			}
			return nil
		}
	return nil
}