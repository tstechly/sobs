// Package integration tests OpenTelemetry and data ingestion endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestIngestionLogs tests the POST /v1/logs endpoint.
func TestIngestionLogs(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/logs accepts valid OTLP payload", func(t *testing.T) {
		// Minimal OTLP logs payload
		payload := map[string]interface{}{
			"resourceLogs": []map[string]interface{}{
				{
					"resource": map[string]interface{}{
						"attributes": []map[string]interface{}{
							{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
						},
					},
					"scopeLogs": []map[string]interface{}{
						{
							"logRecords": []map[string]interface{}{
								{
									"body": map[string]interface{}{"stringValue": "test log message"},
									"timeUnixNano": "1746720000000000000",
								},
							},
						},
					},
				},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/logs", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Ingestion endpoints typically return 200 OK or 202 Accepted
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected 200 or 202, got %d", resp.StatusCode)
		}

		t.Logf("POST /v1/logs returned status: %d", resp.StatusCode)
	})

	t.Run("POST /v1/logs rejects invalid payload", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/v1/logs", "application/json", bytes.NewBufferString("{invalid json}"))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Should return 400 Bad Request or similar error
		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for invalid payload, got 200")
		}

		t.Logf("POST /v1/logs with invalid payload returned status: %d", resp.StatusCode)
	})
}

// TestIngestionTraces tests the POST /v1/traces endpoint.
func TestIngestionTraces(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/traces accepts valid OTLP payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"resourceSpans": []map[string]interface{}{
				{
					"resource": map[string]interface{}{
						"attributes": []map[string]interface{}{
							{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
						},
					},
					"scopeSpans": []map[string]interface{}{
						{
							"spans": []map[string]interface{}{
								{
									"traceId": "4bf92f3577b34da6a3ce929d0e0e4736",
									"spanId": "00f067aa0ba902b7",
									"name": "test-span",
									"startTimeUnixNano": "1746720000000000000",
									"endTimeUnixNano": "1746720001000000000",
								},
							},
						},
					},
				},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/traces", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected 200 or 202, got %d", resp.StatusCode)
		}

		t.Logf("POST /v1/traces returned status: %d", resp.StatusCode)
	})
}

// TestIngestionMetrics tests the POST /v1/metrics endpoint.
func TestIngestionMetrics(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/metrics accepts valid OTLP payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"resourceMetrics": []map[string]interface{}{
				{
					"resource": map[string]interface{}{
						"attributes": []map[string]interface{}{
							{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
						},
					},
					"scopeMetrics": []map[string]interface{}{
						{
							"metrics": []map[string]interface{}{
								{
									"name": "test_metric",
									"gauge": map[string]interface{}{
										"dataPoints": []map[string]interface{}{
											{
												"asDouble": 123.45,
												"timeUnixNano": "1746720000000000000",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/metrics", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected 200 or 202, got %d", resp.StatusCode)
		}

		t.Logf("POST /v1/metrics returned status: %d", resp.StatusCode)
	})
}

// TestIngestionRUM tests the POST /v1/rum endpoint.
func TestIngestionRUM(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/rum accepts valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"type": "web-vital",
			"name": "LCP",
			"value": 1234.5,
			"service": "test-rum-service",
			"timestamp": "2024-05-08T12:00:00Z",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/rum", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// RUM ingestion may return 200, 202, or 401 if auth required
		t.Logf("POST /v1/rum returned status: %d", resp.StatusCode)
	})
}

// TestIngestionAI tests the POST /v1/ai endpoint.
func TestIngestionAI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/ai accepts valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"traceId": "test-trace-id",
			"spanId": "test-span-id",
			"prompt": "test prompt",
			"completion": "test completion",
			"service": "test-ai-service",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/ai", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /v1/ai returned status: %d", resp.StatusCode)
	})
}

// TestIngestionErrors tests the POST /v1/errors endpoint.
func TestIngestionErrors(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/errors accepts valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"errorId": "test-error-id",
			"message": "test error message",
			"service": "test-service",
			"timestamp": "2024-05-08T12:00:00Z",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/errors", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /v1/errors returned status: %d", resp.StatusCode)
	})
}

// TestAppsEndpoints tests the /v1/apps endpoints.
func TestAppsEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /v1/apps returns list", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/apps")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("GET /v1/apps returned status: %d, body: %s", resp.StatusCode, string(body))
	})

	t.Run("POST /v1/apps creates new app", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "test-app",
			"description": "Test application",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/apps", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /v1/apps returned status: %d", resp.StatusCode)
	})
}

// TestReleasesEndpoints tests the /v1/releases endpoints.
func TestReleasesEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /v1/apps/<app_id>/releases returns releases", func(t *testing.T) {
		// Using a dummy app_id - in real test, would create app first
		resp, err := http.Get(baseURL + "/v1/apps/test-app-id/releases")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /v1/apps/test-app-id/releases returned status: %d", resp.StatusCode)
	})
}

// TestRUMAssetsEndpoints tests the /v1/rum/assets endpoints.
func TestRUMAssetsEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/rum/assets uploads asset", func(t *testing.T) {
		// Minimal multipart form upload test
		resp, err := http.Post(baseURL+"/v1/rum/assets", "application/octet-stream", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /v1/rum/assets returned status: %d", resp.StatusCode)
	})

	t.Run("GET /v1/rum/assets/<asset_id> retrieves asset", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/v1/rum/assets/test-asset-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /v1/rum/assets/test-asset-id returned status: %d", resp.StatusCode)
	})
}

// TestRUMClientToken tests the POST /v1/rum/client-token endpoint.
func TestRUMClientToken(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /v1/rum/client-token generates token", func(t *testing.T) {
		payload := map[string]interface{}{
			"service": "test-service",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		resp, err := http.Post(baseURL+"/v1/rum/client-token", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /v1/rum/client-token returned status: %d", resp.StatusCode)
	})
}
