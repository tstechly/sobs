// Package integration contains integration tests for SOBS endpoints.
package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sobstelemetry "github.com/abartrim/sobs/go/telemetry"
)

// TestHealthEndpoint tests the basic health check endpoint.
func TestHealthEndpoint(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	// Initialize test context with telemetry
	tc, cleanup := InitTest(t, "TestHealthEndpoint")
	defer cleanup()

	t.Run("GET /health returns 200 OK", func(t *testing.T) {
		ctx := context.Background()
		if tc != nil {
			ctx = tc.Ctx
		}

		resp, err := sobstelemetry.Get(ctx, baseURL+"/health")
		if err != nil {
			t.Fatalf("Failed to make request to /health: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /health", http.StatusOK)
		assertContentTypeContains(t, resp, "GET /health", "application/json")
		assertJSONBody(t, resp, "GET /health")
	})

	t.Run("GET /health response time is acceptable", func(t *testing.T) {
		ctx := context.Background()
		if tc != nil {
			ctx = tc.Ctx
		}

		start := time.Now()
		resp, err := sobstelemetry.Get(ctx, baseURL+"/health")
		if err != nil {
			t.Fatalf("Failed to make request to /health: %v", err)
		}
		defer resp.Body.Close()
		elapsed := time.Since(start)

		if elapsed > 5*time.Second {
			t.Errorf("Health endpoint took too long: %v", elapsed)
		}
		assertStatusIn(t, resp, "GET /health (latency)", http.StatusOK)
	})
}

// TestHealthDBEndpoint tests the database health check endpoint.
func TestHealthDBEndpoint(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	// Initialize test context with telemetry
	tc, cleanup := InitTest(t, "TestHealthDBEndpoint")
	defer cleanup()

	t.Run("GET /health/db returns 200 or 503", func(t *testing.T) {
		ctx := context.Background()
		if tc != nil {
			ctx = tc.Ctx
		}

		resp, err := sobstelemetry.Get(ctx, baseURL+"/health/db")
		if err != nil {
			t.Fatalf("Failed to make request to /health/db: %v", err)
		}
		defer resp.Body.Close()

		// 200 when DB is healthy, 503 when unhealthy.
		assertStatusIn(t, resp, "GET /health/db", http.StatusOK, http.StatusServiceUnavailable)
		assertContentTypeContains(t, resp, "GET /health/db", "application/json")
		assertJSONBody(t, resp, "GET /health/db")
	})

	t.Run("GET /health/db response is non-empty JSON object", func(t *testing.T) {
		ctx := context.Background()
		if tc != nil {
			ctx = tc.Ctx
		}

		resp, err := sobstelemetry.Get(ctx, baseURL+"/health/db")
		if err != nil {
			t.Fatalf("Failed to make request to /health/db: %v", err)
		}
		defer resp.Body.Close()

		v := assertJSONBody(t, resp, "GET /health/db")
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected JSON object, got %T", v)
		}
		if len(m) == 0 {
			t.Error("Database health response is empty")
		}
	})
}

// TestHealthEndpointsConcurrent tests that health endpoints handle concurrent requests.
func TestHealthEndpointsConcurrent(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	// Initialize test context with telemetry
	tc, cleanup := InitTest(t, "TestHealthEndpointsConcurrent")
	defer cleanup()

	t.Run("Concurrent requests to /health", func(t *testing.T) {
		numRequests := 10
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(id int) {
				ctx := context.Background()
				if tc != nil {
					ctx = tc.Ctx
				}

				resp, err := sobstelemetry.Get(ctx, baseURL+"/health")
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

		if successCount != numRequests {
			t.Errorf("Only %d/%d concurrent requests succeeded", successCount, numRequests)
		}
	})
}

// TestHealthEndpointWithInvalidMethod tests that health endpoint handles non-GET methods appropriately.
func TestHealthEndpointWithInvalidMethod(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	// Initialize test context with telemetry
	tc, cleanup := InitTest(t, "TestHealthEndpointWithInvalidMethod")
	defer cleanup()

	t.Run("POST to /health returns 405", func(t *testing.T) {
		ctx := context.Background()
		if tc != nil {
			ctx = tc.Ctx
		}

		resp, err := sobstelemetry.Post(ctx, baseURL+"/health", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make POST request to /health: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /health", http.StatusMethodNotAllowed)
	})
}

// TestHealthEndpointWithTestServer demonstrates using httptest for controlled testing.
func TestHealthEndpointWithTestServer(t *testing.T) {
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

		assertStatusIn(t, resp, "TestServer GET /health", http.StatusOK)
		assertJSONBody(t, resp, "TestServer GET /health")
	})

	t.Run("TestServer: GET /health/db returns 200", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/health/db")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "TestServer GET /health/db", http.StatusOK)
		assertJSONBody(t, resp, "TestServer GET /health/db")
	})
}

// BenchmarkHealthEndpoint provides a simple benchmark for the health endpoint.
// Note: Benchmarks don't use telemetry to avoid overhead during benchmarking.
func BenchmarkHealthEndpoint(b *testing.B) {
	baseURL := getBaseURL()

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
