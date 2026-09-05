package util

import (
	"explo/src/models"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)


type metadataTags struct {
	Title             string
	Album             string
	Artist            string
	AlbumArtist       string
	ArtistSort        string
	Date              string
	Media             string
	ReleaseType       string
	ReleaseStatus     string
	ReleaseGroupID    string
	AlbumArtistID     string
	TrackID           string
	AlbumID            string
	ReleaseTrackID    string
	ArtistID           string
	OriginalYear      string
	TrackNumber       string
	TrackTotal        string
	DiscNumber        string
	DiscTotal         string
}

// Return absolute difference between tracks
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func addStringTag(metadata []string, key string, value string) []string {
	if value != "" {
		metadata = append(metadata, key+"="+value)
	}
	return metadata
}

func addIntTag(metadata []string, key string, value int) []string {
	if value != 0 {
		metadata = append(metadata, fmt.Sprintf("%s=%d", key, value))
	}
	return metadata
}

func buildMetadata(track models.Track, tags metadataTags) []string {
	metadata := []string{}

	artist := track.Artist
	if len(track.Artists) > 0 {
		artist = strings.Join(track.Artists, "; ")
	}

	metadata = addStringTag(metadata, tags.Artist, artist)
	metadata = addStringTag(metadata, tags.Title, track.Title)
	metadata = addStringTag(metadata, tags.Album, track.Album)
	metadata = addStringTag(metadata, tags.AlbumArtist, track.AlbumArtist)
	metadata = addStringTag(metadata, tags.ArtistSort, track.ArtistSort)
	metadata = addStringTag(metadata, tags.Date, track.OriginalDate)
	metadata = addStringTag(metadata, "genre", track.Genres)
	metadata = addStringTag(metadata, tags.Media, track.Media)

	metadata = addStringTag(metadata, tags.ReleaseType, track.ReleaseType)
	metadata = addStringTag(metadata, tags.ReleaseStatus, track.ReleaseStatus)
	metadata = addStringTag(metadata, tags.ReleaseGroupID, track.MusicBrainzReleaseGroupID)
	metadata = addStringTag(metadata, tags.AlbumArtistID, track.MusicBrainzAlbumArtistID)
	metadata = addStringTag(metadata, tags.TrackID, track.MusicBrainzTrackID)
	metadata = addStringTag(metadata, tags.AlbumID, track.MusicBrainzAlbumID)
	metadata = addStringTag(metadata, tags.ReleaseTrackID, track.MusicBrainzReleaseTrackID)
	metadata = addStringTag(metadata, tags.ArtistID, track.MusicBrainzArtistID)

	metadata = addIntTag(metadata, tags.OriginalYear, track.OriginalYear)
	metadata = addIntTag(metadata, tags.TrackNumber, track.TrackNumber)
	metadata = addIntTag(metadata, tags.TrackTotal, track.TrackTotal)
	metadata = addIntTag(metadata, tags.DiscNumber, track.DiscNumber)
	metadata = addIntTag(metadata, tags.DiscTotal, track.DiscTotal)

	for _, isrc := range track.ISRCs {
		metadata = addStringTag(metadata, "ISRC", isrc)
	}

	return metadata
}


func basicTags() metadataTags {
	return metadataTags{
		Title:       "title",
        Album:       "album",
        Artist:      "artist",
        AlbumArtist: "album_artist",
        ArtistSort:  "artist-sort",
        Date:        "date",
        TrackNumber: "track",
        DiscNumber:  "disc",
    }
}

func id3Tags() metadataTags {
	return metadataTags{
		Title:          "title",
		Album:          "album",
		Artist:         "artist",
		AlbumArtist:    "album_artist",
		ArtistSort:     "artist-sort",
		Date:           "date",
		Media:          "TMED",
		ReleaseType:    "MusicBrainz Album Type",
		ReleaseStatus:  "MusicBrainz Album Status",
		ReleaseGroupID: "MusicBrainz Release Group Id",
		AlbumArtistID:  "MusicBrainz Album Artist Id",
		TrackID:        "MusicBrainz Track Id",
		AlbumID:        "MusicBrainz Album Id",
		ReleaseTrackID: "MusicBrainz Release Track Id",
		ArtistID:       "MusicBrainz Artist Id",
		OriginalYear:   "originalyear",
		TrackNumber:    "track",
		TrackTotal:     "Tracktotal",
		DiscNumber:     "disc",
		DiscTotal:      "Disctotal",
	}
}

func vorbisTags() metadataTags {
	return metadataTags{
		Title:          "title",
		Album:          "album",
		Artist:         "artist",
		AlbumArtist:    "albumartist",
		ArtistSort:     "artistsort",
		Date:           "date",
		Media:          "Media",
		ReleaseType:    "ReleaseType",
		ReleaseStatus:  "ReleaseStatus",
		ReleaseGroupID: "MusicBrainz_ReleaseGroupId",
		AlbumArtistID:  "MusicBrainz_AlbumArtistId",
		TrackID:        "MusicBrainz_TrackId",
		AlbumID:        "MusicBrainz_AlbumId",
		ReleaseTrackID: "MusicBrainz_ReleaseTrackId",
		ArtistID:       "MusicBrainz_ArtistId",
		OriginalYear:   "originalyear",
		TrackNumber:    "track",
		TrackTotal:     "Tracktotal",
		DiscNumber:     "discnumber",
		DiscTotal:      "Disctotal",
	}
}

func apeTags() metadataTags {
	return metadataTags{
		Title:          "title",
		Album:          "album",
		Artist:         "artist",
		AlbumArtist:    "albumartist",
		ArtistSort:     "artistsort",
		Date:           "Year",
		Media:          "Media",
		ReleaseType:    "MusicBrainz_AlbumType",
		ReleaseStatus:  "MusicBrainz_AlbumStatus",
		ReleaseGroupID: "MusicBrainz_ReleaseGroupId",
		AlbumArtistID:  "MusicBrainz_AlbumArtistId",
		TrackID:        "MusicBrainz_TrackId",
		AlbumID:        "MusicBrainz_AlbumId",
		ReleaseTrackID: "MusicBrainz_ReleaseTrackId",
		ArtistID:       "MusicBrainz_ArtistId",
		OriginalYear:   "originalyear",
		TrackNumber:    "track",
		DiscNumber:     "disc",
	}
}


func BuildffmpegMetadata(track models.Track) []string {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(track.File)), ".")
	switch strings.ToLower(extension) {
	case "mp3":
		return buildMetadata(track, id3Tags())

	case "flac", "opus":
		return buildMetadata(track, vorbisTags())

	case "ape", "wv", "mpc":
		return buildMetadata(track, apeTags())

	default:
		slog.Debug("using basic metadata mapping", "ext", extension)
		return buildMetadata(track, basicTags())

	}
}

func WriteMetadata(streams []*ffmpeg.Stream, ffmpegPath, filePath string, opts ffmpeg.KwArgs) error {

	cmd := ffmpeg.Output(streams, filePath, opts).OverWriteOutput().ErrorToStdOut()

	if ffmpegPath != "" {
		cmd.SetFfmpegPath(ffmpegPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}