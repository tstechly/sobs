// Package integration tests notification-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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

		resp, err := http.Post(baseURL+"/settings/notifications/channels", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/channels returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/notifications/channels/<channel_id>/delete deletes channel", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/notifications/channels/test-channel-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/channels/test-channel-id/delete returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/notifications/channels/<channel_id>/toggle toggles channel", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/notifications/channels/test-channel-id/toggle", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/channels/test-channel-id/toggle returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/notifications/channels/<channel_id>/test tests channel", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/notifications/channels/test-channel-id/test", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/notifications/channels/test-channel-id/test returned status: %d", resp.StatusCode)
	})
}

// TestNotificationRules tests notification rule endpoints.
func TestNotificationRules(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /settings/notifications/rules creates rule", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":    "test-rule",
			"enabled": true,
			"conditionsJson": `{"severity": "critical"}`,
			"channelIds": "test-channel-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/settings/notifications/rules", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/rules returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/notifications/rules/<rule_id>/toggle toggles rule", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/notifications/rules/test-rule-id/toggle", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/rules/test-rule-id/toggle returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/notifications/rules/<rule_id>/delete deletes rule", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/notifications/rules/test-rule-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/notifications/rules/test-rule-id/delete returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/notifications/rules/auto-generate auto-generates rules", func(t *testing.T) {
		payload := map[string]interface{}{
			"signalSource": "logs",
			"signalName":  "error_volume",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/notifications/rules/auto-generate", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/notifications/rules/auto-generate returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/notifications/check checks notifications", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/notifications/check", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/notifications/check returned status: %d", resp.StatusCode)
	})
}

// TestPushNotifications tests push notification endpoints.
func TestPushNotifications(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/notifications/vapid-public-key returns VAPID key", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/notifications/vapid-public-key")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("GET /api/notifications/vapid-public-key returned status: %d, body: %s", resp.StatusCode, string(body))
	})

	t.Run("POST /api/notifications/subscribe subscribes to push", func(t *testing.T) {
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

		t.Logf("POST /api/notifications/subscribe returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/notifications/vapid-keygen generates VAPID keys", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/notifications/vapid-keygen", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/notifications/vapid-keygen returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/notifications/vapid-keys deletes VAPID keys", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/notifications/vapid-keys", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("DELETE /api/notifications/vapid-keys returned status: %d", resp.StatusCode)
	})
}
