package discovery

import (
	"explo/src/models"
	cfg "explo/src/config"
)

type DiscoverClient struct {
	cfg *cfg.DiscoveryConfig
	Discovery Discovery
}
type Discovery interface {
	QueryTracks() ([]*models.Track, error)
}

func NewDiscoverer(cfg cfg.DiscoveryConfig) *DiscoverClient {
	c := &DiscoverClient{cfg: &cfg}

	switch cfg.Discovery {
	case "listenbrainz":
		c.Discovery = NewListenBrainz(cfg)
	default:
		return nil
	}
	return c
}

func (c *DiscoverClient) Discover() ([]*models.Track, error) {
	return c.Discovery.QueryTracks()
}