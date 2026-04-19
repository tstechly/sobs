package web

import (
	"net/http"
	"strings"
)

type createNotificationRuleRequest struct {
	Name string `json:"name"`
}

func (s *Server) settingsNotificationsChannelsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	sub, err := s.notificationService.Subscribe(strings.TrimSpace(vals["endpoint"]))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) settingsNotificationsRulesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	rule, err := s.notificationService.CreateRule(strings.TrimSpace(vals["name"]))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) settingsNotificationsRulesActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/settings/notifications/rules/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	action := parts[1]
	switch action {
	case "toggle":
		r, ok := s.notificationService.ToggleRule(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": r.ID, "enabled": r.Enabled})
	case "delete":
		if !s.notificationService.DeleteRule(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) apiNotificationsRulesAutoGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := s.notificationService.AutoGenerateRules()
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
