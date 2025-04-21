package discovery

import (
	"explo/src/models"
	cfg "explo/src/config"
	"explo/src/util"
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
	return c.Discovery.QueryTracks()
}