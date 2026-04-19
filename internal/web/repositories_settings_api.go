package web

import (
	"net/http"
	"net/url"
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
		if r.URL.Path != "/settings/repositories" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}

		aiSettings := buildAISettingsForTemplate(s.settingsService.AI())
		_, expiryStatus := githubTokenExpiryStatus(aiSettings)
		githubToken := pickSetting(aiSettings, "ai.github_token", "github_token")
		defaultAgentRepo := pickSetting(aiSettings, "ai.github_repo", "github_repo")

		repos := s.repositoryService.List()
		apps := make([]map[string]any, 0, len(repos))
		realtimeSeed := map[string]any{}
		for _, repo := range repos {
			owner, name := parseGithubOwnerRepo(repo.URL)
			slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(repo.Name), " ", "-"))
			if slug == "" {
				slug = strings.ToLower(strings.TrimSpace(repo.ID))
			}
			expiryMessage := "No CI ingest key configured"
			expiryState := "unknown"
			if strings.HasPrefix(strings.TrimSpace(repo.CIIngestKeyHash), "scrypt:v1:") || strings.TrimSpace(repo.CIIngestKey) != "" {
				expiryMessage = "CI ingest key configured"
				expiryState = "healthy"
			}
			app := map[string]any{
				"id":                    repo.ID,
				"name":                  repo.Name,
				"slug":                  slug,
				"repo_url":              repo.URL,
				"repo_owner":            owner,
				"repo_name":             name,
				"release_count":         len(repo.Releases),
				"latest_versions":       trimTo(repo.Releases, 3),
				"enabled":               true,
				"repo_token_configured": false,
				"ci_push_plain":         "",
				"ci_push_status": map[string]any{
					"realtime_enabled": repo.Realtime,
					"configured":       strings.HasPrefix(strings.TrimSpace(repo.CIIngestKeyHash), "scrypt:v1:") || strings.TrimSpace(repo.CIIngestKey) != "",
					"expiry": map[string]any{
						"state":   expiryState,
						"message": expiryMessage,
					},
				},
			}
			apps = append(apps, app)
			realtimeSeed[repo.ID] = map[string]any{"realtime_enabled": repo.Realtime}
		}

		ctx := map[string]any{
			"title":                            "GitHub Repositories",
			"mobile_breakpoint_max":            "575.98px",
			"request":                          map[string]any{"endpoint": "settings/repositories"},
			"apps":                             apps,
			"realtime_seed":                    realtimeSeed,
			"github_token_configured":          strings.TrimSpace(githubToken) != "",
			"default_agent_repo":               defaultAgentRepo,
			"github_token_expiry_status":       expiryStatus,
			"github_token_expiry_warning_days": 14,
			"github_token_validation_status": map[string]any{
				"status":            pickSetting(aiSettings, "ai.github_token_validation_status", "github_token_validation_status"),
				"last_validated_at": pickSetting(aiSettings, "ai.github_token_last_validated_at", "github_token_last_validated_at"),
				"message":           pickSetting(aiSettings, "ai.github_token_validation_message", "github_token_validation_message"),
			},
			"ci_push_default_ttl_days": 30,
			"ci_push_max_ttl_days":     365,
		}
		s.renderTemplate(w, "settings_repositories.html", ctx)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		repo, err := s.repositoryService.Create(strings.TrimSpace(vals["name"]), strings.TrimSpace(vals["url"]))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, repo)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseGithubOwnerRepo(raw string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	return owner, repo
}

func trimTo(items []string, n int) []string {
	if n <= 0 || len(items) == 0 {
		return []string{}
	}
	if len(items) <= n {
		out := make([]string, len(items))
		copy(out, items)
		return out
	}
	out := make([]string, n)
	copy(out, items[:n])
	return out
}

func (s *Server) settingsRepositoriesValidateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": repositories.ValidateGitHubToken(vals["token"])})
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
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		repo, ok := s.repositoryService.Update(id, strings.TrimSpace(vals["name"]), strings.TrimSpace(vals["url"]))
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
		enabled := false
		if vals, err := decodeStringMap(r); err == nil {
			enabled = parseBool(vals["enabled"])
		}
		repo, ok := s.repositoryService.SetRealtime(id, enabled)
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
		repo, keyPlain, ok := s.repositoryService.RotateCIIngestKey(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"repository": repo, "ci_ingest_key": keyPlain})
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
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		repo, ok := s.repositoryService.AddRelease(id, strings.TrimSpace(vals["release"]))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, repo)
		return
	}

	http.NotFound(w, r)
}
