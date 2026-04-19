package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type createAppRequest struct {
	Name string `json:"name"`
}

type patchAppRequest struct {
	Name *string `json:"name"`
}

type createReleaseRequest struct {
	Version string `json:"version"`
}

type createArtifactRequest struct {
	ArtifactType   string         `json:"artifactType"`
	Name           string         `json:"name"`
	ContentType    string         `json:"contentType"`
	Size           uint64         `json:"size"`
	StorageRef     string         `json:"storageRef"`
	ChecksumSHA256 string         `json:"checksumSha256"`
	Platform       string         `json:"platform"`
	Architecture   string         `json:"architecture"`
	Metadata       map[string]any `json:"metadata"`
}

func (s *Server) v1Apps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.appService.ListApps()})
	case http.MethodPost:
		var req createAppRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		a, err := s.appService.CreateApp(strings.TrimSpace(req.Name))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, a)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) v1AppsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/apps/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	appID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			a, ok := s.appService.GetApp(appID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, a)
		case http.MethodPatch:
			var req patchAppRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			a, err := s.appService.PatchApp(appID, req.Name)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, a)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "releases" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.appService.GetApp(appID); !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": s.appService.ListReleases(appID)})
		case http.MethodPost:
			var req createReleaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			rls, err := s.appService.CreateRelease(appID, strings.TrimSpace(req.Version))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, rls)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) v1ReleasesSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/releases/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	releaseID := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rls, ok := s.appService.GetRelease(releaseID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, rls)
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.appService.GetRelease(releaseID); !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": s.appService.ListArtifacts(releaseID)})
			return
		case http.MethodPost:
			var req createArtifactRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
			item, err := s.appService.CreateArtifact(
				releaseID,
				strings.TrimSpace(req.ArtifactType),
				strings.TrimSpace(req.Name),
				strings.TrimSpace(req.ContentType),
				req.Size,
				strings.TrimSpace(req.StorageRef),
				strings.TrimSpace(req.ChecksumSHA256),
				strings.TrimSpace(req.Platform),
				strings.TrimSpace(req.Architecture),
				req.Metadata,
			)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, item)
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	if len(parts) == 3 && parts[1] == "artifacts" && parts[2] == "meta" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if _, ok := s.appService.GetRelease(releaseID); !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		var req createArtifactRequest
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &req)
		}
		if strings.TrimSpace(req.ArtifactType) == "" || strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "meta": raw})
			return
		}
		item, err := s.appService.CreateArtifact(
			releaseID,
			strings.TrimSpace(req.ArtifactType),
			strings.TrimSpace(req.Name),
			strings.TrimSpace(req.ContentType),
			req.Size,
			strings.TrimSpace(req.StorageRef),
			strings.TrimSpace(req.ChecksumSHA256),
			strings.TrimSpace(req.Platform),
			strings.TrimSpace(req.Architecture),
			req.Metadata,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "item": item})
		return
	}

	http.NotFound(w, r)
}
