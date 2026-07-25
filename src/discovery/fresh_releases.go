package discovery

import (
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

const (
	freshDays        = 30
	freshMaxReleases = 8
	freshArtistCount = 100
	mbMinInterval    = 1100 * time.Millisecond
)

type freshReleasesResp struct {
	Payload struct {
		Releases []freshRelease `json:"releases"`
	} `json:"payload"`
}

type freshRelease struct {
	ArtistCreditName        string   `json:"artist_credit_name"`
	ArtistMbids             []string `json:"artist_mbids"`
	CaaReleaseMbid          string   `json:"caa_release_mbid"`
	ReleaseDate             string   `json:"release_date"`
	ReleaseGroupMbid        string   `json:"release_group_mbid"`
	ReleaseGroupPrimaryType string   `json:"release_group_primary_type"`
	ReleaseMbid             string   `json:"release_mbid"`
	ReleaseName             string   `json:"release_name"`
	Confidence              float64  `json:"confidence"`
}

type topArtistsResp struct {
	Payload struct {
		Artists []struct {
			ArtistMbid string `json:"artist_mbid"`
			ArtistName string `json:"artist_name"`
		} `json:"artists"`
	} `json:"payload"`
}

type mbRelease struct {
	Status       string `json:"status"`
	ArtistCredit []struct {
		Name   string `json:"name"`
		Artist struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artist"`
	} `json:"artist-credit"`
	Media []struct {
		Position int `json:"position"`
		Tracks   []struct {
			Position  int    `json:"position"`
			Title     string `json:"title"`
			Length    int    `json:"length"`
			Recording struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Length int    `json:"length"`
			} `json:"recording"`
		} `json:"tracks"`
	} `json:"media"`
}

// FetchFreshReleaseTracks loads Fresh Releases tracks for UI prefetch.
func FetchFreshReleaseTracks(httpClient *util.HttpClient, user string) ([]*models.Track, error) {
	return (&ListenBrainz{
		HttpClient: httpClient,
		cfg:        cfg.Listenbrainz{User: user, CoverArtSize: "250"},
	}).getFreshReleaseTracks(user)
}

func (c *ListenBrainz) getFreshReleaseTracks(user string) ([]*models.Track, error) {
	releases, err := c.resolveFreshReleases(user)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no fresh releases for %s", user)
	}

	tracks := make([]*models.Track, 0)
	for i, rel := range releases {
		if i > 0 {
			time.Sleep(mbMinInterval)
		}
		albumTracks, err := c.tracksFromRelease(rel)
		if err != nil {
			slog.Warn("fresh-releases: skip release", "album", rel.ReleaseName, "err", err)
			continue
		}
		tracks = append(tracks, albumTracks...)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("could not get any tracks from fresh releases")
	}
	slog.Info("fresh-releases: ready", "user", user, "tracks", len(tracks))
	return tracks, nil
}

func (c *ListenBrainz) resolveFreshReleases(user string) ([]freshRelease, error) {
	path := fmt.Sprintf(
		"user/%s/fresh_releases?days=%d&past=true&future=false&sort=confidence",
		url.PathEscape(user), freshDays,
	)
	if releases, err := c.fetchFreshReleases(path); err != nil {
		slog.Debug("fresh-releases: personalized unavailable", "err", err)
	} else if picked := pickFreshReleases(releases); len(picked) > 0 {
		return picked, nil
	}

	releases, err := c.fetchMatchedFreshReleases(user)
	if err != nil {
		return nil, err
	}
	return pickFreshReleases(releases), nil
}

func (c *ListenBrainz) fetchFreshReleases(path string) ([]freshRelease, error) {
	body, err := c.lbRequest(path)
	if err != nil {
		return nil, err
	}
	var resp freshReleasesResp
	if err := util.ParseResp(body, &resp); err != nil {
		return nil, err
	}
	return resp.Payload.Releases, nil
}

func (c *ListenBrainz) fetchMatchedFreshReleases(user string) ([]freshRelease, error) {
	body, err := c.lbRequest(fmt.Sprintf(
		"stats/user/%s/artists?range=year&count=%d",
		url.PathEscape(user), freshArtistCount,
	))
	if err != nil {
		return nil, fmt.Errorf("top artists: %w", err)
	}
	var artists topArtistsResp
	if err := util.ParseResp(body, &artists); err != nil {
		return nil, err
	}
	if len(artists.Payload.Artists) == 0 {
		return nil, fmt.Errorf("no artist stats for %s", user)
	}

	mbids := map[string]struct{}{}
	names := map[string]struct{}{}
	for _, a := range artists.Payload.Artists {
		if a.ArtistMbid != "" {
			mbids[a.ArtistMbid] = struct{}{}
		}
		if a.ArtistName != "" {
			names[normalizeName(a.ArtistName)] = struct{}{}
		}
	}

	releases, err := c.fetchFreshReleases(fmt.Sprintf(
		"explore/fresh-releases/?days=%d&past=true&future=false&sort=release_date",
		freshDays,
	))
	if err != nil {
		return nil, err
	}

	out := make([]freshRelease, 0)
	for _, rel := range releases {
		if matchesArtists(rel, mbids, names) {
			out = append(out, rel)
		}
	}
	return out, nil
}

func matchesArtists(rel freshRelease, mbids, names map[string]struct{}) bool {
	for _, id := range rel.ArtistMbids {
		if _, ok := mbids[id]; ok {
			return true
		}
	}
	_, ok := names[normalizeName(rel.ArtistCreditName)]
	return ok
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func pickFreshReleases(in []freshRelease) []freshRelease {
	ranked := append([]freshRelease(nil), in...)
	sort.SliceStable(ranked, func(i, j int) bool {
		pi, pj := typeRank(ranked[i].ReleaseGroupPrimaryType), typeRank(ranked[j].ReleaseGroupPrimaryType)
		if pi != pj {
			return pi < pj
		}
		if ranked[i].Confidence != ranked[j].Confidence {
			return ranked[i].Confidence > ranked[j].Confidence
		}
		return ranked[i].ReleaseDate > ranked[j].ReleaseDate
	})

	out := make([]freshRelease, 0, freshMaxReleases)
	seen := map[string]struct{}{}
	for _, rel := range ranked {
		if rel.ReleaseMbid == "" {
			continue
		}
		key := rel.ReleaseGroupMbid
		if key == "" {
			key = rel.ReleaseMbid
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, rel)
		if len(out) >= freshMaxReleases {
			break
		}
	}
	return out
}

func typeRank(t string) int {
	switch strings.ToLower(t) {
	case "album":
		return 0
	case "ep":
		return 1
	case "single":
		return 2
	default:
		return 3
	}
}

func (c *ListenBrainz) tracksFromRelease(rel freshRelease) ([]*models.Track, error) {
	body, err := c.HttpClient.MakeRequest("GET",
		"https://musicbrainz.org/ws/2/release/"+url.PathEscape(rel.ReleaseMbid)+"?inc=recordings+artists&fmt=json",
		nil, nil)
	if err != nil {
		return nil, err
	}
	var mb mbRelease
	if err := util.ParseResp(body, &mb); err != nil {
		return nil, err
	}
	if mb.Status != "" && !strings.EqualFold(mb.Status, "Official") {
		return nil, fmt.Errorf("non-official release (%s)", mb.Status)
	}

	artist := rel.ArtistCreditName
	mainArtist, mainArtistID := artist, ""
	if len(mb.ArtistCredit) > 0 {
		ac := mb.ArtistCredit[0]
		mainArtistID = ac.Artist.ID
		if name := firstNonEmpty(ac.Artist.Name, ac.Name); name != "" {
			mainArtist = name
		}
		if artist == "" {
			artist = mainArtist
		}
	}

	coverID := firstNonEmpty(rel.CaaReleaseMbid, rel.ReleaseMbid)
	coverURL := fmt.Sprintf("https://coverartarchive.org/release/%s/front-250", coverID)

	tracks := make([]*models.Track, 0)
	for _, media := range mb.Media {
		for _, t := range media.Tracks {
			title := firstNonEmpty(t.Recording.Title, t.Title)
			if title == "" {
				continue
			}
			length := t.Recording.Length
			if length == 0 {
				length = t.Length
			}
			tracks = append(tracks, &models.Track{
				Album:                     rel.ReleaseName,
				AlbumArtist:               artist,
				Artist:                    artist,
				MainArtist:                mainArtist,
				MainArtistID:              mainArtistID,
				CleanTitle:                title,
				Title:                     title,
				Duration:                  length,
				CoverURL:                  coverURL,
				OriginalDate:              rel.ReleaseDate,
				ReleaseType:               rel.ReleaseGroupPrimaryType,
				TrackNumber:               t.Position,
				MusicBrainzTrackID:        t.Recording.ID,
				MusicBrainzAlbumID:        rel.ReleaseMbid,
				MusicBrainzReleaseGroupID: rel.ReleaseGroupMbid,
				MusicBrainzArtistID:       mainArtistID,
			})
		}
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no recordings on %s", rel.ReleaseMbid)
	}
	return tracks, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
