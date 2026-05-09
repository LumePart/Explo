package downloader

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	cfg "explo/src/config"
	"explo/src/models"
)

type playlistManifest struct {
	Version  int      `json:"version"`
	Playlist string   `json:"playlist"`
	Files    []string `json:"files"`
}

type orphanCleanupResult struct {
	Scanned    int
	Referenced int
	Removed    int
	Skipped    int
}

func playlistManifestCleanupRequired(downloadCfg *cfg.DownloadConfig) bool {
	return downloadCfg.UseSubDir && !downloadSubdirectoryFormatHasPlaylistRoot(downloadCfg.DownloadSubdirectoryFormat)
}

func cleanupOrphanDownloads(downloadCfg *cfg.DownloadConfig) (orphanCleanupResult, error) {
	var result orphanCleanupResult

	if strings.TrimSpace(downloadCfg.DownloadDir) == "" {
		return result, fmt.Errorf("DOWNLOAD_DIR is empty")
	}

	referencedFiles, err := referencedManifestFiles(downloadCfg)
	if err != nil {
		return result, err
	}
	result.Referenced = len(referencedFiles)

	downloadRoot, err := filepath.Abs(downloadCfg.DownloadDir)
	if err != nil {
		return result, err
	}
	manifestDir, err := filepath.Abs(downloadCfg.PlaylistManifestDir)
	if err != nil {
		return result, err
	}
	skipManifestDir := strings.TrimSpace(downloadCfg.PlaylistManifestDir) != ""

	err = filepath.WalkDir(downloadCfg.DownloadDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipManifestDir && samePathOrInside(path, manifestDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			result.Skipped++
			return nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(downloadRoot, absPath)
		if err != nil {
			return err
		}
		if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
			return fmt.Errorf("refusing to remove path outside download directory: %s", path)
		}

		rel = filepath.ToSlash(rel)
		if _, ok := referencedFiles[rel]; ok {
			result.Scanned++
			return nil
		}

		fullPath, err := safeDownloadPath(downloadCfg.DownloadDir, rel)
		if err != nil {
			return err
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		removeEmptyParents(filepath.Dir(fullPath), downloadCfg.DownloadDir)
		result.Scanned++
		result.Removed++
		return nil
	})
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}

func (c *DownloadClient) writePlaylistManifest(tracks []*models.Track, filesBeforeDownload map[string]struct{}) error {
	files := make([]string, 0)

	for _, track := range tracks {
		if !track.Present || track.File == "" {
			continue
		}

		rel, err := downloadedTrackRelPath(c.Cfg, track)
		if err != nil {
			return err
		}
		if _, existedBefore := filesBeforeDownload[rel]; existedBefore || filesBeforeDownload == nil {
			referenced, err := fileReferencedByAnyPlaylistManifest(c.Cfg, rel)
			if err != nil {
				return err
			}
			if !referenced {
				continue
			}
		}
		files = append(files, rel)
	}

	manifest := playlistManifest{
		Version:  1,
		Playlist: c.Cfg.PlaylistName,
		Files:    files,
	}

	path := playlistManifestPath(c.Cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func snapshotDownloadFiles(downloadDir string) (map[string]struct{}, error) {
	files := make(map[string]struct{})

	err := filepath.WalkDir(downloadDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(downloadDir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if os.IsNotExist(err) {
		return files, nil
	}
	return files, err
}

func (c *DownloadClient) deletePlaylistManifestFiles(downloadCfg *cfg.DownloadConfig) error {
	path := playlistManifestPath(downloadCfg)
	manifest, err := readPlaylistManifest(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, relPath := range manifest.Files {
		referenced, err := fileReferencedByOtherPlaylistManifests(downloadCfg, path, relPath)
		if err != nil {
			slog.Warn("skipping playlist manifest cleanup because references could not be checked", "file", relPath, "context", err.Error())
			continue
		}
		if referenced {
			continue
		}

		fullPath, err := safeDownloadPath(downloadCfg.DownloadDir, relPath)
		if err != nil {
			slog.Warn("skipping invalid playlist manifest path", "file", relPath, "context", err.Error())
			continue
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove playlist manifest download", "file", relPath, "context", err.Error())
			continue
		}
		removeEmptyParents(filepath.Dir(fullPath), downloadCfg.DownloadDir)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func playlistManifestPath(downloadCfg *cfg.DownloadConfig) string {
	manifestName := downloadCfg.PlaylistType
	if strings.TrimSpace(manifestName) == "" {
		manifestName = downloadCfg.PlaylistName
	}
	return filepath.Join(downloadCfg.PlaylistManifestDir, folderPart(manifestName, "playlist")+".json")
}

func readPlaylistManifest(path string) (playlistManifest, error) {
	var manifest playlistManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func fileReferencedByOtherPlaylistManifests(downloadCfg *cfg.DownloadConfig, currentManifestPath, relPath string) (bool, error) {
	return fileReferencedByPlaylistManifests(downloadCfg, currentManifestPath, relPath)
}

func fileReferencedByAnyPlaylistManifest(downloadCfg *cfg.DownloadConfig, relPath string) (bool, error) {
	return fileReferencedByPlaylistManifests(downloadCfg, "", relPath)
}

func fileReferencedByPlaylistManifests(downloadCfg *cfg.DownloadConfig, currentManifestPath, relPath string) (bool, error) {
	manifestDir := filepath.Dir(playlistManifestPath(downloadCfg))
	entries, err := os.ReadDir(manifestDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	cleanCurrent := ""
	if currentManifestPath != "" {
		cleanCurrent, err = filepath.Abs(currentManifestPath)
		if err != nil {
			return false, err
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(manifestDir, entry.Name())
		cleanPath, err := filepath.Abs(path)
		if err != nil {
			return false, err
		}
		if cleanCurrent != "" && cleanPath == cleanCurrent {
			continue
		}

		manifest, err := readPlaylistManifest(path)
		if err != nil {
			return false, err
		}
		for _, file := range manifest.Files {
			if filepath.ToSlash(filepath.Clean(filepath.FromSlash(file))) == filepath.ToSlash(filepath.Clean(filepath.FromSlash(relPath))) {
				return true, nil
			}
		}
	}
	return false, nil
}

func referencedManifestFiles(downloadCfg *cfg.DownloadConfig) (map[string]struct{}, error) {
	files := make(map[string]struct{})
	if strings.TrimSpace(downloadCfg.PlaylistManifestDir) == "" {
		return files, nil
	}

	manifestDir := filepath.Dir(playlistManifestPath(downloadCfg))
	entries, err := os.ReadDir(manifestDir)
	if os.IsNotExist(err) {
		return files, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		manifest, err := readPlaylistManifest(filepath.Join(manifestDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		for _, file := range manifest.Files {
			relPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))
			if _, err := safeDownloadPath(downloadCfg.DownloadDir, relPath); err != nil {
				slog.Warn("skipping invalid playlist manifest path", "file", file, "context", err.Error())
				continue
			}
			files[relPath] = struct{}{}
		}
	}
	return files, nil
}

func samePathOrInside(path, dir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
