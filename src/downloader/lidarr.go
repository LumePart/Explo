package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"

	"github.com/devopsarr/lidarr-go/lidarr"
)

type Lidarr struct {
	DownloadDir string
	HttpClient  *util.HttpClient
	Client      *lidarr.APIClient
}

func NewLidarr(cfg cfg.Lidarr, discovery, downloadDir string, httpClient *util.HttpClient) *Lidarr {
	// Create Lidarr SDK config
	apiCfg := lidarr.NewConfiguration()
	apiCfg.Host = cfg.URL
	apiCfg.Scheme = cfg.Scheme
	apiCfg.DefaultHeader["X-Api-Key"] = cfg.APIKey
	apiCfg.HTTPClient = httpClient.Client

	client := lidarr.NewAPIClient(apiCfg)

	l := &Lidarr{
		DownloadDir: downloadDir,
		HttpClient:  httpClient,
		Client:      client,
	}
	ctx := context.Background()
	l.startCleanupWorker(ctx)

	return l
}

func (c *Lidarr) QueryTrack(track *models.Track) error {
	ctx := context.Background()

	query := fmt.Sprintf("%s %s", track.MainArtist, track.Album)
	albums, _ := c.albumLookup(ctx, query)

	if len(albums) == 0 || len(albums[0].Releases) == 0 {
		return fmt.Errorf("could not find album for track: %s - %s", track.Title, track.MainArtist)
	}

	var err error
	if albums[0].Id == nil || albums[0].ArtistId == nil {
		return fmt.Errorf("album or artist ID was nil for track: %s - %s", track.Title, track.MainArtist)
	}
	track.Present, err = c.checkExistingTrack(ctx, *albums[0].Id, *albums[0].ArtistId, track)
	if err != nil {
		return fmt.Errorf("failed to check existing tracks: %w", err)
	}

	return nil
}

func (c *Lidarr) GetTrack(track *models.Track) error {
	ctx := context.Background()

	if track.Present {
		return nil
	}
	// Get the defaults from the root dir
	rootFolders, _, err := c.Client.RootFolderAPI.ListRootFolder(ctx).Execute()
	if err != nil || len(rootFolders) == 0 {
		return fmt.Errorf("failed to get root folders: %w", err)
	}
	root := rootFolders[0]

	artist, err := c.findArtist(ctx, track.MainArtist)
	if err != nil {
		return err
	}
	if err := c.addArtistIfNeeded(ctx, artist, root); err != nil {
		return err
	}

	album, err := c.findAlbum(ctx, track.Album)
	if err != nil {
		return err
	}

	chosen, err := c.findReleases(ctx, *album.Id, *album.ArtistId)
	if err != nil {
		return err
	}

	// Start download
	release, err := c.createRelease(ctx, chosen)
	if err != nil {
		return err
	}

	track.Present, err = c.checkExistingTrack(ctx, *release.AlbumId.Get(), *release.ArtistId.Get(), track)
	if err != nil {
		return err
	}

	return nil
}

func (c *Lidarr) checkExistingTrack(ctx context.Context, albumID, artistID int32, track *models.Track) (bool, error) {
	log.Print("checking for existing tracks")
	tracks, _, err := c.Client.TrackAPI.ListTrack(ctx).
		AlbumId(albumID).
		ArtistId(artistID).
		Execute()
	if err != nil {
		return false, fmt.Errorf("failed to get album tracks: %w", err)
	}

	for _, t := range tracks {
		if t.Title.IsSet() && t.Title.Get() != nil && *t.Title.Get() == track.Title {
			log.Printf("Track already downloaded: %s", *t.Title.Get())
			return true, nil
		}
	}

	return false, nil
}

func (c *Lidarr) findArtist(ctx context.Context, name string) (*lidarr.ArtistResource, error) {
	// Lookup Artist
	log.Printf("Finding artist: %s", name)
	resp, err := c.Client.ArtistLookupAPI.GetArtistLookup(ctx).
		Term(name).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("Lidarr artist ID lookup failed with error: %w", err)
	}
	var artists []lidarr.ArtistResource
	if err := json.NewDecoder(resp.Body).Decode(&artists); err != nil {
		return nil, fmt.Errorf("failed to decode Lidarr response: %w", err)
	}
	if len(artists) == 0 {
		return nil, fmt.Errorf("no artist found for: %s", name)
	}

	return &artists[0], nil
}

func (c *Lidarr) addArtistIfNeeded(ctx context.Context, artist *lidarr.ArtistResource, root lidarr.RootFolderResource) error {
	// Ensure we aren't adding an artist that already exists
	if artist.Path.IsSet() && artist.Added != nil && !artist.Added.IsZero() {
		log.Printf("Skipping adding already added artist: %v", *artist.ArtistName.Get())
		return nil
	}

	a := lidarr.NewArtistResourceWithDefaults()
	a.ArtistName = artist.ArtistName
	a.ForeignArtistId = artist.ForeignArtistId
	a.RootFolderPath = root.Path
	a.MetadataProfileId = root.DefaultMetadataProfileId
	a.QualityProfileId = root.DefaultQualityProfileId

	_, httpResp, err := c.Client.ArtistAPI.CreateArtist(ctx).
		ArtistResource(*a).
		Execute()
	if err != nil && (httpResp == nil || httpResp.StatusCode != 400) {
		return fmt.Errorf("failed to create artist: %w", err)
	}
	return nil
}

func (c *Lidarr) albumLookup(ctx context.Context, query string) ([]lidarr.AlbumResource, error) {
	resp, err := c.Client.AlbumLookupAPI.GetAlbumLookup(ctx).
		Term(query).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("Lidarr album lookup error: %w", err)
	}

	var albums []lidarr.AlbumResource
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, fmt.Errorf("failed to decode Lidarr response: %w", err)
	}
	return albums, nil
}

func (c *Lidarr) findAlbum(ctx context.Context, albumName string) (*lidarr.AlbumResource, error) {
	log.Print("Finding album: ", albumName)
	albums, _ := c.albumLookup(ctx, albumName)

	for _, album := range albums {
		// Skip if the album has been searched recently
		if album.LastSearchTime.IsSet() {
			if time.Since(*album.LastSearchTime.Get()) < time.Hour {
				log.Printf("Skipping recently searched album: %s", *album.Title.Get())
				continue
			}
		}

		// Return the first album that's not recently searched
		if album.Id != nil && album.ArtistId != nil {
			return &album, nil
		}
	}

	return nil, fmt.Errorf("no new album found for: %s", albumName)
}

func (c *Lidarr) findReleases(ctx context.Context, albumID, artistID int32) (*lidarr.ReleaseResource, error) {
	log.Print("Finding release")
	releases, _, _ := c.Client.ReleaseAPI.ListRelease(ctx).
		AlbumId(albumID).
		ArtistId(artistID).
		Execute()
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for album ID %d", albumID)
	}

	var chosen lidarr.ReleaseResource
	found := false

	// Ensure release isn't rejected
	for i := range releases {
		if releases[i].Rejected != nil && *releases[i].Rejected {
			continue
		}
		chosen = releases[i]
		found = true
		break
	}

	if !found {
		return nil, fmt.Errorf("no valid releases found")
	}

	chosen.Protocol = nil

	return &chosen, nil
}

func (c *Lidarr) createRelease(ctx context.Context, chosen *lidarr.ReleaseResource) (*lidarr.ReleaseResource, error) {
	log.Print("Starting download")
	release, _, err := c.Client.ReleaseAPI.CreateRelease(ctx).
		ReleaseResource(*chosen).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create release: %w", err)
	}
	body, _ := json.MarshalIndent(release, "", "  ")
	log.Println("Release created", string(body))
	return release, nil
}

func (c *Lidarr) startCleanupWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Cleanup worker stopped")
				return
			case <-ticker.C:
				if err := c.cleanStaleDownloads(ctx); err != nil {
					log.Printf("Cleanup worker error: %v", err)
				}
			}
		}
	}()
}

func (c *Lidarr) cleanStaleDownloads(ctx context.Context) error {
	queue, _, err := c.Client.QueueAPI.GetQueue(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}
	records := queue.Records
	for _, record := range records {
		// skip invalid or incomplete entries
		if record.Size == nil || record.Sizeleft == nil {
			continue
		}

		// Check if download is older than 15 minutes and has not progressed
		age := time.Since(*record.Added.Get())
		if age > 15*time.Minute && *record.Size == *record.Sizeleft {
			log.Printf("Removing stale download: %s (no progress in %v)", *record.Title.Get(), age)
			_, err := c.Client.QueueAPI.DeleteQueue(ctx, *record.Id).Execute()
			if err != nil {
				log.Printf("Failed to delete record %d from queue: %v", *record.Id, err)
			}
		}
	}
	return nil
}
