// Package integration contains common helpers for SOBS integration tests.
package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// getBaseURL returns the base URL for the SOBS server.
// It checks the SOBS_TEST_URL environment variable, or defaults to localhost:5000.
func getBaseURL() string {
	if url := os.Getenv("SOBS_TEST_URL"); url != "" {
		return url
	}
	return "http://localhost:44317"
}

// waitForServer waits for the server to be ready, up to a timeout.
func waitForServer(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server at %s not ready within %v", baseURL, timeout)
}

// skipIfServerNotAvailable skips the test if the server is not available.
func skipIfServerNotAvailable(t *testing.T, baseURL string) {
	t.Helper()
	if err := waitForServer(baseURL, 5*time.Second); err != nil {
		t.Skipf("Skipping test: server not available: %v", err)
	}
}

// newTestServer creates a new httptest.Server for unit-style integration tests.
func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}
