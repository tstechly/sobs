package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/apps"
)

type createAppRequest struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Slug               string         `json:"slug"`
	OwnerTeam          string         `json:"ownerTeam"`
	RepoURL            string         `json:"repoUrl"`
	DefaultEnvironment string         `json:"defaultEnvironment"`
	Enabled            *bool          `json:"enabled"`
	Metadata           map[string]any `json:"metadata"`
}

type patchAppRequest struct {
	Name               *string        `json:"name"`
	Slug               *string        `json:"slug"`
	OwnerTeam          *string        `json:"ownerTeam"`
	RepoURL            *string        `json:"repoUrl"`
	DefaultEnvironment *string        `json:"defaultEnvironment"`
	Enabled            *bool          `json:"enabled"`
	Metadata           map[string]any `json:"metadata"`
}

type createReleaseRequest struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	CommitSHA   string         `json:"commitSha"`
	BuildID     string         `json:"buildId"`
	Environment string         `json:"environment"`
	ReleasedAt  string         `json:"releasedAt"`
	Metadata    map[string]any `json:"metadata"`
}

type createArtifactRequest struct {
	ID             string         `json:"id"`
	ArtifactType   string         `json:"artifactType"`
	Name           string         `json:"name"`
	ContentType    string         `json:"contentType"`
	Size           uint64         `json:"size"`
	StorageRef     string         `json:"storageRef"`
	ChecksumSHA256 string         `json:"checksumSha256"`
	Platform       string         `json:"platform"`
	Architecture   string         `json:"architecture"`
	Metadata       map[string]any `json:"metadata"`
	UploadedAt     string         `json:"uploadedAt"`
}

func (s *Server) v1Apps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := s.appService.ListApps()
		query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
		if query != "" {
			filtered := make([]apps.App, 0, len(items))
			for _, item := range items {
				if strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.Slug), query) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		raw := decodeJSONObjectLenient(r)
		var req createAppRequest
		mapIntoStruct(raw, &req)
		item, err := s.appService.CreateApp(apps.CreateAppInput{
			ID:                 strings.TrimSpace(req.ID),
			Name:               strings.TrimSpace(req.Name),
			Slug:               strings.TrimSpace(req.Slug),
			OwnerTeam:          strings.TrimSpace(req.OwnerTeam),
			RepoURL:            strings.TrimSpace(req.RepoURL),
			DefaultEnvironment: strings.TrimSpace(req.DefaultEnvironment),
			Enabled:            req.Enabled,
			Metadata:           req.Metadata,
		})
		if err != nil {
			writeAppsError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
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
			item, ok := s.appService.GetApp(appID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodPatch:
			raw := decodeJSONObjectLenient(r)
			var req patchAppRequest
			mapIntoStruct(raw, &req)
			item, err := s.appService.PatchApp(appID, apps.PatchAppInput{
				Name:               req.Name,
				Slug:               req.Slug,
				OwnerTeam:          req.OwnerTeam,
				RepoURL:            req.RepoURL,
				DefaultEnvironment: req.DefaultEnvironment,
				Enabled:            req.Enabled,
				Metadata:           req.Metadata,
				HasMetadata:        hasJSONField(raw, "metadata"),
			})
			if err != nil {
				writeAppsError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "releases" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.appService.GetApp(appID); !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
				return
			}
			writeJSON(w, http.StatusOK, s.appService.ListReleases(appID))
		case http.MethodPost:
			raw := decodeJSONObjectLenient(r)
			var req createReleaseRequest
			mapIntoStruct(raw, &req)
			item, err := s.appService.CreateRelease(appID, apps.CreateReleaseInput{
				ID:          strings.TrimSpace(req.ID),
				Version:     strings.TrimSpace(req.Version),
				CommitSHA:   strings.TrimSpace(req.CommitSHA),
				BuildID:     strings.TrimSpace(req.BuildID),
				Environment: strings.TrimSpace(req.Environment),
				ReleasedAt:  strings.TrimSpace(req.ReleasedAt),
				Metadata:    req.Metadata,
			})
			if err != nil {
				writeAppsError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, item)
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
		release, ok := s.appService.GetRelease(releaseID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"release":   release,
			"artifacts": s.appService.ListArtifacts(releaseID),
		})
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := s.appService.GetRelease(releaseID); !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found"})
			return
		}
		writeJSON(w, http.StatusOK, s.appService.ListArtifacts(releaseID))
		return
	}

	if len(parts) == 3 && parts[1] == "artifacts" && parts[2] == "meta" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		raw := decodeJSONObjectLenient(r)
		var req createArtifactRequest
		mapIntoStruct(raw, &req)
		item, err := s.appService.CreateArtifact(releaseID, apps.CreateArtifactInput{
			ID:             strings.TrimSpace(req.ID),
			ArtifactType:   strings.TrimSpace(req.ArtifactType),
			Name:           strings.TrimSpace(req.Name),
			ContentType:    strings.TrimSpace(req.ContentType),
			Size:           req.Size,
			StorageRef:     strings.TrimSpace(req.StorageRef),
			ChecksumSHA256: strings.TrimSpace(req.ChecksumSHA256),
			Platform:       strings.TrimSpace(req.Platform),
			Architecture:   strings.TrimSpace(req.Architecture),
			Metadata:       req.Metadata,
			UploadedAt:     strings.TrimSpace(req.UploadedAt),
		})
		if err != nil {
			writeAppsError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
		return
	}

	http.NotFound(w, r)
}

func decodeJSONObjectLenient(r *http.Request) map[string]any {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return map[string]any{}
	}
	return payload
}

func mapIntoStruct(raw map[string]any, target any) {
	blob, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(blob, target)
}

func hasJSONField(raw map[string]any, key string) bool {
	_, ok := raw[key]
	return ok
}

func writeAppsError(w http.ResponseWriter, err error) {
	switch err {
	case nil:
		return
	case apps.ErrNameRequired, apps.ErrVersionRequired, apps.ErrArtifactFieldsRequired:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case apps.ErrAppSlugAlreadyExists:
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case apps.ErrAppNotFound, apps.ErrReleaseNotFound, apps.ErrNotFound:
		status := http.StatusNotFound
		writeJSON(w, status, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}