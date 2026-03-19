package discovery

import (
	"fmt"
	"explo/src/models"
	cfg "explo/src/config"
	"explo/src/util"
)

type DiscoverClient struct {
	Config *cfg.DiscoveryConfig
	Discovery Discovery
}
type Discovery interface {
	QueryTracks() ([]*models.Track, error)
}

func NewDiscoverer(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *DiscoverClient {
	c := &DiscoverClient{Config: &cfg}

	switch cfg.Discovery {
	case "listenbrainz":
		c.Discovery = NewListenBrainz(cfg, httpClient)
	default:
		return nil
	}
	return c
}

func (c *DiscoverClient) Discover() ([]*models.Track, error) {
	if c == nil || c.Config == nil || c.Discovery == nil {
		return nil, fmt.Errorf("discovery client not initialized")
	}
	return c.Discovery.QueryTracks()
}