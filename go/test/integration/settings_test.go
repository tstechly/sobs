// Package integration tests settings-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
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

		assertStatusIn(t, resp, "GET /settings/masking", http.StatusOK)
		assertContentTypeContains(t, resp, "GET /settings/masking", "text/html")
	})

	t.Run("POST /settings/masking/keys adds masking key", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("key", "test-key")
		payload.Set("value", "test-value")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/keys", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/keys", http.StatusFound)
	})

	t.Run("POST /settings/masking/keys/delete deletes masking key", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("key", "test-key")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/keys/delete", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/keys/delete", http.StatusFound)
	})

	t.Run("POST /settings/masking/patterns adds masking pattern", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("pattern", "test-pattern")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/patterns", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/patterns", http.StatusFound)
	})

	t.Run("POST /settings/masking/patterns/delete deletes masking pattern", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("pattern", "test-pattern")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/patterns/delete", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/patterns/delete", http.StatusFound)
	})

	t.Run("POST /settings/masking/output toggles masking output", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/output", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/output", http.StatusFound)
	})

	t.Run("POST /settings/masking/sql-output toggles SQL masking output", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("enabled", "true")

		resp, err := postFormNoRedirect(baseURL+"/settings/masking/sql-output", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/masking/sql-output", http.StatusFound)
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

		assertStatusIn(t, resp, "POST /api/settings/masking/preview", http.StatusOK)
		assertJSONBody(t, resp, "POST /api/settings/masking/preview")
	})

	t.Run("GET /api/settings/masking/rules returns masking rules", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/settings/masking/rules")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/settings/masking/rules", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/settings/masking/rules")
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

		assertStatusIn(t, resp, "GET /settings/tags", http.StatusOK)
		assertContentTypeContains(t, resp, "GET /settings/tags", "text/html")
	})

	t.Run("GET /api/settings/tags/condition-suggestions returns suggestions", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/settings/tags/condition-suggestions")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/settings/tags/condition-suggestions", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/settings/tags/condition-suggestions")
	})

	t.Run("POST /settings/tags/auto auto-generates tag rules", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/settings/tags/auto", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/tags/auto", http.StatusOK)
	})

	t.Run("POST /settings/tags creates tag rule", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-tag-rule")
		payload.Set("recordTypes", "logs")
		payload.Set("matchField", "body")
		payload.Set("matchOperator", "contains")
		payload.Set("matchValue", "error")

		resp, err := postFormNoRedirect(baseURL+"/settings/tags", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/tags", http.StatusFound)
	})

	t.Run("POST /settings/tags/<rule_id>/delete deletes tag rule", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/tags/test-rule-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/tags/test-rule-id/delete", http.StatusFound)
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

		assertStatusIn(t, resp, "GET /api/tags/logs/test-record-id", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/tags/logs/test-record-id")
	})

	t.Run("POST /api/tags/<record_type>/<record_id> adds tag", func(t *testing.T) {
		payload := map[string]interface{}{
			"tagKey":   "test-key",
			"tagValue": "test-value",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/tags/logs/test-record-id", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/tags/logs/test-record-id", http.StatusBadRequest)
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

		assertStatusIn(t, resp, "DELETE /api/tags/logs/test-record-id/test-key", http.StatusNotFound)
	})
}
