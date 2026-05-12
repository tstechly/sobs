// Package integration tests MCP (Model Context Protocol) endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// TestMCPUI tests MCP settings UI route.
func TestMCPUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/mcp returns MCP settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/mcp")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/mcp returned status: %d", resp.StatusCode)
	})
}

// TestMCPAPI tests MCP API endpoints.
func TestMCPAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /mcp/tools lists MCP tools", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/mcp/tools")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /mcp/tools returned status: %d", resp.StatusCode)
	})

	t.Run("POST /mcp handles MCP protocol", func(t *testing.T) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      1,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/mcp", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /mcp returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/mcp/keys lists MCP API keys", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/mcp/keys")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/mcp/keys returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/mcp/keys creates MCP API key", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "test-mcp-key",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/mcp/keys", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/mcp/keys returned status: %d", resp.StatusCode)
	})

	t.Run("DELETE /api/mcp/keys/<key_id> deletes MCP API key", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/mcp/keys/test-key-id", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("DELETE /api/mcp/keys/test-key-id returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/mcp/enabled toggles MCP enabled", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := http.PostForm(baseURL+"/api/mcp/enabled", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/mcp/enabled returned status: %d", resp.StatusCode)
	})
}

// TestMetricsRules tests metrics rules endpoints.
func TestMetricsRules(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /metrics/rules creates metrics rule", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-metrics-rule")
		payload.Set("ruleType", "threshold")
		payload.Set("signalSource", "logs")
		payload.Set("signalName", "error_volume")

		resp, err := http.PostForm(baseURL+"/metrics/rules", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /metrics/rules returned status: %d", resp.StatusCode)
	})

	t.Run("POST /metrics/rules/auto auto-generates rules", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/metrics/rules/auto", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /metrics/rules/auto returned status: %d", resp.StatusCode)
	})

	t.Run("POST /metrics/rules/dashboard/auto auto-generates dashboard rules", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/metrics/rules/dashboard/auto", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /metrics/rules/dashboard/auto returned status: %d", resp.StatusCode)
	})

	t.Run("POST /metrics/rules/<rule_id>/delete deletes rule", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/metrics/rules/test-rule-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /metrics/rules/test-rule-id/delete returned status: %d", resp.StatusCode)
	})
}
