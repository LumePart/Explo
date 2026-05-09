package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cfg "explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

func cleanupDownloadDir(downloadCfg *cfg.DownloadConfig) string {
	if !downloadCfg.UseSubDir {
		return downloadCfg.DownloadDir
	}

	if downloadSubdirectoryFormatHasPlaylistRoot(downloadCfg.DownloadSubdirectoryFormat) {
		root := renderDownloadSubdirectoryPart(firstSubdirectoryFormatPart(downloadCfg.DownloadSubdirectoryFormat), downloadCfg.PlaylistName, nil)
		root = folderPart(root, "")
		if root != "" {
			return filepath.Join(downloadCfg.DownloadDir, root)
		}
	}

	if !subdirectoryFormatUsesTrackFields(downloadCfg.DownloadSubdirectoryFormat) {
		return trackDownloadDir(downloadCfg, nil)
	}

	return downloadCfg.DownloadDir
}

func subdirectoryFormatUsesTrackFields(format string) bool {
	return strings.Contains(format, "{artist}") ||
		strings.Contains(format, "{album}")
}

func downloadSubdirectoryFormatHasPlaylistRoot(format string) bool {
	root := firstSubdirectoryFormatPart(format)
	return strings.Contains(root, "{playlist}") &&
		!subdirectoryFormatUsesTrackFields(root)
}

func firstSubdirectoryFormatPart(format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		format = "{playlist}"
	}

	parts := strings.FieldsFunc(format, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			return part
		}
	}
	return "{playlist}"
}

func trackDownloadDir(downloadCfg *cfg.DownloadConfig, track *models.Track) string {
	if !downloadCfg.UseSubDir {
		return downloadCfg.DownloadDir
	}

	subdir := renderDownloadSubdirectoryFormat(downloadCfg.DownloadSubdirectoryFormat, downloadCfg.PlaylistName, track)
	if subdir == "" {
		return downloadCfg.DownloadDir
	}

	return filepath.Join(downloadCfg.DownloadDir, subdir)
}

func downloadedTrackRelPath(downloadCfg *cfg.DownloadConfig, track *models.Track) (string, error) {
	fullPath := track.File
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(trackDownloadDir(downloadCfg, track), track.File)
	}
	relPath, err := filepath.Rel(downloadCfg.DownloadDir, fullPath)
	if err != nil {
		return "", err
	}
	if _, err := safeDownloadPath(downloadCfg.DownloadDir, relPath); err != nil {
		return "", err
	}
	return filepath.ToSlash(relPath), nil
}

func renderDownloadSubdirectoryFormat(format, playlist string, track *models.Track) string {
	format = strings.TrimSpace(format)
	if format == "" {
		format = "{playlist}"
	}

	parts := strings.FieldsFunc(format, func(r rune) bool {
		return r == '/' || r == '\\'
	})

	safeParts := make([]string, 0, len(parts))
	for _, part := range parts {
		rendered := renderDownloadSubdirectoryPart(part, playlist, track)
		safePart := folderPart(rendered, "")
		if safePart != "" {
			safeParts = append(safeParts, safePart)
		}
	}

	return filepath.Join(safeParts...)
}

func renderDownloadSubdirectoryPart(part, playlist string, track *models.Track) string {
	replacer := strings.NewReplacer(
		"{playlist}", folderValue(playlist, "Playlist"),
		"{artist}", trackFolderValue(track, "artist"),
		"{album}", trackFolderValue(track, "album"),
	)
	return replacer.Replace(part)
}

func trackFolderValue(track *models.Track, field string) string {
	if track == nil {
		return ""
	}

	switch field {
	case "artist":
		if track.MainArtist != "" {
			return track.MainArtist
		}
		return folderValue(track.Artist, "Unknown Artist")
	case "album":
		return folderValue(track.Album, "Unknown Album")
	default:
		return ""
	}
}

func folderValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func folderPart(value, fallback string) string {
	part := strings.Trim(util.FilenameSafe(strings.TrimSpace(value)), " ._")
	if part == "" {
		return fallback
	}
	return part
}

func safeDownloadPath(downloadDir, relPath string) (string, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes download directory")
	}

	fullPath := filepath.Join(downloadDir, cleanRel)
	absRoot, err := filepath.Abs(downloadDir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes download directory")
	}
	return fullPath, nil
}

func removeEmptyParents(dir, stopDir string) {
	absStop, err := filepath.Abs(stopDir)
	if err != nil {
		return
	}
	for {
		absDir, err := filepath.Abs(dir)
		if err != nil || absDir == absStop {
			return
		}
		if rel, err := filepath.Rel(absStop, absDir); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
