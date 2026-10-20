package downloader

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
)

// errorStub reports one track as errored with a full slskd state string.
type errorStub struct {
	conf MonitorConfig
}

func (s *errorStub) GetConf() (MonitorConfig, error)    { return s.conf, nil }
func (s *errorStub) Cleanup(models.Track, string) error { return nil }
func (s *errorStub) MoveDownload(string, string, string, *models.Track) (string, error) {
	return "/library/album", nil
}

func (s *errorStub) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	statuses := make(map[string]FileStatus, len(tracks))
	for _, track := range tracks {
		// As the real slskd mapping reports it: normalized "Errored"
		// with the original "Completed, Errored" preserved.
		statuses[track.ID] = FileStatus{ID: "1", Filename: track.File, State: "Errored", RawState: "Completed, Errored", PercentComplete: 10, QueueID: "q1"}
	}
	return statuses, nil
}

// The errored line must carry the provider state string: without it the
// log says a download errored but never why.
func TestMonitorDownloads_ErroredLineLogsState(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	stub := &errorStub{conf: MonitorConfig{
		CheckInterval: time.Millisecond,
		StallDuration: time.Hour,
		MaxDuration:   24 * time.Hour,
		Service:       "slskd",
	}}
	track := &models.Track{ID: "track-1", File: "song.flac"}

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

	out := buf.String()
	if !strings.Contains(out, "download errored") {
		t.Fatalf("no errored line logged:\n%s", out)
	}
	if !strings.Contains(out, "Completed, Errored") {
		t.Errorf("errored line lacks the provider state string:\n%s", out)
	}
}
