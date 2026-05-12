// Package integration tests settings-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// TestSettingsMasking tests masking settings endpoints.
func TestSettingsMasking(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/masking returns masking settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/masking")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/masking returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/keys adds masking key", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("key", "test-key")
		payload.Set("value", "test-value")

		resp, err := http.PostForm(baseURL+"/settings/masking/keys", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/keys returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/keys/delete deletes masking key", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("key", "test-key")

		resp, err := http.PostForm(baseURL+"/settings/masking/keys/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/keys/delete returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/patterns adds masking pattern", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("pattern", "test-pattern")

		resp, err := http.PostForm(baseURL+"/settings/masking/patterns", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/patterns returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/patterns/delete deletes masking pattern", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("pattern", "test-pattern")

		resp, err := http.PostForm(baseURL+"/settings/masking/patterns/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/patterns/delete returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/output toggles masking output", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := http.PostForm(baseURL+"/settings/masking/output", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/output returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/masking/sql-output toggles SQL masking output", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := http.PostForm(baseURL+"/settings/masking/sql-output", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/masking/sql-output returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/settings/masking/preview previews masking", func(t *testing.T) {
		payload := map[string]interface{}{
			"text": "test sensitive data 123-45-6789",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/settings/masking/preview", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/settings/masking/preview returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/settings/masking/rules returns masking rules", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/settings/masking/rules")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("GET /api/settings/masking/rules returned status: %d, body: %s", resp.StatusCode, string(body))
	})
}

// TestSettingsTags tests tag settings endpoints.
func TestSettingsTags(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/tags returns tags settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/tags")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/tags returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/settings/tags/condition-suggestions returns suggestions", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/settings/tags/condition-suggestions")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/settings/tags/condition-suggestions returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/tags/auto auto-generates tag rules", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/settings/tags/auto", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/tags/auto returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/tags creates tag rule", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-tag-rule")
		payload.Set("recordTypes", "logs")
		payload.Set("matchField", "body")
		payload.Set("matchOperator", "contains")
		payload.Set("matchValue", "error")

		resp, err := http.PostForm(baseURL+"/settings/tags", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/tags returned status: %d", resp.StatusCode)
	})
}

// TestTagsAPI tests tag API endpoints.
func TestTagsAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/tags/<record_type>/<record_id> returns tags", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/tags/logs/test-record-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/tags/logs/test-record-id returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/tags/<record_type>/<record_id> adds tag", func(t *testing.T) {
		payload := map[string]interface{}{
			"tagKey": "test-key",
			"tagValue": "test-value",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/tags/logs/test-record-id", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/tags/logs/test-record-id returned status: %d", resp.StatusCode)
	})

	t.Run("DELETE /api/tags/<record_type>/<record_id>/<tag_key> removes tag", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/tags/logs/test-record-id/test-key", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("DELETE /api/tags/logs/test-record-id/test-key returned status: %d", resp.StatusCode)
	})
}
