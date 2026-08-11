package util

import (
	"explo/src/models"
	"fmt"
	"log/slog"
	"strings"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

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

func BuildffmpegMetadata(track models.Track) []string {
	metadata := []string{}

	if len(track.Artists) > 0 {
		metadata = append(metadata, "artist="+strings.Join(track.Artists, "; "))
	} else {
		metadata = addStringTag(metadata, "artist", track.Artist)
	}

	metadata = addStringTag(metadata, "title", track.Title)
	metadata = addStringTag(metadata, "album", track.Album)

	albumArtist := track.AlbumArtist
	if albumArtist == "" && len(track.Artists) > 0 {
		albumArtist = track.Artists[0]
	}
	if albumArtist == "" {
		albumArtist = track.MainArtist
	}
	if albumArtist == "" {
		albumArtist = track.Artist
	}
	metadata = addStringTag(metadata, "albumartist", albumArtist)
	metadata = addStringTag(metadata, "artistsort", track.ArtistSort)
	metadata = addStringTag(metadata, "date", track.OriginalDate)
	metadata = addStringTag(metadata, "genre", track.Genres)
	metadata = addStringTag(metadata, "TMED", track.Media)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumType", track.ReleaseType)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumStatus", track.ReleaseStatus)
	metadata = addStringTag(metadata, "MusicBrainz_ReleaseGroupId", track.MusicBrainzReleaseGroupID)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumArtistId", track.MusicBrainzAlbumArtistID)
	metadata = addStringTag(metadata, "MusicBrainz_TrackId", track.MusicBrainzTrackID)
	metadata = addStringTag(metadata, "MusicBrainz_AlbumId", track.MusicBrainzAlbumID)
	metadata = addStringTag(metadata, "MusicBrainz_ReleaseTrackId", track.MusicBrainzReleaseTrackID)
	metadata = addStringTag(metadata, "MusicBrainz_ArtistId", track.MusicBrainzArtistID)

	metadata = addIntTag(metadata, "originalyear", track.OriginalYear)
	metadata = addIntTag(metadata, "track", track.TrackNumber)
	metadata = addIntTag(metadata, "Tracktotal", track.TrackTotal)
	metadata = addIntTag(metadata, "disc", track.DiscNumber)
	metadata = addIntTag(metadata, "Disctotal", track.DiscTotal)

	for _, isrc := range track.ISRCs {
		metadata = addStringTag(metadata, "ISRC", isrc)
	}

	return metadata
}

func WriteMetadata(streams []*ffmpeg.Stream, ffmpegPath, filePath string, opts ffmpeg.KwArgs) error {

	cmd := ffmpeg.Output(streams, filePath, opts).OverWriteOutput().ErrorToStdOut().Silent(true)

	slog.Debug("ffmpeg command", "args", "ffmpeg "+strings.Join(cmd.GetArgs(), " "))

	if ffmpegPath != "" {
		cmd.SetFfmpegPath(ffmpegPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}
