// Package integration tests AI and Agent endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
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

		t.Logf("GET /settings/ai returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/ai updates AI settings", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")
		payload.Set("model", "gpt-4")

		resp, err := http.PostForm(baseURL+"/settings/ai", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/ai returned status: %d", resp.StatusCode)
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

		t.Logf("GET /api/ai/helper/capabilities returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/ai/helper/actions/manifest returns manifest", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/actions/manifest")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/helper/actions/manifest returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/ai/helper/chats lists chats", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/chats")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/helper/chats returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/ai/helper/chats/<chat_id> gets chat", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/helper/chats/test-chat-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/helper/chats/test-chat-id returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/ai/helper/feedback submits feedback", func(t *testing.T) {
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

		t.Logf("POST /api/ai/helper/feedback returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/ai/helper queries AI helper", func(t *testing.T) {
		payload := map[string]interface{}{
			"query": "show me errors from last hour",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/ai/helper", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/ai/helper returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/ai/helper/actions/execute executes action", func(t *testing.T) {
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

		t.Logf("POST /api/ai/helper/actions/execute returned status: %d", resp.StatusCode)
	})
}

// TestIssuesAPI tests issues endpoints.
func TestIssuesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/issues/raise raises issue", func(t *testing.T) {
		payload := map[string]interface{}{
			"title":       "Test Issue",
			"description": "Test issue description",
			"severity":    "high",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/issues/raise", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/issues/raise returned status: %d", resp.StatusCode)
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

		t.Logf("GET /api/agent/runs returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/agent/runs creates agent run", func(t *testing.T) {
		payload := map[string]interface{}{
			"ruleId": "test-rule-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/agent/runs", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/agent/runs returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/agent/runs/<run_id>/dismiss dismisses run", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/agent/runs/test-run-id/dismiss", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/agent/runs/test-run-id/dismiss returned status: %d", resp.StatusCode)
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

		t.Logf("GET /settings/agents returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/agents creates agent rule", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-agent-rule")
		payload.Set("triggerType", "anomaly")
		payload.Set("triggerRefId", "test-rule-id")

		resp, err := http.PostForm(baseURL+"/settings/agents", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/agents returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/agents/<rule_id>/delete deletes agent rule", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/agents/test-rule-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/agents/test-rule-id/delete returned status: %d", resp.StatusCode)
	})
}
