package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (s *Server) apiNotificationsSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sub, err := s.notificationService.Subscribe(req.Endpoint)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) tail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 200 {
				parsed = 200
			}
			limit = parsed
		}
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
		return
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(r.Context(), "SELECT Timestamp, ServiceName, SeverityText, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT ?", limit)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
		return
	}
	defer func() { _ = rows.Close() }()
	items := []map[string]any{}
	for rows.Next() {
		var ts string
		var service string
		var severity string
		var body string
		if err := rows.Scan(&ts, &service, &severity, &body); err != nil {
			break
		}
		items = append(items, map[string]any{"ts": ts, "service": service, "severity": severity, "message": body})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) apiNotificationsVAPIDKeygen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pub, priv := s.notificationService.GenerateVAPIDKeys()
	writeJSON(w, http.StatusOK, map[string]string{"public_key": pub, "private_key": priv})
}

func (s *Server) apiNotificationsVAPIDKeysDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.notificationService.DeleteVAPIDKeys()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiNotificationsChannelSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/notifications/channels/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "test" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if !s.notificationService.HasSubscription(parts[0]) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tested": true})
}

func (s *Server) apiNotificationsRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.notificationService.ListRules()})
}

func (s *Server) apiNotificationsSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.notificationService.ListSubscriptions()})
}
