package discovery

import (
	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
	"log/slog"
)

type DiscoverClient struct {
	cfg *cfg.DiscoveryConfig
	Discovery Discovery
}
type Discovery interface {
	QueryTracks() ([]*models.Track, error)
}

func NewDiscoverer(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *DiscoverClient {
	c := &DiscoverClient{cfg: &cfg}

	switch cfg.Discovery {
	case "listenbrainz":
		c.Discovery = NewListenBrainz(cfg, httpClient)
	default:
		return nil
	}
	return c
}

func (c *DiscoverClient) Discover() ([]*models.Track, error) {
	tracks, err := c.Discovery.QueryTracks()
	if err != nil {
		return nil, err
	}

	return c.filterArtists(tracks), nil
}

func (c DiscoverClient) filterArtists(tracks []*models.Track) []*models.Track {
	if len(c.cfg.ArtistBlacklist) == 0 {
		return tracks
	}

	// create map for faster lookup
	blacklist := make(map[string]struct{}, len(c.cfg.ArtistBlacklist))
	for _, artist := range c.cfg.ArtistBlacklist {
		blacklist[artist] = struct{}{}
	}

	filtered := tracks[:0]

	for _, track := range tracks {
		_, blockedName := blacklist[track.MainArtist]
		_, blockedMBID := blacklist[track.MusicBrainzArtistID]

		if blockedName || blockedMBID {
			slog.Debug("filtered out artist",
				"name", track.MainArtist,
				"mbid", track.MusicBrainzArtistID,
			)
			continue
		}

		filtered = append(filtered, track)
	}

	return filtered
}