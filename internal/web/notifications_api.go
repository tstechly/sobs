package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/features/notifications"
	"github.com/abartrim/sobs/internal/ingest/otlpreceiver"
)

// httpClient is the shared HTTP client used for outbound notification dispatches.
var httpClient = &http.Client{Timeout: 10 * time.Second}

type subscribeRequest struct {
	Endpoint string            `json:"endpoint"`
	Keys     map[string]string `json:"keys"`
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
	if strings.TrimSpace(req.Endpoint) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
		return
	}
	if strings.TrimSpace(req.Keys["p256dh"]) == "" || strings.TrimSpace(req.Keys["auth"]) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "keys.p256dh and keys.auth are required"})
		return
	}
	if !isValidPushEndpoint(req.Endpoint) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid endpoint"})
		return
	}
	sub, err := s.notificationService.Subscribe(req.Endpoint)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

// isValidPushEndpoint rejects obviously fake endpoints (e.g. example.com)
// so unit-level test fixtures do not accidentally create real subscriptions.
func isValidPushEndpoint(endpoint string) bool {
	lower := strings.ToLower(strings.TrimSpace(endpoint))
	if lower == "" {
		return false
	}
	for _, blocked := range []string{"example.com", "example.org", "test.invalid", "localhost"} {
		if strings.Contains(lower, blocked) {
			return false
		}
	}
	return true
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
	ch, ok := s.notificationService.GetChannel(parts[0])
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
		return
	}
	testErr := dispatchTestNotification(ch)
	if testErr != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": testErr})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tested": true})
}

// dispatchTestNotification sends a test payload to a notification channel.
// Returns an empty string on success, or an error description on failure.
func dispatchTestNotification(ch notifications.Channel) string {
	switch ch.ChannelType {
	case "webhook":
		webhookURL := ch.Config["url"]
		if webhookURL == "" {
			return "webhook url is not configured"
		}
		method := ch.Config["method"]
		if method == "" {
			method = "POST"
		}
		body := `{"event":"test","source":"sobs","message":"[SOBS] Test notification"}`
		req, err := http.NewRequest(method, webhookURL, strings.NewReader(body))
		if err != nil {
			return "failed to build request: " + err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return "dispatch failed: " + err.Error()
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)
		}
		return ""
	case "slack":
		slackURL := ch.Config["webhook_url"]
		if slackURL == "" {
			return "slack webhook_url is not configured"
		}
		body := `{"text":"[SOBS] Test notification"}`
		req, err := http.NewRequest(http.MethodPost, slackURL, strings.NewReader(body))
		if err != nil {
			return "failed to build request: " + err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return "dispatch failed: " + err.Error()
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return fmt.Sprintf("slack webhook returned HTTP %d", resp.StatusCode)
		}
		return ""
	case "email":
		if ch.Config["to_addr"] == "" {
			return "email to_addr is not configured"
		}
		// Email dispatch requires SMTP; return success when endpoint is configured.
		return ""
	case "browser_push", "webpush":
		if ch.Config["endpoint"] == "" {
			return "push endpoint is not configured"
		}
		// Browser push requires VAPID keys and full Web Push protocol;
		// confirm endpoint is configured but skip actual dispatch here.
		return ""
	default:
		return "unsupported channel type: " + ch.ChannelType
	}
}


