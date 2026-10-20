package discovery

import (
	"encoding/json"
	"testing"
)

// BUG-22: enrichment always took Media[0].Tracks[0], so every song was
// tagged track 1 of disc 1.
func parseRelease(t *testing.T, raw string) MBRelease {
	t.Helper()
	var rel MBRelease
	if err := json.Unmarshal([]byte(raw), &rel); err != nil {
		t.Fatal(err)
	}
	return rel
}

const twoDiscRelease = `{"media":[
	{"position":1,"format":"CD","track-count":2,"tracks":[
		{"id":"rt-a1","position":1,"number":"1","recording":{"id":"rec-a"}},
		{"id":"rt-a2","position":2,"number":"2","recording":{"id":"rec-b"}}]},
	{"position":2,"format":"CD","track-count":1,"tracks":[
		{"id":"rt-b1","position":1,"number":"1","recording":{"id":"rec-c"}}]}
]}`

func TestFindReleaseTrack_FindsSecondDiscRecording(t *testing.T) {
	rel := parseRelease(t, twoDiscRelease)

	medium, track, found := findReleaseTrack(rel, "rec-c")
	if !found {
		t.Fatal("recording rec-c not found")
	}
	if medium.Position != 2 || track.Position != 1 || track.ID != "rt-b1" {
		t.Errorf("got disc %d track %+v, want disc 2 rt-b1", medium.Position, track)
	}
}

func TestFindReleaseTrack_UnknownRecordingReportsAbsent(t *testing.T) {
	rel := parseRelease(t, twoDiscRelease)

	if _, _, found := findReleaseTrack(rel, "rec-missing"); found {
		t.Error("unknown recording reported as found")
	}
}

func TestFindReleaseTrack_EmptyMedia(t *testing.T) {
	rel := parseRelease(t, `{"media":[]}`)
	if _, _, found := findReleaseTrack(rel, "rec-a"); found {
		t.Error("recording found in empty media")
	}
}
