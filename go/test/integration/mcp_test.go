// Package integration tests MCP (Model Context Protocol) endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
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

		assertStatusIn(t, resp, "GET /settings/mcp", http.StatusOK)
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

		assertStatusIn(t, resp, "GET /mcp/tools", http.StatusOK)
		assertJSONBody(t, resp, "GET /mcp/tools")
	})

	t.Run("POST /mcp without API key returns 401", func(t *testing.T) {
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

		assertStatusIn(t, resp, "POST /mcp", http.StatusUnauthorized)
	})

	t.Run("GET /api/mcp/keys lists MCP API keys", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/mcp/keys")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/mcp/keys", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/mcp/keys")
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

		assertStatusIn(t, resp, "POST /api/mcp/keys", http.StatusOK, http.StatusCreated)
	})

	t.Run("DELETE /api/mcp/keys/<key_id> returns 404 for missing key", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/mcp/keys/test-key-id", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "DELETE /api/mcp/keys/test-key-id", http.StatusNotFound)
	})

	t.Run("POST /api/mcp/enabled toggles MCP enabled", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := http.PostForm(baseURL+"/api/mcp/enabled", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/mcp/enabled", http.StatusOK)
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

		resp, err := postFormNoRedirect(baseURL+"/metrics/rules", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /metrics/rules", http.StatusFound)
	})

	t.Run("POST /metrics/rules/auto auto-generates rules", func(t *testing.T) {
		resp, err := http.PostForm(baseURL+"/metrics/rules/auto", url.Values{})
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /metrics/rules/auto", http.StatusOK)
	})

	t.Run("POST /metrics/rules/dashboard/auto auto-generates dashboard rules", func(t *testing.T) {
		resp, err := http.PostForm(baseURL+"/metrics/rules/dashboard/auto", url.Values{})
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /metrics/rules/dashboard/auto", http.StatusOK)
	})

	t.Run("POST /metrics/rules/<rule_id>/delete deletes rule", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/metrics/rules/test-rule-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /metrics/rules/test-rule-id/delete", http.StatusFound)
	})
}
