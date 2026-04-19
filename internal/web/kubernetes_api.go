package web

import (
	"net/http"
	"strconv"
)

func (s *Server) settingsKubernetes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"k8s_settings": s.kubernetesService.GetSettings()})
			return
		}
		s.pageTemplateHandler("/settings/kubernetes", "settings_kubernetes.html")(w, r)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		enabled := parseBool(vals["enabled"])
		ns := vals["default_namespace"]
		writeJSON(w, http.StatusOK, s.kubernetesService.SaveSettings(enabled, ns))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) kubernetesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.kubernetesService.GetSettings().Enabled {
		http.Error(w, "Kubernetes health view is disabled. Enable it in Settings -> Kubernetes.", http.StatusNotFound)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	s.pageTemplateHandler("/kubernetes", "kubernetes.html")(w, r)
}

func (s *Server) apiKubernetesStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := s.kubernetesService.Status()
	if ok, _ := status["ok"].(bool); !ok {
		writeJSON(w, http.StatusNotFound, status)
		return
	}
	status["nodes_page"] = parseIntQuery(r, "nodes_page", 1)
	status["pods_page"] = parseIntQuery(r, "pods_page", 1)
	status["deployments_page"] = parseIntQuery(r, "deployments_page", 1)
	writeJSON(w, http.StatusOK, status)
}

func parseIntQuery(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
