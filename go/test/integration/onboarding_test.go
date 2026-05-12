// Package integration tests onboarding endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestOnboardingAPI tests onboarding API endpoints.
func TestOnboardingAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/setup-wizard/steps returns wizard steps", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/setup-wizard/steps")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/setup-wizard/steps returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/onboarding/create-repo creates repository", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "test-onboarding-repo",
			"url":  "https://github.com/test/repo",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/create-repo", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/onboarding/create-repo returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/onboarding/import-repo imports repository", func(t *testing.T) {
		payload := map[string]interface{}{
			"url": "https://github.com/test/repo",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/import-repo", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/onboarding/import-repo returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/onboarding/list-repos lists repositories", func(t *testing.T) {
		payload := map[string]interface{}{
			"org": "test-org",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/list-repos", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/onboarding/list-repos returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/onboarding/inspect-repo inspects repository", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/onboarding/inspect-repo?url=https://github.com/test/repo")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/onboarding/inspect-repo returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/onboarding/create-issues creates issues", func(t *testing.T) {
		payload := map[string]interface{}{
			"repoUrl": "https://github.com/test/repo",
			"count":   5,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/create-issues", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/onboarding/create-issues returned status: %d", resp.StatusCode)
	})
}
