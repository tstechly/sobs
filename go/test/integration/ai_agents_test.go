// Package integration tests AI and Agent endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestAISettings tests AI settings endpoints.
func TestAISettings(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/ai returns AI settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/ai")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /settings/ai", http.StatusOK)
	})

	t.Run("POST /settings/ai updates AI settings", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")
		payload.Set("model", "gpt-4")

		resp, err := postFormNoRedirect(baseURL+"/settings/ai", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/ai", http.StatusFound)
	})
}

// TestAIHelper tests AI helper endpoints.
func TestAIHelper(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/ai/helper/capabilities returns capabilities", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/capabilities")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/ai/helper/capabilities", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/ai/helper/capabilities")
	})

	t.Run("GET /api/ai/helper/actions/manifest returns manifest", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/actions/manifest")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/ai/helper/actions/manifest", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/ai/helper/actions/manifest")
	})

	t.Run("GET /api/ai/helper/chats lists chats", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/chats")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/ai/helper/chats", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/ai/helper/chats")
	})

	t.Run("GET /api/ai/helper/chats/<chat_id> gets chat", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/chats/test-chat-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/ai/helper/chats/test-chat-id", http.StatusOK)
	})

	t.Run("POST /api/ai/helper/feedback rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"chatId":    "test-chat-id",
			"messageId": "test-message-id",
			"rating":    "positive",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/ai/helper/feedback", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/ai/helper/feedback", http.StatusBadRequest)
	})

	t.Run("POST /api/ai/helper rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"query": "show me errors from last hour",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/ai/helper", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/ai/helper", http.StatusBadRequest)
	})

	t.Run("POST /api/ai/helper/actions/execute rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"action": "query_logs",
			"params": map[string]interface{}{
				"filter": "severity=ERROR",
			},
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/ai/helper/actions/execute", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/ai/helper/actions/execute", http.StatusBadRequest)
	})
}

// TestIssuesAPI tests issues endpoints.
func TestIssuesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/issues/raise creates issue", func(t *testing.T) {
		payload := map[string]interface{}{
			"title": "Test Issue",
			"body":  "Test issue description",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/issues/raise", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("AI endpoint not configured; skipping issue creation assertion")
		}

		assertStatusIn(t, resp, "POST /api/issues/raise", http.StatusCreated)
		v := assertJSONBody(t, resp, "POST /api/issues/raise")
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected JSON object, got %T", v)
		}
		if _, ok := m["id"].(string); !ok {
			t.Errorf("Expected string id field, got %v", m["id"])
		}
		if title, _ := m["title"].(string); title != "Test Issue" {
			t.Errorf("Expected title %q, got %v", "Test Issue", m["title"])
		}
	})

	t.Run("POST /api/issues/raise rejects empty body", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/issues/raise", "application/json", bytes.NewReader([]byte(`{}`)))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("AI endpoint not configured; skipping empty-body validation")
		}

		assertStatusIn(t, resp, "POST /api/issues/raise (empty)", http.StatusBadRequest)
	})

	t.Run("GET /api/issues/raise rejects non-POST", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/issues/raise")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/issues/raise", http.StatusMethodNotAllowed)
	})
}

// TestAgentRuns tests agent run endpoints.
func TestAgentRuns(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/agent/runs lists agent runs", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/agent/runs")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/agent/runs", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/agent/runs")
	})

	t.Run("POST /api/agent/runs rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"ruleId": "test-rule-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/agent/runs", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/agent/runs", http.StatusBadRequest)
	})

	t.Run("POST /api/agent/runs/<run_id>/dismiss returns 404 for unknown run", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/agent/runs/test-run-id/dismiss", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/agent/runs/test-run-id/dismiss", http.StatusNotFound)
	})
}

// TestAgentSettings tests agent settings endpoints.
func TestAgentSettings(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/agents returns agents settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/agents")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /settings/agents", http.StatusOK)
	})

	t.Run("POST /settings/agents creates agent rule", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-agent-rule")
		payload.Set("triggerType", "anomaly")
		payload.Set("triggerRefId", "test-rule-id")

		resp, err := postFormNoRedirect(baseURL+"/settings/agents", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/agents", http.StatusFound)
	})

	t.Run("POST /settings/agents/<rule_id>/delete deletes agent rule", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/agents/test-rule-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/agents/test-rule-id/delete", http.StatusFound)
	})
}
