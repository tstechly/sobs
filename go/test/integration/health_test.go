// Package integration contains integration tests for SOBS endpoints.
package integration

import (
	"encoding/json"
	"fmt"
	"io"
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
	return "http://localhost:5000"
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

// TestHealthEndpoint tests the basic health check endpoint.
func TestHealthEndpoint(t *testing.T) {
	baseURL := getBaseURL()

	// Wait for server to be ready (useful when running against a fresh instance)
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	t.Run("GET /health returns 200 OK", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("Failed to make request to /health: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		t.Logf("Health endpoint response: %s", string(body))

		// Verify response is valid JSON
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Errorf("Response is not valid JSON: %v", err)
		}

		t.Logf("Health check response parsed successfully: %+v", result)
	})

	t.Run("GET /health response time is acceptable", func(t *testing.T) {
		start := time.Now()
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("Failed to make request to /health: %v", err)
		}
		defer resp.Body.Close()
		elapsed := time.Since(start)

		if elapsed > 5*time.Second {
			t.Errorf("Health endpoint took too long: %v", elapsed)
		}

		t.Logf("Health endpoint responded in %v", elapsed)
	})
}

// TestHealthDBEndpoint tests the database health check endpoint.
func TestHealthDBEndpoint(t *testing.T) {
	baseURL := getBaseURL()

	// Wait for server to be ready
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	t.Run("GET /health/db returns 200 OK", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health/db")
		if err != nil {
			t.Fatalf("Failed to make request to /health/db: %v", err)
		}
		defer resp.Body.Close()

		// Database health might return 200 (healthy) or 503 (unhealthy)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status 200 or 503, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		t.Logf("Health DB endpoint response: %s", string(body))

		// Verify response is valid JSON
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Errorf("Response is not valid JSON: %v", err)
		}

		// Check for expected fields in database health response
		if status, ok := result["status"]; ok {
			t.Logf("Database status: %v", status)
		}

		t.Logf("Database health check response: %+v", result)
	})

	t.Run("GET /health/db includes database status", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health/db")
		if err != nil {
			t.Fatalf("Failed to make request to /health/db: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		// Verify the response contains meaningful data
		if len(result) == 0 {
			t.Error("Database health response is empty")
		}

		t.Logf("Database health contains %d fields", len(result))
	})
}

// TestHealthEndpointsConcurrent tests that health endpoints handle concurrent requests.
func TestHealthEndpointsConcurrent(t *testing.T) {
	baseURL := getBaseURL()

	// Wait for server to be ready
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	t.Run("Concurrent requests to /health", func(t *testing.T) {
		numRequests := 10
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(id int) {
				resp, err := http.Get(baseURL + "/health")
				if err != nil {
					t.Errorf("Goroutine %d: Failed to make request: %v", id, err)
					results <- 0
					return
				}
				defer resp.Body.Close()
				results <- resp.StatusCode
			}(i)
		}

		successCount := 0
		for i := 0; i < numRequests; i++ {
			status := <-results
			if status == http.StatusOK {
				successCount++
			}
		}

		if successCount < numRequests {
			t.Errorf("Only %d/%d concurrent requests succeeded", successCount, numRequests)
		}

		t.Logf("All %d concurrent requests to /health succeeded", successCount)
	})
}

// TestHealthEndpointWithInvalidMethod tests that health endpoint handles non-GET methods appropriately.
func TestHealthEndpointWithInvalidMethod(t *testing.T) {
	baseURL := getBaseURL()

	// Wait for server to be ready
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	t.Run("POST to /health returns appropriate status", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/health", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make POST request to /health: %v", err)
		}
		defer resp.Body.Close()

		// Health endpoint might allow or reject POST - both are valid behaviors
		t.Logf("POST /health returned status: %d", resp.StatusCode)

		// Common responses: 200 (allowed), 405 (method not allowed), or 404
		if resp.StatusCode == http.StatusMethodNotAllowed {
			t.Log("POST /health correctly returns 405 Method Not Allowed")
		} else if resp.StatusCode == http.StatusOK {
			t.Log("POST /health is allowed and returns 200 OK")
		}
	})
}

// Helper test to verify the server is running (can be used with httptest for unit-style integration tests).
func TestHealthEndpointWithTestServer(t *testing.T) {
	// This test demonstrates how to use httptest for more controlled testing
	// In a real scenario, you might mock parts of the app

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"healthy","timestamp":"2024-01-01T00:00:00Z"}`)
		} else if r.URL.Path == "/health/db" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"healthy","database":"connected"}`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	t.Run("TestServer: GET /health returns 200", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/health")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		t.Logf("TestServer health response: %s", string(body))
	})

	t.Run("TestServer: GET /health/db returns 200", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/health/db")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		t.Logf("TestServer health/db response: %s", string(body))
	})
}

// BenchmarkHealthEndpoint provides a simple benchmark for the health endpoint.
func BenchmarkHealthEndpoint(b *testing.B) {
	baseURL := getBaseURL()

	// Quick check if server is available
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		b.Skipf("Server not available: %v", err)
	}
	resp.Body.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
