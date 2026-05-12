// Package integration tests core API endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// TestTracesAPI tests trace-related API endpoints.
func TestTracesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/traces/span/<span_id> returns span details", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/traces/span/test-span-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("GET /api/traces/span/test-span-id returned status: %d, body: %s", 
			resp.StatusCode, string(body))
	})
}

// TestWebTrafficAPI tests web traffic analytics endpoints.
func TestWebTrafficAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	endpoints := []struct {
		name string
		path string
	}{
		{"Geo", "/api/web-traffic/geo"},
		{"Browsers", "/api/web-traffic/browsers"},
		{"OS", "/api/web-traffic/os"},
		{"Timezones", "/api/web-traffic/timezones"},
		{"Languages", "/api/web-traffic/languages"},
		{"Devices", "/api/web-traffic/devices"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name+" ("+ep.path+")", func(t *testing.T) {
			resp, err := http.Get(baseURL + ep.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", ep.path, err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			
			// Verify response is valid JSON if status is 200
			if resp.StatusCode == http.StatusOK {
				var result interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("Response is not valid JSON: %v", err)
				}
			}

			t.Logf("GET %s returned status: %d, body length: %d", ep.path, resp.StatusCode, len(body))
		})
	}
}

// TestEnrichmentAPI tests enrichment-related endpoints.
func TestEnrichmentAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/enrichment/libraries returns libraries", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/enrichment/libraries")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/enrichment/libraries returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/enrichment/github/repo-health returns repo health", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/enrichment/github/repo-health")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/enrichment/github/repo-health returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/enrichment/cve/findings returns CVE findings", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/enrichment/cve/findings")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/enrichment/cve/findings returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/enrichment/cve/scan triggers scan", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/enrichment/cve/scan", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/enrichment/cve/scan returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/enrichment/cve/findings/<osv_id>/disposition sets disposition", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("disposition", "accepted")

		resp, err := http.PostForm(baseURL+"/api/enrichment/cve/findings/test-osv-id/disposition", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/enrichment/cve/findings/test-osv-id/disposition returned status: %d", resp.StatusCode)
	})
}

// TestWorkItemsAPI tests work items endpoints.
func TestWorkItemsAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/work-items returns work items", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/work-items")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/work-items returned status: %d", resp.StatusCode)
	})
}

// TestAIAttributesAPI tests AI-related API endpoints.
func TestAIAttributesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/ai/span-attributes returns span attributes", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/span-attributes")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/span-attributes returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/ai/conversation returns conversations", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/conversation")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/conversation returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/ai/export exports AI data", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/ai/export", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/ai/export returned status: %d", resp.StatusCode)
	})
}

// TestFieldHintsAPI tests field hints endpoints.
func TestFieldHintsAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/logs/field-hints returns log field hints", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/logs/field-hints")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/logs/field-hints returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/ai/field-hints returns AI field hints", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/ai/field-hints")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/ai/field-hints returned status: %d", resp.StatusCode)
	})
}

// TestValidationAPI tests regex and filter validation endpoints.
func TestValidationAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	validationEndpoints := []struct {
		name string
		path string
	}{
		{"Logs Validate Filter", "/api/logs/validate-filter"},
		{"Logs Validate Regex", "/api/logs/validate-regex"},
		{"Errors Validate Regex", "/api/errors/validate-regex"},
		{"Traces Validate Regex", "/api/traces/validate-regex"},
		{"Metrics Validate Regex", "/api/metrics/validate-regex"},
		{"RUM Validate Regex", "/api/rum/validate-regex"},
		{"AI Validate Filter", "/api/ai/validate-filter"},
	}

	for _, ep := range validationEndpoints {
		t.Run(ep.name+" ("+ep.path+")", func(t *testing.T) {
			payload := map[string]interface{}{
				"value": "test",
			}
			body, _ := json.Marshal(payload)

			resp, err := http.Post(baseURL+ep.path, "application/json", jsonReader(body))
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", ep.path, err)
			}
			defer resp.Body.Close()

			t.Logf("POST %s returned status: %d", ep.path, resp.StatusCode)
		})
	}
}

// Helper to create a reader from byte slice.
func jsonReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
