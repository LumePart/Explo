package discovery

import (
	"fmt"
	"strings"
	"time"
	"log/slog"
	"errors"

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
		Length int    `json:"length"`
		Name   string `json:"name"`
		Rels   []any  `json:"rels"`
	} `json:"recording"`
	Release struct {
		AlbumArtistName  string `json:"album_artist_name"`
		CaaID            int64  `json:"caa_id"`
		CaaReleaseMbid   string `json:"caa_release_mbid"`
		Mbid             string `json:"mbid"`
		Name             string `json:"name"`
		ReleaseGroupMbid string `json:"release_group_mbid"`
		Year             int    `json:"year"`
	} `json:"release"`
}

type Recordings map[string]Metadata

type CreatedFor struct {
	Count         int `json:"count"`
	Offset        int `json:"offset"`
	PlaylistCount int `json:"playlist_count"`
	Playlists     []struct {
		Playlist struct {
			Creator    string    `json:"creator"`
			Date       time.Time `json:"date"`
			Extension  struct {
				HTTPSJspfPlaylist struct {
					AdditionalMetadata struct {
						AlgorithmMetadata struct {
							SourcePatch string `json:"source_patch"`
						} `json:"algorithm_metadata"`
					} `json:"additional_metadata"`
					CreatedFor     string    `json:"created_for"`
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
			Album      string `json:"album"`
			Creator    string `json:"creator"`
			Duration   int `json:"duration"`
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
			Title      string `json:"title"`
		} `json:"track"`
	} `json:"playlist"`
}

type ListenBrainz struct {
	HttpClient *util.HttpClient
	cfg cfg.Listenbrainz
	Separator string
}


func NewListenBrainz(cfg cfg.DiscoveryConfig, httpClient *util.HttpClient) *ListenBrainz {
	return &ListenBrainz{
		cfg: cfg.Listenbrainz,
		HttpClient: httpClient,
	}
}
func (c *ListenBrainz) QueryTracks() ([]*models.Track, error)  {
	var tracks []*models.Track

	switch c.cfg.Discovery {
	case "playlist":
		id, err := c.getImportPlaylist(c.cfg.User)
		if err != nil {
			return nil, err
		}
		tracks, err = c.parsePlaylist(id, c.cfg.SingleArtist)
		if err != nil {
			return nil, err
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
	for _, recording := range recordings {
		rec := recording.Recording

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
			Album:       recording.Release.Name,
			Artist:      artist,
			MainArtist:  mainArtist,
			CleanTitle:  rec.Name,
			Title:       title,
			Duration:    rec.Length,
		})
	}

	return tracks, nil

}

func (c *ListenBrainz) getImportPlaylist(user string) (string, error) { // Get user LB playlists and find wanted playlists ID
	var offset int
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
		err = util.ParseResp(body, &playlists)
		if err != nil {
			return "", fmt.Errorf("getImportPlaylist(): %s", err.Error())
		}
	
		if id, err := c.parseCreatedFor(playlists); err == nil {
			return id, nil
		}

		if playlists.Count+playlists.Offset >= playlists.PlaylistCount {
			break
		}

		offset += playlists.Count
	}
	return "", fmt.Errorf("failed to get %s playlist, check if ListenBrainz has generated one", c.cfg.ImportPlaylist)
}

func (c *ListenBrainz) parseCreatedFor(playlists CreatedFor) (string, error) {
	var currentWeek, currentDay int
	now := time.Now().Local()
	isDaily := c.cfg.ImportPlaylist == "daily-jams"
	if isDaily {
		currentDay = now.YearDay()
	} else {
		_, currentWeek = now.ISOWeek()
	}
	
	for _, p := range playlists.Playlists {
		meta := p.Playlist.Extension.HTTPSJspfPlaylist.AdditionalMetadata

		if meta.AlgorithmMetadata.SourcePatch != c.cfg.ImportPlaylist {
			continue
		}

		created := p.Playlist.Date.Local()
		var timeMatch bool

		if isDaily {
			timeMatch = created.YearDay() == currentDay
		} else {
			_, w := created.ISOWeek()
			timeMatch = w == currentWeek
		}

		if !timeMatch {
			continue
		}

		parts := strings.Split(p.Playlist.Identifier, "/")
		return parts[len(parts)-1], nil
	}

	slog.Debug("playlist output", "playlists", playlists)
	return "", errors.New("playlist not found in this page")
}

func (c *ListenBrainz) parsePlaylist(identifier string, singleArtist bool) ([]*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("playlist/%s", identifier))
	if err != nil {
		return nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	var exploration Exploration
	err = util.ParseResp(body, &exploration)
	if err != nil {
		return nil, fmt.Errorf("parsePlaylist: %s", err.Error())
	}
	srcTracks := exploration.Playlist.Tracks
	if len(srcTracks) == 0 {
		return nil, fmt.Errorf("no tracks found in playlist %s", identifier)
	}

	tracks := make([]*models.Track, 0, len(srcTracks))
	for _, track := range srcTracks {
		title := track.Title
		artist := track.Creator
		mainArtist := track.Creator

		trackMeta := track.Extension.HTTPSJspfTrack.AdditionalMetadata
		trackArtists := trackMeta.Artists

		if len(trackMeta.Artists) > 1 {
			mainArtist = trackMeta.Artists[0].ArtistCreditName
			if singleArtist {
				var b strings.Builder
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

		tracks = append(tracks, &models.Track{
			Album:      track.Album,
			MainArtist: mainArtist,
			Artist:     artist,
			CleanTitle: track.Title,
			Title:      title,
			Duration:   track.Duration,
		})
	}

	return tracks, nil

}

func (c *ListenBrainz) lbRequest(path string) ([]byte, error) { // Handle ListenBrainz API requests


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