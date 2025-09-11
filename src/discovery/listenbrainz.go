package discovery

import (
	"fmt"
	"slices"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/debug"
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

type Playlists struct {
	Playlist []struct {
		Data struct {
			Date       time.Time `json:"date"`
			Identifier string    `json:"identifier"`
			Title      string    `json:"title"`
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
				HTTPSMusicbrainzOrgDocJspfTrack struct {
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
	var dailyTracks []*models.Track
	var weeklyTracks []*models.Track

	switch c.cfg.Discovery {
	case "playlist":
		id, err := c.getWeeklyExploration(c.cfg.User)
		if err != nil {
			return nil, err
		}
		if id != "" {
			weeklyTracks, err = c.parseExploration(id, c.cfg.SingleArtist)
			if err != nil {
				return nil, err
			}
		}

		if c.cfg.IncludeDaily {
			id, err = c.getDailyExploration(c.cfg.User)
			if err != nil {
				return nil, err
			}
			if id != "" {
				dailyTracks, err = c.parseExploration(id, c.cfg.SingleArtist)
		if err != nil {
			return nil, err
				}
			}
		}

		tracks = slices.Concat(weeklyTracks, dailyTracks)

		if len(tracks) == 0 {
			return nil, fmt.Errorf("no new playlists found. Check to see if ListenBrainz has created any")
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
		title := recording.Recording.Name
		artist := recording.Artist.Name
		mainArtist := recording.Artist.Name

		if len(recording.Artist.Artists) > 1 {
			mainArtist = recording.Artist.Artists[0].Name
			if singleArtist {
				var tempTitle strings.Builder
				joinPhrase := " feat. "
				for i, artist := range recording.Artist.Artists[1:] {
					if i > 0 {
						joinPhrase = ", "
					}
					tempTitle.WriteString(joinPhrase)
					tempTitle.WriteString(artist.Name)
				}
				title = fmt.Sprintf("%s%s", recording.Recording.Name, tempTitle.String())
				artist = recording.Artist.Artists[0].Name
			}
		}

		tracks = append(tracks, &models.Track{
			Album:       recording.Release.Name,
			Artist:      artist,
			MainArtist:  mainArtist,
			CleanTitle:  recording.Recording.Name,
			Title:       title,
			Duration:    recording.Recording.Length,
		})
	}

	return tracks, nil

}

func (c *ListenBrainz) getWeeklyExploration(user string) (string, error) { // Get user LB playlists and find Weekly Exploration's ID
	body, err := c.lbRequest(fmt.Sprintf("user/%s/playlists/createdfor", user))
	if err != nil {
		return "", fmt.Errorf("getWeeklyExploration(): %s", err.Error())
	}

	var playlists Playlists
	err = util.ParseResp(body, &playlists)
	if err != nil {
		return "", fmt.Errorf("getWeeklyExploration(): %s", err.Error())
	}

	for _, playlist := range playlists.Playlist {

		_, currentWeek := time.Now().Local().ISOWeek()
		_, creationWeek := playlist.Data.Date.Local().ISOWeek()

		if strings.Contains(playlist.Data.Title, "Weekly Exploration") && currentWeek == creationWeek {
			id := strings.Split(playlist.Data.Identifier, "/")
			return id[len(id)-1], nil
		}
	}
	debug.Debug(fmt.Sprintf("playlist output: %v", playlists))
	debug.Debug("failed to get new Weekly exploration playlist, check if ListenBrainz has generated one this week")
	return "", nil
}

func (c *ListenBrainz) getDailyExploration(user string) (string, error) { // Get user LB playlists and find Daily Exploration ID
	body, err := c.lbRequest(fmt.Sprintf("user/%s/playlists/createdfor", user))
	if err != nil {
		return "", fmt.Errorf("getDailyExploration(): %s", err.Error())
	}

	var playlists Playlists
	err = util.ParseResp(body, &playlists)
	if err != nil {
		return "", fmt.Errorf("getDailyExploration(): %s", err.Error())
	}

	for _, playlist := range playlists.Playlist {

		currentDay := time.Now().Local()
		playlistTitle := fmt.Sprintf("Daily Jams for %s, %s %s", c.cfg.User, currentDay.Format(time.DateOnly), currentDay.Weekday().String()[:3])

		if strings.Compare(playlist.Data.Title, playlistTitle) == 0 {
			id := strings.Split(playlist.Data.Identifier, "/")
			return id[len(id)-1], nil
		}
	}
	debug.Debug(fmt.Sprintf("playlist output: %v", playlists))
	debug.Debug("failed to get new daily exploration playlist, check if ListenBrainz has generated one today")
	return "", nil
}

func (c *ListenBrainz) parseExploration(identifier string, singleArtist bool) ([]*models.Track, error) {
	body, err := c.lbRequest(fmt.Sprintf("playlist/%s", identifier))
	if err != nil {
		return nil, fmt.Errorf("parseExploration(): %s", err.Error())
	}

	var exploration Exploration
	err = util.ParseResp(body, &exploration)
	if err != nil {
		return nil, fmt.Errorf("parseExploration(): %s", err.Error())
	}

	if len(exploration.Playlist.Tracks) == 0 {
		return nil, fmt.Errorf("no tracks found in playlist %s", identifier)
	}

	tracks := make([]*models.Track, 0, len(exploration.Playlist.Tracks))
	for _, track := range exploration.Playlist.Tracks {
		title := track.Title
		artist := track.Creator
		mainArtist := track.Creator

		if len(track.Extension.HTTPSMusicbrainzOrgDocJspfTrack.AdditionalMetadata.Artists) > 1 {
			mainArtist = track.Extension.HTTPSMusicbrainzOrgDocJspfTrack.AdditionalMetadata.Artists[0].ArtistCreditName
			if singleArtist {
				var tempTitle strings.Builder
				joinPhrase := " feat. "
				for i, artist := range track.Extension.HTTPSMusicbrainzOrgDocJspfTrack.AdditionalMetadata.Artists[1:] {
					if i > 0 {
						joinPhrase = ", "
					}
					tempTitle.WriteString(joinPhrase)
					tempTitle.WriteString(artist.ArtistCreditName)
				}
				title = fmt.Sprintf("%s%s", track.Title, tempTitle.String())
				artist = track.Extension.HTTPSMusicbrainzOrgDocJspfTrack.AdditionalMetadata.Artists[0].ArtistCreditName
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
