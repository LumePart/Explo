package util

import (
	"explo/src/models"
	"fmt"
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
	metadata = addStringTag(metadata, "album_artist", track.AlbumArtist)
	metadata = addStringTag(metadata, "artist-sort", track.ArtistSort)
	metadata = addStringTag(metadata, "date", track.OriginalDate)
	metadata = addStringTag(metadata, "genre", track.Genres)
	metadata = addStringTag(metadata, "TMED", track.Media)
	metadata = addStringTag(metadata, "MusicBrainz Album Type", track.ReleaseType)
	metadata = addStringTag(metadata, "MusicBrainz Album Status", track.ReleaseStatus)
	metadata = addStringTag(metadata, "MusicBrainz Release Group Id", track.MusicBrainzReleaseGroupID)
	metadata = addStringTag(metadata, "MusicBrainz Album Artist Id", track.MusicBrainzAlbumArtistID)
	metadata = addStringTag(metadata, "MusicBrainz Track Id", track.MusicBrainzTrackID)
	metadata = addStringTag(metadata, "MusicBrainz Album Id", track.MusicBrainzAlbumID)
	metadata = addStringTag(metadata, "MusicBrainz Release Track Id", track.MusicBrainzReleaseTrackID)
	metadata = addStringTag(metadata, "MusicBrainz Artist Id", track.MusicBrainzArtistID)

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

	cmd := ffmpeg.Output(streams, filePath, opts).OverWriteOutput().ErrorToStdOut()

	if ffmpegPath != "" {
		cmd.SetFfmpegPath(ffmpegPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}
