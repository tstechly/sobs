package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/repositories"
)

type createRepositoryRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type updateRepositoryRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type validateTokenRequest struct {
	Token string `json:"token"`
}

type realtimeModeRequest struct {
	Enabled bool `json:"enabled"`
}

type releaseRequest struct {
	Release string `json:"release"`
}

func (s *Server) settingsRepositories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"items": s.repositoryService.List()})
			return
		}
		s.pageTemplateHandler("/settings/repositories", "settings_repositories.html")(w, r)
	case http.MethodPost:
		var req createRepositoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		repo, err := s.repositoryService.Create(strings.TrimSpace(req.Name), strings.TrimSpace(req.URL))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, repo)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) settingsRepositoriesValidateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req validateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": repositories.ValidateGitHubToken(req.Token)})
}

func (s *Server) settingsRepositoriesSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/settings/repositories/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req updateRepositoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		repo, ok := s.repositoryService.Update(id, strings.TrimSpace(req.Name), strings.TrimSpace(req.URL))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	if len(parts) == 2 && parts[1] == "delete" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.repositoryService.Delete(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
		return
	}

	if len(parts) == 2 && parts[1] == "realtime-mode" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req realtimeModeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		repo, ok := s.repositoryService.SetRealtime(id, req.Enabled)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	if len(parts) == 3 && parts[1] == "ci-ingest-key" && parts[2] == "rotate" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repo, ok := s.repositoryService.RotateCIIngestKey(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	if len(parts) == 3 && parts[1] == "ci-ingest-key" && parts[2] == "revoke" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		repo, ok := s.repositoryService.RevokeCIIngestKey(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	if len(parts) == 2 && parts[1] == "releases" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req releaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		repo, ok := s.repositoryService.AddRelease(id, strings.TrimSpace(req.Release))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	http.NotFound(w, r)
}
