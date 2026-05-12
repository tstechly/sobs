package web

import (
	"net/http"
	"strings"
)

func (s *Server) settingsNotificationsChannelActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/settings/notifications/channels/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	action := parts[1]
	switch action {
	case "toggle":
		_, _ = s.notificationService.ToggleSubscription(id)
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
	case "delete":
		_ = s.notificationService.DeleteSubscription(id)
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}
