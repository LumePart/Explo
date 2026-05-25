package util

import (
	"explo/src/models"
	"fmt"
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

	metadata = addStringTag(metadata, "artist", track.Artist)
	metadata = addStringTag(metadata, "title", track.Title)
	metadata = addStringTag(metadata, "album", track.Album)
	metadata = addStringTag(metadata, "album_artist", track.AlbumArtist)
	metadata = addStringTag(metadata, "artist-sort", track.ArtistSort)
	metadata = addStringTag(metadata, "ORIGINALDATE", track.OriginalDate)
	metadata = addStringTag(metadata, "RELEASECOUNTRY", track.ReleaseCountry)
	metadata = addStringTag(metadata, "RELEASESTATUS", track.ReleaseStatus)
	metadata = addStringTag(metadata, "RELEASETYPE", track.ReleaseType)
	metadata = addStringTag(metadata, "genre", track.Genres)
	metadata = addStringTag(metadata, "media", track.Media)
	metadata = addStringTag(metadata, "MUSICBRAINZ_RELEASEGROUPID", track.MusicBrainzReleaseGroupID)
	metadata = addStringTag(metadata, "MUSICBRAINZ_ALBUMARTISTID", track.MusicBrainzAlbumArtistID)
	metadata = addStringTag(metadata, "MUSICBRAINZ_TRACKID", track.MusicBrainzTrackID)
	metadata = addStringTag(metadata, "MUSICBRAINZ_ALBUMID", track.MusicBrainzAlbumID)
	metadata = addStringTag(metadata, "MUSICBRAINZ_RELEASETRACKID", track.MusicBrainzReleaseTrackID)
	metadata = addStringTag(metadata, "MUSICBRAINZ_ARTISTID", track.MusicBrainzArtistID)

	metadata = addIntTag(metadata, "ORIGINALYEAR", track.OriginalYear)
	metadata = addIntTag(metadata, "TRACKNUMBER", track.TrackNumber)
	metadata = addIntTag(metadata, "TRACKTOTAL", track.TrackTotal)
	metadata = addIntTag(metadata, "DISCNUMBER", track.DiscNumber)
	metadata = addIntTag(metadata, "DISCTOTAL", track.DiscTotal)

	for _, isrc := range track.ISRCs {
		if isrc != "" {
			metadata = append(metadata, "ISRC="+isrc)
		}
	}

	return metadata
}
