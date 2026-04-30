package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"explo/src/client"
	"explo/src/config"
)

const (
	plexEventRate         = "media.rate"
	plexTrackType         = "track"
	mbidPrefix            = "mbid://"
	musicBrainzGUIDPrefix = "com.plexapp.agents.musicbrainz://"
	albumRefreshTimeout   = 30 * time.Second
	albumRefreshPoll      = 2 * time.Second
)

// PlexWebhookPayload mirrors the JSON Plex POSTs for webhook events.
// Only the fields we use are declared.
type PlexWebhookPayload struct {
	Event   string `json:"event"`
	Account struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	} `json:"Account"`
	Metadata client.PlexTrackMetadata `json:"Metadata"`
}

type ratingEvent struct {
	RatingKey            string
	ArtistName           string
	ArtistMBID           string
	AlbumName            string
	AlbumMBID            string
	GrandparentRatingKey string
	UserRating           float64
	AccountTitle         string
	Source               string // "webhook" | "poll"
}

type LidarrSync struct {
	plex         *client.Plex
	lidarr       *client.Lidarr
	state        *RatingState
	cfg          config.LidarrConfig
	expectedUser string
	libraryName  string
	events       chan ratingEvent
	webhookToken string
}

func NewLidarrSync(cfg config.LidarrConfig, plex *client.Plex, lidarr *client.Lidarr, state *RatingState, clientCfg config.ClientConfig) (*LidarrSync, error) {
	token, err := state.WebhookToken()
	if err != nil {
		return nil, err
	}
	return &LidarrSync{
		plex:         plex,
		lidarr:       lidarr,
		state:        state,
		cfg:          cfg,
		expectedUser: clientCfg.Creds.User,
		libraryName:  clientCfg.LibraryName,
		events:       make(chan ratingEvent, 64),
		webhookToken: token,
	}, nil
}

func (s *LidarrSync) WebhookToken() string { return s.webhookToken }

// Start launches the worker goroutine and (if poll interval > 0) the poll-ticker goroutine.
// Both exit when ctx is canceled.
func (s *LidarrSync) Start(ctx context.Context) {
	go s.worker(ctx)
	if s.cfg.PollInterval > 0 {
		go s.poller(ctx)
	}
}

func (s *LidarrSync) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-s.events:
			if s.state.Has(ev.RatingKey) {
				continue
			}
			if err := s.processEvent(ctx, ev); err != nil {
				slog.Warn("lidarr sync failed", "ratingKey", ev.RatingKey, "artist", ev.ArtistName, "album", ev.AlbumName, "err", err.Error())
				if perr := s.state.IncrementRetry(ev.RatingKey, err.Error()); perr != nil {
					slog.Warn("failed to persist rating state", "err", perr.Error())
				}
			}
		}
	}
}

func (s *LidarrSync) poller(ctx context.Context) {
	// Run once shortly after startup so a fresh container catches up before the first tick.
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		if err := s.pollOnce(); err != nil {
			slog.Warn("initial lidarr poll failed", "err", err.Error())
		}
	}

	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.pollOnce(); err != nil {
				slog.Warn("lidarr poll failed", "err", err.Error())
			}
		}
	}
}

func (s *LidarrSync) pollOnce() error {
	tracks, err := s.plex.GetRatedTracks()
	if err != nil {
		return err
	}
	for _, t := range tracks {
		if t.Type != plexTrackType {
			continue
		}
		if s.state.Has(t.RatingKey) {
			continue
		}
		ev := ratingEvent{
			RatingKey:            t.RatingKey,
			ArtistName:           t.GrandparentTitle,
			ArtistMBID:           extractArtistMBID(t.Guid, t.GrandparentGUID),
			AlbumName:            t.ParentTitle,
			AlbumMBID:            mbidFromGUID(t.ParentGUID),
			GrandparentRatingKey: t.GrandparentRatingKey,
			UserRating:           t.UserRating,
			Source:               "poll",
		}
		s.enqueue(ev)
	}
	return nil
}

// HandleWebhook is the POST /api/plex/webhook handler.
// Plex POSTs multipart/form-data with a single "payload" form field containing JSON.
func (s *LidarrSync) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	got := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.webhookToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		// Plex always sends multipart, but tolerate alternative encodings just in case.
		if perr := r.ParseForm(); perr != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}
	raw := r.FormValue("payload")
	if raw == "" {
		http.Error(w, "missing payload", http.StatusBadRequest)
		return
	}

	var payload PlexWebhookPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)

	if !s.shouldHandle(payload) {
		return
	}

	ev := ratingEvent{
		RatingKey:            payload.Metadata.RatingKey,
		ArtistName:           payload.Metadata.GrandparentTitle,
		ArtistMBID:           extractArtistMBID(payload.Metadata.Guid, payload.Metadata.GrandparentGUID),
		AlbumName:            payload.Metadata.ParentTitle,
		AlbumMBID:            mbidFromGUID(payload.Metadata.ParentGUID),
		GrandparentRatingKey: payload.Metadata.GrandparentRatingKey,
		UserRating:           payload.Metadata.UserRating,
		AccountTitle:         payload.Account.Title,
		Source:               "webhook",
	}
	s.enqueue(ev)
}

func (s *LidarrSync) shouldHandle(p PlexWebhookPayload) bool {
	if p.Event != plexEventRate {
		return false
	}
	if p.Metadata.Type != plexTrackType {
		return false
	}
	if p.Metadata.UserRating <= 0 {
		return false
	}
	if !strings.EqualFold(p.Metadata.LibrarySectionTitle, s.libraryName) {
		slog.Debug("ignoring webhook from non-Explo library", "library", p.Metadata.LibrarySectionTitle)
		return false
	}
	if s.expectedUser != "" && p.Account.Title != "" && !strings.EqualFold(p.Account.Title, s.expectedUser) {
		slog.Debug("ignoring webhook from non-configured user", "user", p.Account.Title)
		return false
	}
	return true
}

func (s *LidarrSync) enqueue(ev ratingEvent) {
	select {
	case s.events <- ev:
	default:
		slog.Warn("lidarr sync queue full, dropping event", "ratingKey", ev.RatingKey)
	}
}

func (s *LidarrSync) processEvent(ctx context.Context, ev ratingEvent) error {
	slog.Info("processing rating", "source", ev.Source, "artist", ev.ArtistName, "album", ev.AlbumName, "rating", ev.UserRating)

	mbid := ev.ArtistMBID
	if mbid == "" && ev.GrandparentRatingKey != "" {
		resolved, err := s.resolveArtistMBID(ev.GrandparentRatingKey)
		if err != nil {
			slog.Debug("failed to resolve artist MBID, will fall back to name lookup", "err", err.Error())
		}
		mbid = resolved
	}

	artistID, err := s.ensureArtist(mbid, ev.ArtistName)
	if err != nil {
		return fmt.Errorf("ensure artist: %w", err)
	}

	if err := s.lidarr.RefreshArtist(artistID); err != nil {
		slog.Debug("RefreshArtist failed (non-fatal)", "err", err.Error())
	}

	album, err := s.findAlbum(ctx, artistID, ev.AlbumName, ev.AlbumMBID)
	if err != nil {
		return fmt.Errorf("find album: %w", err)
	}

	if !album.Monitored {
		if err := s.lidarr.MonitorAlbum(*album); err != nil {
			return fmt.Errorf("monitor album: %w", err)
		}
	}
	if err := s.lidarr.SearchAlbum(album.ID); err != nil {
		return fmt.Errorf("search album: %w", err)
	}

	entry := RatingStateEntry{
		SyncedAt:       time.Now().UTC().Format(time.RFC3339),
		Artist:         ev.ArtistName,
		Album:          ev.AlbumName,
		LidarrArtistID: artistID,
		LidarrAlbumID:  album.ID,
		Status:         "ok",
	}
	if err := s.state.Mark(ev.RatingKey, entry); err != nil {
		slog.Warn("failed to persist successful rating state", "err", err.Error())
	}
	slog.Info("queued album for download in Lidarr", "artist", ev.ArtistName, "album", ev.AlbumName, "albumId", album.ID)
	return nil
}

func (s *LidarrSync) resolveArtistMBID(grandparentRatingKey string) (string, error) {
	resp, err := s.plex.GetArtistMetadata(grandparentRatingKey)
	if err != nil {
		return "", err
	}
	for _, m := range resp.MediaContainer.Metadata {
		if mbid := extractArtistMBID(m.Guid, m.GUID); mbid != "" {
			return mbid, nil
		}
	}
	return "", fmt.Errorf("no MBID in artist metadata")
}

// ensureArtist returns the Lidarr artist ID for the given MBID/name, adding it
// to Lidarr if absent. Existing artists are not mutated.
func (s *LidarrSync) ensureArtist(mbid, name string) (int, error) {
	existing, err := s.lidarr.GetArtists()
	if err != nil {
		return 0, fmt.Errorf("list artists: %w", err)
	}
	for _, a := range existing {
		if mbid != "" && strings.EqualFold(a.ForeignArtistID, mbid) {
			return a.ID, nil
		}
		if mbid == "" && strings.EqualFold(a.ArtistName, name) {
			return a.ID, nil
		}
	}

	var lookups []client.LidarrArtist
	if mbid != "" {
		lookups, err = s.lidarr.LookupArtist(mbid)
		if err != nil {
			return 0, fmt.Errorf("lookup by MBID: %w", err)
		}
	}
	if len(lookups) == 0 {
		lookups, err = s.lidarr.LookupArtistByName(name)
		if err != nil {
			return 0, fmt.Errorf("lookup by name: %w", err)
		}
	}
	if len(lookups) == 0 {
		return 0, fmt.Errorf("artist %q not found in Lidarr lookup", name)
	}

	chosen := lookups[0]
	if mbid != "" {
		for _, l := range lookups {
			if strings.EqualFold(l.ForeignArtistID, mbid) {
				chosen = l
				break
			}
		}
	}

	chosen.Monitored = true
	chosen.MonitorNewItems = "none"
	chosen.QualityProfileID = s.cfg.QualityProfileID
	chosen.MetadataProfileID = s.cfg.MetadataProfileID
	chosen.RootFolderPath = s.cfg.RootFolderPath
	chosen.AddOptions = &client.LidarrAddOptions{
		Monitor:                "none",
		SearchForMissingAlbums: false,
	}

	created, err := s.lidarr.AddArtist(chosen)
	if err != nil {
		return 0, fmt.Errorf("add artist: %w", err)
	}
	return created.ID, nil
}

func (s *LidarrSync) findAlbum(ctx context.Context, artistID int, title, mbid string) (*client.LidarrAlbum, error) {
	deadline := time.Now().Add(albumRefreshTimeout)
	for {
		albums, err := s.lidarr.GetAlbumsByArtist(artistID)
		if err != nil {
			return nil, err
		}
		if match := matchAlbum(albums, title, mbid); match != nil {
			return match, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("album %q did not appear in Lidarr after %s", title, albumRefreshTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(albumRefreshPoll):
		}
	}
}

func matchAlbum(albums []client.LidarrAlbum, title, mbid string) *client.LidarrAlbum {
	if mbid != "" {
		for i := range albums {
			if strings.EqualFold(albums[i].ForeignAlbumID, mbid) {
				return &albums[i]
			}
		}
	}
	for i := range albums {
		if strings.EqualFold(albums[i].Title, title) {
			return &albums[i]
		}
	}
	return nil
}

func extractArtistMBID(guids []client.PlexGuid, fallback string) string {
	for _, g := range guids {
		if mbid := mbidFromGUID(g.ID); mbid != "" {
			return mbid
		}
	}
	return mbidFromGUID(fallback)
}

func mbidFromGUID(g string) string {
	if g == "" {
		return ""
	}
	if strings.HasPrefix(g, mbidPrefix) {
		return strings.TrimPrefix(g, mbidPrefix)
	}
	if strings.HasPrefix(g, musicBrainzGUIDPrefix) {
		rest := strings.TrimPrefix(g, musicBrainzGUIDPrefix)
		if i := strings.IndexAny(rest, "?#"); i >= 0 {
			rest = rest[:i]
		}
		return rest
	}
	return ""
}
