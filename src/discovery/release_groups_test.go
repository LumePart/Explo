package discovery

import (
	"testing"

	"explo/src/models"
)

func groupTrack(artist, album, groupID string) *models.Track {
	return &models.Track{
		MainArtist:                artist,
		Album:                     album,
		MusicBrainzReleaseGroupID: groupID,
	}
}

// Two tracks resolved to the standard edition, one to a deluxe: the odd one
// joins the majority so the album downloads once.
func TestUnifyReleaseGroups_MajorityWins(t *testing.T) {
	tracks := []*models.Track{
		groupTrack("Weezer", "Weezer", "group-standard"),
		groupTrack("Weezer", "Weezer", "group-standard"),
		groupTrack("Weezer", "Weezer", "group-deluxe"),
	}

	UnifyReleaseGroups(tracks)

	for _, track := range tracks {
		if track.MusicBrainzReleaseGroupID != "group-standard" {
			t.Errorf("group ID = %q, want the majority group-standard", track.MusicBrainzReleaseGroupID)
		}
	}
}

// A track enrichment left without an ID still joins its album's consensus
// instead of forcing a second download path.
func TestUnifyReleaseGroups_FillsMissingIDs(t *testing.T) {
	tracks := []*models.Track{
		groupTrack("Weezer", "Weezer", "group-standard"),
		groupTrack("Weezer", "Weezer", "group-standard"),
		groupTrack("Weezer", "Weezer", ""),
	}

	UnifyReleaseGroups(tracks)

	if tracks[2].MusicBrainzReleaseGroupID != "group-standard" {
		t.Errorf("missing group ID = %q, want group-standard", tracks[2].MusicBrainzReleaseGroupID)
	}
}

// No consensus, no force: a tie and an all-empty group stay exactly as they
// were rather than unifying on a possibly wrong identity.
func TestUnifyReleaseGroups_LeavesTiesAlone(t *testing.T) {
	tied := []*models.Track{
		groupTrack("Weezer", "Weezer", "group-a"),
		groupTrack("Weezer", "Weezer", "group-b"),
	}
	UnifyReleaseGroups(tied)
	if tied[0].MusicBrainzReleaseGroupID != "group-a" || tied[1].MusicBrainzReleaseGroupID != "group-b" {
		t.Errorf("tied group changed to %q, %q -- ties must stay untouched",
			tied[0].MusicBrainzReleaseGroupID, tied[1].MusicBrainzReleaseGroupID)
	}

	empty := []*models.Track{
		groupTrack("Weezer", "Weezer", ""),
		groupTrack("Weezer", "Weezer", ""),
	}
	UnifyReleaseGroups(empty)
	if empty[0].MusicBrainzReleaseGroupID != "" || empty[1].MusicBrainzReleaseGroupID != "" {
		t.Error("group with no IDs gained one -- nothing to unify from")
	}
}

// Punctuation must not split an album: "km/h" and "km h" normalize alike,
// matching the slskd album key so both halves agree downstream.
func TestUnifyReleaseGroups_NormalizesPunctuation(t *testing.T) {
	tracks := []*models.Track{
		groupTrack("t.A.T.u.", "200 km/h in the Wrong Lane", "group-a"),
		groupTrack("t.A.T.u.", "200 km h in the Wrong Lane", "group-a"),
		groupTrack("t.A.T.u.", "200 km h in the Wrong Lane", "group-b"),
	}

	UnifyReleaseGroups(tracks)

	for _, track := range tracks {
		if track.MusicBrainzReleaseGroupID != "group-a" {
			t.Errorf("group ID = %q, want group-a -- punctuated variants split the album", track.MusicBrainzReleaseGroupID)
		}
	}
}

// Different albums never merge, however similar the vote counts.
func TestUnifyReleaseGroups_KeepsAlbumsSeparate(t *testing.T) {
	tracks := []*models.Track{
		groupTrack("Weezer", "Blue Album", "group-blue"),
		groupTrack("Weezer", "Green Album", "group-green"),
	}

	UnifyReleaseGroups(tracks)

	if tracks[0].MusicBrainzReleaseGroupID != "group-blue" || tracks[1].MusicBrainzReleaseGroupID != "group-green" {
		t.Error("distinct albums were merged -- grouping leaked across albums")
	}
}
