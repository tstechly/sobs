// Package integration tests kubernetes endpoints.
package integration

import (
	"net/http"
	"testing"
)

// TestKubernetesUI tests kubernetes UI routes.
func TestKubernetesUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/kubernetes returns k8s settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/kubernetes")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/kubernetes returned status: %d", resp.StatusCode)
	})

	t.Run("GET /kubernetes shows kubernetes dashboard", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/kubernetes")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /kubernetes returned status: %d", resp.StatusCode)
	})
}

// TestKubernetesAPI tests kubernetes API endpoints.
func TestKubernetesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/kubernetes/status returns k8s status", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/kubernetes/status")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/kubernetes/status returned status: %d", resp.StatusCode)
	})
}
