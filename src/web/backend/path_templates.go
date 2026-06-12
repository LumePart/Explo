package backend

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// PathTemplatePreset is a named folder-structure template saved by the user.
type PathTemplatePreset struct {
	Name     string `json:"name"`
	Template string `json:"template"`
}

func pathTemplatesFilePath(cfgDir string) string {
	return filepath.Join(cfgDir, "path-templates.json")
}

func loadPathTemplates(cfgDir string) []PathTemplatePreset {
	data, err := os.ReadFile(pathTemplatesFilePath(cfgDir))
	if err != nil {
		return nil
	}
	var out []PathTemplatePreset
	if err := json.Unmarshal(data, &out); err != nil {
		slog.Warn("path-templates: failed to parse", "err", err)
		return nil
	}
	return out
}

func savePathTemplates(cfgDir string, presets []PathTemplatePreset) error {
	raw, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pathTemplatesFilePath(cfgDir), raw, 0644)
}

// handlePathTemplates handles GET and POST for /api/ui/path-templates.
func (s *Server) handlePathTemplates(w http.ResponseWriter, r *http.Request) {
	cfgDir := s.cfg.WebDataDir
	switch r.Method {
	case http.MethodGet:
		presets := loadPathTemplates(cfgDir)
		if presets == nil {
			presets = []PathTemplatePreset{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(presets); err != nil {
			slog.Error("failed encoding path templates", "err", err.Error())
		}
	case http.MethodPost:
		var body PathTemplatePreset
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.Name == "" || body.Template == "" {
			http.Error(w, "name and template are required", http.StatusBadRequest)
			return
		}
		presets := loadPathTemplates(cfgDir)
		presets = append(presets, body)
		if err := savePathTemplates(cfgDir, presets); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.Error("failed encoding path template", "err", err.Error())
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeletePathTemplate handles DELETE /api/ui/path-templates/{name}.
func (s *Server) handleDeletePathTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/ui/path-templates/")
	name, err := url.PathUnescape(raw)
	if err != nil || name == "" {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	cfgDir := s.cfg.WebDataDir
	presets := loadPathTemplates(cfgDir)
	filtered := presets[:0]
	for _, p := range presets {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(presets) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := savePathTemplates(cfgDir, filtered); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
