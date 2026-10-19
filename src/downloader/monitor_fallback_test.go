package downloader

import (
	"testing"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
)

// fallbackStub always reports progress but never completes, unless
// completeAfter is set, in which case it completes after that many polls.
type fallbackStub struct {
	conf          MonitorConfig
	completeAfter int
	polls         int
	cleanups      int
}

func (s *fallbackStub) GetConf() (MonitorConfig, error) { return s.conf, nil }
func (s *fallbackStub) Cleanup(models.Track, string) error {
	s.cleanups++
	return nil
}
func (s *fallbackStub) MoveDownload(string, string, string, *models.Track) (string, error) {
	return "", nil
}

func (s *fallbackStub) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	s.polls++
	statuses := make(map[string]FileStatus, len(tracks))
	for _, track := range tracks {
		if s.completeAfter > 0 && s.polls > s.completeAfter {
			statuses[track.ID] = FileStatus{ID: "1", Filename: track.File, State: "Completed, Succeeded", PercentComplete: 100, QueueID: "q1"}
			continue
		}
		statuses[track.ID] = FileStatus{ID: "1", Filename: track.File, State: "InProgress", BytesTransferred: s.polls, BytesRemaining: 100, QueueID: "q1"}
	}
	return statuses, nil
}

func runFallbackMonitor(t *testing.T, stub *fallbackStub, track *models.Track) error {
	t.Helper()
	client := &DownloadClient{Cfg: &cfg.DownloadConfig{}}
	done := make(chan error, 1)
	go func() { done <- client.MonitorDownloads([]*models.Track{track}, stub) }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("MonitorDownloads never returned")
		return nil
	}
}

func TestMonitorDownloads_FallbackAfterAbandonsUnresolvedTracks(t *testing.T) {
	stub := &fallbackStub{conf: MonitorConfig{
		CheckInterval: time.Millisecond,
		StallDuration: time.Hour,
		MaxDuration:   24 * time.Hour,
		FallbackAfter: 20 * time.Millisecond,
		Service:       "lidarr",
	}}
	track := &models.Track{ID: "track-1", File: "song.flac"}

	if err := runFallbackMonitor(t, stub, track); err != nil {
		t.Fatalf("MonitorDownloads() = %v, want nil so the next provider can try", err)
	}
	if track.Present {
		t.Error("unfinished track was marked present")
	}
	if stub.cleanups == 0 {
		t.Error("abandoned track was never cleaned up; the provider would keep downloading in the background")
	}
}

func TestMonitorDownloads_FallbackAfterZeroWaitsForCompletion(t *testing.T) {
	stub := &fallbackStub{completeAfter: 3, conf: MonitorConfig{
		CheckInterval: time.Millisecond,
		StallDuration: time.Hour,
		MaxDuration:   24 * time.Hour,
		FallbackAfter: 0,
		Service:       "lidarr",
	}}
	track := &models.Track{ID: "track-1", File: "song.flac"}

	if err := runFallbackMonitor(t, stub, track); err != nil {
		t.Fatalf("MonitorDownloads() = %v, want nil", err)
	}
	if !track.Present {
		t.Error("completed track was not marked present with fallback disabled")
	}
}

// BUG-18: a trickling download moves bytes every poll, so the progress
// branch must not bypass the MaxDuration deadline.
func TestMonitorDownloads_TricklingTrackHitsMaxDuration(t *testing.T) {
	stub := &fallbackStub{conf: MonitorConfig{
		CheckInterval: time.Millisecond,
		StallDuration: time.Hour,
		MaxDuration:   20 * time.Millisecond,
		Service:       "slskd",
	}}
	track := &models.Track{ID: "track-1", File: "song.flac"}

	if err := runFallbackMonitor(t, stub, track); err != nil {
		t.Fatalf("MonitorDownloads() = %v, want nil", err)
	}
	if track.Present {
		t.Error("trickling track was marked present after MaxDuration")
	}
	if stub.cleanups == 0 {
		t.Error("timed-out track was never cleaned up")
	}
}
