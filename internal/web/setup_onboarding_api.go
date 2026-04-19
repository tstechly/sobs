package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

var wizardEnvs = map[string]bool{"dev": true, "prod": true}
var wizardLanguages = map[string]bool{"python": true, "node": true, "go": true, "java": true, "dotnet": true, "ruby": true, "php": true}
var wizardDeployments = map[string]bool{"docker": true, "kubernetes": true, "baremetal": true, "cloud": true}

func (s *Server) apiSetupWizardSteps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	env := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("env")))
	if env == "" {
		env = "dev"
	}
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("language")))
	if lang == "" {
		lang = "python"
	}
	dep := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("deployment")))
	if dep == "" {
		dep = "docker"
	}
	if !wizardEnvs[env] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid env '" + env + "'. Must be one of: [dev prod]"})
		return
	}
	if !wizardLanguages[lang] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid language '" + lang + "'. Must be one of: [dotnet go java node php python ruby]"})
		return
	}
	if !wizardDeployments[dep] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid deployment '" + dep + "'. Must be one of: [baremetal cloud docker kubernetes]"})
		return
	}
	steps := []map[string]any{
		{"id": "sdk_install", "title": "Install OTEL SDK", "commands": []string{"install sdk for " + lang}},
		{"id": "collector", "title": "Run OTEL Collector", "commands": []string{"deployment=" + dep}},
		{"id": "verify", "title": "Verify in SOBS", "commands": []string{"open /"}},
	}
	checklist := []map[string]any{{"id": "sdk", "label": "Install & initialise the SDK"}, {"id": "collector", "label": "Run the OpenTelemetry Collector"}, {"id": "verify", "label": "Verify data in SOBS"}}
	if env == "prod" {
		checklist = append(checklist, map[string]any{"id": "anomaly", "label": "Configure anomaly detection"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "1", "env": env, "language": lang, "deployment": dep, "steps": steps, "checklist": checklist})
}

func (s *Server) apiOnboardingCreateRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	repo, err := s.onboardingService.CreateRepo(req["name"], req["slug"], req["repo_url"], req["repo_owner"], req["repo_name"])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "app_id": repo.AppID, "name": repo.Name, "slug": repo.Slug, "repo_url": repo.RepoURL, "owner": repo.Owner, "repo": repo.Repo})
}

func (s *Server) apiOnboardingImportRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	res, err := s.onboardingService.ImportRepo(req["repo_url"], req["repo_owner"], req["repo_name"])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) apiOnboardingListRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	owner := strings.TrimSpace(req["owner"])
	if owner == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Owner or username is required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "owner": owner, "repos": s.onboardingService.ListRepos(owner), "token_used": false, "visibility_note": "Need PAT to see private repositories."})
}

func (s *Server) apiOnboardingInspectRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, code, msg := s.onboardingService.InspectRepo(strings.TrimSpace(r.URL.Query().Get("app_id")), strings.TrimSpace(r.URL.Query().Get("repo")))
	if msg != "" {
		writeJSON(w, code, map[string]any{"ok": false, "error": msg})
		return
	}
	writeJSON(w, code, res)
}

func (s *Server) apiOnboardingCreateIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	appID, _ := req["app_id"].(string)
	repo, _ := req["repo"].(string)
	createCI, _ := req["create_ci"].(bool)
	createOTEL, _ := req["create_otel"].(bool)
	res, code, msg := s.onboardingService.CreateIssues(appID, repo, createCI, createOTEL)
	if msg != "" {
		writeJSON(w, code, map[string]any{"ok": false, "error": msg})
		return
	}
	writeJSON(w, code, res)
}
