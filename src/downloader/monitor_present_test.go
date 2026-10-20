package downloader

import (
	"fmt"
	"testing"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
)

// moveStub completes every download immediately; its MoveDownload can be
// made to fail.
type moveStub struct {
	conf    MonitorConfig
	moveErr error
}

func (s *moveStub) GetConf() (MonitorConfig, error)    { return s.conf, nil }
func (s *moveStub) Cleanup(models.Track, string) error { return nil }
func (s *moveStub) MoveDownload(string, string, string, *models.Track) (string, error) {
	if s.moveErr != nil {
		return "", s.moveErr
	}
	return "/library/album", nil
}

func (s *moveStub) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	statuses := make(map[string]FileStatus, len(tracks))
	for _, track := range tracks {
		statuses[track.ID] = FileStatus{ID: "1", Filename: track.File, State: "Completed, Succeeded", PercentComplete: 100, QueueID: "q1"}
	}
	return statuses, nil
}

func runMoveMonitor(t *testing.T, stub *moveStub, track *models.Track) {
	t.Helper()
	client := &DownloadClient{Cfg: &cfg.DownloadConfig{}}
	done := make(chan error, 1)
	go func() { done <- client.MonitorDownloads([]*models.Track{track}, stub) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("MonitorDownloads() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MonitorDownloads never returned")
	}
}

func migrateCfg() MonitorConfig {
	return MonitorConfig{
		CheckInterval:   time.Millisecond,
		StallDuration:   time.Hour,
		MaxDuration:     24 * time.Hour,
		MigrateDownload: true,
		Service:         "lidarr",
	}
}

// BUG-11: the track was marked Present and counted before MoveDownload
// ran, so a failed move still put a song that never arrived into the
// playlist.
func TestMonitorDownloads_FailedMoveDoesNotMarkPresent(t *testing.T) {
	stub := &moveStub{conf: migrateCfg(), moveErr: fmt.Errorf("disk full")}
	track := &models.Track{ID: "track-1", File: "song.flac"}

	runMoveMonitor(t, stub, track)

	if track.Present {
		t.Error("track marked Present even though the move failed; it would enter the playlist without existing")
	}
}

func TestMonitorDownloads_SuccessfulMoveMarksPresent(t *testing.T) {
	stub := &moveStub{conf: migrateCfg()}
	track := &models.Track{ID: "track-1", File: "song.flac"}

	runMoveMonitor(t, stub, track)

	if !track.Present {
		t.Error("track not Present after a successful move")
	}
}
