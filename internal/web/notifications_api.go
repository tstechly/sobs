package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/ingest/otlpreceiver"
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
	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	if source == "" {
		source = "all"
	}
	serviceFilter := strings.TrimSpace(r.URL.Query().Get("service"))
	subscriber, unsubscribe := s.tailBroker.Subscribe(otlpreceiverSSEQueueMax())
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "retry: 5000\n\n")
	controller := http.NewResponseController(w)
	_ = controller.Flush()

	keepalive := time.NewTicker(otlpreceiverKeepaliveInterval())
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			_, _ = io.WriteString(w, ": keepalive\n\n")
			_ = controller.Flush()
		case event := <-subscriber:
			if source != "all" && event.Source != source {
				continue
			}
			if serviceFilter != "" && event.Service != serviceFilter {
				continue
			}
			payload, err := marshalTailEvent(event)
			if err != nil {
				continue
			}
			_, _ = io.WriteString(w, fmt.Sprintf("data: %s\n\n", payload))
			_ = controller.Flush()
		}
	}
}

func marshalTailEvent(event otlpreceiver.TailEvent) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(event); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func otlpreceiverSSEQueueMax() int {
	return otlpreceiver.EnvSSEQueueMax()
}

func otlpreceiverKeepaliveInterval() time.Duration {
	return otlpreceiver.KeepaliveInterval()
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
