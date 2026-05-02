package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This integration test posts an OTLP/JSON log payload to a running SOBS
// instance and verifies the message becomes visible on the Logs UI page.
//
// It is opt-in: set SOBS_BASE or SOBS_BASE_URL to run it.

func sobsBase(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("SOBS_BASE"); v != "" {
		return v
	}
	if v := os.Getenv("SOBS_BASE_URL"); v != "" {
		return v
	}
	t.Skip("set SOBS_BASE or SOBS_BASE_URL to run this integration test")
	return ""
}

func tsNs() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func otlpLogPayload(message, service string) map[string]any {
	return map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": map[string]any{"stringValue": service}}}},
				"scopeLogs": []any{map[string]any{"logRecords": []any{map[string]any{"timeUnixNano": tsNs(), "severityText": "INFO", "body": map[string]any{"stringValue": message}}}}},
			},
		},
	}
}

func postJSON(t *testing.T, url string, v any) *http.Response {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	r, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return r
}

func waitForText(t *testing.T, base string, path string, expected string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + path)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		last = string(body)
		if bytes.Contains([]byte(last), []byte(expected)) {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
	require.Failf(t, "timed out waiting for text", "expected %q on %s%s. last_len=%d", expected, base, path, len(last))
	return last
}

func TestLogsEndpointVisibleInUI(t *testing.T) {
	base := sobsBase(t)
	marker := fmt.Sprintf("go-logs-integ-%d", time.Now().UnixMilli())
	payload := otlpLogPayload(marker, "go-integration-service")

	resp := postJSON(t, base+"/v1/logs", payload)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Response shape is JSON with accepted count; be permissive and parse if present.
	var j map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&j) // ignore decode error, just ensure status ok
	if v, ok := j["accepted"]; ok {
		switch vv := v.(type) {
		case float64:
			require.GreaterOrEqual(t, int(vv), 1)
		case int:
			require.GreaterOrEqual(t, vv, 1)
		}
	}

	// Wait for the Logs UI page to render the marker text.
	waitForText(t, base, fmt.Sprintf("/logs?q=%s", marker), marker, 15*time.Second)
}
