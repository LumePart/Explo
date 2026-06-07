package discovery

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type Recommendations struct {
	Payload struct {
		Count       int    `json:"count"`
		Entity      string `json:"entity"`
		LastUpdated int    `json:"last_updated"`
		Mbids       []struct {
			LatestListenedAt time.Time `json:"latest_listened_at"`
			RecordingMbid    string    `json:"recording_mbid"`
			Score            float64   `json:"score"`
		} `json:"mbids"`
		TotalMbidCount int    `json:"total_mbid_count"`
		UserName       string `json:"user_name"`
	} `json:"payload"`
}

type Metadata struct {
	Tag struct {
		Artist       []LBTag `json:"artist"`
		Recording    []LBTag `json:"recording"`
		ReleaseGroup []LBTag `json:"release_group"`
	} `json:"tag"`
	Artist struct {
		ArtistCreditID int `json:"artist_credit_id"`
		Artists        []struct {
			ArtistMbid string `json:"artist_mbid"`
			BeginYear  int    `json:"begin_year"`
			EndYear    int    `json:"end_year,omitempty"`
			JoinPhrase string `json:"join_phrase"`
			Name       string `json:"name"`
		} `json:"artists"`
		Name string `json:"name"`
	} `json:"artist"`
	Recording struct {
		FirstReleaseDate string   `json:"first_release_date"`
		ISRCs            []string `json:"isrcs"`
		Length           int      `json:"length"`
		Name             string   `json:"name"`
		Rels             []any    `json:"rels"`
		URLRels          []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"url_rels"`
	} `json:"recording"`
	Release ReleaseMetadata `json:"release"`
}

type ReleaseMetadata struct {
	AlbumArtistName string `json:"album_artist_name"`
	CaaID           int64  `json:"caa_id"`
	CaaReleaseMbid  string `json:"caa_release_mbid"`
	ReleaseGroup    struct {
		ID               string   `json:"id"`
		PrimaryType      string   `json:"primary_type"`
		SecondaryTypes   []string `json:"secondary_types"`
		FirstReleaseDate string   `json:"first_release_date"`
		Title            string   `json:"title"`
	} `json:"release_group"`
	Status           string `json:"status"`
	Mbid             string `json:"mbid"`
	MBReleaseID      string `json:"mb_release_id"`
	Name             string `json:"name"`
	ReleaseGroupMbid string `json:"release_group_mbid"`
	Year             int    `json:"year"`
}

type Recordings map[string]Metadata

type CreatedFor struct {
	Count         int `json:"count"`
	Offset        int `json:"offset"`
	PlaylistCount int `json:"playlist_count"`
	Playlists     []struct {
		Playlist struct {
			Creator   string    `json:"creator"`
			Date      time.Time `json:"date"`
			Extension struct {
				HTTPSJspfPlaylist struct {
					AdditionalMetadata struct {
						AlgorithmMetadata struct {
							SourcePatch string `json:"source_patch"`
						} `json:"algorithm_metadata"`
					} `json:"additional_metadata"`
					CreatedFor string `json:"created_for"`
				} `json:"https://musicbrainz.org/doc/jspf#playlist"`
			} `json:"extension"`
			Identifier string `json:"identifier"`
		} `json:"playlist"`
	} `json:"playlists"`
}

type Exploration struct {
	Playlist struct {
		Annotation string    `json:"annotation"`
		Creator    string    `json:"creator"`
		Date       time.Time `json:"date"`
		Identifier string    `json:"identifier"`
		Title      string    `json:"title"`
		Tracks     []struct {
			Album     string `json:"album"`
			Creator   string `json:"creator"`
			Duration  int    `json:"duration"`
			Extension struct {
				HTTPSJspfTrack struct {
					AddedAt            time.Time `json:"added_at"`
					AddedBy            string    `json:"added_by"`
					AdditionalMetadata struct {
						Artists []struct {
							ArtistCreditName string `json:"artist_credit_name"`
							ArtistMbid       string `json:"artist_mbid"`
							JoinPhrase       string `json:"join_phrase"`
						} `json:"artists"`
						CaaID          int64  `json:"caa_id"`
						CaaReleaseMbid string `json:"caa_release_mbid"`
					} `json:"additional_metadata"`
					ArtistIdentifiers []string `json:"artist_identifiers"`
				} `json:"https://musicbrainz.org/doc/jspf#track"`
			} `json:"extension"`
			Identifier []string `json:"identifier"`
			Title      string   `json:"title"`
		} `json:"track"`
	} `json:"playlist"`
}

type TopRecordings struct {
	Payload struct {
		Recordings []struct {
			ArtistName  string `json:"artist_name"`
			ReleaseMbid string `json:"release_mbid"`
			ReleaseName string `json:"release_name"`
			TrackName   string `json:"track_name"`
		} `json:"recordings"`
	} `json:"payload"`
}

type MBRecording struct {
	ID       string `json:"id"`
	Releases []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Status       string `json:"status"`
		Country      string `json:"country"`
		Date         string `json:"date"`
		Year         int    `json:"year,omitempty"`
		ArtistCredit []struct {
			Name   string `json:"name"`
			Artist struct {
				ID       string `json:"id"`
				SortName string `json:"sort-name"`
			} `json:"artist"`
		} `json:"artist-credit"`
		ReleaseGroup struct {
			ID          string `json:"id"`
			PrimaryType string `json:"primary-type"`
		} `json:"release-group"`
		Media []struct {
			Position   int    `json:"position"`
			Format     string `json:"format"`
			TrackCount int    `json:"track-count"`
			Tracks     []struct {
				ID       string `json:"id"`
				Position int    `json:"position"`
				Number   string `json:"number"`
			} `json:"tracks"`
		} `json:"media"`
	} `json:"releases"`
}

type ListenBrainz struct {
	HttpClient *util.HttpClient
	cfg        cfg.Listenbrainz
	Separator  string
}

func NewListenBrainz(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *ListenBrainz {
	return &ListenBrainz{
		cfg:        cfg.Listenbrainz,
		HttpClient: httpClient,
	}
}
func (c *ListenBrainz) QueryTracks() ([]*models.Track, error) {
	// Stats-based playlists bypass the discovery mode switch
	if c.cfg.ImportPlaylist == "on-repeat" {
		return c.getTopRecordings(c.cfg.User)
	}

	var tracks []*models.Track

	switch c.cfg.Discovery {
	case "playlist":
		id, err := c.getImportPlaylist(c.cfg.User)
		if err != nil {
			return nil, err
		}
		_, tracks, err = c.parsePlaylist(id, c.cfg.SingleArtist)
		if err != nil {
			return nil, err
		}
		if c.cfg.EnrichTrackMetadata && len(tracks) > 0 {
			enrichedTracks, err := c.enrichTracks(tracks, c.cfg.SingleArtist)
			if err != nil {
				slog.Warn("failed to enrich playlist metadata", "error", err)
			} else {
				tracks = enrichedTracks
			}
		}

	default:
		mbids, err := c.getAPIRecommendations(c.cfg.User)
		if err != nil {
			return nil, err
		}
		tracks, err = c.getTracks(mbids, c.cfg.SingleArtist)
		if err != nil {
			return nil, err
		}
	}
	return tracks, nil
}

func (c *ListenBrainz) getAPIRecommendations(user string) ([]string, error) {
	var mbids []string

	body, err := c.lbRequest(fmt.Sprintf("cf/recommendation/user/%s/recording", user))
	if err != nil {
		return mbids, fmt.Errorf("could not get recommendations from API: %s", err.Error())
	}

	var reccs Recommendations
	err = util.ParseResp(body, &reccs)
	if err != nil {
		return mbids, fmt.Errorf("could not get recommendations from API: %s", err.Error())
	}

	for _, rec := range reccs.Payload.Mbids {
		mbids = append(mbids, rec.RecordingMbid)
	}

	if len(mbids) == 0 {
		return mbids, fmt.Errorf("no recommendations found, exiting")
	}
	return mbids, nil
}

func (c *ListenBrainz) getTopRecordings(user string) ([]*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("stats/user/%s/recordings?count=30&range=month", user))
	if err != nil {
		return nil, fmt.Errorf("getTopRecordings(): %s", err.Error())
	}

	var resp TopRecordings
	if err := util.ParseResp(body, &resp); err != nil {
		return nil, fmt.Errorf("getTopRecordings(): %s", err.Error())
	}

	if len(resp.Payload.Recordings) == 0 {
		return nil, fmt.Errorf("no top recordings found for user %s", user)
	}

	tracks := make([]*models.Track, 0, len(resp.Payload.Recordings))
	for _, rec := range resp.Payload.Recordings {
		var coverURL string
		if rec.ReleaseMbid != "" {
			coverURL = fmt.Sprintf("https://coverartarchive.org/release/%s/front-%s", rec.ReleaseMbid, c.cfg.CoverArtSize)
		}
		tracks = append(tracks, &models.Track{
			Title:      rec.TrackName,
			CleanTitle: rec.TrackName,
			Artist:     rec.ArtistName,
			MainArtist: rec.ArtistName,
			Album:      rec.ReleaseName,
			CoverURL:   coverURL,
		})
	}

	return tracks, nil
}
func (c *ListenBrainz) LookupRecording(mbid string) (*models.Track, error) {
	tracks, err := c.getTracks([]string{mbid}, false)
	if err != nil {
		return nil, err
	}
	return tracks[0], nil
}
func (c *ListenBrainz) getTracks(mbids []string, singleArtist bool) ([]*models.Track, error) {
	strMbids := strings.Join(mbids, ",")

	body, err := c.lbRequest(fmt.Sprintf("metadata/recording/?recording_mbids=%s&inc=release+artist", strMbids))
	if err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	var recordings Recordings
	if err := util.ParseResp(body, &recordings); err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	if len(recordings) == 0 {
		return nil, fmt.Errorf("no recordings found for MBIDs: %s", strMbids)
	}

	tracks := make([]*models.Track, 0, len(recordings))
	for mbTrackID, recording := range recordings {
		rec := recording.Recording
		rel := recording.Release

		title := rec.Name
		artist := recording.Artist.Name
		mainArtist := recording.Artist.Name

		recArtists := recording.Artist.Artists

		if len(recArtists) > 1 {
			mainArtist = recArtists[0].Name
			if singleArtist {
				var b strings.Builder
				b.WriteString(title)
				b.WriteString(" feat. ")
				b.WriteString(recArtists[1].Name)

				for _, a := range recArtists[2:] {
					b.WriteString(", ")
					b.WriteString(a.Name)
				}

				title = b.String()
				artist = mainArtist
			}
		}

		tracks = append(tracks, &models.Track{
			Album:                     recording.Release.Name,
			Artist:                    artist,
			AlbumArtist:               rel.AlbumArtistName,
			MainArtist:                mainArtist,
			CleanTitle:                rec.Name,
			Title:                     title,
			Duration:                  rec.Length,
			MusicBrainzTrackID:        mbTrackID,
			MusicBrainzAlbumID:        rel.CaaReleaseMbid,
			MusicBrainzReleaseGroupID: rel.ReleaseGroupMbid,
			MusicBrainzArtistID:       recArtists[0].ArtistMbid,
		})
	}

	return tracks, nil

}

func (c *ListenBrainz) enrichTracks(tracks []*models.Track, singleArtist bool) ([]*models.Track, error) {
	mbids := make([]string, 0, len(tracks))
	// wait time in s between MusicBrainz requests
	waitTime := 2
	totalWait := waitTime * len(tracks)
	slog.Info("enriching tracks with metadata. This may take a moment", "estimated_seconds", totalWait)

	for _, track := range tracks {
		if track.MusicBrainzTrackID != "" {
			mbids = append(mbids, track.MusicBrainzTrackID)
		}
	}
	strMbids := strings.Join(mbids, ",")

	body, err := c.lbRequest(fmt.Sprintf("metadata/recording/?recording_mbids=%s&inc=release+artist+tag+release_group+recording", strMbids))
	if err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	var recordings Recordings
	if err := util.ParseResp(body, &recordings); err != nil {
		return nil, fmt.Errorf("getTracks(): %s", err.Error())
	}

	if len(recordings) == 0 {
		return nil, fmt.Errorf("no recordings found for MBIDs: %s", strMbids)
	}

	for i, track := range tracks {
		recording, ok := recordings[track.MusicBrainzTrackID]
		if !ok {
			continue
		}

		rec := recording.Recording

		title := rec.Name
		artist := recording.Artist.Name
		mainArtist := recording.Artist.Name
		mainArtistID := ""
		if len(recording.Artist.Artists) > 0 {
			mainArtistID = recording.Artist.Artists[0].ArtistMbid
		}

		recArtists := recording.Artist.Artists
		artists := make([]string, 0, len(recArtists))

		genres := topTags(recording.Tag.Recording, 3)
		if len(genres) == 0 {
			genres = topTags(recording.Tag.ReleaseGroup, 3)
		}
		if len(genres) == 0 {
			genres = topTags(recording.Tag.Artist, 3)
		}
		originalDate := rec.FirstReleaseDate
		originalYear := recording.Release.Year
		if originalYear == 0 && len(originalDate) >= 4 {
			if year, err := strconv.Atoi(originalDate[:4]); err == nil {
				originalYear = year
			}
		}

		if len(recArtists) > 1 {
			mainArtist = recArtists[0].Name
			if singleArtist {
				var b strings.Builder
				b.WriteString(title)
				b.WriteString(" feat. ")
				b.WriteString(recArtists[1].Name)

				for _, a := range recArtists[2:] {
					b.WriteString(", ")
					b.WriteString(a.Name)
				}

				title = b.String()
				artist = mainArtist
			} else {
				for _, recArtist := range recArtists {
					artists = append(artists, recArtist.Name)
				}
			}
		}

		releaseCountry := ""
		releaseStatus := recording.Release.Status
		mbReleaseGroupID := recording.Release.ReleaseGroupMbid
		mbAlbumArtistID := ""
		artistSort := ""
		media := ""
		trackNumber := 0
		trackTotal := 0
		discNumber := 0
		discTotal := 0
		mbReleaseTrackID := ""
		releaseType := ""

		var mbData *MBRecording
		var mbErr error
		for attempt := 1; attempt <= 3; attempt++ {

			if attempt != 1 && attempt <= 3 {
				time.Sleep(time.Duration(waitTime) * time.Second)
			}
			mbData, mbErr = c.mbRequest(fmt.Sprintf("recording/%s?inc=media+releases+artist-credits+release-groups&fmt=json", track.MusicBrainzTrackID))
			if mbErr == nil && mbData != nil {
				break
			}
		}

		if mbData != nil {
			if len(mbData.Releases) > 0 {
				// Find best matching release
				bestRelease := mbData.Releases[0]
				if track.MusicBrainzAlbumID != "" {
					for _, rel := range mbData.Releases {
						if rel.ID == track.MusicBrainzAlbumID {
							bestRelease = rel
							break
						}
					}
				}

				if bestRelease.Country != "" {
					releaseCountry = bestRelease.Country
				}
				if bestRelease.Status != "" {
					releaseStatus = bestRelease.Status
				}
				if bestRelease.ReleaseGroup.ID != "" {
					mbReleaseGroupID = bestRelease.ReleaseGroup.ID
				}
				if bestRelease.ReleaseGroup.PrimaryType != "" {
					releaseType = bestRelease.ReleaseGroup.PrimaryType
				}
				if len(bestRelease.ArtistCredit) > 0 {
					mbAlbumArtistID = bestRelease.ArtistCredit[0].Artist.ID
					artistSort = bestRelease.ArtistCredit[0].Artist.SortName
				}

				discTotal = len(bestRelease.Media)
				if len(bestRelease.Media) > 0 {
					firstMedia := bestRelease.Media[0]
					media = firstMedia.Format
					discNumber = firstMedia.Position
					trackTotal = firstMedia.TrackCount

					if len(firstMedia.Tracks) > 0 {
						firstTrack := firstMedia.Tracks[0]
						trackNumber = firstTrack.Position
						mbReleaseTrackID = firstTrack.ID
					}
				}
			}
		} else {
			slog.Debug("failed to enrich from MusicBrainz after retries", "mbid", track.MusicBrainzTrackID, "error", mbErr)
		}

		tracks[i] = &models.Track{
			ID:                        track.ID,
			File:                      track.File,
			Size:                      track.Size,
			Present:                   track.Present,
			Album:                     recording.Release.Name,
			AlbumArtist:               recording.Release.AlbumArtistName,
			Artist:                    artist,
			Artists:                   artists,
			MainArtist:                mainArtist,
			MainArtistID:              mainArtistID,
			ArtistSort:                artistSort,
			CleanTitle:                rec.Name,
			Title:                     title,
			Duration:                  rec.Length,
			ReleaseCountry:            releaseCountry,
			ReleaseStatus:             releaseStatus,
			ReleaseType:               releaseType,
			OriginalDate:              originalDate,
			OriginalYear:              originalYear,
			CoverURL:                  track.CoverURL,
			Genres:                    strings.Join(genres, "; "),
			ISRCs:                     append([]string(nil), rec.ISRCs...),
			Media:                     media,
			TrackNumber:               trackNumber,
			TrackTotal:                trackTotal,
			DiscNumber:                discNumber,
			DiscTotal:                 discTotal,
			MusicBrainzTrackID:        track.MusicBrainzTrackID,
			MusicBrainzAlbumID:        recording.Release.CaaReleaseMbid,
			MusicBrainzReleaseGroupID: mbReleaseGroupID,
			MusicBrainzAlbumArtistID:  mbAlbumArtistID,
			MusicBrainzReleaseTrackID: mbReleaseTrackID,
			MusicBrainzArtistID: func() string {
				if len(recArtists) == 0 {
					return ""
				}
				return recArtists[0].ArtistMbid
			}(),
		}
	}

	return tracks, nil

}

// Get user LB playlists and find wanted playlists ID
func (c *ListenBrainz) getImportPlaylist(user string) (string, error) {
	var offset int
	var bestDate time.Time
	var bestID string

	for {
		var body []byte
		var err error

		for retries := range 5 {
			body, err = c.lbRequest(fmt.Sprintf("user/%s/playlists/createdfor?offset=%d", user, offset))
			if err == nil {
				break
			}
			slog.Warn(
				"failed getting response from ListenBrainz, retrying in 5 minutes",
				"retry", retries+1,
				"error", err,
			)
			time.Sleep(5 * time.Minute)
		}

		if err != nil {
			return "", fmt.Errorf("failed getting ListenBrainz playlist after retries: %s", err.Error())
		}

		var playlists CreatedFor
		if err = util.ParseResp(body, &playlists); err != nil {
			return "", fmt.Errorf("getImportPlaylist(): %s", err.Error())
		}

		for _, p := range playlists.Playlists {
			meta := p.Playlist.Extension.HTTPSJspfPlaylist.AdditionalMetadata
			if meta.AlgorithmMetadata.SourcePatch != c.cfg.ImportPlaylist {
				continue
			}
			if bestID == "" || p.Playlist.Date.After(bestDate) {
				bestDate = p.Playlist.Date
				parts := strings.Split(p.Playlist.Identifier, "/")
				bestID = parts[len(parts)-1]
			}
		}

		if playlists.Count+playlists.Offset >= playlists.PlaylistCount || playlists.Count == 0 {
			break
		}
		offset += playlists.Count
	}

	if bestID == "" {
		return "", fmt.Errorf("failed to get %s playlist, check if ListenBrainz has generated one", c.cfg.ImportPlaylist)
	}
	return bestID, nil
}


// FetchPlaylistByMBID fetches a LB playlist by MBID. For use outside the discovery flow.
func FetchPlaylistByMBID(httpClient *util.HttpClient, mbid string) (string, []*models.Track, error) {
	lb := &ListenBrainz{HttpClient: httpClient}
	return lb.parsePlaylist(mbid, false)
}

// FetchTopRecordings returns the user's top recordings for the current month.
func FetchTopRecordings(httpClient *util.HttpClient, user string) ([]*models.Track, error) {
	lb := &ListenBrainz{HttpClient: httpClient}
	return lb.getTopRecordings(user)
}

// FetchMostRecentPlaylistByType finds and fetches the most recent LB-generated playlist of the given type for the user.
func FetchMostRecentPlaylistByType(httpClient *util.HttpClient, user, playlistType string) ([]*models.Track, error) {
	lb := &ListenBrainz{HttpClient: httpClient, cfg: cfg.Listenbrainz{ImportPlaylist: playlistType}}
	id, err := lb.getImportPlaylist(user)
	if err != nil {
		return nil, err
	}
	_, tracks, err := lb.parsePlaylist(id, false)
	return tracks, err
}

func (c *ListenBrainz) parsePlaylist(identifier string, singleArtist bool) (string, []*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("playlist/%s", identifier))
	if err != nil {
		return "", nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	var exploration Exploration
	err = util.ParseResp(body, &exploration)
	if err != nil {
		return "", nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	srcTracks := exploration.Playlist.Tracks
	if len(srcTracks) == 0 {
		return "", nil, fmt.Errorf("no tracks found in playlist %s", identifier)
	}

	tracks := make([]*models.Track, 0, len(srcTracks))
	for _, track := range srcTracks {
		title := track.Title
		artist := track.Creator
		mainArtist := track.Creator

		trackMeta := track.Extension.HTTPSJspfTrack.AdditionalMetadata
		trackArtists := trackMeta.Artists

		var coverURL string
		if trackMeta.CaaReleaseMbid != "" && trackMeta.CaaID != 0 {
			coverURL = fmt.Sprintf("https://coverartarchive.org/release/%s/%d-%s.jpg",
				trackMeta.CaaReleaseMbid, trackMeta.CaaID, c.cfg.CoverArtSize)
		}

		if len(trackMeta.Artists) > 1 {
			mainArtist = trackMeta.Artists[0].ArtistCreditName
			if singleArtist {
				var b strings.Builder
				b.WriteString(title)
				b.WriteString(" feat. ")
				b.WriteString(trackArtists[1].ArtistCreditName)

				for _, a := range trackArtists[2:] {
					b.WriteString(", ")
					b.WriteString(a.ArtistCreditName)
				}
				title = b.String()
				artist = trackArtists[0].ArtistCreditName
			}
		}

		recordingMBID := ""
		if len(track.Identifier) > 0 {
			parts := strings.Split(track.Identifier[0], "/")
			recordingMBID = parts[len(parts)-1]
		}

		artistMBID := ""
		if len(trackMeta.Artists) > 0 {
			artistMBID = trackMeta.Artists[0].ArtistMbid
		}

		tracks = append(tracks, &models.Track{
			Album:               track.Album,
			MainArtist:          mainArtist,
			Artist:              artist,
			CleanTitle:          track.Title,
			Title:               title,
			Duration:            track.Duration,
			CoverURL:            coverURL,
			MusicBrainzTrackID:  recordingMBID,
			MusicBrainzArtistID: artistMBID,
			MusicBrainzAlbumID:  trackMeta.CaaReleaseMbid,
		})
	}

	return exploration.Playlist.Title, tracks, nil

}

// Handle ListenBrainz API requests
func (c *ListenBrainz) lbRequest(path string) ([]byte, error) {

	reqURL := fmt.Sprintf("https://api.listenbrainz.org/1/%s", path)
	body, err := c.HttpClient.MakeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to ListenBrainz API: %s", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("ListenBrainz API returned empty response for: %s", reqURL)
	}

	return body, nil
}

func (c *ListenBrainz) mbRequest(path string) (*MBRecording, error) {
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/%s", path)
	body, err := c.HttpClient.MakeRequest("GET", reqURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to MusicBrainz API: %s", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("MusicBrainz API returned empty response for: %s", reqURL)
	}

	var recording MBRecording
	if err := util.ParseResp(body, &recording); err != nil {
		return nil, fmt.Errorf("failed to parse MusicBrainz response: %s", err)
	}

	return &recording, nil
}

type LBTag struct {
	Count int    `json:"count"`
	Tag   string `json:"tag"`
}

func topTags(tags []LBTag, limit int) []string {
	if len(tags) == 0 || limit <= 0 {
		return nil
	}

	ordered := append([]LBTag(nil), tags...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Count == ordered[j].Count {
			return ordered[i].Tag < ordered[j].Tag
		}
		return ordered[i].Count > ordered[j].Count
	})

	if limit > len(ordered) {
		limit = len(ordered)
	}

	out := make([]string, 0, limit)
	for _, tag := range ordered[:limit] {
		if tag.Tag != "" {
			out = append(out, tag.Tag)
		}
	}
	return out
}
