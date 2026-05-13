// Package integration contains common helpers for SOBS integration tests.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sobstelemetry "github.com/abartrim/sobs/go/telemetry"
)

// getBaseURL returns the base URL for the SOBS server.
// It checks the SOBS_TEST_URL environment variable, or defaults to localhost:44317.
func getBaseURL() string {
	if url := os.Getenv("SOBS_TEST_URL"); url != "" {
		return url
	}
	return "http://localhost:8080" //8080" //44317"
}

// waitForServer waits for the server to be ready, up to a timeout.
func waitForServer(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := sobstelemetry.Get(context.Background(), baseURL+"/health")
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

// noRedirectClient returns an http.Client that does not follow redirects.
// Used to assert that POST endpoints respond with the expected 3xx status
// rather than transparently following to the redirect target.
func noRedirectClient() *http.Client {
	baseClient := sobstelemetry.GetClient()
	return &http.Client{
		Transport: baseClient.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
}

// postForm issues a POST with form-encoded body using a client that does not follow redirects.
func postFormNoRedirect(urlStr string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := noRedirectClient()
	return client.Do(req)
}

// postJSONNoRedirect issues a POST with a JSON body using a client that does not follow redirects.
func postJSONNoRedirect(urlStr string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := noRedirectClient()
	return client.Do(req)
}

// assertStatusIn fails the test if the response status is not one of the expected codes.
// Includes a snippet of the response body in the failure message for diagnosis.
func assertStatusIn(t *testing.T, resp *http.Response, endpoint string, expected ...int) {
	t.Helper()
	if resp == nil {
		t.Fatalf("%s: nil response", endpoint)
	}
	for _, e := range expected {
		if resp.StatusCode == e {
			return
		}
	}
	body := readBodySnippet(resp)
	t.Errorf("%s: got status %d, expected one of %v, body=%q", endpoint, resp.StatusCode, expected, body)
}

// assertJSONBody fails the test if the response body is not valid JSON.
// Returns the parsed value for further assertions; nil if parsing failed.
func assertJSONBody(t *testing.T, resp *http.Response, endpoint string) interface{} {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("%s: failed to read body: %v", endpoint, err)
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Errorf("%s: response is not valid JSON: %v, body=%q", endpoint, err, truncate(string(body), 200))
		return nil
	}
	return v
}

// assertContentTypeContains fails the test if the response Content-Type does not contain the expected substring.
func assertContentTypeContains(t *testing.T, resp *http.Response, endpoint, want string) {
	t.Helper()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, want) {
		t.Errorf("%s: Content-Type %q does not contain %q", endpoint, ct, want)
	}
}

// readBodySnippet reads up to 512 bytes from the response body for inclusion in error messages.
// Safe to call after the body has been consumed (returns empty string in that case).
func readBodySnippet(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return truncate(string(b), 256)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
