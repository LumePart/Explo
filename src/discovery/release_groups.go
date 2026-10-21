package discovery

import (
	"log/slog"
	"strings"

	"explo/src/models"
	"explo/src/util"
)

// UnifyReleaseGroups gives every track of an album the same release-group
// ID. Enrichment resolves each track independently, so two songs from one
// album can land on different editions -- and both downloaders key on the
// group ID (Lidarr's library lookup, slskd's album sharing). Split IDs mean
// the same album downloads twice.
//
// Tracks group by artist and album under the same normalization the slskd
// album key uses, so the two can never disagree about what an album is. A
// strict majority wins; ties and groups with no IDs stay untouched rather
// than forcing a possibly wrong identity. The release (not group) ID is
// deliberately left alone: it only feeds file tags, where per-track truth
// beats coherence.
func UnifyReleaseGroups(tracks []*models.Track) {
	groups := make(map[string][]*models.Track)
	var order []string
	for _, track := range tracks {
		if track == nil {
			continue
		}
		artist := strings.ToLower(util.AlnumOnly(track.MainArtist))
		album := strings.ToLower(util.AlnumOnly(track.Album))
		if artist == "" || album == "" {
			continue
		}
		key := artist + "\x00" + album
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], track)
	}

	for _, key := range order {
		group := groups[key]
		votes := make(map[string]int)
		for _, track := range group {
			if track.MusicBrainzReleaseGroupID != "" {
				votes[track.MusicBrainzReleaseGroupID]++
			}
		}
		if len(votes) == 0 {
			continue
		}
		var winner string
		winnerVotes := 0
		tied := false
		for id, n := range votes {
			if n > winnerVotes {
				winner, winnerVotes, tied = id, n, false
			} else if n == winnerVotes {
				tied = true
			}
		}
		if tied {
			continue
		}
		// Strict majority: more than half the group (counting members
		// without an ID, which abstain but still join the consensus).
		if winnerVotes*2 <= len(group) {
			continue
		}
		for _, track := range group {
			if track.MusicBrainzReleaseGroupID != winner {
				slog.Info("unified album to one release group",
					"album", track.Album, "from", track.MusicBrainzReleaseGroupID, "to", winner)
				track.MusicBrainzReleaseGroupID = winner
			}
		}
	}
}
