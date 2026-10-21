package discovery

import (
	"log/slog"
	"strings"

	"explo/src/models"
)

// FilterNonAlbums drops tracks whose MusicBrainz release group is not an
// album -- singles, EPs, broadcasts -- for listeners who only want complete
// albums in the library. A track whose release type is unknown stays: without
// enrichment there is nothing to judge it by, and dropping it would punish
// the unenriched rather than the non-album.
func FilterNonAlbums(tracks []*models.Track) []*models.Track {
	kept := make([]*models.Track, 0, len(tracks))
	for _, track := range tracks {
		if track == nil {
			continue
		}
		if t := strings.TrimSpace(track.ReleaseType); t != "" && !strings.EqualFold(t, "Album") {
			slog.Info("skipping non-album release", "title", track.CleanTitle,
				"artist", track.MainArtist, "type", track.ReleaseType)
			continue
		}
		kept = append(kept, track)
	}
	if dropped := len(tracks) - len(kept); dropped > 0 {
		slog.Info("albums-only filter dropped tracks", "dropped", dropped, "kept", len(kept))
	}
	return kept
}
