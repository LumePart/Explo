package client

import (
	"errors"
	"fmt"
	"os"

	"explo/src/config"
	"explo/src/debug"
	"explo/src/models"
)

type MPD struct {
	Cfg config.ClientConfig
}

func NewMPD(cfg config.ClientConfig) *MPD {
	return &MPD{Cfg: cfg}
}

func (c *MPD) GetLibrary() error {
	return nil
}

func (c *MPD) GetAuth() error {
	return nil
}

func (c *MPD) AddHeader() error {
	return nil
}

func (c *MPD) AddLibrary() error {
	return nil
}

func (c *MPD) SearchSongs(_ []*models.Track) error {
	return nil
}

func (c *MPD) RefreshLibrary() error {
	return nil
}

func (c *MPD) CreatePlaylist(tracks []*models.Track) error {
	f, err := os.OpenFile(c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	for _, track := range tracks {
		fullFile := fmt.Sprintf("%s%s.mp3\n",c.Cfg.DownloadDir, track.File)
		_, err := f.Write([]byte(fullFile))
		if err != nil {
			debug.Debug(fmt.Sprintf("failed to write song to file: %s", err.Error()))
		}
	}
	return nil
}

func (c *MPD) SearchPlaylist() error {
	if _, err := os.Stat(c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u"); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("did not find playlist: %s", c.Cfg.PlaylistName)
	} else {
		c.Cfg.PlaylistID = c.Cfg.PlaylistDir+c.Cfg.PlaylistName+".m3u"
		return nil
	}
}

func (c *MPD) UpdatePlaylist(description string) error {
	return nil
}

func (c *MPD) DeletePlaylist() error {
	if c.Cfg.PlaylistID != "" {
		if err := os.Remove(c.Cfg.PlaylistID); err != nil {
			return fmt.Errorf("failed to delete playlist: %s", err.Error())
		}
		return nil
	}
	return fmt.Errorf("playlist not found")
}