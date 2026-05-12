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

		assertStatusIn(t, resp, "GET /api/setup-wizard/steps", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/setup-wizard/steps")
	})

	t.Run("POST /api/onboarding/create-repo rejects unauth or invalid request", func(t *testing.T) {
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

		assertStatusIn(t, resp, "POST /api/onboarding/create-repo", http.StatusBadRequest)
	})

	t.Run("POST /api/onboarding/import-repo rejects unauth or invalid request", func(t *testing.T) {
		payload := map[string]interface{}{
			"url": "https://github.com/test/repo",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/import-repo", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/onboarding/import-repo", http.StatusBadRequest)
	})

	t.Run("POST /api/onboarding/list-repos rejects unauth or invalid request", func(t *testing.T) {
		payload := map[string]interface{}{
			"org": "test-org",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/onboarding/list-repos", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/onboarding/list-repos", http.StatusBadRequest)
	})

	t.Run("GET /api/onboarding/inspect-repo rejects unauth or invalid request", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/onboarding/inspect-repo?url=https://github.com/test/repo")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/onboarding/inspect-repo", http.StatusBadRequest)
	})

	t.Run("POST /api/onboarding/create-issues rejects unauth or invalid request", func(t *testing.T) {
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

		assertStatusIn(t, resp, "POST /api/onboarding/create-issues", http.StatusBadRequest)
	})
}
