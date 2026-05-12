// Package integration tests notification-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestNotificationChannels tests notification channel endpoints.
func TestNotificationChannels(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /settings/notifications/channels creates channel", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "test-channel",
			"channelType": "webhook",
			"configJson":  `{"url": "https://example.com/webhook"}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := postJSONNoRedirect(baseURL+"/settings/notifications/channels", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/channels", http.StatusFound)
	})

	t.Run("POST /settings/notifications/channels/<channel_id>/delete deletes channel", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/notifications/channels/test-channel-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/channels/test-channel-id/delete", http.StatusFound)
	})

	t.Run("POST /settings/notifications/channels/<channel_id>/toggle toggles channel", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/notifications/channels/test-channel-id/toggle", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/channels/test-channel-id/toggle", http.StatusFound)
	})

	t.Run("POST /api/notifications/channels/<channel_id>/test returns 404 for missing channel", func(t *testing.T) {
		// endpoints.txt documents GET, but the route is registered for POST and returns
		// 404 when the channel does not exist.
		resp, err := http.Post(baseURL+"/api/notifications/channels/test-channel-id/test", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/notifications/channels/test-channel-id/test", http.StatusNotFound)
	})
}

// TestNotificationRules tests notification rule endpoints.
func TestNotificationRules(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /settings/notifications/rules creates rule", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":           "test-rule",
			"enabled":        true,
			"conditionsJson": `{"severity": "critical"}`,
			"channelIds":     "test-channel-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := postJSONNoRedirect(baseURL+"/settings/notifications/rules", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/rules", http.StatusFound)
	})

	t.Run("POST /settings/notifications/rules/<rule_id>/toggle toggles rule", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/notifications/rules/test-rule-id/toggle", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/rules/test-rule-id/toggle", http.StatusFound)
	})

	t.Run("POST /settings/notifications/rules/<rule_id>/delete deletes rule", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/notifications/rules/test-rule-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/notifications/rules/test-rule-id/delete", http.StatusFound)
	})

	t.Run("POST /api/notifications/rules/auto-generate auto-generates rules", func(t *testing.T) {
		payload := map[string]interface{}{
			"signalSource": "logs",
			"signalName":   "error_volume",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/notifications/rules/auto-generate", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/notifications/rules/auto-generate", http.StatusOK)
	})

	t.Run("POST /api/notifications/check checks notifications", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/notifications/check", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/notifications/check", http.StatusOK)
	})
}

// TestPushNotifications tests push notification endpoints.
func TestPushNotifications(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/notifications/vapid-public-key returns 404 when no key generated", func(t *testing.T) {
		// Server returns 404 until a VAPID key has been generated via /vapid-keygen.
		resp, err := http.Get(baseURL + "/api/notifications/vapid-public-key")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/notifications/vapid-public-key", http.StatusOK, http.StatusNotFound)
	})

	t.Run("POST /api/notifications/subscribe rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"endpoint": "https://example.com/push",
			"keys": map[string]string{
				"p256dh": "test-key",
				"auth":   "test-auth",
			},
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/notifications/subscribe", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/notifications/subscribe", http.StatusBadRequest)
	})

	t.Run("POST /api/notifications/vapid-keygen generates VAPID keys", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/notifications/vapid-keygen", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/notifications/vapid-keygen", http.StatusOK)
	})

	t.Run("DELETE /api/notifications/vapid-keys deletes VAPID keys", func(t *testing.T) {
		// endpoints.txt documents POST for "Delete VAPID keys" but the actual
		// REST verb used by the server is DELETE.
		req, err := http.NewRequest("DELETE", baseURL+"/api/notifications/vapid-keys", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "DELETE /api/notifications/vapid-keys", http.StatusOK, http.StatusNoContent)
	})
}
